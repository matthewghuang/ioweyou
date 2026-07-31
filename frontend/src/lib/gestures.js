// @ts-check

/**
 * Svelte action: swipe-back from left edge.
 *
 * Applies a right-swipe gesture recogniser on the element.
 * When the user swipes right from the left 60px zone beyond `threshold` px,
 * and the horizontal component dominates (|dx| > |dy| * 1.5), the `onBack`
 * callback is invoked after animating the element off-screen.
 *
 * During the gesture the element is translated and a drop-shadow is applied
 * to give visual feedback.
 *
 * @param {HTMLElement} node
 * @param {{ onBack: () => void, threshold?: number }} params
 * @returns {import('svelte/action').ActionReturn}
 */
export function swipeBack(node, { onBack, threshold = 80 }) {
  let startX = 0;
  let startY = 0;
  let currentDx = 0;
  let active = false;

  function onTouchStart(e) {
    const t = e.touches[0];
    if (t.clientX > 60) return;          // only from left edge
    startX = t.clientX;
    startY = t.clientY;
    currentDx = 0;
    active = false;
    node.style.transition = 'none';
    node.style.willChange = 'transform';
  }

  function onTouchMove(e) {
    if (startX === 0) return;
    const t = e.touches[0];
    const dx = t.clientX - startX;
    const dy = t.clientY - startY;

    // Mostly vertical — let pull-to-refresh handle it
    if (Math.abs(dy) > Math.abs(dx) * 1.5 && Math.abs(dx) < 15) return;

    if (dx <= 0) {
      // User dragged left or didn't move right — reset
      if (!active) return;
      currentDx = Math.max(0, dx);
    } else {
      active = true;
      // Eased translation with resistance
      currentDx = Math.min(dx * 0.4, window.innerWidth * 0.35);
    }

    node.style.transform = `translateX(${currentDx}px)`;
    node.style.boxShadow =
      currentDx > 0 ? '2px 0 20px rgba(0,0,0,0.4)' : '';

    e.preventDefault();
  }

  function onTouchEnd() {
    if (!active) {
      startX = 0;
      return;
    }

    node.style.transition =
      'transform 0.3s cubic-bezier(0.25, 0.46, 0.45, 0.94), box-shadow 0.3s ease';

    if (currentDx >= threshold) {
      // Animate full slide then navigate back
      node.style.transform = 'translateX(100vw)';
      setTimeout(() => onBack(), 280);
    } else {
      // Snap back
      node.style.transform = 'translateX(0)';
      node.style.boxShadow = '';
    }

    startX = 0;
    active = false;
    currentDx = 0;
  }

  function onTouchCancel() {
    if (active) {
      node.style.transition =
        'transform 0.25s ease, box-shadow 0.25s ease';
      node.style.transform = 'translateX(0)';
      node.style.boxShadow = '';
    }
    startX = 0;
    active = false;
    currentDx = 0;
  }

  node.addEventListener('touchstart', onTouchStart, { passive: true });
  node.addEventListener('touchmove', onTouchMove, { passive: false });
  node.addEventListener('touchend', onTouchEnd);
  node.addEventListener('touchcancel', onTouchCancel);

  return {
    destroy() {
      node.removeEventListener('touchstart', onTouchStart);
      node.removeEventListener('touchmove', onTouchMove);
      node.removeEventListener('touchend', onTouchEnd);
      node.removeEventListener('touchcancel', onTouchCancel);
    },
  };
}

// ---- Shared state for swipeReveal ----------------------------------------

/** @type {{ node: HTMLElement, content: HTMLElement } | null} */
let revealedCard = null;

function dismissRevealed() {
  if (revealedCard) {
    revealedCard.content.style.transition =
      'transform 0.25s cubic-bezier(0.25, 0.46, 0.45, 0.94)';
    revealedCard.content.style.transform = 'translateX(0)';
    revealedCard = null;
  }
}

// ---------------------------------------------------------------------------
// Inject style blocks once
// ---------------------------------------------------------------------------
if (typeof document !== 'undefined' && !document.getElementById('ptr-styles')) {
  const style = document.createElement('style');
  style.id = 'ptr-styles';
  style.textContent = `
    .ptr-indicator {
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 0.5rem;
      font-size: 0.875rem;
      color: var(--text-secondary, #8b949e);
      pointer-events: none;
      -webkit-user-select: none;
      user-select: none;
    }
    .ptr-spinner {
      display: inline-block;
      width: 1rem;
      height: 1rem;
      border: 2px solid var(--border, #30363d);
      border-top-color: var(--accent, #58a6ff);
      border-radius: 50%;
      opacity: 0;
      transition: opacity 0.2s ease;
    }
    .ptr-spinner.ptr-spinning {
      animation: ptr-spin 0.6s linear infinite;
    }
    @keyframes ptr-spin {
      to { transform: rotate(360deg); }
    }
  `;
  document.head.appendChild(style);
}

