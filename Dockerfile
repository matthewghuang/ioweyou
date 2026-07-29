# syntax=docker/dockerfile:1
# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /src

# Cache module downloads
COPY go.mod go.sum ./
RUN go mod download

# Build the binary
COPY . .
RUN CGO_ENABLED=0 go build -o /ioweyou ./cmd/server

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

COPY --from=builder /ioweyou /ioweyou

# Use non-root user
USER appuser

EXPOSE 8080

ENTRYPOINT ["/ioweyou"]
