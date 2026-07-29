# I Owe You — Frontend

A Svelte-based single-page application for the I Owe You group-expense tracking app.

## Prerequisites

- **Bun** (package manager and runtime) — install from https://bun.sh
- The [backend server](https://github.com/matthewghuang/ioweyou) running on `localhost:8080`

## Getting Started

```bash
# Install dependencies
bun install

# Start the dev server (with API proxy to localhost:8080)
bun run dev
```

The app runs at `http://localhost:5173`. API calls to `/api/*` are proxied to the backend at `http://localhost:8080`.

## Building for Production

```bash
bun run build
```

Output goes to `dist/`. Serve it with any static file server.

## Project Structure

```
frontend/
├── index.html              # Entry point
├── vite.config.js          # Vite config with proxy
├── svelte.config.js        # Svelte config
├── package.json
├── README.md
└── src/
    ├── main.js             # App bootstrap
    ├── app.css             # Global dark theme styles
    ├── App.svelte          # Root component with routing
    ├── lib/
    │   ├── api.js          # API client (auth header, fetch wrapper)
    │   └── stores.js       # Svelte stores for page/group/user state
    └── pages/
        ├── Register.svelte   # New user registration
        ├── Login.svelte      # API key login
        ├── Groups.svelte     # Group list + create
        └── GroupDetail.svelte # Group detail with tabs
```

## Pages

| Page | Route (state) | Description |
|------|--------------|-------------|
| Register | `currentPage = 'register'` | Create a new user account, get an API key |
| Login | `currentPage = 'login'` | Enter an existing API key |
| Groups | `currentPage = 'groups'` | List all groups, create new ones |
| Group Detail | `currentPage = 'group'` | View expenses, payments, and settlement recommendations |

## API

The app communicates with the backend via REST API:

- `POST /api/auth/register` — Register a new user
- `GET /api/groups` — List user's groups
- `POST /api/groups` — Create a group
- `GET /api/groups/{id}` — Get group details
- `GET /api/groups/{id}/expenses` — List expenses
- `POST /api/groups/{id}/expenses` — Add an expense
- `GET /api/groups/{id}/payments` — List payments
- `POST /api/groups/{id}/payments` — Record a payment
- `POST /api/payments/{id}/confirm` — Confirm a payment
- `DELETE /api/payments/{id}` — Cancel a payment
- `GET /api/groups/{id}/balances` — Get settlement recommendations
