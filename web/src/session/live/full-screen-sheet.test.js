import { describe, expect, it, vi } from 'vitest';
import { showSheet } from './full-screen-sheet.js';

function createMockDocument() {
  let lastWrapperHTML = '';
  let lastBackdrop = null;
  let storedBodyEl = null;
  let storedPanel = null;

  const body = { appendChild: vi.fn() };

  function createElement() {
    const el = {
      innerHTML: '',
      firstElementChild: null,
      classList: {
        _classes: new Set(),
        add: vi.fn((...cls) => cls.forEach(c => el.classList._classes.add(c))),
        remove: vi.fn(),
        contains: vi.fn((cls) => el.classList._classes.has(cls)),
      },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      remove: vi.fn(),
      style: {},
      focus: vi.fn(),
      appendChild: vi.fn(),
    };
    Object.defineProperty(el, 'innerHTML', {
      get() { return lastWrapperHTML; },
      set(val) {
        lastWrapperHTML = val;
        lastBackdrop = {
          classList: { add: vi.fn(), remove: vi.fn(), contains: vi.fn(() => false) },
          addEventListener: vi.fn(),
          remove: vi.fn(),
          focus: vi.fn(),
          querySelectorAll: vi.fn(() => []),
          contains: vi.fn(() => true),
        };
        el.firstElementChild = lastBackdrop;
      },
    });
    return el;
  }

  function mockGetElementById(id) {
    if (id && id.endsWith('-body')) {
      if (!storedBodyEl) {
        storedBodyEl = {
          innerHTML: '',
          appendChild: vi.fn((child) => { storedBodyEl._child = child; }),
          style: {},
        };
      }
      return storedBodyEl;
    }
    if (id && id.endsWith('-panel')) {
      if (!storedPanel) {
        storedPanel = {
          classList: { add: vi.fn(), remove: vi.fn() },
          focus: vi.fn(),
          querySelectorAll: vi.fn(() => []),
          contains: vi.fn(() => true),
        };
      }
      return storedPanel;
    }
    return null;
  }

  return {
    createElement,
    body,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    getElementById: vi.fn(mockGetElementById),
    activeElement: { focus: vi.fn() },
    _getLastWrapperHTML: () => lastWrapperHTML,
    _getLastBackdrop: () => lastBackdrop,
    _getBodyEl: () => storedBodyEl,
    _getPanel: () => storedPanel,
  };
}

const noopRaf = vi.fn((fn) => fn());

const mockWindow = {
  matchMedia: vi.fn(() => ({ matches: false })),
  setTimeout: vi.fn((fn) => fn()),
  requestAnimationFrame: vi.fn((fn) => fn()),
};

function createMockMobileWindow() {
  const listeners = {};
  const history = {
    state: null,
    pushState: vi.fn(function(state) { this.state = state; }),
    back: vi.fn(function() { this.state = null; }),
  };
  return {
    matchMedia: vi.fn(() => ({ matches: true })),
    setTimeout: vi.fn((fn) => fn()),
    requestAnimationFrame: vi.fn((fn) => fn()),
    addEventListener: vi.fn((type, fn) => { listeners[type] = fn; }),
    removeEventListener: vi.fn(),
    history,
    location: { href: 'http://example.test/session?id=abc' },
    _listeners: listeners,
  };
}

