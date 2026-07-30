<script>
  import { dismissToast } from './toastStore.js';

  let { id, message, type = 'info', duration = 3000, action = null } = $props();

  let hiding = $state(false);

  function handleAction() {
    if (action) {
      action.onClick();
    }
    handleDismiss();
  }

  function handleDismiss() {
    hiding = true;
    setTimeout(() => dismissToast(id), 200);
  }

  $effect(() => {
    if (duration > 0) {
      const timer = setTimeout(() => {
        if (!hiding) handleDismiss();
      }, duration);
      return () => clearTimeout(timer);
    }
  });
</script>

{#if !hiding}
  <div class="toast toast-{type}" role="alert">
    <span class="toast-message">{message}</span>
    {#if action}
      <button class="toast-action" onclick={handleAction}>{action.label}</button>
    {/if}
    <button class="toast-close" onclick={handleDismiss} aria-label="Close">&times;</button>
  </div>
{/if}

<style>
  .toast {
    position: fixed;
    bottom: 1.5rem;
    left: 50%;
    transform: translateX(-50%) translateY(0);
    max-width: 400px;
    width: calc(100% - 2rem);
    background: #1c2333;
    border: 1px solid #30363d;
    border-radius: 8px;
    padding: 0.75rem 1rem;
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    z-index: 200;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
    border-left: 3px solid #8b949e;
    animation: toast-slide-up 0.25s ease-out;
    color: #e6edf3;
    font-size: 0.9rem;
    line-height: 1.4;
  }

  .toast-error {
    border-left-color: #f85149;
  }

  .toast-success {
    border-left-color: #3fb950;
  }

  .toast-info {
    border-left-color: #58a6ff;
  }

  .toast-message {
    flex: 1;
    min-width: 0;
    word-break: break-word;
  }

  .toast-action {
    flex-shrink: 0;
    background: var(--accent-dim, #1f6feb);
    border: none;
    color: #fff;
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    padding: 0.25rem 0.625rem;
    border-radius: var(--radius-sm, 4px);
    min-height: 32px;
    white-space: nowrap;
  }

  .toast-action:hover {
    background: var(--accent, #58a6ff);
  }

  .toast-close {
    flex-shrink: 0;
    background: none;
    border: none;
    color: #8b949e;
    font-size: 1.25rem;
    cursor: pointer;
    line-height: 1;
    padding: 0.25rem;
    min-width: 44px;
    min-height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: -0.25rem;
  }

  .toast-close:hover {
    color: #e6edf3;
  }

  @keyframes toast-slide-up {
    from {
      transform: translateX(-50%) translateY(100%);
      opacity: 0;
    }
    to {
      transform: translateX(-50%) translateY(0);
      opacity: 1;
    }
  }
</style>
