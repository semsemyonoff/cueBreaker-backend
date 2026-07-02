# --- SPA build ---
FROM node:22-alpine AS frontend-builder

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# --- shntool build (shnsplit) ---
FROM alpine:3.20 AS shntool-builder

RUN apk add --no-cache build-base curl
RUN curl -fsSL http://shnutils.freeshell.org/shntool/dist/src/shntool-3.0.10.tar.gz \
    | tar xz -C /tmp \
 && cd /tmp/shntool-3.0.10 \
 && curl -fsSL -o config.guess 'https://raw.githubusercontent.com/gcc-mirror/gcc/master/config.guess' \
 && curl -fsSL -o config.sub   'https://raw.githubusercontent.com/gcc-mirror/gcc/master/config.sub' \
 && chmod +x config.guess config.sub \
 && ./configure --prefix=/usr CFLAGS="-O2 -std=gnu11" \
 && make -j$(nproc) \
 && make install DESTDIR=/out

# --- Go build (SPA embedded) ---
FROM golang:1.26-alpine AS backend-builder

ARG APP_VERSION=dev

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
COPY --from=frontend-builder /src/frontend/dist/. web/dist/
RUN go build -ldflags "-X main.version=${APP_VERSION}" -o /out/cuebreaker ./cmd/cuebreaker

# --- Runtime ---
FROM alpine:3.20

COPY --from=shntool-builder /out/usr/bin/shntool /usr/bin/shntool
RUN ln -s shntool /usr/bin/shnsplit

RUN apk add --no-cache \
    cuetools \
    flac

COPY --from=backend-builder /out/cuebreaker /usr/local/bin/cuebreaker

# Create the default mount points so the server starts even when run without a
# bind mount; EvalSymlinks(/input) at startup would otherwise fail and exit(1).
RUN mkdir -p /input /output

ENV CUEBREAKER_INPUT_DIR=/input \
    CUEBREAKER_OUTPUT_DIR=/output \
    CUEBREAKER_PORT=5000

EXPOSE 5000

CMD ["cuebreaker"]
