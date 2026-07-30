/**
 * CRDT — pure JavaScript port of the Go CRDT package.
 *
 * Implements Hybrid Logical Clocks (HLC), LWW-register merge, and RGA
 * list merge with speculative-delete semantics.
 *
 * No external dependencies.
 *
 * @module crdt
 */

// ---------------------------------------------------------------------------
// OpType constants
// ---------------------------------------------------------------------------

/** @type {"lww"} */
const OpLWW = "lww";
/** @type {"rga_insert"} */
const OpRGAInsert = "rga_insert";
/** @type {"rga_delete"} */
const OpRGADelete = "rga_delete";

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/**
 * Return the deduplication key for an operation.
 * @param {object} op
 * @returns {string}
 */
function opKey(op) {
	return `${op.doc_id}|${op.author_id}|${op.timestamp.wall_time}|${op.timestamp.logical}`;
}

/**
 * Try to parse a JSON string; return as-is on failure or if not a string.
 * @param {*} v
 * @returns {*}
 */
function parseValue(v) {
	if (typeof v === "string") {
		try {
			return JSON.parse(v);
		} catch {
			return v;
		}
	}
	return v;
}

// ---------------------------------------------------------------------------
// Comparison functions
// ---------------------------------------------------------------------------

/**
 * Compare two operations by (wallTime, logical, authorID).
 * Returns negative, 0, or positive.
 *
 * @param {object} a
 * @param {object} b
 * @returns {number}
 */
function compareOpTimestamp(a, b) {
	const at = a.timestamp;
	const bt = b.timestamp;

	if (at.wall_time !== bt.wall_time) {
		return at.wall_time - bt.wall_time;
	}
	if (at.logical !== bt.logical) {
		return at.logical - bt.logical;
	}
	if (a.author_id < b.author_id) return -1;
	if (a.author_id > b.author_id) return 1;
	return 0;
}

/**
 * Compare two operations by (wallTime, logical, authorID, itemID).
 * Returns negative, 0, or positive.
 *
 * @param {object} a
 * @param {object} b
 * @returns {number}
 */
function compareOpTimestampItem(a, b) {
	const c = compareOpTimestamp(a, b);
	if (c !== 0) return c;
	if (a.item_id < b.item_id) return -1;
	if (a.item_id > b.item_id) return 1;
	return 0;
}

// ---------------------------------------------------------------------------
// Hybrid Logical Clock
// ---------------------------------------------------------------------------

/**
 * A Hybrid Logical Clock.
 *
 * The wall_time is stored in nanoseconds (Date.now() * 1e6) to match the Go
 * implementation.  Timestamps use snake_case field names (wall_time, logical)
 * to match the JSON serialization format of Operation objects.
 */
class HLC {
	constructor() {
		/** @type {number} */
		this.wall_time = Date.now() * 1e6;
		/** @type {number} */
		this.logical = 0;
	}

	/**
	 * Return the current timestamp and advance the clock.
	 * @returns {{ wall_time: number, logical: number }}
	 */
	now() {
		const wt = Date.now() * 1e6;
		if (wt > this.wall_time) {
			this.wall_time = wt;
			this.logical = 0;
		}
		this.logical++;
		return { wall_time: this.wall_time, logical: this.logical };
	}

	/**
	 * Merge a received timestamp into this clock to maintain causal ordering
	 * across nodes.
	 * @param {{ wall_time: number, logical: number }} ts
	 * @returns {void}
	 */
	observe(ts) {
		if (ts.wall_time > this.wall_time) {
			this.wall_time = ts.wall_time;
			if (ts.logical > this.logical) {
				this.logical = ts.logical;
			}
			this.logical++;
		} else if (ts.wall_time === this.wall_time) {
			if (ts.logical > this.logical) {
				this.logical = ts.logical;
			}
			this.logical++;
		} else {
			// ts.wall_time < this.wall_time
			this.logical++;
		}
	}
}

// ---------------------------------------------------------------------------
// RGA internal helpers
// ---------------------------------------------------------------------------

/**
 * Return the set of item_ids that have a delete op whose timestamp is
 * greater than at least one corresponding insert op.
 *
 * isLocalOp marks operations that originated from the local slice. A delete
 * from the local node is treated as speculative: it only applies to inserts
 * that were also present in the local slice (the deleting node knew about
 * them). A delete from remote applies to all inserts.
 *
 * @param {object[]} inserts  RGA insert operations
 * @param {object[]} deletes  RGA delete operations
 * @param {object<string,boolean>} isLocalOp  opKey → true if originated locally
 * @returns {object<string,boolean>}  itemID → true if deleted
 */
