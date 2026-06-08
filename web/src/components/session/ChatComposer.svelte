<script module>
  // The chat composer runtime stays DI-friendly so tests can drive it directly
  // and inject selector implementations. Model selector wiring has been split
  // into components/session/chat/model-selector.js; the remaining selector
  // helpers below are still being extracted incrementally.
  import { icon, Maximize2 } from '../../shared/icons.js';
  import { t } from '../../shared/i18n.js';
  import {
    THINKING_LEVELS,
  } from '../../session/chat/chat-selectors.js';
  import { setupModelSelector, renderModelList } from './chat/model-selector.js';
  import { setupThinkingLevelSelector, renderThinkingLevelList } from './chat/thinking-selector.js';
  import {
    filterCommands,
    groupCommands,
    isPaletteCommand,
    parseSlashTrigger,
    renderCommandList,
    setupSlashCommands,
  } from './chat/slash-command.js';
  import {
    parseAtTrigger,
    renderFileList,
    setupMentionAutocomplete,
  } from './chat/mention-autocomplete.js';
  import { createContextUsageController } from './chat/context-usage.js';
  import { setupComposerExpansion } from './chat/composer-expand.js';
  import { setupWorkerStatusPolling } from './chat/worker-status.js';
  import { setupAskQuestionHandlers } from './chat/ask-question-handler.js';
  import { setupContextPopover } from './chat/context-popover.js';
  import { setupTextareaControls } from './chat/textarea-controls.js';
  import { setupAttachmentManager } from './chat/attachment-manager.js';
  import { setupCwdCopy } from './chat/cwd-copy.js';
  import { createChatToolbarState } from './chat/toolbar-state.js';
  import { setupChatSubmission } from './chat/chat-submit.js';
  import { createChatSelectorLoaders } from './chat/selector-loaders.js';

  export {
    setupModelSelector,
    renderModelList,
    setupThinkingLevelSelector,
    renderThinkingLevelList,
    filterCommands,
    groupCommands,
    isPaletteCommand,
    parseSlashTrigger,
    renderCommandList,
    setupSlashCommands,
    parseAtTrigger,
    renderFileList,
    setupMentionAutocomplete,
  };

