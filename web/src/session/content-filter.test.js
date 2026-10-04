import { describe, expect, it } from 'vitest';
import {
  buildContentToolIndex,
  filterContentEntry,
  normalizeContentFilter,
} from './content-filter.js';

const text = { type: 'text', text: 'answer' };
const thinking = { type: 'thinking', thinking: 'thinking text' };
const read = { type: 'toolCall', id: 'r', name: 'read', arguments: { path: 'x' } };
const bash = { type: 'toolCall', id: 'b', name: 'bash', arguments: { command: 'ls' } };
const entry = {
  id: 'a',
  type: 'message',
  message: { role: 'assistant', content: [text, thinking, read, bash] },
};
const result = {
  id: 'result',
  type: 'message',
  message: {
    role: 'toolResult',
    toolCallId: 'r',
    toolName: 'read',
    content: [{ type: 'text', text: 'output' }],
  },
};
const index = buildContentToolIndex([entry, result]);
const filter = (mode, extra = {}) => normalizeContentFilter({ mode, ...extra });

describe('content filtering', () => {
  it('indexes discovered names and results once', () => {
    expect(index.names).toEqual(['bash', 'read']);
    expect(index.results.get('r')).toBe(result);
  });

  it('keeps original objects in the default view', () => {
    expect(filterContentEntry(entry, filter('all'), index)).toBe(entry);
  });

  it('removes tool blocks and thinking without touching original entries', () => {
    const view = filterContentEntry(entry, filter('text', { showThinking: false }), index);
    expect(view.message.content).toEqual([text]);
    expect(entry.message.content).toEqual([text, thinking, read, bash]);
    expect(filterContentEntry(result, filter('text'), index)).toBeNull();
  });

  it('filters selected tools and preserves unrelated text and newly discovered tools', () => {
    const custom = filter('custom', { hiddenTools: ['read'] });
    expect(filterContentEntry(entry, custom, index).message.content).toEqual([
      text,
      thinking,
      bash,
    ]);
    expect(filterContentEntry(result, custom, index)).toBeNull();
    const execution = { message: { role: 'bashExecution', output: 'done' } };
    expect(filterContentEntry(execution, custom, index)).toBe(execution);
    expect(filterContentEntry(execution, filter('text'), index)).toBeNull();
  });

  it('omits empty tool-only assistant wrappers but keeps failure states', () => {
    const toolOnly = { ...entry, message: { ...entry.message, content: [read] } };
    expect(filterContentEntry(toolOnly, filter('text'), index)).toBeNull();
    const error = {
      ...toolOnly,
      message: { ...toolOnly.message, stopReason: 'error', errorMessage: 'Failed' },
    };
    expect(filterContentEntry(error, filter('text'), index).message.errorMessage).toBe('Failed');
  });

  it.each(['ask_user_question', 'pi_web_ask_user_question'])(
    'preserves interactive %s even when explicitly hidden',
    (name) => {
      const call = { type: 'toolCall', id: 'q', name, arguments: { questions: [] } };
      const question = { ...entry, message: { role: 'assistant', content: [call] } };
      for (const details of [null, { awaitingChatReply: true }, { cancelled: true }]) {
        const records = [question];
        if (details) records.push({ message: { role: 'toolResult', toolCallId: 'q', details } });
        const view = filterContentEntry(
          question,
          filter('custom', { hiddenTools: [name] }),
          buildContentToolIndex(records),
        );
        expect(view.message.content).toEqual([call]);
      }
      const answered = {
        message: { role: 'toolResult', toolCallId: 'q', details: { answers: { question: 'yes' } } },
      };
      expect(
        filterContentEntry(question, filter('text'), buildContentToolIndex([question, answered])),
      ).toBeNull();
    },
  );

  it('normalizes corrupt preferences and supports string assistant content', () => {
    expect(
      normalizeContentFilter({
        mode: 'bad',
        hiddenTools: [null, 'bash', 'bash'],
        showThinking: 'false',
      }),
    ).toEqual({ mode: 'all', hiddenTools: ['bash'], showThinking: true });
    const stringEntry = { message: { role: 'assistant', content: 'answer' } };
    expect(filterContentEntry(stringEntry, filter('text'), index).message.content).toEqual([text]);
  });
});
