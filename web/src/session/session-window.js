export const SESSION_WINDOW_SIZES = [50, 100, 200];
const COOKIE = 'pi_session_window';

export function sessionWindowSize(value) {
  const size = Number(value);
  return SESSION_WINDOW_SIZES.includes(size) ? size : 100;
}

export function readSessionWindowSize(documentImpl = globalThis.document) {
  const value = documentImpl?.cookie?.split('; ').find((part) => part.startsWith(`${COOKIE}=`));
  return sessionWindowSize(value?.slice(COOKIE.length + 1));
}

export function saveSessionWindowSize(size, documentImpl = globalThis.document) {
  if (documentImpl) {
    documentImpl.cookie = `${COOKIE}=${sessionWindowSize(size)}; Path=/; Max-Age=31536000; SameSite=Lax`;
  }
}

export function sessionWindowUrl(sessionId, { limit, before = null, targetId, leafId } = {}) {
  const params = new URLSearchParams({ id: sessionId, paginate: '1' });
  params.set('limit', String(sessionWindowSize(limit ?? readSessionWindowSize())));
  if (Number.isInteger(before) && before >= 0) params.set('before', String(before));
  if (targetId) params.set('targetId', targetId);
  else if (leafId) params.set('leafId', leafId);
  return `/api/session?${params}`;
}

// Shared by paging controls and SSE. Coalesce identical requests and reject
// stale responses so a slow live refresh cannot undo a newer page selection.
export function createSessionWindowLoader({ model, sessionId, fetchImpl = fetch }) {
  let query = { limit: model.windowSize, before: model.windowEnd };
  let pending = null;
  let revision = 0;

  return function loadWindow(options = {}) {
    query = { ...query, ...options };
    const url = sessionWindowUrl(sessionId, query);
    if (pending?.url === url) return pending.promise;
    const request = ++revision;
    model.windowBusy = true;
    model.windowError = '';
    const promise = (async () => {
      try {
        const response = await fetchImpl(url, { headers: { Accept: 'application/json' } });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        if (request !== revision) return null;
        model.reconcileWindow(data);
        query = { limit: model.windowSize, before: model.windowEnd };
        return data;
      } catch (error) {
        if (request !== revision) return null;
        model.windowError = error.message || String(error);
        throw error;
      }
    })().finally(() => {
      if (request === revision) {
        model.windowBusy = false;
        pending = null;
      }
    });
    pending = { url, promise };
    return promise;
  };
}
