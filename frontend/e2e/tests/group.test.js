// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  apiGroupInfo,
  setGroupAuth,
  goToGroup,
  clickButton,
  fillByLabel,
  waitForText,
  expectVisible,
} from '../helpers.js';

test.describe('Group creation and joining', () => {
  test('creates a group from the landing page and sees it in the list', async ({ page }) => {
    const groupName = `E2E Test Group ${Date.now()}`;
    const creatorName = 'Alice';
    const secret = 'test-secret-123';

    // 1. Go to the landing page
    await page.goto('/');
    await page.waitForLoadState('networkidle');

    // 2. Fill in the creation form
    await page.waitForSelector('text=Create Group');
    await fillByLabel(page, 'Group name', groupName);
    await fillByLabel(page, 'Your name', creatorName);
    await fillByLabel(page, 'Password', secret);

    // 3. Submit
    await clickButton(page, 'Create Group');

    // 4. Should see success message with 'Go to Groups' button
    await expectVisible(page, /Group created/i);
    await clickButton(page, 'Go to Groups');

    // 5. Should now be on the groups list page showing the new group
    await expectVisible(page, groupName);
    await expectVisible(page, creatorName);

    // 5. Group slug should be stored — click the group to go to detail
    const groupCard = page.getByText(groupName).first();
    await groupCard.click();

    // Should land on group detail page with tabs
    await page.waitForURL(/\/groups\//);
    await expectVisible(page, groupName);
    await expectVisible(page, 'Expenses');
    await expectVisible(page, 'Payments');
    await expectVisible(page, 'Balance');
  });

  test('joins an existing group via invite link', async ({ page, context }) => {
    // Setup: create a group via API first (as Alice)
    const groupName = `E2E Join Test ${Date.now()}`;
    const aliceName = 'Alice';
    const secret = 'shared-secret';

    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, aliceName, secret);

    // 1. Navigate to the invite link as a fresh user (no auth → join page)
    //    Visit the app first so we're on the right origin, then clear any stray state
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    await page.evaluate(() => localStorage.clear());
    await page.goto(`/group/${slug}`);
    await page.goto(`/group/${slug}`);
    await page.waitForLoadState('networkidle');

    // 2. Should show the join form with the group info loaded
    await expectVisible(page, 'Join Group');
    await expectVisible(page, groupName);

    // 3. Fill in Bob's details
    await fillByLabel(page, 'Your name', 'Bob');
    await fillByLabel(page, 'Password', secret);

    // 4. Submit
    await clickButton(page, 'Join Group');

    // 5. Should redirect to groups list, both Alice and Bob's group visible
    await expectVisible(page, groupName);
  });

  test('requires existing member password to re-join', async ({ page }) => {
    const groupName = `E2E Rejoin ${Date.now()}`;
    const { slug } = await apiCreateGroup(groupName, 'Alice', 'alice-secret');

    // First, join as Bob with a known password
    await apiJoinGroup(slug, 'Bob', 'bob-password');

    // Navigate to join page fresh (no stored auth)
    await page.goto('/');
    await page.waitForLoadState('domcontentloaded');
    await page.evaluate(() => localStorage.clear());
    await page.goto(`/group/${slug}`);
    await page.waitForLoadState('networkidle');
    await expectVisible(page, groupName);

    // Fill Bob's name with WRONG password
    await fillByLabel(page, 'Your name', 'Bob');
    await fillByLabel(page, 'Password', 'wrong-password');
    await clickButton(page, 'Join Group');

    // Should show an error
    await expectVisible(page, /invalid secret/i);

    // Now try with CORRECT password
    await fillByLabel(page, 'Your name', 'Bob');
    await fillByLabel(page, 'Password', 'bob-password');
    await clickButton(page, 'Join Group');

    // Should succeed and redirect to groups
    await expectVisible(page, groupName);
  });

  test('shows group detail page with correct info', async ({ page }) => {
    const groupName = `E2E Info Test ${Date.now()}`;
    const creatorName = 'Alice';
    const secret = 'test-secret';

    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, creatorName, secret);

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: creatorName,
      member_id,
      internal_id,
    });

    await goToGroup(page, slug);

    // Verify group detail shows the group name
    await expectVisible(page, groupName);

    // Verify tabs are present
    await expect(page.locator('button.tab').filter({ hasText: 'Expenses' })).toBeVisible();
    await expect(page.locator('button.tab').filter({ hasText: 'Payments' })).toBeVisible();
    await expect(page.locator('button.tab').filter({ hasText: 'Balance' })).toBeVisible();
  });
});
