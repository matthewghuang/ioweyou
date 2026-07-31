// @ts-check
import { expect } from '@playwright/test';

const BACKEND = process.env.BACKEND_URL || 'http://localhost:8080';

// ─── API helpers (test setup only) ─────────────────────────────────────────

async function apiFetch(method, url, body, token) {
  const headers = {};
  if (token) headers['X-Group-Token'] = token;
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  const res = await fetch(`${BACKEND}${url}`, {
    method,
    headers,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });

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
  return { ...data, slug: data.id };
}

export async function apiJoinGroup(slug, name, secret) {
  return apiFetch('POST', '/api/groups/join', { slug, name, secret });
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

// ─── Group session fixture ─────────────────────────────────────────────────

/**
 * Create a group (creator "Alice") plus any extra members via the API, and
 * return a session object used to authenticate the browser as any member.
 *
 * @param {string} name
 * @param {string[]} [memberNames] - extra members to join (besides Alice)
 * @param {string} [secret]
 */
export async function createGroup(name, memberNames = [], secret = 'secret') {
  const creator = await apiCreateGroup(name, 'Alice', secret);
  const members = [
    { name: 'Alice', member_id: creator.member_id, cookie_token: creator.cookie_token },
  ];
  for (const memberName of memberNames) {
    const joined = await apiJoinGroup(creator.slug, memberName, secret);
    members.push({ name: memberName, member_id: joined.member_id, cookie_token: joined.cookie_token });
  }

  return {
    slug: creator.slug,
    internal_id: creator.internal_id,
    members,
    /** @returns {{ name: string, member_id: string, cookie_token: string }} */
    member(who) {
      return members.find((m) => m.name === who);
    },
  };
}

/**
 * Authenticate the page as `who` and navigate to the group detail page.
 * Waits (web-first) for the group header to render.
 *
 * @param {import('@playwright/test').Page} page
 * @param {ReturnType<typeof createGroup>} session
 * @param {string} who - member name to authenticate as
 * @param {string} groupName
 */
export async function openGroup(page, session, who, groupName) {
  const member = session.member(who);
  await page.goto('/');
  await page.evaluate(
    ({ slug, token, info }) => {
      localStorage.setItem(`ioweyou_token_${slug}`, token);
      localStorage.setItem(`ioweyou_group_${slug}`, JSON.stringify(info));
    },
    {
      slug: session.slug,
      token: member.cookie_token,
      info: {
        name: groupName,
        member_name: member.name,
        member_id: member.member_id,
        internal_id: session.internal_id,
      },
    },
  );
  await page.goto(`/groups/${session.slug}`);
  await expect(page.getByText(groupName).first()).toBeVisible();
}

/**
 * Click a group detail tab (Expenses / Payments / Balance).
 */
export async function openTab(page, tab) {
  await page.getByRole('button', { name: new RegExp(`^${tab}`) }).click();
}

/**
 * The "not yet synced" badge shown on locally-queued expenses/payments.
 */
export function pendingSyncBadge(page) {
  return page.locator('.badge-warning[title="Not yet synced"]');
}
