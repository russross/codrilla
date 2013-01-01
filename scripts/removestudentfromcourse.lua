-- called with: email tag

if not ARGV[1] or ARGV[1] == '' then
	error("removestudenttocourse: missing email address")
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("removestudentfromcourse: missing course tag")
end
local tag = ARGV[2]

-- make sure the student exists and is active
if redis.call('sismember', 'index:students:active', email) == 0 then
	error("removestudentfromcourse: student is not active/does not exist")
end

-- make sure the course is active
if redis.call('sismember', 'index:courses:active', tag) == 0 then
	error("removestudentfromcourse: course is not active/does not exist")
end

-- make sure the the student is in the course
if redis.call('sismember', 'course:'..tag..':students', email) == 0 then
	-- not in the course
	error("removestudentfromcourse: student is not in the course")
end

-- remove the student from the course
redis.call('srem', 'course:'..tag..':students', email)
redis.call('srem', 'student:'..email..':courses', tag)

-- did this make the student inactive?
if redis.call('scard', 'student:'..email..':courses') == 0 then
	redis.call('smove', 'index:students:active', 'index:students:inactive', email)
end

return ''
