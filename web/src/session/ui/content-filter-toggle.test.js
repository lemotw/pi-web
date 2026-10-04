import { expect, it } from 'vitest';
import { applyToggleStateToNode } from './toggle-state.js';

it('does not let the legacy tools toggle hide an interactive question', () => {
  const node = document.createElement('div');
  node.innerHTML =
    '<div class="tool-execution" id="ordinary"></div><div class="tool-execution" id="question"><button class="ask-question-option-action">Yes</button></div>';
  applyToggleStateToNode(node, { toolsVisible: false });
  expect(node.querySelector('#ordinary').style.display).toBe('none');
  expect(node.querySelector('#question').style.display).toBe('');
});
