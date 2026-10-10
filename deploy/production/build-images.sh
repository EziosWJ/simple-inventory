#!/usr/bin/env bash
set -euo pipefail
release=${1:?Usage: build-images.sh RELEASE [IMAGE_PREFIX]}
image_prefix=${2:-simple-inventory}
[[ $release =~ ^[a-zA-Z0-9][a-zA-Z0-9_.-]*$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
[[ $image_prefix =~ ^[a-zA-Z0-9][a-zA-Z0-9/_.:-]*$ ]] || { echo 'Invalid image prefix' >&2; exit 1; }
project_root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$project_root"
image="$image_prefix:$release"
alpine_repository=${ALPINE_REPOSITORY:-https://dl-cdn.alpinelinux.org/alpine}
go_proxy=${GOPROXY:-https://proxy.golang.org,direct}
docker build -f server/Dockerfile --build-arg "ALPINE_REPOSITORY=$alpine_repository" --build-arg "GOPROXY=$go_proxy" --target release -t "$image" .
# Keep the earlier service tags as aliases while every service uses this one image ID.
docker tag "$image" "$image_prefix-api:$release"
docker tag "$image" "$image_prefix-migrate:$release"
docker tag "$image" "$image_prefix-backup:$release"
docker image inspect "$image" "$image_prefix-api:$release" "$image_prefix-migrate:$release" "$image_prefix-backup:$release" --format '{{.RepoTags}} {{.Id}}'
