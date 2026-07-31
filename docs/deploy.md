# Deployment Guide

## Overview

I Owe You is a single-binary Go application with an embedded SQLite database. It serves both the REST API and the compiled frontend static files. Deployment is straightforward — build the Docker image, mount a volume for SQLite, and run.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [fly.io CLI](https://fly.io/docs/hands-on/install-flyctl/) (for Fly deployment)
- A [fly.io](https://fly.io) account (free tier works)

## Local Deployment (Docker)

```bash
# Build and run with docker-compose
docker compose up --build

# The app will be at http://localhost:8080
```

The `docker-compose.yml` mounts a persistent volume for SQLite and passes the correct `-db` and `-static` flags.

## Fly.io Deployment

### Step 1: Launch

```bash
# From the project root
fly launch --generate-name --no-deploy
```

This creates a `fly.toml` and prepares the app. The `--no-deploy` flag prevents an immediate (likely failing) deployment.

### Step 2: Create a volume for SQLite

```bash
fly volumes create data --region iad --size 1
```

This creates a 1 GB persistent volume named `data` in the `iad` region. The volume is mounted at `/data` as configured in `fly.toml`.

### Step 3: Configure the server command

Update the Dockerfile's default arguments or pass them at deploy time. The server needs:

- `-db /data/data.db` — SQLite database path on the persistent volume
- `-static /frontend/dist` — frontend static files (built into the image)

The `fly.toml` already configures the volume mount. Add or update the `cmd` setting in your `fly.toml`:

```toml
[processes]
  app = "ioweyou"

[[services]]
  internal_port = 8080
  protocol = "tcp"
  # ...
```

Then deploy:

```bash
fly deploy
```

### Step 4: Verify

```bash
fly open
```

The app should load. Create a group to verify the database is working.

## Configuration

The server is configured entirely through CLI flags:

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:8080` | HTTP listen address |
| `-db` | `data.db` | SQLite database path |
| `-static` | `frontend/dist` | Frontend static files directory |

On Fly.io, the Dockerfile's `EXPOSE 8080` and `fly.toml`'s `internal_port` should match the port the server listens on. The default `-addr :8080` works out of the box with `fly launch` defaults.

To customize, modify the `cmd` in `fly.toml`:

```toml
[processes]
  app = "/ioweyou -addr :8080 -db /data/data.db -static /frontend/dist"
```

## Production Considerations

### Backup

The SQLite database lives on the Fly volume. To back it up:

```bash
fly ssh console -C "cp /data/data.db /data/backup-$(date +%Y%m%d).db"
```

Or set up automated backups via Fly's volume snapshot feature.

### Scaling

The app is stateless (SQLite is the only state, on the volume). You can scale to multiple machines, but SQLite doesn't support concurrent writes from multiple instances. For most group expense tracking use cases, a single machine is sufficient.

To scale vertically (more RAM/CPU):

```bash
fly machine update <machine-id> --memory 512
```

### Monitoring

```bash
fly logs
fly status
```

### Updating

```bash
git pull
fly deploy
```

The database is on the persistent volume and survives deployments.

## Architecture Notes

- The Docker image uses a multi-stage build: Go binary in stage 1, Bun frontend build in stage 2, combined in a minimal Alpine runtime stage.
- The frontend is compiled to static files and served by the Go server. There's no separate frontend server in production.
- SQLite WAL mode handles concurrent reads efficiently. Writes are serialized.
- The CRDT operation log is append-only, so the database grows monotonically. Consider periodic compaction or setting up WAL checkpointing for long-running deployments.
