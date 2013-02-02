-- called with: solutionID resultData

if not ARGV[1] or ARGV[1] == '' then
	error('missing solutionID')
end
local solutionID = ARGV[1]

if not ARGV[2] or ARGV[2] == '' then
	error('missing resultData')
end
local resultData = ARGV[2]

-- make sure the resultData is valid JSON
local data = cjson.decode(resultData)

-- make sure this solutionID exists
if redis.call('exists', 'solution:'..solutionID..':student') == 0 then
	error('invalid solutionID: '..solutionID)
end

-- check how many submissions this solution has waiting
local precount = redis.call('llen', 'solution:'..solutionID..':submissions')
local postcount = redis.call('llen', 'solution:'..solutionID..':graded')
if postcount >= precount then
	error('no submissions waiting for results')
end
redis.call('rpush', 'solution:'..solutionID..':graded', resultData)

local passed = 'false'
if data.Passed then passed = 'true' end
redis.call('set', 'solution:'..solutionID..':passed', passed)

-- is that the last submission for this solution?
if precount <= postcount+1 then
	redis.call('srem', 'solution:queue', solutionID)
end

return ''
