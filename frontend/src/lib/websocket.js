import { getApiKey } from './api.js';
import { currentPage, currentGroupId, currentUser } from './stores.js';

let ws = null;
let reconnectTimer = null;
let subscribedGroupId = null;

let onUpdateCallback = null;

export function setOnUpdate(cb) {
  onUpdateCallback = cb;
}

function getWSURL() {
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const host = window.location.host;
  const key = getApiKey();
  if (!key) return null;
  return `${proto}://${host}/api/ws?token=${encodeURIComponent(key)}`;
}

export function connect() {
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) return;

  const url = getWSURL();
  if (!url) return;

  try {
    ws = new WebSocket(url);
  } catch (e) {
    scheduleReconnect();
    return;
  }

  ws.onopen = () => {
    // Re-subscribe if we were subscribed before
    if (subscribedGroupId) {
      subscribe(subscribedGroupId);
    }
  };

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type === 'change' && msg.operations && onUpdateCallback) {
        onUpdateCallback(msg.operations);
      }
    } catch (e) {
      // ignore malformed messages
    }
  };

  ws.onclose = () => {
    ws = null;
    scheduleReconnect();
  };

  ws.onerror = () => {
    // onclose will fire after this
  };
}

export function disconnect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  subscribedGroupId = null;
  if (ws) {
    ws.onclose = null;
    ws.close();
    ws = null;
  }
}

function scheduleReconnect() {
  if (reconnectTimer) return;
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null;
    connect();
  }, 5000);
}

export function subscribe(groupId) {
  subscribedGroupId = groupId;
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'subscribe', group_id: groupId }));
  }
}

export function unsubscribe() {
  subscribedGroupId = null;
}
