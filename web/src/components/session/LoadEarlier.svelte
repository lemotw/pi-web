<script>
  import { tick } from 'svelte';
  import { t } from '../../shared/i18n.js';
  import { SESSION_WINDOW_SIZES, saveSessionWindowSize } from '../../session/session-window.js';

  let { model, loadWindow, navigateTo = null } = $props();

  const end = $derived(model.from + model.entries.length);

  async function selectWindow(options) {
    try {
      const data = await loadWindow(options);
      if (!data) return;
      saveSessionWindowSize(model.windowSize);
      const url = new URL(window.location.href);
      url.searchParams.set('limit', String(model.windowSize));
      url.searchParams.delete('targetId');
      url.searchParams.delete('leafId');
      window.history.replaceState(window.history.state, '', url);
      window.dispatchEvent(new CustomEvent('pi-session-window-changed'));
      await tick();
      navigateTo?.(model.leafId, 'bottom');
    } catch {
      // The shared loader preserves the current page and exposes a retryable error.
    }
  }
</script>

<div class="session-window" aria-busy={model.windowBusy}>
  <label>
    {t('session.windowSize')}
    <select
      aria-label={t('session.windowSize')}
      value={model.windowSize}
      disabled={model.windowBusy}
      onchange={(event) => selectWindow({ limit: Number(event.currentTarget.value), before: null })}
    >
      {#each SESSION_WINDOW_SIZES as size (size)}<option value={size}>{size}</option>{/each}
    </select>
  </label>
  <span class="range" aria-live="polite"
    >{t('session.windowRange', {
      from: model.entries.length ? model.from + 1 : 0,
      to: end,
      total: model.total,
    })}</span
  >
  <button
    disabled={model.windowBusy || model.from === 0}
    onclick={() => selectWindow({ before: model.from })}
  >
    {t('session.windowEarlier')}
  </button>
  <button
    disabled={model.windowBusy || end >= model.total}
    onclick={() =>
      selectWindow({
        before: end + model.windowSize >= model.total ? null : end + model.windowSize,
      })}>{t('session.windowNewer')}</button
  >
  <button
    disabled={model.windowBusy || model.windowEnd === null}
    onclick={() => selectWindow({ before: null })}
  >
    {t('session.windowLatest')}
  </button>
  <small>{t('session.windowHint')}</small>
  {#if model.windowError}
    <span role="alert">{t('session.loadEarlierFailed', { error: model.windowError })}</span>
    <button disabled={model.windowBusy} onclick={() => selectWindow({})}
      >{t('session.windowRetry')}</button
    >
  {/if}
</div>

<style>
  .session-window {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem;
    color: var(--text-soft);
    font-size: 0.8rem;
  }
  label {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  select,
  button {
    font: inherit;
    color: var(--text);
    background: var(--surface-2);
    border: 1px solid var(--dim);
    border-radius: 6px;
    padding: 0.35rem 0.5rem;
  }
  button:not(:disabled),
  select {
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.5;
  }
  .range {
    margin-right: auto;
  }
  small {
    flex-basis: 100%;
  }
</style>
