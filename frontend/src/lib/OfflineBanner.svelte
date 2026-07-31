<script>
  import { online, pendingOpsCount } from './networkStore.js';
  import { offline } from './networkStore.js';
</script>

{#if $offline}
  <div class="banner banner--offline" role="status" aria-live="polite">
    <svg class="banner-icon" width="16" height="16" viewBox="0 0 16 16" fill="none">
      <path d="M1.5 1.5l13 13" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
      <path d="M3.5 6.5a5.5 5.5 0 017-5.3" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
      <path d="M5.5 4.5a3.5 3.5 0 015.5 2" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
      <path d="M11.5 9.5A5.5 5.5 0 016 13" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
      <circle cx="8" cy="11.5" r="1" fill="currentColor"/>
    </svg>
    <span>
      You're offline{@html $pendingOpsCount > 0 ? ` &mdash; ${$pendingOpsCount} change${$pendingOpsCount === 1 ? '' : 's'} will sync when connected` : ''}
    </span>
  </div>
{:else if $pendingOpsCount > 0}
  <div class="banner banner--syncing" role="status" aria-live="polite">
    <span class="banner-spinner"></span>
    <span>Syncing {$pendingOpsCount} change{$pendingOpsCount === 1 ? '' : 's'}&hellip;</span>
  </div>
{/if}

<style>
  .banner {
    position: fixed;
    top: var(--header-height, 52px);
    left: 0;
    right: 0;
    z-index: 40;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 0.5rem;
    min-height: var(--touch-target, 44px);
    padding: 0.5rem 1rem;
    font-size: 0.85rem;
    line-height: 1.4;
    text-align: center;
    animation: slideDown 0.3s ease-out;
    box-sizing: border-box;
    padding-top: calc(0.5rem + var(--safe-top, 0px));
  }

  .banner--offline {
    background: rgba(210, 153, 34, 0.12);
    border-bottom: 1px solid rgba(210, 153, 34, 0.3);
    color: var(--warning, #d29922);
  }

  .banner--syncing {
    background: rgba(88, 166, 255, 0.1);
    border-bottom: 1px solid rgba(88, 166, 255, 0.3);
    color: var(--accent, #58a6ff);
  }

  .banner-icon {
    flex-shrink: 0;
  }

  .banner-spinner {
    display: inline-block;
    width: 14px;
    height: 14px;
    border: 2px solid transparent;
    border-top-color: currentColor;
    border-radius: 50%;
    flex-shrink: 0;
    animation: spin 0.6s linear infinite;
  }

  @keyframes slideDown {
    from {
      transform: translateY(-100%);
      opacity: 0;
    }
    to {
      transform: translateY(0);
      opacity: 1;
    }
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
