-- called with: email courseTag

if not ARGV[1] or ARGV[1] == '' then
	error("dropstudent: missing email address")
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("dropstudent: missing course tag")
end
local courseTag = ARGV[2]

-- make sure the course is active
if redis.call('sismember', 'index:courses:active', courseTag) == 0 then
	error("dropstudent: course is not active/does not exist")
end

-- is the student in the course?
if redis.call('sismember', 'course:'..courseTag..':students', email) == 0 then
	-- not enrolled
	error('student is not enrolled in this course')
end

-- remove the student
redis.call('srem', 'course:'..courseTag..':students', email)
redis.call('srem', 'student:'..email..':courses', courseTag)

-- was that the last course the student was enrolled in?
if redis.call('scard', 'student:'..email..':courses') == 0 then
	redis.call('smove', 'index:students:active', 'index:students:inactive', email)
end

return ''
