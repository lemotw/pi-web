import { isDoneNotifyEnabled } from '../chat/done-notifier.js';

function applyTheme(windowImpl, next) {
  document.documentElement.dataset.theme = next || 'dark';
  try { localStorage.setItem('pi-web-theme', next); } catch (e) {}
  var meta = document.querySelector('meta[name="theme-color"]');
  if (meta) meta.content = (next || 'dark') === 'dark' ? '#0e0e13' : '#f8f8f8';
}

function toggleTheme(windowImpl) {
  var current = document.documentElement.dataset.theme || 'dark';
  var next = current === 'dark' ? 'light' : 'dark';
  applyTheme(windowImpl, next);
}

function showToast(message, documentImpl, windowImpl) {
  let notice = documentImpl.getElementById('command-menu-toast');
  if (!notice) {
    notice = documentImpl.createElement('div');
    notice.id = 'command-menu-toast';
    notice.className = 'toast-notice';
    documentImpl.body.appendChild(notice);
  }
  notice.textContent = message;
  notice.classList.add('visible');
  clearTimeout(showToast._timer);
  showToast._timer = windowImpl.setTimeout(() => {
    notice.classList.remove('visible');
  }, 1500);
}

function clickHiddenButton(id, documentImpl) {
  const btn = documentImpl.getElementById(id);
  if (btn) btn.click();
}

function isMobileLayout(windowImpl) {
  return windowImpl.matchMedia('(max-width: 900px)').matches;
}

export function setupCommandMenu({
  documentImpl = document,
  windowImpl = window,
  setSidebarOpen = null,
  setSidebarCollapsed = null,
} = {}) {
  const mobileBackdrop = documentImpl.getElementById('mobile-command-backdrop');
  const mobilePanel = documentImpl.getElementById('mobile-command-panel');
  const desktopPopover = documentImpl.getElementById('command-menu-popover');
  const menuBtn = documentImpl.getElementById('command-menu-btn');
  if (!menuBtn) return;

  let open = false;

  function syncNotifyToggle() {
    const enabled = isDoneNotifyEnabled({ storage: windowImpl.localStorage });
    const mobileStatus = documentImpl.getElementById('mobile-command-notify-status');
    const desktopStatus = documentImpl.getElementById('command-menu-notify-status');
    [mobileStatus, desktopStatus].forEach((el) => {
      if (!el) return;
      el.textContent = enabled ? 'ON' : 'OFF';
      el.classList.toggle('on', enabled);
    });
  }

  function openMobilePanel() {
    if (!mobileBackdrop || !mobilePanel) return;
    mobileBackdrop.style.display = '';
    mobilePanel.style.display = '';
    syncNotifyToggle();
    requestAnimationFrame(() => {
      mobileBackdrop.classList.add('open');
      mobilePanel.classList.add('open');
    });
  }

  function closeMobilePanel() {
    if (!mobileBackdrop || !mobilePanel) return;
    mobileBackdrop.classList.remove('open');
    mobilePanel.classList.remove('open');
    windowImpl.setTimeout(() => {
      if (!mobilePanel.classList.contains('open')) {
        mobileBackdrop.style.display = 'none';
        mobilePanel.style.display = 'none';
      }
    }, 260);
  }

  function openDesktopPopover() {
    if (!desktopPopover) return;
    syncNotifyToggle();
    desktopPopover.style.display = '';
    requestAnimationFrame(() => {
      desktopPopover.classList.add('open');
    });
  }

  function closeDesktopPopover() {
    if (!desktopPopover) return;
    desktopPopover.classList.remove('open');
    windowImpl.setTimeout(() => {
      if (!desktopPopover && !desktopPopover.classList.contains('open')) return;
      if (!desktopPopover.classList.contains('open')) {
        desktopPopover.style.display = 'none';
      }
    }, 160);
  }

  function openMenu() {
    open = true;
    if (isMobileLayout(windowImpl)) {
      openMobilePanel();
    } else {
      openDesktopPopover();
    }
  }

  function closeMenu() {
    open = false;
    closeMobilePanel();
    closeDesktopPopover();
  }

  menuBtn.addEventListener('click', (e) => {
    e.stopPropagation();
    if (open) { closeMenu(); } else { openMenu(); }
  });

  if (mobileBackdrop) {
    mobileBackdrop.addEventListener('click', closeMenu);
  }

  documentImpl.addEventListener('click', (e) => {
    if (!open) return;
    if (desktopPopover && desktopPopover.contains(e.target)) return;
    if (menuBtn && menuBtn.contains(e.target)) return;
    closeMenu();
  });

  documentImpl.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && open) {
      closeMenu();
    }
  });

  function handleAction(action) {
    switch (action) {
      case 'theme': {
        toggleTheme(windowImpl);
        showToast('Theme toggled', documentImpl, windowImpl);
        closeMenu();
        break;
      }
      case 'notifications': {
        clickHiddenButton('notify-toggle', documentImpl);
        syncNotifyToggle();
        windowImpl.setTimeout(syncNotifyToggle, 400);
        break;
      }
      case 'share': {
        clickHiddenButton('share-btn', documentImpl);
        closeMenu();
        break;
      }
      case 'new-session': {
        clickHiddenButton('new-btn', documentImpl);
        closeMenu();
        break;
      }
      case 'terminal': {
        clickHiddenButton('resume-btn', documentImpl);
        closeMenu();
        break;
      }
      case 'tree': {
        if (isMobileLayout(windowImpl)) {
          if (setSidebarOpen) setSidebarOpen(true);
        } else {
          if (setSidebarCollapsed) setSidebarCollapsed(false);
        }
        closeMenu();
        break;
      }
      case 'model-usage': {
        const header = documentImpl.getElementById('header-container');
        if (header) {
          header.scrollIntoView({ behavior: 'smooth', block: 'start' });
          header.classList.add('highlight');
          windowImpl.setTimeout(() => header.classList.remove('highlight'), 2000);
        }
        closeMenu();
        break;
      }
      case 'rename': {
        const titleEl = documentImpl.getElementById('session-header-title');
        const current = titleEl ? titleEl.textContent : '';
        const next = windowImpl.prompt('Rename session', current);
        if (next && next.trim() && next.trim() !== current) {
          if (titleEl) titleEl.textContent = next.trim();
          showToast('Renamed', documentImpl, windowImpl);
        }
        closeMenu();
        break;
      }
      case 'fork':
      case 'clone':
      case 'diff':
        showToast('Not yet implemented', documentImpl, windowImpl);
        closeMenu();
        break;
      default:
        break;
    }
  }

  const containers = [mobilePanel, desktopPopover].filter(Boolean);
  containers.forEach((container) => {
    container.addEventListener('click', (e) => {
      const item = e.target.closest('.mobile-command-item') || e.target.closest('.command-menu-item');
      if (!item) return;
      const action = item.dataset.action;
      if (!action) return;
      handleAction(action);
    });
  });
}