/**
 * Generate a UUID v4 string.
 *
 * `crypto.randomUUID()` only exists in secure contexts (HTTPS or localhost)
 * and in browsers newer than ~2021 (Safari < 15.4, Chrome < 92 lack it even
 * in secure contexts). On plain-HTTP origins it is undefined, so calling it
 * unconditionally throws `crypto.randomUUID is not a function` and aborts
 * form submission. Fall back to `crypto.getRandomValues` (available in every
 * context) and finally to Math.random so ID generation never throws.
 *
 * @returns {string} a UUID v4 string
 */
export function uid() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  if (typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function') {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    // RFC 4122: version 4, variant 10
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }
  // Last resort: no Web Crypto at all (ancient/edge webviews).
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === 'x' ? r : (r & 0x3) | 0x8).toString(16);
  });
}
