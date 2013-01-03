-- called with: timestamp
-- updates time-sensitive data

if not ARGV[1] or ARGV[1] == '' then
	error("cron: missing timestamp")
end
local now = ARGV[1]

-- open any assignments ready to go
local lst = redis.call('zrangebyscore', 'index:assignments:futurebyopen', '-inf', now)
for index, id in ipairs(lst) do
	local course = redis.call('get', 'assignment:'..id..':course')
	local close = redis.call('get', 'assignment:'..id..':close')
	redis.call('zadd', 'index:assignments:activebyclose', close, id)
	redis.call('smove', 'course:'..course..':assignments:future',
						'course:'..course..':assignments:active', id)
	redis.call('zrem', 'course:'..course..':assignments:futurebyopen', id)
end
redis.call('zremrangebyscore', 'index:assignments:futurebyopen', '-inf', now)


-- close any assignments past their time
local lst = redis.call('zrangebyscore', 'index:assignments:activebyclose', '-inf', now)
for index, id in ipairs(lst) do
	local course = redis.call('get', 'assignment:'..id..':course')
	redis.call('sadd', 'index:assignments:past', id)
	redis.call('smove', 'course:'..course..':assignments:active',
						'course:'..course..':assignments:past', id)
end
redis.call('zremrangebyscore', 'index:assignments:activebyclose', '-inf', now)

-- close any courses past their time
local lst = redis.call('zrangebyscore', 'index:courses:activebyclose', '-inf', now)
for index, course in ipairs(lst) do
	-- refuse to close a course with active/future assignments
	if redis.call('scard', 'course:'..course..':assignments:active') > 0 then
		error('Cannot close course '..course..' with active assignements')
	end
	if redis.call('scard', 'course:'..course..':assignments:future') > 0 then
		error('Cannot close course '..course..' with future assignements')
	end

	-- close the course
	redis.call('smove', 'index:courses:active', 'index:courses:inactive', course)

	-- close out all of the students
	local students = redis.call('smembers', 'course:'..course..':students')
	for i, email in ipairs(students) do
		redis.call('smove', 'student:'..email..':courses',
							'student:'..email..':oldcourses', course)

		-- was that the last active course for this student?
		if redis.call('scard', 'student:'..email..':courses') == 0 then
			redis.call('smove', 'index:students:active', 'index:students:inactive', email)
		end
	end

	-- close out all of the instructors
	local instructors = redis.call('smembers', 'course:'..course..':instructors')
	for i, email in ipairs(instructors) do
		redis.call('smove', 'instructor:'..email..':courses',
							'instructor:'..email..':oldcourses', course)

		-- was that the last active course for this instructor?
		if redis.call('scard', 'instructor:'..email..':courses') == 0 then
			redis.call('smove', 'index:instructors:active', 'index:instructors:inactive', email)
		end
	end
end
redis.call('zremrangebyscore', 'index:courses:activebyclose', '-inf', now)

return ''
