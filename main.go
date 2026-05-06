package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	_ "embed"
)

const defaultPort = "27183"

//go:embed templates/template.html
var templateHtml string

//go:embed templates/template.css
var templateCss string

//go:embed templates/template.js
var templateJs string

//go:embed templates/vendor/marked.min.js
var markedJs string

//go:embed templates/vendor/highlight.min.js
var hljsJs string

// ── Live-reload: fetch JSON, diff, append new entries ──────────────────────
const liveReloadJs = `
<script>
(function() {
  var SEEN = new Set();
  document.querySelectorAll('[id^="entry-"]').forEach(function(el) {
    SEEN.add(el.id.replace('entry-', ''));
  });

  function escapeHtml(t) {
    var d = document.createElement('div');
    d.textContent = t;
    return d.innerHTML;
  }
  function extractContent(c) {
    if (typeof c === 'string') return c;
    if (Array.isArray(c)) return c.filter(function(x){return x.type==='text'}).map(function(x){return x.text}).join('');
    return '';
  }
  function fmtTs(ts) {
    if (!ts) return '';
    var d = new Date(ts);
    return d.toLocaleTimeString(undefined,{hour:'2-digit',minute:'2-digit',second:'2-digit'});
  }
  function shortenPath(p) {
    if (typeof p !== 'string') return '';
    if (p.indexOf('/Users/')===0) { var parts=p.split('/'); if(parts.length>2) return '~'+p.slice(('/Users/'+parts[2]).length); }
    if (p.indexOf('/home/')===0) { var parts=p.split('/'); if(parts.length>2) return '~'+p.slice(('/home/'+parts[2]).length); }
    return p;
  }
  function formatToolCall(name, args) {
    args = args || {};
    switch(name) {
      case 'read':
        var path = shortenPath(String(args.path || args.file_path || ''));
        var off = args.offset, lim = args.limit;
        if (off !== undefined || lim !== undefined) { var s = off||1, e = lim ? s+lim-1 : ''; path += ':'+s+(e?'-'+e:''); }
        return '[read: '+path+']';
      case 'write': return '[write: '+shortenPath(String(args.path || args.file_path || ''))+']';
      case 'edit': return '[edit: '+shortenPath(String(args.path || args.file_path || ''))+']';
      case 'bash':
        var raw = String(args.command || '');
        var cmd = raw.replace(/[\n\t]/g,' ').trim().slice(0,50);
        return '[bash: '+cmd+(raw.length>50?'...':'')+']';
      case 'grep': return '[grep: /'+(args.pattern||'')+'/ in '+shortenPath(String(args.path||'.'))+']';
      case 'find': return '[find: '+(args.pattern||'')+' in '+shortenPath(String(args.path||'.'))+']';
      case 'ls': return '[ls: '+shortenPath(String(args.path||'.'))+']';
      default:
        var s = JSON.stringify(args).slice(0,40);
        return '['+name+': '+s+(JSON.stringify(args).length>40?'...':'')+']';
    }
  }
  function safeMarked(text) {
    try { return marked.parse(text); } catch(e) { return escapeHtml(text); }
  }

  var TOOL_RESULT_CACHE = {};
  function cacheToolResults(entries) {
    entries.forEach(function(e) {
      if (e.type === 'message' && e.message && e.message.role === 'toolResult' && e.message.toolCallId) {
        TOOL_RESULT_CACHE[e.message.toolCallId] = e.message;
      }
    });
  }

  function renderToolCall(call, result) {
    var isError = result && result.isError;
    var status = result ? (isError ? 'error' : 'success') : 'pending';
    var html = '<div class="tool-execution '+status+'">';
    var args = call.arguments || {};
    var invalid = '<span class="tool-error">[invalid arg]</span>';

    function getResultText() {
      if (!result) return '';
      return (result.content||[]).filter(function(c){return c.type==='text'}).map(function(c){return c.text}).join('\n');
    }
    function getResultImages() {
      if (!result) return [];
      return (result.content||[]).filter(function(c){return c.type==='image'});
    }

    if (call.name === 'bash') {
      var cmd = typeof args.command === 'string' ? escapeHtml(args.command) : invalid;
      html += '<div class="tool-command">$ '+cmd+'</div>';
      if (result) {
        var out = getResultText().trim();
        if (out) html += '<div class="tool-output"><pre>'+escapeHtml(out)+'</pre></div>';
      }
    } else if (call.name === 'read') {
      var fp = typeof (args.file_path || args.path) === 'string' ? shortenPath(String(args.file_path || args.path || '')) : invalid;
      html += '<div class="tool-header"><span class="tool-name">read</span> <span class="tool-path">'+escapeHtml(fp)+'</span></div>';
      if (result) {
        var imgs = getResultImages();
        imgs.forEach(function(img) {
          html += '<img src="data:'+escapeHtml(img.mimeType||'image/png')+';base64,'+img.data+'" class="tool-image" />';
        });
        var out = getResultText();
        if (out) html += '<div class="tool-output"><pre>'+escapeHtml(out)+'</pre></div>';
      }
    } else if (call.name === 'write') {
      var fp = typeof (args.file_path || args.path) === 'string' ? escapeHtml(shortenPath(String(args.file_path || args.path || ''))) : invalid;
      html += '<div class="tool-header"><span class="tool-name">write</span> <span class="tool-path">'+fp+'</span></div>';
      if (typeof args.content === 'string') {
        html += '<div class="tool-output"><pre>'+escapeHtml(args.content)+'</pre></div>';
      } else if (args.content !== undefined) {
        html += '<div class="tool-error">[invalid content arg - expected string]</div>';
      }
      if (result) {
        var out = getResultText().trim();
        if (out) html += '<div class="tool-output"><div>'+escapeHtml(out)+'</div></div>';
      }
    } else if (call.name === 'edit') {
      var fp = typeof (args.file_path || args.path) === 'string' ? escapeHtml(shortenPath(String(args.file_path || args.path || ''))) : invalid;
      html += '<div class="tool-header"><span class="tool-name">edit</span> <span class="tool-path">'+fp+'</span></div>';
      if (result && result.details && result.details.diff) {
        var lines = result.details.diff.split('\n');
        html += '<div class="tool-diff">';
        lines.forEach(function(line) {
          var cls = line.match(/^\+/) ? 'diff-added' : line.match(/^-/) ? 'diff-removed' : 'diff-context';
          html += '<div class="'+cls+'">'+escapeHtml(line)+'</div>';
        });
        html += '</div>';
      } else if (result) {
        var out = getResultText().trim();
        if (out) html += '<div class="tool-output"><pre>'+escapeHtml(out)+'</pre></div>';
      }
    } else if (call.name === 'ls') {
      var dp = typeof args.path === 'string' ? escapeHtml(shortenPath(String(args.path || '.'))) : invalid;
      html += '<div class="tool-header"><span class="tool-name">ls</span> <span class="tool-path">'+dp+'</span></div>';
      if (result) {
        var out = getResultText().trim();
        if (out) html += '<div class="tool-output"><pre>'+escapeHtml(out)+'</pre></div>';
      }
    } else {
      html += '<div class="tool-header"><span class="tool-name">'+escapeHtml(call.name)+'</span></div>';
      html += '<div class="tool-output"><pre>'+escapeHtml(JSON.stringify(args,null,2))+'</pre></div>';
      if (result) {
        var out = getResultText();
        if (out) html += '<div class="tool-output"><pre>'+escapeHtml(out)+'</pre></div>';
      }
    }
    html += '</div>';
    return html;
  }

  function renderEntry(entry, allEntries) {
    cacheToolResults(allEntries);
    var ts = fmtTs(entry.timestamp);
    var tsHtml = ts ? '<div class="message-timestamp">'+ts+'</div>' : '';
    var eid = 'entry-' + entry.id;

    if (entry.type === 'message') {
      var msg = entry.message;
      if (msg.role === 'user') {
        var html = '<div class="user-message" id="'+eid+'">'+tsHtml;
        var content = msg.content;
        if (Array.isArray(content)) {
          var imgs = content.filter(function(c){return c.type==='image'});
          if (imgs.length) {
            html += '<div class="message-images">';
            imgs.forEach(function(img) {
              html += '<img src="data:'+escapeHtml(img.mimeType||'image/png')+';base64,'+img.data+'" class="message-image" />';
            });
            html += '</div>';
          }
        }
        var text = typeof content === 'string' ? content : extractContent(content);
        if (text.trim()) html += '<div class="markdown-content">'+safeMarked(text)+'</div>';
        html += '</div>';
        return html;
      }
      if (msg.role === 'assistant') {
        var html = '<div class="assistant-message" id="'+eid+'">'+tsHtml;
        var content = msg.content || [];
        content.forEach(function(block) {
          if (block.type === 'text' && block.text.trim()) {
            html += '<div class="assistant-text markdown-content">'+safeMarked(block.text)+'</div>';
          } else if (block.type === 'thinking' && block.thinking.trim()) {
            html += '<div class="thinking-block"><div class="thinking-text">'+escapeHtml(block.thinking)+'</div></div>';
          }
        });
        content.forEach(function(block) {
          if (block.type === 'toolCall') {
            var result = TOOL_RESULT_CACHE[block.id];
            html += renderToolCall(block, result);
          }
        });
        if (msg.stopReason === 'aborted') html += '<div class="error-text">Aborted</div>';
        else if (msg.stopReason === 'error') html += '<div class="error-text">Error: '+escapeHtml(msg.errorMessage||'Unknown error')+'</div>';
        html += '</div>';
        return html;
      }
      if (msg.role === 'bashExecution') {
        var isErr = msg.cancelled || (msg.exitCode !== 0 && msg.exitCode !== null);
        var html = '<div class="tool-execution '+(isErr?'error':'success')+'" id="'+eid+'">'+tsHtml;
        html += '<div class="tool-command">$ '+escapeHtml(msg.command)+'</div>';
        if (msg.output) html += '<div class="tool-output"><pre>'+escapeHtml(msg.output)+'</pre></div>';
        if (msg.cancelled) html += '<div style="color:var(--warning)">(cancelled)</div>';
        else if (msg.exitCode !== 0 && msg.exitCode !== null) html += '<div style="color:var(--error)">(exit '+msg.exitCode+')</div>';
        html += '</div>';
        return html;
      }
      if (msg.role === 'toolResult') return '';
    }
    if (entry.type === 'model_change') {
      return '<div class="model-change" id="'+eid+'">'+tsHtml+'Switched to model: <span class="model-name">'+escapeHtml(entry.provider)+'/'+escapeHtml(entry.modelId)+'</span></div>';
    }
    if (entry.type === 'compaction') {
      return '<div class="compaction" id="'+eid+'"><div class="compaction-label">[compaction]</div><div class="compaction-collapsed">Compacted from '+entry.tokensBefore.toLocaleString()+' tokens</div></div>';
    }
    if (entry.type === 'branch_summary') {
      return '<div class="branch-summary" id="'+eid+'">'+tsHtml+'<div class="branch-summary-header">Branch Summary</div><div class="markdown-content">'+safeMarked(entry.summary||'')+'</div></div>';
    }
    if (entry.type === 'custom_message' && entry.display) {
      return '<div class="hook-message" id="'+eid+'">'+tsHtml+'<div class="hook-type">['+escapeHtml(entry.customType)+']</div><div class="markdown-content">'+safeMarked(typeof entry.content === 'string' ? entry.content : JSON.stringify(entry.content))+'</div></div>';
    }
    return '';
  }

  // ── Follow mode (like terminal/chat) ───────────────────────────────────
  var FOLLOW = true;
  var followBtn = null;
  var pendingCount = 0;

  function isAtBottom() {
    // Check document-level scroll (what users actually scroll)
    var de = document.documentElement;
    var body = document.body;
    var docHeight = Math.max(de.scrollHeight, body.scrollHeight);
    var scrolled = window.scrollY || window.pageYOffset || de.scrollTop || body.scrollTop;
    var viewport = window.innerHeight;
    var remaining = docHeight - scrolled - viewport;

    // Also check #content if it has its own scrollbar
    var content = document.getElementById('content');
    if (content && content.scrollHeight > content.clientHeight) {
      var contentRemaining = content.scrollHeight - content.scrollTop - content.clientHeight;
      remaining = Math.max(remaining, contentRemaining);
    }

    return remaining < 80;
  }

  function scrollToBottom(smooth) {
    var content = document.getElementById('content');
    if (content && content.scrollHeight > content.clientHeight) {
      content.scrollTo({ top: content.scrollHeight, behavior: smooth ? 'smooth' : 'auto' });
    }
    window.scrollTo({ top: Math.max(document.documentElement.scrollHeight, document.body.scrollHeight), behavior: smooth ? 'smooth' : 'auto' });
  }

  function showFollowButton() {
    if (followBtn) {
      followBtn.textContent = '↓ ' + pendingCount + ' new' + (pendingCount > 1 ? 's' : '');
      return;
    }
    followBtn = document.createElement('button');
    followBtn.textContent = '↓ ' + pendingCount + ' new' + (pendingCount > 1 ? 's' : '');
    followBtn.style.cssText = 'position:fixed;bottom:20px;right:20px;z-index:200;padding:6px 14px;font-size:11px;font-family:inherit;background:var(--accent);color:var(--body-bg);border:none;border-radius:4px;cursor:pointer;opacity:0;transition:opacity 0.2s;box-shadow:0 2px 8px rgba(0,0,0,0.3);';
    document.body.appendChild(followBtn);
    requestAnimationFrame(function() { followBtn.style.opacity = '1'; });
    followBtn.addEventListener('click', function() {
      FOLLOW = true;
      pendingCount = 0;
      scrollToBottom(true);
      hideFollowButton();
    });
  }

  function hideFollowButton() {
    if (!followBtn) return;
    followBtn.style.opacity = '0';
    setTimeout(function() {
      if (followBtn && followBtn.parentNode) {
        followBtn.parentNode.removeChild(followBtn);
      }
      followBtn = null;
    }, 200);
  }

  function onScroll() {
    var wasFollowing = FOLLOW;
    FOLLOW = isAtBottom();
    if (FOLLOW && followBtn) {
      hideFollowButton();
      pendingCount = 0;
    }
  }

  // Listen on both window and content element
  window.addEventListener('scroll', onScroll, { passive: true });
  var contentEl = document.getElementById('content');
  if (contentEl) contentEl.addEventListener('scroll', onScroll, { passive: true });
  // Start at bottom on first load
  scrollToBottom(false);

  function appendEntry(entry, allEntries) {
    if (SEEN.has(entry.id)) return;
    var html = renderEntry(entry, allEntries);
    if (!html) { SEEN.add(entry.id); return; }
    var container = document.getElementById('messages');
    if (!container) return;
    var wrap = document.createElement('div');
    wrap.innerHTML = html;
    var node = wrap.firstElementChild;
    if (!node) return;
    container.appendChild(node);
    SEEN.add(entry.id);
    node.style.transition = 'box-shadow 0.8s ease-out';
    node.style.boxShadow = '0 0 0 2px var(--accent)';
    setTimeout(function() { node.style.boxShadow = ''; }, 1500);

    if (FOLLOW) {
      scrollToBottom(true);
    } else {
      pendingCount++;
      showFollowButton();
    }
  }

  function updateStats(entries) {
    var user=0, assistant=0, toolCalls=0;
    var tokens={input:0,output:0,cacheRead:0,cacheWrite:0};
    var cost={input:0,output:0,cacheRead:0,cacheWrite:0};
    var models=new Set();
    entries.forEach(function(e) {
      if (e.type !== 'message' || !e.message) return;
      var m = e.message;
      if (m.role === 'user') user++;
      if (m.role === 'assistant') {
        assistant++;
        if (m.model) models.add(m.provider ? m.provider+'/'+m.model : m.model);
        if (m.usage) {
          tokens.input += m.usage.input||0;
          tokens.output += m.usage.output||0;
          tokens.cacheRead += m.usage.cacheRead||0;
          tokens.cacheWrite += m.usage.cacheWrite||0;
          if (m.usage.cost) {
            cost.input += m.usage.cost.input||0;
            cost.output += m.usage.cost.output||0;
            cost.cacheRead += m.usage.cost.cacheRead||0;
            cost.cacheWrite += m.usage.cost.cacheWrite||0;
          }
        }
        toolCalls += (m.content||[]).filter(function(c){return c.type==='toolCall'}).length;
      }
    });
    var totalCost = cost.input+cost.output+cost.cacheRead+cost.cacheWrite;
    var headerInfo = document.querySelector('.header-info');
    if (!headerInfo) return;
    var parts = [];
    if (user) parts.push(user+' user');
    if (assistant) parts.push(assistant+' assistant');
    var rows = headerInfo.querySelectorAll('.info-item');
    rows.forEach(function(row) {
      var label = row.querySelector('.info-label');
      if (!label) return;
      var text = label.textContent;
      var val = row.querySelector('.info-value');
      if (!val) return;
      if (text.indexOf('Messages:') >= 0) val.textContent = parts.join(', ') || '0';
      if (text.indexOf('Tool Calls:') >= 0) val.textContent = toolCalls;
      if (text.indexOf('Models:') >= 0) val.textContent = Array.from(models).join(', ') || 'unknown';
      if (text.indexOf('Tokens:') >= 0) {
        var tps = [];
        if (tokens.input) tps.push('\u2191'+formatTokens(tokens.input));
        if (tokens.output) tps.push('\u2193'+formatTokens(tokens.output));
        if (tokens.cacheRead) tps.push('R'+formatTokens(tokens.cacheRead));
        if (tokens.cacheWrite) tps.push('W'+formatTokens(tokens.cacheWrite));
        val.textContent = tps.join(' ') || '0';
      }
      if (text.indexOf('Cost:') >= 0) val.textContent = '$'+totalCost.toFixed(3);
    });
  }
  function formatTokens(n) {
    if (n < 1000) return n.toString();
    if (n < 10000) return (n/1000).toFixed(1)+'k';
    if (n < 1000000) return Math.round(n/1000)+'k';
    return (n/1000000).toFixed(1)+'M';
  }

  var sessId = location.search.split('id=')[1]?.split('&')[0] || '';
  var es = new EventSource('/events?id=' + encodeURIComponent(sessId));
  var indicator = null;

  function showIndicator() {
    if (indicator) return;
    indicator = document.createElement('div');
    indicator.textContent = 'updated';
    indicator.style.cssText = 'position:fixed;top:8px;right:8px;z-index:200;padding:2px 8px;font-size:10px;font-family:inherit;background:var(--accent);color:var(--body-bg);border-radius:3px;opacity:0;transition:opacity 0.3s;';
    document.body.appendChild(indicator);
    requestAnimationFrame(function() { indicator.style.opacity = '1'; });
    setTimeout(function() {
      indicator.style.opacity = '0';
      setTimeout(function() { if(indicator){document.body.removeChild(indicator);indicator=null;} }, 300);
    }, 1200);
  }

  es.onmessage = function(e) {
    if (e.data !== 'reload') return;
    fetch('/api/session?id=' + encodeURIComponent(sessId))
      .then(function(r){return r.json();})
      .then(function(data) {
        var entries = data.entries || [];
        var newCount = 0;
        entries.forEach(function(entry) {
          if (entry.id && !SEEN.has(entry.id)) {
            appendEntry(entry, entries);
            newCount++;
          }
        });
        if (newCount > 0) {
          showIndicator();
          updateStats(entries);
        }
      })
      .catch(function(err){ console.error('Live update failed:', err); });
  };
  es.onerror = function() {};

  // ── Share button ────────────────────────────────────────────────────────
  var shareBtn = document.getElementById('share-btn');
  var shareOverlay = null;

  function showShareResult(gistUrl, previewUrl) {
    if (shareOverlay) shareOverlay.remove();
    shareOverlay = document.createElement('div');
    shareOverlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;z-index:300;background:rgba(0,0,0,0.6);display:flex;align-items:center;justify-content:center;';
    var box = document.createElement('div');
    box.style.cssText = 'background:var(--container-bg);border:1px solid var(--dim);border-radius:4px;padding:calc(var(--line-height)*2);max-width:500px;width:90%;font-family:inherit;';
    box.innerHTML = '<h3 style="margin:0 0 var(--line-height);font-size:12px;color:var(--border-accent);">Session Shared</h3>' +
      '<div style="margin-bottom:var(--line-height);"><label style="display:block;font-size:11px;color:var(--muted);margin-bottom:4px;">Gist URL</label>' +
      '<input readonly value="'+escapeHtml(gistUrl)+'" style="width:100%;padding:4px 8px;font-size:11px;font-family:inherit;background:var(--body-bg);color:var(--text);border:1px solid var(--dim);border-radius:3px;cursor:pointer;" onclick="this.select()"></div>' +
      '<div style="margin-bottom:var(--line-height);"><label style="display:block;font-size:11px;color:var(--muted);margin-bottom:4px;">Preview URL</label>' +
      '<input readonly value="'+escapeHtml(previewUrl)+'" style="width:100%;padding:4px 8px;font-size:11px;font-family:inherit;background:var(--body-bg);color:var(--text);border:1px solid var(--dim);border-radius:3px;cursor:pointer;" onclick="this.select()"></div>' +
      '<div style="display:flex;gap:8px;justify-content:flex-end;">' +
      '<button id="share-copy-gist" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--accent);color:var(--body-bg);border:none;border-radius:3px;cursor:pointer;">Copy Gist</button>' +
      '<button id="share-copy-preview" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--container-bg);color:var(--text);border:1px solid var(--dim);border-radius:3px;cursor:pointer;">Copy Preview</button>' +
      '<button id="share-close" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--container-bg);color:var(--text);border:1px solid var(--dim);border-radius:3px;cursor:pointer;">Close</button></div>';
    shareOverlay.appendChild(box);
    document.body.appendChild(shareOverlay);

    shareOverlay.addEventListener('click', function(e) {
      if (e.target === shareOverlay) { shareOverlay.remove(); shareOverlay = null; }
    });
    document.getElementById('share-close').addEventListener('click', function() {
      shareOverlay.remove(); shareOverlay = null;
    });
    document.getElementById('share-copy-gist').addEventListener('click', function() {
      navigator.clipboard.writeText(gistUrl).catch(function(){});
      this.textContent = 'Copied!';
      setTimeout(function(){ document.getElementById('share-copy-gist').textContent = 'Copy Gist'; }, 1200);
    });
    document.getElementById('share-copy-preview').addEventListener('click', function() {
      navigator.clipboard.writeText(previewUrl).catch(function(){});
      this.textContent = 'Copied!';
      setTimeout(function(){ document.getElementById('share-copy-preview').textContent = 'Copy Preview'; }, 1200);
    });
  }

  function showShareError(msg) {
    if (shareOverlay) shareOverlay.remove();
    shareOverlay = document.createElement('div');
    shareOverlay.style.cssText = 'position:fixed;top:0;left:0;right:0;bottom:0;z-index:300;background:rgba(0,0,0,0.6);display:flex;align-items:center;justify-content:center;';
    var box = document.createElement('div');
    box.style.cssText = 'background:var(--container-bg);border:1px solid var(--error);border-radius:4px;padding:calc(var(--line-height)*2);max-width:400px;width:90%;font-family:inherit;';
    box.innerHTML = '<h3 style="margin:0 0 var(--line-height);font-size:12px;color:var(--error);">Share Failed</h3>' +
      '<p style="font-size:11px;color:var(--text);margin:0 0 var(--line-height);white-space:pre-wrap;">'+escapeHtml(msg)+'</p>' +
      '<div style="display:flex;justify-content:flex-end;"><button id="share-close-err" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--container-bg);color:var(--text);border:1px solid var(--dim);border-radius:3px;cursor:pointer;">Close</button></div>';
    shareOverlay.appendChild(box);
    document.body.appendChild(shareOverlay);
    document.getElementById('share-close-err').addEventListener('click', function() {
      shareOverlay.remove(); shareOverlay = null;
    });
    shareOverlay.addEventListener('click', function(e) {
      if (e.target === shareOverlay) { shareOverlay.remove(); shareOverlay = null; }
    });
  }

  if (shareBtn) {
    shareBtn.addEventListener('click', function() {
      shareBtn.textContent = '...';
      shareBtn.disabled = true;
      fetch('/share?id=' + encodeURIComponent(sessId))
        .then(function(r){ return r.json(); })
        .then(function(data) {
          shareBtn.textContent = '↗ Share';
          shareBtn.disabled = false;
          if (data.error) {
            showShareError(data.error + (data.stderr ? '\n\n' + data.stderr : ''));
          } else {
            showShareResult(data.gistUrl, data.previewUrl);
          }
        })
        .catch(function(err) {
          shareBtn.textContent = '↗ Share';
          shareBtn.disabled = false;
          showShareError(err.message || 'Network error');
        });
    });
  }
})();
</script>
`

