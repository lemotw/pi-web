import { describe, expect, it } from 'vitest';
import { createSessionsPage } from './index.js';

describe('createSessionsPage', () => {
  it('creates the sessions page Alpine state object', () => {
    const page = createSessionsPage();
    expect(page).toMatchObject({ query: '', modal: false, path: '', recent: [], creating: false, error: '' });
    expect(typeof page.filter).toBe('function');
    expect(typeof page.openModal).toBe('function');
    expect(typeof page.create).toBe('function');
  });
});
