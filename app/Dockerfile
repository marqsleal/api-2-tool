# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS base
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

FROM base AS test
RUN go test ./...

FROM base AS build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api

FROM alpine:3.20 AS run
RUN apk add --no-cache ca-certificates \
    && addgroup -S app \
    && adduser -S app -G app

WORKDIR /app

COPY --from=build /out/api /app/api-server
COPY api /app/api

RUN mkdir -p /app/data \
    && chown -R app:app /app

USER app

EXPOSE 8080

ENTRYPOINT ["/app/api-server"]
