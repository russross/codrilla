-- called with: email coursetag

if not ARGV[1] or ARGV[1] == '' then
    error('studentlistgrades: missing email address')
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
    error('studentlistgrades: missing course tag')
end
local courseTag = ARGV[2]

local getAssignmentListingGeneric = function(assignment)
    local result = {}
    local problem = redis.call('get', 'assignment:'..assignment..':problem')
    if not problem or problem == '' then
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

local main = function (email, courseTag)
    -- make sure this is an active student
    if redis.call('sismember', 'index:students:active', email) == 0 then
        error('studentlistgrades: not an active student')
    end

    -- make sure course is active
    if redis.call('sismember', 'index:courses:active', courseTag) == 0 then
        error('studentlistgrades: not an active course')
    end

    -- make sure student is enrolled in course
    if redis.call('sismember', 'student:'..email..':courses', courseTag) == 0 then
        error('studentlistgrades: student not enrolled in course')
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

    local response = {}
    for _, asstID in ipairs(assignments) do
        local assignment = getAssignmentListingGeneric(asstID)
        getAssignmentListingStudent(courseTag, asstID, email, assignment)
        table.insert(response, assignment)
    end

    -- sort the assignments by deadline
    local compare = function (a, b)
        return a.Close < a.Close
    end
    table.sort(response, compare)

    return response
end

return cjson.encode(main(email, courseTag))
