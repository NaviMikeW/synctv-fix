package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/synctv-org/synctv/internal/version"
)

func TestSelfUpdateIsDisabledWithDockerInstructions(t *testing.T) {
	err := SelfUpdate(nil, nil)
	if !errors.Is(err, version.ErrSelfUpdateDisabled) {
		t.Fatalf("SelfUpdate() error = %v, want ErrSelfUpdateDisabled", err)
	}

	for _, want := range []string{
		"docker compose pull",
		"docker compose up -d --force-recreate",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("SelfUpdate() error does not contain %q", want)
		}
	}
}
