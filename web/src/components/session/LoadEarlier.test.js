import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import LoadEarlier from './LoadEarlier.svelte';
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
  render(LoadEarlier, { model, loadWindow, navigateTo });
  return { model, fetchImpl, navigateTo };
}

afterEach(() => {
  document.cookie = 'pi_session_window=; Path=/; Max-Age=0';
  window.history.replaceState({}, '', '/');
});

describe('record window controls', () => {
  it('replaces the page when loading earlier and offers a return to latest', async () => {
    const { model, fetchImpl, navigateTo } = setup();
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

  it('changes page size and remembers it without retaining stale deep-link params', async () => {
    const { fetchImpl } = setup();
    window.history.replaceState({}, '', '/session?id=s&targetId=old');
    fetchImpl.mockResolvedValueOnce(
      new Response(
        JSON.stringify({ entries: [], total: 0, from: 0, windowSize: 100, windowEnd: null }),
      ),
    );
    await fireEvent.change(screen.getByLabelText('Records per page'), { target: { value: '100' } });
    await waitFor(() => expect(document.cookie).toContain('pi_session_window=100'));
    expect(window.location.search).toBe('?id=s&limit=100');
    expect(fetchImpl.mock.calls[0][0]).toBe('/api/session?id=s&paginate=1&limit=100');
  });

  it('exposes errors without destroying the old page', async () => {
    const { fetchImpl, model } = setup();
    fetchImpl.mockResolvedValueOnce(new Response('', { status: 503 }));
    await fireEvent.click(screen.getByRole('button', { name: 'Earlier' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('HTTP 503');
    expect(model.from).toBe(200);
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(model.from).toBe(150));
  });
});
