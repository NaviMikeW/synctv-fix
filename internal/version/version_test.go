package version_test

import (
	"errors"
	"testing"

	"github.com/synctv-org/synctv/internal/version"
)

func TestSelfUpdateDisabled(t *testing.T) {
	v, err := version.NewVersionInfo()
	if err != nil {
		t.Fatal(err)
	}

	if err := v.SelfUpdate(t.Context()); !errors.Is(err, version.ErrSelfUpdateDisabled) {
		t.Fatalf("Info.SelfUpdate() error = %v, want ErrSelfUpdateDisabled", err)
	}

	if err := version.SelfUpdate(t.Context(), "https://example.invalid/synctv"); !errors.Is(
		err,
		version.ErrSelfUpdateDisabled,
	) {
		t.Fatalf("SelfUpdate() error = %v, want ErrSelfUpdateDisabled", err)
	}
}

func TestCheckLatest(t *testing.T) {
	v, err := version.NewVersionInfo()
	if err != nil {
		t.Fatal(err)
	}

	s, err := v.CheckLatest(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	t.Log(s)
}

func TestLatestBinaryURL(t *testing.T) {
	v, err := version.NewVersionInfo()
	if err != nil {
		t.Fatal(err)
	}

	s, err := v.LatestBinaryURL(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	t.Log(s)
}
