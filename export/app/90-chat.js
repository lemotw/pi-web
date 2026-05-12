let onWorkerModelUpdate = null;
let knownModelLabel = "";
let knownThinkingLevel = "";
let currentModelForThinking = null;

function setChatStatus(text, cls) {
  const status = document.getElementById("pi-chat-status");
  const cancelButton = document.getElementById("pi-chat-cancel");
  const isRunning =
    cls === "running" ||
    text === "running" ||
    text === "sending" ||
    text === "accepted" ||
    text === "cancelling";
  if (status) {
    status.textContent = text;
    status.className = "pi-chat-status" + (cls ? " " + cls : "");
  }
  if (cancelButton) {
    cancelButton.style.display = isRunning ? "" : "none";
    cancelButton.disabled = text === "cancelling";
  }
}

function setModelLabel(label) {
  const btn = document.getElementById("pi-chat-model-label");
  if (!btn) return;
  if (label) {
    btn.textContent = label;
    btn.style.display = "";
  } else {
    btn.style.display = "none";
  }
}

const THINKING_LEVELS = ["off", "minimal", "low", "medium", "high", "xhigh"];
const THINKING_COLORS = {
  off: "var(--thinkingOff)",
  minimal: "var(--thinkingMinimal)",
  low: "var(--thinkingLow)",
  medium: "var(--thinkingMedium)",
  high: "var(--thinkingHigh)",
  xhigh: "var(--thinkingXhigh)",
};

function setThinkingLabel(level) {
  const btn = document.getElementById("pi-chat-thinking-label");
  if (!btn) return;
  if (level) {
    btn.textContent = level;
    btn.style.display = "";
    btn.className = "pi-chat-thinking-label thinking-" + level;
  } else {
    btn.style.display = "none";
  }
}

function showCwdToast(message, isError) {
  const composer = document.getElementById("pi-chat-composer");
  if (!composer) return;
  var notice = document.getElementById("pi-chat-cwd-toast");
  if (!notice) {
    notice = document.createElement("div");
    notice.id = "pi-chat-cwd-toast";
    notice.style.cssText =
      "position:fixed;top:8px;right:8px;z-index:200;padding:2px 8px;font-size:10px;font-family:inherit;background:var(--accent);color:var(--body-bg);border-radius:3px;opacity:0;transition:opacity 0.3s;pointer-events:none;";
    document.body.appendChild(notice);
  }
  notice.textContent = message;
  notice.style.background = isError ? "var(--error)" : "var(--accent)";
  notice.style.opacity = "1";
  clearTimeout(notice._hideTimer);
  notice._hideTimer = setTimeout(function () {
    notice.style.opacity = "0";
    setTimeout(function () {
      if (notice.parentNode) notice.parentNode.removeChild(notice);
    }, 300);
  }, 1200);
}

function setupCwdCopy() {
  var cwdEl = document.querySelector(".pi-chat-cwd");
  if (!cwdEl) {
    console.log("[pi-chat] no .pi-chat-cwd element found");
    return;
  }
  console.log("[pi-chat] attaching cwd copy handler to", cwdEl);
  cwdEl.addEventListener("click", async function () {
    var path = cwdEl.dataset.cwd || cwdEl.textContent.replace(/^cwd:\s*/, "");
    console.log("[pi-chat] cwd clicked, path =", path);
    var ok = false;
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(path);
        ok = true;
        console.log("[pi-chat] clipboard API success");
      }
    } catch (err) {
      console.error("[pi-chat] Clipboard API failed:", err);
    }
    if (!ok) {
      try {
        var ta = document.createElement("textarea");
        ta.value = path;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        ok = document.execCommand("copy");
        document.body.removeChild(ta);
        console.log("[pi-chat] execCommand copy result:", ok);
      } catch (err) {
        console.error("[pi-chat] execCommand copy failed:", err);
      }
    }
    if (ok) {
      showCwdToast("Path copied");
    } else {
      showCwdToast("Copy failed", true);
    }
  });
}