function buildDeleteSet(inserts, deletes, isLocalOp) {
	const deleted = {};
	for (const ins of inserts) {
		for (const del of deletes) {
			if (del.item_id !== ins.item_id) continue;
			if (compareOpTimestamp(del, ins) <= 0) continue;
			// Speculative delete: local delete only removes local inserts.
			if (isLocalOp[opKey(del)] && !isLocalOp[opKey(ins)]) continue;
			deleted[ins.item_id] = true;
			break;
		}
	}
	return deleted;
}

/**
 * Return the set of item_ids that have a delete op whose timestamp is
 * strictly greater than at least one corresponding insert op among the
 * given operations. No local/remote distinction — used by snapshot.
 *
 * @param {object[]} ops
 * @returns {object<string,boolean>}
 */
function buildDeleteSetFromOps(ops) {
	const inserts = [];
	const deletes = [];
	for (const op of ops) {
		if (op.op_type === OpRGAInsert) inserts.push(op);
		else if (op.op_type === OpRGADelete) deletes.push(op);
	}
	const deleted = {};
	for (const ins of inserts) {
		for (const del of deletes) {
			if (del.item_id !== ins.item_id) continue;
			if (compareOpTimestamp(del, ins) <= 0) continue;
			deleted[ins.item_id] = true;
			break;
		}
	}
	return deleted;
}

/**
 * Order surviving inserts by their prev_item_id references.
 * Inserts are assumed already sorted by (wallTime, logical, authorID, itemID).
 * When prev_item_id is not found (deleted or never existed) the item is
 * appended at the end of the list.
 *
 * @param {object[]} inserts  presorted RGA insert operations
 * @returns {object[]}        ordered insert operations
 */
function orderRGAList(inserts) {
	/** @type {{ op: object, itemID: string }[]} */
	const list = [];

	for (const ins of inserts) {
		let pos = -1;
		if (ins.prev_item_id) {
			for (let i = 0; i < list.length; i++) {
				if (list[i].itemID === ins.prev_item_id) {
					pos = i;
					break;
				}
			}
		}

		const entry = { op: ins, itemID: ins.item_id };
		if (pos === -1) {
			// Not found or empty prev — append at end.
			list.push(entry);
		} else {
			// Find the correct position: items after prev that have a LOWER
			// timestamp than this insert should come first (inserts arrive
			// in timestamp order).
			let insPos = pos + 1;
			while (insPos < list.length && compareOpTimestamp(list[insPos].op, ins) < 0) {
				insPos++;
			}
			// Insert at insPos.
			list.splice(insPos, 0, entry);
		}
	}

	return list.map((e) => e.op);
}

// ---------------------------------------------------------------------------
// MergeState
// ---------------------------------------------------------------------------

/**
 * Merge local and remote operation slices, deduplicate by
 * (docID, authorID, wallTime, logical), resolve LWW field conflicts by
 * keeping the highest-timestamp operation per (doc_id, field), and
 * reconstruct RGA lists by ordering surviving inserts and removing deleted
 * items.
 *
 * Returns the canonical merged operation set (same semantics as server).
 *
 * @param {object[]} local   operations from local (offline-queued) source
 * @param {object[]} remote  operations from remote (server-pulled) source
 * @returns {object[]}
 */
