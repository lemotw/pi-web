import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, fireEvent, waitFor, within } from '@testing-library/svelte';
import ReadingSettings from './ReadingSettings.svelte';
import { SessionDataModel } from '../../session/data/session-data.svelte.js';
import { createSessionWindowLoader } from '../../session/session-window.js';

function setup() {
  const entries = Array.from({ length: 50 }, (_, i) => ({
    id: `e${i}`,
    type: 'message',
    message: { role: 'user', content: 'hello' },
  }));
  const model = new SessionDataModel({
    entries,
    total: 250,
    from: 200,
    leafId: 'e49',
    windowSize: 50,
  });
  const fetchImpl = vi.fn(
    async () =>
      new Response(
        JSON.stringify({ entries, total: 250, from: 150, windowSize: 50, windowEnd: 200 }),
      ),
  );
  const navigateTo = vi.fn();
  const loadWindow = createSessionWindowLoader({ model, sessionId: 's', fetchImpl });
  const { container } = render(ReadingSettings, { model, loadWindow, navigateTo });
  // jsdom has no native dialog implementation; real focus/Escape/backdrop
  // behavior is covered by Playwright on desktop and mobile.
  const dialog = container.querySelector('dialog');
  dialog.showModal = vi.fn(() => dialog.setAttribute('open', ''));
  dialog.close = vi.fn(() => {
    dialog.removeAttribute('open');
    dialog.dispatchEvent(new Event('close'));
  });
  const open = () => fireEvent.click(screen.getByRole('button', { name: 'Reading settings' }));
  return { model, fetchImpl, navigateTo, dialog, open };
}

afterEach(() => {
  cleanup();
  localStorage.clear();
  document.cookie = 'pi_session_window=; Path=/; Max-Age=0';
  window.history.replaceState({}, '', '/');
  vi.restoreAllMocks();
});

describe('reading settings', () => {
  it('keeps controls out of the conversation until opened, then restores focus on close', async () => {
    const { open } = setup();
    expect(screen.queryByRole('dialog')).toBeNull();
    const trigger = screen.getByRole('button', { name: 'Reading settings' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    await open();
    expect(screen.getByRole('dialog', { name: 'Reading settings' })).toBeInTheDocument();
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    expect(document.body).toHaveClass('reading-settings-open');
    await fireEvent.click(screen.getByRole('button', { name: 'Close', exact: true }));
    expect(trigger).toHaveFocus();
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(document.body).not.toHaveClass('reading-settings-open');
  });

  it('replaces the page when loading earlier and offers a return to latest', async () => {
    const { model, fetchImpl, navigateTo, open } = setup();
    await open();
    expect(screen.getByText('Records 201–250 of 250')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Latest' })).toBeDisabled();
    await fireEvent.click(screen.getByRole('button', { name: 'Earlier' }));
    await waitFor(() => expect(navigateTo).toHaveBeenCalledWith('e49', 'bottom'));
    expect(fetchImpl.mock.calls[0][0]).toBe('/api/session?id=s&paginate=1&limit=50&before=200');
    expect(model.entries).toHaveLength(50);
    expect(screen.getByRole('button', { name: 'Latest' })).not.toBeDisabled();
    await fireEvent.click(screen.getByRole('button', { name: 'Latest' }));
    expect(fetchImpl.mock.calls[1][0]).toBe('/api/session?id=s&paginate=1&limit=50');
  });

  it('changes page size immediately and remembers it without retaining stale deep links', async () => {
    const { fetchImpl, open } = setup();
    await open();
    window.history.replaceState({}, '', '/session?id=s&targetId=old');
    fetchImpl.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ entries: [], total: 0, from: 0, windowSize: 100, windowEnd: null }),
      ),
    );
    const sizes = within(screen.getByRole('group', { name: 'Records per page' }));
    await fireEvent.click(sizes.getByRole('button', { name: '100', exact: true }));
    await waitFor(() => expect(document.cookie).toContain('pi_session_window=100'));
    expect(window.location.search).toBe('?id=s&limit=100');
    expect(sizes.getByRole('button', { name: '100', exact: true })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
    expect(fetchImpl.mock.calls[0][0]).toBe('/api/session?id=s&paginate=1&limit=100');
  });

  it('exposes errors without destroying the old page', async () => {
    const { fetchImpl, model, open } = setup();
    await open();
    fetchImpl.mockResolvedValueOnce(new Response('', { status: 503 }));
    await fireEvent.click(screen.getByRole('button', { name: 'Earlier' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('HTTP 503');
    expect(model.from).toBe(200);
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(model.from).toBe(150));
  });

  it('reflects filters in the persistent header summary', async () => {
    const { open, model } = setup();
    await open();
    await fireEvent.click(screen.getByRole('button', { name: 'Compact', exact: true }));
    expect(model.contentFilter.mode).toBe('text');
    expect(screen.getByRole('switch', { name: 'Include thinking' })).not.toBeChecked();
    expect(screen.getByRole('button', { name: 'Reading settings' })).toHaveAttribute(
      'title',
      'Reading settings — 50 records · Compact',
    );
  });

  it('isolates keyboard shortcuts and releases scroll lock on route teardown', async () => {
    const { dialog, open } = setup();
    await open();
    const key = vi.fn();
    document.addEventListener('keydown', key);
    await fireEvent.keyDown(dialog, { key: 'o' });
    expect(key).not.toHaveBeenCalled();
    document.removeEventListener('keydown', key);
    cleanup();
    expect(document.body).not.toHaveClass('reading-settings-open');
  });
});
