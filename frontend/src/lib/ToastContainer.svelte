<script>
  import { toasts } from './toastStore.js';
  import Toast from './Toast.svelte';
</script>

{#each $toasts as toast}
  <Toast id={toast.id} message={toast.message} type={toast.type} duration={toast.duration} />
{/each}
