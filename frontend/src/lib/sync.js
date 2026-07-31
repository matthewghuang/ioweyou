import { writable, get } from 'svelte/store';
import { mergeState, snapshot } from './crdt.js';
import {
	addOp,
	addOps,
	getOps,
	clearOps,
	getVersionVector,
	setVersionVector,
	getAllGroupMetas
} from './db.js';
import { api, getToken, getAllGroups } from './api.js';
import { online, pendingOpsCount } from './networkStore.js';

/**
 * Whether a sync is currently in progress for any group.
 * Components use this to show spinner / disable actions.
 */
export const syncInProgress = writable(false);

// ---------------------------------------------------------------------------
// Core sync operations
// ---------------------------------------------------------------------------

/**
 * Flush all locally queued ops for a group to the server, then pull newer ops
 * and merge.  Called on reconnect and periodically.
 *
 * Never throws — errors are caught and logged.
 *
 * @param {string} slug - group slug
 */
export async function syncGroup(slug) {
	const token = getToken(slug);
	if (!token) return;

	syncInProgress.set(true);

	try {
		const localOps = await getOps(slug);
		const versionVector = await getVersionVector(slug);

		// Collect every doc_id we know about before any clearing
		const docIds = new Set(localOps.map((op) => op.doc_id));

		// Only push ops that aren't already covered by the version vector
		const unpushedOps = localOps.filter((op) => !isOpCovered(op, versionVector));

		if (unpushedOps.length > 0) {
			try {
				await api.post('/api/sync/push', { operations: unpushedOps }, token);
				// Clear the local queue — the authoritative set comes from the pull below
				await clearOps(slug);
			} catch (err) {
				if (err.status && err.status >= 400 && err.status < 500) {
					// Server rejection — remove queued ops (they're invalid / duplicates)
					console.error('Push rejected by server:', err);
					await clearOps(slug);
				} else {
					// Network error — ops stay queued for next retry
					console.error('Push failed (network):', err);
					syncInProgress.set(false);
					await updatePendingCount(slug);
					return;
				}
			}
		}

		// Start with the cursors we had before (including any pushed above)
		let cursors = versionVector ? deepClone(versionVector) : {};

		// Pull for each known doc_id
		for (const docId of docIds) {
			try {
				const response = await api.post(
					'/api/sync/pull',
					{ doc_id: docId, cursors },
					token
				);

				if (response.operations && response.operations.length > 0) {
					await addOps(response.operations, slug);
				}

				if (response.cursors) {
					cursors = mergeCursors(cursors, response.cursors);
				}
			} catch (err) {
				// Network or server error — skip this doc, continue with others
				console.error(`Pull failed for "${docId}":`, err);
			}
		}

		// Persist the merged version vector
		if (Object.keys(cursors).length > 0) {
			await setVersionVector(slug, cursors);
		}

		await updatePendingCount(slug);
	} catch (err) {
		console.error('syncGroup error:', err);
	} finally {
		syncInProgress.set(false);
	}
}

/**
 * Sync every group known to the client.  Groups are discovered from both
 * localStorage and IndexedDB metadata.
 */
export async function syncAll() {
	const groups = getAllGroups();
	const seen = new Set();

	for (const g of groups) {
		seen.add(g.slug);
		// eslint-disable-next-line no-await-in-loop
		await syncGroup(g.slug);
	}

	// Also discover groups that only exist in IndexedDB meta
	const metas = await getAllGroupMetas();
	for (const meta of metas) {
		if (!seen.has(meta.group_slug)) {
			seen.add(meta.group_slug);
			// eslint-disable-next-line no-await-in-loop
			await syncGroup(meta.group_slug);
		}
	}
}

// ---------------------------------------------------------------------------
// Op creation
// ---------------------------------------------------------------------------

/**
 * Create a CRDT operation: store locally, attempt immediate push when online,
 * and keep `pendingOpsCount` up to date.
 *
 * @param {object} op   — CRDT operation (must have doc_id, op_type, field,
 *                        value, author_id, timestamp)
 * @param {string} slug — group slug for IndexedDB scoping
 */
