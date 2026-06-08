import { afterEach, describe, expect, it, vi } from 'vitest';
import { createRightSidebarVisibility } from './right-sidebar-visibility.js';

function renderControls() {
  document.body.innerHTML = `
    <button id="toggle"></button>
    <button id="close"></button>
    <button id="expand"></button>
    <button id="backdrop"></button>
  `;
  return {
    toggleBtn: document.getElementById('toggle'),
    closeBtn: document.getElementById('close'),
    expandBtn: document.getElementById('expand'),
    backdrop: document.getElementById('backdrop'),
  };
}

function createController(loadScratchpad = vi.fn()) {
  const storage = { setItem: vi.fn() };
  const controller = createRightSidebarVisibility({
    documentImpl: document,
    storage,
    collapsedStorageKey: 'collapsed-key',
    loadScratchpad,
  });
  return { controller, storage, loadScratchpad };
}

afterEach(() => {
  document.body.innerHTML = '';
  document.body.className = '';
  vi.restoreAllMocks();
});

describe('createRightSidebarVisibility', () => {
  it('opens only from collapsed state and loads the scratchpad', () => {
    const { controller, storage, loadScratchpad } = createController();

    controller.open();
    expect(loadScratchpad).not.toHaveBeenCalled();

    document.body.classList.add('right-sidebar-collapsed');
    controller.open();

    expect(document.body.classList.contains('right-sidebar-collapsed')).toBe(false);
    expect(storage.setItem).toHaveBeenCalledWith('collapsed-key', 'false');
    expect(loadScratchpad).toHaveBeenCalledTimes(1);
  });

  it('toggles collapse state and clears expanded mode when closing', () => {
    const { controller, storage, loadScratchpad } = createController();
    document.body.classList.add('right-sidebar-expanded');

    controller.toggle();

    expect(document.body.classList.contains('right-sidebar-collapsed')).toBe(true);
    expect(document.body.classList.contains('right-sidebar-expanded')).toBe(false);
    expect(storage.setItem).toHaveBeenCalledWith('collapsed-key', 'true');
    expect(loadScratchpad).not.toHaveBeenCalled();

    controller.toggle();
    expect(document.body.classList.contains('right-sidebar-collapsed')).toBe(false);
    expect(loadScratchpad).toHaveBeenCalledTimes(1);
  });

  it('binds close/backdrop collapse and expand button behavior', () => {
    const controls = renderControls();
    const { controller, loadScratchpad } = createController();
    const cleanup = controller.bindControls(controls);

    document.body.classList.add('right-sidebar-collapsed');
    controls.expandBtn.click();
    expect(document.body.classList.contains('right-sidebar-collapsed')).toBe(false);
    expect(document.body.classList.contains('right-sidebar-expanded')).toBe(true);
    expect(loadScratchpad).toHaveBeenCalledTimes(1);

    controls.expandBtn.click();
    expect(document.body.classList.contains('right-sidebar-expanded')).toBe(false);

    controls.expandBtn.click();
    controls.closeBtn.click();
    expect(document.body.classList.contains('right-sidebar-collapsed')).toBe(true);
    expect(document.body.classList.contains('right-sidebar-expanded')).toBe(false);

    controller.open();
    controls.backdrop.click();
    expect(document.body.classList.contains('right-sidebar-collapsed')).toBe(true);

    cleanup();
    controller.open();
    controls.closeBtn.click();
    expect(document.body.classList.contains('right-sidebar-collapsed')).toBe(false);
  });
});
