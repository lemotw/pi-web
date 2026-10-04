import { test, expect, collapseScratchpad } from "../lib/test";
import {
  appendEntry,
  buildSession,
  uniqueSessionName,
  writeSession,
} from "../lib/sessions";

test("filters tool DOM without hiding replies and preserves the setting on reload", async ({
  page,
  sessionsDir,
}, testInfo) => {
  const initial = buildSession();
  const call = {
    id: "filter-call",
    parentId: initial.lastId,
    type: "message",
    timestamp: new Date().toISOString(),
    message: {
      role: "assistant",
      content: [
        { type: "text", text: "FILTER_VISIBLE_REPLY" },
        { type: "thinking", thinking: "FILTER_THINKING" },
        {
          type: "toolCall",
          id: "filter-read",
          name: "read",
          arguments: { path: "example.txt" },
        },
        {
          type: "toolCall",
          id: "filter-bash",
          name: "bash",
          arguments: { command: "echo FILTER_BASH" },
        },
      ],
    },
  };
  const result = {
    id: "filter-result",
    parentId: call.id,
    type: "message",
    timestamp: new Date().toISOString(),
    message: {
      role: "toolResult",
      toolCallId: "filter-read",
      toolName: "read",
      content: [{ type: "text", text: "FILTER_TOOL_OUTPUT" }],
    },
  };
  const name = uniqueSessionName(testInfo, "filter");
  writeSession(sessionsDir, name, [...initial.entries, call, result]);
  await collapseScratchpad(page);
  await page.goto(`/session?id=${encodeURIComponent(name)}`);
  const messages = page.locator("#messages-list");
  await expect(messages.locator(".tool-execution")).toHaveCount(2);
  await page
    .getByRole("combobox", { name: "Message display" })
    .selectOption("text");
  await expect(messages.locator(".tool-execution")).toHaveCount(0);
  await expect(messages.locator(".thinking-block")).toHaveCount(0);
  await expect(messages).toContainText("FILTER_VISIBLE_REPLY");
  await page.reload();
  await expect(
    page.getByRole("combobox", { name: "Message display" }),
  ).toHaveValue("text");
  await expect(messages.locator(".tool-execution")).toHaveCount(0);

  await page
    .getByRole("combobox", { name: "Message display" })
    .selectOption("custom");
  await page.getByRole("checkbox", { name: "read", exact: true }).uncheck();
  await expect(messages.locator(".tool-execution")).toHaveCount(1);
  await expect(messages).not.toContainText("FILTER_TOOL_OUTPUT");
  await expect(messages).toContainText("FILTER_BASH");
  appendEntry(sessionsDir, name, {
    ...call,
    id: "filter-next",
    parentId: result.id,
    message: {
      ...call.message,
      content: call.message.content.map((block) =>
        block.type === "toolCall"
          ? { ...block, id: `${block.id}-next` }
          : block,
      ),
    },
  });
  await expect(messages.locator(".tool-execution")).toHaveCount(2, {
    timeout: 15000,
  });
  await expect(messages).not.toContainText("FILTER_TOOL_OUTPUT");
});
