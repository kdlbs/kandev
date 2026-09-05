import { createElement, useEffect, useRef } from "react";
import {
  filterPreviewSrcSet,
  isAllowedPreviewResourceUrl,
  sanitizePreviewCss,
} from "./preview-resource-policy";
import type { PreviewEvent, PreviewSnapshot, PreviewSnapshotNode } from "./preview-runtime-types";

const BLOCKED_TAGS = new Set([
  "base",
  "embed",
  "frame",
  "frameset",
  "iframe",
  "link",
  "meta",
  "object",
  "portal",
  "script",
]);

const NAVIGATION_ATTRIBUTES = new Set([
  "action",
  "cite",
  "download",
  "formaction",
  "href",
  "ping",
  "srcdoc",
  "target",
  "xlink:href",
]);

const RESOURCE_ATTRIBUTES = new Set(["poster", "src"]);
const EVENT_TYPES: PreviewEvent["type"][] = ["click", "input", "change", "submit", "keydown"];
const SVG_TAGS = new Set([
  "circle",
  "ellipse",
  "g",
  "line",
  "path",
  "polygon",
  "polyline",
  "rect",
  "svg",
]);

const ALLOWED_TAGS = new Set([
  "a",
  "abbr",
  "article",
  "aside",
  "b",
  "blockquote",
  "body",
  "br",
  "button",
  "caption",
  "code",
  "col",
  "colgroup",
  "dd",
  "details",
  "div",
  "dl",
  "dt",
  "em",
  "fieldset",
  "figcaption",
  "figure",
  "footer",
  "form",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "header",
  "hr",
  "i",
  "img",
  "input",
  "kbd",
  "label",
  "legend",
  "li",
  "main",
  "mark",
  "nav",
  "ol",
  "option",
  "output",
  "p",
  "pre",
  "progress",
  "section",
  "select",
  "small",
  "span",
  "strong",
  "style",
  "sub",
  "summary",
  "sup",
  "table",
  "tbody",
  "td",
  "textarea",
  "tfoot",
  "th",
  "thead",
  "time",
  "tr",
  "u",
  "ul",
  "video",
  "audio",
  "source",
  "track",
  "svg",
  "circle",
  "ellipse",
  "g",
  "line",
  "path",
  "polygon",
  "polyline",
  "rect",
  "text",
]);

type PreviewRenderRoot = HTMLElement | ShadowRoot;
type RenderContext = {
  document: Document;
  ownedBlobTokens: ReadonlySet<string>;
  resourceUrls: ReadonlyMap<string, string>;
  nodeIds: WeakMap<Element, string>;
};

export function renderPreviewSnapshot(
  root: PreviewRenderRoot,
  snapshot: PreviewSnapshot,
  onEvent: (event: PreviewEvent) => void,
): () => void {
  const document = root.ownerDocument;
  const ownedBlobTokens = new Set(snapshot.resources.map((resource) => resource.token));
  const resourceUrls = createResourceUrls(snapshot);
  const nodeIds = new WeakMap<Element, string>();
  const renderContext: RenderContext = { document, ownedBlobTokens, resourceUrls, nodeIds };
  const container = document.createElement("div");
  container.className = "preview-runtime-root";
  renderNode(container, snapshot.root, renderContext);
  root.replaceChildren(container);

  const listeners = EVENT_TYPES.map((type) => {
    const listener = (event: Event) => {
      const element = findMappedElement(event.target, nodeIds);
      if (!element) return;
      const nodeId = nodeIds.get(element);
      if (!nodeId) return;
      if (type === "submit") event.preventDefault();
      event.stopPropagation();
      onEvent(createPreviewEvent(type, nodeId, event, element));
    };
    root.addEventListener(type, listener);
    return { type, listener };
  });

  return () => {
    for (const { type, listener } of listeners) root.removeEventListener(type, listener);
    for (const url of resourceUrls.values()) URL.revokeObjectURL(url);
    root.replaceChildren();
  };
}

export function PreviewSurface({
  snapshot,
  onEvent,
}: {
  snapshot: PreviewSnapshot;
  onEvent: (event: PreviewEvent) => void;
}) {
  const hostRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return undefined;
    const shadowRoot = host.shadowRoot ?? host.attachShadow({ mode: "open" });
    return renderPreviewSnapshot(shadowRoot, snapshot, onEvent);
  }, [onEvent, snapshot]);

  return createElement("div", {
    ref: hostRef,
    className: "h-full min-h-0 overflow-auto",
    "data-testid": "html-preview-surface",
  });
}

