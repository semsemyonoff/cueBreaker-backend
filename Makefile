# Docker image name and tag
CUEBREAKER_IMAGE ?= semsemyonoff/cuebreaker
CUEBREAKER_TAG ?= latest
# Target platforms for multi-arch build
CUEBREAKER_PLATFORMS ?= linux/amd64,linux/arm64

export CUEBREAKER_IMAGE CUEBREAKER_TAG CUEBREAKER_PLATFORMS

APP_VERSION ?= dev

.PHONY: frontend-build build dev test lint docker-build

# Build the SPA and copy it into backend/web/dist (embedded by the Go build)
frontend-build:
	npm --prefix frontend ci
	npm --prefix frontend run build
	rm -rf backend/web/dist
	mkdir -p backend/web/dist
	cp -r frontend/dist/. backend/web/dist/
	touch backend/web/dist/.gitkeep

# Build the single cuebreaker binary with the SPA embedded
build: frontend-build
	cd backend && go build -ldflags "-X main.version=$(APP_VERSION)" -o cuebreaker ./cmd/cuebreaker

# Run the Vite dev server (proxies /api to a locally running backend) and the Go backend together
dev:
	npm --prefix frontend run dev & \
	cd backend && go run ./cmd/cuebreaker; \
	wait

test:
	cd backend && go test ./...
	npm --prefix frontend run test

lint:
	cd backend && go vet ./...
	npm --prefix frontend run lint

# Build multi-arch image and push to registry
docker-build:
	./build.sh