export async function createOp(op, slug) {
	// Persist locally first
	await addOp(op, slug);

	// Attempt push when online
	if (get(online)) {
		const token = getToken(slug);
		if (token) {
			try {
				await api.post('/api/sync/push', { operations: [op] }, token);

				// Push succeeded — advance the version vector so subsequent
				// syncs don't re-push this op.
				const vv = (await getVersionVector(slug)) || {};
				const ts = op.timestamp;
				const existing = vv[op.author_id];
				if (
					!existing ||
					ts.wall_time > existing.wall_time ||
					(ts.wall_time === existing.wall_time && ts.logical > existing.logical)
				) {
					vv[op.author_id] = { wall_time: ts.wall_time, logical: ts.logical };
					await setVersionVector(slug, vv);
				}
			} catch (err) {
				if (err.status && err.status >= 400 && err.status < 500) {
					// Server rejected the op — log but don't throw; the op stays
					// in the queue until the next full syncGroup cleans it up.
					console.error('createOp rejected by server:', err);
				} else {
					// Network error — leave queued for later retry
					console.error('createOp push failed (network):', err);
				}
			}
		}
	}

	await updatePendingCount(slug);
}

// ---------------------------------------------------------------------------
// Read helpers
// ---------------------------------------------------------------------------

/**
 * Get the merged document snapshot, optionally including still-uncommitted
 * local operations.
 *
 * @param {string}  docId         — CRDT document ID
 * @param {string}  [groupSlug]   — scope to this group's stored ops
 * @param {object[]} [extraLocalOps] — speculative ops to merge transitively
 * @returns {Promise<object>}
 */
export async function getSnapshot(docId, groupSlug, extraLocalOps) {
	const ops = groupSlug ? await getOps(groupSlug) : [];

	if (docId) {
		const docOps = ops.filter((op) => op.doc_id === docId);
		if (extraLocalOps && extraLocalOps.length > 0) {
			const merged = mergeState(extraLocalOps, docOps);
			return snapshot(merged);
		}
		return snapshot(docOps);
	}

	// No specific docId — snapshot everything available
	if (extraLocalOps && extraLocalOps.length > 0) {
		const merged = mergeState(extraLocalOps, ops);
		return snapshot(merged);
	}

	return snapshot(ops);
}

/**
 * Return all locally-queued (unpushed) operations for a group.
 * An operation is considered unpushed when its timestamp is higher than
 * the version-vector cursor for its author.
 *
 * @param {string} slug
 * @returns {Promise<object[]>}
 */
export async function getLocalOps(slug) {
	const ops = await getOps(slug);
	const vv = await getVersionVector(slug);
	if (!vv) return ops;
	return ops.filter((op) => !isOpCovered(op, vv));
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/**
 * Return `true` when an operation's timestamp is within the range already
 * acknowledged by the version vector.
 * @param {object} op
 * @param {object|null} versionVector
 * @returns {boolean}
 */
function isOpCovered(op, versionVector) {
	if (!versionVector) return false;
	const cursor = versionVector[op.author_id];
	if (!cursor) return false;
	const t = op.timestamp;
	return (
		t.wall_time < cursor.wall_time ||
		(t.wall_time === cursor.wall_time && t.logical <= cursor.logical)
	);
}

/**
 * Merge two cursor maps, keeping the highest timestamp per author.
 * Returns a new object (no mutation).
 * @param {object} a
 * @param {object} b
 * @returns {object}
 */
function mergeCursors(a, b) {
	const result = { ...a };
	for (const [authorId, ts] of Object.entries(b)) {
		const existing = result[authorId];
		if (
			!existing ||
			ts.wall_time > existing.wall_time ||
			(ts.wall_time === existing.wall_time && ts.logical > existing.logical)
		) {
			result[authorId] = { wall_time: ts.wall_time, logical: ts.logical };
		}
	}
	return result;
}

/**
 * Shallow-clone a plain object (version vectors are a single level of nesting).
 * @param {object} obj
 * @returns {object}
 */
function deepClone(obj) {
	return JSON.parse(JSON.stringify(obj));
}

/**
 * Recalculate `pendingOpsCount` from the local queue for a given slug.
 * @param {string} slug
 */
async function updatePendingCount(slug) {
	const ops = await getLocalOps(slug);
	pendingOpsCount.set(ops.length);
}
