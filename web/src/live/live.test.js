import { describe, expect, it } from 'vitest';
import { liveEntrypointLoaded } from './live.js';

describe('live entrypoint', () => {
  it('exports a load marker for smoke testing', () => {
    expect(liveEntrypointLoaded).toBe(true);
  });
});
