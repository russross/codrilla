-- called with: id

if not ARGV[1] or ARGV[1] == '' then
	error('problemget: missing id')
end
local id = ARGV[1]

-- make sure it exists
if redis.call('sismember', 'index:problems:all', id) == 0 then
	error('No such problem')
end

local result = {}
result.ID = tonumber(id)
result.Name = redis.call('get', 'problem:'..id..':name')
result.Type = redis.call('get', 'problem:'..id..':type')
result.Tags = redis.call('smembers', 'problem:'..id..':tags')
result.Data = cjson.decode(redis.call('get', 'problem:'..id..':data'))

return cjson.encode(result)
