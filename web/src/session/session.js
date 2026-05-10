import { marked } from 'marked';
import hljs from 'highlight.js';

import { loadSessionData } from './data/session-data.js';
import { buildActivePathIds as buildActivePathIdsForModel, buildTree as buildTreeForModel, buildTreeNodeMap, buildTreePrefix, findNewestLeaf as findNewestLeafInTree, flattenTree, getPath as getPathForModel } from './tree/session-tree.js';
import { extractContent, filterNodes as filterNodesForState, getSearchableText, hasTextContent, recalculateVisualStructure } from './tree/session-filter.js';
import { escapeHtml, formatToolCall, getTreeNodeDisplayHtml as getTreeNodeDisplayHtmlForState, shortenPath, truncate } from './render/session-format.js';
import { createTreeRenderer } from './tree/tree-renderer.js';
import renderEntryJs from './legacy/render-entry.js?raw';
import headerJs from './legacy/header.js?raw';
import { createSessionNavigator } from './navigation/session-navigation.js';
import uiJs from './legacy/ui.js?raw';
import chatJs from './legacy/chat.js?raw';
import liveReloadJs from '../../../live_templates/live_reload.js?raw';
export { buildSessionLookups, createSessionDataModel, decodeBase64JSON, getSessionSearchParams, loadSessionData, readSessionPayload } from './data/session-data.js';
export { buildActivePathIds, buildTree, buildTreeNodeMap, buildTreePrefix, findNewestLeaf, flattenTree, getPath } from './tree/session-tree.js';
export { createTreeRenderer } from './tree/tree-renderer.js';
export { createSessionNavigator } from './navigation/session-navigation.js';
export { extractContent, filterNodes, getSearchableText, hasTextContent, recalculateVisualStructure } from './tree/session-filter.js';
export { escapeHtml, formatToolCall, getTreeNodeDisplayHtml, shortenPath, truncate } from './render/session-format.js';

export const sessionEntrypointLoaded = true;

export const sessionDataPrelude = `
const __piSessionData = window.__piSessionDataModel;
const data = __piSessionData.payload;
const urlParams = __piSessionData.params;
const header = __piSessionData.header;
const entries = __piSessionData.entries;
const defaultLeafId = __piSessionData.defaultLeafId;
const urlLeafId = __piSessionData.urlLeafId;
const urlTargetId = __piSessionData.urlTargetId;
const leafId = __piSessionData.leafId;
const systemPrompt = __piSessionData.systemPrompt;
const tools = __piSessionData.tools;
const renderedTools = __piSessionData.renderedTools;
const byId = __piSessionData.byId;
const toolCallMap = __piSessionData.toolCallMap;
const labelMap = __piSessionData.labelMap;
function buildTree() { return window.__piSessionTree.buildTree(); }
function buildActivePathIds(targetId) { return window.__piSessionTree.buildActivePathIds(targetId); }
function getPath(targetId) { return window.__piSessionTree.getPath(targetId); }
let treeNodeMap = null;
function findNewestLeaf(nodeId) { return window.__piSessionTree.findNewestLeaf(nodeId); }
const flattenTree = window.__piSessionTree.flattenTree;
const buildTreePrefix = window.__piSessionTree.buildTreePrefix;
let filterMode = 'default';
let searchQuery = '';
window.__piFilterState = { filterMode, searchQuery };
const hasTextContent = window.__piSessionFilter.hasTextContent;
const extractContent = window.__piSessionFilter.extractContent;
const getSearchableText = window.__piSessionFilter.getSearchableText;
const recalculateVisualStructure = window.__piSessionFilter.recalculateVisualStructure;
function filterNodes(flatNodes, currentLeafId) { return window.__piSessionFilter.filterNodes(flatNodes, currentLeafId, { filterMode, searchQuery }); }
const shortenPath = window.__piSessionFormat.shortenPath;
const formatToolCall = window.__piSessionFormat.formatToolCall;
const escapeHtml = window.__piSessionFormat.escapeHtml;
const truncate = window.__piSessionFormat.truncate;
function getTreeNodeDisplayHtml(entry, label) { return window.__piSessionFormat.getTreeNodeDisplayHtml(entry, label); }
const __piTreeRenderer = window.__piTreeRenderer;
let currentLeafId = __piTreeRenderer.currentLeafId;
let currentTargetId = __piTreeRenderer.currentTargetId;
function __syncTreeRendererState() { window.__piFilterState.filterMode = filterMode; window.__piFilterState.searchQuery = searchQuery; __piTreeRenderer.currentLeafId = currentLeafId; __piTreeRenderer.currentTargetId = currentTargetId; }
function renderTree() { __syncTreeRendererState(); return __piTreeRenderer.renderTree(); }
function forceTreeRerender() { __syncTreeRendererState(); return __piTreeRenderer.forceTreeRerender(); }
function __getPiSessionNavigator() {
  if (!window.__piSessionNavigator) {
    window.__piSessionNavigator = window.__createSessionNavigator({
      documentImpl: document,
      windowImpl: window,
      getPath,
      renderTree,
      renderHeader,
      attachHeaderHandlers,
      renderEntry,
      buildShareUrl,
      copyToClipboard,
      onNavigate: (leaf, targetId) => {
        currentLeafId = leaf;
        currentTargetId = targetId;
        __piTreeRenderer.currentLeafId = leaf;
        __piTreeRenderer.currentTargetId = targetId;
      }
    });
  }
  return window.__piSessionNavigator;
}
function renderEntryToNode(entry) { return __getPiSessionNavigator().renderEntryToNode(entry); }
function navigateTo(targetId, scrollMode = 'target', scrollToEntryId = null) { return __getPiSessionNavigator().navigateTo(targetId, scrollMode, scrollToEntryId); }
`;