func main() {
	port := flag.String("p", defaultPort, "port to listen on")
	hostOverride := flag.String("host", "", "host/IP to bind; defaults to Tailscale IP when available, otherwise 127.0.0.1")
	open := flag.Bool("o", false, "auto-open browser")
	flag.Parse()

	sessionsDir := filepath.Join(os.Getenv("HOME"), ".pi", "agent", "sessions")
	if _, err := os.Stat(sessionsDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "sessions directory not found: %s\n", sessionsDir)
		os.Exit(1)
	}

	bindHost, usedTailscale := chooseBindHost(*hostOverride, detectTailscaleIP)
	srv := newServer(sessionsDir)
	http.HandleFunc("/", srv.handleIndex)
	http.HandleFunc("/session", srv.handleSession)
	http.HandleFunc("/api/session", srv.handleApiSession)
	http.HandleFunc("/api/chat", srv.handleChat)
	http.HandleFunc("/api/worker-status", srv.handleWorkerStatus)
	http.HandleFunc("/share", srv.handleShare)
	http.HandleFunc("/events", srv.handleEvents)

	addr := net.JoinHostPort(bindHost, *port)
	url := "http://" + addr
	fmt.Printf("Pi Sessions Viewer -> %s\n", url)
	if !usedTailscale && *hostOverride == "" {
		fmt.Println("Tailscale IP not detected; using localhost.")
	}
	fmt.Printf("Serving from: %s\n", sessionsDir)

	if *open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}

	if err := http.ListenAndServe(addr, nil); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	exec.Command(cmd, args...).Start()
}

