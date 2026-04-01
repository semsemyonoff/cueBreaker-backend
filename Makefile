# Docker image name and tag
CUEBREAKER_IMAGE ?= semsemyonoff/cuebreaker
CUEBREAKER_TAG ?= latest
# Target platforms for multi-arch build
CUEBREAKER_PLATFORMS ?= linux/amd64,linux/arm64

export CUEBREAKER_IMAGE CUEBREAKER_TAG CUEBREAKER_PLATFORMS

.PHONY: build

# Build multi-arch image and push to registry
build:
	./build.sh
