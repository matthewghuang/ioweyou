<script>
  import { haptic } from './haptic.js';

  let {
    show = false,
    title = '',
    message = '',
    confirmText = 'Confirm',
    cancelText = 'Cancel',
    variant = 'default',
    onConfirm,
    onCancel,
  } = $props();

  function handleBackdrop(e) {
    if (e.target === e.currentTarget) {
      onCancel?.();
    }
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') {
      onCancel?.();
    }
  }
</script>

<svelte:window onkeydown={handleKeydown} />

{#if show}
  <div
    class="backdrop"
    onclick={handleBackdrop}
    onkeydown={handleKeydown}
    role="dialog"
    aria-modal="true"
    tabindex="-1"
  >
    <div class="sheet">
      <h2 class="sheet-title">{title}</h2>
      {#if message}
        <p class="sheet-message">{message}</p>
      {/if}
      <div class="sheet-actions">
        <button class="btn btn-sheet btn-sheet-cancel" onclick={onCancel} use:haptic>
          {cancelText}
        </button>
        <button
          class="btn btn-sheet"
          class:btn-sheet-danger={variant === 'danger'}
          class:btn-sheet-primary={variant === 'default'}
          onclick={onConfirm}
          use:haptic
        >
          {confirmText}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: flex-end;
    justify-content: center;
    z-index: 150;
  }

  .sheet {
    background: var(--bg-card, #1c2333);
    border-radius: 12px 12px 0 0;
    padding: 1.5rem;
    max-width: 480px;
    width: 100%;
    animation: sheet-slide-up 0.25s ease-out;
  }

  .sheet-title {
    font-size: 1.1rem;
    font-weight: 700;
    margin-bottom: 0.5rem;
    color: var(--text-primary, #e6edf3);
  }

  .sheet-message {
    font-size: 0.9rem;
    color: var(--text-secondary, #8b949e);
    margin-bottom: 1.25rem;
    line-height: 1.5;
  }

  .sheet-actions {
    display: flex;
    gap: 0.75rem;
  }

  .btn-sheet {
    flex: 1;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.9rem;
    font-weight: 600;
    border: none;
    border-radius: 8px;
    cursor: pointer;
    padding: 0.5rem 1rem;
  }

  .btn-sheet-cancel {
    background: var(--bg-hover, #21262d);
    color: var(--text-primary, #e6edf3);
  }

  .btn-sheet-cancel:hover {
    background: var(--border, #30363d);
  }

  .btn-sheet-primary {
    background: var(--accent, #58a6ff);
    color: #fff;
  }

  .btn-sheet-primary:hover {
    background: var(--accent-hover, #79c0ff);
  }

  .btn-sheet-danger {
    background: var(--danger, #f85149);
    color: #fff;
  }

  .btn-sheet-danger:hover {
    background: var(--danger-hover, #ff6b63);
  }

  @keyframes sheet-slide-up {
    from {
      transform: translateY(100%);
    }
    to {
      transform: translateY(0);
    }
  }
</style>
