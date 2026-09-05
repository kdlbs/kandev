import type { QuickJSHandle } from "quickjs-emscripten";
import { parse, parseFragment } from "parse5";
import type { DefaultTreeAdapterTypes } from "parse5";
import type { PreviewEventType, PreviewSnapshotNode } from "./preview-runtime-types";
import { sanitizePreviewAttributes } from "./preview-resource-policy";

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

const SUPPORTED_INLINE_EVENTS: Record<string, PreviewEventType> = {
  onclick: "click",
  oninput: "input",
  onchange: "change",
  onsubmit: "submit",
  onkeydown: "keydown",
};

export type PreviewInlineHandler = {
  node: VirtualNode;
  type: PreviewEventType;
  source: string;
};

export type VirtualNode = {
  id: string;
  tagName: string;
  namespace: string;
  parent: VirtualNode | null;
  children: VirtualNode[];
  attributes: Map<string, string>;
  eventHandlers: Map<PreviewEventType, QuickJSHandle[]>;
  wrapper?: QuickJSHandle;
  dataset?: QuickJSHandle;
  style?: QuickJSHandle;
  classList?: QuickJSHandle;
  text?: string;
  blocked?: boolean;
};

type PreviewTreeNode = DefaultTreeAdapterTypes.Node;
type PreviewElement = DefaultTreeAdapterTypes.Element;
const HTML_NAMESPACE = "http://www.w3.org/1999/xhtml";

function isElement(node: PreviewTreeNode): node is PreviewElement {
  return "tagName" in node;
}

function isText(node: PreviewTreeNode): node is DefaultTreeAdapterTypes.TextNode {
  return node.nodeName === "#text";
}

function textContent(node: PreviewTreeNode): string {
  if (isText(node)) return node.value;
  if (!("childNodes" in node)) return "";
  return node.childNodes.map(textContent).join("");
}

function isSupportedScript(element: PreviewElement): boolean {
  const type = element.attrs.find((attribute) => attribute.name.toLowerCase() === "type")?.value;
  return !type || /^(?:text|application)\/javascript$|^module$/i.test(type.trim());
}

function normalizeAttributeName(name: string): string {
  return name.toLowerCase() === "xlink:href" ? "xlink:href" : name.toLowerCase();
}

export class PreviewVirtualDocument {
  readonly html: VirtualNode;
  readonly head: VirtualNode;
  readonly body: VirtualNode;
  readonly scripts: string[] = [];
  readonly inlineHandlers: PreviewInlineHandler[] = [];
  private readonly nodes = new Map<string, VirtualNode>();
  private nextNodeId = 1;

  constructor(source: string) {
    const parsed = parse(source);
    this.html = this.createNode("html", HTML_NAMESPACE, null);
    this.head = this.createNode("head", HTML_NAMESPACE, this.html);
    this.body = this.createNode("body", HTML_NAMESPACE, this.html);
    this.html.children.push(this.head, this.body);

    const htmlElement = parsed.childNodes.find(
      (node): node is PreviewElement => isElement(node) && node.tagName === "html",
    );
    const headElement = htmlElement?.childNodes.find(
      (node): node is PreviewElement => isElement(node) && node.tagName === "head",
    );
    const bodyElement = htmlElement?.childNodes.find(
      (node): node is PreviewElement => isElement(node) && node.tagName === "body",
    );

    if (headElement) this.copyAttributes(this.head, headElement);
    if (bodyElement) this.copyAttributes(this.body, bodyElement);

    for (const node of headElement?.childNodes ?? []) this.appendParsedNode(this.head, node, true);
    for (const node of bodyElement?.childNodes ?? []) this.appendParsedNode(this.body, node, true);
  }

  createElement(tagName: string, namespace = "http://www.w3.org/1999/xhtml"): VirtualNode {
    const normalized = tagName.toLowerCase();
    const node = this.createNode(normalized, namespace, null);
    if (BLOCKED_TAGS.has(normalized)) node.blocked = true;
    return node;
  }

