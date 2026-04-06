package cache

import redisV8 "github.com/go-redis/redis/v8"

// 当key存在才进行incr操作, 过期时间不主动延长
var tryHashIncrByScript = redisV8.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
    local result = {}
    for i = 1, #ARGV, 2 do
        local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
        table.insert(result, ARGV[i])
        table.insert(result, value)
    end
   	 
    return result
else
    return -1
end
`)

// 当key存在才进行incr操作, 并主动延长过期时间
// 注意第二个参数会作为过期时间处理
var tryHashIncrByWithExtendScript = redisV8.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
   local result = {}
    for i = 2, #ARGV, 2 do
        local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
        table.insert(result, ARGV[i])
        table.insert(result, value)
    end
    redis.call('expire', KEYS[1], tonumber(ARGV[1]))
    return result
else
    return -1
end
`)

var hashIncrByScript = redisV8.NewScript(`
local result = {}
for i = 1, #ARGV, 2 do
	local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
	table.insert(result, ARGV[i])
	table.insert(result, value)
end

return result
`)

var hashIncrByWithExtendScript = redisV8.NewScript(`
local result = {}
for i = 2, #ARGV, 2 do
	local value =  redis.call("HINCRBY", KEYS[1], ARGV[i], tonumber(ARGV[i + 1]))
	table.insert(result, ARGV[i])
	table.insert(result, value)
end
redis.call('expire', KEYS[1], tonumber(ARGV[1]))
return result
`)

// 当key存在才进行set操作, 过期时间不主动延长
var tryHashSetScript = redisV8.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	local args = {}
    for i = 1, #ARGV, 2 do
        table.insert(args, ARGV[i])
        table.insert(args, ARGV[i + 1])
    end
    redis.call("HMSET", KEYS[1], unpack(args))
    return 1
else
	return -1
end
`)

// 当key存在才进行set操作, 并主动延长过期时间
// 注意第二个参数会作为过期时间处理
var tryHashSetWithExtendScript = redisV8.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
	local args = {}
    for i = 2, #ARGV, 2 do
        table.insert(args, ARGV[i])
        table.insert(args, ARGV[i + 1])
    end
    redis.call("HMSET", KEYS[1], unpack(args))
	redis.call('expire', KEYS[1], tonumber(ARGV[1]))
	return 1
else
	return -1
end
`)
