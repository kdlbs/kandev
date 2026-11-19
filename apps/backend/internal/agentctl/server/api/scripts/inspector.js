(function () {
  'use strict';

  var SOURCE = 'kandev-inspector';
  var PROTOCOL_VERSION = 3;
  var Z_INDEX = 2147483640;
  var MAX_OUTER_HTML = 65536;
  var MAX_REGIONS = 32;
  // An anchor that changes size by more than this has probably reflowed its content.
  var ANCHOR_TOLERANCE_PX = 8;
  var ANCHOR_TOLERANCE_RATIO = 0.05;
  var VIEWPORT_TOLERANCE_PX = 8;
  var HIGHLIGHT_NAME = 'kandev-comment-target';
  var HIGHLIGHT_COLOR = 'rgba(99,102,241,0.28)';
  var MAX_COMPARED_TEXT = 4096;
  var mode = null;
  var annotationActive = false;
  var surfaceOpen = false;
  var highlightedId = null;
  var hoveredId = null;
  var targetBoxes = [];
  var highlightStyle = null;
  var revealId = null;
  var revealedId = null;
  var candidate = null;
  var candidateOverlay = null;
  var candidateLabel = null;
  var screenshotOverlay = null;
  var screenshotStart = null;
  var screenshotCurrent = null;
  var projectedMarkers = [];
  var projectedRegions = [];
  var screenshotContainer = null;
  var lastRegionLayout = '';
  var lastSelectionRect = 'null';
  var selectionFrame = false;
  var layoutFrame = false;
  var mutationObserver = null;
  var resizeObserver = null;
  var originalCursor = document.documentElement.style.cursor;
  var originalTouchAction = document.documentElement.style.touchAction;

  function send(type, payload) {
    try {
      window.parent.postMessage({
        source: SOURCE,
        version: PROTOCOL_VERSION,
        type: type,
        payload: payload,
      }, '*');
    } catch (error) {}
  }

  function sanitizedSearch() {
    var sensitiveQueryParameter = /(?:^|[_-])(access[_-]?token|id[_-]?token|token|secret|password|passwd|auth(?:orization)?|credential|cookie|session|capability|signature|sig|nonce|code|state|key)(?:$|[_-])/i;
    var kept = [];
    try {
      var params = new URLSearchParams(location.search);
      params.forEach(function (value, name) {
        if (sensitiveQueryParameter.test(name) || /^kandev_/.test(name)) return;
        kept.push(encodeURIComponent(name) + '=' + encodeURIComponent(value));
      });
    } catch (error) {}
    return kept.length ? '?' + kept.join('&') : '';
  }

  function currentPageRoute() {
    var path = location.pathname;
    var prefix = window.__kandevProxyPrefix;
    if (typeof prefix === 'string' && prefix) {
      if (path === prefix || path.indexOf(prefix + '/') === 0) {
        path = path.slice(prefix.length) || '/';
      }
    }
    return path + sanitizedSearch();
  }

  function pageIdentity() {
    return {
      page_route: currentPageRoute(),
      page_title: String(document.title || '').slice(0, 1024),
    };
  }

  function escapeSelector(value) {
    if (window.CSS && typeof window.CSS.escape === 'function') return window.CSS.escape(value);
    return String(value).replace(/[^a-zA-Z0-9_-]/g, function (character) {
      return '\\' + character;
    });
  }

  function selectorIsUnique(selector) {
    try {
      return document.querySelectorAll(selector).length === 1;
    } catch (error) {
      return false;
    }
  }

  function getSelector(element) {
    var parts = [];
    var current = element;
    while (current && current.nodeType === Node.ELEMENT_NODE && parts.length < 8) {
      var tag = current.tagName.toLowerCase();
      var id = current.getAttribute('id');
      if (id) {
        parts.unshift(tag + '#' + escapeSelector(id));
      } else {
        var part = tag;
        var parent = current.parentElement;
        if (parent) {
          var siblings = Array.prototype.filter.call(parent.children, function (child) {
            return child.tagName === current.tagName;
          });
          if (siblings.length > 1) {
            part += ':nth-of-type(' + (siblings.indexOf(current) + 1) + ')';
          }
        }
        parts.unshift(part);
      }
      var selector = parts.join(' > ');
      if (selector.length <= 4096 && selectorIsUnique(selector)) return selector;
      current = current.parentElement;
    }
    return undefined;
  }

  function elementClasses(element) {
    if (!element.classList) return [];
    return Array.prototype.slice.call(element.classList, 0, 128)
      .map(function (value) { return String(value).slice(0, 1024); });
  }

  function accessibleLabel(element) {
    var explicit = element.getAttribute('aria-label');
    if (explicit) return explicit.slice(0, 16384);
    var labelledBy = element.getAttribute('aria-labelledby');
    if (labelledBy) {
      var labels = labelledBy.split(/\s+/).map(function (id) {
        var label = document.getElementById(id);
        return label ? label.textContent || '' : '';
      }).join(' ').replace(/\s+/g, ' ').trim();
      if (labels) return labels.slice(0, 16384);
    }
    var alt = element.getAttribute('alt') || element.getAttribute('title');
    return alt ? alt.slice(0, 16384) : undefined;
  }

  function captureElement(element) {
    if (!element || element.nodeType !== Node.ELEMENT_NODE) return null;
    var html = element.outerHTML || '';
    return {
      tag: element.tagName.toLowerCase().slice(0, 128),
      id: element.id ? String(element.id).slice(0, 1024) : undefined,
      classes: elementClasses(element),
      role: (element.getAttribute('role') || '').slice(0, 1024) || undefined,
      accessible_label: accessibleLabel(element),
      visible_text: (element.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 16384) || undefined,
      selector: getSelector(element),
      outer_html: html.length > MAX_OUTER_HTML ? html.slice(0, MAX_OUTER_HTML) : html,
    };
  }

  function rectSnapshot(rect) {
    return {
      x: rect.left,
      y: rect.top,
      width: rect.width,
      height: rect.height,
      document_x: rect.left + window.scrollX,
      document_y: rect.top + window.scrollY,
      scroll_x: window.scrollX,
      scroll_y: window.scrollY,
      viewport_width: window.innerWidth,
      viewport_height: window.innerHeight,
      device_pixel_ratio: window.devicePixelRatio || 1,
    };
  }

  function unionRect(rects) {
    if (!rects.length) return null;
    var left = rects[0].left;
    var top = rects[0].top;
    var right = rects[0].right;
    var bottom = rects[0].bottom;
    for (var index = 1; index < rects.length; index += 1) {
      left = Math.min(left, rects[index].left);
      top = Math.min(top, rects[index].top);
      right = Math.max(right, rects[index].right);
      bottom = Math.max(bottom, rects[index].bottom);
    }
    return rectSnapshot({
      left: left,
      top: top,
      width: right - left,
      height: bottom - top,
    });
  }

  function commonElement(range) {
    var node = range.commonAncestorContainer;
    if (node.nodeType === Node.ELEMENT_NODE) return node;
    return node.parentElement;
  }

  function nodePath(root, node) {
    var path = [];
    var current = node;
    while (current && current !== root) {
      var parent = current.parentNode;
      if (!parent) return [];
      var index = Array.prototype.indexOf.call(parent.childNodes, current);
      if (index < 0) return [];
      path.unshift(index);
      current = parent;
      if (path.length > 128) return [];
    }
    return current === root ? path : [];
  }

  function captureTextEndpoint(node, offset, root) {
    return {
      selector: getSelector(root),
      node_path: nodePath(root, node),
      offset: offset,
    };
  }

  function captureTextSelection() {
    var selection = window.getSelection();
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) return false;
    var selectedText = selection.toString();
    if (!selectedText) return false;
    var range = selection.getRangeAt(0);
    var container = commonElement(range);
    var element = captureElement(container);
    if (!container || !element) return false;
    var clientRects = Array.prototype.slice.call(range.getClientRects(), 0, 512);
    var identity = pageIdentity();
    send('capture-completed', {
      kind: 'text',
      page_route: identity.page_route,
      page_title: identity.page_title,
      selected_text: selectedText.slice(0, 262144),
      text_anchor: {
        start: captureTextEndpoint(range.startContainer, range.startOffset, container),
        end: captureTextEndpoint(range.endContainer, range.endOffset, container),
        rects: clientRects.map(rectSnapshot),
        union_rect: unionRect(clientRects),
        scroll_x: window.scrollX,
        scroll_y: window.scrollY,
        viewport_width: window.innerWidth,
        viewport_height: window.innerHeight,
        device_pixel_ratio: window.devicePixelRatio || 1,
        containing_element: element,
      },
    });
    selection.removeAllRanges();
    return true;
  }

  function isInspectorNode(element) {
    return !!(element && element.closest && element.closest('[data-kandev-inspector-ui]'));
  }

  function candidateName(element) {
    var name = element.tagName.toLowerCase();
    if (element.id) name += '#' + element.id;
    var classes = elementClasses(element).slice(0, 2);
    if (classes.length) name += '.' + classes.join('.');
    return name.slice(0, 1024);
  }

  function ensureCandidateOverlay() {
    if (candidateOverlay) return;
    candidateOverlay = document.createElement('div');
    candidateOverlay.setAttribute('data-kandev-inspector-ui', 'candidate');
    candidateOverlay.setAttribute('aria-hidden', 'true');
    candidateOverlay.style.cssText = 'position:fixed;display:none;pointer-events:none;box-sizing:border-box;'
      + 'border:2px solid #6366f1;background:rgba(99,102,241,0.14);z-index:' + Z_INDEX + ';';
    candidateLabel = document.createElement('div');
    candidateLabel.style.cssText = 'position:absolute;left:-2px;bottom:100%;max-width:320px;padding:3px 6px;'
      + 'overflow:hidden;text-overflow:ellipsis;white-space:nowrap;background:#4f46e5;color:#fff;'
      + 'font:600 11px/16px system-ui,sans-serif;border-radius:3px 3px 0 0;';
    candidateOverlay.appendChild(candidateLabel);
    document.documentElement.appendChild(candidateOverlay);
  }

  function showCandidate(element) {
    if (!element || element.nodeType !== Node.ELEMENT_NODE || isInspectorNode(element)) return;
    ensureCandidateOverlay();
    candidate = element;
    var rect = element.getBoundingClientRect();
    candidateOverlay.style.display = 'block';
    candidateOverlay.style.left = rect.left + 'px';
    candidateOverlay.style.top = rect.top + 'px';
    candidateOverlay.style.width = rect.width + 'px';
    candidateOverlay.style.height = rect.height + 'px';
    candidateLabel.textContent = candidateName(element);
    send('candidate-changed', { label: candidateLabel.textContent });
  }

  function hideCandidate() {
    candidate = null;
    if (candidateOverlay) candidateOverlay.style.display = 'none';
    send('candidate-changed', { label: null });
  }

  function ensureScreenshotOverlay() {
    if (screenshotOverlay) return;
    screenshotOverlay = document.createElement('div');
    screenshotOverlay.setAttribute('data-kandev-inspector-ui', 'screenshot-region');
    screenshotOverlay.setAttribute('aria-hidden', 'true');
    screenshotOverlay.style.cssText = 'position:fixed;display:none;pointer-events:none;box-sizing:border-box;'
      + 'border:2px solid #f59e0b;background:rgba(245,158,11,0.16);z-index:' + Z_INDEX + ';';
    document.documentElement.appendChild(screenshotOverlay);
  }

  function positionScreenshotOverlay(start, end) {
    ensureScreenshotOverlay();
    var left = Math.min(start.x, end.x);
    var top = Math.min(start.y, end.y);
    var width = Math.abs(end.x - start.x);
    var height = Math.abs(end.y - start.y);
    screenshotOverlay.style.display = 'block';
    screenshotOverlay.style.left = left + 'px';
    screenshotOverlay.style.top = top + 'px';
    screenshotOverlay.style.width = width + 'px';
    screenshotOverlay.style.height = height + 'px';
  }

  function hideScreenshotOverlay() {
    screenshotStart = null;
    screenshotCurrent = null;
    if (screenshotOverlay) screenshotOverlay.style.display = 'none';
  }

  function captureCandidate(element) {
    if (!element || isInspectorNode(element)) return;
    var snapshot = captureElement(element);
    if (!snapshot) return;
    var identity = pageIdentity();
    send('capture-completed', {
      kind: 'element',
      page_route: identity.page_route,
      page_title: identity.page_title,
      element_snapshot: snapshot,
      capture_rect: rectSnapshot(element.getBoundingClientRect()),
    });
  }

  function onElementPointerMove(event) {
    if (mode !== 'element') return;
    showCandidate(event.target);
  }

  function onElementFocus(event) {
    if (mode !== 'element') return;
    showCandidate(event.target);
  }

  function onElementTouchStart(event) {
    if (mode !== 'element' || !event.touches || !event.touches.length) return;
    var touch = event.touches[0];
    showCandidate(document.elementFromPoint(touch.clientX, touch.clientY));
  }

  function onElementTouchEnd(event) {
    if (mode !== 'element' || !candidate) return;
    event.preventDefault();
    event.stopPropagation();
    captureCandidate(candidate);
  }

  function onElementClick(event) {
    if (mode !== 'element' || isInspectorNode(event.target)) return;
    event.preventDefault();
    event.stopPropagation();
    event.stopImmediatePropagation();
    captureCandidate(event.target);
  }

  function onTextMouseUp() {
    if (mode !== 'text') return;
    window.setTimeout(captureTextSelection, 0);
  }

  function onTextTouchEnd() {
    if (mode !== 'text') return;
    window.setTimeout(captureTextSelection, 0);
  }

  function isMarkerNode(element) {
    return !!(element && element.closest && element.closest('[data-kandev-inspector-marker]'));
  }

  // A drag that starts on a marker still draws a region; a plain click opens the marker.
  function onScreenshotPointerDown(event) {
    if (mode !== 'screenshot') return;
    if (isInspectorNode(event.target) && !isMarkerNode(event.target)) return;
    event.preventDefault();
    event.stopPropagation();
    screenshotStart = { x: event.clientX, y: event.clientY, pointerId: event.pointerId };
    screenshotContainer = scrollContainerAt(event.clientX, event.clientY);
    screenshotCurrent = { x: event.clientX, y: event.clientY };
    positionScreenshotOverlay(screenshotStart, screenshotStart);
  }

  function onScreenshotPointerMove(event) {
    if (mode !== 'screenshot') return;
    if (!screenshotStart || screenshotStart.pointerId !== event.pointerId) return;
    event.preventDefault();
    event.stopPropagation();
    screenshotCurrent = { x: event.clientX, y: event.clientY };
    positionScreenshotOverlay(screenshotStart, screenshotCurrent);
  }

  function completeScreenshot(end) {
    if (mode !== 'screenshot' || !screenshotStart || !end) return false;
    var start = screenshotStart;
    var left = Math.min(start.x, end.x);
    var top = Math.min(start.y, end.y);
    var width = Math.abs(end.x - start.x);
    var height = Math.abs(end.y - start.y);
    hideScreenshotOverlay();
    if (width < 5 || height < 5) return false;
    var identity = pageIdentity();
    var viewportRect = { left: left, top: top, width: width, height: height };
    var captureRect = rectSnapshot(viewportRect);
    var anchor = regionAnchor(viewportRect);
    if (anchor) Object.assign(captureRect, anchor);
    var container = screenshotContainer;
    screenshotContainer = null;
    var selector = container ? getSelector(container) : undefined;
    if (selector) {
      captureRect.scroll_container_selector = selector;
      captureRect.container_scroll_x = container.scrollLeft;
      captureRect.container_scroll_y = container.scrollTop;
    }
    send('screenshot-region-selected', {
      page_route: identity.page_route,
      page_title: identity.page_title,
      capture_rect: captureRect,
    });
    return true;
  }

  function onScreenshotPointerUp(event) {
    if (mode !== 'screenshot') return;
    if (!screenshotStart || screenshotStart.pointerId !== event.pointerId) return;
    event.preventDefault();
    event.stopPropagation();
    event.stopImmediatePropagation();
    completeScreenshot({ x: event.clientX, y: event.clientY });
  }

  function onScreenshotPointerCancel(event) {
    if (mode !== 'screenshot') return;
    if (screenshotStart && screenshotStart.pointerId !== event.pointerId) return;
    hideScreenshotOverlay();
  }

  function onKeyDown(event) {
    if (event.key === 'Escape') {
      if (!mode && !annotationActive) return;
      event.preventDefault();
      event.stopPropagation();
      hideScreenshotOverlay();
      send('escape', {});
      return;
    }
    if (!mode) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      event.stopPropagation();
      if (mode === 'text') {
        window.setTimeout(captureTextSelection, 0);
      } else if (mode === 'screenshot') {
        completeScreenshot(screenshotCurrent);
      }
    }
  }

  function removeCaptureListeners() {
    document.removeEventListener('mousemove', onElementPointerMove, true);
    document.removeEventListener('focusin', onElementFocus, true);
    document.removeEventListener('touchstart', onElementTouchStart, true);
    document.removeEventListener('touchend', onElementTouchEnd, true);
    document.removeEventListener('click', onElementClick, true);
    document.removeEventListener('mouseup', onTextMouseUp, true);
    document.removeEventListener('touchend', onTextTouchEnd, true);
    document.removeEventListener('pointerdown', onScreenshotPointerDown, true);
    document.removeEventListener('pointermove', onScreenshotPointerMove, true);
    document.removeEventListener('pointerup', onScreenshotPointerUp, true);
    document.removeEventListener('pointercancel', onScreenshotPointerCancel, true);
  }

  function setMode(nextMode) {
    removeCaptureListeners();
    mode = nextMode;
    hideCandidate();
    hideScreenshotOverlay();
    document.documentElement.style.cursor = mode === 'element' || mode === 'screenshot'
      ? 'crosshair'
      : originalCursor;
    document.documentElement.style.touchAction = mode === 'screenshot' ? 'none' : originalTouchAction;
    if (!mode) {
      send('capture-mode-changed', { mode: null });
      onSelectionChange();
      return;
    }
    if (lastSelectionRect !== 'null') {
      lastSelectionRect = 'null';
      send('selection-changed', { rect: null });
    }
    switch (mode) {
      case 'element':
        document.addEventListener('mousemove', onElementPointerMove, true);
        document.addEventListener('focusin', onElementFocus, true);
        document.addEventListener('touchstart', onElementTouchStart, true);
        document.addEventListener('touchend', onElementTouchEnd, true);
        document.addEventListener('click', onElementClick, true);
        break;
      case 'text':
        document.addEventListener('mouseup', onTextMouseUp, true);
        document.addEventListener('touchend', onTextTouchEnd, true);
        break;
      case 'screenshot':
        document.addEventListener('pointerdown', onScreenshotPointerDown, true);
        document.addEventListener('pointermove', onScreenshotPointerMove, true);
        document.addEventListener('pointerup', onScreenshotPointerUp, true);
        document.addEventListener('pointercancel', onScreenshotPointerCancel, true);
        break;
      default:
        break;
    }
    send('capture-mode-changed', { mode: mode });
  }

  function isScrollable(element) {
    var style = window.getComputedStyle(element);
    var scrollsY = /(auto|scroll|overlay)/.test(style.overflowY || style.overflow || '')
      && element.scrollHeight > element.clientHeight;
    var scrollsX = /(auto|scroll|overlay)/.test(style.overflowX || style.overflow || '')
      && element.scrollWidth > element.clientWidth;
    return scrollsY || scrollsX;
  }

  // Topmost page element under a point, skipping Kandev's own markers and overlays.
  function pageElementAt(x, y) {
    if (typeof document.elementsFromPoint === 'function') {
      var stack = document.elementsFromPoint(x, y);
      for (var index = 0; index < stack.length; index += 1) {
        if (!isInspectorNode(stack[index])) return stack[index];
      }
      return null;
    }
    return typeof document.elementFromPoint === 'function' ? document.elementFromPoint(x, y) : null;
  }

  // Nearest scrollable ancestor under a point, or null for the document scroller.
  // The scroll container under a point, as stored on a capture rectangle.
  function scrollContainerSnapshot(x, y) {
    var container = scrollContainerAt(x, y);
    var selector = container ? getSelector(container) : '';
    if (!selector) return null;
    return {
      scroll_container_selector: selector,
      container_scroll_x: container.scrollLeft,
      container_scroll_y: container.scrollTop,
    };
  }

  function scrollContainerAt(x, y) {
    var current = pageElementAt(x, y);
    while (current && current !== document.body && current !== document.documentElement) {
      if (current.nodeType === Node.ELEMENT_NODE && !isInspectorNode(current) && isScrollable(current)) {
        return current;
      }
      current = current.parentElement;
    }
    return null;
  }

  function containsRect(outer, inner) {
    return outer.width > 0 && outer.height > 0
      && outer.left <= inner.left + 1 && outer.top <= inner.top + 1
      && outer.left + outer.width >= inner.left + inner.width - 1
      && outer.top + outer.height >= inner.top + inner.height - 1;
  }

  // The smallest page element containing a viewport rectangle, so the region
  // can follow it when the layout changes. The document itself never anchors.
  function regionAnchor(viewportRect) {
    var current = pageElementAt(viewportRect.left + viewportRect.width / 2,
      viewportRect.top + viewportRect.height / 2);
    while (current && current !== document.body && current !== document.documentElement) {
      if (current.nodeType === Node.ELEMENT_NODE && !isInspectorNode(current)) {
        var bounds = current.getBoundingClientRect();
        if (containsRect(bounds, viewportRect)) {
          var selector = getSelector(current);
          if (!selector) return null;
          return {
            anchor_selector: selector,
            anchor_offset_x: viewportRect.left - bounds.left,
            anchor_offset_y: viewportRect.top - bounds.top,
            anchor_width: bounds.width,
            anchor_height: bounds.height,
          };
        }
      }
      current = current.parentElement;
    }
    return null;
  }

  function sizeChanged(current, captured) {
    if (typeof captured !== 'number' || !isFinite(captured)) return false;
    return Math.abs(current - captured) > Math.max(ANCHOR_TOLERANCE_PX, captured * ANCHOR_TOLERANCE_RATIO);
  }

  function queryOrNull(selector) {
    try { return document.querySelector(selector); } catch (error) { return null; }
  }

  function finiteOr(value, fallback) {
    return typeof value === 'number' && isFinite(value) ? value : fallback;
  }

  // Viewport position of an anchored region, or null when its anchor has no
  // rendered box. The anchor's bounding box already includes every scroll.
  function anchoredPosition(rect, container) {
    var anchor = queryOrNull(rect.anchor_selector);
    if (!anchor) return null;
    var bounds = anchor.getBoundingClientRect();
    if (!bounds.width && !bounds.height) return null;
    observeAnchor(anchor);
    var position = {
      x: bounds.left + finiteOr(rect.anchor_offset_x, 0),
      y: bounds.top + finiteOr(rect.anchor_offset_y, 0),
      drifted: sizeChanged(bounds.width, rect.anchor_width) || sizeChanged(bounds.height, rect.anchor_height),
    };
    // Content inside the container scrolls without moving an anchor that holds the container.
    if (container && anchor.contains(container)) {
      position.x -= container.scrollLeft - finiteOr(rect.container_scroll_x, 0);
      position.y -= container.scrollTop - finiteOr(rect.container_scroll_y, 0);
    }
    return position;
  }

  // Capture position minus the container and document scroll that happened since.
  function documentPosition(rect, container) {
    var dx = container ? container.scrollLeft - finiteOr(rect.container_scroll_x, 0) : 0;
    var dy = container ? container.scrollTop - finiteOr(rect.container_scroll_y, 0) : 0;
    var documentX = finiteOr(rect.document_x, finiteOr(rect.x, 0) + finiteOr(rect.scroll_x, 0));
    var documentY = finiteOr(rect.document_y, finiteOr(rect.y, 0) + finiteOr(rect.scroll_y, 0));
    return { x: documentX - window.scrollX - dx, y: documentY - window.scrollY - dy };
  }

  function viewportWidthChanged(rect) {
    return typeof rect.viewport_width === 'number'
      && Math.abs(window.innerWidth - rect.viewport_width) > VIEWPORT_TOLERANCE_PX;
  }

  // Current viewport rectangle of a region, and whether it still lines up with
  // the content it was drawn over.
  function regionLayout(region) {
    var unclipped = unclippedRegionLayout(region);
    var container = unclipped.container;
    var layout = unclipped.layout;
    return container && layout.visible
      ? clipToContainer(layout, container.getBoundingClientRect())
      : layout;
  }

  function unclippedRegionLayout(region) {
    var rect = region.capture_rect || {};
    var container = null;
    var visible = true;
    if (rect.scroll_container_selector) {
      container = queryOrNull(rect.scroll_container_selector);
      if (!container) visible = false;
    }
    var position = rect.anchor_selector ? anchoredPosition(rect, container) : null;
    var drifted;
    if (position) {
      drifted = position.drifted;
    } else {
      position = documentPosition(rect, container);
      drifted = rect.anchor_selector ? true : viewportWidthChanged(rect);
    }
    var layout = {
      id: region.id,
      rect: {
        x: position.x,
        y: position.y,
        width: finiteOr(rect.width, 0),
        height: finiteOr(rect.height, 0),
      },
      visible: visible,
      drifted: drifted,
    };
    return { layout: layout, container: container };
  }

  // A region scrolled partly out of its container is clipped to the container's
  // box; one scrolled entirely out is hidden, so it never covers other content.
  function clipToContainer(layout, box) {
    var r = layout.rect;
    var right = r.x + r.width;
    var bottom = r.y + r.height;
    if (right <= box.left || r.x >= box.right || bottom <= box.top || r.y >= box.bottom) {
      layout.visible = false;
      return layout;
    }
    if (r.x < box.left || r.y < box.top || right > box.right || bottom > box.bottom) {
      layout.clip = { x: box.left, y: box.top, width: box.width, height: box.height };
    }
    return layout;
  }

  function routeMarker(id, route) {
    if (!id) return null;
    return projectedMarkers.filter(function (candidate) {
      return candidate.id === id && candidate.page_route === route;
    })[0] || null;
  }

  // Where a marker's target is, so Kandev can open its comment beside it.
  function markerLayout(marker) {
    var rect = marker ? markerRect(marker) : null;
    if (!rect) return null;
    return {
      id: marker.id,
      rect: { x: rect.left, y: rect.top, width: rect.width || 0, height: rect.height || 0 },
    };
  }

  function highlightedLayout(route) {
    return markerLayout(routeMarker(highlightedId, route));
  }

  function observeAnchor(anchor) {
    if (resizeObserver) resizeObserver.observe(anchor);
  }

  // Layout changes that scroll and resize events miss: reflow and DOM changes.
  function observeLayout(active) {
    if (!active) {
      if (mutationObserver) mutationObserver.disconnect();
      if (resizeObserver) resizeObserver.disconnect();
      mutationObserver = null;
      resizeObserver = null;
      return;
    }
    if (mutationObserver || !document.body) return;
    if (typeof MutationObserver === 'function') {
      mutationObserver = new MutationObserver(scheduleRegionLayout);
      mutationObserver.observe(document.body, {
        childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'class'],
      });
    }
    if (typeof ResizeObserver === 'function') {
      resizeObserver = new ResizeObserver(scheduleRegionLayout);
      resizeObserver.observe(document.documentElement);
      resizeObserver.observe(document.body);
    }
  }

  function sendRegionLayout() {
    layoutFrame = false;
    var route = currentPageRoute();
    var routeRegions = projectedRegions.filter(function (region) { return region.page_route === route; });
    observeLayout(routeRegions.length > 0);
    var payload = { regions: routeRegions.map(regionLayout), highlighted: highlightedLayout(route) };
    var serialized = JSON.stringify(payload);
    if (serialized === lastRegionLayout) return;
    lastRegionLayout = serialized;
    send('region-layout', payload);
  }

  function scheduleRegionLayout() {
    if (layoutFrame) return;
    layoutFrame = true;
    window.requestAnimationFrame(sendRegionLayout);
  }

  function isEditable(node) {
    var element = node && node.nodeType === Node.ELEMENT_NODE ? node : node && node.parentElement;
    if (!element || !element.closest) return false;
    return !!element.closest('input, textarea, [contenteditable=""], [contenteditable="true"]');
  }

  function currentSelectionRect() {
    var selection = window.getSelection();
    if (!selection || selection.isCollapsed || selection.rangeCount === 0) return null;
    var range = selection.getRangeAt(0);
    var container = commonElement(range);
    if (!container || isInspectorNode(container) || isEditable(container)) return null;
    if (!selection.toString().trim()) return null;
    var rects = range.getClientRects();
    if (!rects || !rects.length) return null;
    return rectSnapshot(rects[rects.length - 1]);
  }

  function sendSelectionChange() {
    selectionFrame = false;
    var rect = mode ? null : currentSelectionRect();
    var serialized = JSON.stringify(rect);
    if (serialized === lastSelectionRect) return;
    lastSelectionRect = serialized;
    send('selection-changed', { rect: rect });
  }

  function onSelectionChange() {
    if (mode || selectionFrame) return;
    selectionFrame = true;
    window.requestAnimationFrame(sendSelectionChange);
  }

  function clearMarkerNodes() {
    var markers = document.querySelectorAll('[data-kandev-inspector-marker]');
    for (var index = 0; index < markers.length; index += 1) {
      if (markers[index].parentNode) markers[index].parentNode.removeChild(markers[index]);
    }
  }

  function markerElement(marker) {
    var selector = marker.element_snapshot && marker.element_snapshot.selector;
    return selector ? queryOrNull(selector) : null;
  }

  // A saved rectangle's current viewport position, from its document coordinates.
  function savedViewportRect(rect) {
    if (!rect) return null;
    var documentX = typeof rect.document_x === 'number'
      ? rect.document_x
      : rect.x + (typeof rect.scroll_x === 'number' ? rect.scroll_x : 0);
    var documentY = typeof rect.document_y === 'number'
      ? rect.document_y
      : rect.y + (typeof rect.scroll_y === 'number' ? rect.scroll_y : 0);
    return {
      left: documentX - window.scrollX,
      top: documentY - window.scrollY,
      width: rect.width,
      height: rect.height,
    };
  }

  function markerRect(marker) {
    var element = markerElement(marker);
    if (element) return element.getBoundingClientRect();
    var saved = savedViewportRect(marker.capture_rect || (marker.text_anchor && marker.text_anchor.union_rect));
    return saved ? { left: saved.left, top: saved.top } : null;
  }

  function onMarkerClick(event) {
    event.preventDefault();
    event.stopPropagation();
    var id = event.currentTarget.getAttribute('data-kandev-inspector-marker');
    var layout = markerLayout(routeMarker(id, currentPageRoute()));
    send('marker-activated', layout ? { id: id, rect: layout.rect } : { id: id });
  }

  function endpointNode(endpoint) {
    var root = endpoint && endpoint.selector ? queryOrNull(endpoint.selector) : null;
    if (!root || !Array.isArray(endpoint.node_path)) return null;
    var node = root;
    for (var index = 0; index < endpoint.node_path.length && node; index += 1) {
      node = node.childNodes[endpoint.node_path[index]];
    }
    return node || null;
  }

  function comparableText(value) {
    return String(value).replace(/\s+/g, ' ').trim();
  }

  // The commented text as it is on the page now, or null when it has changed.
  function textRange(marker) {
    var anchor = marker.text_anchor;
    var expected = typeof marker.selected_text === 'string'
      ? comparableText(marker.selected_text.slice(0, MAX_COMPARED_TEXT))
      : '';
    if (!anchor || !expected) return null;
    var start = endpointNode(anchor.start);
    var end = endpointNode(anchor.end);
    if (!start || !end) return null;
    var range = document.createRange();
    try {
      range.setStart(start, anchor.start.offset);
      range.setEnd(end, anchor.end.offset);
    } catch (error) {
      return null;
    }
    return comparableText(range.toString()).indexOf(expected) === 0 ? range : null;
  }

  function highlightRegistry() {
    if (typeof Highlight !== 'function' || typeof CSS === 'undefined' || !CSS.highlights) return null;
    return CSS.highlights;
  }

  function ensureHighlightStyle() {
    if (highlightStyle) return;
    highlightStyle = document.createElement('style');
    highlightStyle.setAttribute('data-kandev-inspector-ui', 'highlight-style');
    highlightStyle.textContent = '::highlight(' + HIGHLIGHT_NAME + '){background-color:' + HIGHLIGHT_COLOR + ';}';
    (document.head || document.documentElement).appendChild(highlightStyle);
  }

  function drawTargetBox(rect, outlined) {
    var box = document.createElement('div');
    box.setAttribute('data-kandev-inspector-ui', 'target-highlight');
    box.setAttribute('aria-hidden', 'true');
    box.style.cssText = 'position:fixed;pointer-events:none;box-sizing:border-box;border-radius:2px;'
      + 'background:' + HIGHLIGHT_COLOR + ';'
      + (outlined ? 'border:2px solid #6366f1;' : '')
      + 'z-index:' + (Z_INDEX - 1) + ';';
    box.style.left = rect.left + 'px';
    box.style.top = rect.top + 'px';
    box.style.width = rect.width + 'px';
    box.style.height = rect.height + 'px';
    document.documentElement.appendChild(box);
    targetBoxes.push(box);
  }

  function clearTargetHighlight() {
    targetBoxes.forEach(function (box) {
      if (box.parentNode) box.parentNode.removeChild(box);
    });
    targetBoxes = [];
    var registry = highlightRegistry();
    if (registry) registry.delete(HIGHLIGHT_NAME);
  }

  // Adds what a text or element comment points at: live text, the live element,
  // or the boxes saved at capture time when the page no longer has them.
  function collectTarget(marker, target) {
    if (marker.kind === 'text') {
      var range = textRange(marker);
      if (range) {
        target.ranges.push(range);
        return;
      }
      var saved = marker.text_anchor && Array.isArray(marker.text_anchor.rects) ? marker.text_anchor.rects : [];
      saved.map(savedViewportRect).forEach(function (rect) {
        if (rect) target.boxes.push({ rect: rect, outlined: false });
      });
      return;
    }
    var element = markerElement(marker);
    var rect = element ? element.getBoundingClientRect() : savedViewportRect(marker.capture_rect);
    if (rect) target.boxes.push({ rect: rect, outlined: true });
  }

  // Highlights the hovered marker's target and the target of the open comment.
  function renderTargetHighlight() {
    clearTargetHighlight();
    var route = currentPageRoute();
    var target = { ranges: [], boxes: [] };
    [hoveredId, highlightedId].forEach(function (id, index, ids) {
      if (!id || ids.indexOf(id) !== index) return;
      var marker = routeMarker(id, route);
      if (marker && marker.kind !== 'screenshot') collectTarget(marker, target);
    });
    var registry = highlightRegistry();
    if (registry && target.ranges.length) {
      ensureHighlightStyle();
      var highlight = new Highlight();
      target.ranges.forEach(function (range) { highlight.add(range); });
      registry.set(HIGHLIGHT_NAME, highlight);
    } else {
      target.ranges.forEach(function (range) {
        Array.prototype.forEach.call(range.getClientRects(), function (rect) {
          target.boxes.push({ rect: rect, outlined: false });
        });
      });
    }
    target.boxes.forEach(function (box) { drawTargetBox(box.rect, box.outlined); });
  }

  function setHoveredMarker(id) {
    if (id === hoveredId) return;
    hoveredId = id;
    renderTargetHighlight();
  }

  function onMarkerHover(event) {
    var node = event.target && event.target.closest
      ? event.target.closest('[data-kandev-inspector-marker]')
      : null;
    setHoveredMarker(node ? node.getAttribute('data-kandev-inspector-marker') : null);
  }

  function onPointerLeavePage(event) {
    if (!event.relatedTarget) setHoveredMarker(null);
  }

  function markerNode(marker, index, rect) {
    var node = document.createElement('div');
    var highlighted = marker.id === highlightedId;
    node.setAttribute('data-kandev-inspector-marker', marker.id);
    node.setAttribute('data-kandev-inspector-ui', 'marker');
    node.setAttribute('aria-hidden', 'true');
    if (highlighted) node.setAttribute('data-highlighted', 'true');
    var colors = marker.pending
      ? 'background:#fff;color:#4f46e5;border:2px dashed #4f46e5;'
      : 'background:#4f46e5;color:#fff;border:2px solid #4f46e5;';
    var ring = highlighted ? '0 0 0 4px rgba(79,70,229,0.45),' : '';
    node.style.cssText = 'position:fixed;left:' + (rect.left - 11) + 'px;top:' + (rect.top - 11)
      + 'px;width:22px;height:22px;box-sizing:border-box;border-radius:50%;' + colors
      + 'font:600 11px/18px system-ui,sans-serif;text-align:center;'
      + 'pointer-events:auto;cursor:pointer;'
      + 'box-shadow:' + ring + '0 1px 4px rgba(0,0,0,0.3);z-index:' + Z_INDEX + ';';
    node.textContent = String(typeof marker.number === 'number' ? marker.number : index + 1);
    node.addEventListener('click', onMarkerClick);
    return node;
  }

  // Only a capture picked from the comment list scrolls the page; new captures never do.
  // Scrolls by the distance that centers a span [top, bottom] in [start, end], if it is outside.
  function centeringScroll(top, bottom, start, end) {
    if (top >= start && bottom <= end) return 0;
    return top - start - (end - start - (bottom - top)) / 2;
  }

  // A region capture has no in-page marker, so it is revealed from its layout,
  // first inside its scroll container and then in the document.
  function revealRegion(region) {
    var unclipped = unclippedRegionLayout(region);
    if (!unclipped.layout.visible) return;
    var rect = unclipped.layout.rect;
    var top = rect.y;
    var bottom = rect.y + rect.height;
    if (unclipped.container) {
      var box = unclipped.container.getBoundingClientRect();
      var inner = centeringScroll(top, bottom, box.top, box.bottom);
      if (inner) {
        unclipped.container.scrollBy({ top: inner, behavior: 'smooth' });
        top = box.top + Math.max(0, (box.height - rect.height) / 2);
        bottom = top + rect.height;
      }
    }
    var outer = centeringScroll(top, bottom, 0, window.innerHeight);
    if (outer) window.scrollBy({ top: outer, behavior: 'smooth' });
  }

  function scrollRevealIntoView() {
    if (!revealId || revealId === revealedId) return;
    var node = document.querySelector('[data-kandev-inspector-marker="' + revealId.replace(/["\\]/g, '') + '"]');
    var region = node ? null : routeRegion(revealId, currentPageRoute());
    if (!node && !region) return;
    revealedId = revealId;
    if (region) {
      revealRegion(region);
      return;
    }
    var rect = node.getBoundingClientRect();
    if (rect.top >= 0 && rect.bottom <= window.innerHeight) return;
    window.scrollBy({ top: rect.top - window.innerHeight / 2, behavior: 'smooth' });
  }

  function routeRegion(id, route) {
    return projectedRegions.filter(function (candidate) {
      return candidate.id === id && candidate.page_route === route;
    })[0] || null;
  }

  function onPagePointerDown(event) {
    if (!surfaceOpen || isInspectorNode(event.target)) return;
    send('page-pointerdown', {});
  }

  function renderMarkers() {
    clearMarkerNodes();
    var route = currentPageRoute();
    var regionIds = {};
    projectedRegions.forEach(function (region) { regionIds[region.id] = true; });
    var visible = projectedMarkers.filter(function (marker) {
      return marker.page_route === route && !regionIds[marker.id];
    });
    visible.forEach(function (marker, index) {
      var rect = markerRect(marker);
      if (!rect) return;
      document.documentElement.appendChild(markerNode(marker, index, rect));
    });
    renderTargetHighlight();
    scrollRevealIntoView();
    scheduleRegionLayout();
  }

  function answerAnchorRequest(payload) {
    var rect = payload.rect || {};
    var valid = ['x', 'y', 'width', 'height'].every(function (field) {
      return typeof rect[field] === 'number' && isFinite(rect[field]);
    });
    var anchor = valid
      ? regionAnchor({ left: rect.x, top: rect.y, width: rect.width, height: rect.height })
      : null;
    send('region-anchored', {
      request_id: String(payload.request_id || ''),
      anchor: anchor,
      container: valid ? scrollContainerSnapshot(rect.x + rect.width / 2, rect.y + rect.height / 2) : null,
    });
  }

  function routeDidChange() {
    hideCandidate();
    var identity = pageIdentity();
    send('route-changed', identity);
    renderMarkers();
  }

  var originalPushState = history.pushState;
  history.pushState = function () {
    var result = originalPushState.apply(this, arguments);
    routeDidChange();
    return result;
  };
  var originalReplaceState = history.replaceState;
  history.replaceState = function () {
    var result = originalReplaceState.apply(this, arguments);
    routeDidChange();
    return result;
  };
  document.addEventListener('keydown', onKeyDown, true);
  document.addEventListener('pointerdown', onPagePointerDown, true);
  document.addEventListener('mouseover', onMarkerHover, true);
  document.addEventListener('mouseout', onPointerLeavePage, true);
  document.addEventListener('selectionchange', onSelectionChange);
  window.addEventListener('popstate', routeDidChange);
  window.addEventListener('hashchange', routeDidChange);
  window.addEventListener('scroll', renderMarkers, true);
  window.addEventListener('resize', function () {
    if (candidate) showCandidate(candidate);
    renderMarkers();
  });

  window.addEventListener('message', function (event) {
    var message = event.data;
    if (!message || message.source !== SOURCE || message.version !== PROTOCOL_VERSION) return;
    switch (message.type) {
      case 'set-capture-mode':
        setMode(message.payload && message.payload.mode);
        break;
      case 'project-markers':
        projectedMarkers = message.payload && Array.isArray(message.payload.markers)
          ? message.payload.markers.slice()
          : [];
        projectedRegions = message.payload && Array.isArray(message.payload.regions)
          ? message.payload.regions.slice(0, MAX_REGIONS)
          : [];
        highlightedId = message.payload && typeof message.payload.highlighted_id === 'string'
          ? message.payload.highlighted_id
          : null;
        revealId = message.payload && typeof message.payload.reveal_id === 'string'
          ? message.payload.reveal_id
          : null;
        if (!revealId) revealedId = null;
        renderMarkers();
        break;
      case 'set-annotation-active':
        annotationActive = !!(message.payload && message.payload.active);
        surfaceOpen = !!(message.payload && message.payload.surface_open);
        renderMarkers();
        break;
      case 'capture-selection':
        captureTextSelection();
        break;
      case 'anchor-region':
        answerAnchorRequest(message.payload || {});
        break;
      default:
        break;
    }
  });

  send('inspector-ready', pageIdentity());
})();
