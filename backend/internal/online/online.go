package online

import (
	"sync"
	"time"
)

// 在线用户跟踪：鉴权中间件在每次有效请求时刷新 lastSeen，
// Count 统计最近 within 内有活动的用户数，并顺带清理过期条目。
var (
	mu       sync.Mutex
	lastSeen = map[uint]time.Time{}
)

// Track 记录用户最近一次有效请求时间
func Track(userID uint) {
	if userID == 0 {
		return
	}
	mu.Lock()
	lastSeen[userID] = time.Now()
	mu.Unlock()
}

// Count 返回最近 within 内有活动的在线用户数
func Count(within time.Duration) int {
	cutoff := time.Now().Add(-within)
	mu.Lock()
	defer mu.Unlock()
	n := 0
	for id, t := range lastSeen {
		if t.Before(cutoff) {
			delete(lastSeen, id)
			continue
		}
		n++
	}
	return n
}
