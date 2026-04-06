package driver

var bloomFilterAddScript = `
local itemExists = 1
for k, v in ipairs(ARGV) do
	if k > 1 and redis.call("GETBIT", KEYS[1], v) == 0 then
		itemExists = 0
		break
	end
end
if itemExists == 1 then
	return 0
end

for k, v in ipairs(ARGV) do
	if k > 1 then
		redis.call("SETBIT", KEYS[1], v, 1)
	end
end

local ttl = tonumber(ARGV[1])
if ttl > 0 then
	-- 检查key是否已经设置了过期时间
	local keyTTL = redis.call("TTL", KEYS[1])
	-- 如果key不存在或没有设置过期时间（TTL返回-1），则设置过期时间
	if keyTTL <= -1 then
		redis.call("EXPIRE", KEYS[1], ttl)
	end
end
return 1
`

var bloomFilterInsertScript = `
local keyExists = redis.call("EXISTS", KEYS[1])

local itemExists = 1
for k, v in ipairs(ARGV) do
	if k > 1 and redis.call("GETBIT", KEYS[1], v) == 0 then
		itemExists = 0
		break
	end
end

if itemExists == 0 then
	for k, v in ipairs(ARGV) do
		if k > 1 then
			redis.call("SETBIT", KEYS[1], v, 1)
		end
	end
end

local ttl = tonumber(ARGV[1])
if ttl > 0 then
	-- 检查key是否已经设置了过期时间
	local keyTTL = redis.call("TTL", KEYS[1])
	-- 如果key不存在或没有设置过期时间（TTL返回-1），则设置过期时间
	if keyTTL <= -1 then
		redis.call("EXPIRE", KEYS[1], ttl)
	end
end

if itemExists == 0 then
	return 1
end

return 0
`

var bloomFilterExistsCheckScript = `
local exists = redis.call("EXISTS", KEYS[1])
if exists == 0 then
	return 0 
end

for k, v in ipairs(ARGV) do 
	if redis.call("GETBIT", KEYS[1], v) == 0 then
		exists = 0
		break
	end
end
return exists
`

var bloomFilterMSExistsCheckScript = `
local exists = redis.call("EXISTS", KEYS[1])
if exists == 0 then
    return -1
end

local chunkSize = tonumber(ARGV[1])
local result = {}
local chunkIndex = 1

for i = 2, #ARGV, chunkSize do
    local endIndex = i + chunkSize - 1
    if endIndex > #ARGV then
        endIndex = #ARGV
    end
	local exists = 1
    for j = i, endIndex do
        local bitResult = redis.call("GETBIT", KEYS[1], ARGV[j])
        if bitResult == 0 then
            exists = 0
            break
        end
    end
	
	table.insert(result, exists)
end
return result
`

// bloomFilterSlidingExistsCheckScript 滑动窗口布隆过滤器检查脚本
// KEYS: 多个窗口的 key (KEYS[1], KEYS[2], ...)
// ARGV: 元素的位置索引数组
// 返回: 1 如果元素存在于任何一个窗口中, 0 如果不存在于任何窗口
var bloomFilterSlidingExistsCheckScript = `
-- 遍历所有窗口
for windowIdx = 1, #KEYS do
	local windowKey = KEYS[windowIdx]
	local keyExists = redis.call("EXISTS", windowKey)
	
	-- 如果窗口 key 存在，检查元素是否在其中
	if keyExists == 1 then
		-- 检查所有位置是否都为 1
		local allBitsSet = 1
		for locIdx = 1, #ARGV do
			local bitValue = redis.call("GETBIT", windowKey, ARGV[locIdx])
			if bitValue == 0 then
				allBitsSet = 0
				break
			end
		end
		
		-- 如果所有位都设置了，说明元素存在于这个窗口
		if allBitsSet == 1 then
			return 1
		end
	end
end

-- 所有窗口都没有找到
return 0
`

// bloomFilterSlidingMSExistsCheckScript 滑动窗口布隆过滤器批量检查脚本
// KEYS: 多个窗口的 key (KEYS[1], KEYS[2], ...)
// ARGV: [chunkSize, loc1, loc2, ..., loc1, loc2, ...]
// ARGV[1] 是每个元素的位置数量（chunkSize），后续是所有元素的位置索引
// 返回: 数组，每个元素对应一个 item 的存在状态 (1=存在, 0=不存在)
var bloomFilterSlidingMSExistsCheckScript = `
local chunkSize = tonumber(ARGV[1])
local result = {}

-- 遍历每个元素（从 ARGV[2] 开始，每 chunkSize 个位置为一组）
for i = 2, #ARGV, chunkSize do
	local endIndex = i + chunkSize - 1
	if endIndex > #ARGV then
		endIndex = #ARGV
	end
	
	local itemExists = 0
	
	-- 遍历所有窗口，检查当前元素是否存在于任何一个窗口
	for windowIdx = 1, #KEYS do
		local windowKey = KEYS[windowIdx]
		local keyExists = redis.call("EXISTS", windowKey)
		
		-- 如果窗口存在，检查元素是否在其中
		if keyExists == 1 then
			local allBitsSet = 1
			-- 检查当前元素的所有位置
			for j = i, endIndex do
				local bitValue = redis.call("GETBIT", windowKey, ARGV[j])
				if bitValue == 0 then
					allBitsSet = 0
					break
				end
			end
			
			-- 如果在这个窗口中找到了元素
			if allBitsSet == 1 then
				itemExists = 1
				break
			end
		end
	end
	
	table.insert(result, itemExists)
end

return result
`
