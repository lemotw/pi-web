import { describe, expect, it } from 'vitest';
import { sessionEntrypointLoaded } from './session.js';

describe('session entrypoint', () => {
  it('exports a load marker for smoke testing', () => {
    expect(sessionEntrypointLoaded).toBe(true);
  });
});
