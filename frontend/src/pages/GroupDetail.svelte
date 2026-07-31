<script>
  import { currentGroupSlug } from '../lib/stores.js';
  import { api, getToken, getGroupInfo } from '../lib/api.js';
  import { connect, subscribe, setOnUpdate, disconnect as wsDisconnect } from '../lib/websocket.js';
  import ShareModal from '../lib/ShareModal.svelte';

  let { onBack } = $props();

  let slug = $derived($currentGroupSlug);
  let group = $state(null);
  let members = $state([]);
  let expenses = $state([]);
  let payments = $state([]);
  let balances = $state({ members: [], payments: [], settlements: [] });
  let loading = $state(true);
  let error = $state('');
  let activeTab = $state('expenses');
  let showShare = $state(false);
  let inviteLink = $derived(slug ? `${window.location.origin}/group/${slug}` : '');

  // Expense form
  let showExpForm = $state(false);
  let expDesc = $state('');
  let expAmt = $state(0);
  let expSplitType = $state('equal');
  let expCustomSplits = $state([]);
let expCustomSplitsSum = $derived(expCustomSplits.reduce((s, x) => s + Number(x.amount || 0), 0));
  let expCreating = $state(false);
  let expError = $state('');

  // Payment form
  let showPayForm = $state(false);
  let payFrom = $state('');
  let payTo = $state('');
  let payAmt = $state(0);
  let payMethod = $state('');
  let payCreating = $state(false);
  let payError = $state('');

  let currentMemberId = $state(null);
  let loadGen = 0;

  // ---- Helpers ----

  function truncId(id) {
    return id ? id.slice(0, 8) + '…' : '?';
  }

  function formatDate(ts) {
    if (!ts) return '';
    const d = new Date(Number(ts));
    return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) +
      ' ' + d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  }

  function fmt(n) {
    return Number(n || 0).toFixed(2);
  }

  // ---- Data loading ----

  async function loadAll() {
    if (!slug) return;
    error = '';
    loading = true;
    const token = getToken(slug);
    if (!token) {
      error = 'Not authenticated for this group';
      loading = false;
      return;
    }
    const gen = ++loadGen;
    try {
      const [g, exps, pays, bals] = await Promise.all([
        api.get(`/api/groups/${slug}`, token),
        api.get(`/api/groups/${slug}/expenses`, token),
        api.get(`/api/groups/${slug}/payments`, token),
        api.get(`/api/groups/${slug}/balances`, token),
      ]);
      if (gen !== loadGen) return; // stale response, ignore
      group = g;
      members = g.members || [];
      expenses = exps;
      payments = pays;
      balances = bals;
      // If user selected custom split before members loaded, populate now
      if (expSplitType === 'custom') {
        handleSplitTypeChange();
      }
    } catch (e) {
      if (gen !== loadGen) return; // stale error
      error = e.message;
    } finally {
      if (gen === loadGen) loading = false;
    }
  }

  // ---- Expense form ----

  function resetExpForm() {
    expDesc = '';
    expAmt = 0;
    expSplitType = 'equal';
    expCustomSplits = [];
    expError = '';
    showExpForm = false;
  }

  function handleSplitTypeChange() {
    if (expSplitType === 'custom') {
      if (members.length === 0) return; // members not loaded yet; loadAll will populate
      const existing = new Map(expCustomSplits.map(s => [s.user_id, s.amount]));
      expCustomSplits = members.map(m => ({
        user_id: m.id,
        amount: existing.get(m.id) || 0,
      }));
    }
  }

  async function handleCreateExpense(e) {
    e.preventDefault();
    expError = '';
    expCreating = true;
    const token = getToken(slug);
    try {
      const body = {
        description: expDesc,
        amount: parseFloat(expAmt),
        split_type: expSplitType,
      };
      if (expSplitType === 'custom') {
        body.splits = expCustomSplits
          .filter(s => s.amount > 0)
          .map(s => ({ user_id: s.user_id, amount: parseFloat(s.amount) }));
      }
      await api.post(`/api/groups/${slug}/expenses`, body, token);
      resetExpForm();
      const [exps, bals] = await Promise.all([
        api.get(`/api/groups/${slug}/expenses`, token),
        api.get(`/api/groups/${slug}/balances`, token),
      ]);
      expenses = exps;
      balances = bals;
    } catch (e) {
      expError = e.message;
    } finally {
      expCreating = false;
    }
  }

  // ---- Payment form ----

  function resetPayForm() {
    payFrom = currentMemberId || '';
    payTo = '';
    payMethod = '';
    payAmt = 0;
    payError = '';
    showPayForm = false;
  }

  async function handleCreatePayment(e) {
    e.preventDefault();
    payError = '';
    payCreating = true;
    const token = getToken(slug);
    try {
      await api.post(`/api/groups/${slug}/payments`, {
        from_user: payFrom,
        to_user: payTo,
        amount: parseFloat(payAmt),
        method: payMethod || undefined,
      }, token);
      resetPayForm();
      const [pays, bals] = await Promise.all([
        api.get(`/api/groups/${slug}/payments`, token),
        api.get(`/api/groups/${slug}/balances`, token),
      ]);
      payments = pays;
      balances = bals;
    } catch (e) {
      payError = e.message;
    } finally {
      payCreating = false;
    }
  }

  // ---- Payment actions ----

  async function handleConfirmPayment(payId) {
    const token = getToken(slug);
    try {
      await api.post(`/api/payments/${payId}/confirm`, undefined, token);
      const [pays, bals] = await Promise.all([
        api.get(`/api/groups/${slug}/payments`, token),
        api.get(`/api/groups/${slug}/balances`, token),
      ]);
      payments = pays;
      balances = bals;
    } catch (e) {
      alert('Failed to confirm: ' + e.message);
    }
  }

  async function handleCancelPayment(payId) {
    if (!confirm('Cancel this payment?')) return;
    const token = getToken(slug);
    try {
      await api.del(`/api/payments/${payId}`, token);
      const [pays, bals] = await Promise.all([
        api.get(`/api/groups/${slug}/payments`, token),
        api.get(`/api/groups/${slug}/balances`, token),
      ]);
      payments = pays;
      balances = bals;
    } catch (e) {
      alert('Failed to cancel: ' + e.message);
    }
  }

  // ---- Split helpers for display ----

  function getMemberName(uid) {
    const member = members.find(m => m.id === uid);
    if (!member) return truncId(uid);
    return uid === currentMemberId ? `${member.name} (you)` : member.name;
  }

  // ---- Init ----

  function loadFromLocal() {
    const info = getGroupInfo(slug);
    if (info) {
      currentMemberId = info.member_id || null;
    }
  }

  $effect(() => {
    const s = slug;
    if (!s) return;

    loadFromLocal();
    loadAll();
    connect(s);
    const info = getGroupInfo(s);
    if (info && info.internal_id) {
      subscribe(info.internal_id);
    }
    setOnUpdate(() => {
      loadAll();
    });

    return () => {
      loadGen++;
      setOnUpdate(null);
      wsDisconnect();
    };
  });

  $effect(() => {
    if (currentMemberId) {
      payFrom = currentMemberId;
    }
  });

