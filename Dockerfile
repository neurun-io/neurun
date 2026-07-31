# syntax=docker/dockerfile:1.7

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG VERSION=0.1.0
ARG COMMIT=unknown
ARG BUILT_AT=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w \
      -X github.com/neurun-io/neurun/internal/buildinfo.Version=${VERSION} \
      -X github.com/neurun-io/neurun/internal/buildinfo.Commit=${COMMIT} \
      -X github.com/neurun-io/neurun/internal/buildinfo.BuiltAt=${BUILT_AT}" \
    -o /out/neurun ./cmd/neurun

FROM python:3.13-slim-bookworm
RUN apt-get update \
    && apt-get install -y --no-install-recommends build-essential ca-certificates \
    && rm -rf /var/lib/apt/lists/*
RUN groupadd --gid 10001 neurun \
    && useradd --uid 10001 --gid neurun --create-home --shell /usr/sbin/nologin neurun \
    && mkdir -p /var/lib/neurun \
    && chown -R neurun:neurun /var/lib/neurun
COPY --from=build --chown=neurun:neurun /out/neurun /usr/local/bin/neurun
ENV NEURUN_DATA_DIRECTORY=/var/lib/neurun \
    NEURUN_PYTHON_EXECUTABLE=python3
EXPOSE 1267
USER neurun
ENTRYPOINT ["/usr/local/bin/neurun"]
