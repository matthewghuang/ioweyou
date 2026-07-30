let _db = null;

/**
 * Open (or create) the IndexedDB database.
 * Caches the instance internally — subsequent calls return the same db.
 * @returns {Promise<IDBDatabase>}
 */
export function openDb() {
  if (_db) return Promise.resolve(_db);
  return new Promise((resolve, reject) => {
    const request = indexedDB.open('ioweyou', 1);
    request.onupgradeneeded = (event) => {
      const db = event.target.result;
      if (!db.objectStoreNames.contains('ops')) {
        db.createObjectStore('ops', {
          keyPath: ['doc_id', 'author_id', 'wall_time', 'logical']
        });
      }
      if (!db.objectStoreNames.contains('version_vectors')) {
        db.createObjectStore('version_vectors', { keyPath: 'group_slug' });
      }
      if (!db.objectStoreNames.contains('group_meta')) {
        db.createObjectStore('group_meta', { keyPath: 'group_slug' });
      }
    };
    request.onsuccess = (event) => {
      _db = event.target.result;
      resolve(_db);
    };
    request.onerror = () => reject(request.error);
  });
}

/**
 * Store a single CRDT operation. Idempotent — uses put with the compound key.
 * @param {object} op
 * @returns {Promise<void>}
 */
export async function addOp(op) {
  const db = await openDb();
  const tx = db.transaction('ops', 'readwrite');
  tx.objectStore('ops').put(op);
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

/**
 * Store multiple CRDT operations in a single transaction.
 * @param {object[]} ops
 * @returns {Promise<void>}
 */
export async function addOps(ops) {
  if (ops.length === 0) return;
  const db = await openDb();
  const tx = db.transaction('ops', 'readwrite');
  const store = tx.objectStore('ops');
  for (const op of ops) {
    store.put(op);
  }
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

/**
 * Get all stored operations. Optionally filter by group slug prefix match on doc_id.
 * @param {string} [groupSlug]
 * @returns {Promise<object[]>}
 */
export async function getOps(groupSlug) {
  const db = await openDb();
  const tx = db.transaction('ops', 'readonly');
  const store = tx.objectStore('ops');
  let request;
  if (groupSlug) {
    request = store.getAll(
      IDBKeyRange.bound([groupSlug], [groupSlug + '\uffff'])
    );
  } else {
    request = store.getAll();
  }
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}

/**
 * Delete all operations whose doc_id starts with the given group slug.
 * @param {string} groupSlug
 * @returns {Promise<void>}
 */
export async function clearOps(groupSlug) {
  const db = await openDb();
  const tx = db.transaction('ops', 'readwrite');
  const store = tx.objectStore('ops');
  const range = IDBKeyRange.bound([groupSlug], [groupSlug + '\uffff']);
  return new Promise((resolve, reject) => {
    const cursorReq = store.openCursor(range);
    cursorReq.onsuccess = (event) => {
      const cursor = event.target.result;
      if (cursor) {
        cursor.delete();
        cursor.continue();
      }
    };
    cursorReq.onerror = () => reject(cursorReq.error);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

/**
 * Get the version vector (cursors) for a group.
 * @param {string} groupSlug
 * @returns {Promise<object|null>}
 */
export async function getVersionVector(groupSlug) {
  const db = await openDb();
  const tx = db.transaction('version_vectors', 'readonly');
  const store = tx.objectStore('version_vectors');
  const request = store.get(groupSlug);
  return new Promise((resolve, reject) => {
    request.onsuccess = () => {
      const result = request.result;
      resolve(result ? result.cursors : null);
    };
    request.onerror = () => reject(request.error);
  });
}

/**
 * Set the version vector (cursors) for a group.
 * @param {string} groupSlug
 * @param {object} cursors
 * @returns {Promise<void>}
 */
export async function setVersionVector(groupSlug, cursors) {
  const db = await openDb();
  const tx = db.transaction('version_vectors', 'readwrite');
  tx.objectStore('version_vectors').put({ group_slug: groupSlug, cursors });
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

/**
 * Get cached group metadata.
 * @param {string} groupSlug
 * @returns {Promise<object|null>}
 */
export async function getGroupMeta(groupSlug) {
  const db = await openDb();
  const tx = db.transaction('group_meta', 'readonly');
  const store = tx.objectStore('group_meta');
  const request = store.get(groupSlug);
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result || null);
    request.onerror = () => reject(request.error);
  });
}

/**
 * Store group metadata (merged with the group_slug key).
 * @param {string} groupSlug
 * @param {object} meta
 * @returns {Promise<void>}
 */
export async function setGroupMeta(groupSlug, meta) {
  const db = await openDb();
  const tx = db.transaction('group_meta', 'readwrite');
  tx.objectStore('group_meta').put({ group_slug: groupSlug, ...meta });
  return new Promise((resolve, reject) => {
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
    tx.onabort = () => reject(tx.error);
  });
}

/**
 * Get all cached group metadata entries.
 * @returns {Promise<object[]>}
 */
export async function getAllGroupMetas() {
  const db = await openDb();
  const tx = db.transaction('group_meta', 'readonly');
  const store = tx.objectStore('group_meta');
  const request = store.getAll();
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error);
  });
}
