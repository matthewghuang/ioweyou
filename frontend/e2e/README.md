# E2E Tests (Playwright)

End-to-end tests for the I Owe You frontend, covering the core user flows:

- **Group creation and joining** — creating groups, joining via invite link, auth errors
- **Expense management** — adding equal-split and custom-split expenses, empty states, clean submission when `crypto.randomUUID` is unavailable (regression: console errors fail the test)
- **Payment flow** — recording, confirming, and cancelling payments
- **Balance display** — settlement calculations, partial payments, per-expense breakdowns

## Prerequisites

1. **Backend** running on `http://localhost:8080`:
   ```bash
   go run ./cmd/server -db /tmp/test.db -static frontend/dist
   ```
   Or use `docker-compose up` to build and run both.

2. **Frontend** dev server running on `http://localhost:5173`:
   ```bash
   cd frontend && bun install && bun run dev
   ```

3. **Playwright browsers** installed (run once):
   ```bash
   cd frontend && npx playwright install chromium
   ```

   For mobile tests (Mobile Safari / iPhone 14), install WebKit as well:
   ```bash
   cd frontend && npx playwright install webkit
   ```

## Running

Tests run against **Desktop Chrome** and **Mobile Safari (iPhone 14)** by default.

```bash
cd frontend
npx playwright test          # headless, all projects
npx playwright test --headed # with visible browser
npx playwright test --ui     # Playwright UI mode
npx playwright test --debug  # debug mode
```

To run tests for a specific project:
```bash
npx playwright test --project="chromium"
npx playwright test --project="Mobile Safari"
```

## Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `BASE_URL` | `http://localhost:5173` | Frontend URL |
| `BACKEND_URL` | `http://localhost:8080` | Backend API URL |

## Test architecture

Tests use the **backend API directly for test setup** (creating groups, joining members, creating expenses/payments as test data) and then **interact with the UI** to verify the frontend displays the correct state.

Key patterns:
- `apiCreateGroup` / `apiJoinGroup` — set up test state via API calls
- `setGroupAuth` — inject auth tokens into `localStorage` (bypasses the create/join UI flow)
- `goToGroup` — navigate directly to a group detail page
- UI assertions use Playwright's `getByText`, `getByRole`, and `locator` selectors matching the Svelte component markup
