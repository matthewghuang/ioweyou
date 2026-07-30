# I Owe You

Group expense tracking with offline-first CRDT sync. Installable PWA.

## Quick Start

### Prerequisites
- Go 1.25+
- [Bun](https://bun.sh) (for frontend)
- The backend runs standalone (SQLite, no external services)

### Run the backend
```bash
go run ./cmd/server -db data.db -static frontend/dist
```

### Run the frontend (dev mode)
```bash
cd frontend
bun install
bun run dev
```

The frontend proxies `/api/*` to `localhost:8080`.

### Run with Docker
```bash
docker compose up --build
```

### Run E2E tests
```bash
cd frontend
npx playwright install chromium webkit
# start backend first, then:
npx playwright test
```

## Architecture

### CRDT Data Model
All data is stored as an append-only log of CRDT operations in a `crdt_operations` table:
- **LWW (Last-Writer-Wins)** — scalar fields like description, amount, status
- **RGA (Replicated Growable Array)** — ordered lists like expense splits

Operations are identified by `(doc_id, author_id, wall_time, logical)` — the server uses `INSERT OR IGNORE` for idempotent deduplication.

### Sync Protocol
- **Push** — `POST /api/sync/push` — client sends operations; server deduplicates and broadcasts via WebSocket
- **Pull** — `POST /api/sync/pull` — client sends per-author cursors; server returns newer operations
- **WebSocket** — `/api/ws` — real-time fan-out of operations to group subscribers

### Offline-First
The PWA client maintains a local IndexedDB copy of operations. When offline:
1. Writes are stored locally as CRDT operations
2. UI updates optimistically with a "Pending" badge
3. On reconnect, pushes queued ops and pulls new ones
4. CRDT merge semantics resolve conflicts automatically

### Frontend
- Svelte 5 with `$state`/`$derived`/`$effect` reactive primitives
- Mobile-first single-column layout with 44px touch targets
- Gestures: swipe-back, pull-to-refresh, swipe-to-reveal actions
- Lazy-loaded GroupDetail component (code-split at route level)

## Project Structure

```
├── cmd/server/main.go      # Go entry point
├── internal/
│   ├── api/                # HTTP handlers (groups, expenses, payments, balances)
│   ├── auth/               # Token-based group authentication
│   ├── crdt/               # CRDT engine (LWW, RGA, HLC, merge, snapshot)
│   ├── store/              # SQLite persistence layer
│   └── sync/               # WebSocket + push/pull sync protocol
├── frontend/
│   ├── src/                # Svelte 5 app
│   │   ├── lib/            # Shared modules (CRDT, sync, gestures, stores, components)
│   │   └── pages/          # Page components (Landing, Join, Groups, GroupDetail)
│   ├── e2e/                # Playwright E2E tests
│   └── public/             # Static assets (app icons)
├── docs/
│   ├── api.md              # Full API documentation
│   └── mobile-plan.md      # Mobile version development plan
├── Dockerfile              # Multi-stage build (Go + Bun + Alpine runtime)
├── docker-compose.yml      # Single-service deployment
└── .github/workflows/      # CI pipeline (build, test, Docker)
```

## Project Status

All six phases of the mobile version are complete:

| Phase | Status |
|-------|--------|
| PWA Infrastructure & Mobile Layout | :white_check_mark: |
| Navigation & Gestures | :white_check_mark: |
| CRDT Offline Sync | :white_check_mark: |
| Touch & Interaction Polish | :white_check_mark: |
| Native Feel (icons, haptics) | :white_check_mark: |
| Testing & Performance | :white_check_mark: |

See [docs/mobile-plan.md](docs/mobile-plan.md) for details.

## API Documentation

Full API reference at [docs/api.md](docs/api.md).

## License

MIT
