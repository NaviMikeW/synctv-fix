#!/bin/bash
set -euo pipefail

cat >&2 <<'EOF'
The upstream binary installer is disabled in synctv-fix.
It would replace this custom build with synctv-org/synctv.

Use the maintained GHCR image with Docker Compose instead:
  docker compose pull
  docker compose up -d --force-recreate
EOF

exit 1