// ── Server with live-reload SSE ────────────────────────────────────────────

type sseClient struct {
	ch     chan string
	sessID string
}

type server struct {
	sessionsDir string
	clients     []*sseClient
	clientsMu   sync.RWMutex
	fileMod     map[string]time.Time
	fileModMu   sync.RWMutex
	chatSender  ChatSender
}

func newServer(sessionsDir string) *server {
	s := &server{
		sessionsDir: sessionsDir,
		clients:     make([]*sseClient, 0),
		fileMod:     make(map[string]time.Time),
		chatSender:  NewWorkerManager(newPiRPCWorker),
	}
	go s.watchFiles()
	return s
}

func (s *server) addClient(sessID string) *sseClient {
	c := &sseClient{ch: make(chan string, 4), sessID: sessID}
	s.clientsMu.Lock()
	s.clients = append(s.clients, c)
	s.clientsMu.Unlock()
	return c
}

func (s *server) removeClient(target *sseClient) {
	s.clientsMu.Lock()
	filtered := s.clients[:0]
	for _, c := range s.clients {
		if c != target {
			filtered = append(filtered, c)
		}
	}
	s.clients = filtered
	s.clientsMu.Unlock()
	close(target.ch)
}

func (s *server) broadcast(sessID, msg string) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()
	for _, c := range s.clients {
		if c.sessID == sessID {
			select {
			case c.ch <- msg:
			default:
			}
		}
	}
}

