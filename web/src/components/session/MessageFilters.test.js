import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import MessageFilters from './MessageFilters.svelte';
import SessionContent from './SessionContent.svelte';
import { SessionDataModel } from '../../session/data/session-data.svelte.js';
import { readContentFilter } from '../../session/content-filter-preferences.js';

const user = { id: 'u', type: 'message', message: { role: 'user', content: 'question' } };
const assistant = {
  id: 'a',
  parentId: 'u',
  type: 'message',
  message: {
    role: 'assistant',
    content: [
      { type: 'text', text: 'answer' },
      { type: 'thinking', thinking: 'thinking text' },
      { type: 'toolCall', id: 'r', name: 'read', arguments: { path: 'secret.txt' } },
      { type: 'toolCall', id: 'b', name: 'bash', arguments: { command: 'echo hi' } },
    ],
  },
};
const result = {
  id: 'result',
  parentId: 'a',
  type: 'message',
  message: {
    role: 'toolResult',
    toolCallId: 'r',
    toolName: 'read',
    content: [{ type: 'text', text: 'LARGE_OUTPUT_MARKER' }],
  },
};
function setup(live = true) {
  const model = new SessionDataModel({ entries: [user, assistant, result], leafId: 'result' });
  render(MessageFilters, { model });
  const view = render(SessionContent, { model, live });
  return { model, ...view };
}

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe('message display filters', () => {
  it('removes tool DOM and thinking, preserves text, and remembers the preference', async () => {
    const { container, model } = setup();
    expect(container.querySelectorAll('.tool-execution')).toHaveLength(2);
    await fireEvent.click(screen.getByRole('button', { name: 'Compact', exact: true }));
    await waitFor(() => expect(container.querySelectorAll('.tool-execution')).toHaveLength(0));
    expect(container.querySelector('.thinking-block')).toBeNull();
    expect(container).not.toHaveTextContent('LARGE_OUTPUT_MARKER');
    expect(container).toHaveTextContent('answer');
    expect(container).toHaveTextContent('question');
    expect(readContentFilter()).toMatchObject({ mode: 'text', showThinking: false });
    expect(model.entries).toHaveLength(3);
    expect(model.entries[1].message.content).toHaveLength(4);
  });

  it('hides selected tool calls and their outputs and applies to later live entries', async () => {
    const { container, model } = setup();
    await fireEvent.click(screen.getByRole('button', { name: 'Custom', exact: true }));
    await fireEvent.click(screen.getByRole('checkbox', { name: 'read' }));
    expect(container.querySelectorAll('.tool-execution')).toHaveLength(1);
    expect(container).not.toHaveTextContent('LARGE_OUTPUT_MARKER');
    expect(container).toHaveTextContent('echo hi');
    const later = { ...assistant, id: 'later', parentId: 'result' };
    model.reconcile([...model.entries, later]);
    await waitFor(() => expect(container.querySelectorAll('.tool-execution')).toHaveLength(2));
    expect(container).not.toHaveTextContent('secret.txt');
  });

  it('retains question buttons when the tool has returned awaitingChatReply', async () => {
    const { container, model } = setup();
    const question = {
      id: 'q',
      parentId: 'result',
      type: 'message',
      message: {
        role: 'assistant',
        content: [
          {
            type: 'toolCall',
            id: 'ask',
            name: 'pi_web_ask_user_question',
            arguments: {
              questions: [{ question: 'Continue?', options: [{ label: 'Yes' }, { label: 'No' }] }],
            },
          },
        ],
      },
    };
    const pending = {
      id: 'pending',
      parentId: 'q',
      type: 'message',
      message: {
        role: 'toolResult',
        toolCallId: 'ask',
        content: [],
        details: { awaitingChatReply: true },
      },
    };
    model.reconcile([...model.entries, question, pending]);
    await fireEvent.click(screen.getByRole('button', { name: 'Compact', exact: true }));
    expect(container.querySelector('.ask-question-option-action')).not.toBeNull();
    expect(screen.getByRole('button', { name: 'Yes' })).toBeInTheDocument();
  });

  it('does not apply live display preferences to static exports', async () => {
    const { container } = setup(false);
    await fireEvent.click(screen.getByRole('button', { name: 'Compact', exact: true }));
    expect(container.querySelectorAll('.tool-execution')).toHaveLength(2);
    expect(container.querySelector('.thinking-block')).not.toBeNull();
  });
});
