/**
 * Svelte action: trigger haptic feedback on pointer down.
 * Uses navigator.vibrate() on Android, or the HapticFeedback API on iOS.
 * Falls back silently on unsupported browsers.
 *
 * Usage: <button use:haptic>Click me</button>
 *
 * @param {HTMLElement} node
 * @param {{ duration?: number, type?: 'light' | 'medium' | 'heavy' }} [params]
 * @returns {import('svelte/action').ActionReturn}
 */
export function haptic(node, { duration = 15, type = 'light' } = {}) {
  function onPointerDown() {
    if (typeof navigator === 'undefined') return;
    
    // iOS Haptic Feedback API (UIKit-style)
    if ('vibrate' in navigator) {
      // Map type to duration
      const ms = type === 'light' ? duration : type === 'medium' ? duration * 2 : duration * 3;
      navigator.vibrate(ms);
    }
    // Future: can add iOS HapticFeedback (WKInterfaceDevice) if needed
  }

  node.addEventListener('pointerdown', onPointerDown);

  return {
    update(newParams) {
      duration = newParams?.duration ?? 15;
      type = newParams?.type ?? 'light';
    },
    destroy() {
      node.removeEventListener('pointerdown', onPointerDown);
    }
  };
}
