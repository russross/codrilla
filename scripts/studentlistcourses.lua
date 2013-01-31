-- called with: email

if not ARGV[1] or ARGV[1] == '' then
	error('studentlistcourses: missing email address')
end
local email = ARGV[1]

local getCourseListing = function(course)
	local result = {}
	result.Instructors = {}
	result.OpenAssignments = {}

	-- get the course info
	result.Name = redis.call('get', 'course:'..course..':name')
	result.Tag = course
	result.Close = tonumber(redis.call('get', 'course:'..course..':close'))
	result.Instructors = redis.call('smembers', 'course:'..course..':instructors')
	table.sort(result.Instructors)

	return result
end

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
	result.Passed = redis.call('hget', 'student:'..email..':solutions:'..course, assignment) == 'true'
end

local main = function (email)
	-- make sure this is an active student
	if redis.call('sismember', 'index:students:active', email) == 0 then
		error('studentlistcourses: not an active student')
	end

	local response = {}
	response.Email = email
	response.Courses = {}
	response.Name = redis.call('get', 'student:'..email..':name')

	-- get the list of courses
	local courseList = redis.call('smembers', 'student:'..email..':courses')
	table.sort(courseList)
	for _, courseTag in ipairs(courseList) do
		local course = getCourseListing(courseTag)

		-- get the list of active assignments
		local assignments = redis.call('smembers', 'course:'..courseTag..':assignments:active')
		for _, asstID in ipairs(assignments) do
			local assignment = getAssignmentListingGeneric(asstID)
			getAssignmentListingStudent(courseTag, asstID, email, assignment)
			table.insert(course.OpenAssignments, assignment)
		end

		-- sort the assignments by deadline
		local compare = function (a, b)
			return a.Close < a.Close
		end
		table.sort(course.OpenAssignments, compare)

		-- if there are no active assignments, delete the entry
		-- so it does not show up in JSON as an empty object (vs empty array)
		if #course.OpenAssignments == 0 then
			course.OpenAssignments = nil
		end

		-- get the next assignment that will be posted
		local future = redis.call('zrange', 'course:'..courseTag..':assignments:futurebyopen', 0, 1)
		if #future == 0 then
			course.NextAssignment = nil
		else
			course.NextAssignment = getAssignmentListingGeneric(future[1])
		end

		-- add this course to the list
		table.insert(response.Courses, course)
	end

	return response
end

return cjson.encode(main(email))
