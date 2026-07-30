package middlewares

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/synctv-org/synctv/server/model"
	limiter "github.com/ulule/limiter/v3"
	mgin "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

type rateLimitKeyGetter func(*limiter.Limiter, *gin.Context) string

func NewLimiter(period time.Duration, limit int64, options ...limiter.Option) gin.HandlerFunc {
	return newLimiter(period, limit, nil, options...)
}

func newLimiter(
	period time.Duration,
	limit int64,
	keyGetter rateLimitKeyGetter,
	options ...limiter.Option,
) gin.HandlerFunc {
	instance := limiter.New(memory.NewStore(), limiter.Rate{
		Period: period,
		Limit:  limit,
	}, options...)
	if keyGetter == nil {
		keyGetter = func(instance *limiter.Limiter, c *gin.Context) string {
			return instance.GetIPKey(c.Request)
		}
	}

	return mgin.NewMiddleware(
		instance,
		mgin.WithKeyGetter(func(c *gin.Context) string {
			// The Gin adapter defaults to c.ClientIP(), which can trust spoofed
			// forwarding headers independently of limiter.Options. Use the
			// limiter's request parser so the configured trust policy applies.
			return keyGetter(instance, c)
		}),
		mgin.WithLimitReachedHandler(func(c *gin.Context) {
			c.JSON(http.StatusTooManyRequests, model.NewAPIErrorStringResp("too many requests"))
		}),
	)
}
