# I Owe You — Frontend

A single-column PWA for group expense tracking, built with Svelte 5.

Features:
- Create and join expense groups with password protection
- Add expenses with equal or custom splits
- Record and confirm payments between members
- Real-time balance breakdown
- Offline support via CRDT-based sync (operations queue locally, replay when online)
- Installable PWA with service worker and Workbox caching
- Mobile-first gestures: swipe-back, pull-to-refresh, swipe-to-reveal actions
- Touch-optimized with 44px minimum tap targets
- Dark theme with safe-area-inset support for notched phones

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
├── index.html              # Entry point with PWA meta tags
├── vite.config.js          # Vite config with PWA plugin
├── svelte.config.js        # Svelte config
├── package.json
├── README.md
├── playwright.config.js    # E2E test config (chromium + Mobile Safari)
├── public/
│   ├── icon-192.png        # PWA app icon
│   └── icon-512.png        # PWA app icon
├── e2e/
│   ├── helpers.js          # Test helpers (API + UI interactions)
│   ├── README.md           # Testing instructions
│   └── tests/
│       ├── group.test.js
│       ├── expense.test.js
│       ├── payment.test.js
│       ├── balance.test.js
│       └── offline-sync.test.js  # Offline → online CRDT sync test
└── src/
    ├── main.js             # App bootstrap
    ├── app.css             # Global dark theme + mobile-first styles
    ├── App.svelte          # Root component with routing (lazy-loads GroupDetail)
    ├── lib/
    │   ├── api.js          # REST API client + auth helpers
    │   ├── stores.js       # Svelte stores (currentPage, currentGroupSlug)
    │   ├── websocket.js    # WebSocket connection for live updates
    │   ├── gestures.js     # Svelte actions: swipeBack, pullToRefresh, swipeReveal
    │   ├── crdt.js         # CRDT engine: mergeState, snapshot, HLC
    │   ├── db.js           # IndexedDB persistence for offline ops
    │   ├── sync.js         # Sync service: push/pull orchestration
    │   ├── networkStore.js # Online/offline detection store
    │   ├── toastStore.js   # Toast notification store
    │   ├── Toast.svelte    # Toast notification component
    │   ├── ToastContainer.svelte
    │   ├── BottomSheet.svelte   # Confirmation bottom sheet
    │   ├── OfflineBanner.svelte # Offline status banner
    │   ├── haptic.js       # Haptic feedback Svelte action
    │   └── forms.js        # Form utilities (scrollIntoViewOnFocus)
    └── pages/
        ├── Landing.svelte      # Create a new group
        ├── Join.svelte         # Join an existing group
        ├── Groups.svelte       # List of joined groups
        └── GroupDetail.svelte  # Group detail with tabs (lazy-loaded)
```

## Pages

| Page | Route (state) | Description |
|------|--------------|-------------|
| Landing | `currentPage = 'landing'` | Create a new group with name, member name, and password |
| Join | `currentPage = 'join'` | Join an existing group via invite slug/code |
| Groups | `currentPage = 'groups'` | List all joined groups, share invites |
| Group Detail | `currentPage = 'group'` | View expenses, payments, and balance (lazy-loaded) |

## API

The app communicates with the backend via REST API. See `docs/api.md` for full documentation.
