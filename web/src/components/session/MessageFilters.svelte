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

<section class="message-filters" aria-label={t('session.contentFilterGroup')}>
  <h3>{t('session.contentFilterLabel')}</h3>
  <div class="mode-options" role="group" aria-label={t('session.contentFilterLabel')}>
    {#each ['all', 'text', 'custom'] as mode (mode)}
      <button
        type="button"
        aria-pressed={model.contentFilter.mode === mode}
        onclick={() =>
          update({
            mode,
            showThinking:
              mode === 'text' ? false : mode === 'all' ? true : model.contentFilter.showThinking,
          })}
      >
        {t(`session.readingMode.${mode}`)}
      </button>
    {/each}
  </div>
  {#if model.contentFilter.mode === 'custom'}
    <fieldset class="tools">
      <legend>{t('session.contentFilterTools')}</legend>
      <div class="tool-options">
        {#each tools as name (name)}
          <label
            class="tool-option"
            class:excluded={model.contentFilter.hiddenTools.includes(name)}
          >
            <input
              type="checkbox"
              checked={!model.contentFilter.hiddenTools.includes(name)}
              onchange={(event) => toggleTool(name, event.currentTarget.checked)}
            />
            <span>{name}</span>
          </label>
        {/each}
        {#if !tools.length}<span class="empty-tools">{t('session.contentFilterNoTools')}</span>{/if}
      </div>
    </fieldset>
  {/if}
  <label class="thinking-option">
    <span>{t('session.contentFilterThinking')}</span>
    <input
      type="checkbox"
      role="switch"
      checked={model.contentFilter.showThinking}
      onchange={(event) => update({ showThinking: event.currentTarget.checked })}
    />
    <span class="switch-track" aria-hidden="true"></span>
  </label>
  {#if model.contentFilter.mode !== 'all'}<p class="filter-hint">
      {t('session.contentFilterHint')}
    </p>{/if}
</section>
