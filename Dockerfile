FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY src/go.mod src/go.sum ./
RUN go mod download

COPY src/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /immich-exif .

FROM alpine:latest

LABEL org.opencontainers.image.source="https://github.com/Majorfi/immich-exif"

WORKDIR /app

# The exiftool CLI lives in its own Alpine package; perl-image-exiftool is
# only the Perl library and does not provide /usr/bin/exiftool.
RUN apk add --no-cache exiftool

COPY --from=builder /immich-exif .

# The tool creates its scratch dir in the working directory, so give appuser
# a writable one instead of the root-owned /app.
RUN adduser -D -g '' appuser && mkdir /data && chown appuser /data
USER appuser
WORKDIR /data

ENTRYPOINT ["/app/immich-exif"]