func (s *server) watchFiles() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		entries, err := os.ReadDir(s.sessionsDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			subDir := filepath.Join(s.sessionsDir, e.Name())
			subs, err := os.ReadDir(subDir)
			if err != nil {
				continue
			}
			for _, f := range subs {
				if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
					continue
				}
				sessID := f.Name()
				path := filepath.Join(subDir, f.Name())
				info, err := os.Stat(path)
				if err != nil {
					continue
				}

				s.fileModMu.Lock()
				lastMod, known := s.fileMod[sessID]
				s.fileMod[sessID] = info.ModTime()
				s.fileModMu.Unlock()

				if known && info.ModTime().After(lastMod) {
					s.broadcast(sessID, "reload")
				}
			}
		}
	}
}

// ── HTTP Handlers ──────────────────────────────────────────────────────────

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	sessions, err := loadAllSessions(s.sessionsDir)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := indexTmpl.Execute(w, sessions); err != nil {
		fmt.Fprintf(os.Stderr, "template error: %v\n", err)
	}
}

func (s *server) handleSession(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}

	sessions, err := loadAllSessions(s.sessionsDir)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, sess := range sessions {
		if sess.ID == id {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(generateExportHtml(sess, true)))
			return
		}
	}
	http.Error(w, "session not found", 404)
}