function renderNode(parent: Element, node: PreviewSnapshotNode, context: RenderContext): void {
  if (node.tagName === "#text") {
    parent.append(context.document.createTextNode(node.text ?? ""));
    return;
  }
  const tagName = node.tagName.toLowerCase();
  if (BLOCKED_TAGS.has(tagName) || !ALLOWED_TAGS.has(tagName)) return;
  const element = SVG_TAGS.has(tagName)
    ? context.document.createElementNS("http://www.w3.org/2000/svg", tagName)
    : context.document.createElement(tagName);
  context.nodeIds.set(element, node.id);
  for (const [name, value] of Object.entries(node.attributes)) {
    const attribute = renderAttribute(
      tagName,
      name,
      value,
      context.ownedBlobTokens,
      context.resourceUrls,
    );
    if (!attribute) continue;
    element.setAttribute(attribute.name, attribute.value);
  }
  if (tagName === "style") {
    const css = rewritePreviewBlobUrls(
      sanitizePreviewCss(node.text ?? "", context.ownedBlobTokens),
      context.resourceUrls,
    );
    if (css) element.textContent = css;
    parent.append(element);
    return;
  }
  const style = renderStyles(node, context.ownedBlobTokens, context.resourceUrls);
  if (style) element.setAttribute("style", style);
  for (const child of node.children) renderNode(element, child, context);
  parent.append(element);
}

function renderAttribute(
  tagName: string,
  rawName: string,
  rawValue: string,
  ownedBlobTokens: ReadonlySet<string>,
  resourceUrls: ReadonlyMap<string, string>,
): { name: string; value: string } | undefined {
  const name = rawName.toLowerCase();
  if (name.startsWith("on") || NAVIGATION_ATTRIBUTES.has(name)) return undefined;
  if (name === "style")
    return {
      name,
      value: rewritePreviewBlobUrls(sanitizePreviewCss(rawValue, ownedBlobTokens), resourceUrls),
    };
  if (name === "srcset") {
    const value = rewritePreviewBlobUrls(
      filterPreviewSrcSet(rawValue, ownedBlobTokens),
      resourceUrls,
    );
    return value ? { name, value } : undefined;
  }
  if (RESOURCE_ATTRIBUTES.has(name)) {
    if (!isAllowedPreviewResourceUrl(rawValue, ownedBlobTokens)) return undefined;
    return { name, value: resourceUrls.get(rawValue.trim()) ?? rawValue.trim() };
  }
  if (
    !/^(?:id|class|title|role|lang|dir|name|value|type|alt|width|height|size|rows|cols|rowspan|colspan|for|tabindex|placeholder|checked|selected|disabled|readonly|multiple|open|hidden|kind|label|datetime|scope|viewbox|fill|stroke|stroke-width|d|cx|cy|r|rx|ry|x|x1|x2|y|y1|y2|points|preserveaspectratio|xmlns|xmlns:xlink|data-[\w:.-]+|aria-[\w:.-]+)$/i.test(
      name,
    )
  ) {
    return undefined;
  }
  return { name, value: rawValue };
}

function renderStyles(
  node: PreviewSnapshotNode,
  ownedBlobTokens: ReadonlySet<string>,
  resourceUrls: ReadonlyMap<string, string>,
): string {
  const declarations = Object.entries(node.styles).map(([name, value]) => `${name}: ${value}`);
  const attributeStyle = node.attributes.style;
  if (attributeStyle) declarations.push(attributeStyle);
  return rewritePreviewBlobUrls(
    sanitizePreviewCss(declarations.join("; "), ownedBlobTokens),
    resourceUrls,
  );
}

function createResourceUrls(snapshot: PreviewSnapshot): Map<string, string> {
  const urls = new Map<string, string>();
  for (const resource of snapshot.resources) {
    try {
      urls.set(
        resource.token,
        URL.createObjectURL(new Blob([resource.content], { type: resource.mediaType })),
      );
    } catch {
      // An unavailable browser blob API fails closed by omitting the resource.
    }
  }
  return urls;
}

export function rewritePreviewBlobUrls(
  value: string,
  resourceUrls: ReadonlyMap<string, string>,
): string {
  let result = value;
  const entries = [...resourceUrls.entries()].sort(([left], [right]) => right.length - left.length);
  for (const [token, url] of entries) result = result.replaceAll(token, url);
  return result;
}

function findMappedElement(
  target: EventTarget | null,
  nodeIds: WeakMap<Element, string>,
): Element | undefined {
  let current: Element | null = null;
  if (target instanceof Element) current = target;
  else if (target instanceof Node) current = target.parentElement;
  while (current) {
    if (nodeIds.has(current)) return current;
    current = current.parentElement;
  }
  return undefined;
}

function createPreviewEvent(
  type: PreviewEvent["type"],
  nodeId: string,
  event: Event,
  element: Element,
): PreviewEvent {
  if (type === "keydown") return { type, nodeId, key: (event as KeyboardEvent).key };
  if (type === "input" || type === "change") {
    const input = element as HTMLInputElement;
    return {
      type,
      nodeId,
      value: "value" in input ? input.value : (element.textContent ?? ""),
      checked: "checked" in input ? input.checked : undefined,
    };
  }
  return { type, nodeId };
}
