import { writable, derived } from 'svelte/store';

/** Whether the browser is currently online */
export const online = writable(
  typeof navigator !== 'undefined' ? navigator.onLine : true
);

/** How many sync operations are pending (set by sync service) */
export const pendingOpsCount = writable(0);

/** Convenience: true when offline */
export const offline = derived(online, ($online) => !$online);

if (typeof window !== 'undefined') {
  window.addEventListener('online', () => online.set(true));
  window.addEventListener('offline', () => online.set(false));
}
