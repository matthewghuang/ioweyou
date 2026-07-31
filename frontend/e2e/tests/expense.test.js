// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateExpense,
  apiListExpenses,
  createGroup,
  openGroup,
} from '../helpers.js';

test.describe('Expense management', () => {
  test('adds an expense with equal split', async ({ page }) => {
    const groupName = `E2E Expense ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    await openGroup(page, session, 'Alice', groupName);

    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Dinner');
    await page.getByLabel('Amount').fill('100');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();

    await expect(page.getByText('Dinner')).toBeVisible();
    await expect(page.getByText('$100.00').first()).toBeVisible();
  });

  test('adds an expense with custom split', async ({ page }) => {
    const groupName = `E2E Custom Split ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    await openGroup(page, session, 'Alice', groupName);

    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Pizza');
    await page.getByLabel('Amount').fill('60');
    await page.getByLabel('Split type').selectOption('custom');

    // One split input per member
    const splitInputs = page.locator('.split-input');
    await expect(splitInputs).toHaveCount(2);
    await splitInputs.nth(0).fill('40');
    await splitInputs.nth(1).fill('20');

    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();
    await expect(page.getByText('Pizza')).toBeVisible();
    await expect(page.getByText('$60.00').first()).toBeVisible();
  });

  test('shows empty state when no expenses exist', async ({ page }) => {
    const groupName = `E2E Empty ${Date.now()}`;
    const session = await createGroup(groupName);
    await openGroup(page, session, 'Alice', groupName);

    await expect(page.getByText(/no expenses/i)).toBeVisible();
  });

  test('submits an expense without console errors when crypto.randomUUID is unavailable', async ({ page }) => {
    // `crypto.randomUUID` only exists in secure contexts (HTTPS/localhost) and
    // modern browsers. On plain-HTTP origins (LAN access, older webviews) it is
    // undefined, which used to throw in the submit handler and block expense
    // creation. Simulate such a browser and require a clean submission.
    await page.addInitScript(() => {
      try { Crypto.prototype.randomUUID = undefined; } catch { /* noop */ }
      try {
        Object.defineProperty(globalThis.crypto, 'randomUUID', { value: undefined, configurable: true });
      } catch { /* noop */ }
    });

    const errors = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error' || msg.type() === 'warning') {
        errors.push(`[${msg.type()}] ${msg.text()}`);
      }
    });
    page.on('pageerror', (err) => errors.push(`[pageerror] ${err.message}`));

    const groupName = `E2E No RandomUUID ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    await openGroup(page, session, 'Alice', groupName);

    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Coffee');
    await page.getByLabel('Amount').fill('12');
    await page.getByLabel('Split type').selectOption('custom');

    // Custom split exercises both randomUUID call sites (expense id + split item ids).
    const splitInputs = page.locator('.split-input');
    await splitInputs.nth(0).fill('7');
    await splitInputs.nth(1).fill('5');

    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();

    await expect(page.getByText('Coffee')).toBeVisible();
    await expect(page.getByText('$12.00').first()).toBeVisible();
    expect(errors, `Console/page errors while submitting:\n${errors.join('\n')}`).toHaveLength(0);
  });

  test('lists expenses via API match what UI shows', async ({ page }) => {
    const groupName = `E2E API Verify ${Date.now()}`;
    const session = await createGroup(groupName);
    const alice = session.member('Alice');

    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Groceries', amount: 50, split_type: 'equal' });
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Uber', amount: 30, split_type: 'equal' });

    await openGroup(page, session, 'Alice', groupName);
    await expect(page.getByText('Groceries')).toBeVisible();
    await expect(page.getByText('Uber')).toBeVisible();

    const apiExpenses = await apiListExpenses(session.slug, alice.cookie_token);
    expect(apiExpenses.map((e) => e.description)).toEqual(expect.arrayContaining(['Groceries', 'Uber']));
  });
});
