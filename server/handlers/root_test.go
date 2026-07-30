package handlers

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/synctv-org/synctv/internal/conf"
)

func TestManagedPasswordRequestRequiresHTTPS(t *testing.T) {
	previousConfig := conf.Conf
	conf.Conf = conf.DefaultConfig()
	t.Cleanup(func() {
		conf.Conf = previousConfig
	})

	request := httptest.NewRequest("POST", "http://synctv/api/admin/user/managed-password", nil)
	if managedPasswordRequestIsSecure(request) {
		t.Fatal("plain HTTP request unexpectedly accepted")
	}

	request.Header.Set("X-Forwarded-Proto", "https")
	if managedPasswordRequestIsSecure(request) {
		t.Fatal("untrusted forwarded protocol unexpectedly accepted")
	}

	conf.Conf.Security.GuardianTrustForwardedProto = true
	if !managedPasswordRequestIsSecure(request) {
		t.Fatal("trusted HTTPS forwarded protocol was rejected")
	}

	request.Header.Set("X-Forwarded-Proto", "http")
	request.TLS = &tls.ConnectionState{}
	if !managedPasswordRequestIsSecure(request) {
		t.Fatal("direct TLS request was rejected")
	}
}
