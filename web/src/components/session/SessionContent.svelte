<script>
  // The message pane: renders the active root→leaf path from the reactive model
  // as <SessionEntry> components, replacing the navigator's imperative #messages
  // build. Keyed by entry id so navigation and live reload add/update/remove
  // entries reactively. `afterRender(container)` runs after each (re)render to
  // re-apply toggle state, lazy-highlight pending code, and scroll — concerns the
  // imperative layer still owns. Shared by the live app + the static export.
  import { getSessionModel } from '../../session/session-context.js';
  import SessionEntry from './SessionEntry.svelte';
  import { buildContentToolIndex, filterContentEntry } from '../../session/content-filter.js';
  import { t } from '../../shared/i18n.js';

  let { model = getSessionModel(), afterRender = null, live = false } = $props();

  let containerEl = $state(null);
  const toolIndex = $derived(live ? buildContentToolIndex(model.entries) : null);
  const visibleEntries = $derived(
    live && model.contentFilter
      ? model.activePath
          .map((entry) => filterContentEntry(entry, model.contentFilter, toolIndex))
          .filter(Boolean)
      : model.activePath,
  );

  // Re-run post-render side effects whenever the rendered path changes.
  $effect(() => {
    visibleEntries;
    if (containerEl && typeof afterRender === 'function') {
      afterRender(containerEl);
    }
  });
</script>

<div id="messages-list" class="messages-list" bind:this={containerEl}>
  {#if live && model.activePath.length && !visibleEntries.length}
    <p role="status">{t('session.contentFilterEmpty')}</p>
  {/if}
  {#each visibleEntries as entry (entry.id)}
    <SessionEntry {entry} {model} {live} />
  {/each}
</div>
