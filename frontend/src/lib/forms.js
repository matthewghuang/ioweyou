/**
 * Svelte action: ensure the input is scrolled into view when focused,
 * important for iOS Safari where the keyboard can obscure the input.
 *
 * Usage: <input use:scrollIntoViewOnFocus />
 */
export function scrollIntoViewOnFocus(node) {
  function onFocus() {
    // Small delay to let the keyboard begin its animation
    setTimeout(() => {
      node.scrollIntoView({ behavior: 'smooth', block: 'center' });
    }, 300);
  }

  node.addEventListener('focus', onFocus);

  return {
    destroy() {
      node.removeEventListener('focus', onFocus);
    }
  };
}