  createTextNode(value: string): VirtualNode {
    const node = this.createNode("#text", "", null);
    node.text = value;
    return node;
  }

  appendChild(parent: VirtualNode, child: VirtualNode): VirtualNode {
    this.detach(child);
    child.parent = parent;
    parent.children.push(child);
    return child;
  }

  insertBefore(
    parent: VirtualNode,
    child: VirtualNode,
    reference: VirtualNode | null,
  ): VirtualNode {
    this.detach(child);
    child.parent = parent;
    if (!reference) {
      parent.children.push(child);
      return child;
    }
    const index = parent.children.indexOf(reference);
    if (index < 0) parent.children.push(child);
    else parent.children.splice(index, 0, child);
    return child;
  }

  removeChild(parent: VirtualNode, child: VirtualNode): VirtualNode {
    const index = parent.children.indexOf(child);
    if (index >= 0) {
      parent.children.splice(index, 1);
      child.parent = null;
    }
    return child;
  }

  remove(node: VirtualNode): void {
    if (node.parent) this.removeChild(node.parent, node);
  }

  replaceChildren(parent: VirtualNode, children: VirtualNode[]): void {
    for (const child of parent.children) child.parent = null;
    parent.children = [];
    for (const child of children) this.appendChild(parent, child);
  }

  setTextContent(node: VirtualNode, value: string): void {
    if (node.tagName === "#text") {
      node.text = value;
      return;
    }
    this.replaceChildren(node, value ? [this.createTextNode(value)] : []);
  }

  getTextContent(node: VirtualNode): string {
    if (node.tagName === "#text") return node.text ?? "";
    return node.children.map((child) => this.getTextContent(child)).join("");
  }

  getElementById(id: string): VirtualNode | undefined {
    return this.find((node) => node.attributes.get("id") === id);
  }

  querySelector(selector: string): VirtualNode | undefined {
    const normalized = selector.trim();
    if (!normalized || /[\s>+~]/.test(normalized)) return undefined;
    return this.find((node) => this.matchesSelector(node, normalized));
  }

  querySelectorAll(selector: string): VirtualNode[] {
    const normalized = selector.trim();
    if (!normalized || /[\s>+~]/.test(normalized)) return [];
    return [...this.nodes.values()].filter(
      (node) => node.tagName !== "#text" && this.matchesSelector(node, normalized),
    );
  }

  getElementsByTagName(tagName: string): VirtualNode[] {
    const normalized = tagName.trim().toLowerCase();
    if (!normalized) return [];
    return [...this.nodes.values()].filter(
      (node) => node.tagName !== "#text" && (normalized === "*" || node.tagName === normalized),
    );
  }

  getChildren(node: VirtualNode): VirtualNode[] {
    return [...node.children];
  }

  setAttribute(node: VirtualNode, name: string, value: string): void {
    const normalized = normalizeAttributeName(name);
    if (normalized.startsWith("on")) return;
    node.attributes.set(normalized, value);
  }

  getAttribute(node: VirtualNode, name: string): string | null {
    return node.attributes.get(normalizeAttributeName(name)) ?? null;
  }

  hasAttribute(node: VirtualNode, name: string): boolean {
    return node.attributes.has(normalizeAttributeName(name));
  }

  removeAttribute(node: VirtualNode, name: string): void {
    node.attributes.delete(normalizeAttributeName(name));
  }

  parseFragmentInto(parent: VirtualNode, source: string): VirtualNode[] {
    const fragment = parseFragment(source);
    const nodes = fragment.childNodes.flatMap((node) => this.convertParsedNode(node, false));
    for (const node of nodes) this.appendChild(parent, node);
    return nodes;
  }

  snapshot(ownedBlobTokens: ReadonlySet<string>): PreviewSnapshotNode {
    const headStyles = this.head.children.filter((child) => child.tagName === "style");
    return this.snapshotNode(this.body, ownedBlobTokens, [...headStyles, ...this.body.children]);
  }

