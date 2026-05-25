import { describe, expect, it } from 'vitest';
import { buildActivePathIds, buildTree, buildTreePrefix, buildTreeNodeMap, findNewestLeaf, flattenTree, getPath } from './session-tree.js';

const entries = [
  { id: 'root', timestamp: '2026-01-01T00:00:00Z' },
  { id: 'old', parentId: 'root', timestamp: '2026-01-01T00:01:00Z' },
  { id: 'new', parentId: 'root', timestamp: '2026-01-01T00:02:00Z' },
  { id: 'leaf', parentId: 'new', timestamp: '2026-01-01T00:03:00Z' },
  { id: 'orphan', parentId: 'missing', timestamp: '2026-01-01T00:04:00Z' }
];

function byId() {
  return new Map(entries.map((entry) => [entry.id, entry]));
}

describe('session tree helpers', () => {
  it('builds roots, children, labels, and timestamp ordering', () => {
    const roots = buildTree(entries, new Map([['new', 'label']]));
    expect(roots.map((n) => n.entry.id)).toEqual(['root', 'orphan']);
    expect(roots[0].children.map((n) => n.entry.id)).toEqual(['old', 'new']);
    expect(roots[0].children[1].label).toBe('label');
  });

  it('ignores metadata entries without ids', () => {
    const roots = buildTree([...entries, { type: 'session_info', name: 'Renamed' }]);
    const flat = flattenTree(roots, buildActivePathIds('leaf', byId()));
    expect(flat.map((f) => f.node.entry.id)).toEqual(['root', 'new', 'leaf', 'old', 'orphan']);
  });

  it('builds active path and path entries from leaf to root', () => {
    expect([...buildActivePathIds('leaf', byId())]).toEqual(['leaf', 'new', 'root']);
    expect(getPath('leaf', byId()).map((e) => e.id)).toEqual(['root', 'new', 'leaf']);
  });

  it('finds newest reachable leaf', () => {
    const roots = buildTree(entries);
    expect(findNewestLeaf('root', buildTreeNodeMap(roots))).toBe('leaf');
    expect(findNewestLeaf('missing', roots)).toBe('missing');
  });

  it('flattens active branch first and builds prefixes', () => {
    const roots = buildTree(entries);
    const flat = flattenTree(roots, buildActivePathIds('leaf', byId()));
    expect(flat.map((f) => f.node.entry.id)).toEqual(['root', 'new', 'leaf', 'old', 'orphan']);
    expect(buildTreePrefix(flat[1])).toContain('├');
  });
});
