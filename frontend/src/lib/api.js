const BASE = '';

let _apiKey = null;

export function setApiKey(key) {
  _apiKey = key;
  if (key) {
    localStorage.setItem('ioweyou_api_key', key);
  } else {
    localStorage.removeItem('ioweyou_api_key');
  }
}

export function getApiKey() {
  if (_apiKey === null) {
    _apiKey = localStorage.getItem('ioweyou_api_key') || null;
  }
  return _apiKey;
}

export function clearApiKey() {
  _apiKey = null;
  localStorage.removeItem('ioweyou_api_key');
}

async function request(method, path, body) {
  const headers = {};
  const key = getApiKey();
  if (key) {
    headers['Authorization'] = `Bearer ${key}`;
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
  get: (path) => request('GET', path),
  post: (path, body) => request('POST', path, body),
  patch: (path, body) => request('PATCH', path, body),
  del: (path) => request('DELETE', path),
};
