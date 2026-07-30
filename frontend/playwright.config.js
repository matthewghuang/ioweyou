// @ts-check
import { defineConfig, devices } from '@playwright/test';

const BACKEND_URL = process.env.BACKEND_URL || 'http://localhost:8080';
const BASE_URL = process.env.BASE_URL || 'http://localhost:5173';

export default defineConfig({
  testDir: './e2e/tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1, // sequential — each test shares the same backend
  reporter: process.env.CI ? 'github' : 'list',

  use: {
    baseURL: BASE_URL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    // Expose backend URL so helpers can reach it
    extraHTTPHeaders: {
      'x-test-backend': BACKEND_URL,
    },
  },

  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        // Dark theme matches the app's default
        colorScheme: 'dark',
      },
    },
    {
      name: 'Mobile Safari',
      use: {
        ...devices['iPhone 14'],
        colorScheme: 'dark',
      },
    },
  ],

  webServer: process.env.CI
    ? []
    : [
        // Expect the developer to run the backend and frontend themselves
        // when running locally (`npm run dev` in one terminal, `go run .` in another).
        // CI pipelines should start both before `npx playwright test`.
      ],
});
