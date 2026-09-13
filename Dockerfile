# syntax=docker/dockerfile:1

# Build stage — compiles the vaktd binary. Pinned to the Go version in
# go.mod so the container toolchain matches local dev exactly.
FROM golang:1.27.1-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# web/dist is the frontend's build output (owned by create-frontend-scaffold
# and later frontend tasks). It may not exist yet in a fresh checkout —
# make sure it does, even empty, so the runtime stage's COPY below always
# has something to copy.
RUN mkdir -p web/dist
RUN CGO_ENABLED=0 go build -trimpath -o /out/vaktd ./cmd/vaktd

# Runtime stage — slim image with just the binary, the (possibly empty)
# frontend bundle, and CA certs for outbound HTTPS (webpush).
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=build /out/vaktd /usr/local/bin/vaktd
COPY --from=build /src/web/dist ./web/dist

ENV VAKT_ADDR=:8080
ENV VAKT_WEB_DIR=/app/web/dist

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/vaktd"]
