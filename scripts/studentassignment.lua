-- called with email asstID

if not ARGV[1] or ARGV[1] == '' then
	error('studentassignment: missing email address')
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error('studentassignment: missing assignment ID')
end
local asstID = ARGV[2]

local getAssignmentListingGeneric = function(assignment)
	local result = {}
	local problem = redis.call('get', 'assignment:'..assignment..':problem')
	if problem == '' then
		error('getAssignmentListingGeneric: assignment '..assignment..' mapped to blank problem ID')
	end

	result.Name = redis.call('get', 'problem:'..problem..':name')
	result.ID = tonumber(assignment)
	result.Open = tonumber(redis.call('get', 'assignment:'..assignment..':open'))
	result.Close = tonumber(redis.call('get', 'assignment:'..assignment..':close'))
	result.Active = true
	if redis.call('sismember', 'index:assignments:past', assignment) == 1 then
		result.Active = false
	end
	result.ForCredit = redis.call('get', 'assignment:'..assignment..':forcredit') == 'true'

	return result
end

local getAssignmentListingStudent = function(course, assignment, email, result)
	-- get the student solution id
	local solution = redis.call('hget', 'student:'..email..':solutions:'..course, assignment)
	if not solution or solution == '' then
		result.Attempts = 0
		result.ToBeGraded = 0
		result.Passed = false
		return
	end

	result.Attempts = tonumber(redis.call('llen', 'solution:'..solution..':submissions'))
	result.ToBeGraded = result.Attempts - tonumber(redis.call('llen', 'solution:'..solution..':graded'))
	result.Passed = redis.call('get', 'solution:'..solution..':passed') == 'true'
end

-- get the course
local course = redis.call('get', 'assignment:'..asstID..':course')
if not course or course == '' then
	error('studentassignment: no such assignment')
end

-- make sure this is an active course and the student is enrolled
if redis.call('sismember', 'index:courses:active', course) == 0 then
	error('['..course..'] is not an active course')
end
if redis.call('sismember', 'course:'..course..':students', email) == 0 then
	error(email..' is not enrolled in '..course)
end

-- make sure this is an active assignment
if redis.call('sismember', 'course:'..course..':assignments:active', asstID) == 0 then
	error(asstID..' is not an active assignment')
end

local problem = redis.call('get', 'assignment:'..asstID..':problem')

local result = {}
result.Assignment = getAssignmentListingGeneric(asstID)
getAssignmentListingStudent(course, asstID, email, result.Assignment)
result.CourseName = redis.call('get', 'course:'..course..':name')
result.CourseTag = course
local problemtypetag = redis.call('get', 'problem:'..problem..':type')

if redis.call('hexists', 'problem:types', problemtypetag) == 0 then
	error('Problem is of unknown type: '..problemtypetag)
end
result.ProblemType = cjson.decode(redis.call('hget', 'problem:types', problemtypetag))

-- WARNING: this is the raw problem; it must be filtered against
-- the fieldlist before being handed to the student
result.ProblemData = cjson.decode(redis.call('get', 'problem:'..problem..':data'))

result.Passed = false

-- see if the student has an attempt
local solution = redis.call('hget', 'student:'..email..':solutions:'..course, asstID)
if solution and tonumber(solution) > 0 then
	result.Attempt = cjson.decode(redis.call('lindex', 'solution:'..solution..':submissions', -1))
	if redis.call('get', 'solution:'..solution..':passed') == 'true' then
		result.Passed = true
	end
end

return cjson.encode(result)
