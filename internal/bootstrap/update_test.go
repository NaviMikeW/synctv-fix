package bootstrap

import (
	"bytes"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestInitCheckUpdateIsForkSafe(t *testing.T) {
	var output bytes.Buffer
	logger := log.StandardLogger()
	previousOutput := logger.Out
	logger.SetOutput(&output)
	t.Cleanup(func() {
		logger.SetOutput(previousOutput)
	})

	if err := InitCheckUpdate(t.Context()); err != nil {
		t.Fatalf("InitCheckUpdate() error = %v", err)
	}

	message := output.String()
	if !strings.Contains(message, "automatic update checks are disabled in synctv-fix") {
		t.Fatalf("disabled update-check message missing: %s", message)
	}
	if strings.Contains(message, "synctv-org/synctv/releases") {
		t.Fatalf("upstream release URL was advertised: %s", message)
	}
}
