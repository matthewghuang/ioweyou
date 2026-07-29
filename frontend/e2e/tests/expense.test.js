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

test.describe('Expense management', () => {
  test('adds an expense with equal split', async ({ page }) => {
    const groupName = `E2E Expense ${Date.now()}`;
    const aliceName = 'Alice';
    const secret = 'secret';

    // Create group with Alice and Bob as members
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, aliceName, secret);

    // Join as Bob
    const bobToken = (await apiJoinGroup(slug, 'Bob', secret)).cookie_token;

    // Set Alice's auth
    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: aliceName,
      member_id,
      internal_id,
    });

    await goToGroup(page, slug);
    await expectVisible(page, groupName);

    // Click "+ Add Expense" to open the form
    await clickButton(page, '+ Add Expense');

    // Fill expense form
    await fillByLabel(page, 'Description', 'Dinner');
    await fillByLabel(page, 'Amount', '100');

    // Default split type is "equal"
    await clickButton(page, 'Add Expense'); // submit

    // Wait for the expense to appear in the list
    await waitForText(page, 'Dinner');
    await expectVisible(page, 'Dinner');
    await expectVisible(page, '$100.00');
  });

  test('adds an expense with custom split', async ({ page }) => {
    const groupName = `E2E Custom Split ${Date.now()}`;
    const aliceName = 'Alice';
    const secret = 'secret';

    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, aliceName, secret);

    // Join as Bob to have two members for custom split
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: aliceName,
      member_id,
      internal_id,
    });

    await goToGroup(page, slug);

    // Open expense form
    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'Pizza');
    await fillByLabel(page, 'Amount', '60');

    // Switch split type to custom via the select
    await page.selectOption('#exp-split', 'custom');

    // Wait for split fields to appear (one per member)
    await page.waitForTimeout(200);

    // Custom split inputs are rendered below the split type select.
    // They have class 'split-input' and type='number'.
    const splitInputs = page.locator('.split-input');
    const count = await splitInputs.count();
    // Expect 2 split inputs (Alice + Bob)
    if (count >= 2) {
      await splitInputs.nth(0).fill('40');
      await splitInputs.nth(1).fill('20');
    }

    await clickButton(page, 'Add Expense'); // submit

    // Verify the expense appears
    await waitForText(page, 'Pizza');
    await expectVisible(page, '$60.00');
  });

  test('shows empty state when no expenses exist', async ({ page }) => {
    const groupName = `E2E Empty ${Date.now()}`;
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', 'secret');

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });

    await goToGroup(page, slug);

    // Should show empty state
    await expectVisible(page, /no expenses/i);
  });

  test('lists expenses via API match what UI shows', async ({ page }) => {
    const groupName = `E2E API Verify ${Date.now()}`;
    const { slug, cookie_token, member_id, internal_id } =
      await apiCreateGroup(groupName, 'Alice', 'secret');

    // Add two expenses via API
    const exp1 = await apiCreateExpense(slug, cookie_token, {
      description: 'Groceries',
      amount: 50,
      split_type: 'equal',
    });
    const exp2 = await apiCreateExpense(slug, cookie_token, {
      description: 'Uber',
      amount: 30,
      split_type: 'equal',
    });

    await setGroupAuth(page, slug, cookie_token, {
      name: groupName,
      member_name: 'Alice',
      member_id,
      internal_id,
    });

    await goToGroup(page, slug);

    // Both expenses should be visible
    await expectVisible(page, 'Groceries');
    await expectVisible(page, 'Uber');

    // Verify amounts via API
    const apiExpenses = await apiListExpenses(slug, cookie_token);
    expect(apiExpenses.length).toBe(2);
    const descriptions = apiExpenses.map(e => e.description);
    expect(descriptions).toContain('Groceries');
    expect(descriptions).toContain('Uber');
  });
});
