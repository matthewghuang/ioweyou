// @ts-check
// Regression test: adding an expense or recording a payment must NOT remount
// the page (the full-page "Loading group…" state). The server broadcasts a WS
// change to every subscriber — including the client that made the change — and
// the refresh triggered by that broadcast used to flip `loading`, swapping the
// whole page for a spinner (white flash).
import { test, expect } from '@playwright/test';
import {
  apiCreateExpense,
  createGroup,
  openGroup,
  openTab,
} from '../helpers.js';

/**
 * Installs a MutationObserver in the page that counts how many times the
 * full-page loading state mounts. Must be called AFTER the initial load has
 * finished (i.e. the group name is visible). Catches even a sub-frame flash.
 * @param {import('@playwright/test').Page} page
 */
async function countLoadingMounts(page) {
  await page.evaluate(() => {
    window.__loadingMounts = 0;
    const obs = new MutationObserver((muts) => {
      for (const m of muts) {
        for (const n of m.addedNodes) {
          if (n.nodeType === Node.ELEMENT_NODE && n.textContent.includes('Loading group')) {
            window.__loadingMounts += 1;
          }
        }
      }
    });
    obs.observe(document.body, { childList: true, subtree: true });
    window.__loadingMountsObs = obs;
  });
}

/**
 * Asserts the page never showed the loading state since countLoadingMounts.
 * @param {import('@playwright/test').Page} page
 */
async function expectNoReloadFlash(page) {
  expect(await page.evaluate(() => window.__loadingMounts)).toBe(0);
}

test.describe('Page refresh regression', () => {
  test('adding an expense does not remount the page', async ({ page }) => {
    const groupName = `E2E NoReload Expense ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    await openGroup(page, session, 'Alice', groupName);

    await countLoadingMounts(page);

    await page.getByRole('button', { name: '+ Add Expense', exact: true }).click();
    await page.getByLabel('Description').fill('Dinner');
    await page.getByLabel('Amount').fill('100');
    await page.getByRole('button', { name: 'Add Expense', exact: true }).click();

    await expect(page.getByText('Dinner')).toBeVisible();
    await expect(page.getByText('$100.00').first()).toBeVisible();
    await expectNoReloadFlash(page);
  });

  test('recording a payment does not remount the page', async ({ page }) => {
    const groupName = `E2E NoReload Payment ${Date.now()}`;
    const session = await createGroup(groupName, ['Bob']);
    const alice = session.member('Alice');
    const bob = session.member('Bob');

    // Pre-create an expense so the payment has context
    await apiCreateExpense(session.slug, alice.cookie_token, { description: 'Lunch', amount: 100, split_type: 'equal' });

    // Auth as Bob — the payment "from" is always the current member
    await openGroup(page, session, 'Bob', groupName);
    await openTab(page, 'Payments');
    await expect(page.getByText(/no payments/i)).toBeVisible();

    await countLoadingMounts(page);

    await page.getByRole('button', { name: '+ Record Payment', exact: true }).click();
    await page.getByLabel('To (recipient)').selectOption(alice.member_id);
    await page.getByLabel('Amount').fill('50');
    await page.getByRole('button', { name: 'Record Payment', exact: true }).click();

    await expect(page.getByText('pending', { exact: true })).toBeVisible();
    await expect(page.getByText('$50.00').first()).toBeVisible();
    await expectNoReloadFlash(page);
  });
});
