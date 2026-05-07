import { describe, expect, it, vi } from 'vitest';
import { getJSON, postJSON } from './api.js';

describe('api helpers', () => {
  it('returns parsed JSON for a successful GET', async () => {
    const fetchImpl = vi.fn(async () => new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await expect(getJSON('/api/models', { fetchImpl })).resolves.toEqual({ ok: true });
    expect(fetchImpl).toHaveBeenCalledWith('/api/models', { headers: { Accept: 'application/json' } });
  });

  it('throws the JSON error message for failed responses', async () => {
    const fetchImpl = vi.fn(async () => new Response(JSON.stringify({ error: 'bad request' }), { status: 400 }));
    await expect(getJSON('/api/bad', { fetchImpl })).rejects.toThrow('bad request');
  });

  it('POSTs JSON bodies with the expected headers', async () => {
    const fetchImpl = vi.fn(async () => new Response(JSON.stringify({ ok: true }), { status: 200 }));
    await expect(postJSON('/api/new-session', { path: '/tmp/project' }, { fetchImpl })).resolves.toEqual({ ok: true });
    expect(fetchImpl).toHaveBeenCalledWith('/api/new-session', {
      method: 'POST',
      headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: '/tmp/project' })
    });
  });
});
