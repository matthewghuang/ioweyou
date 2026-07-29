<script>
  import { onMount, onDestroy } from 'svelte';
  import { currentPage, currentUser, currentGroupId } from './lib/stores.js';
  import { api, getApiKey, setApiKey, clearApiKey } from './lib/api.js';
  import { connect, disconnect } from './lib/websocket.js';

  import Register from './pages/Register.svelte';
  import Login from './pages/Login.svelte';
  import Groups from './pages/Groups.svelte';
  import GroupDetail from './pages/GroupDetail.svelte';

  let loading = $state(true);

  onDestroy(() => {
    disconnect();
  });

  onMount(async () => {
    const key = getApiKey();
    if (key) {
      try {
        await api.get('/api/groups');
        const stored = localStorage.getItem('ioweyou_user');
        if (stored) {
          currentUser.set(JSON.parse(stored));
        }
        connect();
        currentPage.set('groups');
      } catch {
        clearApiKey();
        localStorage.removeItem('ioweyou_user');
        currentPage.set('login');
      }
    } else {
      currentPage.set('login');
    }
    loading = false;
  });

  function handleLogout() {
    disconnect();
    clearApiKey();
    localStorage.removeItem('ioweyou_user');
    currentUser.set(null);
    currentGroupId.set(null);
    currentPage.set('login');
  }

  function handleRegister(user) {
    setApiKey(user.api_key);
    localStorage.setItem('ioweyou_user', JSON.stringify({ id: user.id, name: user.name }));
    currentUser.set({ id: user.id, name: user.name });
    connect();
    currentPage.set('groups');
  }

  function handleLogin(user) {
    setApiKey(user.api_key);
    localStorage.setItem('ioweyou_user', JSON.stringify({ id: user.id, name: user.name }));
    currentUser.set({ id: user.id, name: user.name });
    connect();
    currentPage.set('groups');
  }

  function handleSelectGroup(id) {
    currentGroupId.set(id);
    currentPage.set('group');
  }

  function handleBackToGroups() {
    import('./lib/websocket.js').then(m => m.unsubscribe());
    currentPage.set('groups');
  }

  function handleRegisterClick() {
    currentPage.set('register');
  }

  function handleLoginClick() {
    currentPage.set('login');
  }
</script>

{#if loading}
  <div class="loading-screen">
    <div class="spinner"></div>
  </div>
{:else}
  {#if $currentPage !== 'login' && $currentPage !== 'register'}
    <header class="header">
      <div class="container header-inner">
        <div class="header-title">I Owe You</div>
        <div class="header-right">
          {#if $currentUser}
            <span class="header-user">{$currentUser.name}</span>
          {/if}
          <button class="btn btn-sm" onclick={handleLogout}>Logout</button>
        </div>
      </div>
    </header>
  {/if}
  <main class="container">
    {#if $currentPage === 'register'}
      <Register onRegister={handleRegister} onLoginClick={handleLoginClick} />
    {:else if $currentPage === 'login'}
      <Login onLogin={handleLogin} onRegisterClick={handleRegisterClick} />
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
</style>
