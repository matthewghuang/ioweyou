import { writable } from 'svelte/store';

export const toasts = writable([]);

export function showToast(message, type = 'info', duration = 3000, action = null) {
  const id = Date.now() + Math.random();
  toasts.update(t => [...t, { id, message, type, duration, action }]);
  return id;
}

export function dismissToast(id) {
  toasts.update(t => t.filter(x => x.id !== id));
}
