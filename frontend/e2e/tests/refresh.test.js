// @ts-check
// Regression test: adding an expense or recording a payment must NOT remount
// the page (the full-page "Loading group…" state). The server broadcasts a WS
// change to every subscriber — including the client that made the change — and
// the refresh triggered by that broadcast used to flip `loading`, swapping the
// whole page for a spinner (white flash).
import { test, expect } from '@playwright/test';
import {
  apiCreateGroup,
  apiJoinGroup,
  apiCreateExpense,
  setGroupAuth,
  goToGroup,
  clickButton,
  fillByLabel,
  waitForText,
  expectVisible,
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
    await goToGroup(page, slug);
    await expectVisible(page, groupName);

    await countLoadingMounts(page);

    await clickButton(page, '+ Add Expense');
    await fillByLabel(page, 'Description', 'Dinner');
    await fillByLabel(page, 'Amount', '100');
    await clickButton(page, 'Add Expense');

    await waitForText(page, 'Dinner');
    await expectVisible(page, '$100.00');
    await expectNoReloadFlash(page);
  });

  test('recording a payment does not remount the page', async ({ page }) => {
    const groupName = `E2E NoReload Payment ${Date.now()}`;
    const secret = 'secret';

    const { slug, cookie_token: aliceToken, member_id: aliceId, internal_id } =
      await apiCreateGroup(groupName, 'Alice', secret);
    const bob = await apiJoinGroup(slug, 'Bob', secret);

    // Pre-create an expense so the payment has context
    await apiCreateExpense(slug, aliceToken, {
      description: 'Lunch',
      amount: 100,
      split_type: 'equal',
    });

    // Auth as Bob — the payment "from" is always the current member
    await setGroupAuth(page, slug, bob.cookie_token, {
      name: groupName,
      member_name: 'Bob',
      member_id: bob.member_id,
      internal_id,
    });
    await goToGroup(page, slug);
    await expectVisible(page, groupName);

    await page.locator('button.tab').filter({ hasText: 'Payments' }).click();
    await waitForText(page, /no payments/i);

    await countLoadingMounts(page);

    await clickButton(page, '+ Record Payment');
    await page.selectOption('#pay-to', aliceId);
    await fillByLabel(page, 'Amount', '50');
    await clickButton(page, 'Record Payment');

    await waitForText(page, /pending/i);
    await expectVisible(page, '$50.00');
    await expectNoReloadFlash(page);
  });
});
