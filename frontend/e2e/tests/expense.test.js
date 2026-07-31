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
