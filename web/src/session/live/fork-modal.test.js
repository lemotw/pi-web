import { describe, expect, it, vi } from 'vitest';
import { JSDOM } from 'jsdom';
import { showForkModal } from './fork-modal.js';

function makeDom() {
  const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'http://localhost/session?id=s.jsonl' });
  dom.window.matchMedia = vi.fn(() => ({ matches: false }));
  dom.window.requestAnimationFrame = (fn) => fn();
  return dom;
}

const entries = [
  { id: 'a1', type: 'message', message: { role: 'user', content: 'First request' } },
  { id: 'b2', type: 'message', message: { role: 'assistant', content: 'Ignore me' } },
  { id: 'c3', type: 'message', message: { role: 'user', content: 'Implement the palette redesign with keyboard nav' } },
];

describe('showForkModal', () => {
  it('renders latest user messages first as flat palette rows', () => {
    const dom = makeDom();

    showForkModal({ entries, documentImpl: dom.window.document, windowImpl: dom.window });

    const rows = Array.from(dom.window.document.querySelectorAll('.fork-message-item'));
    expect(rows).toHaveLength(2);
    expect(rows[0].textContent).toContain('#2');
    expect(rows[0].textContent).toContain('Implement the palette');
    expect(rows[1].textContent).toContain('#1');
    expect(dom.window.document.querySelector('.fork-message-preview').textContent).toContain('Implement the palette redesign');
  });

  it('filters messages and selects the highlighted row with enter', () => {
    const dom = makeDom();
    const onSelect = vi.fn();

    showForkModal({ entries, documentImpl: dom.window.document, windowImpl: dom.window, onSelect });

    const input = dom.window.document.querySelector('.fork-search-input');
    input.value = 'first';
    input.dispatchEvent(new dom.window.Event('input', { bubbles: true }));
    input.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));

    expect(onSelect).toHaveBeenCalledWith('a1');
  });
});
