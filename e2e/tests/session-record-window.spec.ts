import { test, expect, collapseScratchpad } from "../lib/test";
import {
  appendEntry,
  assistantTextEntry,
  buildSession,
  uniqueSessionName,
  writeSession,
} from "../lib/sessions";

test("record windows stay bounded through paging, refresh, and live reload", async ({
  page,
  sessionsDir,
}, testInfo) => {
  const initial = buildSession();
  const entries: unknown[] = [...initial.entries];
  let lastId = initial.lastId;
  for (let i = 0; i < 260; i++) {
    const next = assistantTextEntry(lastId, `WINDOW_RECORD_${i}`);
    lastId = next.id;
    entries.push(next.entry);
  }
  const name = uniqueSessionName(testInfo, "window");
  writeSession(sessionsDir, name, entries);
  const recordResponses: number[] = [];
  page.on("response", async (response) => {
    if (new URL(response.url()).pathname !== "/api/session") return;
    const data = await response.json().catch(() => null);
    if (Array.isArray(data?.entries)) recordResponses.push(data.entries.length);
  });
  await collapseScratchpad(page);
  await page.goto(`/session?id=${encodeURIComponent(name)}&limit=50`);
  const messages = page.locator("#messages-list");
  await expect(messages.locator(".assistant-message")).toHaveCount(50);
  await expect(messages).toContainText("WINDOW_RECORD_259");
  await expect(messages).not.toContainText("WINDOW_RECORD_100");
  await page.getByRole("button", { name: "Earlier", exact: true }).click();
  await expect(messages).toContainText("WINDOW_RECORD_209");
  await expect(messages).not.toContainText("WINDOW_RECORD_259");

  const next = assistantTextEntry(lastId, "NEW_WINDOW_RECORD");
  appendEntry(sessionsDir, name, next.entry);
  await expect(page.locator(".session-window .range")).toContainText("of 266", {
    timeout: 15000,
  });
  await expect(messages).not.toContainText("NEW_WINDOW_RECORD");
  await page.getByRole("button", { name: "Latest", exact: true }).click();
  await expect(messages).toContainText("NEW_WINDOW_RECORD");
  await expect(messages.locator(".assistant-message")).toHaveCount(50);

  await page.getByLabel("Records per page").selectOption("100");
  await expect(messages.locator(".assistant-message")).toHaveCount(100);
  await page.reload();
  await expect(page.getByLabel("Records per page")).toHaveValue("100");
  await expect(messages.locator(".assistant-message")).toHaveCount(100);
  expect(recordResponses.length).toBeGreaterThan(0);
  expect(recordResponses.every((count) => count <= 100)).toBeTruthy();
});
