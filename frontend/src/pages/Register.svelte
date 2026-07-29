<script>
  import { api } from '../lib/api.js';

  let { onRegister, onLoginClick } = $props();

  let name = $state('');
  let error = $state('');
  let loading = $state(false);

  async function handleSubmit(e) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      const data = await api.post('/api/auth/register', { name });
      onRegister({ id: data.id, name: data.name, api_key: data.api_key });
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }
</script>

<div class="auth-page">
  <div class="auth-card card">
    <h1 class="auth-title">I Owe You</h1>
    <p class="auth-subtitle">Group expense tracking</p>

    <div class="auth-tabs">
      <button class="auth-tab active">Register</button>
      <button class="auth-tab" onclick={onLoginClick}>Login</button>
    </div>

    {#if error}
      <div class="alert alert-error">{error}</div>
    {/if}

    <form onsubmit={handleSubmit}>
      <div class="form-group">
        <label class="form-label" for="reg-name">Your name</label>
        <input
          id="reg-name"
          class="form-input"
          type="text"
          placeholder="e.g. Alice"
          bind:value={name}
          required
          disabled={loading}
        />
      </div>
      <button class="btn btn-primary btn-block" type="submit" disabled={loading}>
        {#if loading}
          <span class="spinner"></span> Registering…
        {:else}
          Register
        {/if}
      </button>
    </form>

    <p class="auth-alt">
      Already have an API key?
      <button class="link-btn" onclick={onLoginClick}>Login instead</button>
    </p>
  </div>
</div>

<style>
  .auth-page {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    padding: 1rem;
  }

  .auth-card {
    width: 100%;
    max-width: 400px;
    padding: 2rem;
  }

  .auth-title {
    font-size: 1.5rem;
    font-weight: 700;
    text-align: center;
    margin-bottom: 0.25rem;
  }

  .auth-subtitle {
    text-align: center;
    color: var(--text-secondary);
    margin-bottom: 1.5rem;
    font-size: 0.9rem;
  }

  .auth-tabs {
    display: flex;
    gap: 0;
    margin-bottom: 1.25rem;
    border-bottom: 1px solid var(--border);
  }

  .auth-tab {
    flex: 1;
    padding: 0.5rem 1rem;
    border: none;
    background: transparent;
    color: var(--text-secondary);
    cursor: pointer;
    font-size: 0.875rem;
    font-weight: 500;
    border-bottom: 2px solid transparent;
    margin-bottom: -1px;
    transition: color 0.15s, border-color 0.15s;
  }

  .auth-tab:hover {
    color: var(--text-primary);
  }

  .auth-tab.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
  }

  .btn-block {
    width: 100%;
  }

  .auth-alt {
    text-align: center;
    margin-top: 1rem;
    font-size: 0.85rem;
    color: var(--text-secondary);
  }

  .link-btn {
    background: none;
    border: none;
    color: var(--accent);
    cursor: pointer;
    font-size: inherit;
    padding: 0;
    text-decoration: underline;
  }

  .link-btn:hover {
    color: var(--accent-hover);
  }
</style>
