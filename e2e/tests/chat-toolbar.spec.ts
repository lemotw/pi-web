import { test, expect, collapseScratchpad, isMobileLayout } from "../lib/test";
import {
  buildSession,
  realWorkingDir,
  uniqueSessionName,
  writeSession,
} from "../lib/sessions";

test("running composer keeps metadata and actions separated", async ({
  page,
  sessionsDir,
}, testInfo) => {
  const model = "a-very-long-model-name-with-a-long-provider";
  const { entries } = buildSession({ cwd: realWorkingDir(), modelId: model });
  const withUsage = entries.map((entry) =>
    entry.message?.role === "assistant"
      ? {
          ...entry,
          message: {
            ...entry.message,
            usage: { input: 31000, output: 0, totalTokens: 31000 },
          },
        }
      : entry,
  );
  const id = writeSession(
    sessionsDir,
    uniqueSessionName(testInfo, "chat-toolbar"),
    withUsage,
  );
  await page.route("**/api/worker-status?**", (route) =>
    route.fulfill({
      json: {
        state: "running",
        model,
        modelProvider: "test-provider",
        thinkingLevel: "medium",
      },
    }),
  );
  await page.route("**/api/models", (route) =>
    route.fulfill({
      json: {
        models: [
          { id: model, provider: "test-provider", contextWindow: 100000 },
        ],
      },
    }),
  );
  await collapseScratchpad(page);
  await page.goto(`/session?id=${encodeURIComponent(id)}`);
  await expect(page.locator("#pi-chat-status")).toHaveText("running");
  await expect(page.locator("#pi-chat-model-label")).toContainText(model);
  await expect(page.locator(".pi-context-text")).toHaveText("31%");

  const mobile = await isMobileLayout(page);
  const initialViewport = page.viewportSize()!;
  const widths = mobile
    ? [initialViewport.width, 320, 430]
    : [initialViewport.width];
  for (const width of widths) {
    await page.setViewportSize({ ...initialViewport, width });
    const ids = [
      "attach",
      "thinking-label",
      "model-label",
      "context-usage",
      "status",
      "cancel",
      "queue",
      "send",
    ];
    const toolbar = page.locator(".pi-chat-toolbar-controls");
    const bounds = await toolbar.boundingBox();
    expect(bounds).not.toBeNull();
    const boxes = [];
    for (const id of ids) {
      const control = page.locator(`#pi-chat-${id}`);
      await expect(control).toBeVisible();
      const box = (await control.boundingBox())!;
      expect(box.width, `${id} has room at ${width}px`).toBeGreaterThan(0);
      expect(box.x).toBeGreaterThanOrEqual(bounds!.x);
      expect(box.x + box.width).toBeLessThanOrEqual(
        bounds!.x + bounds!.width + 1,
      );
      if (mobile && id !== "status") {
        expect(box.width).toBeGreaterThanOrEqual(44);
        expect(box.height).toBeGreaterThanOrEqual(44);
      }
      for (const other of boxes) {
        const overlapX =
          Math.min(box.x + box.width, other.x + other.width) -
          Math.max(box.x, other.x);
        const overlapY =
          Math.min(box.y + box.height, other.y + other.height) -
          Math.max(box.y, other.y);
        expect(
          overlapX > 1 && overlapY > 1,
          `${id} overlaps ${other.id} at ${width}px`,
        ).toBe(false);
      }
      boxes.push({ id, ...box });
    }
    if (mobile) {
      const metadata = (await page
        .locator(".pi-chat-toolbar-left")
        .boundingBox())!;
      const actions = (await page
        .locator(".pi-chat-toolbar-right")
        .boundingBox())!;
      expect(actions.y).toBeGreaterThanOrEqual(metadata.y + metadata.height);
    }
    await expect(page.locator("#pi-chat-model-label")).toHaveCSS(
      "text-overflow",
      "ellipsis",
    );
  }
});
