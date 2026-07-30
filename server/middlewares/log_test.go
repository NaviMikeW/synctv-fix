package middlewares

import (
	"net/url"
	"testing"
)

func TestSanitizedQueryRedactsSensitiveValues(t *testing.T) {
	raw := "page=2&TOKEN=first&Api_Key=second&db.password=third&client-secret=fourth" +
		"&X-Emby-Token=fifth&Signature=sixth&AUTHORIZATION=seventh&credential=eighth" +
		"&code=ninth&state=tenth"

	got := sanitizedQuery(raw)
	query, err := url.ParseQuery(got)
	if err != nil {
		t.Fatalf("parse sanitized query: %v", err)
	}

	if got := query.Get("page"); got != "2" {
		t.Fatalf("page = %q, want %q", got, "2")
	}

	for _, key := range []string{
		"TOKEN",
		"Api_Key",
		"db.password",
		"client-secret",
		"X-Emby-Token",
		"Signature",
		"AUTHORIZATION",
		"credential",
		"code",
		"state",
	} {
		if got := query.Get(key); got != redactedQueryValue {
			t.Errorf("%s = %q, want %q", key, got, redactedQueryValue)
		}
	}
}

func TestSanitizedQueryDropsMalformedQuery(t *testing.T) {
	if got := sanitizedQuery("token=secret&bad=%zz"); got != "" {
		t.Fatalf("sanitizedQuery() = %q, want an empty query", got)
	}
}

func TestRequestLogPathUsesRouteTemplate(t *testing.T) {
	got := requestLogPath("/api/room/:roomId/movie/:movieId", "token=secret&quality=1080p")
	want := "/api/room/:roomId/movie/:movieId?quality=1080p&token=REDACTED"
	if got != want {
		t.Fatalf("requestLogPath() = %q, want %q", got, want)
	}
}

func TestRequestLogPathDoesNotExposeUnmatchedPath(t *testing.T) {
	got := requestLogPath("", "password=secret")
	want := "<unmatched>?password=REDACTED"
	if got != want {
		t.Fatalf("requestLogPath() = %q, want %q", got, want)
	}
}
