// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateExpense,
  apiCreatePayment,
  apiConfirmPayment,
  apiGetBalances,
  createGroup,
  openGroup,
  openTab,
} from '../helpers.js';

test.describe('Balance calculations and display', () => {
  test('shows settled state when everyone is balanced', async ({ page }) => {
    const groupName = `E2E Settled ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    // Alice pays $60, split equal (2 people → $30 each)
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Lunch', amount: 60, split_type: 'equal' });

    // Bob pays Alice his half; Alice confirms
    const payment = await apiCreatePayment(session.slug, bob.cookie_token, { from_user: bob.member_id, to_user: alice.member_id, amount: 30 });
    await apiConfirmPayment(payment.id, alice.cookie_token);

    // Everyone settled — API returns an empty recommendation list
    expect(await apiGetBalances(session.slug, alice.cookie_token)).toHaveLength(0);

    await openGroup(page, session, 'Alice', groupName);
    await openTab(page, 'Balance');
    await expect(page.getByText(/all balanced up/i)).toBeVisible();
  });

  test('shows correct balance after expense (no payment yet)', async ({ page }) => {
    const groupName = `E2E Owed ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    // Alice pays $100 dinner (split equal → $50 each)
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Dinner', amount: 100, split_type: 'equal' });

    const balances = await apiGetBalances(session.slug, alice.cookie_token);
    expect(balances).toHaveLength(1);
    expect(balances[0]).toMatchObject({ from: bob.member_id, to: alice.member_id, amount: 50 });

    await openGroup(page, session, 'Alice', groupName);
    await openTab(page, 'Balance');
    await expect(page.getByText(/Bob/).first()).toBeVisible();
    await expect(page.getByText(/Alice/).first()).toBeVisible();
    await expect(page.getByText('$50.00').first()).toBeVisible();
  });

  test('shows per-expense breakdown in balances', async ({ page }) => {
    const groupName = `E2E Breakdown ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Dinner', amount: 80, split_type: 'equal' });
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Taxi', amount: 20, split_type: 'equal' });

    const balances = await apiGetBalances(session.slug, alice.cookie_token);
    expect(balances).toHaveLength(1);
    expect(balances[0].amount).toBe(50);
    expect(balances[0].breakdown.map((b) => b.expense_name)).toEqual(expect.arrayContaining(['Dinner', 'Taxi']));

    await openGroup(page, session, 'Alice', groupName);
    await openTab(page, 'Balance');
    await expect(page.getByText('$50.00').first()).toBeVisible();
    await expect(page.getByText(/Dinner/).first()).toBeVisible();
    await expect(page.getByText(/Taxi/).first()).toBeVisible();
  });

  test('shows reduced balance after partial payment', async ({ page }) => {
    const groupName = `E2E Partial ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    // Alice pays $100 for dinner (split equal → $50 each)
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Fancy Dinner', amount: 100, split_type: 'equal' });

    // Bob pays $20 of his $50 debt
    const payment = await apiCreatePayment(session.slug, bob.cookie_token, { from_user: bob.member_id, to_user: alice.member_id, amount: 20 });
    await apiConfirmPayment(payment.id, alice.cookie_token);

    const balances = await apiGetBalances(session.slug, alice.cookie_token);
    expect(balances).toHaveLength(1);
    expect(balances[0]).toMatchObject({ from: bob.member_id, to: alice.member_id, amount: 30 });

    await openGroup(page, session, 'Alice', groupName);
    await openTab(page, 'Balance');
    await expect(page.getByText('$30.00').first()).toBeVisible();
  });
});
