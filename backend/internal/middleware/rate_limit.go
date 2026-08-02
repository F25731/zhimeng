package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/F25731/zhimeng/backend/internal/httpx"
	"github.com/gin-gonic/gin"
)

type rateBucket struct {
	count   int
	resetAt time.Time
}

// RateLimitIP provides a small process-local guard for public endpoints. Card and
// session state is still enforced transactionally in PostgreSQL.
func RateLimitIP(limit int, window time.Duration) gin.HandlerFunc {
	var mu sync.Mutex
	buckets := map[string]rateBucket{}
	return func(c *gin.Context) {
		now := time.Now()
		key := c.ClientIP()
		mu.Lock()
		bucket := buckets[key]
		if bucket.resetAt.Before(now) {
			bucket = rateBucket{resetAt: now.Add(window)}
		}
		bucket.count++
		buckets[key] = bucket
		if len(buckets) > 4096 {
			for bucketKey, item := range buckets {
				if item.resetAt.Before(now) {
					delete(buckets, bucketKey)
				}
			}
		}
		mu.Unlock()

		if bucket.count > limit {
			seconds := int(time.Until(bucket.resetAt).Seconds())
			if seconds < 1 {
				seconds = 1
			}
			c.Header("Retry-After", strconv.Itoa(seconds))
			httpx.Error(c, http.StatusTooManyRequests, 42900, "请求过于频繁，请稍后重试")
			c.Abort()
			return
		}
		c.Next()
	}
}