func (s *server) handleApiSession(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, 400)
		return
	}

	sessions, err := loadAllSessions(s.sessionsDir)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 500)
		return
	}

	for _, sess := range sessions {
		if sess.ID == id {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"header":  sess.Header,
				"entries": sess.Entries,
			})
			return
		}
	}
	http.Error(w, `{"error":"not found"}`, 404)
}

func findGh() string {
	candidates := []string{
		"/opt/homebrew/bin/gh",
		"/usr/local/bin/gh",
		"/usr/bin/gh",
		"/bin/gh",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	// Fallback to PATH lookup
	if p, err := exec.LookPath("gh"); err == nil {
		return p
	}
	return ""
}

func (s *server) handleShare(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, 400)
		return
	}

	// Verify gh is installed and logged in
	ghPath := findGh()
	if ghPath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{"error": "GitHub CLI (gh) not installed. Install from https://cli.github.com/"})
		return
	}
	authCheck := exec.Command(ghPath, "auth", "status")
	if err := authCheck.Run(); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]any{"error": "GitHub CLI not logged in. Run 'gh auth login' first."})
		return
	}

	// Generate session HTML
	sessions, err := loadAllSessions(s.sessionsDir)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}

	var html string
	for _, sess := range sessions {
		if sess.ID == id {
			html = generateExportHtml(sess, false)
			break
		}
	}
	if html == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]any{"error": "session not found"})
		return
	}

	// Write to temp file with proper name for gist
	tmpDir, err := os.MkdirTemp(os.TempDir(), "pi-share-*")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]any{"error": "failed to create temp dir: " + err.Error()})
		return
	}
	defer os.RemoveAll(tmpDir)
	tmpFile := filepath.Join(tmpDir, "session.html")
	if err := os.WriteFile(tmpFile, []byte(html), 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]any{"error": "failed to write temp file: " + err.Error()})
		return
	}

	// Create secret gist
	cmd := exec.Command(ghPath, "gist", "create", "--public=false", tmpFile)
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]any{"error": "failed to create gist", "stderr": stderr})
		return
	}

	gistUrl := strings.TrimSpace(string(out))
	gistId := ""
	if parts := strings.Split(gistUrl, "/"); len(parts) > 0 {
		gistId = parts[len(parts)-1]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"gistUrl":    gistUrl,
		"gistId":     gistId,
		"previewUrl": "https://pi.dev/session/#" + gistId,
	})
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sessID := r.URL.Query().Get("id")
	if sessID == "" {
		http.Error(w, "missing id", 400)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	client := s.addClient(sessID)
	defer s.removeClient(client)

	fmt.Fprintf(w, ":ok\n\n")
	flusher.Flush()

	for {
		select {
		case msg, open := <-client.ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// ── Session data model ─────────────────────────────────────────────────────

type Session struct {
	ID           string
	Filename     string
	Project      string
	LastActivity string
	MessageCount int
	TokenTotal   int
	CostTotal    float64
	Header       map[string]any
	Entries      []map[string]any
}

// ── Loading ────────────────────────────────────────────────────────────────

func loadAllSessions(dir string) ([]Session, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var sessions []Session
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(dir, e.Name())
		subs, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		for _, f := range subs {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(subDir, f.Name())
			sess, err := parseSession(path, e.Name(), f.Name())
			if err != nil {
				continue
			}
			sessions = append(sessions, sess)
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		ti, _ := time.Parse(time.RFC3339, sessions[i].LastActivity)
		tj, _ := time.Parse(time.RFC3339, sessions[j].LastActivity)
		return ti.After(tj)
	})

	return sessions, nil
}

func parseSession(path, dirName, fileName string) (Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}

	sess := Session{
		ID:       fileName,
		Filename: fileName,
		Project:  cleanProjectName(dirName),
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		sess.Entries = append(sess.Entries, raw)

		if raw["type"] == "session" {
			sess.Header = raw
			continue
		}

		if ts, ok := raw["timestamp"].(string); ok {
			sess.LastActivity = ts
		}

		if raw["type"] == "message" {
			msg, ok := raw["message"].(map[string]any)
			if ok {
				sess.MessageCount++
				if usage, ok := msg["usage"].(map[string]any); ok {
					if t, ok := usage["totalTokens"].(float64); ok {
						sess.TokenTotal += int(t)
					}
					if cost, ok := usage["cost"].(map[string]any); ok {
						if total, ok := cost["total"].(float64); ok {
							sess.CostTotal += total
						}
					}
				}
			}
		}
	}

	if sess.LastActivity == "" {
		info, _ := os.Stat(path)
		if info != nil {
			sess.LastActivity = info.ModTime().Format(time.RFC3339)
		}
	}

	return sess, nil
}

func cleanProjectName(dirName string) string {
	s := strings.TrimPrefix(dirName, "--")
	s = strings.TrimSuffix(s, "--")
	s = strings.ReplaceAll(s, "--", "/")
	return s
}

// ── HTML Export Generation (replicates pi's /export exactly) ───────────────

func generateExportHtml(session Session, showButtons bool) string {
	leafID := ""
	if len(session.Entries) > 0 {
		if id, ok := session.Entries[len(session.Entries)-1]["id"].(string); ok {
			leafID = id
		}
	}

	sessionData := map[string]any{
		"header":        session.Header,
		"entries":       session.Entries,
		"leafId":        leafID,
		"systemPrompt":  nil,
		"tools":         nil,
		"renderedTools": nil,
	}

	dataJSON, _ := json.Marshal(sessionData)
	dataBase64 := base64.StdEncoding.EncodeToString(dataJSON)

	themeVars := generateThemeVars()
	bodyBg := "#18181e"
	cardBg := "#1e1e24"
	infoBg := "#3c3728"

	css := templateCss
	css = strings.Replace(css, "{{THEME_VARS}}", themeVars, 1)
	css = strings.Replace(css, "{{BODY_BG}}", bodyBg, 1)
	css = strings.Replace(css, "{{CONTAINER_BG}}", cardBg, 1)
	css = strings.Replace(css, "{{INFO_BG}}", infoBg, 1)

	html := templateHtml
	html = strings.Replace(html, "{{CSS}}", css, 1)
	html = strings.Replace(html, "{{JS}}", templateJs, 1)
	html = strings.Replace(html, "{{SESSION_DATA}}", dataBase64, 1)
	html = strings.Replace(html, "{{MARKED_JS}}", markedJs, 1)
	html = strings.Replace(html, "{{HIGHLIGHT_JS}}", hljsJs, 1)

	if showButtons {
		btns := `<div style="position:fixed;top:10px;right:10px;z-index:101;display:flex;flex-direction:column;gap:6px;">
<a href="/" title="Back to sessions" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--container-bg);color:var(--muted);border:1px solid var(--dim);border-radius:3px;text-decoration:none;cursor:pointer;text-align:center;">← Sessions</a>
<button id="share-btn" title="Share session as GitHub Gist" style="padding:4px 10px;font-size:11px;font-family:inherit;background:var(--container-bg);color:var(--muted);border:1px solid var(--dim);border-radius:3px;cursor:pointer;">↗ Share</button>
</div>`
		html = strings.Replace(html, "<body>", "<body>"+btns, 1)
		html = strings.Replace(html, "{{CHAT_COMPOSER}}", chatComposerHtml(session.ID), 1)
		html = strings.Replace(html, "</body>", liveReloadJs+"</body>", 1)
	} else {
		html = strings.Replace(html, "{{CHAT_COMPOSER}}", "", 1)
	}

	return html
}

func chatComposerHtml(sessionID string) string {
	return `<form id="pi-chat-composer" class="pi-chat-composer" data-session-id="` + template.HTMLEscapeString(sessionID) + `">
  <input id="pi-chat-images" name="images" type="file" accept="image/*" multiple hidden>
  <button type="button" id="pi-chat-attach" class="pi-chat-icon-button" title="Attach images">◉</button>
  <div class="pi-chat-main">
    <textarea id="pi-chat-message" name="message" rows="2" placeholder="Continue this pi session…"></textarea>
    <div id="pi-chat-attachments" class="pi-chat-attachments"></div>
    <div id="pi-chat-status" class="pi-chat-status">idle</div>
  </div>
  <button type="submit" id="pi-chat-send" class="pi-chat-send">Send</button>
</form>`
}

func generateThemeVars() string {
	vars := map[string]string{
		"cyan": "#00d7ff", "blue": "#5f87ff", "green": "#b5bd68", "red": "#cc6666",
		"yellow": "#ffff00", "gray": "#808080", "dimGray": "#666666", "darkGray": "#505050",
		"accent": "#8abeb7", "selectedBg": "#3a3a4a", "userMessageBg": "#343541",
		"toolPendingBg": "#282832", "toolSuccessBg": "#283228", "toolErrorBg": "#3c2828",
		"customMessageBg": "#2d2838", "customMessageLabel": "#9575cd", "thinkingText": "#808080",
		"mdHeading": "#f0c674", "mdLink": "#81a2be", "mdLinkUrl": "#666666",
		"mdCode": "#8abeb7", "mdCodeBlock": "#b5bd68", "mdCodeBlockBorder": "#808080",
		"mdQuote": "#808080", "mdQuoteBorder": "#808080", "mdHr": "#808080",
		"mdListBullet": "#8abeb7", "toolDiffAdded": "#b5bd68", "toolDiffRemoved": "#cc6666",
		"toolDiffContext": "#808080", "syntaxComment": "#6A9955", "syntaxKeyword": "#569CD6",
		"syntaxFunction": "#DCDCAA", "syntaxVariable": "#9CDCFE", "syntaxString": "#CE9178",
		"syntaxNumber": "#B5CEA8", "syntaxType": "#4EC9B0", "syntaxOperator": "#D4D4D4",
		"syntaxPunctuation": "#D4D4D4", "thinkingOff": "#505050", "thinkingMinimal": "#6e6e6e",
		"thinkingLow": "#5f87af", "thinkingMedium": "#81a2be", "thinkingHigh": "#b294bb",
		"thinkingXhigh": "#d183e8", "bashMode": "#b5bd68", "success": "#b5bd68",
		"error": "#cc6666", "warning": "#ffff00", "muted": "#808080", "dim": "#666666",
		"text": "#c9d1d9", "border": "#5f87ff", "borderAccent": "#00d7ff", "borderMuted": "#505050",
		"toolOutput": "#808080",
	}
	var lines []string
	for k, v := range vars {
		lines = append(lines, fmt.Sprintf("      --%s: %s;", k, v))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// ── Template helpers ───────────────────────────────────────────────────────

func fmtTime(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("Jan 2, 2006 3:04 PM")
}

func fmtTokens(n int) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func fmtCost(n float64) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("$%.4f", n)
}

func sessionName(s Session) string {
	if s.Header != nil {
		if name, ok := s.Header["name"].(string); ok && name != "" {
			return name
		}
	}
	for _, e := range s.Entries {
		if e["type"] == "message" {
			msg, ok := e["message"].(map[string]any)
			if ok {
				if role, _ := msg["role"].(string); role == "user" {
					content := msg["content"]
					var text string
					switch v := content.(type) {
					case string:
						text = v
					case []any:
						for _, item := range v {
							if block, ok := item.(map[string]any); ok {
								if t, _ := block["type"].(string); t == "text" {
									text += fmt.Sprintf("%v", block["text"])
								}
							}
						}
					}
					if len(text) > 80 {
						text = text[:80] + "…"
					}
					return text
				}
			}
		}
	}
	return s.Filename
}

var funcMap = template.FuncMap{
	"fmtTime":     fmtTime,
	"fmtTokens":   fmtTokens,
	"fmtCost":     fmtCost,
	"sessionName": sessionName,
}

var indexTmpl = template.Must(template.New("index").Funcs(funcMap).Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Pi Sessions</title>
<style>
:root {
  --body-bg: #18181e;
  --container-bg: #1e1e24;
  --text: #c9d1d9;
  --muted: #808080;
  --dim: #666666;
  --accent: #8abeb7;
  --border: #5f87ff;
  --border-accent: #00d7ff;
  --line-height: 18px;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: ui-monospace, 'Cascadia Code', 'Source Code Pro', Menlo, Consolas, 'DejaVu Sans Mono', monospace;
  font-size: 12px;
  line-height: var(--line-height);
  color: var(--text);
  background: var(--body-bg);
  min-height: 100vh;
}
.header {
  background: var(--container-bg);
  border-bottom: 1px solid var(--dim);
  padding: calc(var(--line-height)) calc(var(--line-height) * 2);
}
.header-inner {
  max-width: 1200px;
  margin: 0 auto;
}
.header h1 {
  font-size: 12px;
  font-weight: bold;
  color: var(--border-accent);
  margin-bottom: var(--line-height);
}
.search-box {
  margin-bottom: var(--line-height);
}
.search-box input {
  width: 100%;
  padding: 4px 8px;
  font-size: 11px;
  font-family: inherit;
  background: var(--body-bg);
  color: var(--text);
  border: 1px solid var(--dim);
  border-radius: 3px;
}
.search-box input:focus {
  outline: none;
  border-color: var(--accent);
}
.search-box input::placeholder {
  color: var(--muted);
}
.stats-bar {
  font-size: 11px;
  color: var(--dim);
}
.content {
  padding: calc(var(--line-height)) calc(var(--line-height) * 2);
  max-width: 1200px;
  margin: 0 auto;
}
.project-group {
  margin-bottom: calc(var(--line-height) * 2);
}
.project-name {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  color: var(--muted);
  letter-spacing: 0.05em;
  margin-bottom: var(--line-height);
  padding-bottom: 4px;
  border-bottom: 1px solid var(--dim);
}
.session-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: var(--line-height);
}
.session-card {
  background: var(--container-bg);
  border: 1px solid var(--dim);
  border-radius: 4px;
  padding: var(--line-height);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}
.session-card:hover {
  border-color: var(--accent);
  background: color-mix(in srgb, var(--container-bg) 95%, var(--accent));
}
.session-card.hidden {
  display: none;
}
.session-title {
  font-size: 12px;
  font-weight: 500;
  color: var(--text);
  margin-bottom: 4px;
  line-height: var(--line-height);
  overflow: hidden;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}
.session-project {
  font-size: 11px;
  color: var(--accent);
  margin-bottom: 8px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.session-meta {
  font-size: 11px;
  color: var(--muted);
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.empty-state {
  text-align: center;
  padding: 60px 20px;
  color: var(--muted);
}
.empty-state h3 {
  margin: 0 0 8px;
  color: var(--text);
}
::-webkit-scrollbar { width: 8px; }
::-webkit-scrollbar-track { background: transparent; }
::-webkit-scrollbar-thumb { background: var(--dim); border-radius: 4px; }
::-webkit-scrollbar-thumb:hover { background: #484f58; }
@media (max-width: 700px) {
  .header, .content { padding: var(--line-height) 16px; }
  .session-grid { grid-template-columns: 1fr; }
}
</style>
</head>
<body>
<div class="header">
  <div class="header-inner">
    <h1>🥧 Pi Sessions</h1>
    <div class="search-box">
      <input type="text" id="search" placeholder="Search sessions..." autofocus>
    </div>
    <div class="stats-bar">{{ len . }} sessions</div>
  </div>
</div>
<div class="content">
  {{ $currentProject := "" }}
  {{ range . }}
    {{ if ne .Project $currentProject }}
      {{ if ne $currentProject "" }}</div>{{ end }}
      <div class="project-group" data-project="{{ .Project | html }}">
        <div class="project-name">{{ .Project | html }}</div>
        <div class="session-grid">
      {{ $currentProject = .Project }}
    {{ end }}
    <div class="session-card" data-id="{{ .ID }}" data-search="{{ sessionName . | html }} {{ .Project | html }}">
      <div class="session-title">{{ sessionName . | html }}</div>
      <div class="session-project">{{ .Project | html }}</div>
      <div class="session-meta">
        <span>{{ fmtTime .LastActivity }}</span>
        <span>{{ .MessageCount }} msgs</span>
        <span>{{ fmtTokens .TokenTotal }} tok</span>
        <span>{{ fmtCost .CostTotal }}</span>
      </div>
    </div>
  {{ end }}
  {{ if ne $currentProject "" }}</div></div>{{ end }}
</div>
<script>
const search = document.getElementById('search');
const cards = document.querySelectorAll('.session-card');
const groups = document.querySelectorAll('.project-group');

search.addEventListener('input', (e) => {
  const q = e.target.value.toLowerCase();
  cards.forEach(card => {
    card.classList.toggle('hidden', !card.dataset.search.toLowerCase().includes(q));
  });
  groups.forEach(g => {
    const visible = g.querySelectorAll('.session-card:not(.hidden)').length;
    g.style.display = visible > 0 ? '' : 'none';
  });
});

cards.forEach(card => {
  card.addEventListener('click', () => {
    window.location.href = '/session?id=' + encodeURIComponent(card.dataset.id);
  });
});
</script>
</body>
</html>`))
