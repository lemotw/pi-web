import { test, expect, collapseScratchpad } from "../lib/test";
import {
  buildSession,
  realWorkingDir,
  uniqueSessionName,
  writeSession,
} from "../lib/sessions";

// Chat is driven by a `pi --mode rpc` worker. CI has no real pi, so a stub pi
// (e2e/lib/stub-pi/pi, prepended to PATH by the server harness) answers the rpc
// protocol and writes a deterministic reply into the session file, which the
// browser then picks up via the live-reload SSE path.

test.describe("chat (stubbed pi)", () => {
  test("prompt uses a zoom-safe font on mobile and touch devices", async ({
    page,
    sessionsDir,
  }, testInfo) => {
    const { entries } = buildSession({ cwd: realWorkingDir() });
    const id = writeSession(
      sessionsDir,
      uniqueSessionName(testInfo, "chat-font"),
      entries,
    );
    await collapseScratchpad(page);
    await page.goto(`/session?id=${encodeURIComponent(id)}`);

    const textarea = page.locator("#pi-chat-message");
    await expect(textarea).toBeEnabled();
    const needsZoomSafeFont = await page.evaluate(
      () =>
        window.matchMedia("(max-width: 900px)").matches ||
        window.matchMedia("(pointer: coarse)").matches,
    );

    // Emulation cannot reproduce iOS keyboard auto-zoom; verify its CSS trigger
    // instead, including touch tablets whose landscape layout exceeds 900px.
    for (const uiSize of [10, 12, 20]) {
      await page.evaluate((size) => {
        document.documentElement.style.setProperty(
          "--font-size-ui",
          `${size}px`,
        );
      }, uiSize);
      const expectedSize = needsZoomSafeFont ? Math.max(16, uiSize) : 12;
      await expect(textarea).toHaveCSS("font-size", `${expectedSize}px`);
      await textarea.fill("Mobile prompt 字級測試");
      await expect(textarea).toBeFocused();
      await expect(textarea).toHaveCSS("font-size", `${expectedSize}px`);
      await page.locator("#pi-chat-expand").click();
      await expect(page.locator(".pi-chat-shell")).toHaveClass(/expanded/);
      await expect(textarea).toHaveCSS("font-size", `${expectedSize}px`);
      await page.locator("#pi-chat-expand").click();
    }

    // Prevent auto-zoom through legible text, not by disabling accessible zoom.
    const viewport = await page
      .locator('meta[name="viewport"]')
      .getAttribute("content");
    expect(viewport).not.toMatch(
      /user-scalable\s*=\s*(?:no|0)|maximum-scale\s*=/i,
    );
  });

  test("sending a message shows the assistant reply", async ({
    page,
    sessionsDir,
  }, testInfo) => {
    // Session cwd must exist on disk or chat is disabled ("View only").
    const cwd = realWorkingDir();
    const { entries } = buildSession({ cwd });
    const name = uniqueSessionName(testInfo, "chat");
    const id = writeSession(sessionsDir, name, entries);

    await collapseScratchpad(page);
    await page.goto(`/session?id=${encodeURIComponent(id)}`);

    // Composer should be enabled (cwd exists -> chat available).
    const composer = page.locator("#pi-chat-composer");
    await expect(composer).toHaveAttribute("data-chat-available", "true");

    const textarea = page.locator("#pi-chat-message");
    const prompt = `e2e-chat-${testInfo.workerIndex}-${Date.now()}`;
    await textarea.fill(prompt);
    await page.locator("#pi-chat-send").click();

    // The stub echoes "Stub reply: <prompt>" and writes it to the session file;
    // it surfaces in the message pane via the fsnotify -> SSE reload.
    await expect(page.locator("#messages")).toContainText(
      `Stub reply: ${prompt}`,
      {
        timeout: 20000,
      },
    );
  });
});
