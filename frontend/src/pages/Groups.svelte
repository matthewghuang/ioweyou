<script>
  import { onMount } from 'svelte';
  import { api } from '../lib/api.js';

  let { onSelectGroup } = $props();

  let groups = $state([]);
  let loading = $state(true);
  let error = $state('');

  let showCreate = $state(false);
  let newGroupName = $state('');
  let creating = $state(false);
  let createError = $state('');

  async function loadGroups() {
    error = '';
    loading = true;
    try {
      groups = await api.get('/api/groups');
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  async function handleCreate(e) {
    e.preventDefault();
    createError = '';
    creating = true;
    try {
      await api.post('/api/groups', { name: newGroupName });
      newGroupName = '';
      showCreate = false;
      await loadGroups();
    } catch (e) {
      createError = e.message;
    } finally {
      creating = false;
    }
  }

  function formatDate(ts) {
    if (!ts) return '';
    const ns = Number(ts);
    const d = new Date(Math.floor(ns / 1e6));
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
  }

  onMount(loadGroups);
</script>

<div class="page-header">
  <h2 class="page-title">Your Groups</h2>
  <button class="btn btn-primary" onclick={() => showCreate = !showCreate}>
    {showCreate ? 'Cancel' : '+ New Group'}
  </button>
</div>

{#if showCreate}
  <div class="form-section">
    <div class="form-section-title">Create Group</div>
    {#if createError}
      <div class="alert alert-error">{createError}</div>
    {/if}
    <form onsubmit={handleCreate}>
      <div class="form-group">
        <label class="form-label" for="grp-name">Group name</label>
        <input
          id="grp-name"
          class="form-input"
          type="text"
          placeholder="e.g. Ski Trip 2025"
          bind:value={newGroupName}
          required
          disabled={creating}
        />
      </div>
      <button class="btn btn-primary" type="submit" disabled={creating}>
        {creating ? 'Creating…' : 'Create'}
      </button>
    </form>
  </div>
{/if}

{#if loading}
  <div class="empty-state"><span class="spinner"></span> Loading groups…</div>
{:else if error}
  <div class="alert alert-error">{error}</div>
  <button class="btn" onclick={loadGroups}>Retry</button>
{:else if groups.length === 0}
  <div class="empty-state">
    <p>You're not in any groups yet.</p>
    <p style="margin-top: 0.5rem;">Create one to get started!</p>
  </div>
{:else}
  <div class="group-list">
    {#each groups as group}
      <button class="group-card card" onclick={() => onSelectGroup(group.id)}>
        <div class="group-card-main">
          <div class="group-card-name">{group.name}</div>
          <div class="group-card-meta">
            <span class="badge">{group.members?.length || 0} member{(group.members?.length || 0) !== 1 ? 's' : ''}</span>
            {#if group.created_at}
              <span class="group-card-date">Created {formatDate(group.created_at)}</span>
            {/if}
          </div>
        </div>
        <div class="group-card-arrow">&rarr;</div>
      </button>
    {/each}
  </div>
{/if}

<style>
  .page-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1.25rem;
  }

  .page-title {
    font-size: 1.25rem;
    font-weight: 700;
  }

  .group-list {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .group-card {
    display: flex;
    align-items: center;
    justify-content: space-between;
    cursor: pointer;
    transition: background 0.15s, border-color 0.15s;
    text-align: left;
    width: 100%;
    border: 1px solid var(--border);
  }

  .group-card:hover {
    background: var(--bg-hover);
    border-color: var(--text-muted);
  }

  .group-card-main {
    flex: 1;
    min-width: 0;
  }

  .group-card-name {
    font-weight: 600;
    font-size: 1rem;
    margin-bottom: 0.25rem;
  }

  .group-card-meta {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    font-size: 0.8rem;
    color: var(--text-secondary);
  }

  .group-card-date {
    font-size: 0.8rem;
  }

  .group-card-arrow {
    font-size: 1.2rem;
    color: var(--text-muted);
    margin-left: 0.75rem;
  }
</style>
