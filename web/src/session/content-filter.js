export function normalizeContentFilter(value) {
  return {
    mode: ['all', 'text', 'custom'].includes(value?.mode) ? value.mode : 'all',
    hiddenTools: Array.isArray(value?.hiddenTools)
      ? [...new Set(value.hiddenTools.filter((name) => typeof name === 'string'))]
      : [],
    showThinking: typeof value?.showThinking === 'boolean' ? value.showThinking : true,
  };
}

export function buildContentToolIndex(entries = []) {
  const calls = new Map();
  const results = new Map();
  const names = new Set();
  for (const entry of entries) {
    const message = entry?.message;
    if (message?.role === 'toolResult') {
      results.set(message.toolCallId, entry);
      if (message.toolName) names.add(message.toolName);
    }
    if (message?.role === 'bashExecution') names.add('bash');
    if (message?.role !== 'assistant' || !Array.isArray(message.content)) continue;
    for (const block of message.content) {
      if (block.type !== 'toolCall') continue;
      calls.set(block.id, block);
      names.add(block.name);
    }
  }
  return { calls, results, names: [...names].filter(Boolean).sort() };
}

function showTool(call, filter, index) {
  // Hiding an unanswered question would prevent the user from continuing.
  if (['ask_user_question', 'pi_web_ask_user_question'].includes(call.name)) {
    const result = index.results.get(call.id)?.message;
    if (!result || result.isError || result.details?.awaitingChatReply || result.details?.cancelled)
      return true;
  }
  return (
    filter.mode === 'all' || (filter.mode === 'custom' && !filter.hiddenTools.includes(call.name))
  );
}

// Return a view-only copy, never mutate canonical entries or tool lookups.
// Removed blocks never instantiate ToolCall/ToolOutput or parse their markup.
export function filterContentEntry(entry, filter, index) {
  if (filter.mode === 'all' && filter.showThinking) return entry;
  const message = entry.message;
  if (!message) return entry;
  if (message.role === 'bashExecution')
    return showTool({ name: 'bash' }, filter, index) ? entry : null;
  if (message.role === 'toolResult') {
    const call = index.calls.get(message.toolCallId) || {
      name: message.toolName,
      id: message.toolCallId,
    };
    return showTool(call, filter, index) ? entry : null;
  }
  if (message.role !== 'assistant') return entry;
  const blocks =
    typeof message.content === 'string'
      ? [{ type: 'text', text: message.content }]
      : message.content || [];
  const content = blocks.filter((block) => {
    if (block.type === 'toolCall') return showTool(block, filter, index);
    if (block.type === 'thinking') return filter.showThinking && !!block.thinking?.trim();
    if (block.type === 'text') return !!block.text?.trim();
    return true;
  });
  if (!content.length && !['error', 'aborted'].includes(message.stopReason)) return null;
  return { ...entry, message: { ...message, content } };
}
