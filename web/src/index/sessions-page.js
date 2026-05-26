import { getJSON, postJSON } from '../shared/api.js';
import { createStatusEvents as defaultCreateStatusEvents } from '../shared/status-events.js';
import { renderSessionCard } from './session-card.js';

function defaultNavigate(url) {
  window.location = url;
}

function sessionCards(root = document) {
  return Array.from(root.querySelectorAll('.session-card[data-session-id]'));
}

function formatRunningModel(status) {
  if (!status || typeof status !== 'object') return '';
  const model = typeof status.modelName === 'string' && status.modelName ? status.modelName
    : typeof status.model === 'string' ? status.model : '';
  const provider = typeof status.modelProvider === 'string' ? status.modelProvider : '';
  if (model && provider) return `${provider}/${model}`;
  return model || provider;
}

function syncRunningCardClasses(runningSessionIds, runningStatuses = new Map(), root = document) {
  sessionCards(root).forEach((card) => {
    const id = card.dataset.sessionId;
    const running = !!id && runningSessionIds.has(id);
    card.classList.toggle('session-card--running', running);
    const model = running ? formatRunningModel(runningStatuses.get(id)) : '';
    const runningModelEl = card.querySelector('[data-running-model]');
    if (runningModelEl) runningModelEl.textContent = model;
    const sessionModelEl = card.querySelector('[data-session-model]');
    if (sessionModelEl && model) sessionModelEl.textContent = model;
  });
}

function normalizeSession(raw = {}) {
  return {
    id: raw.id || raw.ID || '',
    sessionUUID: raw.sessionUUID || raw.SessionUUID || '',
    project: raw.project || raw.Project || '',
    lastActivity: raw.lastActivity || raw.LastActivity || '',
    name: raw.name || raw.Name || '',
    messageCount: raw.messageCount ?? raw.MessageCount ?? 0,
    tokenTotal: raw.tokenTotal ?? raw.TokenTotal ?? 0,
    costTotal: raw.costTotal ?? raw.CostTotal ?? 0,
    model: raw.model || raw.Model || '',
    modelProvider: raw.modelProvider || raw.ModelProvider || '',
    chatAvailable: raw.chatAvailable ?? raw.ChatAvailable ?? true,
    chatDisabledReason: raw.chatDisabledReason || raw.ChatDisabledReason || ''
  };
}

function groupSessionsByProject(sessions = []) {
  const groups = [];
  let current = null;
  for (const session of sessions) {
    const project = session.project || '';
    if (!current || current.project !== project) {
      current = { project, sessions: [] };
      groups.push(current);
    }
    current.sessions.push(session);
  }
  return groups;
}

function renderSessionsList(sessions = [], root = document) {
  const container = root.querySelector('[data-sessions-content]');
  if (!container) return false;
  if (sessions.length === 0) {
    container.innerHTML = '<div class="empty-state"><h3>No sessions yet</h3><p>Start a new session to begin.</p></div>';
    return true;
  }
  container.innerHTML = groupSessionsByProject(sessions).map((group) => `
    <div class="project-group" data-project="${group.project.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')}">
      <button class="project-toggle" type="button" aria-expanded="true">
        <svg class="project-chevron" viewBox="0 0 12 12" aria-hidden="true"><path fill="currentColor" d="M3 4 L9 4 L6 8 Z"/></svg>
        <span class="project-name">${group.project.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;')}</span>
        <span class="project-count" data-project-count data-running="0" data-total="${group.sessions.length}">${group.sessions.length} sessions</span>
      </button>
      <div class="session-grid">${group.sessions.map(renderSessionCard).join('')}</div>
    </div>`).join('');
  root.dispatchEvent(new CustomEvent('pi-index-sessions-rendered', { bubbles: true }));
  return true;
}

function filterSessionCards(query, root = document) {
  const q = query.toLowerCase();
  root.querySelectorAll('.session-card').forEach((card) => {
    const match = (card.dataset.search || '').toLowerCase().includes(q);
    card.classList.toggle('hidden', !match);
  });
  root.querySelectorAll('.project-group').forEach((group) => {
    const anyVisible = group.querySelector('.session-card:not(.hidden)') !== null;
    group.style.display = anyVisible ? '' : 'none';
  });
}

