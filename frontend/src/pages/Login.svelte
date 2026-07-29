<script>
  import { api, setApiKey, clearApiKey } from '../lib/api.js';

  let { onLogin, onRegisterClick } = $props();

  let apiKey = $state('');
  let error = $state('');
  let loading = $state(false);

  async function handleSubmit(e) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      setApiKey(apiKey.trim());
      // Verify the key and get user info
      const user = await api.get('/api/auth/whoami');
      // Key is valid — mark as logged in with real user data
      onLogin({ id: user.id, name: user.name, api_key: apiKey.trim() });
    } catch (e) {
      error = e.message || 'Invalid API key';
      clearApiKey();
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
      <button class="auth-tab" onclick={onRegisterClick}>Register</button>
      <button class="auth-tab active">Login</button>
    </div>

    {#if error}
      <div class="alert alert-error">{error}</div>
    {/if}

    <form onsubmit={handleSubmit}>
      <div class="form-group">
        <label class="form-label" for="login-key">API Key</label>
        <textarea
          id="login-key"
          class="form-input"
          placeholder="Paste your API key here"
          bind:value={apiKey}
          rows="2"
          required
          disabled={loading}
        ></textarea>
      </div>
      <button class="btn btn-primary btn-block" type="submit" disabled={loading}>
        {#if loading}
          <span class="spinner"></span> Verifying…
        {:else}
          Login
        {/if}
      </button>
    </form>

    <p class="auth-alt">
      Don't have one?
      <button class="link-btn" onclick={onRegisterClick}>Register instead</button>
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
