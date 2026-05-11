#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$script_dir/.." && pwd)

dockerfile="$repo_root/Dockerfile"
image_name="${GOOMBA_CI_IMAGE:-goomba-ci-local:latest}"
project_dir="$repo_root/_testproject/cgo"

if [[ ! -f "$dockerfile" ]]; then
  echo "Dockerfile not found at $dockerfile" >&2
  exit 1
fi

if [[ ! -d "$project_dir" ]]; then
  echo "Test project not found at $project_dir" >&2
  exit 1
fi

echo ">> Building CI image: $image_name"
if docker buildx version >/dev/null 2>&1; then
  docker buildx build --load -t "$image_name" -f "$dockerfile" "$repo_root"
else
  echo "docker buildx not available; falling back to docker build" >&2
  docker build -t "$image_name" -f "$dockerfile" "$repo_root"
fi

echo ">> Running goomba build in _testproject/cgo"
docker run --rm \
  -e GOOMBA_SOURCE_DIR=/work/_testproject/cgo \
  -e GOOMBA_PLATFORMS=linux \
  -e GOOMBA_ARCH=x64 \
  -e GOOMBA_CGO_ENABLED=true \
  -v "$repo_root":/work \
  "$image_name"
