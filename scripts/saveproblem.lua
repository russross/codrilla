-- called with instructoremail problemJson

if not ARGV[1] or ARGV[1] == '' then
	error("saveproblem: missing instructor email address")
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("saveproblem: missing problem JSON")
end
local problemJson = ARGV[2]

local problem = cjson.decode(problemJson)

if problem.Name == '' then
	error('saveproblem: missing Name field')
end
if #problem.Tags == 0 then
	error('saveproblem: must have at least one tag')
end

-- make sure the type is recognized
if redis.call('hexists', 'grader:problemtypes', problem.Type) == 0 then
	error('saveproblem: unrecognized problem type')
end

-- allocate an ID number if needed
local id = problem.ID
if not id or id < 1 then
	id = redis.call('incr', 'problem:counter')
	problem.ID = id
end

-- store basic fields
redis.call('set', 'problem:'..id..':name', problem.Name)
redis.call('set', 'problem:'..id..':type', problem.Type)
redis.call('set', 'problem:'..id..':data', cjson.encode(problem.Data))
redis.call('sadd', 'index:problems:all', id)

-- store tags
for i, tag in ipairs(problem.Tags) do
	-- is this a new tag?
	if redis.call('sismember', 'index:tags:all', tag) == 0 then
		-- create a new tag
		redis.call('set', 'tag:'..tag..':description', tag)
		redis.call('set', 'tag:'..tag..':priority', 0)
		redis.call('sadd', 'index:tags:all', tag)
		redis.call('zadd', 'index:tags:bypriority', 0, tag)
	end

	-- link the problem to the tag
	redis.call('sadd', 'problem:'..id..':tags', tag)

	-- link the tag to the problem
	redis.call('sadd', 'tag:'..tag..':problems', id)
end

-- re-encode the problem and return it
return cjson.encode(problem)
