import { test, expect, collapseScratchpad, isMobileLayout } from "../lib/test";
import {
  assistantTextEntry,
  buildSession,
  uniqueSessionName,
  writeSession,
} from "../lib/sessions";

test("reading settings stay reachable, adapt to viewport, and close accessibly", async ({
  page,
  sessionsDir,
}, testInfo) => {
  const initial = buildSession();
  const entries: unknown[] = [...initial.entries];
  let lastId = initial.lastId;
  for (let i = 0; i < 130; i++) {
    const next = assistantTextEntry(
      lastId,
      `Example reply ${i}.\n\nThis is a reading-settings layout test.`,
    );
    entries.push(next.entry);
    lastId = next.id;
  }
  const name = uniqueSessionName(testInfo, "reading-settings");
  writeSession(sessionsDir, name, entries);
  await collapseScratchpad(page);
  await page.goto(`/session?id=${encodeURIComponent(name)}&limit=50`);
  const trigger = page.getByRole("button", {
    name: "Reading settings",
    exact: true,
  });
  const panel = page.getByRole("dialog", { name: "Reading settings" });
  await expect(trigger).toBeVisible();
  await expect(page.locator("#messages .message-filters")).toHaveCount(0);
  await expect(page.locator("#content .session-window")).toHaveCount(0);
  await expect(panel).toBeHidden();
  await page.locator("#content").evaluate((el) => {
    el.scrollTop = el.scrollHeight;
  });
  await expect(trigger).toBeInViewport();
  const viewport = page.viewportSize()!;
  const triggerBox = (await trigger.boundingBox())!;
  expect(triggerBox.x).toBeGreaterThanOrEqual(0);
  expect(triggerBox.x + triggerBox.width).toBeLessThanOrEqual(viewport.width);
  await trigger.click();
  await expect(panel).toBeVisible();
  const box = (await panel.boundingBox())!;
  await page.screenshot({ path: testInfo.outputPath("reading-settings.png") });
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
  if (await isMobileLayout(page)) {
    expect(Math.abs(box.y + box.height - viewport.height)).toBeLessThan(2);
    expect(box.y).toBeGreaterThan(0);
  } else {
    expect(box.x).toBeGreaterThan(viewport.width / 2);
    expect(box.y).toBeGreaterThanOrEqual(triggerBox.y + triggerBox.height);
  }
  for (const key of ["Tab", "Shift+Tab"]) {
    for (let i = 0; i < 16; i++) {
      await page.keyboard.press(key);
      expect(
        await panel.evaluate((el) => el.contains(document.activeElement)),
      ).toBeTruthy();
    }
  }
  await page.keyboard.press("Escape");
  await expect(panel).toBeHidden();
  await expect(trigger).toBeFocused();

  await trigger.click();
  await panel.getByRole("button", { name: "100", exact: true }).click();
  await expect(
    panel.getByRole("button", { name: "100", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await panel.getByRole("button", { name: "Compact", exact: true }).click();
  await panel.getByRole("button", { name: "Close", exact: true }).click();
  await expect(panel).toBeHidden();
  await expect(trigger).toHaveAttribute(
    "title",
    "Reading settings — 100 records · Compact",
  );
  await page.reload();
  await trigger.click();
  await expect(
    panel.getByRole("button", { name: "100", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    panel.getByRole("button", { name: "Compact", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await page.mouse.click(4, 60);
  await expect(panel).toBeHidden();
  await expect(trigger).toBeFocused();

  await page.setViewportSize({ width: 320, height: 568 });
  await expect(trigger).toBeInViewport({ ratio: 1 });
  await trigger.click();
  const narrow = (await panel.boundingBox())!;
  expect(narrow.x).toBeGreaterThanOrEqual(0);
  expect(narrow.x + narrow.width).toBeLessThanOrEqual(321);
  await panel.getByRole("button", { name: "Close", exact: true }).click();
});
