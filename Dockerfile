# syntax=docker/dockerfile:1.7

# AxonHub is resolved as a Go module from go.mod.

ARG NODE_VERSION=22
ARG GO_VERSION=1.26
ARG ALPINE_VERSION=3.21

FROM node:${NODE_VERSION}-bookworm-slim AS frontend
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@latest --activate
COPY src/web/package.json src/web/pnpm-lock.yaml ./web/
WORKDIR /src/web
RUN pnpm install --frozen-lockfile
COPY src/web/ ./
ARG APP_VERSION=dev
ENV VITE_APP_VERSION=${APP_VERSION}
RUN pnpm run build

FROM golang:${GO_VERSION}-bookworm AS backend
WORKDIR /workspace
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOFLAGS=-mod=mod
COPY src/go.mod src/go.sum ./octopus/
COPY src/third_party ./octopus/third_party
WORKDIR /workspace/octopus
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY src/ ./
COPY --from=frontend /src/static/out ./static/out
ARG APP_VERSION=dev
ARG BUILD_TIME=unknown
ARG GIT_AUTHOR=MUKAPP
ARG COMMIT_ID=unknown
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /out/octopus \
      -ldflags="-X 'github.com/bestruirui/octopus/internal/conf.Version=${APP_VERSION}' \
                -X 'github.com/bestruirui/octopus/internal/conf.BuildTime=${BUILD_TIME}' \
                -X 'github.com/bestruirui/octopus/internal/conf.Author=${GIT_AUTHOR}' \
                -X 'github.com/bestruirui/octopus/internal/conf.Commit=${COMMIT_ID}' \
                -s -w" \
      -tags=jsoniter \
      .

FROM alpine:${ALPINE_VERSION}
ENV TZ=Asia/Shanghai
RUN apk add --no-cache alpine-conf ca-certificates su-exec tzdata && \
    /usr/sbin/setup-timezone -z Asia/Shanghai && \
    apk del alpine-conf && \
    rm -rf /var/cache/apk/* && \
    mkdir -p /app
COPY --from=backend /out/octopus /app/octopus
COPY src/scripts/dockerfiles/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh /app/octopus
WORKDIR /app
EXPOSE 8080
CMD ["/entrypoint.sh"]
