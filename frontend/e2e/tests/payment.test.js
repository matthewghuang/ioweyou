// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  apiCreateExpense,
  apiCreatePayment,
  apiConfirmPayment,
  apiListPayments,
  setGroupAuth,
  goToGroup,
  clickButton,
  fillByLabel,
  waitForText,
  expectVisible,
} from '../helpers.js';

test.describe('Payment flow', () => {
  test('records a payment as the current member', async ({ page }) => {
    const groupName = `E2E Payment ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);

    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Pre-create an expense via API so the group has context
    await apiCreateExpense(slug, aliceToken, {
      description: 'Dinner',
      amount: 100,
      split_type: 'equal',
    });

    // Auth as Bob — the payment "from" field is always the current member
    await setGroupAuth(page, slug, bob.cookie_token, {
      name: groupName,
      member_name: 'Bob',
      member_id: bob.member_id,
      internal_id,
    });

    await goToGroup(page, slug);

    // Navigate to Payments tab using the tab button text
    await page.locator('button.tab').filter({ hasText: 'Payments' }).click();
    await waitForText(page, /no payments/i);

    // Click "+ Record Payment" to open the form
    await clickButton(page, '+ Record Payment');

    // "From" is shown as static text (always the current member = Bob)
    // Verify the static display shows "Bob" or similar
    await expect(page.getByText('Bob').first()).toBeVisible();

    // Select "To" = Alice
    await page.selectOption('#pay-to', aliceId);

    // Fill Amount
    await fillByLabel(page, 'Amount', '50');

    // Fill Method (optional)
    await fillByLabel(page, 'Method (optional)', 'Venmo');

    // Click submit button "Record Payment"
    await clickButton(page, 'Record Payment');

    // Payment should appear in the list with pending status
    await waitForText(page, /pending/i);
    await expectVisible(page, '$50.00');
    // Verify the direction shows "Bob → Alice"
    await expectVisible(page, /Bob.*→.*Alice|Bob.*Alice/);
  });

  test('confirms a payment as the recipient', async ({ page }) => {
    const groupName = `E2E Confirm ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);

    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Create an expense as Alice
    await apiCreateExpense(slug, aliceToken, {
      description: 'Dinner',
      amount: 100,
      split_type: 'equal',
    });

    // Create a payment as Bob → Alice via API
    const payment = await apiCreatePayment(slug, bob.cookie_token, {
      from_user: bob.member_id,
      to_user: aliceId,
      amount: 50,
    });

    // Auth as Alice (the recipient) — she should see the Confirm button
    await setGroupAuth(page, slug, aliceToken, {
      name: groupName,
      member_name: 'Alice',
      member_id: aliceId,
      internal_id,
    });

    await goToGroup(page, slug);
    await page.locator('button.tab').filter({ hasText: 'Payments' }).click();

    // The payment should be visible with a pending badge
    await waitForText(page, /pending/i);
    await expectVisible(page, '$50.00');

    // Alice is the recipient — the "Confirm" button should be visible
    const confirmBtn = page.getByRole('button', { name: 'Confirm' });
    await expect(confirmBtn).toBeVisible();
    await confirmBtn.click();

    // Status should change to confirmed
    await waitForText(page, /confirmed/i);
  });

  test('cancels a payment', async ({ page }) => {
    const groupName = `E2E Cancel ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);

    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Create a payment via API (Bob → Alice)
    const payment = await apiCreatePayment(slug, bob.cookie_token, {
      from_user: bob.member_id,
      to_user: aliceId,
      amount: 25,
    });

    // Auth as Alice
    await setGroupAuth(page, slug, aliceToken, {
      name: groupName,
      member_name: 'Alice',
      member_id: aliceId,
      internal_id,
    });

    await goToGroup(page, slug);
    await page.locator('button.tab').filter({ hasText: 'Payments' }).click();
    await waitForText(page, /pending/i);

    // Handle the confirmation dialog
    page.once('dialog', (dialog) => {
      expect(dialog.message()).toContain('Cancel');
      dialog.accept();
    });

    await clickButton(page, 'Cancel');

    // After cancellation the payment should disappear
    // Either show "no payments" or show cancelled status text
    await page.waitForTimeout(500);
    const hasEmptyState = await page.getByText(/no payments/i).isVisible().catch(() => false);
    expect(hasEmptyState).toBeTruthy();
  });

  test('lists payments from API match what UI shows', async ({ page }) => {
    const groupName = `E2E PayList ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Create two payments via API
    const p1 = await apiCreatePayment(slug, bob.cookie_token, {
      from_user: bob.member_id, to_user: aliceId, amount: 30, method: 'Cash',
    });
    const p2 = await apiCreatePayment(slug, aliceToken, {
      from_user: aliceId, to_user: bob.member_id, amount: 15, method: 'Venmo',
    });

    // View as Alice
    await setGroupAuth(page, slug, aliceToken, {
      name: groupName,
      member_name: 'Alice',
      member_id: aliceId,
      internal_id,
    });

    await goToGroup(page, slug);
    await page.locator('button.tab').filter({ hasText: 'Payments' }).click();

    // Both payments should appear
    await expectVisible(page, '$30.00');
    await expectVisible(page, '$15.00');

    // API should return the same count
    const apiPayments = await apiListPayments(slug, aliceToken);
    expect(apiPayments.length).toBe(2);

    // Verify payment directions shown
    await expectVisible(page, /→/); // arrow character between names
  });
});
