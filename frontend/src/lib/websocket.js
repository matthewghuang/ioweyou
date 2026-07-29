import { getToken } from './api.js';

let ws = null;
let reconnectTimer = null;
let subscribedGroupSlug = null;
let pendingSubscribe = null;

let onUpdateCallback = null;

export function setOnUpdate(cb) {
  onUpdateCallback = cb;
}

function getWSURL() {
  const slug = subscribedGroupSlug;
  if (!slug) return null;
  const token = getToken(slug);
  if (!token) return null;
  const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const host = window.location.host;
  return `${proto}://${host}/api/ws?token=${encodeURIComponent(token)}`;
}

function doSendSubscribe() {
  const id = pendingSubscribe;
  if (!id) return;
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: 'subscribe', group_id: id }));
    pendingSubscribe = null;
  }
}

export function connect(slug) {
  // Close existing connection if switching groups
  if (ws && subscribedGroupSlug && subscribedGroupSlug !== slug) {
    ws.close();
    ws = null;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
  }

  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CONNECTING)) {
    subscribedGroupSlug = slug;
    return;
  }

  subscribedGroupSlug = slug;
  // Clear any pending reconnect timer before creating fresh connection
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  const url = getWSURL();
  if (!url) return;

  try {
    ws = new WebSocket(url);
  } catch (e) {
    scheduleReconnect();
    return;
  }

  ws.onopen = () => {
    console.log('[WS] connected, subscribing...');
    // Send any pending subscribe message
    doSendSubscribe();
  };

  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      console.log('[WS] message:', msg.type, msg.operations?.length || 0, 'ops');
      if (msg.type === 'change' && msg.operations && onUpdateCallback) {
        onUpdateCallback(msg.operations);
      }
    } catch (e) {
      // ignore malformed messages
    }
  };

  ws.onclose = (e) => {
    console.log('[WS] closed:', e.code, e.reason);
    // Only react to close events for the current ws instance.
    // Ignore stale events from old connections after group switch.
    if (ws && ws === e.currentTarget) {
      ws = null;
      scheduleReconnect();
    }
  };

  ws.onerror = (e) => {
    console.error('[WS] error:', e);
    // onclose will fire after this
  };
}

export function disconnect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer);
    reconnectTimer = null;
  }
  subscribedGroupSlug = null;
  pendingSubscribe = null;
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
    if (subscribedGroupSlug) {
      connect(subscribedGroupSlug);
    }
  }, 5000);
}

export function subscribe(groupId) {
  pendingSubscribe = groupId;
  // Try to send now; if not connected, onopen will pick it up
  doSendSubscribe();
}

export function unsubscribe() {
  disconnect();
}
