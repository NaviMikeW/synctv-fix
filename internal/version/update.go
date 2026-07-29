package version

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/cavaliergopher/grab/v3"
	log "github.com/sirupsen/logrus"
)

const SelfUpdateDisabledMessage = `self-update is disabled in synctv-fix to avoid replacing this custom build with upstream SyncTV.
Docker Compose users should update the image:
  docker compose pull
  docker compose up -d --force-recreate
Native binary auto-update is not available for synctv-fix; use a verified build from:
  https://github.com/NaviMikeW/synctv-fix`

var ErrSelfUpdateDisabled = errors.New(SelfUpdateDisabledMessage)

func SelfUpdate(_ context.Context, _ string) error {
	return ErrSelfUpdateDisabled
}

func DownloadWithProgress(ctx context.Context, url, path string) (string, error) {
	req, err := grab.NewRequest(path, url)
	if err != nil {
		return "", err
	}

	req = req.WithContext(ctx)
	resp := grab.NewClient().Do(req)

	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()

	for {
		select {
		case <-t.C:
			log.Infof("self update: transferred %d / %d bytes (%.2f%%)",
				resp.BytesComplete(),
				resp.Size(),
				100*resp.Progress())

		case <-resp.Done:
			return resp.Filename, resp.Err()
		}
	}
}

// get current executable file
func ExecutableFile() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}

	return filepath.EvalSymlinks(p)
}
