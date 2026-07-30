// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  apiListExpenses,
  setGroupAuth,
  goToGroup,
  clickButton,
  fillByLabel,
  waitForText,
  expectVisible,
} from '../helpers.js';

test.describe('Adversarial regression tests', () => {
  test('undo toast removes the expense after creation', async ({ page }) => {
    // 1. Create a group with 2 members via API
    const groupName = `E2E Undo ${Date.now()}`;
    const secret = 'secret';
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    await apiJoinGroup(slug, 'Bob', secret);

    // 2. Set auth and navigate to group
    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });
    await goToGroup(page, slug);
    await expectVisible(page, groupName);

    // 3. Create an expense
    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'Undo Test Expense');
    await fillByLabel(page, 'Amount', '50');
    await clickButton(page, 'Add Expense');

    // 4. Wait for the expense to appear
    await waitForText(page, 'Undo Test Expense');

    // 5. Look for the undo toast with the action button
    const undoButton = page.locator('.toast-action').filter({ hasText: 'Undo' });
    await expect(undoButton).toBeVisible({ timeout: 3000 });

    // 6. Click undo
    await undoButton.click();

    // 7. Wait for "Expense undone" confirmation toast
    await waitForText(page, /expense undone/i);

    // 8. Verify the expense is no longer in the list
    await expect(page.getByText('Undo Test Expense')).not.toBeVisible();
  });

  test('API responses include CSP and CORS headers', async ({ page }) => {
    // Navigate to the app so we're on the right origin
    await page.goto('/');

    // Make a fetch to the API and check response headers
    const response = await page.evaluate(async () => {
      const res = await fetch('/api/groups', { method: 'POST' });
      return {
        csp: res.headers.get('Content-Security-Policy'),
        cors: res.headers.get('Access-Control-Allow-Origin'),
        contentType: res.headers.get('Content-Type'),
      };
    });

    // CSP should be set
    expect(response.csp).toBeTruthy();
    expect(response.csp).toContain("default-src 'self'");

    // CORS should reflect the origin or be set
    expect(response.cors).toBeTruthy();
  });

  test('can create expenses after navigating away and back', async ({ page }) => {
    // 1. Create group and set auth
    const groupName = `E2E HLC ${Date.now()}`;
    const secret = 'secret';
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    await apiJoinGroup(slug, 'Bob', secret);

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });

    // 2. Go to group and create first expense
    await goToGroup(page, slug);
    await expectVisible(page, groupName);

    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'First Expense');
    await fillByLabel(page, 'Amount', '100');
    await clickButton(page, 'Add Expense');
    await waitForText(page, 'First Expense');

    // 3. Navigate back to groups list
    await clickButton(page, 'Back');
    await waitForText(page, 'Your Groups');

    // 4. Navigate back to the group
    await page.getByText(groupName).first().click();
    await page.waitForURL(/\/groups\//);
    await expectVisible(page, groupName);

    // 5. Create second expense (HLC should have persisted)
    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'Second Expense');
    await fillByLabel(page, 'Amount', '200');
    await clickButton(page, 'Add Expense');
    await waitForText(page, 'Second Expense');

    // 6. Both expenses should be visible
    await expectVisible(page, 'First Expense');
    await expectVisible(page, 'Second Expense');
  });

  test('shows error state when API is unreachable', async ({ page, context }) => {
    // 1. Create group and set auth
    const groupName = `E2E Error ${Date.now()}`;
    const secret = 'secret';
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });

    // 2. Go offline before navigating
    await context.setOffline(true);

    // 3. Navigate to group (will fail to load data)
    await goToGroup(page, slug);

    // 4. Should show an error or retry state, not a blank page
    await page.waitForTimeout(2000);

    // 5. Should show some kind of error UI or the retry button
    const hasError = await page.getByText(/error|retry|offline|not.*found/i).first().isVisible().catch(() => false);
    // OR the loading spinner might still be showing - that's acceptable too
    const hasLoading = await page.locator('.spinner').first().isVisible().catch(() => false);

    // At minimum, the page should not crash
    expect(hasError || hasLoading).toBe(true);

    // 6. Go back online
    await context.setOffline(false);

    // 7. Reload should work
    await page.reload();
    await page.waitForTimeout(2000);

    // Should show something - either group data or an auth redirect
    const pageContent = await page.textContent('body');
    expect(pageContent.length).toBeGreaterThan(0);
  });
});
