package cache

const (
	// SafeDelScript is the script for safe deletion with value check
	safeDelScript = `
local key_value = redis.call('GET', KEYS[1])

if key_value == ARGV[1] then
    redis.call('DEL', KEYS[1])
    return 1
else
    return 0
end
`

	// HMSetScript is the script for HMSET with existence check and TTL options
	hMSetScript = `
local key = KEYS[1]
local checkExists = tonumber(ARGV[1])
local extendTTL = tonumber(ARGV[2]) 
local ttl = tonumber(ARGV[3])
local exists = 0

if checkExists == 1 or extendTTL == 1 then
    exists = redis.call('EXISTS', key)
    if exists == 0 then
        return -1
    end
elseif ttl > 0 then
    exists = redis.call('EXISTS', key)
end

for i = 4, #ARGV, 2 do
    local field = ARGV[i]
    local value = ARGV[i + 1]
    redis.call('HSET', key, field, value)
end

if ttl > 0 then
    if extendTTL == 1 and exists == 1 then
        redis.call('EXPIRE', key, ttl)
    elseif exists == 0 then
        redis.call('EXPIRE', key, ttl)
    end
end

return 1
`

	// StringSetScript is the script for SET with existence check and TTL options
	stringSetScript = `
local key = KEYS[1]
local checkExists = tonumber(ARGV[1])
local extendTTL = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local value = ARGV[4]
local exists = 0

if checkExists == 1 or extendTTL == 1 then
    exists = redis.call('EXISTS', key)
    if exists == 0 then
        return -1
    end
elseif ttl > 0 then
    exists = redis.call('EXISTS', key)
end

if ttl > 0 then
    if extendTTL == 1 and exists == 1 then
		redis.call('SET', key, value, 'EX', ttl)
    elseif exists == 0 then
		redis.call('SET', key, value, 'EX', ttl)
    end
else
	redis.call('SET', key, value)
end

return 1
`

	// HashIncrByScript is the script for HINCRBY with existence check and TTL options
	hashIncrByScript = `
local key = KEYS[1]
local checkExists = tonumber(ARGV[1])
local extendTTL = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local field = ARGV[4]
local increment = tonumber(ARGV[5])
local exists = 0

if checkExists == 1 or extendTTL == 1 then
    exists = redis.call('EXISTS', key)
    if exists == 0 then
        return -1
    end
elseif ttl > 0 then
    exists = redis.call('EXISTS', key)
end

local result = redis.call('HINCRBY', key, field, increment)

if ttl > 0 then
    if extendTTL == 1 and exists == 1 then
        redis.call('EXPIRE', key, ttl)
    elseif exists == 0 then
        redis.call('EXPIRE', key, ttl)
    end
end

return result
`

	// IncrByScript is the script for INCRBY with existence check and TTL options
	incrByScript = `
local key = KEYS[1]
local checkExists = tonumber(ARGV[1])
local extendTTL = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local increment = tonumber(ARGV[4])
local exists = 0

if checkExists == 1 or extendTTL == 1 then
    exists = redis.call('EXISTS', key)
    if exists == 0 then
        return -1
    end
elseif ttl > 0 then
    exists = redis.call('EXISTS', key)
end

local result = redis.call('INCRBY', key, increment)

if ttl > 0 then
    if extendTTL == 1 and exists == 1 then
        redis.call('EXPIRE', key, ttl)
    elseif exists == 0 then
        redis.call('EXPIRE', key, ttl)
    end
end

return result
`

	// zsetScript is the script for ZADD with existence check and TTL options
	zsetScript = `
local key = KEYS[1]
local checkExists = tonumber(ARGV[1])
local extendTTL = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local exists = 0

if checkExists == 1 or checkExists == 2 or extendTTL == 1 then
    exists = redis.call('EXISTS', key)
    if (checkExists == 1 and exists == 0) or (checkExists == 2 and exists == 1) then
        return -1
    end
elseif ttl > 0 then
    exists = redis.call('EXISTS', key)
end

local args = {}
for i = 4, #ARGV, 2 do
    table.insert(args, ARGV[i]) -- score
    table.insert(args, ARGV[i + 1]) -- member
end

local result = redis.call('ZADD', key, unpack(args))

if ttl > 0 then
    if extendTTL == 1 and exists == 1 then
        redis.call('EXPIRE', key, ttl)
    elseif exists == 0 then
        redis.call('EXPIRE', key, ttl)
    end
end

return result
`

	// listPushScript is the script for LPUSH/RPUSH with existence check and TTL options
	listPushScript = `
local key = KEYS[1]
local checkExists = tonumber(ARGV[1])
local extendTTL = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local direction = ARGV[4]
local exists = 0

if checkExists == 1 or extendTTL == 1 then
    exists = redis.call('EXISTS', key)
    if exists == 0 then
        return -1
    end
elseif ttl > 0 then
    exists = redis.call('EXISTS', key)
end

local args = {}
for i = 5, #ARGV do
    table.insert(args, ARGV[i])
end

local result
if direction == 'LEFT' then
    result = redis.call('LPUSH', key, unpack(args))
else
    result = redis.call('RPUSH', key, unpack(args))
end

if ttl > 0 then
    if extendTTL == 1 and exists == 1 then
        redis.call('EXPIRE', key, ttl)
    elseif exists == 0 then
        redis.call('EXPIRE', key, ttl)
    end
end

return result
`

	// setScript is the script for SADD with existence check and TTL options
	setScript = `
local key = KEYS[1]
local checkExists = tonumber(ARGV[1])
local extendTTL = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local exists = 0

if checkExists == 1 or extendTTL == 1 then
    exists = redis.call('EXISTS', key)
    if exists == 0 then
        return -1
    end
elseif ttl > 0 then
    exists = redis.call('EXISTS', key)
end

local args = {}
for i = 4, #ARGV do
    table.insert(args, ARGV[i])
end

local result = redis.call('SADD', key, unpack(args))

if ttl > 0 then
    if extendTTL == 1 and exists == 1 then
        redis.call('EXPIRE', key, ttl)
    elseif exists == 0 then
        redis.call('EXPIRE', key, ttl)
    end
end

return result
`

	// zsetAddFixLenScript is the script for ZADD with existence check and TTL options
	zsetAddFixLenScript = `
-- 检查key是否存在
local exists = redis.call('EXISTS', KEYS[1]) == 1
local autoExtend = tonumber(ARGV[#ARGV])
local ttl = tonumber(ARGV[#ARGV - 1])

-- 如果key存在且不自动延长过期时间,则返回
if exists and autoExtend == 0 then
    ttl = 0
end

-- 添加新元素到有序集合
for i = 1, #ARGV - 4, 2 do
    redis.call('ZADD', KEYS[1], ARGV[i+1], ARGV[i])
end

-- 获取集合大小
local size = redis.call('ZCARD', KEYS[1])

-- 如果超出最大长度，删除多余的元素
local maxSize = tonumber(ARGV[#ARGV - 2])
if maxSize > 0 and size > maxSize then
    redis.call('ZREMRANGEBYRANK', KEYS[1], 0, size - maxSize - 1)
end

-- 设置过期时间
if ttl > 0 then
    redis.call('EXPIRE', KEYS[1], ttl)
end

return 1
`

	// setBitsScript 批量设置多个位并设置过期时间
	// KEYS[1] = key
	// ARGV[1] = TTL (秒)
	// ARGV[2...] = positions
	setBitsScript = `
for i = 2, #ARGV do
    redis.call('SETBIT', KEYS[1], ARGV[i], 1)
end
local ttl = redis.call('TTL', KEYS[1])
if ttl <= 0 and tonumber(ARGV[1]) > 0 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return 1
`
)
