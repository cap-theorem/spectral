FROM --platform=$BUILDPLATFORM golang:1.27 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  go mod download

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
  -trimpath \
  -ldflags "-s -w -X github.com/cap-theorem/spectral/internal/version.Version=${VERSION}" \
  -o /out/collector \
  ./cmd/collector

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/collector /collector

EXPOSE 8100
ENTRYPOINT ["/collector"]
