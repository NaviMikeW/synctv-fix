package bootstrap

import (
	"context"

	log "github.com/sirupsen/logrus"
	"github.com/synctv-org/synctv/internal/version"
)

// InitCheckUpdate deliberately avoids contacting or advertising the upstream
// SyncTV release feed. synctv-fix can only be updated through a verified fork
// image or native build.
func InitCheckUpdate(_ context.Context) error {
	log.Info("automatic update checks are disabled in synctv-fix")
	log.Info(version.SelfUpdateDisabledMessage)

	return nil
}
