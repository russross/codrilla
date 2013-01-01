-- called with: email name tag

if not ARGV[1] or ARGV[1] == '' then
	error("addstudenttocourse: missing email address")
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("addstudenttocourse: missing name")
end
local name = ARGV[2]
if not ARGV[3] or ARGV[3] == '' then
	error("addstudenttocourse: missing course tag")
end
local tag = ARGV[3]

-- make sure the course is active
if redis.call('sismember', 'index:courses:active', tag) == 0 then
	error("addstudenttocourse: course is not active/does not exist")
end

-- check if the student is new
if redis.call('sismember', 'index:students:active', email) == 1 then
	-- do nothing
elseif redis.call('sismember', 'index:students:inactive', email) == 1 then
	-- make this student active
	redis.call('smove', 'index:students:inactive', 'index:students:active', email)
else
	-- create the student
	redis.call('set', 'student:'..email..':name', name)
	redis.call('sadd', 'index:students:active', email)
end

-- is the student already in the course?
if redis.call('sismember', 'course:'..tag..':students', email) == 1 then
	-- already in the course
	return 'noop'
end

-- add the student to the course
redis.call('sadd', 'course:'..tag..':students', email)
redis.call('sadd', 'student:'..email..':courses', tag)

return ''
