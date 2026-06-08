export function createRightSidebarVisibility({
  documentImpl = document,
  storage = globalThis.localStorage,
  collapsedStorageKey,
  loadScratchpad = () => {},
} = {}) {
  function isCollapsed() {
    return documentImpl.body.classList.contains('right-sidebar-collapsed');
  }

  function isExpanded() {
    return documentImpl.body.classList.contains('right-sidebar-expanded');
  }

  function setCollapsed(collapsed) {
    documentImpl.body.classList.toggle('right-sidebar-collapsed', collapsed);
    try { storage?.setItem(collapsedStorageKey, String(collapsed)); } catch {}
  }

  function setExpanded(expanded) {
    documentImpl.body.classList.toggle('right-sidebar-expanded', expanded);
  }

  function toggle() {
    if (isCollapsed()) {
      setCollapsed(false);
      loadScratchpad();
    } else {
      setCollapsed(true);
      setExpanded(false);
    }
  }

  function open() {
    if (isCollapsed()) {
      setCollapsed(false);
      loadScratchpad();
    }
  }

  function collapse() {
    setExpanded(false);
    setCollapsed(true);
  }

  function toggleExpanded() {
    if (isExpanded()) {
      setExpanded(false);
    } else {
      if (isCollapsed()) setCollapsed(false);
      setExpanded(true);
      loadScratchpad();
    }
  }

  function bindControls({
    toggleBtn = null,
    closeBtn = null,
    expandBtn = null,
    backdrop = null,
  } = {}) {
    const cleanups = [];
    const add = (target, eventName, handler) => {
      if (!target) return;
      target.addEventListener(eventName, handler);
      cleanups.push(() => target.removeEventListener(eventName, handler));
    };

    add(toggleBtn, 'click', toggle);
    add(closeBtn, 'click', collapse);
    add(expandBtn, 'click', toggleExpanded);
    add(backdrop, 'click', collapse);

    return () => {
      for (const cleanup of cleanups) cleanup();
    };
  }

  return {
    toggle,
    open,
    collapse,
    isCollapsed,
    isExpanded,
    bindControls,
  };
}
