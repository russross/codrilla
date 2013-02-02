-- called with no arguments

-- anything to do?
while redis.call('scard', 'solution:queue') > 0 do
	-- pick a waiting solution randomly
	local solutionID = redis.call('srandmember', 'solution:queue')

	-- check how many submissions this solution has waiting
	local precount = redis.call('llen', 'solution:'..solutionID..':submissions')
	local postcount = redis.call('llen', 'solution:'..solutionID..':graded')
	if postcount >= precount then
		-- nothing to do, so remove this entry from the queue
		redis.call('srem', 'solution:queue', solutionID)
	else 
		-- get the next submission in line
		local submission = cjson.decode(redis.call('lindex', 'solution:'..solutionID..':submissions', postcount))
		local asst = redis.call('get', 'solution:'..solutionID..':assignment')
		local problemID = redis.call('get', 'assignment:'..asst..':problem')
		local problem = cjson.decode(redis.call('get', 'problem:'..problemID..':data'))
		local typeTag = redis.call('get', 'problem:'..problemID..':type')
		if redis.call('hexists', 'problem:types', typeTag) == 0 then
			error('unknown problem type: '..typeTag)
		end
		local problemType = cjson.decode(redis.call('hget', 'problem:types', typeTag))

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
