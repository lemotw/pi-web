<script>
  import { t } from '../../shared/i18n.js';
  import { buildContentToolIndex } from '../../session/content-filter.js';
  import { saveContentFilter } from '../../session/content-filter-preferences.js';
  import { sessionRuntime } from '../../session/session-runtime.js';

  let { model } = $props();
  const tools = $derived(buildContentToolIndex(model.entries).names);

  function update(changes) {
    model.contentFilter = { ...model.contentFilter, ...changes };
    saveContentFilter(model.contentFilter);
    const toggles = sessionRuntime.toggleState;
    if (model.contentFilter.mode !== 'text' && toggles && !toggles.toolsVisible)
      toggles.toggleToolsVisibility();
    if (model.contentFilter.showThinking && toggles && !toggles.thinkingExpanded)
      toggles.toggleThinking();
  }

  function toggleTool(name, visible) {
    const hiddenTools = model.contentFilter.hiddenTools.filter((tool) => tool !== name);
    if (!visible) hiddenTools.push(name);
    update({ hiddenTools });
  }
</script>

<div class="message-filters" role="group" aria-label={t('session.contentFilterGroup')}>
  <label>
    {t('session.contentFilterLabel')}
    <select
      aria-label={t('session.contentFilterLabel')}
      value={model.contentFilter.mode}
      onchange={(event) => {
        const mode = event.currentTarget.value;
        update({
          mode,
          showThinking:
            mode === 'text' ? false : mode === 'all' ? true : model.contentFilter.showThinking,
        });
      }}
    >
      <option value="all">{t('session.contentFilterAll')}</option>
      <option value="text">{t('session.contentFilterText')}</option>
      <option value="custom">{t('session.contentFilterCustom')}</option>
    </select>
  </label>
  <label>
    <input
      type="checkbox"
      checked={model.contentFilter.showThinking}
      onchange={(event) => update({ showThinking: event.currentTarget.checked })}
    />
    {t('session.contentFilterThinking')}
  </label>
  {#if model.contentFilter.mode === 'custom'}
    <details open>
      <summary>{t('session.contentFilterTools')}</summary>
      <div class="tools">
        {#each tools as name (name)}
          <label
            ><input
              type="checkbox"
              checked={!model.contentFilter.hiddenTools.includes(name)}
              onchange={(event) => toggleTool(name, event.currentTarget.checked)}
            />{name}</label
          >
        {/each}
        {#if !tools.length}<span>{t('session.contentFilterNoTools')}</span>{/if}
      </div>
    </details>
  {/if}
  {#if model.contentFilter.mode !== 'all'}<small>{t('session.contentFilterHint')}</small>{/if}
</div>

<style>
  .message-filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 0.75rem;
    padding: 0.5rem 0;
    color: var(--text-soft);
    font-size: 0.8rem;
  }
  label {
    display: flex;
    align-items: center;
    gap: 0.4rem;
  }
  select {
    font: inherit;
    color: var(--text);
    background: var(--surface-2);
    border: 1px solid var(--dim);
    border-radius: 6px;
    padding: 0.35rem;
    max-width: 100%;
  }
  details,
  small {
    flex-basis: 100%;
  }
  summary {
    cursor: pointer;
  }
  .tools {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem 1rem;
    padding: 0.5rem 0;
    overflow-wrap: anywhere;
  }
</style>