export function createSessionsPage({
  root = document,
  fetchRecent = () => getJSON('/api/recent-locations'),
  fetchSessions = () => getJSON('/api/sessions'),
  createSession = (path) => postJSON('/api/new-session', { path }),
  createStatusEvents = defaultCreateStatusEvents,
  navigate = defaultNavigate,
  setTimeoutImpl = setTimeout,
  clearTimeoutImpl = clearTimeout
} = {}) {
  return {
    query: '',
    modal: false,
    path: '',
    recent: [],
    creating: false,
    error: '',
    runningSessionIds: new Set(),
    runningStatuses: new Map(),
    _statusEvents: null,
    _reloadTimer: null,
    _refreshInflight: false,

    sessionCards() {
      return sessionCards(root);
    },

    syncRunningCardClasses() {
      syncRunningCardClasses(this.runningSessionIds, this.runningStatuses, root);
    },

    isSessionRunning(id) {
      return this.runningSessionIds.has(id);
    },

    setRunningSessions(snapshot) {
      const ids = Array.isArray(snapshot) ? snapshot : snapshot?.ids;
      const statuses = snapshot && !Array.isArray(snapshot) ? snapshot.statuses : {};
      this.runningSessionIds = new Set(Array.isArray(ids) ? ids : []);
      this.runningStatuses = new Map(Object.entries(statuses || {}));
      this.syncRunningCardClasses();
    },

    setSessionRunning(id, running, status = {}) {
      if (running) {
        this.runningSessionIds.add(id);
        this.runningStatuses.set(id, status);
      } else {
        this.runningSessionIds.delete(id);
        this.runningStatuses.delete(id);
      }
      this.syncRunningCardClasses();
    },

    applySnapshot(data) {
      try {
        const payload = JSON.parse(data);
        this.setRunningSessions(payload?.running || []);
      } catch {
        /* malformed snapshot — ignore */
      }
    },

    applyDelta(data) {
      try {
        const payload = JSON.parse(data);
        if (!payload || typeof payload.id !== 'string') return;
        this.setSessionRunning(payload.id, !!payload.running);
      } catch {
        /* malformed delta — ignore */
      }
    },

    async refreshSessions() {
      if (this._refreshInflight) return;
      if (this.modal) return;
      this._refreshInflight = true;
      try {
        const response = await fetchSessions();
        const sessions = (response.sessions || []).map(normalizeSession);
        renderSessionsList(sessions, root);
        this.syncRunningCardClasses();
        this.filter();
      } catch {
        // Keep the existing server-rendered list if soft refresh fails.
      } finally {
        this._refreshInflight = false;
      }
    },

    scheduleReload() {
      if (this._reloadTimer) {
        clearTimeoutImpl(this._reloadTimer);
        this._reloadTimer = null;
      }
      if (this.modal) return;
      if (this.query && this.query.trim() !== '') return;
      this._reloadTimer = setTimeoutImpl(() => {
        this._reloadTimer = null;
        this.refreshSessions();
      }, 500);
    },

    cleanup() {
      if (this._reloadTimer) {
        clearTimeoutImpl(this._reloadTimer);
        this._reloadTimer = null;
      }
      if (this._statusEvents) {
        this._statusEvents.cleanup();
        this._statusEvents = null;
      }
    },

    subscribe() {
      try {
        this.cleanup();
        this._statusEvents = createStatusEvents({
          onSnapshot: (snapshot) => this.setRunningSessions(snapshot),
          onDelta: (status) => this.setSessionRunning(status.id, status.running, status),
          onMessage: (message) => {
            if (message === 'new-session') this.refreshSessions();
            if (message === 'reload') this.scheduleReload();
          }
        });
        this._statusEvents.connect();
      } catch {
        /* EventSource unavailable — page degrades to no live status */
      }
    },

    filter() {
      filterSessionCards(this.query, root);
    },

    async openModal() {
      this.modal = true;
      this.path = '';
      this.error = '';
      this.recent = [];
      try {
        const response = await fetchRecent();
        this.recent = (response.locations || []).slice(0, 10);
      } catch {
        // Intentional no-op: recent locations are optional.
      }
      return this.recent;
    },

    async create() {
      const p = this.path.trim();
      if (!p) {
        this.error = 'Please enter a path';
        return;
      }
      this.creating = true;
      this.error = '';
      try {
        const response = await createSession(p);
        if (response.ok && response.id) {
          navigate('/session?id=' + encodeURIComponent(response.id));
          return;
        }
        this.error = response.error || 'Failed to create session';
      } catch (error) {
        this.error = error.message || 'Network error';
      } finally {
        this.creating = false;
      }
    }
  };
}
