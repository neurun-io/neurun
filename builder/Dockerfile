ARG GO_VERSION=1.25
ARG NODE_VERSION=24

FROM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/builder \
    ./cmd/builder

FROM node:${NODE_VERSION}-bookworm

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        git \
        openssh-client \
        python-is-python3 \
        python3 \
        python3-dev \
        python3-pip \
    && rm -rf /var/lib/apt/lists/*

# The worker invokes the Go toolchain when it builds Go workflow repositories.
COPY --from=build /usr/local/go /usr/local/go

RUN groupadd --gid 10001 builder \
    && useradd --uid 10001 --gid builder --create-home --shell /usr/sbin/nologin builder \
    && mkdir -p \
        /home/builder/.cache/go-build \
        /home/builder/.cache/npm \
        /home/builder/.cache/pip \
        /home/builder/go \
    && chown -R builder:builder /home/builder

ENV HOME=/home/builder \
    GOPATH=/home/builder/go \
    GOCACHE=/home/builder/.cache/go-build \
    NPM_CONFIG_CACHE=/home/builder/.cache/npm \
    PIP_CACHE_DIR=/home/builder/.cache/pip \
    PATH=/usr/local/go/bin:/home/builder/go/bin:$PATH

COPY --from=build --chown=builder:builder /out/builder /usr/local/bin/builder

USER builder
WORKDIR /home/builder

STOPSIGNAL SIGTERM
ENTRYPOINT ["builder"]