export function runChatComposer({
  documentImpl = document,
  windowImpl = window,
  locationImpl = windowImpl.location,
  localEntries = [],
  leafId = '',
  urlTargetId = '',
  byId = new Map(),
  navigateTo = () => {},
  escapeHtml = (text) => String(text),
  chatApi,
  chatSelectors = { THINKING_LEVELS },
  modelSelector = { setupModelSelector },
  thinkingSelector = { setupThinkingLevelSelector },
  slashSelector = { setupSlashCommands },
  mentionSelector = { setupMentionAutocomplete },
  FormDataImpl = FormData,
  URLSearchParamsImpl = URLSearchParams,
  CustomEventImpl = CustomEvent,
  setIntervalImpl = setInterval
} = {}) {
  const document = documentImpl;
  const window = windowImpl;
  const location = locationImpl;
  const entries = localEntries;
  const __piChatApi = chatApi;
  const __piChatSelectors = chatSelectors;
  const __piModelSelector = modelSelector;
  const __piThinkingSelector = thinkingSelector;
  const __piSlashSelector = slashSelector;
  const __piMentionSelector = mentionSelector;
  const FormData = FormDataImpl;
  const URLSearchParams = URLSearchParamsImpl;
  const CustomEvent = CustomEventImpl;
  const setInterval = setIntervalImpl;
  let onWorkerModelUpdate = null;
  let knownModelLabel = '';
  let knownThinkingLevel = '';
  let currentModelForThinking = null;
  let positionPopover = () => {};
  const contextUsage = createContextUsageController({
    documentImpl: document,
    entries,
    chatApi,
    getKnownModelLabel: () => knownModelLabel,
    positionPopover: () => positionPopover(),
  });
  const updateContextUsage = () => contextUsage.update();

  function isMobileTextInputMode() {
    return !!(window.matchMedia && window.matchMedia('(hover: none) and (pointer: coarse)').matches);
  }

  const THINKING_LEVELS = __piChatSelectors.THINKING_LEVELS;
  const toolbarState = createChatToolbarState({
    documentImpl: document,
    isMobileTextInputMode,
    updateContextUsage,
  });
  const setChatStatus = toolbarState.setStatus;
  const setModelLabel = toolbarState.setModelLabel;
  const setThinkingLabel = toolbarState.setThinkingLabel;
  const selectorLoaders = createChatSelectorLoaders({
    documentImpl: document,
    windowImpl: window,
    locationImpl: location,
    URLSearchParamsImpl: URLSearchParams,
    entries,
    chatApi: __piChatApi,
    escapeHtml,
    modelSelector: __piModelSelector,
    thinkingSelector: __piThinkingSelector,
    slashSelector: __piSlashSelector,
    mentionSelector: __piMentionSelector,
    setModelLabel,
    setChatStatus,
    setThinkingLabel,
    setKnownModelLabel: (label) => { knownModelLabel = label; },
    getKnownModelLabel: () => knownModelLabel,
    setCurrentModelForThinking: (model) => { currentModelForThinking = model; },
    setWorkerModelUpdate: (handler) => { onWorkerModelUpdate = handler; },
    getCurrentModelForThinking: () => currentModelForThinking,
    getKnownThinkingLevel: () => knownThinkingLevel,
    setKnownThinkingLevel: (level) => { knownThinkingLevel = level; },
  });

  function setupPiChatComposer() {
    const form = document.getElementById('pi-chat-composer');
    if (!form) return false;
    const sessionId = form.dataset.sessionId;
    const chatAvailable = form.dataset.chatAvailable !== 'false';
    if (!chatAvailable) {
      const reason = form.dataset.chatDisabledReason || 'chat unavailable';
      setChatStatus('unavailable', 'error');
      form.title = reason;
      return false;
    }
    const textarea = document.getElementById('pi-chat-message');
    const fileInput = document.getElementById('pi-chat-images');
    const attachButton = document.getElementById('pi-chat-attach');
    const attachmentList = document.getElementById('pi-chat-attachments');
    const status = document.getElementById('pi-chat-status');
    const sendButton = document.getElementById('pi-chat-send');
    const cancelButton = document.getElementById('pi-chat-cancel');

    function updateComposerHeightVar() {
      const height = Math.ceil(form.getBoundingClientRect().height || 0);
      document.documentElement.style.setProperty('--pi-chat-composer-height', height + 'px');
    }

    updateComposerHeightVar();
    window.addEventListener('resize', updateComposerHeightVar, { passive: true });
    if (window.ResizeObserver) {
      new ResizeObserver(updateComposerHeightVar).observe(form);
    }

    // Expand/collapse the composer for larger typing area. State persists
    // per-session in localStorage.
    const shell = form.querySelector('.pi-chat-shell');
    let attachments = { hasAttachments: () => false };

    // Enable Send only when there is text or an attachment.
    function hasComposerContent() {
      const v = textarea ? textarea.value : '';
      return (v && v.trim().length > 0) || attachments.hasAttachments();
    }
    function updateSendEnabled() {
      if (!sendButton) return;
      // Don't fight transient sending/disabled state set by sendChatMessage.
      if (sendButton.dataset.sending === '1') return;
      sendButton.disabled = !hasComposerContent();
    }

    attachments = setupAttachmentManager({
      documentImpl: document,
      windowImpl: window,
      textarea,
      fileInput,
      attachButton,
      attachmentList,
      updateSendEnabled,
    });

    const textareaControls = setupTextareaControls({
      windowImpl: window,
      textarea,
      shell,
      form,
      isMobileTextInputMode,
      getSlashSelector: () => _slashSelectorApi,
      getMentionSelector: () => _mentionSelectorApi,
      getThinkingSelector: () => _thinkingSelectorApi,
      getModelSelector: () => _modelSelectorApi,
      updateSendEnabled,
      updateComposerHeight: updateComposerHeightVar,
    });
    const autoResizeTextarea = textareaControls.autoResize;


    let composerStorage = null;
    try {
      composerStorage = window.localStorage;
    } catch (_) {
      composerStorage = null;
    }
    setupComposerExpansion({
      sessionId,
      shell,
      expandButton: document.getElementById('pi-chat-expand'),
      textarea,
      storage: composerStorage,
      onHeightChange: updateComposerHeightVar,
    });

    function setStatus(text, cls) {
      setChatStatus(text, cls);
    }

    const submission = setupChatSubmission({
      windowImpl: window,
      form,
      textarea,
      sendButton,
      cancelButton,
      attachments,
      chatApi: __piChatApi,
      sessionId,
      setStatus,
      autoResizeTextarea,
      updateSendEnabled,
      FormDataImpl: FormData,
      CustomEventImpl: CustomEvent,
    });

    setupAskQuestionHandlers({ documentImpl: document, sendChatMessage: submission.sendChatMessage });

    const workerStatus = setupWorkerStatusPolling({
      windowImpl: window,
      chatApi: __piChatApi,
      sessionId,
      setStatus,
      setModelLabel,
      setThinkingLabel,
      updateContextUsage,
      getKnownModelLabel: () => knownModelLabel,
      setKnownModelLabel: (label) => { knownModelLabel = label; },
      getKnownThinkingLevel: () => knownThinkingLevel,
      setKnownThinkingLevel: (level) => { knownThinkingLevel = level; },
      getWorkerModelUpdate: () => onWorkerModelUpdate,
      setIntervalImpl: setInterval,
      CustomEventImpl: CustomEvent,
    });
    submission.setRefreshWorkerStatus(workerStatus.refresh);

    // Focus the message textarea on page load so the user can start typing immediately.
    if (textarea && typeof textarea.focus === 'function') {
      textarea.focus();
    }

    const contextPopover = setupContextPopover({ documentImpl: document, windowImpl: window, updateContextUsage });
    positionPopover = contextPopover.position;

    return true;
  }

  let _modelSelectorApi = null;
  let _thinkingSelectorApi = null;
  let _slashSelectorApi = null;
  let _mentionSelectorApi = null;

  function initPiChatControls() {
    setupCwdCopy({ documentImpl: document, windowImpl: window });
    if (!setupPiChatComposer()) return;

    toolbarState.updateInitialTooltips();

    _modelSelectorApi = selectorLoaders.loadModelSelector();
    _thinkingSelectorApi = selectorLoaders.loadThinkingSelector();
    _slashSelectorApi = selectorLoaders.loadSlashSelector();
    _mentionSelectorApi = selectorLoaders.loadMentionSelector();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initPiChatControls);
  } else {
    initPiChatControls();
  }

  // Initial render
  // If URL has targetId, scroll to that specific message; otherwise stay at top
  if (leafId) {
    if (urlTargetId && byId.has(urlTargetId)) {
      // Deep link: navigate to leaf and scroll to target message
      navigateTo(leafId, 'target', urlTargetId);
    } else {
      navigateTo(leafId, 'none');
    }
  } else if (entries.length > 0) {
    // Fallback: use last entry if no leafId
    navigateTo(entries[entries.length - 1].id, 'none');
  }
}

