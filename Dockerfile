# syntax=docker/dockerfile:1

# 1. Build the web UI, which the Go binary embeds.
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN mkdir -p /src/internal/ui/dist && npm run build

# 2. Build the server, with the UI inside it.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /src/internal/ui/dist ./internal/ui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/antennarr ./cmd/antennarr

# 3. A small image to run it.
FROM alpine:3
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/antennarr /usr/local/bin/antennarr
EXPOSE 5004
ENTRYPOINT ["/usr/local/bin/antennarr"]
