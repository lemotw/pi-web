import { test, expect, collapseScratchpad } from "../lib/test";
import { uniqueSessionName, writeSession } from "../lib/sessions";

// Pagination counts all records, including the header. An explicit size keeps
// these live-view tests independent of the static export truncation settings.
const MESSAGE_COUNT = 150;
const EARLY_INDEX = 5;
const EARLY_MARKER = "EARLY_MARKER_LOADME";

function buildLargeSession(): unknown[] {
  const cwd = "/home/user/demo-project";
  const base = Date.parse("2026-05-06T00:00:00.000Z");
  const ts = (i: number) => new Date(base + i * 1000).toISOString();
  const entries: unknown[] = [
    {
      type: "session",
      version: 3,
      id: "019e0000-0000-7000-8000-000000000000",
      timestamp: ts(0),
      cwd,
    },
  ];
  let parentId: string | null = null;
  for (let i = 0; i < MESSAGE_COUNT; i += 1) {
    const id = `m${String(i).padStart(6, "0")}`;
    entries.push({
      type: "message",
      id,
      parentId,
      timestamp: ts(i + 1),
      message: {
        role: i % 2 === 0 ? "user" : "assistant",
        content: [
          {
            type: "text",
            text: i === EARLY_INDEX ? EARLY_MARKER : `message body ${i}`,
          },
        ],
        timestamp: base + (i + 1) * 1000,
      },
    });
    parentId = id;
  }
  return entries;
}

test.describe("earlier record windows", () => {
  test("reaches older messages without accumulating all pages", async ({
    page,
    sessionsDir,
  }, testInfo) => {
    await collapseScratchpad(page);
    const id = writeSession(
      sessionsDir,
      uniqueSessionName(testInfo, "le"),
      buildLargeSession(),
    );
    await page.goto(`/session?id=${encodeURIComponent(id)}&limit=50`);
    await page
      .getByRole("button", { name: "Reading settings", exact: true })
      .click();
    const controls = page.getByRole("dialog", { name: "Reading settings" });
    const messages = page.locator("#messages-list");
    await expect(controls).toContainText("Records 102–151 of 151");
    await expect(messages).not.toContainText(EARLY_MARKER);
    await controls
      .getByRole("button", { name: "Earlier", exact: true })
      .click();
    await expect(controls).toContainText("Records 52–101 of 151");
    await controls
      .getByRole("button", { name: "Earlier", exact: true })
      .click();
    await expect(messages).toContainText(EARLY_MARKER);
    await expect(
      messages.locator(".user-message, .assistant-message"),
    ).toHaveCount(50);
    await expect(messages).not.toContainText("message body 149");
  });

  test("opens a deep-linked old message within a bounded page", async ({
    page,
    sessionsDir,
  }, testInfo) => {
    await collapseScratchpad(page);
    const id = writeSession(
      sessionsDir,
      uniqueSessionName(testInfo, "target"),
      buildLargeSession(),
    );
    await page.goto(
      `/session?id=${encodeURIComponent(id)}&limit=50&targetId=m000005`,
    );
    await expect(page.locator("#messages-list")).toContainText(EARLY_MARKER);
    await page
      .getByRole("button", { name: "Reading settings", exact: true })
      .click();
    await expect(
      page.getByRole("dialog", { name: "Reading settings" }),
    ).toContainText("Records 1–7 of 151");
    await expect(page.locator("#messages-list")).not.toContainText(
      "message body 149",
    );
  });
});
