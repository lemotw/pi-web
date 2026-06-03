import { describe, expect, it, vi, beforeEach } from 'vitest';
import { isSkillTrigger, extractSkills, renderSkillList, setupSkillList, SKILL_LOAD_BUTTON_ID } from './skill-list.js';

describe('isSkillTrigger', () => {
  it('matches exactly /skill ignoring surrounding whitespace', () => {
    expect(isSkillTrigger('/skill')).toBe(true);
    expect(isSkillTrigger('  /skill  ')).toBe(true);
    expect(isSkillTrigger('/skill foo')).toBe(false);
    expect(isSkillTrigger('/ski')).toBe(false);
    expect(isSkillTrigger(null)).toBe(false);
  });
});

describe('extractSkills', () => {
  it('keeps only skill-source commands and strips the skill: prefix', () => {
    const skills = extractSkills([
      { name: 'skill:foo', source: 'skill', description: 'Foo' },
      { name: 'compact', source: 'extension' },
      { name: 'skill:bar', source: 'skill' },
    ]);
    expect(skills).toEqual([
      { name: 'skill:foo', displayName: 'foo', description: 'Foo' },
      { name: 'skill:bar', displayName: 'bar', description: '' },
    ]);
  });

  it('tolerates non-arrays', () => {
    expect(extractSkills(null)).toEqual([]);
  });
});

describe('renderSkillList', () => {
  it('shows a Load skills button when the worker is not ready', () => {
    const html = renderSkillList([], { workerReady: false });
    expect(html).toContain(SKILL_LOAD_BUTTON_ID);
    expect(html).toContain('Load skills');
  });

  it('shows empty state when ready with no skills', () => {
    expect(renderSkillList([], { workerReady: true })).toContain('No skills loaded');
  });

  it('renders one item per skill', () => {
    const html = renderSkillList([{ name: 'skill:foo', displayName: 'foo', description: 'Do foo' }], { workerReady: true });
    expect(html).toContain('data-skill="skill:foo"');
    expect(html).toContain('foo');
    expect(html).toContain('Do foo');
  });
});

describe('setupSkillList', () => {
  beforeEach(() => {
    document.body.innerHTML = `
      <textarea id="pi-chat-message"></textarea>
      <div id="pi-chat-skill-popup" style="display:none"><div id="pi-chat-skill-list"></div></div>
    `;
  });

  function makeChatApi(payload) {
    return {
      getCommands: vi.fn(() => Promise.resolve(new Response(JSON.stringify(payload), { status: 200 }))),
    };
  }

  it('shows skills when the value is exactly /skill', async () => {
    const chatApi = makeChatApi({ commands: [{ name: 'skill:foo', source: 'skill' }], workerReady: true });
    const api = setupSkillList({ documentImpl: document, sessionId: 's', chatApi });
    await api.maybeShow('/skill');
    const popup = document.getElementById('pi-chat-skill-popup');
    expect(popup.style.display).toBe('block');
    expect(popup.innerHTML).toContain('data-skill="skill:foo"');
    expect(chatApi.getCommands).toHaveBeenCalledWith('s', { load: false });
  });

  it('hides and does not fetch for non-trigger input', async () => {
    const chatApi = makeChatApi({ commands: [], workerReady: true });
    const api = setupSkillList({ documentImpl: document, sessionId: 's', chatApi });
    await api.maybeShow('hello');
    expect(chatApi.getCommands).not.toHaveBeenCalled();
    expect(document.getElementById('pi-chat-skill-popup').style.display).toBe('none');
  });

  it('insertSkill writes the slash invocation and closes', () => {
    const api = setupSkillList({ documentImpl: document, sessionId: 's', chatApi: makeChatApi({}) });
    document.getElementById('pi-chat-skill-popup').style.display = 'block';
    api.insertSkill('skill:foo');
    expect(document.getElementById('pi-chat-message').value).toBe('/skill:foo ');
    expect(document.getElementById('pi-chat-skill-popup').style.display).toBe('none');
  });

  it('load fetches with load:true to spawn the worker', async () => {
    const chatApi = makeChatApi({ commands: [], workerReady: true });
    const api = setupSkillList({ documentImpl: document, sessionId: 's', chatApi });
    await api.load();
    expect(chatApi.getCommands).toHaveBeenCalledWith('s', { load: true });
  });
});