if (typeof document !== 'undefined' && !document.getElementById('swipe-reveal-styles')) {
  const s = document.createElement('style');
  s.id = 'swipe-reveal-styles';
  s.textContent = `
    .swipe-reveal-btn {
      position: absolute;
      right: 0;
      top: 0;
      height: 100%;
      min-width: 80px;
      max-width: 120px;
      border: none;
      /* Match the card's radius so no red corner peeks out behind it */
      border-radius: 0 var(--radius, 8px) var(--radius, 8px) 0;
      color: #fff;
      font-size: 0.85rem;
      font-weight: 600;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 0 1rem;
      white-space: nowrap;
    }
  `;
  document.head.appendChild(s);
}

/**
 * Svelte action: pull-to-refresh from top.
 *
 * Detects a pull-down gesture when the scrollable `node` is at `scrollTop === 0`.
 * A visual indicator (spinner + label) is revealed above the content as the user
 * pulls down. Pulling past `threshold` px triggers the `onRefresh` callback
 * (typically a data-reload function). During refresh the indicator remains
 * visible and shows a "Refreshing…" label.
 *
 * The action requires a positioned ancestor (the parent node gets
 * `position: relative; overflow: hidden` applied automatically).
 *
 * @param {HTMLElement} node — the scrollable content element
 * @param {{ onRefresh: () => Promise<void>, threshold?: number }} params
 * @returns {import('svelte/action').ActionReturn}
 */
