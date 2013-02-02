-- called with: email, asstID

if not ARGV[1] or ARGV[1] == '' then
	error("getproblemtypeforsubmission: missing email address")
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("getproblemtypeforsubmission: missing asst ID")
end
local asstID = ARGV[2]

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

-- everything checks out, get the problem type description
local problemID = redis.call('get', 'assignment:'..asstID..':problem')
local problemTypeTag = redis.call('get', 'problem:'..problemID..':type')
local problemType = redis.call('hget', 'problem:types', problemTypeTag)

return problemType