  allNodes(): Iterable<VirtualNode> {
    return this.nodes.values();
  }

  findNode(id: string): VirtualNode | undefined {
    return this.nodes.get(id);
  }

  private createNode(tagName: string, namespace: string, parent: VirtualNode | null): VirtualNode {
    const node: VirtualNode = {
      id: `preview-node-${this.nextNodeId++}`,
      tagName,
      namespace,
      parent,
      children: [],
      attributes: new Map(),
      eventHandlers: new Map(),
    };
    this.nodes.set(node.id, node);
    return node;
  }

  private copyAttributes(target: VirtualNode, source: PreviewElement): void {
    for (const attribute of source.attrs) {
      const name = normalizeAttributeName(attribute.name);
      if (name.startsWith("on")) {
        const eventType = SUPPORTED_INLINE_EVENTS[name];
        if (eventType && attribute.value) {
          this.inlineHandlers.push({ node: target, type: eventType, source: attribute.value });
        }
        continue;
      }
      target.attributes.set(name, attribute.value);
    }
  }

  private appendParsedNode(
    parent: VirtualNode,
    source: PreviewTreeNode,
    collectScripts: boolean,
  ): void {
    for (const node of this.convertParsedNode(source, collectScripts))
      this.appendChild(parent, node);
  }

  private convertParsedNode(source: PreviewTreeNode, collectScripts: boolean): VirtualNode[] {
    if (isText(source)) return [this.createTextNode(source.value)];
    if (!isElement(source)) return [];

    const tagName = source.tagName.toLowerCase();
    if (tagName === "script") {
      if (
        collectScripts &&
        !source.attrs.some((attribute) => attribute.name.toLowerCase() === "src") &&
        isSupportedScript(source)
      ) {
        const code = textContent(source);
        if (code.trim()) this.scripts.push(code);
      }
      return [];
    }
    if (BLOCKED_TAGS.has(tagName)) return [];

    const node = this.createNode(tagName, source.namespaceURI, null);
    this.copyAttributes(node, source);
    for (const child of source.childNodes) this.appendParsedNode(node, child, collectScripts);
    return [node];
  }

  private detach(node: VirtualNode): void {
    if (node.parent) this.removeChild(node.parent, node);
  }

  private find(predicate: (node: VirtualNode) => boolean): VirtualNode | undefined {
    for (const node of this.nodes.values()) {
      if (node.tagName !== "#text" && predicate(node)) return node;
    }
    return undefined;
  }

  private matchesSelector(node: VirtualNode, selector: string): boolean {
    if (selector.startsWith("#")) return node.attributes.get("id") === selector.slice(1);
    if (selector.startsWith(".")) {
      return node.attributes.get("class")?.split(/\s+/).includes(selector.slice(1)) ?? false;
    }
    const attributeMatch = selector.match(/^([\w-]+)?\[([\w:-]+)(?:=["']?([^\]"']+)["']?)?\]$/);
    if (attributeMatch) {
      if (attributeMatch[1] && node.tagName !== attributeMatch[1].toLowerCase()) return false;
      const actual = node.attributes.get(attributeMatch[2].toLowerCase());
      return actual !== undefined && (!attributeMatch[3] || actual === attributeMatch[3]);
    }
    return node.tagName === selector.toLowerCase();
  }

  private snapshotNode(
    node: VirtualNode,
    ownedBlobTokens: ReadonlySet<string>,
    children: VirtualNode[] = node.children,
  ): PreviewSnapshotNode {
    const attributes = sanitizePreviewAttributes(
      node.tagName,
      Object.fromEntries(node.attributes),
      ownedBlobTokens,
    );
    return {
      id: node.id,
      tagName: node.tagName,
      attributes,
      styles: {},
      text: this.getTextContent(node),
      children: children
        .filter((child) => !child.blocked)
        .map((child) => this.snapshotNode(child, ownedBlobTokens)),
      eventTypes: [...node.eventHandlers.keys()],
    };
  }
}
