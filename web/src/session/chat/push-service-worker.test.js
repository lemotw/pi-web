import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import vm from 'node:vm';

function loadServiceWorker({ clients = [] } = {}) {
  const listeners = {};
  const notifications = [];
  const self = {
    addEventListener(type, handler) { listeners[type] = handler; },
    skipWaiting() {},
    clients: {
      claim() {},
      matchAll: async () => clients,
      openWindow: async () => null
    },
    registration: {
      showNotification: async (title, options) => { notifications.push({ title, options }); }
    }
  };
  const code = readFileSync(resolve(process.cwd(), '../internal/ui/live_templates/assets/sw.js'), 'utf8');
  vm.runInNewContext(code, { self }, { filename: 'internal/ui/live_templates/assets/sw.js' });
  return { listeners, notifications };
}

async function dispatchPush(listener, payload = { title: 'pi session', body: 'Response ready', sessionId: 's1' }) {
  const waits = [];
  listener({
    data: { json: () => payload },
    waitUntil: (promise) => waits.push(promise)
  });
  await Promise.all(waits);
}

describe('push service worker notifications', () => {
  it('suppresses system push notifications while an app window is visible', async () => {
    const { listeners, notifications } = loadServiceWorker({
      clients: [{ visibilityState: 'visible', url: 'http://localhost/session?id=s1' }]
    });

    await dispatchPush(listeners.push);

    expect(notifications).toEqual([]);
  });

  it('suppresses system push notifications while an app window is focused', async () => {
    const { listeners, notifications } = loadServiceWorker({
      clients: [{ focused: true, url: 'http://localhost/session?id=s1' }]
    });

    await dispatchPush(listeners.push);

    expect(notifications).toEqual([]);
  });

  it('shows system push notifications when the app is backgrounded or locked', async () => {
    const { listeners, notifications } = loadServiceWorker({
      clients: [{ visibilityState: 'hidden', url: 'http://localhost/session?id=s1' }]
    });

    await dispatchPush(listeners.push);

    expect(notifications).toHaveLength(1);
    expect(notifications[0]).toMatchObject({
      title: 'pi session',
      options: { body: 'Response ready', silent: false, data: { sessionId: 's1' } }
    });
  });

  it('shows system push notifications when the app is closed', async () => {
    const { listeners, notifications } = loadServiceWorker({ clients: [] });

    await dispatchPush(listeners.push);

    expect(notifications).toHaveLength(1);
  });
});
