// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  apiCreateExpense,
  apiListExpenses,
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
});
