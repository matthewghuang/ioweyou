<script>
  import { onMount, onDestroy } from 'svelte';
  import { currentPage, currentGroupSlug } from './lib/stores.js';
  import { clearAllTokens, getAllGroups, getToken } from './lib/api.js';
  import { disconnect, unsubscribe } from './lib/websocket.js';

  import Landing from './pages/Landing.svelte';
  import Join from './pages/Join.svelte';
  import Groups from './pages/Groups.svelte';
  import GroupDetail from './pages/GroupDetail.svelte';

  let loading = $state(true);

  onMount(async () => {
    window.addEventListener('popstate', handlePopState);

    // Check URL path for initial page
    const path = window.location.pathname;
    const joinMatch = path.match(/^\/join\/(.+)/);
    if (joinMatch) {
      currentPage.set('join');
      loading = false;
      return;
    }
    if (path === '/join') {
      currentPage.set('join');
      loading = false;
      return;
    }

    // Handle direct deep-link to a group
    const groupsMatch = path.match(/^\/groups\/(.+)/);
    if (groupsMatch) {
      const slug = decodeURIComponent(groupsMatch[1]);
      const token = getToken(slug);
      if (token) {
        currentGroupSlug.set(slug);
        currentPage.set('group');
        loading = false;
        return;
      }
    }

    // Check localStorage for saved groups
    const groups = getAllGroups();
    if (groups.length > 0) {
      currentPage.set('groups');
    } else {
      currentPage.set('landing');
    }
    loading = false;
  });

  onDestroy(() => {
    disconnect();
    window.removeEventListener('popstate', handlePopState);
  });

  // Handle browser back/forward
  function handlePopState() {
    const path = window.location.pathname;
    if (path === '/groups') {
      currentPage.set('groups');
      currentGroupSlug.set(null);
    } else if (path.startsWith('/groups/')) {
      const slug = path.slice('/groups/'.length);
      currentGroupSlug.set(slug);
      currentPage.set('group');
    } else if (path === '/join' || path.startsWith('/join/')) {
      currentPage.set('join');
      currentGroupSlug.set(null);
    } else {
      currentPage.set('landing');
      currentGroupSlug.set(null);
    }
  }

  function handleLogout() {
    disconnect();
    clearAllTokens();
    currentGroupSlug.set(null);
    history.pushState(null, '', '/');
    currentPage.set('landing');
  }

  function handleCreateGroup() {
    history.pushState(null, '', '/groups');
    currentPage.set('groups');
  }

  function handleJoinGroup() {
    history.pushState(null, '', '/groups');
    currentPage.set('groups');
  }

  function handleSelectGroup(slug) {
    currentGroupSlug.set(slug);
    history.pushState(null, '', `/groups/${slug}`);
    currentPage.set('group');
  }

  function handleBackToGroups() {
    unsubscribe();
    currentGroupSlug.set(null);
    history.pushState(null, '', '/groups');
    currentPage.set('groups');
  }

  function handleGoHome() {
    history.pushState(null, '', '/');
    currentPage.set('landing');
  }
</script>

{#if loading}
  <div class="loading-screen">
    <div class="spinner"></div>
  </div>
{:else}
  {#if $currentPage !== 'landing' && $currentPage !== 'join'}
    <header class="header">
      <div class="container header-inner">
        <button class="header-title-btn" onclick={handleGoHome}>I Owe You</button>
        <div class="header-right">
          <button class="btn btn-sm" onclick={handleLogout}>Logout</button>
        </div>
      </div>
    </header>
  {/if}
  <main class="container">
    {#if $currentPage === 'landing'}
      <Landing onCreate={handleCreateGroup} />
    {:else if $currentPage === 'join'}
      <Join onJoin={handleJoinGroup} />
    {:else if $currentPage === 'groups'}
      <Groups onSelectGroup={handleSelectGroup} />
    {:else if $currentPage === 'group'}
      <GroupDetail onBack={handleBackToGroups} />
    {/if}
  </main>
{/if}

<style>
  .loading-screen {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
  }

  .header-right {
    display: flex;
    align-items: center;
    gap: 0.75rem;
  }

  .header-title-btn {
    background: none;
    border: none;
    color: var(--text-primary);
    font-size: 1.1rem;
    font-weight: 700;
    cursor: pointer;
    padding: 0;
  }

  .header-title-btn:hover {
    color: var(--accent);
  }
</style>
