-- called with: tag name closetime instructor

if not ARGV[1] or ARGV[1] == '' then
	error("createcourse: missing tag name")
end
local tag = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("createcourse: missing full name")
end
local name = ARGV[2]
if not ARGV[3] or ARGV[3] == '' then
	error("createcourse: missing closing timestamp")
end
local closetime = ARGV[3]
if not ARGV[4] or ARGV[4] == '' then
	error("createcourse: missing instructor email")
end
local email = ARGV[4]

-- make sure this course does not already exist
if redis.call('sismember', 'index:courses:active', tag) == 1 then
	error('createcourse: course already exists and is active')
end
if redis.call('sismember', 'index:courses:inactive', tag) == 1 then
	error('createcourse: course already exists and is inactive')
end

-- make sure the instructor is known and make him/her active
if redis.call('sismember', 'index:instructors:active', email) == 1 then
	-- do nothing
elseif redis.call('sismember', 'index:instructors:inactive', email) == 1 then
	-- make this instructor active
	redis.call('smove', 'index:instructors:inactive', 'index:instructors:active', email)
else
	-- no such instructor
	error('createcourse: instructor not found')
end

-- add this course to the list of active courses for the instructor
redis.call('sadd', 'instructor:'..email..':courses', tag)

-- add the course
redis.call('set', 'course:'..tag..':'..'name', name)
redis.call('set', 'course:'..tag..':'..'close', closetime)
redis.call('sadd', 'course:'..tag..':'..'instructors', email)
redis.call('sadd', 'index:courses:active', tag)
redis.call('zadd', 'index:courses:activebyclose', closetime, tag)

return ''
