import { describe, expect, it, vi } from 'vitest';
import { JSDOM } from 'jsdom';
import { setupCommandMenu } from './command-menu.js';

function makeDom() {
  return new JSDOM(`<!doctype html><html><head><title>Old</title></head><body>
    <button id="command-menu-btn">⋯</button>
    <span id="session-header-title">Old</span>
    <div id="command-menu-popover">
      <button class="command-menu-item" data-action="rename">Rename</button>
      <button class="command-menu-item" data-action="model-usage">Model Usage</button>
    </div>
  </body></html>`, { url: 'http://localhost/session?id=session.jsonl' });
}

describe('setupCommandMenu rename', () => {
  it('persists rename through API before updating page title', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({
      ok: true,
      json: async () => ({ ok: true, name: 'New Name' }),
    }));
    dom.window.prompt = vi.fn(() => ' New Name ');
    dom.window.matchMedia = vi.fn(() => ({ matches: false }));
    dom.window.requestAnimationFrame = (fn) => fn();

    setupCommandMenu({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      fetchImpl,
      sessionId: 'session.jsonl',
    });

    dom.window.document.querySelector('[data-action="rename"]').click();
    await new Promise((resolve) => dom.window.setTimeout(resolve, 0));

    expect(fetchImpl).toHaveBeenCalledWith('/api/rename-session?id=session.jsonl', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name: 'New Name' }),
    });
    expect(dom.window.document.getElementById('session-header-title').textContent).toBe('New Name');
    expect(dom.window.document.title).toBe('New Name');
  });

  it('keeps old title when API rename fails', async () => {
    const dom = makeDom();
    const fetchImpl = vi.fn(async () => ({ ok: false, json: async () => ({ error: 'bad' }) }));
    dom.window.prompt = vi.fn(() => 'New Name');
    dom.window.matchMedia = vi.fn(() => ({ matches: false }));
    dom.window.requestAnimationFrame = (fn) => fn();

    setupCommandMenu({ documentImpl: dom.window.document, windowImpl: dom.window, fetchImpl, sessionId: 'session.jsonl' });
    dom.window.document.querySelector('[data-action="rename"]').click();
    await new Promise((resolve) => dom.window.setTimeout(resolve, 0));

    expect(dom.window.document.getElementById('session-header-title').textContent).toBe('Old');
    expect(dom.window.document.title).toBe('Old');
  });

  it('opens model usage from the detail page menu', () => {
    const dom = makeDom();
    dom.window.matchMedia = vi.fn(() => ({ matches: false }));
    dom.window.requestAnimationFrame = (fn) => fn();

    setupCommandMenu({
      documentImpl: dom.window.document,
      windowImpl: dom.window,
      getEntries: () => ([{
        type: 'message',
        message: {
          role: 'assistant',
          provider: 'p',
          model: 'm',
          usage: { input: 1000, output: 2000, cost: { input: 0.001, output: 0.002 } },
          content: [{ type: 'toolCall' }],
        },
      }]),
      escapeHtml: (s) => String(s),
      formatTokens: (n) => String(n),
    });

    expect(() => dom.window.document.querySelector('[data-action="model-usage"]').click()).not.toThrow();
    expect(dom.window.document.querySelector('.pi-sheet-panel')).toBeTruthy();
    expect(dom.window.document.querySelector('.pi-sheet-body').textContent).toContain('Total cost');
  });
});
