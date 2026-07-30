.PHONY: build run dev test clean

# Build the Go binary
build:
	go build -o ioweyou ./cmd/server
	cd frontend && bun run build

# Run the server locally (requires built frontend)
run:
	./ioweyou -db data.db -static frontend/dist

# Run the frontend dev server (with API proxy)
dev:
	cd frontend && bun run dev

# Run tests
test:
	go test ./...
	cd frontend && bun run build

# Clean build artifacts
clean:
	rm -f ioweyou server
	rm -rf frontend/dist
