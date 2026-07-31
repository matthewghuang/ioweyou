<script>
  import { onMount, onDestroy } from 'svelte';
  import { currentPage, currentGroupSlug } from './lib/stores.js';
  import { haptic } from './lib/haptic.js';
  import { clearAllTokens, getAllGroups, getToken } from './lib/api.js';
  import { disconnect, unsubscribe } from './lib/websocket.js';

  import Landing from './pages/Landing.svelte';
  import Join from './pages/Join.svelte';
  import Groups from './pages/Groups.svelte';
  let GroupDetailComponent = $state(null);
  import ToastContainer from './lib/ToastContainer.svelte';
  import OfflineBanner from './lib/OfflineBanner.svelte';

  let loading = $state(true);

  onMount(async () => {
    window.addEventListener('popstate', handlePopState);

    // Check URL path for initial page
    const path = window.location.pathname;

    // Handle direct deep-link to a group — either show group or join form
    const groupMatch = path.match(/^\/group\/(.+)/);
    if (groupMatch) {
      const slug = decodeURIComponent(groupMatch[1]);
      const token = getToken(slug);
      if (token) {
        currentGroupSlug.set(slug);
        currentPage.set('group');
      } else {
        currentPage.set('join');
      }
      loading = false;
      return;
    }
    if (path === '/group') {
      currentPage.set('join');
      loading = false;
      return;
    }

    // Handle direct deep-link to a group (legacy /groups/ path)
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
    } else if (path === '/group') {
      currentPage.set('join');
      currentGroupSlug.set(null);
    } else if (path.startsWith('/group/')) {
      const slug = path.slice('/group/'.length);
      const token = getToken(slug);
      if (token) {
        currentGroupSlug.set(slug);
        currentPage.set('group');
      } else {
        currentPage.set('join');
        currentGroupSlug.set(null);
      }
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

  function handleJoinGroup(data) {
    if (data && data.slug) {
      currentGroupSlug.set(data.slug);
      history.pushState(null, '', `/groups/${data.slug}`);
      currentPage.set('group');
    } else {
      history.pushState(null, '', '/groups');
      currentPage.set('groups');
    }
  }

  function handleSelectGroup(slug) {
    currentGroupSlug.set(slug);
    history.pushState(null, '', `/groups/${slug}`);
    currentPage.set('group');
  }

  $effect(() => {
    if ($currentPage === 'group' && !GroupDetailComponent) {
      import('./pages/GroupDetail.svelte').then(mod => {
        GroupDetailComponent = mod.default;
      });
    }
  });

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
        <button class="header-title-btn" onclick={handleGoHome} use:haptic>I Owe You</button>
        <div class="header-right">
          <button class="btn btn-sm" onclick={handleLogout} use:haptic>Logout</button>
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
      {#if GroupDetailComponent}
        <GroupDetailComponent onBack={handleBackToGroups} />
      {:else}
        <div class="empty-state">
          <span class="spinner"></span> Loading…
        </div>
      {/if}
    {/if}
  </main>
{/if}

<OfflineBanner />
<ToastContainer />

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
    padding: 0.375rem 0;
    min-height: var(--touch-target, 44px);
  }

  .header-title-btn:hover {
    color: var(--accent);
  }

  .header-title-btn:active {
    opacity: 0.7;
  }
</style>
