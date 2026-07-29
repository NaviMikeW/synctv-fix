package middlewares

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/synctv-org/synctv/internal/conf"
	"github.com/synctv-org/synctv/server/model"
	limiter "github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

const (
	maxLoginRateLimitBodySize = 4 << 10
	loginIPLimitMultiplier    = int64(5)
)

func NewLoginLimiter(config conf.LoginRateLimitConfig) (gin.HandlerFunc, error) {
	handler, _, _, err := newLoginLimiter(config)
	return handler, err
}

func newLoginLimiter(
	config conf.LoginRateLimitConfig,
) (gin.HandlerFunc, *limiter.Limiter, *limiter.Limiter, error) {
	if config.Limit <= 0 {
		return nil, nil, nil, errors.New("login rate limit must be greater than zero")
	}

	period, err := time.ParseDuration(config.Period)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("invalid login rate limit period: %w", err)
	}
	if period <= 0 {
		return nil, nil, nil, errors.New("login rate limit period must be greater than zero")
	}

	options := []limiter.Option{
		limiter.WithTrustForwardHeader(config.TrustForwardHeader),
	}
	if config.TrustedClientIPHeader != "" {
		options = append(
			options,
			limiter.WithClientIPHeader(config.TrustedClientIPHeader),
		)
	}

	rate := limiter.Rate{
		Period: period,
		Limit:  config.Limit,
	}
	accountLimiter := limiter.New(memory.NewStore(), rate)

	rate.Limit = multipliedLoginIPLimit(config.Limit)
	ipLimiter := limiter.New(memory.NewStore(), rate, options...)

	handler := func(ctx *gin.Context) {
		requestContext := ctx.Request.Context()
		ipKey := ipLimiter.GetIPKey(ctx.Request)
		ipContext, err := ipLimiter.Get(
			requestContext,
			ipKey,
		)
		if err != nil {
			ctx.AbortWithStatusJSON(
				http.StatusInternalServerError,
				model.NewAPIErrorStringResp("login rate limiter unavailable"),
			)
			return
		}
		if ipContext.Reached {
			setLoginRateLimitHeaders(ctx, ipContext)
			ctx.AbortWithStatusJSON(
				http.StatusTooManyRequests,
				model.NewAPIErrorStringResp("too many requests"),
			)
			return
		}

		// Read and restore the body only after the wider IP bucket permits the
		// request. This prevents blocked clients from creating arbitrary
		// per-account keys while preserving the body for the login handler.
		identity := loginRateLimitIdentity(ctx)
		accountContext, err := accountLimiter.Get(
			requestContext,
			ipKey+"|"+identity,
		)
		if err != nil {
			ctx.AbortWithStatusJSON(
				http.StatusInternalServerError,
				model.NewAPIErrorStringResp("login rate limiter unavailable"),
			)
			return
		}

		setLoginRateLimitHeaders(ctx, accountContext)

		if accountContext.Reached {
			ctx.AbortWithStatusJSON(
				http.StatusTooManyRequests,
				model.NewAPIErrorStringResp("too many requests"),
			)
			return
		}

		ctx.Next()
	}

	return handler, accountLimiter, ipLimiter, nil
}

func setLoginRateLimitHeaders(ctx *gin.Context, limitContext limiter.Context) {
	ctx.Header("X-RateLimit-Limit", strconv.FormatInt(limitContext.Limit, 10))
	ctx.Header("X-RateLimit-Remaining", strconv.FormatInt(limitContext.Remaining, 10))
	ctx.Header("X-RateLimit-Reset", strconv.FormatInt(limitContext.Reset, 10))
}

func multipliedLoginIPLimit(accountLimit int64) int64 {
	if accountLimit > math.MaxInt64/loginIPLimitMultiplier {
		return math.MaxInt64
	}
	return accountLimit * loginIPLimitMultiplier
}

func loginRateLimitIdentity(ctx *gin.Context) string {
	if ctx.Request.Body == nil {
		return "invalid"
	}

	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, maxLoginRateLimitBodySize+1))
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(body) > maxLoginRateLimitBodySize {
		return "invalid"
	}

	var credentials struct {
		Username string `json:"username"`
		Email    string `json:"email"`
	}
	if err = json.Unmarshal(body, &credentials); err != nil {
		return "invalid"
	}

	identity := strings.ToLower(strings.TrimSpace(credentials.Username))
	if identity == "" {
		identity = strings.ToLower(strings.TrimSpace(credentials.Email))
	}
	if identity == "" {
		return "invalid"
	}

	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}
