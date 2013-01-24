-- called with no parameters

local result = {}

local getProblem = function (id)
	local problem = {}
	problem.ID = tonumber(id)
	problem.Name = redis.call('get', 'problem:'..id..':name')
	problem.Type = redis.call('get', 'problem:'..id..':type')
	problem.Tags = redis.call('smembers', 'problem:'..id..':tags')
	problem.UsedBy = redis.call('smembers', 'problem:'..id..':usedby')
	return problem
end

-- get the list of all tags in priority order
local lst = redis.call('zrevrangebyscore', 'index:tags:bypriority', '+inf', '-inf')
for i, tag in ipairs(lst) do
	local elt = {}
	elt.Tag = tag
	elt.Description = redis.call('get', 'tag:'..tag..':description')
	elt.Priority = tonumber(redis.call('get', 'tag:'..tag..':priority'))
	elt.Problems = {}
	for i, id in ipairs(redis.call('smembers', 'tag:'..tag..':problems')) do
		table.insert(elt.Problems, getProblem(id))
	end
	table.insert(result, elt)
end

return result
