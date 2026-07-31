<script>
  import { onMount, onDestroy } from 'svelte';
  import { api, setToken, setGroupInfo } from '../lib/api.js';

  let { onJoin } = $props();

  let slug = $state('');
  let name = $state('');
  let secret = $state('');
  let error = $state('');
  let loading = $state(false);
  let groupInfo = $state(null);
  let infoLoading = $state(false);
  let infoError = $state('');

  function initSlug() {
    const path = window.location.pathname;
    const match = path.match(/\/group\/([^\/]+)/);
    if (match) {
      slug = decodeURIComponent(match[1]);
      loadGroupInfo();
    }
  }

  async function loadGroupInfo() {
    if (!slug.trim()) return;
    infoLoading = true;
    infoError = '';
    try {
      groupInfo = await api.get('/api/groups/' + encodeURIComponent(slug.trim()) + '/info');
    } catch {
      groupInfo = null;
      infoError = 'Could not load group info. Check the invite code.';
    } finally {
      infoLoading = false;
    }
  }

  async function handleSubmit(e) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      const data = await api.post('/api/groups/join', {
        slug: slug.trim(),
        name: name.trim(),
        secret,
      });
      // Store token and group info
      setToken(slug.trim(), data.cookie_token);
      setGroupInfo(slug.trim(), {
        name: data.group_name,
        member_name: name.trim(),
        member_id: data.member_id,
        internal_id: data.internal_id,
      });
      onJoin({ slug: slug.trim(), ...data });
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  let slugDebounceTimer = null;

  function onSlugInput() {
    if (slugDebounceTimer) clearTimeout(slugDebounceTimer);
    slugDebounceTimer = setTimeout(loadGroupInfo, 300);
  }

  onMount(() => {
    initSlug();
  });

  onDestroy(() => {
    if (slugDebounceTimer) clearTimeout(slugDebounceTimer);
  });
</script>

<div class="auth-page">
  <div class="auth-card card">
    <h1 class="auth-title">Join Group</h1>
    <p class="auth-subtitle">Choose your name and password to join</p>

    {#if error}
      <div class="alert alert-error">{error}</div>
    {/if}

    <form onsubmit={handleSubmit}>
      <div class="form-group">
        <label class="form-label" for="join-slug">Group invite code / slug</label>
        <input
          id="join-slug"
          class="form-input"
          type="text"
          placeholder="e.g. ski-trip-a3b2"
          bind:value={slug}
          required
          disabled={loading}
          oninput={onSlugInput}
        />
      </div>

      {#if infoError}
        <div class="alert alert-error">{infoError}</div>
      {/if}
      {#if infoLoading}
        <div class="info-loading"><span class="spinner"></span> Loading group info…</div>
      {:else if groupInfo}
        <div class="group-preview card">
          <div class="group-preview-name">{groupInfo.name}</div>
          <div class="group-preview-members">
            {groupInfo.member_count} member{groupInfo.member_count !== 1 ? 's' : ''}
            {#if groupInfo.members && groupInfo.members.length > 0}
              <span class="member-names">
                — {groupInfo.members.map(m => m.name).join(', ')}
              </span>
            {/if}
          </div>
        </div>
      {/if}

      <div class="form-group">
        <label class="form-label" for="join-name">Your name</label>
        <input
          id="join-name"
          class="form-input"
          type="text"
          placeholder="e.g. Bob"
          bind:value={name}
          required
          disabled={loading}
        />
      </div>
      <div class="form-group">
        <label class="form-label" for="join-secret">Password</label>
        <input
          id="join-secret"
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
          <span class="spinner"></span> Joining…
        {:else}
          Join Group
        {/if}
      </button>
    </form>

    <p class="auth-alt">
      Want to start your own?
      <a href="/" class="link-btn">Create a group</a>
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
    padding: 1.5rem;
  }

  @media (min-width: 480px) {
    .auth-card {
      padding: 2rem;
    }
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

  .group-preview {
    padding: 0.75rem;
    margin-bottom: 1rem;
    border: 1px solid var(--border);
  }

  .group-preview-name {
    font-weight: 600;
    font-size: 1rem;
    margin-bottom: 0.25rem;
  }

  .group-preview-members {
    font-size: 0.85rem;
    color: var(--text-secondary);
  }

  .member-names {
    font-size: 0.8rem;
  }

  .info-loading {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 1rem;
    font-size: 0.85rem;
    color: var(--text-secondary);
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
