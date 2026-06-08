<script>
  import { icon, Paperclip } from '../../../shared/icons.js';
  import { t } from '../../../shared/i18n.js';
  import ContextUsage from './ContextUsage.svelte';

  let {
    chatAvailable = true,
    modelLabel = '',
  } = $props();
</script>

<div class="pi-chat-toolbar">
  <div class="pi-chat-toolbar-left">
    <button type="button" id="pi-chat-attach" class="pi-chat-icon-button pi-chat-photo-button" title={t('composer.attachPhotos')} aria-label={t('composer.attachPhotos')} disabled={!chatAvailable}>{@html icon(Paperclip, { size: 15 })}</button>
    <span id="pi-chat-status" class="pi-chat-status">{chatAvailable ? t('composer.idle') : t('composer.unavailable')}</span>
    <button type="button" id="pi-chat-thinking-label" class="pi-chat-thinking-label" style="display: none" title={t('composer.switchEffort')} disabled={!chatAvailable}></button>
    <button type="button" id="pi-chat-model-label" class="pi-chat-model-label" title={t('composer.switchModel')} style:display={modelLabel ? '' : 'none'} disabled={!chatAvailable}>{modelLabel}</button>
    <ContextUsage />
  </div>
  <div class="actions">
    <button type="button" id="pi-chat-cancel" class="pi-chat-cancel" style="display: none" title={t('composer.cancelRunning')} aria-label={t('composer.cancelRunning')} disabled={!chatAvailable}>{t('composer.cancel')}</button>
    <button type="submit" id="pi-chat-send" class="pi-chat-send" disabled>{t('composer.send')}</button>
  </div>
</div>