export function pullToRefresh(node, { onRefresh, threshold = 60 }) {
  let startY = 0;
  let startX = 0;
  let currentDy = 0;
  let pulling = false;
  let refreshing = false;

  // ---- prepare parent ----
  const parent = /** @type {HTMLElement} */ (node.parentNode);
  if (!parent) throw new Error('pullToRefresh needs a parent node');

  const origParentPos = parent.style.position;
  const origParentOverflow = parent.style.overflow;
  parent.style.position = 'relative';
  parent.style.overflow = 'hidden';

  // ---- indicator ----
  const indicator = document.createElement('div');
  indicator.className = 'ptr-indicator';
  indicator.style.position = 'absolute';
  indicator.style.top = '-60px';
  indicator.style.left = '0';
  indicator.style.right = '0';
  indicator.style.height = '60px';
  indicator.style.zIndex = '10';
  indicator.style.transition = 'opacity 0.2s ease';
  indicator.innerHTML =
    '<span class="ptr-spinner"></span><span class="ptr-text">Pull to refresh</span>';

  parent.insertBefore(indicator, node);

  // ---- prepare node ----
  node.style.willChange = 'transform';

  // ---- touch handlers ----
  function onTouchStart(e) {
    if (refreshing) return;
    // Only at the very top of the scroll area
    if (node.scrollTop > 0) return;

    const t = e.touches[0];
    startY = t.clientY;
    startX = t.clientX;
    currentDy = 0;
    pulling = false;
    node.style.transition = 'none';
  }

  function onTouchMove(e) {
    if (refreshing || startY === 0) return;

    const t = e.touches[0];
    const dy = t.clientY - startY;
    const dx = t.clientX - startX;

    // Scrolling up — ignore
    if (dy < 0) return;

    // Mostly horizontal — let swipe-back handle it
    if (Math.abs(dx) > Math.abs(dy) * 1.5 && Math.abs(dx) > 10) return;

    // User may have scrolled during the gesture
    if (node.scrollTop > 0) return;

    pulling = true;

    // Eased resistance
    currentDy = Math.min(dy * 0.4, 160);

    node.style.transform = `translateY(${currentDy}px)`;

    // Update indicator visual state
    const textEl = indicator.querySelector('.ptr-text');
    const spinnerEl = /** @type {HTMLElement} */ (
      indicator.querySelector('.ptr-spinner')
    );
    if (currentDy >= threshold) {
      if (textEl) textEl.textContent = 'Release to refresh';
      if (spinnerEl) spinnerEl.style.opacity = '1';
    } else {
      if (textEl) textEl.textContent = 'Pull to refresh';
      if (spinnerEl) spinnerEl.style.opacity = currentDy > 15 ? '1' : '0.3';
    }

    e.preventDefault();
  }

  async function onTouchEnd() {
    if (!pulling) {
      startY = 0;
      return;
    }

    const textEl = indicator.querySelector('.ptr-text');
    const spinnerEl = /** @type {HTMLElement} */ (
      indicator.querySelector('.ptr-spinner')
    );

    if (currentDy >= threshold && !refreshing) {
      refreshing = true;
      if (spinnerEl) {
        spinnerEl.classList.add('ptr-spinning');
        spinnerEl.style.opacity = '1';
      }
      if (textEl) textEl.textContent = 'Refreshing…';

      // Stay at threshold while refreshing
      node.style.transition = 'transform 0.2s ease';
      node.style.transform = `translateY(${threshold}px)`;

      try {
        await onRefresh();
      } finally {
        refreshing = false;
        node.style.transition =
          'transform 0.35s cubic-bezier(0.25, 0.46, 0.45, 0.94)';
        node.style.transform = 'translateY(0)';
        if (spinnerEl) {
          spinnerEl.classList.remove('ptr-spinning');
          spinnerEl.style.opacity = '0';
        }
        if (textEl) textEl.textContent = 'Pull to refresh';
      }
    } else {
      node.style.transition = 'transform 0.25s ease';
      node.style.transform = 'translateY(0)';
    }

    startY = 0;
    pulling = false;
    currentDy = 0;
  }

  function onTouchCancel() {
    if (pulling && !refreshing) {
      node.style.transition = 'transform 0.25s ease';
      node.style.transform = 'translateY(0)';
    }
    startY = 0;
    pulling = false;
    currentDy = 0;
  }

  node.addEventListener('touchstart', onTouchStart, { passive: true });
  node.addEventListener('touchmove', onTouchMove, { passive: false });
  node.addEventListener('touchend', onTouchEnd);
  node.addEventListener('touchcancel', onTouchCancel);

  return {
    destroy() {
      node.removeEventListener('touchstart', onTouchStart);
      node.removeEventListener('touchmove', onTouchMove);
      node.removeEventListener('touchend', onTouchEnd);
      node.removeEventListener('touchcancel', onTouchCancel);

      if (indicator.parentNode) {
        indicator.parentNode.removeChild(indicator);
      }
      parent.style.position = origParentPos;
      parent.style.overflow = origParentOverflow;
    },
  };
}

// ---- Global click dismiss for swipeReveal ----------------------------------

let _docSwipeAttached = false;

/**
 * Svelte action: swipe left to reveal an action button behind the card.
 *
 * Expects this structure inside `node`:
 * ```html
 * <div class="swipe-container" use:swipeReveal={{ onAction, actionLabel, actionVariant }}>
 *   <div class="swipe-content"><!-- card content --></div>
 *   <div class="swipe-action"><!-- button injected here --></div>
 * </div>
 * ```
 *
 * The `.swipe-content` element slides left to reveal the action button positioned
 * behind it. Only one card may be revealed at a time.
 *
 * @param {HTMLElement} node — the `.swipe-container` element
 * @param {{ onAction: () => void, actionLabel: string, actionVariant?: 'danger' | 'default' }} params
 * @returns {import('svelte/action').ActionReturn}
 */
