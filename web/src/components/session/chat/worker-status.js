import { t } from '../../../shared/i18n.js';

export function setupWorkerStatusPolling({
  windowImpl = window,
  chatApi,
  sessionId = '',
  setStatus = () => {},
  setModelLabel = () => {},
  setThinkingLabel = () => {},
  updateContextUsage = () => {},
  getKnownModelLabel = () => '',
  setKnownModelLabel = () => {},
  getKnownThinkingLevel = () => '',
  setKnownThinkingLevel = () => {},
  getWorkerModelUpdate = () => null,
  setIntervalImpl = windowImpl.setInterval?.bind(windowImpl),
  clearIntervalImpl = windowImpl.clearInterval?.bind(windowImpl),
  CustomEventImpl = windowImpl.CustomEvent,
  intervalMs = 1500,
} = {}) {
  let inflight = false;
  let pending = false;
  let lastWorkerState = null;
  let generation = 0;
  let disposed = false;

  async function refresh() {
    if (disposed) return;
    if (inflight) {
      // Queue exactly one follow-up so an in-flight response cannot swallow a
      // newer state change, such as the assistant finishing while we poll stale
      // "running" state.
      pending = true;
      return;
    }
    inflight = true;
    const requestGeneration = generation;
    try {
      const response = await chatApi?.getWorkerStatus?.(sessionId);
      if (!response?.ok) throw new Error('Worker status request failed');
      const data = await response.json();
      if (disposed) return;
      if (requestGeneration !== generation) {
        pending = true;
        return;
      }
      const apiModelLabel = data.model
        ? data.model + (data.modelProvider ? ' @ ' + data.modelProvider : '')
        : '';
      if (apiModelLabel) setKnownModelLabel(apiModelLabel);
      if (data.thinkingLevel) setKnownThinkingLevel(data.thinkingLevel);
      if (data.state === 'running') setStatus('running', 'running');
      if (data.state === 'idle') setStatus('idle', '');
      if (data.state === 'unknown') setStatus(t('composer.statusUnknown'), 'running');
      if (data.state === 'absent') setStatus(t('composer.workerAbsent'), '');
      if (data.state === 'error') setStatus(data.error || 'worker error', 'error');
      if (lastWorkerState === 'running' && data.state === 'idle') {
        try {
          windowImpl.dispatchEvent(new CustomEventImpl('pi-worker-done'));
        } catch {
          // Some tests/environments may not support constructing events.
        }
      }
      if (data.state && data.state !== 'unknown') lastWorkerState = data.state;
      setModelLabel(getKnownModelLabel());
      setThinkingLabel(getKnownThinkingLevel());
      updateContextUsage();
      const onWorkerModelUpdate = getWorkerModelUpdate?.();
      if (data.modelProvider && data.model && onWorkerModelUpdate) {
        onWorkerModelUpdate(data.modelProvider, data.model);
      }
    } catch {
      if (!disposed && requestGeneration === generation) {
        setStatus(t('composer.statusUnknown'), 'running');
      }
    } finally {
      inflight = false;
      if (pending) {
        pending = false;
        void refresh();
      }
    }
  }

  const interval = setIntervalImpl?.(refresh, intervalMs);
  void refresh();
  updateContextUsage();

  const onSessionReload = () => {
    void refresh();
    updateContextUsage();
  };
  const onMessageSent = () => {
    generation += 1;
  };
  windowImpl.addEventListener?.('pi-chat-message-sent', onMessageSent);
  windowImpl.addEventListener?.('pi-session-reload', onSessionReload);

  return {
    refresh,
    dispose: () => {
      disposed = true;
      if (interval != null) clearIntervalImpl?.(interval);
      windowImpl.removeEventListener?.('pi-chat-message-sent', onMessageSent);
      windowImpl.removeEventListener?.('pi-session-reload', onSessionReload);
    },
  };
}