</script>

{#if loading}
  <div class="empty-state"><span class="spinner"></span> Loading group…</div>
{:else if error}
  <div class="alert alert-error">{error}</div>
  <button class="btn" onclick={loadAll}>Retry</button>
{:else if group}
  <div class="detail-header">
    <button class="btn btn-sm" onclick={onBack}>&larr; Back</button>
    <div class="detail-header-info">
      <h2 class="detail-title">{group.name}</h2>
      <div class="member-chips">
        {#each members as m}
          <span class="member-chip" title={m.id}>{m.name}{m.id === currentMemberId ? ' (you)' : ''}</span>
        {/each}
      </div>
    </div>
    <button class="btn btn-sm" onclick={() => showShare = true} title="Share invite link">
      Share
    </button>
  </div>

  <!-- Tabs -->
  <div class="tabs">
    <button
      class="tab"
      class:active={activeTab === 'expenses'}
      onclick={() => activeTab = 'expenses'}
    >Expenses ({expenses.length})</button>
    <button
      class="tab"
      class:active={activeTab === 'payments'}
      onclick={() => activeTab = 'payments'}
    >Payments ({payments.length})</button>
    <button
      class="tab"
      class:active={activeTab === 'balances'}
      onclick={() => activeTab = 'balances'}
    >Balance</button>
  </div>

  <!-- ==================== EXPENSES TAB ==================== -->
  {#if activeTab === 'expenses'}
    <div class="section-actions">
      <button class="btn btn-primary btn-sm" onclick={() => showExpForm = !showExpForm}>
        {showExpForm ? 'Cancel' : '+ Add Expense'}
      </button>
    </div>

    {#if showExpForm}
      <div class="form-section">
        <div class="form-section-title">New Expense</div>
        {#if expError}
          <div class="alert alert-error">{expError}</div>
        {/if}
        <form onsubmit={handleCreateExpense}>
          <div class="form-group">
            <label class="form-label" for="exp-desc">Description</label>
            <input id="exp-desc" class="form-input" type="text" placeholder="e.g. Dinner" bind:value={expDesc} required disabled={expCreating} />
          </div>
          <div class="field-row">
            <div class="form-group">
              <label class="form-label" for="exp-amt">Amount</label>
              <input id="exp-amt" class="form-input" type="number" step="0.01" min="0.01" placeholder="0.00" bind:value={expAmt} required disabled={expCreating} />
            </div>
            <div class="form-group">
              <label class="form-label" for="exp-split">Split type</label>
              <select id="exp-split" class="form-select" bind:value={expSplitType} disabled={expCreating} onchange={handleSplitTypeChange}>
                <option value="equal">Equal</option>
                <option value="custom">Custom</option>
              </select>
            </div>
          </div>

          {#if expSplitType === 'custom' && expCustomSplits.length > 0}
            <div class="form-section-title" style="margin-top: 0.75rem;">Split amounts</div>
            {#each expCustomSplits as split, i}
              <div class="split-row">
                <span class="uuid-short">{getMemberName(split.user_id)}</span>
                <input
                  class="form-input split-input"
                  type="number"
                  step="0.01"
                  min="0"
                  placeholder="0.00"
                  bind:value={expCustomSplits[i].amount}
                  disabled={expCreating}
                />
              </div>
            {/each}
            {#if Math.abs(expCustomSplitsSum - Number(expAmt)) > 0.005}
              <div class="split-warning">Split amounts sum (${fmt(expCustomSplitsSum)}) ≠ total (${fmt(expAmt)})</div>
            {/if}
          {/if}

          <button class="btn btn-primary" type="submit" disabled={expCreating} style="margin-top: 0.5rem;">
            {expCreating ? 'Adding…' : 'Add Expense'}
          </button>
        </form>
      </div>
    {/if}

    {#if expenses.length === 0}
      <div class="empty-state">No expenses yet.</div>
    {:else}
      {#each expenses as exp}
        <div class="card">
          <div class="card-header">
            <span class="card-title">{exp.description}</span>
            <span class="amount">${fmt(exp.amount)}</span>
          </div>
          <div class="exp-meta">
            <span class="badge">paid by {getMemberName(exp.paid_by)}</span>
            <span class="badge">{exp.split_type}</span>
            {#if exp.created_at}
              <span class="list-item-subtitle">{formatDate(exp.created_at)}</span>
            {/if}
          </div>
          {#if exp.splits && exp.splits.length > 0}
            <div class="split-list">
              {#each exp.splits as split}
                <div class="split-item">
                  <span class="uuid-short">{getMemberName(split.user_id)}</span>
                  <span class="amount">${fmt(split.amount)}</span>
                </div>
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    {/if}

  <!-- ==================== PAYMENTS TAB ==================== -->
  {:else if activeTab === 'payments'}
    <div class="section-actions">
      <button class="btn btn-primary btn-sm" onclick={() => showPayForm = !showPayForm}>
        {showPayForm ? 'Cancel' : '+ Record Payment'}
      </button>
    </div>

    {#if showPayForm}
      <div class="form-section">
        <div class="form-section-title">Record Payment</div>
        {#if payError}
          <div class="alert alert-error">{payError}</div>
        {/if}
        <form onsubmit={handleCreatePayment}>
          <div class="field-row">
            <div class="form-group">
              <div class="form-label">From (you)</div>
              <div class="form-static-value">{members.find(m => m.id === currentMemberId)?.name || 'You'}</div>
            </div>
            <div class="form-group">
              <label class="form-label" for="pay-to">To (recipient)</label>
              <select id="pay-to" class="form-select" bind:value={payTo} required disabled={payCreating}>
                <option value="">Select recipient</option>
                {#each members as m}
                  <option value={m.id}>{m.name}{m.id === currentMemberId ? ' (you)' : ''}</option>
                {/each}
              </select>
            </div>
          </div>
          <div class="field-row">
            <div class="form-group">
              <label class="form-label" for="pay-amt">Amount</label>
              <input id="pay-amt" class="form-input" type="number" step="0.01" min="0.01" placeholder="0.00" bind:value={payAmt} required disabled={payCreating} />
            </div>
            <div class="form-group">
              <label class="form-label" for="pay-method">Method (optional)</label>
              <input id="pay-method" class="form-input" type="text" placeholder="e.g. Venmo" bind:value={payMethod} disabled={payCreating} />
            </div>
          </div>
          <button class="btn btn-primary" type="submit" disabled={payCreating}>
            {payCreating ? 'Recording…' : 'Record Payment'}
          </button>
        </form>
      </div>
    {/if}

    {#if payments.length === 0}
      <div class="empty-state">No payments recorded yet.</div>
    {:else}
      {#each payments as pay}
        <div class="card">
          <div class="card-header">
            <span class="card-title">{getMemberName(pay.from_user)} &rarr; {getMemberName(pay.to_user)}</span>
            <span class="amount">${fmt(pay.amount)}</span>
          </div>
          <div class="exp-meta">
            {#if pay.status === 'confirmed'}
              <span class="badge badge-success">confirmed</span>
            {:else}
              <span class="badge badge-warning">pending</span>
            {/if}
            {#if pay.method}
              <span class="badge">{pay.method}</span>
            {/if}
            {#if pay.created_at}
              <span class="list-item-subtitle">{formatDate(pay.created_at)}</span>
            {/if}
          </div>
          {#if pay.status === 'pending'}
            <div class="pay-actions">
              {#if currentMemberId && pay.to_user === currentMemberId}
                <button class="btn btn-sm btn-primary" onclick={() => handleConfirmPayment(pay.id)}>Confirm</button>
              {/if}
              <button class="btn btn-sm btn-danger" onclick={() => handleCancelPayment(pay.id)}>Cancel</button>
            </div>
          {/if}
        </div>
      {/each}
    {/if}

  <!-- ==================== BALANCES TAB ==================== -->
  {:else if activeTab === 'balances'}
    {#if (!balances.members || balances.members.length === 0) && (!balances.payments || balances.payments.length === 0) && (!balances.settlements || balances.settlements.length === 0)}
      <div class="empty-state">
        <p>All balanced up!</p>
        <p style="margin-top: 0.25rem; font-size: 0.85rem;">No expenses or payments yet.</p>
      </div>
    {:else}
      {#if balances.members && balances.members.length > 0}
        <div class="card">
          <div class="card-header">
            <span class="card-title">Net Position</span>
          </div>
          <div class="member-bal-list">
            {#each balances.members as m}
              <div class="member-bal-item">
                <span>{getMemberName(m.user_id)}</span>
                <span class="amount" class:amount-positive={m.balance > 0.01} class:amount-negative={m.balance < -0.01}>
                  {m.balance > 0 ? '+' : ''}{fmt(m.balance)}
                </span>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if balances.payments && balances.payments.length > 0}
        <div class="card">
          <div class="card-header">
            <span class="card-title">Payments Made</span>
          </div>
          <div class="payments-list">
            {#each balances.payments as pay}
              <div class="payment-item-inline">
                <div class="payment-item-main">
                  <span class="payment-direction">{getMemberName(pay.from)} &rarr; {getMemberName(pay.to)}</span>
                  <span class="amount">${fmt(pay.amount)}</span>
                </div>
                <div class="payment-meta">
                  {#if pay.status === 'confirmed'}
                    <span class="badge badge-success">confirmed</span>
                  {:else}
                    <span class="badge badge-warning">pending</span>
                  {/if}
                  {#if pay.method}
                    <span class="badge">{pay.method}</span>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/if}

      {#if balances.settlements && balances.settlements.length > 0}
        <div class="card">
          <div class="card-header">
            <span class="card-title">Recommended Settlements</span>
          </div>
          <div class="bal-list">
            {#each balances.settlements as bal}
              <div class="bal-item">
                <div class="bal-main">
                  <div class="bal-direction">
                    <span class="uuid-short">{getMemberName(bal.from)}</span>
                    <span class="bal-arrow">&rarr;</span>
                    <span class="uuid-short">{getMemberName(bal.to)}</span>
                  </div>
                  <span class="amount amount-negative">${fmt(bal.amount)}</span>
                </div>
                {#if bal.breakdown && bal.breakdown.length > 0}
                  <div class="bal-breakdown">
                    {#each bal.breakdown as b}
                      <div class="bal-breakdown-item">
                        <span class="bal-breakdown-name">{b.expense_name}</span>
                        <span class="bal-breakdown-amt" class:negative={b.amount < 0}>{b.amount < 0 ? '-$' : '$'}{fmt(Math.abs(b.amount))}</span>
                      </div>
                    {/each}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {:else}
        <div class="empty-state" style="margin-top: 1rem;">
          <p>All settled up!</p>
        </div>
      {/if}
    {/if}
  {/if}
{/if}

{#if showShare && inviteLink}
  <ShareModal link={inviteLink} onClose={() => showShare = false} />
{/if}

<style>
  .detail-header {
    display: flex;
    align-items: flex-start;
    gap: 0.75rem;
    margin-bottom: 1.25rem;
  }

  .detail-header-info {
    flex: 1;
    min-width: 0;
  }

  .detail-title {
    font-size: 1.25rem;
    font-weight: 700;
    margin-bottom: 0.5rem;
  }

  .section-actions {
    margin-bottom: 1rem;
  }

  .exp-meta {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
    margin-bottom: 0.5rem;
  }

  .split-list {
    border-top: 1px solid var(--border);
    padding-top: 0.625rem;
    margin-top: 0.25rem;
  }

  .split-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.25rem 0;
    font-size: 0.85rem;
  }

  .split-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }

  .split-input {
    width: 120px;
    flex-shrink: 0;
  }

  .split-warning {
    color: var(--text-warning, #d97706);
    font-size: 0.85rem;
    margin-top: 0.25rem;
    font-weight: 500;
  }

  .form-static-value {
    padding: 0.5rem 0.75rem;
    background: var(--bg-muted, var(--bg));
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 0.9rem;
    color: var(--text);
    min-height: 38px;
    display: flex;
    align-items: center;
  }

  .pay-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--border);
  }

  .bal-list {
    display: flex;
    flex-direction: column;
  }

  .bal-item {
    padding: 0.75rem 0;
    border-bottom: 1px solid var(--border);
  }

  .bal-item:last-child {
    border-bottom: none;
  }

  .bal-main {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .bal-direction {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .bal-arrow {
    color: var(--text-muted);
    font-size: 1.1rem;
  }

  .bal-breakdown {
    margin-top: 0.5rem;
    padding-left: 0.25rem;
    border-top: 1px solid var(--border-subtle, var(--border));
    padding-top: 0.5rem;
  }

  .bal-breakdown-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.85rem;
    padding: 0.15rem 0;
  }

  .bal-breakdown-name {
    color: var(--text-muted);
  }

  .bal-breakdown-amt {
    font-variant-numeric: tabular-nums;
  }

  .bal-breakdown-amt.negative {
    color: var(--text-danger, #c0392b);
  }

  .member-bal-list {
    display: flex;
    flex-direction: column;
  }

  .member-bal-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.4rem 0;
    font-size: 0.9rem;
    border-bottom: 1px solid var(--border-subtle, var(--border));
  }

  .member-bal-item:last-child {
    border-bottom: none;
  }

  .payments-list {
    display: flex;
    flex-direction: column;
  }

  .payment-item-inline {
    padding: 0.5rem 0;
    border-bottom: 1px solid var(--border-subtle, var(--border));
  }

  .payment-item-inline:last-child {
    border-bottom: none;
  }

  .payment-item-main {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .payment-direction {
    font-size: 0.9rem;
  }

  .payment-meta {
    display: flex;
    align-items: center;
    gap: 0.375rem;
    margin-top: 0.25rem;
  }
</style>