function setupPiChatComposer() {
  const form = document.getElementById("pi-chat-composer");
  if (!form) return false;
  const sessionId = form.dataset.sessionId;
  const chatAvailable = form.dataset.chatAvailable !== "false";
  if (!chatAvailable) {
    const reason = form.dataset.chatDisabledReason || "chat unavailable";
    setChatStatus("unavailable", "error");
    form.title = reason;
    return false;
  }
  const textarea = document.getElementById("pi-chat-message");
  const fileInput = document.getElementById("pi-chat-images");
  const attachButton = document.getElementById("pi-chat-attach");
  const attachmentList = document.getElementById("pi-chat-attachments");
  const status = document.getElementById("pi-chat-status");
  const sendButton = document.getElementById("pi-chat-send");
  const cancelButton = document.getElementById("pi-chat-cancel");
  let selectedChatFiles = [];

  function setStatus(text, cls) {
    setChatStatus(text, cls);
  }

  function fileKey(file) {
    return [file.name, file.size, file.lastModified].join(":");
  }

  function isMobileTextInputMode() {
    return !!(
      window.matchMedia &&
      window.matchMedia("(hover: none) and (pointer: coarse)").matches
    );
  }

  function renderAttachments() {
    attachmentList.innerHTML = "";
    selectedChatFiles.forEach((file, index) => {
      const item = document.createElement("span");
      item.className = "pi-chat-attachment";
      const name = document.createElement("span");
      name.className = "pi-chat-attachment-name";
      name.textContent = "▧ " + file.name;
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "pi-chat-remove";
      remove.setAttribute("aria-label", "Remove " + file.name);
      remove.textContent = "×";
      remove.addEventListener("click", () => {
        selectedChatFiles.splice(index, 1);
        renderAttachments();
      });
      item.append(name, remove);
      attachmentList.appendChild(item);
    });
  }

  attachButton.addEventListener("click", () => fileInput.click());

  if (cancelButton) {
    cancelButton.addEventListener("click", async () => {
      cancelButton.disabled = true;
      setStatus("cancelling", "running");
      try {
        const response = await fetch(
          "/api/chat/cancel?id=" + encodeURIComponent(sessionId),
          { method: "POST" },
        );
        const data = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(data.error || "cancel failed");
        setStatus("idle", "");
        refreshWorkerStatus();
      } catch (error) {
        setStatus(error.message || String(error), "error");
      } finally {
        cancelButton.disabled = false;
      }
    });
  }
  fileInput.addEventListener("change", () => {
    const seen = new Set(selectedChatFiles.map(fileKey));
    for (const file of fileInput.files) {
      if (!seen.has(fileKey(file))) {
        selectedChatFiles.push(file);
        seen.add(fileKey(file));
      }
    }
    fileInput.value = "";
    renderAttachments();
  });
  textarea.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey) {
      if (isMobileTextInputMode()) return;
      event.preventDefault();
      form.requestSubmit();
    }
  });

  async function sendChatMessage(message, files = selectedChatFiles) {
    if (!message && files.length === 0) {
      setStatus("message or image required", "error");
      return false;
    }
    const body = new FormData();
    body.set("message", message);
    for (const file of files) body.append("images", file);
    sendButton.disabled = true;
    setStatus("sending", "running");
    window.dispatchEvent(new CustomEvent("pi-chat-message-sent"));
    try {
      const response = await fetch(
        "/api/chat?id=" + encodeURIComponent(sessionId),
        { method: "POST", body },
      );
      const data = await response.json();
      if (!response.ok) throw new Error(data.error || "chat request failed");
      setStatus("accepted", "running");
      return true;
    } catch (error) {
      setStatus(error.message || String(error), "error");
      return false;
    } finally {
      sendButton.disabled = false;
    }
  }

  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    const message = textarea.value.trim();
    const sent = await sendChatMessage(message);
    if (sent) {
      textarea.value = "";
      selectedChatFiles = [];
      fileInput.value = "";
      renderAttachments();
    }
  });

  document.addEventListener("click", async (event) => {
    // Submit button: send all collected answers
    const submitBtn = event.target.closest?.(".ask-question-submit-btn");
    if (submitBtn) {
      event.preventDefault();
      const card = submitBtn.closest(".ask-question-card");
      if (!card) return;
      const parts = [];
      card.querySelectorAll(".ask-question-block").forEach((block) => {
        const questionText = block.dataset.questionText || "";
        const sel = block.querySelector(".ask-question-option-action.selected");
        if (sel && questionText)
          parts.push(`"${questionText}" = "${sel.dataset.answer || ""}"`);
      });
      if (parts.length === 0) return;
      card.querySelectorAll(".ask-question-option-action").forEach((b) => {
        b.disabled = true;
      });
      submitBtn.disabled = true;
      const sent = await sendChatMessage(parts.join("\n"), []);
      if (!sent) {
        card.querySelectorAll(".ask-question-option-action").forEach((b) => {
          b.disabled = false;
        });
        submitBtn.disabled = false;
      }
      return;
    }

    // Option click
    const option = event.target.closest?.(".ask-question-option-action");
    if (!option) return;
    event.preventDefault();

    const card = option.closest(".ask-question-card");
    const block = option.closest(".ask-question-block");
    const questionCount = parseInt(card?.dataset.questionCount || "1", 10);

    if (questionCount === 1) {
      // Single question: send immediately
      const question = option.dataset.question || "Question";
      const answer = option.dataset.answer || option.textContent.trim();
      option.disabled = true;
      const sent = await sendChatMessage(`"${question}" = "${answer}"`, []);
      if (!sent) option.disabled = false;
      return;
    }

    // Multi-question: mark selection, show submit button
    if (block) {
      block
        .querySelectorAll(".ask-question-option-action")
        .forEach((b) => b.classList.remove("selected"));
      option.classList.add("selected");
    }
    const actions = card?.querySelector(".ask-question-actions");
    if (actions) actions.style.display = "";
  });

  let workerStatusInflight = false;
  async function refreshWorkerStatus() {
    if (workerStatusInflight) return;
    workerStatusInflight = true;
    try {
      const response = await fetch(
        "/api/worker-status?id=" + encodeURIComponent(sessionId),
      );
      if (!response.ok) return;
      const data = await response.json();
      const apiModelLabel = data.model
        ? data.model + (data.modelProvider ? " @ " + data.modelProvider : "")
        : "";
      if (apiModelLabel) knownModelLabel = apiModelLabel;
      if (data.thinkingLevel) knownThinkingLevel = data.thinkingLevel;
      if (data.state === "running") setStatus("running", "running");
      if (data.state === "idle") setStatus("idle", "");
      if (data.state === "error")
        setStatus(data.error || "worker error", "error");
      setModelLabel(knownModelLabel);
      setThinkingLabel(knownThinkingLevel);
      if (data.modelProvider && data.model && onWorkerModelUpdate) {
        onWorkerModelUpdate(data.modelProvider, data.model);
      }
    } catch {
      setStatus("status unavailable", "error");
    } finally {
      workerStatusInflight = false;
    }
  }

  setInterval(refreshWorkerStatus, 3000);
  refreshWorkerStatus();
  return true;
}

