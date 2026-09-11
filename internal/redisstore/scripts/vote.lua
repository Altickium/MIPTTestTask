-- KEYS[1] gate, KEYS[2] dedup, KEYS[3] counters
-- ARGV[1] ttl seconds, ARGV[2..n] validated option IDs
if redis.call('GET', KEYS[1]) ~= 'active' then
    return -1
end
local inserted = redis.call('SET', KEYS[2], '1', 'NX', 'EX', ARGV[1])
if not inserted then
    return 0
end
redis.call('HINCRBY', KEYS[3], '__participants', 1)
for i = 2, #ARGV do
    redis.call('HINCRBY', KEYS[3], ARGV[i], 1)
end
redis.call('EXPIRE', KEYS[3], ARGV[1])
return 1
