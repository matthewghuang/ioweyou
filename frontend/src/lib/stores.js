import { writable } from 'svelte/store';

/** Current page: 'landing' | 'join' (shown under /group/ without token) | 'groups' | 'group' */
export const currentPage = writable('landing');

/** Current group slug for the group detail view */
export const currentGroupSlug = writable(null);
