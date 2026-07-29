import { writable } from 'svelte/store';

/** Current page: 'register' | 'login' | 'groups' | 'group' */
export const currentPage = writable('login');

/** ID of the currently viewed group */
export const currentGroupId = writable(null);

/** Current authenticated user { id, name } or null */
export const currentUser = writable(null);
