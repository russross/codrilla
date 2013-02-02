-- called with no arguments

-- anything to do?
while redis.call('scard', 'queue:solution:waiting') > 0 do
	-- pick a waiting solution randomly
	local solutionID = redis.call('srandmember', 'queue:solution:waiting')

	-- check how many submissions this solution has waiting
	local precount = redis.call('llen', 'solution:'..solutionID..':submissions')
	local postcount = redis.call('llen', 'solution:'..solutionID..':graded')
	if postcount >= precount then
		-- nothing to do, so remove this entry from the queue
		redis.call('srem', 'queue:solution:waiting', solutionID)
	else 
		-- get the next submission in line
		local submission = cjson.decode(redis.call('lindex', 'solution:'..solutionID..':submissions', postcount))
		local asst = redis.call('get', 'solution:'..solutionID..':assignment')
		local problemID = redis.call('get', 'assignment:'..asst..':problem')
		local problem = cjson.decode(redis.call('get', 'problem:'..problemID..':data'))
		local typeTag = redis.call('get', 'problem:'..problemID..':type')
		local problemType = cjson.decode(redis.call('hget', 'grader:problemtypes', typeTag))

		local result = {}
		result.SolutionID = tonumber(solutionID)
		result.ProblemType = problemType
		result.ProblemData = problem
		result.Attempt = submission
		return cjson.encode(result)
	end
end

-- nothing to do
return ''
