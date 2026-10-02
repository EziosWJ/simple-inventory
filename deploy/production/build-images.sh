#!/usr/bin/env bash
set -euo pipefail
release=${1:?Usage: build-images.sh RELEASE [IMAGE_PREFIX]}
image_prefix=${2:-simple-inventory}
[[ $release =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
[[ $image_prefix =~ ^[a-zA-Z0-9][a-zA-Z0-9/_.:-]*$ ]] || { echo 'Invalid image prefix' >&2; exit 1; }
project_root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$project_root"
docker build -f server/Dockerfile --target api -t "$image_prefix-api:$release" .
docker build -f server/Dockerfile --target migrate -t "$image_prefix-migrate:$release" .
docker image inspect "$image_prefix-api:$release" "$image_prefix-migrate:$release" --format '{{.RepoTags}} {{.Id}}'