export const legacySessionSources = [
  renderEntryJs,
  headerJs,
  uiJs,
  chatJs
];

export const liveReloadSource = liveReloadJs;

export function runLegacySessionApp({ target = window } = {}) {
  target.marked = target.marked || marked;
  target.hljs = target.hljs || hljs;
  target.__piSessionDataModel = target.__piSessionDataModel || loadSessionData({
    documentImpl: target.document,
    windowImpl: target,
    atobImpl: target.atob?.bind(target)
  });
  target.__piSessionFormat = target.__piSessionFormat || {
    shortenPath,
    formatToolCall,
    escapeHtml: (text) => escapeHtml(text, { documentImpl: target.document }),
    truncate,
    getTreeNodeDisplayHtml: (entry, label) => getTreeNodeDisplayHtmlForState(entry, label, {
      extractContent,
      toolCallMap: target.__piSessionDataModel.toolCallMap,
      escapeHtmlImpl: (text) => escapeHtml(text, { documentImpl: target.document })
    })
  };
  target.__piSessionFilter = target.__piSessionFilter || {
    hasTextContent,
    extractContent,
    getSearchableText,
    recalculateVisualStructure,
    filterNodes: filterNodesForState
  };
  target.__piSessionTree = target.__piSessionTree || {
    buildTree: () => buildTreeForModel(target.__piSessionDataModel.entries, target.__piSessionDataModel.labelMap),
    buildActivePathIds: (targetId) => buildActivePathIdsForModel(targetId, target.__piSessionDataModel.byId),
    getPath: (targetId) => getPathForModel(targetId, target.__piSessionDataModel.byId),
    findNewestLeaf: (nodeId) => {
      const roots = buildTreeForModel(target.__piSessionDataModel.entries, target.__piSessionDataModel.labelMap);
      return findNewestLeafInTree(nodeId, buildTreeNodeMap(roots));
    },
    flattenTree,
    buildTreePrefix
  };
  target.__createSessionNavigator = createSessionNavigator;
  target.__piTreeRenderer = target.__piTreeRenderer || createTreeRenderer({
    documentImpl: target.document,
    windowImpl: target,
    initialLeafId: target.__piSessionDataModel.leafId,
    initialTargetId: target.__piSessionDataModel.urlTargetId || target.__piSessionDataModel.leafId,
    buildTree: target.__piSessionTree.buildTree,
    buildActivePathIds: target.__piSessionTree.buildActivePathIds,
    flattenTree,
    filterNodes: (flatNodes, currentLeafId) => target.__piSessionFilter.filterNodes(flatNodes, currentLeafId, { filterMode: target.__piFilterState?.filterMode || 'default', searchQuery: target.__piFilterState?.searchQuery || '' }),
    buildTreePrefix,
    getTreeNodeDisplayHtml: target.__piSessionFormat.getTreeNodeDisplayHtml,
    findNewestLeaf: target.__piSessionTree.findNewestLeaf,
    navigateTo: (...args) => target.navigateTo?.(...args),
    isMobileLayout: () => target.isMobileLayout?.() || false,
    closeSidebar: () => target.closeSidebar?.()
  });
  const bundle = `(function() {\n'use strict';\n${sessionDataPrelude}\n${legacySessionSources.join('\n')}\n})();\n${liveReloadJs}`;
  // Keep the old ordered session app running as a Vite-owned asset while the
  // internals are split into testable modules.
  return target.Function(bundle)();
}

if (typeof window !== 'undefined' && typeof document !== 'undefined' && document.getElementById('session-data')) {
  runLegacySessionApp();
}
