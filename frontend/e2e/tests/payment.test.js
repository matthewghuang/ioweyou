// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateExpense,
  apiCreatePayment,
  apiListPayments,
  createGroup,
  openGroup,
  openTab,
} from '../helpers.js';

test.describe('Payment flow', () => {
  test('records a payment as the current member', async ({ page }) => {
    const groupName = `E2E Payment ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    // Pre-create an expense so the group has context
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Dinner', amount: 100, split_type: 'equal' });

    // Auth as Bob — the payment "from" is always the current member
    await openGroup(page, session, 'Bob', groupName);
    await openTab(page, 'Payments');
    await expect(page.getByText(/no payments/i)).toBeVisible();

    await page.getByRole('button', { name: '+ Record Payment', exact: true }).click();
    await page.getByLabel('To (recipient)').selectOption(alice.member_id);
    await page.getByLabel('Amount').fill('50');
    await page.getByLabel('Method (optional)').fill('Venmo');
    await page.getByRole('button', { name: 'Record Payment', exact: true }).click();

    await expect(page.getByText('pending', { exact: true })).toBeVisible();
    await expect(page.getByText('$50.00').first()).toBeVisible();
    await expect(page.getByText(/Bob.*→.*Alice|Bob.*Alice/).first()).toBeVisible();
  });

  test('confirms a payment as the recipient', async ({ page }) => {
    const groupName = `E2E Confirm ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Dinner', amount: 100, split_type: 'equal' });
    await apiCreatePayment(session.slug, bob.cookie_token, { from_user: bob.member_id, to_user: alice.member_id, amount: 50 });

    await openGroup(page, session, 'Alice', groupName);
    await openTab(page, 'Payments');
    await expect(page.getByText('pending', { exact: true })).toBeVisible();
    await expect(page.getByText('$50.00').first()).toBeVisible();

    await page.getByRole('button', { name: 'Confirm', exact: true }).click();
    await expect(page.getByText('confirmed', { exact: true })).toBeVisible();
  });

  test('cancels a payment via the confirmation sheet', async ({ page }) => {
    const groupName = `E2E Cancel ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    await apiCreatePayment(session.slug, bob.cookie_token, { from_user: bob.member_id, to_user: alice.member_id, amount: 25 });

    await openGroup(page, session, 'Alice', groupName);
    await openTab(page, 'Payments');
    await expect(page.getByText('pending', { exact: true })).toBeVisible();

    // The payment row's Cancel button opens the confirmation sheet
    await page.getByRole('button', { name: 'Cancel', exact: true }).first().click();
    await expect(page.getByRole('button', { name: 'Cancel Payment', exact: true })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel Payment', exact: true }).click();

    // Payment disappears from the list
    await expect(page.getByText('$25.00')).toHaveCount(0);
    await expect(page.getByText(/no payments/i)).toBeVisible();
  });

  test('lists payments from API match what UI shows', async ({ page }) => {
    const groupName = `E2E PayList ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    await apiCreatePayment(session.slug, bob.cookie_token, { from_user: bob.member_id, to_user: alice.member_id, amount: 30, method: 'Cash' });
    await apiCreatePayment(session.slug, alice.cookie_token, { from_user: alice.member_id, to_user: bob.member_id, amount: 15, method: 'Venmo' });

    await openGroup(page, session, 'Alice', groupName);
    await openTab(page, 'Payments');
    await expect(page.getByText('$30.00').first()).toBeVisible();
    await expect(page.getByText('$15.00').first()).toBeVisible();

    const apiPayments = await apiListPayments(session.slug, alice.cookie_token);
    expect(apiPayments).toHaveLength(2);
    await expect(page.getByText('→').first()).toBeVisible();
  });
});
