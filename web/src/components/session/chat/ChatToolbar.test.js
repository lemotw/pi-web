import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, render } from '@testing-library/svelte';
import ChatToolbar from './ChatToolbar.svelte';

afterEach(() => {
  cleanup();
});

describe('ChatToolbar', () => {
  it('renders runtime anchors and enabled controls', () => {
    render(ChatToolbar, { props: { chatAvailable: true, modelLabel: 'gpt-test' } });

    expect(document.getElementById('pi-chat-attach').disabled).toBe(false);
    expect(document.getElementById('pi-chat-status').textContent).toBe('idle');
    expect(document.getElementById('pi-chat-thinking-label').disabled).toBe(false);
    expect(document.getElementById('pi-chat-model-label').textContent).toBe('gpt-test');
    expect(document.getElementById('pi-chat-model-label').style.display).toBe('');
    expect(document.getElementById('pi-chat-cancel').textContent).toBe('Cancel');
    expect(document.getElementById('pi-chat-send').textContent).toBe('Send');
  });

  it('renders unavailable state and hides an empty model label', () => {
    render(ChatToolbar, { props: { chatAvailable: false, modelLabel: '' } });

    expect(document.getElementById('pi-chat-attach').disabled).toBe(true);
    expect(document.getElementById('pi-chat-status').textContent).toBe('unavailable');
    expect(document.getElementById('pi-chat-thinking-label').disabled).toBe(true);
    expect(document.getElementById('pi-chat-model-label').disabled).toBe(true);
    expect(document.getElementById('pi-chat-model-label').style.display).toBe('none');
    expect(document.getElementById('pi-chat-cancel').disabled).toBe(true);
  });
});
