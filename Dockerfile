# syntax=docker/dockerfile:1.7

FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG VERSION=0.1.0-dev
ARG COMMIT=unknown
ARG BUILT_AT=unknown
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w \
      -X github.com/dagflows/neurun-io/internal/buildinfo.Version=${VERSION} \
      -X github.com/dagflows/neurun-io/internal/buildinfo.Commit=${COMMIT} \
      -X github.com/dagflows/neurun-io/internal/buildinfo.BuiltAt=${BUILT_AT}" \
    -o /out/neurun ./cmd/neurun

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/neurun /usr/local/bin/neurun
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/neurun"]

