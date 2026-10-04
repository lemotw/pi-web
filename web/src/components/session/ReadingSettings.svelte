<script>
  import { onMount, tick } from 'svelte';
  import { icon, SlidersHorizontal, X, ChevronLeft, ChevronRight } from '../../shared/icons.js';
  import { t } from '../../shared/i18n.js';
  import { SESSION_WINDOW_SIZES, saveSessionWindowSize } from '../../session/session-window.js';
  import MessageFilters from './MessageFilters.svelte';

  let { model, loadWindow, navigateTo = null } = $props();
  const uid = $props.id();
  let trigger = $state(null);
  let dialog = $state(null);
  let open = $state(false);
  let top = $state(60);
  let right = $state(12);
  const end = $derived(model.from + model.entries.length);
  const mode = $derived(t(`session.readingMode.${model.contentFilter.mode}`));

  function positionPanel() {
    const rect = trigger?.getBoundingClientRect();
    if (!rect) return;
    top = Math.round(rect.bottom + 10);
    right = Math.max(12, Math.round(window.innerWidth - rect.right));
  }

  function show() {
    positionPanel();
    dialog.showModal();
    dialog.querySelector('button')?.focus({ preventScroll: true });
    open = true;
    document.body.classList.add('reading-settings-open');
  }

  function closed() {
    open = false;
    document.body.classList.remove('reading-settings-open');
    if (trigger?.isConnected) trigger.focus({ preventScroll: true });
  }

  onMount(() => {
    // Native dialog supplies an inert background and Escape. Explicit Tab
    // cycling also works when Safari's default tab order skips buttons.
    const element = dialog;
    const onKey = (event) => {
      event.stopPropagation();
      if (event.key !== 'Tab') return;
      const controls = [...element.querySelectorAll('button:not(:disabled), input:not(:disabled)')];
      if (!controls.length) return;
      const index = controls.indexOf(document.activeElement);
      const next = (index + (event.shiftKey ? -1 : 1) + controls.length) % controls.length;
      event.preventDefault();
      controls[next].focus();
    };
    const onBackdrop = (event) => {
      if (event.target !== dialog) return;
      const rect = dialog.getBoundingClientRect();
      if (
        event.clientX < rect.left ||
        event.clientX > rect.right ||
        event.clientY < rect.top ||
        event.clientY > rect.bottom
      )
        dialog.close();
    };
    dialog.addEventListener('keydown', onKey);
    dialog.addEventListener('click', onBackdrop);
    window.addEventListener('resize', positionPanel);
    return () => {
      element.removeEventListener('keydown', onKey);
      element.removeEventListener('click', onBackdrop);
      window.removeEventListener('resize', positionPanel);
      document.body.classList.remove('reading-settings-open');
    };
  });

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
      // The shared loader keeps the old page and exposes a retryable error.
    }
  }
</script>

<!-- eslint-disable svelte/no-at-html-tags -- trusted: Lucide icon SVG -->
<button
  type="button"
  class="reading-settings-trigger"
  class:has-error={!!model.windowError}
  bind:this={trigger}
  aria-label={t('session.readingSettings')}
  title={t('session.readingSettingsSummary', { count: model.windowSize, mode })}
  aria-haspopup="dialog"
  aria-expanded={open}
  aria-controls={`${uid}-reading-panel`}
  onclick={show}
>
  {@html icon(SlidersHorizontal, { size: 16 })}
  <span class="reading-summary" aria-hidden="true"
    >{model.windowSize}<span class="summary-mode"> · {mode}</span></span
  >
</button>

<dialog
  class="reading-settings-dialog"
  id={`${uid}-reading-panel`}
  aria-labelledby={`${uid}-reading-title`}
  aria-modal="true"
  style:--reading-top={`${top}px`}
  style:--reading-right={`${right}px`}
  bind:this={dialog}
  onclose={closed}
>
  <div class="sheet-handle" aria-hidden="true"></div>
  <header class="reading-heading">
    <div>
      <h2 id={`${uid}-reading-title`}>{t('session.readingSettings')}</h2>
      <p>{t('session.readingSettingsDescription')}</p>
    </div>
    <button
      type="button"
      class="close-button"
      aria-label={t('common.close')}
      onclick={() => dialog.close()}
    >
      {@html icon(X, { size: 18 })}
    </button>
  </header>
  <div class="reading-body">
    <section class="window-size" aria-busy={model.windowBusy}>
      <h3>{t('session.windowSize')}</h3>
      <div class="size-options" role="group" aria-label={t('session.windowSize')}>
        {#each SESSION_WINDOW_SIZES as size (size)}
          <button
            type="button"
            aria-pressed={model.windowSize === size}
            disabled={model.windowBusy}
            onclick={() => selectWindow({ limit: size, before: null })}>{size}</button
          >
        {/each}
      </div>
      <p class="section-hint">{t('session.readingRecordHint')}</p>
    </section>
    <MessageFilters {model} />
    <section
      class="window-navigation"
      aria-busy={model.windowBusy}
      aria-label={t('session.readingHistory')}
    >
      <span class="range" aria-live="polite"
        >{t('session.windowRange', {
          from: model.entries.length ? model.from + 1 : 0,
          to: end,
          total: model.total,
        })}</span
      >
      <div class="page-actions">
        <button
          type="button"
          disabled={model.windowBusy || model.from === 0}
          onclick={() => selectWindow({ before: model.from })}
        >
          {@html icon(ChevronLeft, { size: 14 })}{t('session.windowEarlier')}
        </button>
        <button
          type="button"
          disabled={model.windowBusy || end >= model.total}
          onclick={() =>
            selectWindow({
              before: end + model.windowSize >= model.total ? null : end + model.windowSize,
            })}
        >
          {t('session.windowNewer')}{@html icon(ChevronRight, { size: 14 })}
        </button>
        <button
          type="button"
          class="latest"
          disabled={model.windowBusy || model.windowEnd === null}
          onclick={() => selectWindow({ before: null })}>{t('session.windowLatest')}</button
        >
      </div>
      {#if model.windowError}
        <div class="window-error" role="alert">
          {t('session.loadEarlierFailed', { error: model.windowError })}
          <button type="button" disabled={model.windowBusy} onclick={() => selectWindow({})}
            >{t('session.windowRetry')}</button
          >
        </div>
      {/if}
    </section>
  </div>
  <footer>{t('session.readingContextHint')}</footer>
</dialog>
