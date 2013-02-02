-- called with: email, asstID, data

if not ARGV[1] or ARGV[1] == '' then
	error("studentsubmit: missing email address")
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("studentsubmit: missing asst ID")
end
local asstID = ARGV[2]
if not ARGV[3] or ARGV[3] == '' then
	error("studentsubmit: missing submission data")
end
local data = ARGV[3]

-- check if the assignment exists/get the course tag
local courseTag = redis.call('get', 'assignment:'..asstID..':course')
if courseTag == '' then
	error('unknown assignment ID')
end

-- is this an active course?
if redis.call('sismember', 'index:courses:active', courseTag) == 0 then
	error('not an active course: '..courseTag)
end

-- is this an active assignment?
if redis.call('sismember', 'course:'..courseTag..':assignments:active', asstID) == 0 then
	error('not an active assignment')
end

-- is the student in this course?
if redis.call('sismember', 'course:'..courseTag..':students', email) == 0 then
	error('not a student in that course')
end

-- is this the first submission for this assignment?
local solID = redis.call('hget', 'student:'..email..':solutions:'..courseTag, asstID)
if not solID or solID == '' then
	solID = redis.call('incr', 'solution:counter')
	redis.call('set', 'solution:'..solID..':student', email)
	redis.call('set', 'solution:'..solID..':assignment', asstID)
	redis.call('set', 'solution:'..solID..':passed', 'false')
	redis.call('hset', 'student:'..email..':solutions:'..courseTag, asstID, solID)
end
if redis.call('get', 'solution:'..solID..':student') ~= email then
	error('solution '..solID..' has wrong email address for '..email)
end

-- store this submission
redis.call('rpush', 'solution:'..solID..':submissions', data)

-- trigger the grader
redis.call('sadd', 'solution:queue', solID)

return ''
