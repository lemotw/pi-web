import { describe, expect, it, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import SessionEntry from './SessionEntry.svelte';
import { buildSessionLookups } from '../../session/data/session-data.js';

afterEach(cleanup);

function model(entries = []) {
  return { entries, renderedTools: null, ...buildSessionLookups(entries) };
}

describe('SessionEntry', () => {
  it('renders a user message with its text under an entry anchor', () => {
    const entry = { id: 'u', type: 'message', message: { role: 'user', content: 'hello' } };
    const { container } = render(SessionEntry, { props: { entry, model: model([entry]) } });
    const node = container.querySelector('#entry-u');
    expect(node).not.toBeNull();
    expect(node).toHaveClass('user-message');
    expect(node.textContent).toContain('hello');
  });

  it('renders an assistant message', () => {
    const entry = {
      id: 'a',
      type: 'message',
      message: { role: 'assistant', content: [{ type: 'text', text: 'hi' }] },
    };
    const { container } = render(SessionEntry, { props: { entry, model: model([entry]) } });
    const node = container.querySelector('#entry-a');
    expect(node).toHaveClass('assistant-message');
    expect(node.textContent).toContain('hi');
  });

  it('renders a tool result whose call is outside the window', () => {
    const entry = {
      id: 'r',
      type: 'message',
      message: { role: 'toolResult', toolCallId: 'c', content: [] },
    };
    const { container } = render(SessionEntry, { props: { entry, model: model([entry]) } });
    expect(container.querySelector('#entry-r')).toHaveTextContent(
      'Tool call is outside this page.',
    );
  });

  it('does not duplicate a tool result when its call is loaded', () => {
    const call = {
      id: 'a',
      type: 'message',
      message: {
        role: 'assistant',
        content: [{ type: 'toolCall', id: 'c', name: 'read', arguments: {} }],
      },
    };
    const entry = {
      id: 'r',
      type: 'message',
      message: { role: 'toolResult', toolCallId: 'c', content: [] },
    };
    const { container } = render(SessionEntry, { entry, model: model([call, entry]) });
    expect(container.querySelector('#entry-r')).toBeNull();
  });

  it('renders a model change but omits implicit ones', () => {
    const entry = { id: 'm', type: 'model_change', provider: 'p', modelId: 'x' };
    const { container } = render(SessionEntry, { props: { entry, model: model([entry]) } });
    expect(container.querySelector('#entry-m.model-change')?.textContent).toContain('p/x');

    cleanup();
    const implicit = {
      id: 'm2',
      type: 'model_change',
      provider: 'p',
      modelId: 'x',
      implicit: true,
    };
    const { container: c2 } = render(SessionEntry, {
      props: { entry: implicit, model: model([implicit]) },
    });
    expect(c2.querySelector('#entry-m2')).toBeNull();
  });
});
