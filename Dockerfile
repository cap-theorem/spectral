FROM golang:1.27 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
  -trimpath \
  -o /out/spctrld \
  ./cmd/spctrld

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/spctrld /spectrld

ENTRYPOINT ["spectrld"]