describe('showSheet', () => {
  it('appends a sheet to the document body', () => {
    const documentImpl = createMockDocument();
    showSheet({
      title: 'Usage',
      renderBody: () => '<p>Content</p>',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    expect(documentImpl.body.appendChild).toHaveBeenCalled();
    const html = documentImpl._getLastWrapperHTML();
    const bodyEl = documentImpl._getBodyEl();
    expect(html).toContain('Usage');
    expect(bodyEl.innerHTML).toContain('<p>Content</p>');
  });

  it('has dialog semantics without aria-hidden backdrop', () => {
    const documentImpl = createMockDocument();
    showSheet({
      title: 'Test',
      renderBody: () => '',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    const html = documentImpl._getLastWrapperHTML();
    expect(html).toContain('role="dialog"');
    expect(html).toContain('aria-modal="true"');
    expect(html).toContain('class="sr-only"');
    expect(html).toContain('<h2');
    // Backdrop itself should not be aria-hidden (would hide the dialog from AT)
    expect(html).not.toMatch(/backdrop[^>]*aria-hidden/);
  });

  it('escapes quotes in title for attributes', () => {
    const documentImpl = createMockDocument();
    showSheet({
      title: 'Say "hello"',
      renderBody: () => '',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    const html = documentImpl._getLastWrapperHTML();
    expect(html).toContain('&quot;');
    expect(html).not.toContain('aria-label="Close Say "hello""');
  });

  it('hides back button when showBack=false', () => {
    const documentImpl = createMockDocument();
    showSheet({
      title: 'No Back',
      renderBody: () => '',
      showBack: false,
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    const html = documentImpl._getLastWrapperHTML();
    expect(html).not.toContain('class="pi-sheet-back"');
  });

  it('hides close X when showClose=false', () => {
    const documentImpl = createMockDocument();
    showSheet({
      title: 'No Close',
      renderBody: () => '',
      showClose: false,
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    const html = documentImpl._getLastWrapperHTML();
    expect(html).not.toContain('pi-sheet-close-x');
  });

  it('does not register Escape listener when closeOnEscape=false', () => {
    const documentImpl = createMockDocument();
    showSheet({
      title: 'No Esc',
      renderBody: () => '',
      closeOnEscape: false,
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    const keydownCalls = documentImpl.addEventListener.mock.calls.filter(c => c[0] === 'keydown');
    // Only focus trap, no Escape
    expect(keydownCalls.length).toBe(1);
  });

  it('validates renderBody is a function', () => {
    const documentImpl = createMockDocument();
    expect(() => showSheet({
      title: 'Bad',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    })).toThrow('showSheet requires renderBody');
  });

  it('adds open class on next frame', () => {
    const raf = vi.fn((fn) => fn());
    const documentImpl = createMockDocument();
    showSheet({
      title: 'Anim',
      renderBody: () => '',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: raf,
    });
    expect(raf).toHaveBeenCalled();
    const backdrop = documentImpl._getLastBackdrop();
    expect(backdrop.classList.add).toHaveBeenCalledWith('open');
  });

  it('registers escape key and focus trap handlers', () => {
    const documentImpl = createMockDocument();
    showSheet({
      title: 'Key',
      renderBody: () => '',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    const keydownCalls = documentImpl.addEventListener.mock.calls.filter(c => c[0] === 'keydown');
    expect(keydownCalls.length).toBe(2);
  });

  it('returns close and updateBody functions', () => {
    const documentImpl = createMockDocument();
    const sheet = showSheet({
      title: 'Fns',
      renderBody: () => '',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    expect(typeof sheet.close).toBe('function');
    expect(typeof sheet.updateBody).toBe('function');
  });

  it('calls onClose when close is invoked', () => {
    const documentImpl = createMockDocument();
    const onClose = vi.fn();
    const sheet = showSheet({
      title: 'Close',
      renderBody: () => '',
      onClose,
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    sheet.close();
    expect(onClose).toHaveBeenCalled();
  });

  it('passes close function and bodyEl to renderBody', () => {
    const documentImpl = createMockDocument();
    let receivedClose = null;
    let receivedBodyEl = null;
    showSheet({
      title: 'Pass Close',
      renderBody: ({ close, bodyEl }) => { receivedClose = close; receivedBodyEl = bodyEl; return ''; },
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    expect(typeof receivedClose).toBe('function');
    expect(receivedBodyEl).toBeTruthy();
  });

  it('removes keydown listeners on close', () => {
    const documentImpl = createMockDocument();
    const sheet = showSheet({
      title: 'Cleanup',
      renderBody: () => '',
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    sheet.close();
    expect(documentImpl.removeEventListener).toHaveBeenCalledWith('keydown', expect.any(Function));
  });

  it('uses isNode helper instead of instanceof for DOM nodes', () => {
    const documentImpl = createMockDocument();
    const fakeNode = { nodeType: 1 };
    showSheet({
      title: 'Node Test',
      renderBody: () => fakeNode,
      documentImpl,
      windowImpl: mockWindow,
      requestAnimationFrameImpl: noopRaf,
    });
    const bodyEl = documentImpl._getBodyEl();
    expect(bodyEl.appendChild).toHaveBeenCalledWith(fakeNode);
  });

  describe('mobile history close', () => {
    it('pushes a same-url history entry for mobile sheets', () => {
      const documentImpl = createMockDocument();
      const windowImpl = createMockMobileWindow();
      showSheet({
        title: 'History',
        renderBody: () => '',
        documentImpl,
        windowImpl,
        requestAnimationFrameImpl: noopRaf,
      });

      expect(windowImpl.history.pushState).toHaveBeenCalledWith(
        expect.objectContaining({ __piSheet: expect.stringMatching(/^pi-sheet:/) }),
        '',
        'http://example.test/session?id=abc'
      );
      expect(windowImpl.addEventListener).toHaveBeenCalledWith('popstate', expect.any(Function));
    });

    it('closes the sheet when browser back pops the sheet history entry', () => {
      const documentImpl = createMockDocument();
      const windowImpl = createMockMobileWindow();
      const onClose = vi.fn();
      showSheet({
        title: 'Pop Close',
        renderBody: () => '',
        onClose,
        documentImpl,
        windowImpl,
        requestAnimationFrameImpl: noopRaf,
      });

      windowImpl._listeners.popstate();

      expect(onClose).toHaveBeenCalled();
      expect(windowImpl.history.back).not.toHaveBeenCalled();
      expect(windowImpl.removeEventListener).toHaveBeenCalledWith('popstate', expect.any(Function));
    });

    it('pops the synthetic history entry when closed by UI', () => {
      const documentImpl = createMockDocument();
      const windowImpl = createMockMobileWindow();
      const sheet = showSheet({
        title: 'UI Close',
        renderBody: () => '',
        documentImpl,
        windowImpl,
        requestAnimationFrameImpl: noopRaf,
      });

      sheet.close();

      expect(windowImpl.history.back).toHaveBeenCalled();
      expect(windowImpl.removeEventListener).toHaveBeenCalledWith('popstate', expect.any(Function));
    });

    it('does not touch history for desktop sheets', () => {
      const documentImpl = createMockDocument();
      const history = { pushState: vi.fn(), back: vi.fn(), state: null };
      const windowImpl = {
        ...mockWindow,
        history,
        location: { href: 'http://example.test/session?id=abc' },
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      };

      showSheet({
        title: 'Desktop',
        renderBody: () => '',
        documentImpl,
        windowImpl,
        requestAnimationFrameImpl: noopRaf,
      });

      expect(history.pushState).not.toHaveBeenCalled();
      expect(windowImpl.addEventListener).not.toHaveBeenCalledWith('popstate', expect.any(Function));
    });
  });

});