function mergeState(local, remote) {
	// ---- deduplicate and track origins ----
	const seen = new Set();
	/** @type {object<string, number>}  origin 1=local 2=remote */
	const opOrigin = {};
	const deduped = [];

	function add(op, origin) {
		const k = `${op.doc_id}|${op.author_id}|${op.timestamp.wall_time}|${op.timestamp.logical}`;
		if (seen.has(k)) return;
		seen.add(k);
		opOrigin[opKey(op)] = origin;
		deduped.push(op);
	}

	if (local) {
		for (const op of local) {
			add(op, 1); // originLocal
		}
	}
	if (remote) {
		for (const op of remote) {
			add(op, 2); // originRemote
		}
	}

	// ---- split by op type ----
	/** @type {object<string, object[]>}  "docID|field" → LWW ops */
	const lwwByField = {};
	/** @type {object[]} */
	const rgaInserts = [];
	/** @type {object[]} */
	const rgaDeletes = [];
	/** @type {object<string,boolean>} */
	const isLocalOp = {};

	for (const op of deduped) {
		switch (op.op_type) {
			case OpLWW: {
				const df = `${op.doc_id}|${op.field}`;
				if (!lwwByField[df]) lwwByField[df] = [];
				lwwByField[df].push(op);
				break;
			}
			case OpRGAInsert: {
				rgaInserts.push(op);
				if (opOrigin[opKey(op)] === 1) {
					isLocalOp[opKey(op)] = true;
				}
				break;
			}
			case OpRGADelete: {
				rgaDeletes.push(op);
				if (opOrigin[opKey(op)] === 1) {
					isLocalOp[opKey(op)] = true;
				}
				break;
			}
		}
	}

	// ---- LWW: keep winner per (doc_id, field) ----
	const result = [];

	for (const df in lwwByField) {
		const ops = lwwByField[df];
		let winner = ops[0];
		for (let i = 1; i < ops.length; i++) {
			if (compareOpTimestamp(ops[i], winner) > 0) {
				winner = ops[i];
			}
		}
		result.push(winner);
	}

	// ---- RGA: resolve deletions and order surviving inserts ----
	/** @type {object<string, object[]>}  "docID|field" → inserts */
	const rgaInsertByField = {};
	for (const ins of rgaInserts) {
		const k = `${ins.doc_id}|${ins.field}`;
		if (!rgaInsertByField[k]) rgaInsertByField[k] = [];
		rgaInsertByField[k].push(ins);
	}

	const deletedSet = buildDeleteSet(rgaInserts, rgaDeletes, isLocalOp);

	for (const k in rgaInsertByField) {
		const inserts = rgaInsertByField[k];

		// Sort inserts by (wallTime, logical, authorID, itemID).
		inserts.sort(compareOpTimestampItem);

		// Filter out deleted items.
		const alive = inserts.filter((ins) => !deletedSet[ins.item_id]);

		// Build list respecting prev_item_id.
		const ordered = orderRGAList(alive);

		// Each surviving insert in order is a result operation.
		result.push(...ordered);
	}

	return result;
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

/**
 * Snapshot a single document's operations into a flat field → value map.
 *
 * @param {object[]} ops  operations for a single document
 * @returns {object}
 */
function snapshotDoc(ops) {
	const out = {};

	// Collect LWW ops per field, pick winner.
	/** @type {object<string, object[]>} */
	const lwwByField = {};
	/** @type {object<string, object[]>} */
	const rgaByField = {};

	for (const op of ops) {
		switch (op.op_type) {
			case OpLWW: {
				if (!lwwByField[op.field]) lwwByField[op.field] = [];
				lwwByField[op.field].push(op);
				break;
			}
			case OpRGAInsert: {
				if (!rgaByField[op.field]) rgaByField[op.field] = [];
				rgaByField[op.field].push(op);
				break;
			}
		}
	}

	// Build a delete set from all RGA deletes in the input (no speculative
	// semantics — every delete whose timestamp beats its insert applies).
	const deletedSet = buildDeleteSetFromOps(ops);

	// LWW fields.
	for (const field in lwwByField) {
		const fops = lwwByField[field];
		let winner = fops[0];
		for (let i = 1; i < fops.length; i++) {
			if (compareOpTimestamp(fops[i], winner) > 0) {
				winner = fops[i];
			}
		}
		out[field] = parseValue(winner.value);
	}

	// RGA fields.
	for (const field in rgaByField) {
		const inserts = rgaByField[field];

		// Filter out deleted items.
		const alive = [];
		for (const ins of inserts) {
			if (!deletedSet[ins.item_id]) {
				alive.push(ins);
			}
		}

		// Sort by (wallTime, logical, authorID, itemID).
		alive.sort(compareOpTimestampItem);

		// Order by prev_item_id.
		const ordered = orderRGAList(alive);

		const arr = ordered.map((ins) => parseValue(ins.value));
		if (arr.length > 0) {
			out[field] = arr;
		}
	}

	return out;
}

/**
 * Project a merged (canonical) operation set into a plain object.
 *
 * LWW fields become scalar values (the winner by timestamp).
 * RGA lists become ordered arrays of items.
 *
 * If all ops share a single docID, top-level keys are field names.
 * Otherwise the result is keyed by docID with nested field maps.
 *
 * @param {object[]} ops
 * @returns {object}
 */
function snapshot(ops) {
	// Group by docID.
	const byDoc = {};
	for (const op of ops) {
		if (!byDoc[op.doc_id]) byDoc[op.doc_id] = [];
		byDoc[op.doc_id].push(op);
	}

	const docIDs = Object.keys(byDoc);

	if (docIDs.length === 1) {
		return snapshotDoc(byDoc[docIDs[0]]);
	}

	const out = {};
	for (const docID of docIDs) {
		out[docID] = snapshotDoc(byDoc[docID]);
	}
	return out;
}

// ---------------------------------------------------------------------------
// Exports
// ---------------------------------------------------------------------------

export {
	HLC,
	compareOpTimestamp,
	compareOpTimestampItem,
	mergeState,
	snapshot,
	OpLWW,
	OpRGAInsert,
	OpRGADelete,
};
