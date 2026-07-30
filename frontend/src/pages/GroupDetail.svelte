<script>
  import { currentGroupSlug } from '../lib/stores.js';
  import { api, getToken, getGroupInfo } from '../lib/api.js';
  import { createOp, getLocalOps, syncGroup, syncInProgress } from '../lib/sync.js';
  import { HLC } from '../lib/crdt.js';
  import { online, pendingOpsCount } from '../lib/networkStore.js';
  import { get } from 'svelte/store';
  import { connect, subscribe, setOnUpdate, disconnect as wsDisconnect } from '../lib/websocket.js';
  import ShareModal from '../lib/ShareModal.svelte';
import { swipeBack, pullToRefresh, swipeReveal } from '../lib/gestures.js';
  import { haptic } from '../lib/haptic.js';
  import BottomSheet from '../lib/BottomSheet.svelte';
  import { showToast } from '../lib/toastStore.js';
import { scrollIntoViewOnFocus } from '../lib/forms.js';

  let { onBack } = $props();

  let slug = $derived($currentGroupSlug);
  let group = $state(null);
  let members = $state([]);
  let expenses = $state([]);
  let payments = $state([]);
  let balances = $state([]);
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
  let confirmDialog = $state({ show: false, payId: null, expId: null, action: 'cancelPayment' });
  let fabLabel = $derived(activeTab === 'expenses' ? 'Add expense' : 'Record payment');

  function toggleFabAction() {
    if (activeTab === 'expenses') {
      showExpForm = !showExpForm;
    } else if (activeTab === 'payments') {
      showPayForm = !showPayForm;
    }
  }

  let loadGen = 0;
  let hlc = new HLC();

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

    // CRDT ops for offline fallback
    const expenseId = crypto.randomUUID();
    const timestamp = hlc.now();
    const groupInfo = getGroupInfo(slug);
    const groupId = groupInfo?.internal_id || slug;

    const ops = [];
    ops.push({ doc_id: expenseId, op_type: 'lww', field: 'description', value: JSON.stringify(expDesc), author_id: currentMemberId, timestamp });
    ops.push({ doc_id: expenseId, op_type: 'lww', field: 'amount', value: JSON.stringify(parseFloat(expAmt)), author_id: currentMemberId, timestamp });
    ops.push({ doc_id: expenseId, op_type: 'lww', field: 'paid_by', value: JSON.stringify(currentMemberId), author_id: currentMemberId, timestamp });
    ops.push({ doc_id: expenseId, op_type: 'lww', field: 'group_id', value: JSON.stringify(groupId), author_id: currentMemberId, timestamp });
    ops.push({ doc_id: expenseId, op_type: 'lww', field: 'split_type', value: JSON.stringify(expSplitType), author_id: currentMemberId, timestamp });

    if (expSplitType === 'custom' && expCustomSplits.length > 0) {
      for (const split of expCustomSplits.filter(s => s.amount > 0)) {
        ops.push({
          doc_id: expenseId, op_type: 'rga_insert', field: 'splits',
          value: JSON.stringify({ user_id: split.user_id, amount: parseFloat(split.amount) }),
          item_id: crypto.randomUUID(), prev_item_id: '',
          author_id: currentMemberId, timestamp: hlc.now(),
        });
      }
    }

    try {
      if (get(online)) {
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
      } else {
        throw new Error('offline');
      }
      resetExpForm();
      const [exps, bals] = await Promise.all([
        api.get(`/api/groups/${slug}/expenses`, token),
        api.get(`/api/groups/${slug}/balances`, token),
      ]);
      expenses = exps;
      balances = bals;
    } catch (e) {
      if (!get(online) || e.message === 'offline' || e.message?.includes('Network error')) {
        // Offline: queue CRDT ops
        for (const op of ops) {
          await createOp(op, slug);
        }
        const newExpense = {
          id: expenseId,
          description: expDesc,
          amount: parseFloat(expAmt),
          split_type: expSplitType,
          paid_by: currentMemberId,
          created_at: Date.now(),
          splits: expSplitType === 'custom'
            ? expCustomSplits.filter(s => s.amount > 0).map(s => ({ user_id: s.user_id, amount: parseFloat(s.amount) }))
            : [],
          _pending: true,
        };
        expenses = [...expenses, newExpense];
        resetExpForm();
      } else {
        expError = e.message;
      }
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

    // CRDT ops for offline fallback
    const paymentId = crypto.randomUUID();
    const timestamp = hlc.now();
    const groupInfo = getGroupInfo(slug);
    const groupId = groupInfo?.internal_id || slug;

    const ops = [
      { doc_id: paymentId, op_type: 'lww', field: 'from_user', value: JSON.stringify(payFrom), author_id: currentMemberId, timestamp },
      { doc_id: paymentId, op_type: 'lww', field: 'to_user', value: JSON.stringify(payTo), author_id: currentMemberId, timestamp },
      { doc_id: paymentId, op_type: 'lww', field: 'amount', value: JSON.stringify(parseFloat(payAmt)), author_id: currentMemberId, timestamp },
      { doc_id: paymentId, op_type: 'lww', field: 'method', value: JSON.stringify(payMethod || ''), author_id: currentMemberId, timestamp },
      { doc_id: paymentId, op_type: 'lww', field: 'status', value: JSON.stringify('pending'), author_id: currentMemberId, timestamp },
      { doc_id: paymentId, op_type: 'lww', field: 'group_id', value: JSON.stringify(groupId), author_id: currentMemberId, timestamp },
    ];

    try {
      if (get(online)) {
        await api.post(`/api/groups/${slug}/payments`, {
          from_user: payFrom,
          to_user: payTo,
          amount: parseFloat(payAmt),
          method: payMethod || undefined,
        }, token);
      } else {
        throw new Error('offline');
      }
      resetPayForm();
      const [pays, bals] = await Promise.all([
        api.get(`/api/groups/${slug}/payments`, token),
        api.get(`/api/groups/${slug}/balances`, token),
      ]);
      payments = pays;
      balances = bals;
    } catch (e) {
      if (!get(online) || e.message === 'offline' || e.message?.includes('Network error')) {
        // Offline: queue CRDT ops
        for (const op of ops) {
          await createOp(op, slug);
        }
        const newPayment = {
          id: paymentId,
          from_user: payFrom,
          to_user: payTo,
          amount: parseFloat(payAmt),
          method: payMethod || '',
          status: 'pending',
          created_at: Date.now(),
          _pending: true,
        };
        payments = [...payments, newPayment];
        resetPayForm();
      } else {
        payError = e.message;
      }
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
      showToast('Payment confirmed', 'success');
    } catch (e) {
      showToast('Failed to confirm: ' + e.message, 'error');
    }
  }

  async function handleCancelPayment(payId) {
    confirmDialog = { show: true, payId, expId: null, action: 'cancelPayment' };
  }

  async function handleDeleteExpense(expId) {
    confirmDialog = { show: true, payId: null, expId, action: 'deleteExpense' };
  }

  async function onConfirmCancel() {
    if (confirmDialog.action === 'cancelPayment') {
      const payId = confirmDialog.payId;
      confirmDialog = { show: false, payId: null, expId: null, action: 'cancelPayment' };
      if (!payId) return;
      const token = getToken(slug);
      try {
        await api.del(`/api/payments/${payId}`, token);
        const [pays, bals] = await Promise.all([
          api.get(`/api/groups/${slug}/payments`, token),
          api.get(`/api/groups/${slug}/balances`, token),
        ]);
        payments = pays;
        balances = bals;
        showToast('Payment cancelled', 'info');
      } catch (e) {
        showToast('Failed to cancel: ' + e.message, 'error');
      }
    } else if (confirmDialog.action === 'deleteExpense') {
      const expId = confirmDialog.expId;
      confirmDialog = { show: false, payId: null, expId: null, action: 'cancelPayment' };
      if (!expId) return;
      const token = getToken(slug);
      try {
        await api.del(`/api/expenses/${expId}`, token);
        const [exps, bals] = await Promise.all([
          api.get(`/api/groups/${slug}/expenses`, token),
          api.get(`/api/groups/${slug}/balances`, token),
        ]);
        expenses = exps;
        balances = bals;
        showToast('Expense deleted', 'success');
      } catch (e) {
        showToast('Failed to delete: ' + e.message, 'error');
      }
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

    const unsubOnline = online.subscribe(async ($online) => {
      if ($online && s) {
        const localOps = await getLocalOps(s);
        if (localOps.length > 0) {
          await syncGroup(s);
          await loadAll();
        }
      }
    });

    return () => {
      unsubOnline();
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

<div class="detail-page" use:swipeBack={{ onBack }}>
  <div class="detail-scroll" use:pullToRefresh={{ onRefresh: loadAll }}>
{#if loading}
  <div class="empty-state"><span class="spinner"></span> Loading group…</div>
{:else if error}
  <div class="alert alert-error">{error}</div>
  <button class="btn" onclick={loadAll}>Retry</button>
{:else if group}
  <div class="detail-header">
    <button class="btn btn-sm" onclick={onBack} use:haptic>&larr; Back</button>
    <div class="detail-header-info">
      <h2 class="detail-title">{group.name}</h2>
      <div class="member-chips">
        {#each members as m}
          <span class="member-chip" title={m.id}>{m.name}{m.id === currentMemberId ? ' (you)' : ''}</span>
        {/each}
      </div>
    </div>
    <button class="btn btn-sm" onclick={() => showShare = true} title="Share invite link" use:haptic>
      Share
    </button>
  </div>

  <!-- Tabs -->
  <div class="tabs">
    <button
      class="tab"
      class:active={activeTab === 'expenses'}
      onclick={() => activeTab = 'expenses'}
      use:haptic
    >Expenses ({expenses.length})</button>
    <button
      class="tab"
      class:active={activeTab === 'payments'}
      onclick={() => activeTab = 'payments'}
      use:haptic
    >Payments ({payments.length})</button>
    <button
      class="tab"
      class:active={activeTab === 'balances'}
      onclick={() => activeTab = 'balances'}
      use:haptic
    >Balance</button>
  </div>

  <!-- ==================== EXPENSES TAB ==================== -->
  {#if activeTab === 'expenses'}
    <div class="section-actions">
      <button class="btn btn-primary btn-sm" onclick={() => showExpForm = !showExpForm} use:haptic>
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
            <input use:scrollIntoViewOnFocus id="exp-desc" class="form-input" type="text" placeholder="e.g. Dinner" bind:value={expDesc} required disabled={expCreating} />
          </div>
          <div class="field-row">
            <div class="form-group">
              <label class="form-label" for="exp-amt">Amount</label>
              <input use:scrollIntoViewOnFocus id="exp-amt" class="form-input" type="number" step="0.01" min="0.01" placeholder="0.00" bind:value={expAmt} required disabled={expCreating} />
            </div>
            <div class="form-group">
              <label class="form-label" for="exp-split">Split type</label>
              <select use:scrollIntoViewOnFocus id="exp-split" class="form-select" bind:value={expSplitType} disabled={expCreating} onchange={handleSplitTypeChange}>
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
                <input use:scrollIntoViewOnFocus
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

          <button class="btn btn-primary" type="submit" disabled={expCreating} style="margin-top: 0.5rem;" use:haptic>
            {expCreating ? 'Adding…' : 'Add Expense'}
          </button>
        </form>
      </div>
    {/if}

    {#if expenses.length === 0}
      <div class="empty-state">No expenses yet.</div>
    {:else}
      {#each expenses as exp}
        <div class="swipe-container" use:swipeReveal={{ onAction: () => handleDeleteExpense(exp.id), actionLabel: 'Delete', actionVariant: 'danger' }}>
          <div class="swipe-content card">
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
              {#if exp._pending}
                <span class="badge badge-warning" title="Not yet synced">Pending</span>
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
          <div class="swipe-action"></div>
        </div>
      {/each}
    {/if}

  <!-- ==================== PAYMENTS TAB ==================== -->
  {:else if activeTab === 'payments'}
    <div class="section-actions">
      <button class="btn btn-primary btn-sm" onclick={() => showPayForm = !showPayForm} use:haptic>
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
              <select use:scrollIntoViewOnFocus id="pay-to" class="form-select" bind:value={payTo} required disabled={payCreating}>
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
              <input use:scrollIntoViewOnFocus id="pay-amt" class="form-input" type="number" step="0.01" min="0.01" placeholder="0.00" bind:value={payAmt} required disabled={payCreating} />
            </div>
            <div class="form-group">
              <label class="form-label" for="pay-method">Method (optional)</label>
              <input use:scrollIntoViewOnFocus id="pay-method" class="form-input" type="text" placeholder="e.g. Venmo" bind:value={payMethod} disabled={payCreating} />
            </div>
          </div>
          <button class="btn btn-primary" type="submit" disabled={payCreating} use:haptic>
            {payCreating ? 'Recording…' : 'Record Payment'}
          </button>
        </form>
      </div>
    {/if}

    {#if payments.length === 0}
      <div class="empty-state">No payments recorded yet.</div>
    {:else}
      {#each payments as pay}
        <div class="swipe-container" use:swipeReveal={{ onAction: () => handleCancelPayment(pay.id), actionLabel: 'Cancel', actionVariant: 'danger' }}>
          <div class="swipe-content card">
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
              {#if pay._pending}
                <span class="badge badge-warning" title="Not yet synced">Pending</span>
              {/if}
            </div>
            {#if pay.status === 'pending'}
              <div class="pay-actions">
                {#if currentMemberId && pay.to_user === currentMemberId}
                  <button class="btn btn-sm btn-primary" onclick={() => handleConfirmPayment(pay.id)} use:haptic>Confirm</button>
                {/if}
                <button class="btn btn-sm btn-danger" onclick={() => handleCancelPayment(pay.id)} use:haptic>Cancel</button>
              </div>
            {/if}
          </div>
          <div class="swipe-action"></div>
        </div>
      {/each}
    {/if}

  <!-- ==================== BALANCES TAB ==================== -->
  {:else if activeTab === 'balances'}
    {#if balances.length === 0}
      <div class="empty-state">
        <p>All balanced up!</p>
        <p style="margin-top: 0.25rem; font-size: 0.85rem;">No outstanding balances.</p>
      </div>
    {:else}
      <div class="card">
        <div class="card-header">
          <span class="card-title">Balance Breakdown</span>
        </div>
        <div class="bal-list">
          {#each balances as bal}
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
    {/if}
  {/if}
{/if}
  </div>
</div>

<!-- FAB -->
{#if (activeTab === 'expenses' && !showExpForm) || (activeTab === 'payments' && !showPayForm)}
  <button class="fab" onclick={toggleFabAction} use:haptic aria-label={fabLabel}>
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
      <line x1="12" y1="5" x2="12" y2="19" />
      <line x1="5" y1="12" x2="19" y2="12" />
    </svg>
  </button>
{/if}

{#if showShare && inviteLink}
  <ShareModal link={inviteLink} onClose={() => showShare = false} />
{/if}

<BottomSheet
  show={confirmDialog.show}
  title={confirmDialog.action === 'deleteExpense' ? 'Delete Expense' : 'Cancel Payment'}
  message={confirmDialog.action === 'deleteExpense' ? 'Are you sure you want to delete this expense?' : 'Are you sure you want to cancel this payment?'}
  variant="danger"
  confirmText={confirmDialog.action === 'deleteExpense' ? 'Delete' : 'Cancel Payment'}
  onConfirm={onConfirmCancel}
  onCancel={() => confirmDialog = { show: false, payId: null, expId: null, action: 'cancelPayment' }}
/>

<style>
  .detail-header {
    display: flex;
    align-items: flex-start;
    gap: 0.5rem;
    margin-bottom: 1rem;
  }

  .detail-header-info {
    flex: 1;
    min-width: 0;
  }

  .detail-title {
    font-size: 1.25rem;
    font-weight: 700;
    margin-bottom: 0.375rem;
  }

  .section-actions {
    margin-bottom: 0.75rem;
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
    margin-bottom: 0.625rem;
  }

  .split-input {
    width: 120px;
    flex-shrink: 0;
    min-height: var(--touch-target, 44px);
  }

  .split-warning {
    color: var(--text-warning, #d97706);
    font-size: 0.85rem;
    margin-bottom: 0.375rem;
    font-weight: 500;
  }

  .form-static-value {
    padding: 0.5rem 0.75rem;
    background: var(--bg-primary, var(--bg));
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 0.9rem;
    color: var(--text-primary);
    min-height: var(--touch-target, 44px);
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

  .swipe-container {
    overflow: hidden;
    position: relative;
    margin-bottom: 1rem;
  }

  .swipe-content {
    position: relative;
    z-index: 1;
    background: var(--bg-card);
    border-radius: var(--radius);
    will-change: transform;
  }

  .swipe-action {
    position: absolute;
    right: 0;
    top: 0;
    height: 100%;
  }

  .detail-page {
    position: relative;
    overflow: hidden;
    min-height: calc(100dvh - 5rem);
  }

  .detail-scroll {
    overflow-y: auto;
    -webkit-overflow-scrolling: touch;
    overscroll-behavior: contain;
    max-height: calc(100dvh - 5rem);
  }

  .fab {
    position: fixed;
    bottom: calc(1.5rem + var(--safe-bottom, 0px));
    right: calc(1.5rem + var(--safe-right, 0px));
    width: 56px;
    height: 56px;
    border-radius: 50%;
    background: var(--accent-dim, #1f6feb);
    border: none;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.4);
    z-index: 30;
    transition: transform 0.15s, background 0.15s, box-shadow 0.15s;
    -webkit-tap-highlight-color: transparent;
  }

  .fab:hover {
    background: var(--accent, #58a6ff);
    box-shadow: 0 6px 16px rgba(0, 0, 0, 0.5);
  }

  .fab:active {
    transform: scale(0.92);
  }

  .fab svg {
    display: block;
  }
</style>
