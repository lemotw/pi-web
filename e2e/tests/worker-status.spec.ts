import { test, expect, collapseScratchpad } from "../lib/test";
import { buildSession, realWorkingDir, uniqueSessionName, writeSession } from "../lib/sessions";

test("retry and stream silence stay running; idle requires RPC confirmation", async ({ page, request, sessionsDir }, testInfo) => {
  const { entries } = buildSession({ cwd: realWorkingDir() });
  const id = writeSession(sessionsDir, uniqueSessionName(testInfo, "retry-idle"), entries);
  const query = `?id=${encodeURIComponent(id)}`;
  const status = async () => (await request.get(`/api/worker-status${query}`)).json();
  const worker = async () => {
    const metrics = await (await request.get("/api/metrics")).json();
    return metrics.workers.find((item: { session_id: string }) => item.session_id === id);
  };

  await collapseScratchpad(page);
  await page.goto(`/session${query}`);
  await page.locator("#pi-chat-message").fill("[[retry]] [[slow:8000]] keep working after retry");
  await page.locator("#pi-chat-send").click();
  await expect.poll(async () => (await status()).state).toBe("running");
  // The manager reports accepted/spawning sends as running before a PID exists.
  await expect.poll(async () => (await worker())?.state).toBe("running");
  const pid = (await worker()).pid;

  // No browser subscriber is required for status reconciliation.
  await page.goto("about:blank");
  await new Promise((resolve) => setTimeout(resolve, 3100));
  expect((await status()).state).toBe("running");
  expect((await worker()).pid).toBe(pid);
  expect((await worker()).idle_for_s).toBe(0);

  await expect.poll(async () => (await status()).state, { timeout: 20000 }).toBe("idle");
  expect((await status()).checkedAt).toBeGreaterThan(0);
  const idleWorker = await worker();
  expect(idleWorker.pid).toBe(pid);
  expect(idleWorker.idle_for_s).toBeLessThan(5);
  const before = idleWorker.idle_for_s;
  await status();
  await new Promise((resolve) => setTimeout(resolve, 150));
  expect((await worker()).idle_for_s).toBeGreaterThanOrEqual(before);
});
