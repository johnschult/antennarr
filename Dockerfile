# syntax=docker/dockerfile:1

# 1. Build the web UI, which the Go binary embeds.
FROM --platform=$BUILDPLATFORM node:26-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
# The release notes the page shows after an update are bundled from CHANGELOG.md (web/src/hooks/use-whats-new.ts).
COPY CHANGELOG.md /src/CHANGELOG.md
RUN mkdir -p /src/internal/ui/dist && npm run build

# 2. Build the server, with the UI inside it. It is cross-compiled on the build machine's
#    own architecture, so building an arm64 image doesn't mean emulating a compiler.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/internal/ui/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags="-s -w -X github.com/johnschult/antennarr/internal/version.Version=${VERSION}" -o /out/antennarr ./cmd/antennarr

# 3. A small image to run it.
FROM alpine:3
LABEL org.opencontainers.image.source="https://github.com/johnschult/antennarr" \
      org.opencontainers.image.description="Share an HDHomeRun tuner with Plex, Jellyfin or Emby servers on other networks" \
      org.opencontainers.image.licenses="MIT"
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/antennarr /usr/local/bin/antennarr
# Run from /config, so the default settings file (antennarr.json) lands in the volume
# you mount there.
WORKDIR /config
EXPOSE 5004
ENTRYPOINT ["/usr/local/bin/antennarr"]
