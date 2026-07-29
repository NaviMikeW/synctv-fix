package middlewares

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/synctv-org/synctv/internal/conf"
	limiter "github.com/ulule/limiter/v3"
)

func TestLoginLimiterOnlyLimitsLoginRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)

	loginLimiter, err := NewLoginLimiter(conf.LoginRateLimitConfig{
		Period: "1m",
		Limit:  2,
	})
	if err != nil {
		t.Fatalf("NewLoginLimiter() error = %v", err)
	}

	router := gin.New()
	router.POST("/api/user/login", loginLimiter, func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})
	router.GET("/api/playback", func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})

	for attempt := 1; attempt <= 2; attempt++ {
		response := requestLoginLimiterRoute(router, http.MethodPost, "/api/user/login")
		if response.Code != http.StatusNoContent {
			t.Fatalf("login attempt %d status = %d, want %d", attempt, response.Code, http.StatusNoContent)
		}
	}

	response := requestLoginLimiterRoute(router, http.MethodPost, "/api/user/login")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("limited login status = %d, want %d", response.Code, http.StatusTooManyRequests)
	}

	response = requestLoginLimiterRoute(router, http.MethodGet, "/api/playback")
	if response.Code != http.StatusNoContent {
		t.Fatalf("playback status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestLoginLimiterRejectsInvalidConfiguration(t *testing.T) {
	tests := []conf.LoginRateLimitConfig{
		{Period: "1m", Limit: 0},
		{Period: "not-a-duration", Limit: 10},
		{Period: "0s", Limit: 10},
	}

	for _, config := range tests {
		if _, err := NewLoginLimiter(config); err == nil {
			t.Fatalf("NewLoginLimiter(%+v) unexpectedly succeeded", config)
		}
	}
}

func TestLimiterIgnoresSpoofedForwardHeaderByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewLimiter(time.Minute, 2, limiter.WithTrustForwardHeader(false))
	router := gin.New()
	router.GET("/", handler, func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})

	for attempt, forwardedIP := range []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set("X-Forwarded-For", forwardedIP)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		want := http.StatusNoContent
		if attempt == 2 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt+1, response.Code, want)
		}
	}
}

func TestLimiterUsesForwardHeaderOnlyWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := NewLimiter(time.Minute, 1, limiter.WithTrustForwardHeader(true))
	router := gin.New()
	router.GET("/", handler, func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})

	for attempt, forwardedIP := range []string{"198.51.100.1", "198.51.100.2"} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set("X-Forwarded-For", forwardedIP)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf(
				"attempt %d status = %d, want %d",
				attempt+1,
				response.Code,
				http.StatusNoContent,
			)
		}
	}
}

func TestLoginLimiterRotatingAccountsStillReachesIPLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	loginLimiter, err := NewLoginLimiter(conf.LoginRateLimitConfig{
		Period: "1m",
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("NewLoginLimiter() error = %v", err)
	}

	router := gin.New()
	router.POST("/api/user/login", loginLimiter, func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})

	for attempt := int64(1); attempt <= loginIPLimitMultiplier+1; attempt++ {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/user/login",
			bytes.NewBufferString(
				`{"username":"rotated-user-`+strconv.FormatInt(attempt, 10)+`","password":"wrong"}`,
			),
		)
		request.RemoteAddr = "192.0.2.1:1234"
		request.Header.Set(
			"X-Forwarded-For",
			"198.51.100."+strconv.FormatInt(attempt, 10),
		)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		want := http.StatusNoContent
		if attempt == loginIPLimitMultiplier+1 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, want)
		}
	}
}

func TestLoginLimiterDoesNotCreateAccountKeysAfterIPLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	loginLimiter, accountLimiter, ipLimiter, err := newLoginLimiter(
		conf.LoginRateLimitConfig{
			Period: "1m",
			Limit:  1,
		},
	)
	if err != nil {
		t.Fatalf("newLoginLimiter() error = %v", err)
	}

	router := gin.New()
	router.POST("/api/user/login", loginLimiter, func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})

	const remoteAddr = "192.0.2.1:1234"
	for attempt := int64(1); attempt <= loginIPLimitMultiplier+1; attempt++ {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/user/login",
			bytes.NewBufferString(
				`{"username":"blocked-user-`+strconv.FormatInt(attempt, 10)+`","password":"wrong"}`,
			),
		)
		request.RemoteAddr = remoteAddr
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		want := http.StatusNoContent
		if attempt == loginIPLimitMultiplier+1 {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("attempt %d status = %d, want %d", attempt, response.Code, want)
		}
	}

	blockedRequest := httptest.NewRequest(
		http.MethodPost,
		"/api/user/login",
		bytes.NewBufferString(
			`{"username":"blocked-user-6","password":"wrong"}`,
		),
	)
	blockedRequest.RemoteAddr = remoteAddr
	blockedContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	blockedContext.Request = blockedRequest

	ipKey := ipLimiter.GetIPKey(blockedRequest)
	identity := loginRateLimitIdentity(blockedContext)
	limitContext, err := accountLimiter.Peek(
		context.Background(),
		ipKey+"|"+identity,
	)
	if err != nil {
		t.Fatalf("peek blocked account key: %v", err)
	}
	if limitContext.Remaining != 1 {
		t.Fatalf(
			"blocked account remaining = %d, want 1 (key must not be consumed)",
			limitContext.Remaining,
		)
	}
}

func TestLoginLimiterSameAccountAcrossIPsDoesNotCreateGlobalLockout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	loginLimiter, err := NewLoginLimiter(conf.LoginRateLimitConfig{
		Period: "1m",
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("NewLoginLimiter() error = %v", err)
	}

	router := gin.New()
	router.POST("/api/user/login", loginLimiter, func(ctx *gin.Context) {
		ctx.Status(http.StatusNoContent)
	})

	for attempt, remoteAddr := range []string{"192.0.2.1:1234", "198.51.100.1:5678"} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/user/login",
			bytes.NewBufferString(`{"username":"root","password":"wrong"}`),
		)
		request.RemoteAddr = remoteAddr
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf(
				"attempt %d status = %d, want %d",
				attempt+1,
				response.Code,
				http.StatusNoContent,
			)
		}
	}
}

func TestLoginLimiterPreservesBodyAndSeparatesAccountsBehindOneProxy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	loginLimiter, err := NewLoginLimiter(conf.LoginRateLimitConfig{
		Period: "1m",
		Limit:  1,
	})
	if err != nil {
		t.Fatalf("NewLoginLimiter() error = %v", err)
	}

	router := gin.New()
	router.POST("/api/user/login", loginLimiter, func(ctx *gin.Context) {
		var credentials struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(ctx.Request.Body).Decode(&credentials); err != nil {
			ctx.Status(http.StatusBadRequest)
			return
		}
		if credentials.Password != "wrong" {
			ctx.Status(http.StatusBadRequest)
			return
		}
		ctx.Status(http.StatusNoContent)
	})

	for _, username := range []string{"root", "another-user"} {
		request := httptest.NewRequest(
			http.MethodPost,
			"/api/user/login",
			bytes.NewBufferString(`{"username":"`+username+`","password":"wrong"}`),
		)
		request.RemoteAddr = "192.0.2.1:1234"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("username %q status = %d, want %d", username, response.Code, http.StatusNoContent)
		}
	}
}

func requestLoginLimiterRoute(
	handler http.Handler,
	method string,
	target string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, nil)
	request.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	return response
}
