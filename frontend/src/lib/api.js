const BASE = '';

// Token storage helpers
export function setToken(slug, token) {
  localStorage.setItem('ioweyou_token_' + slug, token);
}

export function getToken(slug) {
  return localStorage.getItem('ioweyou_token_' + slug) || null;
}

export function clearAllTokens() {
  const keys = Object.keys(localStorage);
  for (const key of keys) {
    if (key.startsWith('ioweyou_token_') || key.startsWith('ioweyou_group_')) {
      localStorage.removeItem(key);
    }
  }
}

// Group metadata storage
export function setGroupInfo(slug, info) {
  localStorage.setItem('ioweyou_group_' + slug, JSON.stringify(info));
}

export function getGroupInfo(slug) {
  const raw = localStorage.getItem('ioweyou_group_' + slug);
  if (!raw) return null;
  try { return JSON.parse(raw); } catch { return null; }
}

export function getAllGroups() {
  const keys = Object.keys(localStorage);
  const groups = [];
  for (const key of keys) {
    if (key.startsWith('ioweyou_group_')) {
      const slug = key.slice('ioweyou_group_'.length);
      const info = getGroupInfo(slug);
      if (info) {
        groups.push({ slug, ...info });
      }
    }
  }
  return groups;
}

async function request(method, path, body, token) {
  const headers = {};
  if (token) {
    headers['X-Group-Token'] = token;
  }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }

  let res;
  try {
    res = await fetch(`${BASE}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  } catch (err) {
    throw new Error('Network error — is the backend running?');
  }

  let data;
  try {
    data = await res.json();
  } catch {
    data = null;
  }

  if (!res.ok) {
    const msg = (data && data.error) || `Request failed (${res.status})`;
    const error = new Error(msg);
    error.status = res.status;
    throw error;
  }

  return data;
}

export const api = {
  get: (path, token) => request('GET', path, undefined, token),
  post: (path, body, token) => request('POST', path, body, token),
  patch: (path, body, token) => request('PATCH', path, body, token),
  del: (path, token) => request('DELETE', path, undefined, token),
};
