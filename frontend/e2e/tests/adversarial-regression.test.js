// @ts-check
import { test, expect } from '@playwright/test';
import {
  createGroup,
  openGroup,
} from '../helpers.js';

test.describe('Adversarial regression tests', () => {
  test('undo toast removes the expense after creation', async ({ page }) => {
    const groupName = `E2E Undo ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    await openGroup(page, session, 'Alice', groupName);

    // Create an expense — a toast with an Undo action appears
    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Undo Test Expense');
    await page.getByLabel('Amount').fill('50');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();
    await expect(page.getByText('Undo Test Expense')).toBeVisible();

    const undoButton = page.getByRole('button', { name: 'Undo', exact: true });
    await expect(undoButton).toBeVisible();
    await undoButton.click();

    await expect(page.getByText(/expense undone/i)).toBeVisible();
    await expect(page.getByText('Undo Test Expense')).toHaveCount(0);
  });

  test('API responses include CSP and CORS headers', async ({ request }) => {
    // Cross-origin request so the CORS header must echo the Origin
    const response = await request.post('/api/groups', {
      headers: { Origin: 'http://localhost:3000' },
      data: { name: 'x' },
    });

    const headers = response.headers();
    expect(headers['content-security-policy']).toContain("default-src 'self'");
    expect(headers['access-control-allow-origin']).toBe('http://localhost:3000');
  });

  test('can create expenses after navigating away and back', async ({ page }) => {
    const groupName = `E2E HLC ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    await openGroup(page, session, 'Alice', groupName);

    // First expense
    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('First Expense');
    await page.getByLabel('Amount').fill('100');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();
    await expect(page.getByText('First Expense')).toBeVisible();

    // Leave and come back to the group
    await page.getByRole('button', { name: '← Back' }).click();
    await expect(page.getByText('Your Groups')).toBeVisible();
    await page.getByText(groupName).first().click();
    await page.waitForURL(/\/groups\//);
    await expect(page.getByText(groupName).first()).toBeVisible();

    // Second expense — the HLC must have persisted across navigation
    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Second Expense');
    await page.getByLabel('Amount').fill('200');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();
    await expect(page.getByText('Second Expense')).toBeVisible();

    await expect(page.getByText('First Expense')).toBeVisible();
    await expect(page.getByText('Second Expense')).toBeVisible();
  });

  test('shows error state when API is unreachable', async ({ page }) => {
    const groupName = `E2E Error ${Date.now()}`;
    const session = await createGroup(groupName);
    await openGroup(page, session, 'Alice', groupName);

    // Break the API: the SPA still loads, but every request fails
    await page.route('**/api/**', (route) => route.abort());

    // Reload the group page — data load fails and shows the error state
    await page.reload();

    // The group page must show the error state with a Retry action
    await expect(page.locator('.alert-error')).toBeVisible();
    await expect(page.getByRole('button', { name: 'Retry', exact: true })).toBeVisible();
  });
});
