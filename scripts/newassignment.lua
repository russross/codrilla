-- called with: email courseTag problemID open close forCredit

if not ARGV[1] or ARGV[1] == '' then
	error('newassignment: missing email address')
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error('newassignment: missing course tag')
end
local courseTag = ARGV[2]
if not ARGV[3] or ARGV[3] == '' then
	error('newassignment: missing problem ID')
end
local problemID = ARGV[3]
if not ARGV[4] or ARGV[4] == '' then
	error('newassignment: missing open time')
end
local openTime = ARGV[4]
if not ARGV[5] or ARGV[5] == '' then
	error('newassignment: missing close time')
end
local closeTime = ARGV[5]
if not ARGV[6] or ARGV[6] == '' then
	error('newassignment: missing forcredit')
end
local forcredit = (ARGV[6] == 'true')

-- make sure this is an active course
if redis.call('sismember', 'index:courses:active', courseTag) == 0 then
	error('course '..courseTag..' is not active or does not exist')
end

-- make sure this user is an instructor for this course (or is an admin)
if redis.call('sismember', 'index:administrators', email) == 0 then
	if redis.call('sismember', 'course:'..courseTag..':instructors', email) == 0 then
		error('user '..email..' is not an instructor for '..courseTag)
	end
end

-- make sure the problem exists
if redis.call('sismember', 'index:problems:all', problemID) == 0 then
	error('problem '..problemID..' does not exist')
end

-- set it up as a future problem
-- if it should already be open, it will be opened on the next cron
local id = redis.call('incr', 'assignment:counter')
redis.call('set', 'assignment:'..id..':course', courseTag)
redis.call('set', 'assignment:'..id..':problem', problemID)
redis.call('set', 'assignment:'..id..':forcredit', forCredit)
redis.call('set', 'assignment:'..id..':open', openTime)
redis.call('set', 'assignment:'..id..':close', closeTime)
redis.call('zadd', 'index:assignments:futurebyopen', openTime, id)
redis.call('zadd', 'course:'..courseTag..':assignments:futurebyopen', openTime, id)
redis.call('sadd', 'course:'..courseTag..':assignments:future', id)
redis.call('sadd', 'problem:'..problemID..':usedby', courseTag)

return ''
