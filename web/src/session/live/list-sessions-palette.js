function debounce(fn, ms) {
  let timer;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

function renderResults(resultsEl, sessions, documentImpl, navigate) {
  if (sessions.length === 0) {
    resultsEl.innerHTML = '<div class="palette-empty">No sessions found</div>';
    return;
  }
  resultsEl.innerHTML = '';
  for (const session of sessions) {
    const btn = documentImpl.createElement('button');
    btn.type = 'button';
    btn.className = 'palette-result';
    const model = [session.ModelProvider, session.Model].filter(Boolean).join('/') || '';
    btn.innerHTML = `<span class="palette-result-title"></span><span class="palette-result-meta"></span>`;
    btn.querySelector('.palette-result-title').textContent = session.Name || session.ID;
    btn.querySelector('.palette-result-meta').textContent = model;
    btn.addEventListener('click', () => {
      navigate('/session?id=' + encodeURIComponent(session.ID));
    });
    resultsEl.appendChild(btn);
  }
}

function filterSessions(sessions, query) {
  if (!query) return sessions;
  const q = query.toLowerCase();
  return sessions.filter((s) => {
    const name = (s.Name || s.ID || '').toLowerCase();
    return name.includes(q);
  });
}

export function setupListSessionsPalette({
  documentImpl = document,
  windowImpl = window,
  fetchImpl = fetch,
  navigate = (url) => { windowImpl.location.href = url; },
  getCwd = () => {
    try {
      const preload = windowImpl.__piSessionDataModel;
      const data = preload && typeof preload.header === 'object' ? preload.header : {};
      return data.cwd || '';
    } catch {
      return '';
    }
  },
} = {}) {
  const overlay = documentImpl.getElementById('sessionPalette');
  const searchInput = documentImpl.getElementById('session-palette-search');
  const resultsEl = documentImpl.querySelector('[data-palette-results]');
  const closeBtns = Array.from(documentImpl.querySelectorAll('[data-palette-close]'));

  if (!overlay || !searchInput || !resultsEl) return { open: () => {}, close: () => {} };

  let allSessions = [];
  let open = false;
  let escapeHandler = null;
  let overlayClickHandler = null;

  function close() {
    if (!open) return;
    open = false;
    overlay.classList.remove('open');
    overlay.setAttribute('aria-hidden', 'true');
    documentImpl.body?.classList.remove('pi-palette-open');
    if (searchInput) searchInput.value = '';

    if (escapeHandler) {
      windowImpl.removeEventListener('keydown', escapeHandler);
      escapeHandler = null;
    }
    if (overlayClickHandler) {
      overlay.removeEventListener('click', overlayClickHandler);
      overlayClickHandler = null;
    }
  }

  async function openPalette() {
    if (open) return;
    open = true;
    overlay.classList.add('open');
    overlay.setAttribute('aria-hidden', 'false');
    documentImpl.body?.classList.add('pi-palette-open');

    // Register close handlers
    escapeHandler = (e) => {
      if (e.key === 'Escape') close();
    };
    windowImpl.addEventListener('keydown', escapeHandler);

    overlayClickHandler = (e) => {
      if (e.target === overlay) close();
    };
    overlay.addEventListener('click', overlayClickHandler);

    // Fetch sessions
    const cwd = getCwd();
    const url = cwd
      ? '/api/sessions?project=' + encodeURIComponent(cwd)
      : '/api/sessions';

    try {
      const response = await fetchImpl(url);
      const data = await response.json();
      allSessions = (data.sessions || []).sort((a, b) => {
        const da = a.LastActivity || '';
        const db = b.LastActivity || '';
        return db.localeCompare(da); // newest first
      });
      renderResults(resultsEl, allSessions, documentImpl, navigate);
    } catch {
      resultsEl.innerHTML = '<div class="palette-empty">Failed to load sessions</div>';
    }

    if (searchInput) searchInput.focus();
  }

  // Search filtering
  const debouncedFilter = debounce(() => {
    const query = searchInput ? searchInput.value : '';
    const filtered = filterSessions(allSessions, query);
    renderResults(resultsEl, filtered, documentImpl, navigate);
  }, 100);

  if (searchInput) {
    searchInput.addEventListener('input', debouncedFilter);
  }

  closeBtns.forEach((btn) => {
    btn.addEventListener('click', close);
  });

  return { open: openPalette, close };
}
