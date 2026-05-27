import { createSessionsPage } from './sessions-page.js';
import {
  isDoneNotifyEnabled,
  registerPushSubscription,
  requestNotifyPermission,
  setDoneNotifyEnabled,
  unregisterPushSubscription,
} from '../session/chat/done-notifier.js';
import { setupKeyboardNav } from '../shared/keyboard-nav.js';

export { createSessionsPage };

function debounce(fn, ms) {
  let timer;
  return (...args) => {
    clearTimeout(timer);
    timer = setTimeout(() => fn(...args), ms);
  };
}

export function runIndexPage({
  documentImpl = document,
  windowImpl = window,
  setTimeoutImpl = setTimeout,
  clearTimeoutImpl = clearTimeout,
} = {}) {
  const page = createSessionsPage({
    root: documentImpl,
    setTimeoutImpl,
    clearTimeoutImpl,
  });

  setupKeyboardNav({ windowImpl, documentImpl });

  const searchInput = documentImpl.getElementById('search');
  const openSearchBtn = documentImpl.getElementById('open-search');
  const paletteOverlay = documentImpl.getElementById('commandPalette');
  const paletteResults = documentImpl.querySelector('[data-palette-results]');
  const paletteCloseBtns = Array.from(documentImpl.querySelectorAll('[data-palette-close]'));
  const menuBtn = documentImpl.getElementById('web-menu-btn');
  const webMenu = documentImpl.getElementById('web-menu');
  const modalOverlay = documentImpl.getElementById('modalOverlay');
  const newSessionBtns = Array.from(documentImpl.querySelectorAll('[data-new-session-btn]'));
  const modalBackBtn = documentImpl.getElementById('modalBackBtn');
  const cancelBtn = documentImpl.getElementById('cancelBtn');
  const createBtn = documentImpl.getElementById('createBtn');
  const sessionPathInput = documentImpl.getElementById('sessionPath');
  const recentLocations = documentImpl.getElementById('recentLocations');
  const modalError = documentImpl.getElementById('modalError');
  const notifyToggle = documentImpl.getElementById('index-notify-toggle');
  const notifyStatus = documentImpl.getElementById('index-notify-status');
  const layoutBtns = Array.from(documentImpl.querySelectorAll('[data-layout-btn]'));
  const layoutStorageKey = 'pi-sessions:view-layout';

  let modalHideTimer = null;

  function showModal() {
    closePalette();
    closeMenu();
    if (!modalOverlay) return;
    if (modalHideTimer) {
      clearTimeoutImpl(modalHideTimer);
      modalHideTimer = null;
    }
    modalOverlay.classList.add('visible');
    documentImpl.body?.classList.add('modal-sheet-open');
    const requestFrame = windowImpl.requestAnimationFrame?.bind(windowImpl) || ((fn) => setTimeoutImpl(fn, 0));
    requestFrame(() => modalOverlay.classList.add('open'));
  }

  function hideModal() {
    if (modalOverlay) {
      modalOverlay.classList.remove('open');
      if (modalHideTimer) clearTimeoutImpl(modalHideTimer);
      modalHideTimer = setTimeoutImpl(() => {
        modalOverlay.classList.remove('visible');
        modalHideTimer = null;
      }, 300);
    }
    documentImpl.body?.classList.remove('modal-sheet-open');
    page.modal = false;
  }

  function closeMenu() {
    if (webMenu) webMenu.hidden = true;
    if (menuBtn) menuBtn.setAttribute('aria-expanded', 'false');
  }

  function toggleMenu() {
    if (!webMenu || !menuBtn) return;
    const willOpen = webMenu.hidden;
    webMenu.hidden = !willOpen;
    menuBtn.setAttribute('aria-expanded', willOpen ? 'true' : 'false');
  }

  function setLayoutButtonState(layout) {
    documentImpl.documentElement.dataset.sessionLayout = layout;
    layoutBtns.forEach((btn) => {
      btn.setAttribute('aria-pressed', btn.dataset.layoutBtn === layout ? 'true' : 'false');
    });
  }

  function markLayoutReady() {
    documentImpl.querySelector('[data-sessions-content]')?.classList.add('index-layout-ready');
  }

  function visibleSessionCards() {
    return Array.from(documentImpl.querySelectorAll('.session-card[data-session-id]:not(.hidden)'));
  }

  function updatePaletteResults() {
    if (!paletteResults) return;
    const cards = visibleSessionCards().slice(0, 8);
    if (cards.length === 0) {
      paletteResults.innerHTML = '<div class="palette-empty">No sessions found</div>';
      return;
    }
    paletteResults.innerHTML = '';
    for (const card of cards) {
      const btn = documentImpl.createElement('button');
      btn.type = 'button';
      btn.className = 'palette-result';
      const title = card.querySelector('.session-title')?.textContent?.trim() || card.dataset.sessionId || 'Session';
      const meta = card.querySelector('[data-session-model]')?.textContent?.trim()
        || card.querySelector('.session-time')?.textContent?.trim()
        || '';
      btn.innerHTML = `<span class="palette-result-title"></span><span class="palette-result-meta"></span>`;
      btn.querySelector('.palette-result-title').textContent = title;
      btn.querySelector('.palette-result-meta').textContent = meta;
      btn.addEventListener('click', () => {
        const href = card.getAttribute('href');
        if (href) windowImpl.location.href = href;
      });
      paletteResults.appendChild(btn);
    }
  }

  function openPalette() {
    closeMenu();
    if (!paletteOverlay) return;
    paletteOverlay.classList.add('open');
    paletteOverlay.setAttribute('aria-hidden', 'false');
    documentImpl.body?.classList.add('pi-palette-open');
    updatePaletteResults();
    if (searchInput) searchInput.focus();
  }

  function closePalette() {
    if (!paletteOverlay) return;
    paletteOverlay.classList.remove('open');
    paletteOverlay.setAttribute('aria-hidden', 'true');
    documentImpl.body?.classList.remove('pi-palette-open');
  }

  if (searchInput) {
    const debouncedFilter = debounce(() => {
      page.query = searchInput.value;
      page.filter();
      updatePaletteResults();
    }, 50);
    searchInput.addEventListener('input', debouncedFilter);
  }

  if (openSearchBtn) {
    openSearchBtn.addEventListener('click', openPalette);
  }

  paletteCloseBtns.forEach((btn) => {
    btn.addEventListener('click', closePalette);
  });

  layoutBtns.forEach((btn) => {
    btn.addEventListener('click', async () => {
      const layout = btn.dataset.layoutBtn === 'projects' ? 'projects' : 'timeline';
      try { windowImpl.localStorage.setItem(layoutStorageKey, layout); } catch (_) {}
      setLayoutButtonState(layout);
      await page.setLayout(layout);
      markLayoutReady();
      updatePaletteResults();
    });
  });

  if (paletteOverlay) {
    paletteOverlay.addEventListener('click', (e) => {
      if (e.target === paletteOverlay) closePalette();
    });
  }

  if (menuBtn) {
    menuBtn.addEventListener('click', (e) => {
      e.stopPropagation();
      toggleMenu();
    });
  }

  if (webMenu) {
    webMenu.addEventListener('click', (e) => e.stopPropagation());
    windowImpl.addEventListener('click', closeMenu);
  }

  function syncNotifyMenuItem() {
    if (!notifyToggle || !notifyStatus) return;
    const enabled = isDoneNotifyEnabled({ storage: windowImpl.localStorage });
    notifyStatus.textContent = enabled ? 'ON' : 'OFF';
    notifyStatus.classList.toggle('on', enabled);
    notifyToggle.setAttribute('aria-pressed', enabled ? 'true' : 'false');
  }

  if (notifyToggle) {
    syncNotifyMenuItem();
    notifyToggle.addEventListener('click', async () => {
      const enabled = isDoneNotifyEnabled({ storage: windowImpl.localStorage });
      if (enabled) {
        setDoneNotifyEnabled(false, { storage: windowImpl.localStorage });
        await unregisterPushSubscription({ windowImpl });
      } else {
        const permission = await requestNotifyPermission({ windowImpl });
        const granted = permission === 'granted';
        setDoneNotifyEnabled(granted, { storage: windowImpl.localStorage });
        if (granted) await registerPushSubscription({ windowImpl });
      }
      syncNotifyMenuItem();
    });
  }

  async function openNewSessionModal() {
    showModal();
    page.modal = true;
    const recent = await page.openModal();
    if (recentLocations) {
      recentLocations.innerHTML = '';
      for (const loc of recent) {
        const chip = documentImpl.createElement('span');
        chip.className = 'recent-chip';
        chip.textContent = loc;
        chip.addEventListener('click', () => {
          if (sessionPathInput) {
            sessionPathInput.value = loc;
            page.path = loc;
            sessionPathInput.focus();
          }
        });
        recentLocations.appendChild(chip);
      }
    }
    if (sessionPathInput) sessionPathInput.focus();
  }

  newSessionBtns.forEach((btn) => {
    btn.addEventListener('click', openNewSessionModal);
  });

  if (modalBackBtn) {
    modalBackBtn.addEventListener('click', hideModal);
  }

  if (cancelBtn) {
    cancelBtn.addEventListener('click', hideModal);
  }

  if (modalOverlay) {
    modalOverlay.addEventListener('click', (e) => {
      if (e.target === modalOverlay) hideModal();
    });
  }

  windowImpl.addEventListener('keydown', (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
      e.preventDefault();
      openPalette();
      return;
    }
    if (e.key === 'Escape') {
      if (paletteOverlay?.classList.contains('open')) closePalette();
      else if (webMenu && !webMenu.hidden) closeMenu();
      else if (page.modal) hideModal();
    }
  });

  if (sessionPathInput) {
    sessionPathInput.addEventListener('input', () => {
      page.path = sessionPathInput.value;
    });
    sessionPathInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        doCreate();
      }
    });
  }

  async function doCreate() {
    if (createBtn) {
      createBtn.disabled = true;
      createBtn.textContent = 'Creating...';
    }
    if (modalError) modalError.textContent = '';

    await page.create();

    if (createBtn) {
      createBtn.disabled = false;
      createBtn.textContent = 'Create';
    }
    if (modalError) modalError.textContent = page.error || '';
  }

  if (createBtn) {
    createBtn.addEventListener('click', doCreate);
  }

  let initialLayout = 'timeline';
  try {
    initialLayout = windowImpl.localStorage.getItem(layoutStorageKey) === 'projects' ? 'projects' : 'timeline';
  } catch (_) {}
  setLayoutButtonState(initialLayout);

  page.filter();
  updatePaletteResults();
  page.subscribe();
  if (initialLayout === 'projects') {
    page.setLayout('projects')
      .then(updatePaletteResults)
      .catch(() => {})
      .finally(markLayoutReady);
  } else {
    markLayoutReady();
  }
}

if (typeof window !== 'undefined' && typeof document !== 'undefined' && document.getElementById('search')) {
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => runIndexPage());
  } else {
    runIndexPage();
  }
}
