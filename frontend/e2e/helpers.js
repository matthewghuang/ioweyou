// @ts-check
import { expect } from '@playwright/test';

const BACKEND = process.env.BACKEND_URL || 'http://localhost:8080';

/**
 * @typedef {{ slug: string, cookie_token: string, member_id: string, internal_id: string, group_name: string }} GroupSession
 */

// ─── API helpers ───────────────────────────────────────────────────────────

/**
 * Creates a group via the public API.
 * @param {string} name
 * @param {string} creatorName
 * @param {string} secret
 * @returns {Promise<{ slug: string, cookie_token: string, member_id: string, internal_id: string }>}
 */
async function apiFetch(method, url, body, token) {
  const headers = {};
  if (token) headers['X-Group-Token'] = token;
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  const res = await fetch(`${BACKEND}${url}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

  // Read body once, as text
  const text = await res.text();
  let data;
  try { data = JSON.parse(text); } catch { data = null; }

  if (!res.ok) {
    throw new Error(data?.error || `Request failed (${res.status}): ${text.slice(0, 200)}`);
  }
  return data;
}

export async function apiCreateGroup(name, creatorName, secret) {
  const data = await apiFetch('POST', '/api/groups', { name, creator_name: creatorName, secret });
  // The API returns the slug as 'id' — remap for convenience
  return { ...data, slug: data.id };
}

export async function apiJoinGroup(slug, name, secret) {
  return apiFetch('POST', '/api/groups/join', { slug, name, secret });
}

export async function apiGroupInfo(slug) {
  return apiFetch('GET', `/api/groups/${encodeURIComponent(slug)}/info`);
}

export async function apiCreateExpense(slug, token, expense) {
  return apiFetch('POST', `/api/groups/${encodeURIComponent(slug)}/expenses`, expense, token);
}

export async function apiListExpenses(slug, token) {
  return apiFetch('GET', `/api/groups/${encodeURIComponent(slug)}/expenses`, undefined, token);
}

export async function apiCreatePayment(slug, token, payment) {
  return apiFetch('POST', `/api/groups/${encodeURIComponent(slug)}/payments`, payment, token);
}

export async function apiListPayments(slug, token) {
  return apiFetch('GET', `/api/groups/${encodeURIComponent(slug)}/payments`, undefined, token);
}

export async function apiConfirmPayment(paymentId, token) {
  return apiFetch('POST', `/api/payments/${paymentId}/confirm`, undefined, token);
}

export async function apiGetBalances(slug, token) {
  return apiFetch('GET', `/api/groups/${encodeURIComponent(slug)}/balances`, undefined, token);
}

// ─── Browser state helpers ─────────────────────────────────────────────────

/**
 * Navigates to the app origin so localStorage is accessible, then sets the
 * auth tokens that make the SPA recognise the user as authenticated for a group.
 *
 * Call *before* navigating to the group detail page.
 *
 * @param {import('@playwright/test').Page} page
 * @param {string} slug
 * @param {string} token
 * @param {{ name: string, member_name: string, member_id: string, internal_id: string }} groupInfo
 */
export async function setGroupAuth(page, slug, token, groupInfo) {
  // Must be on the same origin before accessing localStorage
  await page.goto('/');
  try { await page.waitForLoadState('domcontentloaded', { timeout: 5000 }); } catch { /* SPA may error, fine */ }

  await page.evaluate(
    ({ slug, token, groupInfo }) => {
      localStorage.setItem(`ioweyou_token_${slug}`, token);
      localStorage.setItem(`ioweyou_group_${slug}`, JSON.stringify(groupInfo));
    },
    { slug, token, groupInfo },
  );
}

/**
 * Navigates to the app root and waits for the SPA to render.
 * @param {import('@playwright/test').Page} page
 */
export async function goToApp(page) {
  await page.goto('/');
  await page.waitForLoadState('networkidle');
}

/**
 * Navigates to the group detail page in the SPA.
 * @param {import('@playwright/test').Page} page
 * @param {string} slug
 */
export async function goToGroup(page, slug) {
  await page.goto(`/groups/${slug}`);
  await page.waitForLoadState('networkidle');
}

// ─── UI interaction helpers ────────────────────────────────────────────────

/**
 * Fills an input by its label text (wraps the element in the same row).
 * Uses the input's own label, placeholder, or aria-label as fallback.
 * @param {import('@playwright/test').Page} page
 * @param {string} label
 * @param {string} value
 */
export async function fillByLabel(page, label, value) {
  await page.getByLabel(label).fill(value);
}

/**
 * Clicks a button by its text.
 * @param {import('@playwright/test').Page} page
 * @param {string} text
 */
export async function clickButton(page, text) {
  await page.getByRole('button', { name: text }).click();
}

/**
 * Waits for the page to show a specific text (useful after navigation/actions).
 * @param {import('@playwright/test').Page} page
 * @param {string} text
 * @param {{ timeout?: number }} [opts]
 */
export async function waitForText(page, text, opts) {
  await page.getByText(text, { exact: false }).waitFor(opts);
}

/**
 * Expects the page to contain a visible element with the given text.
 * @param {import('@playwright/test').Page} page
 * @param {string|RegExp} text
 */
export async function expectVisible(page, text) {
  await expect(page.getByText(text).first()).toBeVisible();
}
