(function () {
  'use strict';

  var SOURCE = 'kandev-inspector';
  var DRAG_THRESHOLD = 5;
  var Z = 2147483640;

  var active = false;
  var mouseDownAt = null;
  var dragRect = null;
  var hoverEl = null;
  var counter = 0;

  var hoverOverlay = null;
  var dragOverlay = null;
  var popup = null;
  var pendingAnnotation = null;

  function send(type, payload) {
    try {
      window.parent.postMessage({ source: SOURCE, type: type, payload: payload }, '*');
    } catch (e) {}
  }

  function getSelector(el) {
    var parts = [];
    var cur = el;
    while (cur && cur.nodeType === 1 && parts.length < 5) {
      var tag = cur.tagName.toLowerCase();
      var id = cur.getAttribute && cur.getAttribute('id');
      if (id) { parts.unshift(tag + '#' + CSS.escape(id)); break; }
      var cls = '';
      if (cur.classList && cur.classList.length) {
        cls = Array.from(cur.classList).slice(0, 2)
          .map(function (c) { return '.' + CSS.escape(c); }).join('');
      }
      var part = tag + cls;
      var parent = cur.parentElement;
      if (parent) {
        var sibs = Array.from(parent.children).filter(function (c) { return c.tagName === cur.tagName; });
        if (sibs.length > 1) part += ':nth-of-type(' + (sibs.indexOf(cur) + 1) + ')';
      }
      parts.unshift(part);
      cur = parent;
    }
    return parts.join(' > ');
  }

  function captureElement(el) {
    if (!el || el.nodeType !== 1) return null;
    var cls = '';
    if (el.className && typeof el.className === 'string') {
      cls = el.className.trim().split(/\s+/).slice(0, 3).join(' ') || '';
    }
    return {
      tag: el.tagName.toLowerCase(),
      id: el.id || undefined,
      classes: cls || undefined,
      role: el.getAttribute('role') || undefined,
      ariaLabel: el.getAttribute('aria-label') || undefined,
      text: ((el.textContent || '').replace(/\s+/g, ' ').trim()).slice(0, 120) || undefined,
      selector: getSelector(el),
    };
  }

  function elementsInRect(rect) {
    var out = [];
    var all = document.querySelectorAll('body *');
    for (var i = 0; i < all.length && out.length < 5; i++) {
      var el = all[i];
      if (el === hoverOverlay || el === dragOverlay || el === popup) continue;
      if (popup && popup.contains(el)) continue;
      var r = el.getBoundingClientRect();
      if (r.width < 4 || r.height < 4) continue;
      if (r.left >= rect.x && r.top >= rect.y
        && r.right <= rect.x + rect.w && r.bottom <= rect.y + rect.h) {
        out.push(captureElement(el));
      }
    }
    return out;
  }

  function makeOverlay(borderColor, bg) {
    var d = document.createElement('div');
    d.style.cssText = 'position:fixed;pointer-events:none;box-sizing:border-box;display:none;z-index:' + Z + ';'
      + 'border:2px solid ' + borderColor + ';background:' + bg + ';';
    document.body.appendChild(d);
    return d;
  }

  function ensureOverlays() {
    if (!hoverOverlay) hoverOverlay = makeOverlay('#6366f1', 'rgba(99,102,241,0.10)');
    if (!dragOverlay) dragOverlay = makeOverlay('#f59e0b', 'rgba(245,158,11,0.15)');
  }

  function positionOverlay(o, x, y, w, h) {
    o.style.display = 'block';
    o.style.left = x + 'px';
    o.style.top = y + 'px';
    o.style.width = w + 'px';
    o.style.height = h + 'px';
  }

  function hideOverlay(o) { if (o) o.style.display = 'none'; }

  function placeMarker(num, x, y) {
    var m = document.createElement('div');
    m.setAttribute('data-kandev-marker', String(num));
    m.style.cssText = 'position:fixed;left:' + (x - 12) + 'px;top:' + (y - 12) + 'px;width:24px;height:24px;'
      + 'border-radius:50%;background:#6366f1;color:#fff;font:600 12px/24px system-ui,sans-serif;'
      + 'text-align:center;z-index:' + Z + ';pointer-events:none;box-shadow:0 1px 4px rgba(0,0,0,0.3);';
    m.textContent = String(num);
    document.body.appendChild(m);
  }

  function clearMarkers() {
    var nodes = document.querySelectorAll('[data-kandev-marker]');
    for (var i = 0; i < nodes.length; i++) nodes[i].parentNode.removeChild(nodes[i]);
    counter = 0;
  }

  function closePopup() {
    if (popup && popup.parentNode) popup.parentNode.removeChild(popup);
    popup = null;
    pendingAnnotation = null;
  }

  function openCommentPopup(anchorX, anchorY, onSubmit) {
    closePopup();
    popup = document.createElement('div');
    popup.style.cssText = 'position:fixed;left:' + anchorX + 'px;top:' + anchorY + 'px;z-index:' + (Z + 1) + ';'
      + 'background:#fff;border:1px solid #e5e7eb;border-radius:6px;box-shadow:0 4px 12px rgba(0,0,0,0.15);'
      + 'padding:8px;width:280px;font-family:system-ui,sans-serif;';
    var ta = document.createElement('textarea');
    ta.placeholder = 'Describe the change (Enter to save, Esc to cancel)';
    ta.style.cssText = 'width:100%;min-height:60px;border:1px solid #e5e7eb;border-radius:4px;padding:6px;'
      + 'font:13px system-ui,sans-serif;resize:vertical;outline:none;box-sizing:border-box;';
    var row = document.createElement('div');
    row.style.cssText = 'display:flex;gap:6px;justify-content:flex-end;margin-top:6px;';
    var cancel = document.createElement('button');
    cancel.textContent = 'Cancel';
    cancel.style.cssText = 'padding:4px 10px;border:1px solid #e5e7eb;background:#fff;border-radius:4px;cursor:pointer;font:12px system-ui,sans-serif;';
    var save = document.createElement('button');
    save.textContent = 'Save';
    save.style.cssText = 'padding:4px 10px;border:0;background:#6366f1;color:#fff;border-radius:4px;cursor:pointer;font:12px system-ui,sans-serif;';

    cancel.addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); closePopup(); });
    save.addEventListener('click', function (e) { e.preventDefault(); e.stopPropagation(); onSubmit(ta.value.trim()); });
    ta.addEventListener('keydown', function (e) {
      e.stopPropagation();
      if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); onSubmit(ta.value.trim()); }
      if (e.key === 'Escape') { e.preventDefault(); closePopup(); }
    });

    popup.appendChild(ta);
    row.appendChild(cancel); row.appendChild(save);
    popup.appendChild(row);
    document.body.appendChild(popup);
    ta.focus();
  }

  function commitAnnotation(comment) {
    if (!pendingAnnotation) return;
    counter += 1;
    var a = pendingAnnotation;
    a.id = 'a-' + Date.now() + '-' + counter;
    a.number = counter;
    a.comment = comment || '';
    placeMarker(counter, a.markerX, a.markerY);
    send('annotation-added', a);
    pendingAnnotation = null;
    closePopup();
  }

  function startPending(kind, rect, element, elements, markerX, markerY) {
    pendingAnnotation = {
      kind: kind,
      rect: rect,
      element: element || null,
      elements: elements || null,
      markerX: markerX,
      markerY: markerY,
      pagePath: location.pathname + location.search,
    };
    var popupX = Math.min(markerX + 14, window.innerWidth - 296);
    var popupY = Math.min(markerY + 14, window.innerHeight - 140);
    openCommentPopup(popupX, popupY, commitAnnotation);
  }

  function onMouseDown(e) {
    if (popup && popup.contains(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
    mouseDownAt = { x: e.clientX, y: e.clientY };
    dragRect = null;
  }

  function onMouseMove(e) {
    if (popup && popup.contains(e.target)) return;
    if (mouseDownAt) {
      var dx = e.clientX - mouseDownAt.x;
      var dy = e.clientY - mouseDownAt.y;
      if (Math.abs(dx) > DRAG_THRESHOLD || Math.abs(dy) > DRAG_THRESHOLD) {
        dragRect = {
          x: Math.min(mouseDownAt.x, e.clientX),
          y: Math.min(mouseDownAt.y, e.clientY),
          w: Math.abs(dx),
          h: Math.abs(dy),
        };
        hideOverlay(hoverOverlay);
        positionOverlay(dragOverlay, dragRect.x, dragRect.y, dragRect.w, dragRect.h);
      }
      return;
    }
    var el = e.target;
    if (el === hoverOverlay || el === dragOverlay || (popup && popup.contains(el))) return;
    hoverEl = el;
    var r = el.getBoundingClientRect();
    positionOverlay(hoverOverlay, r.left, r.top, r.width, r.height);
  }

  function onMouseUp(e) {
    if (popup && popup.contains(e.target)) return;
    var wasDragging = !!dragRect;
    var rect = dragRect;
    var down = mouseDownAt;
    mouseDownAt = null;
    dragRect = null;
    hideOverlay(dragOverlay);

    if (wasDragging && rect && rect.w > DRAG_THRESHOLD && rect.h > DRAG_THRESHOLD) {
      var inside = elementsInRect(rect);
      startPending('area', rect, null, inside, rect.x + rect.w, rect.y);
      return;
    }
    if (down) {
      var clickEl = document.elementFromPoint(down.x, down.y);
      if (!clickEl || clickEl === hoverOverlay || clickEl === dragOverlay) return;
      if (popup && popup.contains(clickEl)) return;
      var el = captureElement(clickEl);
      startPending('pin', null, el, null, down.x, down.y);
    }
  }

  function onClickCapture(e) {
    if (popup && popup.contains(e.target)) return;
    e.preventDefault();
    e.stopPropagation();
    e.stopImmediatePropagation();
  }

  function onKeyDown(e) {
    if (popup) return;
    if (e.key === 'Escape') { e.preventDefault(); setActive(false); send('inspect-exited', {}); }
  }

  function setActive(isOn) {
    if (isOn === active) return;
    active = isOn;
    if (isOn) {
      ensureOverlays();
      document.body.style.cursor = 'crosshair';
      document.addEventListener('mousedown', onMouseDown, true);
      document.addEventListener('mousemove', onMouseMove, true);
      document.addEventListener('mouseup', onMouseUp, true);
      document.addEventListener('click', onClickCapture, true);
      document.addEventListener('keydown', onKeyDown, true);
    } else {
      document.removeEventListener('mousedown', onMouseDown, true);
      document.removeEventListener('mousemove', onMouseMove, true);
      document.removeEventListener('mouseup', onMouseUp, true);
      document.removeEventListener('click', onClickCapture, true);
      document.removeEventListener('keydown', onKeyDown, true);
      document.body.style.cursor = '';
      hideOverlay(hoverOverlay);
      hideOverlay(dragOverlay);
      closePopup();
    }
  }

  window.addEventListener('message', function (e) {
    if (!e.data || e.data.source !== SOURCE) return;
    if (e.data.type === 'toggle-inspect') setActive(!!(e.data.payload && e.data.payload.active));
    if (e.data.type === 'clear-annotations') clearMarkers();
  });

  send('inspector-ready', {});
})();