function initPiChatControls() {
  setupCwdCopy();
  if (!setupPiChatComposer()) return;
  loadModelSelector();
  setupThinkingLevelSelector();
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", initPiChatControls);
} else {
  initPiChatControls();
}

// Model selector
async function loadModelSelector() {
  const sessionId =
    new URLSearchParams(window.location.search).get("id") ||
    (document.getElementById("pi-chat-composer") || {}).dataset?.sessionId ||
    "";
  try {
    const res = await fetch("/api/models");
    const data = await res.json();
    if (!res.ok || !data.models) return;

    let allModels = data.models;
    let selectedModel = null;

    function isScoped(m) {
      return !!(m.isScoped || m.scoped || m.scope);
    }

    function setSelected(m) {
      selectedModel = m;
      currentModelForThinking = m || null;
    }

    function updateToggleFromStatus(provider, modelId) {
      if (!provider || !modelId) return;
      const m = allModels.find(function (x) {
        return (
          (x.provider || "") === provider &&
          ((x.id || "") === modelId || (x.modelId || "") === modelId)
        );
      });
      if (m) setSelected(m);
    }

    onWorkerModelUpdate = updateToggleFromStatus;

    // Chat toolbar popup
    const popup = document.getElementById("pi-chat-model-popup");
    const popupSearch = document.getElementById("pi-chat-model-search");
    const popupList = document.getElementById("pi-chat-model-list");

    function renderPopupList(filter) {
      if (!popupList) return;
      let activeIdx = -1;
      const q = (filter || "").toLowerCase();
      const byProvider = {};
      allModels.forEach(function (m) {
        if (q) {
          const name = (m.name || m.id || m.modelId || "").toLowerCase();
          const prov = (m.provider || "").toLowerCase();
          if (!name.includes(q) && !prov.includes(q)) return;
        }
        const p = m.provider || "unknown";
        if (!byProvider[p]) byProvider[p] = [];
        byProvider[p].push(m);
      });
      const providers = Object.keys(byProvider).sort();
      let html = "";
      if (providers.length === 0) {
        html = '<div class="model-empty">No models match</div>';
      } else {
        providers.forEach(function (provider) {
          html += `<div class="model-provider">${escapeHtml(provider)}</div>`;
          byProvider[provider].forEach(function (m) {
            const id = m.id || m.modelId || "";
            const name = m.name || id;
            const scoped = isScoped(m)
              ? '<span class="model-scope-badge">scoped</span>'
              : "";
            const active =
              selectedModel &&
              selectedModel.provider === provider &&
              (selectedModel.id === id || selectedModel.modelId === id)
                ? " selected"
                : "";
            html += `<button type="button" class="model-item${active}" data-provider="${escapeHtml(provider)}" data-model-id="${escapeHtml(id)}">${escapeHtml(name)}${scoped}</button>`;
          });
        });
      }
      popupList.innerHTML = html;
      activeIdx = -1;
    }

    function openPopup() {
      if (!popup) return;
      popup.style.display = "flex";
      if (popupSearch) {
        popupSearch.value = "";
        popupSearch.focus();
      }
      renderPopupList("");
    }

    function closePopup() {
      if (popup) popup.style.display = "none";
    }

    const modelLabelBtn = document.getElementById("pi-chat-model-label");
    if (modelLabelBtn) {
      modelLabelBtn.addEventListener("click", function (e) {
        e.stopPropagation();
        if (popup && popup.style.display !== "none") {
          closePopup();
        } else {
          openPopup();
        }
      });
    }

    if (popupSearch) {
      popupSearch.addEventListener("input", function () {
        renderPopupList(popupSearch.value);
      });
      popupSearch.addEventListener("keydown", function (e) {
        const items = popupList
          ? popupList.querySelectorAll(".model-item")
          : [];
        let popupActive = parseInt(
          (popupList && popupList.dataset.activeIndex) || "-1",
          10,
        );
        if (e.key === "ArrowDown") {
          e.preventDefault();
          popupActive = Math.min(popupActive + 1, items.length - 1);
        } else if (e.key === "ArrowUp") {
          e.preventDefault();
          popupActive = Math.max(popupActive - 1, 0);
        } else if (e.key === "Enter") {
          e.preventDefault();
          if (popupActive >= 0 && items[popupActive])
            items[popupActive].click();
          return;
        } else if (e.key === "Escape") {
          closePopup();
          modelLabelBtn && modelLabelBtn.focus();
          return;
        }
        if (popupList) popupList.dataset.activeIndex = popupActive;
        items.forEach(function (it, i) {
          it.classList.toggle("active", i === popupActive);
        });
        if (items[popupActive])
          items[popupActive].scrollIntoView({ block: "nearest" });
      });
    }

    if (popupList) {
      popupList.addEventListener("click", async function (e) {
        const item = e.target.closest(".model-item");
        if (!item) return;
        const provider = item.dataset.provider;
        const modelId = item.dataset.modelId;
        if (!provider || !modelId) return;
        closePopup();
        try {
          const res = await fetch(
            "/api/set-model?id=" + encodeURIComponent(sessionId),
            {
              method: "POST",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ provider, modelId }),
            },
          );
          const data = await res.json();
          if (!res.ok) throw new Error(data.error || "set model failed");
          const m = allModels.find(function (x) {
            return (
              (x.provider || "") === provider &&
              (x.id === modelId || x.modelId === modelId)
            );
          });
          setSelected(m || { provider, id: modelId, name: modelId });
          const newLabel =
            (m ? m.name || m.id || m.modelId || modelId : modelId) +
            " @ " +
            provider;
          knownModelLabel = newLabel;
          setModelLabel(newLabel);
          setChatStatus("switched", "ok");
        } catch (err) {
          setChatStatus(err.message || String(err), "error");
        }
      });
    }

    document.addEventListener("click", function (e) {
      if (popup && popup.style.display !== "none") {
        const modelLabelBtnEl = document.getElementById("pi-chat-model-label");
        if (!popup.contains(e.target) && e.target !== modelLabelBtnEl)
          closePopup();
      }
    });

    // Try to detect current model from latest model_change entry, then from last assistant message
    const modelChanges = entries.filter(function (e) {
      return e.type === "model_change";
    });
    let detectedProvider = "",
      detectedModelId = "";
    if (modelChanges.length > 0) {
      const latest = modelChanges[modelChanges.length - 1];
      detectedProvider = latest.provider || "";
      detectedModelId = latest.modelId || "";
    } else {
      // Fallback: last assistant message with model info
      for (let i = entries.length - 1; i >= 0; i--) {
        const e = entries[i];
        if (
          e.type === "message" &&
          e.message &&
          e.message.role === "assistant" &&
          e.message.model
        ) {
          detectedProvider = e.message.provider || "";
          detectedModelId = e.message.model || "";
          break;
        }
      }
    }
    if (detectedModelId) {
      const m = allModels.find(function (x) {
        return (
          (x.provider || "") === detectedProvider &&
          ((x.id || "") === detectedModelId ||
            (x.modelId || "") === detectedModelId)
        );
      });
      if (m) {
        setSelected(m);
        // Seed knownModelLabel so refreshWorkerStatus preserves it
        const detectedLabel =
          (m.name || m.id || m.modelId || "") +
          (m.provider ? " @ " + m.provider : "");
        if (detectedLabel && !knownModelLabel) {
          knownModelLabel = detectedLabel;
          setModelLabel(detectedLabel);
        }
      }
    }
  } catch (e) {
    // silently ignore if pi is not available
  }
}

