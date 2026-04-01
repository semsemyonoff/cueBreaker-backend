#!/usr/bin/env bash
set -euo pipefail

IMAGE="${CUEBREAKER_IMAGE:-semsemyonoff/beetdeck}"
TAG="${CUEBREAKER_TAG:-latest}"
PLATFORMS="${CUEBREAKER_PLATFORMS:-linux/amd64,linux/arm64}"

BUILDER="cueBreaker-multiarch"
if ! docker buildx inspect "$BUILDER" &>/dev/null; then
    docker buildx create --name "$BUILDER" --use
else
    docker buildx use "$BUILDER"
fi

docker buildx build \
    --platform "$PLATFORMS" \
    --tag "${IMAGE}:${TAG}" \
    --push .
