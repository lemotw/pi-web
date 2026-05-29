export function applyTheme(windowImpl, documentImpl, next) {
  documentImpl.documentElement.dataset.theme = next || 'dark';
  try { windowImpl.localStorage.setItem('pi-web-theme', next); } catch (e) {}
  const meta = documentImpl.querySelector('meta[name="theme-color"]');
  if (meta) meta.content = (next || 'dark') === 'dark' ? '#0e0e13' : '#f6f5f2';
}

export function toggleTheme(windowImpl, documentImpl) {
  const current = documentImpl.documentElement.dataset.theme || 'dark';
  const next = current === 'dark' ? 'light' : 'dark';
  applyTheme(windowImpl, documentImpl, next);
}

export function syncThemeIcons(documentImpl) {
  const isDark = (documentImpl.documentElement.dataset.theme || 'dark') === 'dark';
  documentImpl.querySelectorAll('[data-command-theme-icon]').forEach((el) => {
    el.textContent = isDark ? '☀' : '◐';
  });
}
