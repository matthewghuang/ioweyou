<script>
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';

  let { link, onClose } = $props();

  let qrDataUrl = $state('');
  let copied = $state(false);
  let qrError = $state(false);

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(link);
      copied = true;
      setTimeout(() => copied = false, 2000);
    } catch {
      const input = document.createElement('input');
      input.value = link;
      document.body.appendChild(input);
      input.select();
      document.execCommand('copy');
      document.body.removeChild(input);
      copied = true;
      setTimeout(() => copied = false, 2000);
    }
  }

  function handleBackdrop(e) {
    if (e.target === e.currentTarget) onClose();
  }

  function handleKeydown(e) {
    if (e.key === 'Escape') onClose();
  }

  function handleBackdropKeydown(e) {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onClose();
    }
  }

  onMount(async () => {
    try {
      qrDataUrl = await QRCode.toDataURL(link, { width: 256, margin: 2, color: { dark: '#1a1a1a', light: '#ffffff' } });
    } catch {
      qrError = true;
    }
  });
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="backdrop" onclick={handleBackdrop} onkeydown={handleBackdropKeydown} role="dialog" aria-modal="true" tabindex="-1">
    <div class="modal">
      <button class="close-btn" onclick={onClose} aria-label="Close">&times;</button>
      <h2 class="modal-title">Share Group</h2>
      <p class="modal-subtitle">Scan the QR code or share the link to invite members</p>

      <div class="qr-container">
      {#if qrError}
        <p class="qr-error">Could not generate QR code</p>
      {:else if qrDataUrl}
        <img src={qrDataUrl} alt="QR Code for invite link" class="qr-image" />
      {:else}
        <div class="spinner"></div>
      {/if}
      </div>

      <div class="link-box">
        <code class="link-text">{link}</code>
      </div>

      <div class="modal-actions">
        <button class="btn btn-primary" onclick={copyLink}>
          {copied ? 'Copied!' : 'Copy Link'}
        </button>
        <button class="btn" onclick={onClose}>Close</button>
      </div>
    </div>
  </div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
    padding: 1rem;
  }

  .modal {
    background: var(--bg-card, #1e1e1e);
    border-radius: 12px;
    padding: 1.5rem;
    max-width: 380px;
    width: 100%;
    position: relative;
    text-align: center;
    margin: 1rem;
  }

  .close-btn {
    position: absolute;
    top: 0.75rem;
    right: 0.75rem;
    background: none;
    border: none;
    font-size: 1.5rem;
    color: var(--text-secondary, #999);
    cursor: pointer;
    line-height: 1;
    padding: 0.25rem;
  }

  .close-btn:hover {
    color: var(--text-primary, #fff);
  }

  .modal-title {
    font-size: 1.25rem;
    font-weight: 700;
    margin-bottom: 0.25rem;
  }

  .modal-subtitle {
    font-size: 0.85rem;
    color: var(--text-secondary, #999);
    margin-bottom: 1.25rem;
  }

  .qr-container {
    display: flex;
    justify-content: center;
    margin-bottom: 1.25rem;
  }

  .qr-error {
    color: var(--text-secondary, #999);
    font-size: 0.9rem;
  }

  .spinner {
    width: 40px;
    height: 40px;
    border: 4px solid var(--border, #333);
    border-top-color: var(--accent, #646cff);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .qr-image {
    width: 200px;
    height: 200px;
    border-radius: 8px;
    border: 2px solid var(--border, #333);
  }

  .link-box {
    background: var(--bg-hover, #2a2a2a);
    border: 1px solid var(--border, #333);
    border-radius: 6px;
    padding: 0.75rem;
    margin-bottom: 1.25rem;
    overflow-wrap: break-word;
  }

  .link-text {
    font-size: 0.8rem;
    word-break: break-all;
    color: var(--text-primary, #fff);
  }

  .modal-actions {
    display: flex;
    gap: 0.5rem;
    justify-content: center;
  }

  .modal-actions .btn {
    flex: 1;
    max-width: 160px;
  }
</style>
