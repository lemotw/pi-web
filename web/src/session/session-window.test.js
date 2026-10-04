import { afterEach, describe, expect, it, vi } from 'vitest';
import { SessionDataModel } from './data/session-data.svelte.js';
import {
  createSessionWindowLoader,
  readSessionWindowSize,
  saveSessionWindowSize,
  sessionWindowUrl,
} from './session-window.js';

const entry = (id) => ({ id, type: 'message', message: { role: 'user', content: id } });
const payload = (id, extra = {}) => ({
  entries: [entry(id)],
  from: 99,
  total: 100,
  windowSize: 50,
  windowEnd: null,
  ...extra,
});
const response = (data) => new Response(JSON.stringify(data));
afterEach(() => {
  document.cookie = 'pi_session_window=; Path=/; Max-Age=0';
});

describe('session windows', () => {
  it('persists only allowed sizes and builds bounded, encoded URLs', () => {
    expect(readSessionWindowSize()).toBe(100);
    saveSessionWindowSize(50);
    expect(readSessionWindowSize()).toBe(50);
    expect(sessionWindowUrl('a & b', { before: 80 })).toBe(
      '/api/session?id=a+%26+b&paginate=1&limit=50&before=80',
    );
    saveSessionWindowSize(99999);
    expect(readSessionWindowSize()).toBe(100);
  });

  it('replaces the window rather than appending and keeps metadata current on live reload', async () => {
    const model = new SessionDataModel({ entries: [entry('old')], leafId: 'old', windowSize: 50 });
    const fetchImpl = vi.fn(async () => response(payload('new')));
    const load = createSessionWindowLoader({ model, sessionId: 's', fetchImpl });
    await load();
    expect(model.entries.map((e) => e.id)).toEqual(['new']);
    expect(model.from).toBe(99);
    expect(model.total).toBe(100);
    expect(model.currentTargetId).toBe('new');
    expect(model.windowBusy).toBe(false);
    expect(fetchImpl.mock.calls[0][0]).toBe('/api/session?id=s&paginate=1&limit=50');
  });

  it('freezes an earlier window across reloads', async () => {
    const model = new SessionDataModel({ entries: [], windowSize: 50 });
    const fetchImpl = vi.fn(async () =>
      response(payload('old', { from: 0, total: 200, windowEnd: 50 })),
    );
    const load = createSessionWindowLoader({ model, sessionId: 's', fetchImpl });
    await load({ before: 50 });
    await load();
    expect(fetchImpl.mock.calls.map((call) => call[0])).toEqual(
      Array(2).fill('/api/session?id=s&paginate=1&limit=50&before=50'),
    );
  });

  it('coalesces identical requests and ignores older responses after a size change', async () => {
    const model = new SessionDataModel({ entries: [], windowSize: 100 });
    const resolve = [];
    const fetchImpl = vi.fn(() => new Promise((r) => resolve.push(r)));
    const load = createSessionWindowLoader({ model, sessionId: 's', fetchImpl });
    const old = load();
    expect(load()).toBe(old);
    const latest = load({ limit: 50, before: null });
    resolve[1](response(payload('new')));
    await latest;
    resolve[0](response(payload('stale', { windowSize: 100 })));
    expect(await old).toBeNull();
    expect(model.entries[0].id).toBe('new');
    expect(model.windowSize).toBe(50);
    expect(fetchImpl).toHaveBeenCalledTimes(2);
  });

  it('preserves the visible page on error and retries the requested range', async () => {
    const model = new SessionDataModel({ entries: [entry('old')], leafId: 'old', windowSize: 100 });
    const fetchImpl = vi
      .fn()
      .mockResolvedValueOnce(new Response('', { status: 500 }))
      .mockResolvedValueOnce(response(payload('new')));
    const load = createSessionWindowLoader({ model, sessionId: 's', fetchImpl });
    await expect(load({ limit: 50 })).rejects.toThrow('HTTP 500');
    expect(model.entries[0].id).toBe('old');
    expect(model.windowError).toBe('HTTP 500');
    expect(model.windowBusy).toBe(false);
    await load();
    expect(model.windowError).toBe('');
    expect(model.windowSize).toBe(50);
  });
});
