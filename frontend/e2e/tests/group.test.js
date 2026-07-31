// @ts-check
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  createGroup,
  openGroup,
} from '../helpers.js';

test.describe('Group creation and joining', () => {
  test('creates a group from the landing page and sees it in the list', async ({ page }) => {
    const groupName = `E2E Test Group ${Date.now()}`;

    // Fill in the creation form
    await page.goto('/');
    await page.getByLabel('Group name').fill(groupName);
    await page.getByLabel('Your name').fill('Alice');
    await page.getByLabel('Password').fill('test-secret-123');
    await page.getByRole('button', { name: 'Create Group', exact: true }).click();

    // Success state with the invite link
    await expect(page.getByText('Group created!')).toBeVisible();
    await page.getByRole('button', { name: 'Go to Groups', exact: true }).click();

    // Groups list shows the new group
    await expect(page.getByText(groupName).first()).toBeVisible();
    await page.getByText(groupName).first().click();

    // Group detail page with tabs
    await page.waitForURL(/\/groups\//);
    await expect(page.getByRole('button', { name: /^Expenses/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /^Payments/ })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Balance', exact: true })).toBeVisible();
  });

  test('joins an existing group via invite link', async ({ page }) => {
    const groupName = `E2E Join Test ${Date.now()}`;
    const { slug } = await apiCreateGroup(groupName, 'Alice', 'shared-secret');

    // Fresh visitor (no stored auth) opens the invite link
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.goto(`/group/${slug}`);

    await expect(page.getByRole('heading', { name: 'Join Group' })).toBeVisible();
    await expect(page.getByText(groupName)).toBeVisible();

    await page.getByLabel('Your name').fill('Bob');
    await page.getByLabel('Password').fill('shared-secret');
    await page.getByRole('button', { name: 'Join Group', exact: true }).click();

    // Redirected to the groups list with the group visible
    await expect(page.getByText(groupName).first()).toBeVisible();
  });

  test('requires existing member password to re-join', async ({ page }) => {
    const groupName = `E2E Rejoin ${Date.now()}`;
    const { slug } = await apiCreateGroup(groupName, 'Alice', 'alice-secret');
    await apiJoinGroup(slug, 'Bob', 'bob-password');

    // Fresh visitor tries Bob's name with a wrong password
    await page.goto('/');
    await page.evaluate(() => localStorage.clear());
    await page.goto(`/group/${slug}`);
    await expect(page.getByText(groupName)).toBeVisible();

    await page.getByLabel('Your name').fill('Bob');
    await page.getByLabel('Password').fill('wrong-password');
    await page.getByRole('button', { name: 'Join Group', exact: true }).click();
    await expect(page.getByText(/invalid secret/i)).toBeVisible();

    // Correct password succeeds
    await page.getByLabel('Your name').fill('Bob');
    await page.getByLabel('Password').fill('bob-password');
    await page.getByRole('button', { name: 'Join Group', exact: true }).click();
    await expect(page.getByText(groupName).first()).toBeVisible();
  });

  test('shows group detail page with correct info', async ({ page }) => {
    const groupName = `E2E Info Test ${Date.now()}`;
    const session = await createGroup(groupName);
    await openGroup(page, session, 'Alice', groupName);

    await expect(page.getByRole('button', { name: /^Expenses/ })).toBeVisible();
    await expect(page.getByRole('button', { name: /^Payments/ })).toBeVisible();
    await expect(page.getByRole('button', { name: 'Balance', exact: true })).toBeVisible();
  });
});
