# syntax=docker/dockerfile:1

FROM node:24-alpine AS frontend

WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.5-alpine AS build

WORKDIR /src
ENV GOMAXPROCS=2 GOFLAGS=-p=1

ARG ALPINE_REPOSITORY=https://dl-cdn.alpinelinux.org/alpine
RUN sed -i "s#^https://dl-cdn.alpinelinux.org/alpine#${ALPINE_REPOSITORY}#" /etc/apk/repositories \
    && apk add --no-cache gcc musl-dev
ARG GOPROXY=https://proxy.golang.org,direct

COPY server/go.mod server/go.sum ./
RUN GOPROXY="$GOPROXY" go mod download

COPY server/ .
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/backup ./cmd/backup

FROM build AS api-build

COPY --from=frontend /web/dist ./internal/webui/dist
RUN CGO_ENABLED=1 go build -tags=embedweb -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM alpine:3.22 AS runtime

RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --chown=app:app server/configs ./configs
# Named-volume mount point: must exist and be writable by the app user,
# otherwise file uploads fail with permission denied after the first boot.
RUN mkdir -p /var/lib/simple-inventory/uploads && chown -R app:app /var/lib/simple-inventory
USER app

FROM runtime AS release

COPY --from=api-build --chown=app:app /out/api ./api
COPY --from=build /out/backup ./backup
COPY --from=build /out/migrate ./migrate
COPY --chown=app:app server/migrations ./migrations
EXPOSE 8080
ENTRYPOINT ["/app/api"]
