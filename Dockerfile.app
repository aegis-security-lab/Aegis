# syntax=docker/dockerfile:1.7

ARG NODE_VERSION=22.19.0
ARG GO_VERSION=1.25.0

FROM node:${NODE_VERSION}-bookworm-slim AS node-base

FROM node-base AS frontend-build
WORKDIR /build

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY index.html components.json tsconfig.json tsconfig.app.json tsconfig.node.json vite.config.ts ./
COPY public ./public
COPY src ./src
RUN npm run build

FROM golang:${GO_VERSION}-bookworm AS go-base

ENV CGO_ENABLED=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential libsqlite3-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

FROM go-base AS backend-build

ARG VERSION=dev

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
COPY --from=frontend-build /build/dist ./internal/webui/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/aegis ./cmd/server

FROM go-base AS development

COPY --from=node-base /usr/local/ /usr/local/
COPY --from=docker:cli /usr/local/bin/docker /usr/local/bin/docker

RUN --mount=type=cache,target=/go/pkg/mod \
    go install github.com/air-verse/air@v1.66.0

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci

COPY . .

EXPOSE 8080 5173

CMD ["make", "dev-local"]

FROM debian:bookworm-slim AS production

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=docker:cli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=backend-build /out/aegis /usr/local/bin/aegis
# Aegis builds the separate Worker sandbox image from this Dockerfile at runtime.
COPY Dockerfile /app/Dockerfile

ENV AEGIS_HOST=0.0.0.0 \
    AEGIS_DATA_DIR=/var/lib/aegis \
    AEGIS_DIST=/app/dist \
    PORT=8080

VOLUME ["/var/lib/aegis"]
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/aegis"]