export function swipeReveal(node, { onAction, actionLabel, actionVariant = 'danger' }) {
  const content = /** @type {HTMLElement} */ (node.querySelector('.swipe-content'));
  const actionEl = /** @type {HTMLElement} */ (node.querySelector('.swipe-action'));
  if (!content || !actionEl) {
    throw new Error('swipeReveal: node must contain .swipe-content and .swipe-action children');
  }

  // ---- Create / reuse action button ----
  let button = /** @type {HTMLElement | null} */ (actionEl.querySelector('.swipe-reveal-btn'));
  if (!button) {
    button = document.createElement('button');
    button.className = 'swipe-reveal-btn';
    actionEl.appendChild(button);
  }

  function renderButton() {
    if (!button) return;
    button.textContent = actionLabel;
    button.style.background =
      actionVariant === 'danger' ? 'var(--danger, #f85149)' : 'var(--accent, #58a6ff)';
  }
  renderButton();

  function getActionWidth() {
    return Math.min(actionEl.offsetWidth || 80, 120);
  }

  // ---- Touch gesture state ----
  let startX = 0;
  let startY = 0;
  let swiping = false;
  let currentOffset = 0;
  const THRESHOLD = 60;

  // ---- Touch handlers ----
  function onTouchStart(e) {
    const t = e.touches[0];
    startX = t.clientX;
    startY = t.clientY;
    swiping = false;
    currentOffset = 0;

    // If this card is already revealed, keep it revealed (user may tap action)
    const alreadyThis = revealedCard && revealedCard.node === node;

    // If another card is revealed, dismiss it
    if (!alreadyThis && revealedCard) {
      dismissRevealed();
    }

    content.style.transition = 'none';
  }

  function onTouchMove(e) {
    if (startX === 0) return;
    const t = e.touches[0];
    const dx = t.clientX - startX;
    const dy = t.clientY - startY;

    // Minimum movement to activate
    if (Math.abs(dx) < 10) return;

    // Mostly vertical — let scroll handle it
    if (Math.abs(dy) > Math.abs(dx) * 1.5) return;

    swiping = true;
    e.preventDefault();

    const aw = getActionWidth();

    // If the card was already revealed, start from -aw
    const base = revealedCard && revealedCard.node === node ? -aw : 0;
    let offset = base + dx * 0.4;

    // Clamp: cannot go right of 0, cannot go left of -aw
    offset = Math.max(-aw, Math.min(0, offset));
    currentOffset = offset;

    content.style.transform = `translateX(${offset}px)`;
  }

  function onTouchEnd() {
    if (!swiping) {
      startX = 0;
      return;
    }

    const aw = getActionWidth();

    content.style.transition =
      'transform 0.25s cubic-bezier(0.25, 0.46, 0.45, 0.94)';

    if (currentOffset < -THRESHOLD) {
      // Snap to fully reveal
      content.style.transform = `translateX(-${aw}px)`;
      revealedCard = { node, content };
    } else {
      // Snap back
      content.style.transform = 'translateX(0)';
      if (revealedCard && revealedCard.node === node) {
        revealedCard = null;
      }
    }

    startX = 0;
    swiping = false;
    currentOffset = 0;
  }

  function onTouchCancel() {
    if (swiping) {
      content.style.transition =
        'transform 0.25s cubic-bezier(0.25, 0.46, 0.45, 0.94)';
      content.style.transform = 'translateX(0)';
      if (revealedCard && revealedCard.node === node) {
        revealedCard = null;
      }
    }
    startX = 0;
    swiping = false;
    currentOffset = 0;
  }

  // ---- Click handling ----
  function onClick(e) {
    if (revealedCard && revealedCard.node === node) {
      const rect = node.getBoundingClientRect();
      const x = e.clientX - rect.left;
      const aw = getActionWidth();

      if (x > rect.width - aw) {
        // Tap on the action area — trigger callback
        onAction();
      }
      // Either way, dismiss the revealed card
      dismissRevealed();
    }
  }

  // ---- Attach events ----
  node.addEventListener('touchstart', onTouchStart, { passive: true });
  node.addEventListener('touchmove', onTouchMove, { passive: false });
  node.addEventListener('touchend', onTouchEnd);
  node.addEventListener('touchcancel', onTouchCancel);
  node.addEventListener('click', onClick);

  // Global click dismiss — attach once
  if (!_docSwipeAttached) {
    _docSwipeAttached = true;
    document.addEventListener('click', (e) => {
      if (revealedCard && !revealedCard.node.contains(/** @type {Node} */ (e.target))) {
        dismissRevealed();
      }
    });
  }

  return {
    update(newParams) {
      onAction = newParams.onAction;
      actionLabel = newParams.actionLabel;
      actionVariant = newParams.actionVariant ?? 'danger';
      renderButton();
    },
    destroy() {
      node.removeEventListener('touchstart', onTouchStart);
      node.removeEventListener('touchmove', onTouchMove);
      node.removeEventListener('touchend', onTouchEnd);
      node.removeEventListener('touchcancel', onTouchCancel);
      node.removeEventListener('click', onClick);

      if (revealedCard && revealedCard.node === node) {
        revealedCard = null;
      }
      if (button && button.parentNode) {
        button.parentNode.removeChild(button);
      }
    },
  };
}

/**
 * Programmatically dismiss any currently revealed swipe action.
 */
export function resetAllSwipes() {
  dismissRevealed();
}