</script>

<script>
  import { onMount } from 'svelte';
  import { escapeHtml } from '../../session/render/session-format.js';
  import { getSessionRuntime } from '../../session/session-runtime-context.js';
  import * as chatApi from '../../session/chat/chat-api.js';
  import GitFooter from './GitFooter.svelte';
  import ChatToolbar from './chat/ChatToolbar.svelte';
  import ContextUsage from './chat/ContextUsage.svelte';
  import TextAttachmentModal from './chat/TextAttachmentModal.svelte';

  let {
    sessionId = '',
    chatAvailable = true,
    chatDisabledReason = '',
    cwd = '',
    modelLabel = '',
  } = $props();

  // The composer runtime lives in <script module> (runChatComposer). It reads the
  // shared model + navigateTo (owned by SessionPage runtime context) at mount —
  // both are ready before this onMount. <LiveReload> mounts first, so its
  // pi-chat-message-sent listener is attached before the user can send. Live-only.
  onMount(() => {
    const target = window;
    const runtime = getSessionRuntime();
    const model = runtime.model;
    globalThis.__PI_TEST_CHAT_COMPOSER_HOOK__?.();
    runChatComposer({
      documentImpl: document,
      windowImpl: target,
      locationImpl: target.location,
      localEntries: model?.entries || [],
      leafId: model?.leafId || '',
      urlTargetId: model?.urlTargetId || '',
      byId: model?.byId || new Map(),
      navigateTo: runtime.navigateTo,
      escapeHtml: (text) => escapeHtml(text, { documentImpl: document }),
      chatApi,
      FormDataImpl: target.FormData,
      URLSearchParamsImpl: target.URLSearchParams,
      CustomEventImpl: target.CustomEvent,
      setIntervalImpl: target.setInterval.bind(target),
    });
  });
</script>

<form id="pi-chat-composer" class="pi-chat-composer" data-session-id={sessionId} data-chat-available={chatAvailable} data-chat-disabled-reason={chatDisabledReason}>
  <input id="pi-chat-images" name="images" type="file" accept="image/*" multiple hidden disabled={!chatAvailable}>
  <div class="pi-chat-shell">
    <button type="button" id="pi-chat-expand" class="pi-chat-expand-button" title={t('composer.expand')} aria-label={t('composer.expand')} aria-pressed="false" disabled={!chatAvailable}>{@html icon(Maximize2, { size: 14 })}</button>
    {#if cwd}<div class="pi-chat-toolbar pi-chat-cwd-bar"><span class="pi-chat-cwd" title={t('composer.copyPath')} data-cwd={cwd}>cwd: {cwd}</span><span class="pi-chat-focus-shortcut">{t('composer.focusShortcut')}</span></div>{/if}
    {#if !chatAvailable}<div class="pi-chat-disabled-notice">{chatDisabledReason}</div>{/if}
    <textarea id="pi-chat-message" name="message" rows="1" placeholder={t('composer.placeholder')} disabled={!chatAvailable}></textarea>
    <div id="pi-chat-attachments" class="pi-chat-attachments"></div>
    <div id="pi-chat-model-popup" class="pi-chat-model-popup" style="display: none"><input type="text" id="pi-chat-model-search" class="pi-chat-model-search" placeholder={t('composer.searchModels')} autocomplete="off"><div id="pi-chat-model-list" class="pi-chat-model-list"></div></div>
    <div id="pi-chat-thinking-popup" class="pi-chat-thinking-popup" style="display: none"><div id="pi-chat-thinking-list" class="pi-chat-thinking-list"></div></div>
    <div id="pi-chat-slash-popup" class="pi-chat-slash-popup" style="display: none"><div id="pi-chat-slash-list" class="pi-chat-slash-list"></div></div>
    <div id="pi-chat-mention-popup" class="pi-chat-slash-popup" style="display: none"><div id="pi-chat-mention-list" class="pi-chat-slash-list"></div></div>
    <ChatToolbar {chatAvailable} {modelLabel} />
    <ContextUsage popover />
  </div>
  <TextAttachmentModal />
  <GitFooter {sessionId} />
</form>
