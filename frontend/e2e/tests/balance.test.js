// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  apiCreateExpense,
  apiCreatePayment,
  apiConfirmPayment,
  apiGetBalances,
  setGroupAuth,
  goToGroup,
  clickButton,
  waitForText,
  expectVisible,
} from '../helpers.js';

test.describe('Balance calculations and display', () => {
  test('shows settled state when everyone is balanced', async ({ page }) => {
    const groupName = `E2E Settled ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Alice pays $60, split equal (2 people → $30 each)
    await apiCreateExpense(slug, aliceToken, {
      description: 'Lunch',
      amount: 60,
      split_type: 'equal',
    });

    // Bob pays Alice $30 (his half)
    const payment = await apiCreatePayment(slug, bob.cookie_token, {
      from_user: bob.member_id,
      to_user: aliceId,
      amount: 30,
    });

    // Confirm payment as Alice (recipient)
    await apiConfirmPayment(payment.id, aliceToken);

    // Verify via API — should be empty/everyone settled
    const apiBalances = await apiGetBalances(slug, aliceToken);
    expect(apiBalances.length).toBe(0);

    // View in UI
    await setGroupAuth(page, slug, aliceToken, {
      name: groupName,
      member_name: 'Alice',
      member_id: aliceId,
      internal_id,
    });

    await goToGroup(page, slug);
    await page.locator('button.tab').filter({ hasText: 'Balance' }).click();

    // Should show "All balanced up!"
    await expectVisible(page, /all balanced up/i);
  });

  test('shows correct balance after expense (no payment yet)', async ({ page }) => {
    const groupName = `E2E Owed ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Alice pays $100 dinner (split equal → $50 each)
    await apiCreateExpense(slug, aliceToken, {
      description: 'Dinner',
      amount: 100,
      split_type: 'equal',
    });

    // Bob owes Alice $50
    const apiBalances = await apiGetBalances(slug, aliceToken);
    expect(apiBalances.length).toBe(1);
    expect(apiBalances[0].from).toBe(bob.member_id);
    expect(apiBalances[0].to).toBe(aliceId);
    expect(apiBalances[0].amount).toBe(50);

    // View in UI
    await setGroupAuth(page, slug, aliceToken, {
      name: groupName,
      member_name: 'Alice',
      member_id: aliceId,
      internal_id,
    });

    await goToGroup(page, slug);
    await page.locator('button.tab').filter({ hasText: 'Balance' }).click();

    // Should show the balance: Bob owes Alice $50
    await expectVisible(page, /Bob/);
    await expectVisible(page, /Alice/);
    await expectVisible(page, '$50.00');
  });

  test('shows per-expense breakdown in balances', async ({ page }) => {
    const groupName = `E2E Breakdown ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Multiple expenses
    await apiCreateExpense(slug, aliceToken, {
      description: 'Dinner',
      amount: 80,
      split_type: 'equal',
    });
    await apiCreateExpense(slug, aliceToken, {
      description: 'Taxi',
      amount: 20,
      split_type: 'equal',
    });

    // Bob owes Alice $50 total ($40 dinner + $10 taxi)
    const balances = await apiGetBalances(slug, aliceToken);
    expect(balances.length).toBe(1);
    expect(balances[0].from).toBe(bob.member_id);
    expect(balances[0].to).toBe(aliceId);
    expect(balances[0].amount).toBe(50);

    // Verify breakdown has both expense names
    const breakdownNames = balances[0].breakdown.map(b => b.expense_name);
    expect(breakdownNames).toContain('Dinner');
    expect(breakdownNames).toContain('Taxi');

    // View in UI
    await setGroupAuth(page, slug, aliceToken, {
      name: groupName,
      member_name: 'Alice',
      member_id: aliceId,
      internal_id,
    });

    await goToGroup(page, slug);
    await page.locator('button.tab').filter({ hasText: 'Balance' }).click();

    // Should show the balance breakdown
    await expectVisible(page, /Bob/);
    await expectVisible(page, /Alice/);
    await expectVisible(page, '$50.00');
    // Breakdown should contain expense names
    await expectVisible(page, /Dinner/);
    await expectVisible(page, /Taxi/);
  });

  test('shows reduced balance after partial payment', async ({ page }) => {
    const groupName = `E2E Partial ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Alice pays $100 for dinner (split equal → $50 each)
    await apiCreateExpense(slug, aliceToken, {
      description: 'Fancy Dinner',
      amount: 100,
      split_type: 'equal',
    });

    // Bob pays Alice $20 (partial payment)
    const payment = await apiCreatePayment(slug, bob.cookie_token, {
      from_user: bob.member_id,
      to_user: aliceId,
      amount: 20,
    });
    await apiConfirmPayment(payment.id, aliceToken);

    // Bob still owes Alice $30
    const apiBalances = await apiGetBalances(slug, aliceToken);
    expect(apiBalances.length).toBe(1);
    expect(apiBalances[0].from).toBe(bob.member_id);
    expect(apiBalances[0].to).toBe(aliceId);
    expect(apiBalances[0].amount).toBe(30);

    // View in UI
    await setGroupAuth(page, slug, aliceToken, {
      name: groupName,
      member_name: 'Alice',
      member_id: aliceId,
      internal_id,
    });

    await goToGroup(page, slug);
    await page.locator('button.tab').filter({ hasText: 'Balance' }).click();

    // Should show Bob still owes Alice $30
    await expectVisible(page, '$30.00');
  });
});
