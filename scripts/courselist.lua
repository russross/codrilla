-- called with: email

if not ARGV[1] or ARGV[1] == '' then
	error('courselist: missing email address')
end
local email = ARGV[1]

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

-- verify that the user is a valid instructor
if redis.call('sismember', 'index:instructors:all', email) == 0 then
	error('user '..email..' is not an instructor')
end

local result = {}

-- get the list of current courses
local courseList = redis.call('smembers', 'instructor:'..email..':courses')
table.sort(courseList)
for _, courseTag in ipairs(courseList) do
	local elt = {}
	elt.Name = redis.call('get', 'course:'..courseTag..':name')
	elt.Close = redis.call('get', 'course:'..courseTag..':close')
	elt.Instructors = redis.call('get', 'course:'..courseTag..':instructors')
	elt.Students = redis.call('get', 'course:'..courseTag..':students')

	elt.OpenAssignments = {}
	for _, asstID in ipairs(redis.call('smembers', 'course:'..courseTag..':assignments:active')) do
		table.insert(elt.OpenAssignments, getAssignmentListingGeneric(asstID))
	end
	elt.ClosedAssignments = {}
	for _, asstID in ipairs(redis.call('smembers', 'course:'..courseTag..':assignments:past')) do
		table.insert(elt.ClosedAssignments, getAssignmentListingGeneric(asstID))
	end
	elt.FutureAssignments = {}
	for _, asstID in ipairs(redis.call('smembers', 'course:'..courseTag..':assignments:future')) do
		table.insert(elt.FutureAssignments, getAssignmentListingGeneric(asstID))
	end

	-- sort the assignments by deadline
	local compare = function (a, b)
		return a.Close < a.Close
	end
	table.sort(elt.OpenAssignments, compare)
	table.sort(elt.ClosedAssignments, compare)
	table.sort(elt.FutureAssignments, compare)
	
	table.insert(result, elt)
end

return cjson.encode(result)
