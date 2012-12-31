-- called with: email name

if not ARGV[1] or ARGV[1] == '' then
	error("createcourse: missing email")
end
local email = ARGV[1]
if not ARGV[2] or ARGV[2] == '' then
	error("createcourse: missing name")
end
local name = ARGV[2]

-- make sure this instructor does not already exist
if redis.call('sismember', 'index:instructors:active', email) == 1 then
	error('createinstructor: instructor already exists and is active')
end
if redis.call('sismember', 'index:instructors:inactive', email) == 1 then
	error('createinstructor: instructor already exists and is inactive')
end

-- add this instructor
redis.call('set', 'instructor:'..email..':name', name)
redis.call('sadd', 'index:instructors:inactive', email)

return ''
