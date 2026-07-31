// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateExpense,
  apiListExpenses,
  apiListPayments,
  createGroup,
  openGroup,
  openTab,
  pendingSyncBadge,
} from '../helpers.js';

test.describe('Offline sync', () => {
  test('creates an expense while offline and syncs when back online', async ({ page, context }) => {
    const groupName = `E2E Offline ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');

    // Load the group while online
    await openGroup(page, session, 'Alice', groupName);
    await expect(page.getByRole('button', { name: /^Expenses/ })).toBeVisible();

    // Go offline and create an expense — it is queued locally
    await context.setOffline(true);
    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Offline Dinner');
    await page.getByLabel('Amount').fill('75');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();

    await expect(page.getByText('Offline Dinner')).toBeVisible();
    await expect(page.getByText('$75.00').first()).toBeVisible();
    await expect(pendingSyncBadge(page).first()).toBeVisible();

    // Back online — the local queue is pushed and the badge clears
    await context.setOffline(false);
    await expect(pendingSyncBadge(page)).toHaveCount(0, { timeout: 10000 });

    const expenses = await apiListExpenses(session.slug, alice.cookie_token);
    const offlineExpense = expenses.find((e) => e.description === 'Offline Dinner');
    expect(offlineExpense).toBeTruthy();
    expect(Number(offlineExpense.amount)).toBeCloseTo(75, 1);

    await expect(page.getByText('Offline Dinner')).toBeVisible();
    await expect(page.getByText('$75.00').first()).toBeVisible();
  });

  test('records a payment while offline and syncs when back online', async ({ page, context }) => {
    const groupName = `E2E OfflinePay ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    await openGroup(page, session, 'Alice', groupName);
    await expect(page.getByRole('button', { name: /^Expenses/ })).toBeVisible();

    await context.setOffline(true);
    await openTab(page, 'Payments');
    await expect(page.getByText(/no payments/i)).toBeVisible();

    await page.getByRole('button', { name: '+ Record Payment', exact: true }).click();
    await page.getByLabel('To (recipient)').selectOption(bob.member_id);
    await page.getByLabel('Amount').fill('25');
    await page.getByLabel('Method (optional)').fill('Cash');
    await page.getByRole('button', { name: 'Record Payment', exact: true }).click();

    await expect(page.getByText('$25.00').first()).toBeVisible();
    await expect(pendingSyncBadge(page).first()).toBeVisible();

    await context.setOffline(false);
    await expect(pendingSyncBadge(page)).toHaveCount(0, { timeout: 10000 });

    const payments = await apiListPayments(session.slug, alice.cookie_token);
    const offlinePayment = payments.find((p) => Number(p.amount) === 25);
    expect(offlinePayment).toBeTruthy();
    expect(offlinePayment.from_user).toBe(alice.member_id);
    expect(offlinePayment.to_user).toBe(bob.member_id);

    await expect(page.getByText('$25.00')).toBeVisible();
  });

  test('creates multiple offline operations and syncs all when back online', async ({ page, context }) => {
    const groupName = `E2E MultiOffline ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');

    await openGroup(page, session, 'Alice', groupName);

    await context.setOffline(true);

    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Offline Groceries');
    await page.getByLabel('Amount').fill('60');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();
    await expect(page.getByText('Offline Groceries')).toBeVisible();

    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Offline Gas');
    await page.getByLabel('Amount').fill('40');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();
    await expect(page.getByText('Offline Gas')).toBeVisible();

    await expect(pendingSyncBadge(page)).toHaveCount(2);

    await context.setOffline(false);
    await expect(pendingSyncBadge(page)).toHaveCount(0, { timeout: 10000 });

    const expenses = await apiListExpenses(session.slug, alice.cookie_token);
    expect(expenses.find((e) => e.description === 'Offline Groceries')).toBeTruthy();
    expect(expenses.find((e) => e.description === 'Offline Gas')).toBeTruthy();

    await expect(page.getByText('Offline Groceries')).toBeVisible();
    await expect(page.getByText('Offline Gas')).toBeVisible();
  });

  test('edits an expense while offline and syncs when back online', async ({ page, context }) => {
    const groupName = `E2E OfflineEdit ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');

    // Create the expense online first
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Original Dinner', amount: 80, split_type: 'equal' });

    await openGroup(page, session, 'Alice', groupName);
    await expect(page.getByText('Original Dinner')).toBeVisible();

    await context.setOffline(true);

    // Edit the expense while offline
    await page.getByRole('button', { name: 'Edit expense' }).click();
    await page.getByLabel('Description').fill('Edited Dinner');
    await page.getByLabel('Amount').fill('90');
    await page.getByRole('button', { name: 'Update Expense', exact: true }).click();

    await expect(page.getByText('Edited Dinner').first()).toBeVisible();
    await expect(page.getByText('$90.00').first()).toBeVisible();
    await expect(pendingSyncBadge(page).first()).toBeVisible();

    await context.setOffline(false);
    await expect(pendingSyncBadge(page)).toHaveCount(0, { timeout: 10000 });

    const expenses = await apiListExpenses(session.slug, alice.cookie_token);
    expect(expenses.find((e) => e.description === 'Edited Dinner')).toBeTruthy();
    expect(expenses.find((e) => e.description === 'Original Dinner')).toBeFalsy();

    await expect(page.getByText('Edited Dinner').first()).toBeVisible();
    await expect(page.getByText('$90.00').first()).toBeVisible();
  });
});
