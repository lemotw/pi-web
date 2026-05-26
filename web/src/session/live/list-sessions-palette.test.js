import { describe, expect, it, vi } from 'vitest';
import { JSDOM } from 'jsdom';
import { setupListSessionsPalette } from './list-sessions-palette.js';

function makeDom() {
  return new JSDOM(`<!doctype html><html><head><title>Test</title></head><body>
    <div class="command-palette-overlay" id="sessionPalette" aria-hidden="true">
      <div class="command-palette" role="dialog" aria-modal="true" aria-label="List sessions">
        <input type="text" id="session-palette-search" placeholder="Search sessions..." autocomplete="off">
        <div class="palette-results" data-palette-results></div>
      </div>
    </div>
  </body></html>`, { url: 'http://localhost/session?id=s.jsonl' });
}

describe('setupListSessionsPalette', () => {
  it('fetches sessions with project filter on open', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        sessions: [
          { ID: 'a.jsonl', Name: 'Alpha', Project: '/home/user/proj', Model: 'gpt', ModelProvider: 'openai', LastActivity: '2026-01-01T00:00:00Z' },
          { ID: 'b.jsonl', Name: 'Beta', Project: '/home/user/proj', Model: 'claude', ModelProvider: 'anthropic', LastActivity: '2026-01-02T00:00:00Z' },
        ]
      }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/home/user/proj',
    });

    await palette.open();

    expect(fetchImpl).toHaveBeenCalledWith('/api/sessions?project=%2Fhome%2Fuser%2Fproj');
  });

  it('renders session results in the palette', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        sessions: [
          { ID: 'a.jsonl', Name: 'Alpha session', Project: '/proj', Model: 'gpt', ModelProvider: 'openai', LastActivity: '2026-01-01T00:00:00Z' },
        ]
      }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/proj',
    });

    await palette.open();

    const results = dom.window.document.querySelector('[data-palette-results]');
    expect(results.children.length).toBe(1);
    expect(results.children[0].querySelector('.palette-result-title').textContent).toBe('Alpha session');
    expect(results.children[0].querySelector('.palette-result-meta').textContent).toContain('openai/gpt');
  });

  it('shows empty state when no sessions match', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({ sessions: [] }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/empty',
    });

    await palette.open();

    const results = dom.window.document.querySelector('[data-palette-results]');
    expect(results.innerHTML).toContain('palette-empty');
  });

  it('opens and closes the overlay', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({ sessions: [] }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/proj',
    });

    const overlay = dom.window.document.getElementById('sessionPalette');

    await palette.open();
    expect(overlay.classList.contains('open')).toBe(true);
    expect(overlay.getAttribute('aria-hidden')).toBe('false');

    palette.close();
    expect(overlay.classList.contains('open')).toBe(false);
    expect(overlay.getAttribute('aria-hidden')).toBe('true');
  });

  it('filters results by search text', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        sessions: [
          { ID: 'a.jsonl', Name: 'Alpha', Project: '/proj', Model: '', ModelProvider: '', LastActivity: '' },
          { ID: 'b.jsonl', Name: 'Beta session', Project: '/proj', Model: '', ModelProvider: '', LastActivity: '' },
          { ID: 'c.jsonl', Name: 'Gamma', Project: '/proj', Model: '', ModelProvider: '', LastActivity: '' },
        ]
      }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/proj',
    });

    await palette.open();

    const searchInput = dom.window.document.getElementById('session-palette-search');
    searchInput.value = 'Beta';
    searchInput.dispatchEvent(new dom.window.Event('input'));

    // Wait for debounce (100ms)
    await new Promise((r) => dom.window.setTimeout(r, 150));

    const results = dom.window.document.querySelector('[data-palette-results]');
    expect(results.children.length).toBe(1);
    expect(results.children[0].querySelector('.palette-result-title').textContent).toBe('Beta session');
  });

  it('navigates to session on click', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        sessions: [
          { ID: 'target.jsonl', Name: 'Target', Project: '/proj', Model: '', ModelProvider: '', LastActivity: '' },
        ]
      }),
    }));

    const navigate = vi.fn();

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      navigate,
      getCwd: () => '/proj',
    });

    await palette.open();

    const result = dom.window.document.querySelector('.palette-result');
    result.click();

    expect(navigate).toHaveBeenCalledWith('/session?id=target.jsonl');
  });

  it('closes on Escape key', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({ sessions: [] }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/proj',
    });

    await palette.open();

    dom.window.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Escape' }));
    expect(dom.window.document.getElementById('sessionPalette').classList.contains('open')).toBe(false);
  });

  it('closes on overlay click', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({ sessions: [] }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/proj',
    });

    await palette.open();

    const overlay = dom.window.document.getElementById('sessionPalette');
    overlay.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
    expect(overlay.classList.contains('open')).toBe(false);
  });

  it('does not close when clicking inside dialog', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({ sessions: [] }),
    }));

    const palette = setupListSessionsPalette({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      getCwd: () => '/proj',
    });

    await palette.open();

    const dialog = dom.window.document.querySelector('.command-palette');
    dialog.dispatchEvent(new dom.window.MouseEvent('click', { bubbles: true }));
    expect(dom.window.document.getElementById('sessionPalette').classList.contains('open')).toBe(true);
  });
});
