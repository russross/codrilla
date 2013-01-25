-- called with: email coursetag

if not ARGV[1] or ARGV[1] == '' then
	error('coursegrades: missing email address')
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error('coursegrades: missing course tag')
end
local courseTag = ARGV[2]

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
	result.ForCredit = redis.call('get', 'assignment:'..assignment..':forcredit') == 'true'

	return result
end

local getAssignmentListingStudent = function(course, assignment, email, result)
	-- get the student solution id
	local solution = redis.call('hget', 'student:'..email..':solutions:'..course, assignment)
	if solution == '' then
		return nil
	end

	result.Attempts = tonumber(redis.call('llen', 'solution:'..solution..':submissions'))
	result.ToBeGraded = result.Attempts - tonumber(redis.call('llen', 'solution:'..solution..':graded'))
	result.Passed = redis.call('hget', 'student:'..email..':solutions:'..course, assignment) == 'true'
end

-- make sure this is a valid course
if redis.call('sismember', 'index:courses:all', courseTag) == 0 then
	error('not a valid course')
end

-- make sure this is an instructor for this course
if redis.call('sismember', 'index:administrators', email) == 0 then
	if redis.call('sismember', 'course:'..courseTag..':instructors', email) == 0 then
		error('user '..email..' is not an instructor for '..courseTag)
	end
end

-- get the list of closed assignments and open assignments
local closed = redis.call('smembers', 'course:'..courseTag..':assignments:past')
local open = redis.call('smembers', 'course:'..courseTag..':assignments:active')
-- merge the two lists
local assignments = {}
for _, v in ipairs(closed) do
	table.insert(assignments, v)
end
for _, v in ipairs(open) do
	table.insert(assignments, v)
end

local result = {}

-- assignment sorting function
local compare = function (a, b)
	return a.Close < a.Close
end

-- get a list of students
local students = redis.call('course:'..courseTag..':students')
table.sort(students)
for _, student in ipairs(students) do
	local elt = {}
	for _, asstID in ipairs(assignments) do
		local assignment = getAssignmentListingGeneric(asstID)
		getAssignmentListingStudent(courseTag, asstID, student, assignment)
		table.insert(elt, assignment)
	end

	-- sort the assignments by deadline
	table.sort(elt, compare)

	table.insert(result, elt)
end

return cjson.encode(result)
