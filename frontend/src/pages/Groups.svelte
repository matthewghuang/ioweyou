<script>
  import { onMount } from 'svelte';
  import { getAllGroups, getToken } from '../lib/api.js';
  import { currentGroupSlug } from '../lib/stores.js';
  import ShareModal from '../lib/ShareModal.svelte';

  let { onSelectGroup } = $props();

  let groups = $state([]);
  let showShare = $state(false);
  let shareLink = $state('');

  function loadGroups() {
    groups = getAllGroups();
  }

  function selectGroup(slug) {
    const token = getToken(slug);
    if (token) {
      currentGroupSlug.set(slug);
      onSelectGroup(slug);
    }
  }

  function openShare(slug) {
    shareLink = `${window.location.origin}/join/${slug}`;
    showShare = true;
  }

  onMount(loadGroups);
</script>

<div class="page-header">
  <h2 class="page-title">Your Groups</h2>
  <a href="/" class="btn btn-primary">+ New Group</a>
</div>

{#if groups.length === 0}
  <div class="empty-state">
    <p>You're not in any groups yet.</p>
    <p style="margin-top: 0.5rem;"><a href="/" class="btn btn-primary">Create one to get started!</a></p>
  </div>
{:else}
  <div class="group-list">
    {#each groups as group}
      <div class="group-card card">
        <div class="group-card-main" onclick={() => selectGroup(group.slug)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && selectGroup(group.slug)}>
          <div class="group-card-name">{group.name}</div>
          <div class="group-card-meta">
            <span class="badge">Signed in as {group.member_name} (you)</span>
          </div>
        </div>
        <div class="group-card-actions">
          <button class="btn btn-sm share-btn" onclick={() => openShare(group.slug)} title="Share invite link">Share</button>
          <span class="group-card-arrow" onclick={() => selectGroup(group.slug)} role="button" tabindex="0" onkeydown={(e) => e.key === 'Enter' && selectGroup(group.slug)}>&rarr;</span>
        </div>
      </div>
    {/each}
  </div>
{/if}

{#if showShare && shareLink}
  <ShareModal link={shareLink} onClose={() => showShare = false} />
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
    transition: background 0.15s, border-color 0.15s;
    width: 100%;
    border: 1px solid var(--border);
    padding: 0.75rem 1rem;
  }

  .group-card:hover {
    background: var(--bg-hover);
    border-color: var(--text-muted);
  }

  .group-card-main {
    flex: 1;
    min-width: 0;
    cursor: pointer;
    padding: 0;
    background: none;
    border: none;
    text-align: left;
    color: inherit;
    font: inherit;
  }

  .group-card-main:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
    border-radius: 4px;
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

  .group-card-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-shrink: 0;
  }

  .share-btn {
    font-size: 0.8rem;
  }

  .group-card-arrow {
    font-size: 1.2rem;
    color: var(--text-muted);
    cursor: pointer;
    padding: 0.25rem;
  }

  .group-card-arrow:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
    border-radius: 4px;
  }
</style>
