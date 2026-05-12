import { describe, expect, it } from 'vitest';
import { sessionEntrypointLoaded, runSessionApp } from './session.js';

describe('session entrypoint', () => {
  it('exports a load marker for smoke testing', () => {
    expect(sessionEntrypointLoaded).toBe(true);
  });

  it('owns direct module runtime bootstrap', () => {
    expect(runSessionApp).toBeInstanceOf(Function);
  });
});
