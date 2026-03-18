FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY src/go.mod src/go.sum ./
RUN go mod download

COPY src/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /immich-exif .

FROM alpine:latest

LABEL org.opencontainers.image.source="https://github.com/Majorfi/immich-exif"

WORKDIR /app

RUN apk add --no-cache perl-image-exiftool

COPY --from=builder /immich-exif .

RUN adduser -D -g '' appuser
USER appuser

ENTRYPOINT ["./immich-exif"]
