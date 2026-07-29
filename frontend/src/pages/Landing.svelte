<script>
  import { api, setToken, setGroupInfo } from '../lib/api.js';
  import ShareModal from '../lib/ShareModal.svelte';

  let { onCreate } = $props();

  let groupName = $state('');
  let creatorName = $state('');
  let secret = $state('');
  let error = $state('');
  let loading = $state(false);
  let result = $state(null);
  let showShare = $state(false);

  let inviteLink = $derived(result ? `${window.location.origin}/group/${result.id}` : '');

  async function handleSubmit(e) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      const data = await api.post('/api/groups', {
        name: groupName,
        creator_name: creatorName,
        secret,
      });
      // Store token and group info in localStorage
      setToken(data.id, data.cookie_token);
      setGroupInfo(data.id, {
        name: groupName,
        member_name: creatorName,
        member_id: data.member_id,
        internal_id: data.internal_id,
      });
      result = data;
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

    {#if result}
      <div class="result-section">
        <div class="alert alert-success">Group created!</div>
        <p class="invite-label">Invite members with this link:</p>
        <div class="invite-link-box">
          <code class="invite-link">{inviteLink}</code>
        </div>
        <div class="result-actions">
          <button class="btn btn-primary" onclick={() => showShare = true}>
            Share
          </button>
          <button class="btn" onclick={() => onCreate(result)}>
            Go to Groups
          </button>
        </div>
      </div>
    {:else}
      {#if error}
        <div class="alert alert-error">{error}</div>
      {/if}

      <form onsubmit={handleSubmit}>
        <div class="form-group">
          <label class="form-label" for="grp-name">Group name</label>
          <input
            id="grp-name"
            class="form-input"
            type="text"
            placeholder="e.g. Ski Trip 2025"
            bind:value={groupName}
            required
            disabled={loading}
          />
        </div>
        <div class="form-group">
          <label class="form-label" for="creator-name">Your name</label>
          <input
            id="creator-name"
            class="form-input"
            type="text"
            placeholder="e.g. Alice"
            bind:value={creatorName}
            required
            disabled={loading}
          />
        </div>
        <div class="form-group">
          <label class="form-label" for="secret">Password</label>
          <input
            id="secret"
            class="form-input"
            type="password"
            placeholder="Choose a password"
            bind:value={secret}
            required
            disabled={loading}
          />
        </div>
        <button class="btn btn-primary btn-block" type="submit" disabled={loading}>
          {#if loading}
            <span class="spinner"></span> Creating…
          {:else}
            Create Group
          {/if}
        </button>
      </form>

      <p class="auth-alt">
        Have an invite link?
        <a href="/group" class="link-btn">Join a group</a>
      </p>
    {/if}

    {#if showShare && inviteLink}
      <ShareModal link={inviteLink} onClose={() => showShare = false} />
    {/if}
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

  .result-section {
    text-align: center;
  }

  .invite-label {
    margin-top: 1rem;
    font-weight: 600;
    font-size: 0.9rem;
  }

  .invite-link-box {
    background: var(--bg-hover);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.75rem;
    margin: 0.5rem 0;
    overflow-wrap: break-word;
  }

  .invite-link {
    font-size: 0.85rem;
    word-break: break-all;
  }

  .result-actions {
    display: flex;
    gap: 0.5rem;
    justify-content: center;
    margin-top: 1rem;
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