// ── Thinking level selector ──────────────────────────────────────────
function setupThinkingLevelSelector() {
  const sessionId =
    new URLSearchParams(window.location.search).get("id") ||
    (document.getElementById("pi-chat-composer") || {}).dataset?.sessionId ||
    "";
  const thinkingLabelBtn = document.getElementById("pi-chat-thinking-label");
  const thinkingPopup = document.getElementById("pi-chat-thinking-popup");
  const thinkingList = document.getElementById("pi-chat-thinking-list");
  if (!thinkingLabelBtn || !thinkingPopup || !thinkingList) return;

  function supportedThinkingLevels(model) {
    if (!model) return THINKING_LEVELS;
    if (!model.reasoning) return ["off"];
    const map = model.thinkingLevelMap || {};
    return THINKING_LEVELS.filter(function (level) {
      const mapped = map[level];
      if (mapped === null) return false;
      if (level === "xhigh") return mapped !== undefined;
      return true;
    });
  }

  function renderThinkingList(selectedLevel) {
    const supported = supportedThinkingLevels(currentModelForThinking);
    let html = "";
    THINKING_LEVELS.forEach(function (level) {
      const active = level === selectedLevel ? " selected" : "";
      const disabled =
        supported.indexOf(level) < 0
          ? ' disabled title="Not supported by current model"'
          : "";
      const label =
        supported.indexOf(level) < 0 ? level + " (unsupported)" : level;
      html += `<button type="button" class="thinking-level-item thinking-${level}${active}" data-level="${level}"${disabled}>${label}</button>`;
    });
    thinkingList.innerHTML = html;
  }

  function openThinkingPopup() {
    thinkingPopup.style.display = "flex";
    renderThinkingList(knownThinkingLevel);
    const rect = thinkingLabelBtn.getBoundingClientRect();
    const minW = 120;
    let left = rect.right - minW;
    if (left < 4) left = 4;
    if (left + minW > window.innerWidth - 4)
      left = window.innerWidth - minW - 4;
    thinkingPopup.style.bottom = window.innerHeight - rect.top + 4 + "px";
    thinkingPopup.style.left = left + "px";
    thinkingPopup.style.right = "";
  }

  function closeThinkingPopup() {
    thinkingPopup.style.display = "none";
  }

  thinkingLabelBtn.addEventListener("click", function (e) {
    e.stopPropagation();
    if (thinkingPopup.style.display !== "none") {
      closeThinkingPopup();
    } else {
      openThinkingPopup();
    }
  });

  thinkingList.addEventListener("click", async function (e) {
    const item = e.target.closest(".thinking-level-item");
    if (!item) return;
    if (item.disabled) return;
    const level = item.dataset.level;
    if (!level) return;
    closeThinkingPopup();
    try {
      const res = await fetch(
        "/api/set-thinking-level?id=" + encodeURIComponent(sessionId),
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ level }),
        },
      );
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || "set thinking level failed");
      const effectiveLevel = data.thinkingLevel || level;
      knownThinkingLevel = effectiveLevel;
      setThinkingLabel(effectiveLevel);
      setChatStatus("thinking: " + effectiveLevel, "ok");
    } catch (err) {
      setChatStatus(err.message || String(err), "error");
    }
  });

  document.addEventListener("click", function (e) {
    if (
      thinkingPopup.style.display !== "none" &&
      !thinkingPopup.contains(e.target) &&
      e.target !== thinkingLabelBtn
    ) {
      closeThinkingPopup();
    }
  });

  // Detect initial thinking level from latest thinking_level_change entry
  const thinkingChanges = entries.filter(function (e) {
    return e.type === "thinking_level_change";
  });
  if (thinkingChanges.length > 0) {
    const latest = thinkingChanges[thinkingChanges.length - 1];
    if (latest.thinkingLevel) {
      knownThinkingLevel = latest.thinkingLevel;
      setThinkingLabel(latest.thinkingLevel);
    }
  }
}

// Initial render
// If URL has targetId, scroll to that specific message; otherwise stay at top
if (leafId) {
  if (urlTargetId && byId.has(urlTargetId)) {
    // Deep link: navigate to leaf and scroll to target message
    navigateTo(leafId, "target", urlTargetId);
  } else {
    navigateTo(leafId, "none");
  }
} else if (entries.length > 0) {
  // Fallback: use last entry if no leafId
  navigateTo(entries[entries.length - 1].id, "none");
}
