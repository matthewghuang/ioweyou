// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  apiCreateExpense,
  apiListExpenses,
  apiListPayments,
  setGroupAuth,
  goToGroup,
  clickButton,
  fillByLabel,
  waitForText,
  expectVisible,
} from '../helpers.js';

test.describe('Offline sync', () => {
  test('creates an expense while offline and syncs when back online', async ({ page, context }) => {
    // 1. Create a group with 2 members via API (online setup)
    const groupName = `E2E Offline ${Date.now()}`;
    const secret = 'secret';
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    await apiJoinGroup(slug, 'Bob', secret);

    // 2. Set auth in localStorage and navigate to group (data loads successfully)
    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });
    await goToGroup(page, slug);
    await expectVisible(page, groupName);
    await expectVisible(page, /expenses/i);

    // 3. Simulate going offline via the browser context API.
    //    This sets navigator.onLine=false and dispatches the 'offline' event,
    //    which the app's online store subscribes to. All fetch requests will
    //    fail until we toggle back online.
    await context.setOffline(true);

    // 4. Open expense form and fill it out while offline
    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'Offline Dinner');
    await fillByLabel(page, 'Amount', '75');
    // Default split type is "equal"
    await clickButton(page, 'Add Expense');

    // 5. The expense should appear in the UI with a "Pending" badge.
    //    The handleCreateExpense function detects the offline state,
    //    queues CRDT operations locally, and adds the expense to the
    //    UI array with _pending:true.
    await waitForText(page, 'Offline Dinner');
    await expectVisible(page, '$75.00');

    // Look for the Pending badge on the newly created expense
    const pendingBadge = page.locator('.badge-warning').filter({ hasText: 'Pending' });
    await expect(pendingBadge.first()).toBeVisible({ timeout: 3000 });

    // 6. Go back online.
    //    This sets navigator.onLine=true and dispatches the 'online' event,
    //    which triggers the GroupDetail $effect online subscriber.
    //    The subscriber calls syncGroup(s) + loadAll() to push pending
    //    CRDT operations and reload data from the server.
    await context.setOffline(false);

    // 7. Wait for sync to complete — the Pending badge should disappear
    //    as the expense is reloaded from the server (no _pending flag).
    await expect(pendingBadge.first()).not.toBeVisible({ timeout: 10000 });

    // 8. Verify via direct API call that the expense was persisted server-side
    const expenses = await apiListExpenses(slug, cookie_token);
    const offlineExpense = expenses.find(e => e.description === 'Offline Dinner');
    expect(offlineExpense).toBeTruthy();
    expect(Number(offlineExpense.amount)).toBeCloseTo(75, 1);

    // 9. Verify the UI still shows the expense without Pending badge
    await expectVisible(page, 'Offline Dinner');
    await expectVisible(page, '$75.00');
  });

  test('records a payment while offline and syncs when back online', async ({ page, context }) => {
    const groupName = `E2E OfflinePay ${Date.now()}`;
    const secret = 'secret';
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Set Alice's auth and navigate
    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });
    await goToGroup(page, slug);
    await expectVisible(page, groupName);
    await expectVisible(page, /expenses/i);

    // Go offline
    await context.setOffline(true);

    // Navigate to Payments tab
    await page.locator('button.tab').filter({ hasText: 'Payments' }).click();
    await waitForText(page, /no payments/i);

    // Record a payment
    await clickButton(page, '+ Record Payment');
    // "From" shows current member (Alice) — static text
    // Select "To" = Bob
    await page.selectOption('#pay-to', bob.member_id);
    await fillByLabel(page, 'Amount', '25');
    await fillByLabel(page, 'Method (optional)', 'Cash');
    await clickButton(page, 'Record Payment');

    // Verify payment appears with Pending badge
    await waitForText(page, /pending/i);
    await expectVisible(page, '$25.00');

    const pendingBadge = page.locator('.badge-warning').filter({ hasText: 'Pending' });
    await expect(pendingBadge.first()).toBeVisible({ timeout: 3000 });

    // Go back online
    await context.setOffline(false);

    // Wait for sync — Pending badge should disappear
    await expect(pendingBadge.first()).not.toBeVisible({ timeout: 10000 });

    // Verify via API
    const payments = await apiListPayments(slug, cookie_token);
    const offlinePayment = payments.find(p => Number(p.amount) === 25);
    expect(offlinePayment).toBeTruthy();
    expect(offlinePayment.from_user).toBe(member_id);
    expect(offlinePayment.to_user).toBe(bob.member_id);

    // Verify UI still shows the payment
    await expectVisible(page, '$25.00');
  });

  test('creates multiple offline operations and syncs all when back online', async ({ page, context }) => {
    const groupName = `E2E MultiOffline ${Date.now()}`;
    const secret = 'secret';
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });
    await goToGroup(page, slug);
    await expectVisible(page, groupName);

    // Go offline
    await context.setOffline(true);

    // Create expense #1
    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'Offline Groceries');
    await fillByLabel(page, 'Amount', '60');
    await clickButton(page, 'Add Expense');
    await waitForText(page, 'Offline Groceries');

    // Create expense #2
    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'Offline Gas');
    await fillByLabel(page, 'Amount', '40');
    await clickButton(page, 'Add Expense');
    await waitForText(page, 'Offline Gas');

    // Both should have Pending badges
    const pendingBadges = page.locator('.badge-warning').filter({ hasText: 'Pending' });
    await expect(pendingBadges.first()).toBeVisible({ timeout: 3000 });
    await expect(pendingBadges).toHaveCount(2);

    // Go back online
    await context.setOffline(false);

    // Wait for sync — all pending badges should disappear
    await expect(pendingBadges.first()).not.toBeVisible({ timeout: 10000 });
    await expect(pendingBadges).toHaveCount(0);

    // Verify both expenses persisted via API
    const expenses = await apiListExpenses(slug, cookie_token);
    expect(expenses.find(e => e.description === 'Offline Groceries')).toBeTruthy();
    expect(expenses.find(e => e.description === 'Offline Gas')).toBeTruthy();

    // UI should show both without Pending
    await expectVisible(page, 'Offline Groceries');
    await expectVisible(page, 'Offline Gas');
  });

  test('edits an expense while offline and syncs when back online', async ({ page, context }) => {
    const groupName = `E2E OfflineEdit ${Date.now()}`;
    const secret = 'secret';
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    await apiJoinGroup(slug, 'Bob', secret);

    // Create an expense online first
    await apiCreateExpense(slug, cookie_token, {
      description: 'Original Dinner',
      amount: 80,
      split_type: 'equal',
    });

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });
    await goToGroup(page, slug);
    await expectVisible(page, groupName);
    await waitForText(page, 'Original Dinner');

    // Go offline
    await context.setOffline(true);

    // Edit the expense
    await clickButton(page, 'Edit');
    // The form should be pre-filled with the expense data
    await fillByLabel(page, 'Description', '');
    await fillByLabel(page, 'Description', 'Edited Dinner');
    await fillByLabel(page, 'Amount', '');
    await fillByLabel(page, 'Amount', '90');
    await clickButton(page, 'Update Expense');

    // Should show the updated values
    await waitForText(page, 'Edited Dinner');
    await expectVisible(page, '$90.00');

    // Should show Pending badge on the edited expense
    const pendingBadge = page.locator('.badge-warning').filter({ hasText: 'Pending' });
    await expect(pendingBadge.first()).toBeVisible({ timeout: 3000 });

    // Go back online
    await context.setOffline(false);

    // Wait for sync
    await expect(pendingBadge.first()).not.toBeVisible({ timeout: 10000 });

    // Verify via API
    const expenses = await apiListExpenses(slug, cookie_token);
    const edited = expenses.find(e => e.description === 'Edited Dinner');
    expect(edited).toBeTruthy();
    expect(Number(edited.amount)).toBeCloseTo(90, 1);

    // The old description should no longer exist
    expect(expenses.find(e => e.description === 'Original Dinner')).toBeFalsy();

    // UI should show the edited version
    await expectVisible(page, 'Edited Dinner');
    await expectVisible(page, '$90.00');
  });
});
