import type { QuickJSContext, QuickJSHandle } from "quickjs-emscripten";
import { PreviewVirtualDocument, type VirtualNode } from "./preview-runtime-document";
import type { PreviewEvent, PreviewEventType } from "./preview-runtime-types";
import {
  addMethod,
  defineGetter,
  defineSetter,
  defineValue,
  fromDatasetKey,
  fromStyleKey,
  readHiddenString,
  readObjectProperties,
  setProp,
  toBoolean,
  toDatasetKey,
  toString,
  toStyleKey,
  type OwnHandle,
} from "./preview-runtime-vm-helpers";

const NODE_EVENT_ATTRIBUTES: Record<string, PreviewEventType> = {
  onclick: "click",
  oninput: "input",
  onchange: "change",
  onsubmit: "submit",
  onkeydown: "keydown",
};
// i18n-exempt: internal runtime failure marker, never rendered
const PREVIEW_BUDGET_ERROR = "preview budget exceeded";

export type PreviewRuntimeNodeApiOptions = {
  context: QuickJSContext;
  document: PreviewVirtualDocument;
  getDocumentHandle: () => QuickJSHandle | undefined;
  own: OwnHandle;
  releaseHandle: (handle: QuickJSHandle) => void;
  dispatch: (node: VirtualNode, event: PreviewEvent) => void;
  maxEventQueue: number;
};

export class PreviewRuntimeNodeApi {
  private readonly context: QuickJSContext;
  private readonly document: PreviewVirtualDocument;
  private readonly getDocumentHandle: () => QuickJSHandle | undefined;
  private readonly own: OwnHandle;
  private readonly releaseHandle: (handle: QuickJSHandle) => void;
  private readonly dispatch: (node: VirtualNode, event: PreviewEvent) => void;
  private readonly maxEventQueue: number;

  constructor(options: PreviewRuntimeNodeApiOptions) {
    this.context = options.context;
    this.document = options.document;
    this.getDocumentHandle = options.getDocumentHandle;
    this.own = options.own;
    this.releaseHandle = options.releaseHandle;
    this.dispatch = options.dispatch;
    this.maxEventQueue = options.maxEventQueue;
  }

  installExistingNodeWrappers(): void {
    for (const node of this.document.allNodes()) this.getNodeWrapper(node);
  }

  getNodeWrapper(node: VirtualNode): QuickJSHandle {
    if (node.wrapper) return node.wrapper;
    const wrapper = this.own(this.context.newObject());
    node.wrapper = wrapper;
    defineValue(this.context, wrapper, "__previewNodeId", node.id, {
      enumerable: false,
      configurable: false,
    });
    defineValue(
      this.context,
      wrapper,
      "tagName",
      node.tagName === "#text" ? "#text" : node.tagName.toUpperCase(),
    );
    defineValue(
      this.context,
      wrapper,
      "nodeName",
      node.tagName === "#text" ? "#text" : node.tagName.toUpperCase(),
    );
    defineGetter(this.context, wrapper, "ownerDocument", () =>
      (this.getDocumentHandle() ?? this.context.null).dup(),
    );
    defineGetter(this.context, wrapper, "parentNode", () =>
      node.parent ? this.getGuestNodeWrapper(node.parent) : this.context.null,
    );
    defineGetter(this.context, wrapper, "parentElement", () =>
      node.parent && node.parent.tagName !== "#text"
        ? this.getGuestNodeWrapper(node.parent)
        : this.context.null,
    );
    defineGetter(this.context, wrapper, "firstChild", () =>
      node.children[0] ? this.getGuestNodeWrapper(node.children[0]) : this.context.null,
    );
    defineGetter(this.context, wrapper, "lastChild", () => {
      const child = node.children[node.children.length - 1];
      return child ? this.getGuestNodeWrapper(child) : this.context.null;
    });
    defineGetter(this.context, wrapper, "children", () =>
      this.newNodeArray(node.children.filter((child) => child.tagName !== "#text")),
    );
    defineGetter(this.context, wrapper, "childNodes", () => this.newNodeArray(node.children));
    defineGetter(this.context, wrapper, "textContent", () =>
      this.context.newString(this.document.getTextContent(node)),
    );
    defineSetter(this.context, wrapper, "textContent", (value) =>
      this.document.setTextContent(node, toString(this.context, value)),
    );
    defineGetter(this.context, wrapper, "innerText", () =>
      this.context.newString(this.document.getTextContent(node)),
    );
    defineSetter(this.context, wrapper, "innerText", (value) =>
      this.document.setTextContent(node, toString(this.context, value)),
    );
    defineGetter(this.context, wrapper, "innerHTML", () =>
      this.context.newString(this.serializeChildren(node)),
    );
    defineSetter(this.context, wrapper, "innerHTML", (value) => {
      this.document.replaceChildren(node, []);
      this.document.parseFragmentInto(node, toString(this.context, value));
    });
    this.installAttributeProperties(wrapper, node);
    this.installNodeMethods(wrapper, node);
    this.installNodeCollections(wrapper, node);
    this.installNodeEvents(wrapper, node);
    this.installDataset(wrapper, node);
    this.installStyle(wrapper, node);
    this.installClassList(wrapper, node);
    return wrapper;
  }

  getGuestNodeWrapper(node: VirtualNode): QuickJSHandle {
    return this.getNodeWrapper(node).dup();
  }

  syncGuestObjects(): void {
    for (const node of this.document.allNodes()) {
      if (node.dataset) this.syncDataset(node, node.dataset);
      if (node.style) this.syncStyle(node, node.style);
    }
  }

  private installAttributeProperties(wrapper: QuickJSHandle, node: VirtualNode): void {
    for (const name of ["id", "className", "value", "src", "alt", "title", "name", "type"]) {
      const attributeName = name === "className" ? "class" : name;
      defineGetter(this.context, wrapper, name, () => {
        const value = this.document.getAttribute(node, attributeName);
        return this.context.newString(value ?? "");
      });
      defineSetter(this.context, wrapper, name, (value) =>
        this.document.setAttribute(node, attributeName, toString(this.context, value)),
      );
    }
    for (const name of ["checked", "selected", "disabled", "hidden", "open"]) {
      defineGetter(this.context, wrapper, name, () =>
        this.document.hasAttribute(node, name) ? this.context.true : this.context.false,
      );
      defineSetter(this.context, wrapper, name, (value) => {
        if (toBoolean(this.context, value)) this.document.setAttribute(node, name, "");
        else this.document.removeAttribute(node, name);
      });
    }
  }

  private installNodeMethods(wrapper: QuickJSHandle, node: VirtualNode): void {
    addMethod(this.context, this.own, wrapper, "appendChild", (_this, child) => {
      const childNode = this.nodeFromHandle(child);
      if (!childNode) throw new Error("runtime-error");
      this.document.appendChild(node, childNode);
      return child;
    });
    addMethod(this.context, this.own, wrapper, "insertBefore", (_this, child, reference) => {
      const childNode = this.nodeFromHandle(child);
      const referenceNode =
        reference && this.context.typeof(reference) !== "null"
          ? (this.nodeFromHandle(reference) ?? null)
          : null;
      if (!childNode) throw new Error("runtime-error");
      this.document.insertBefore(node, childNode, referenceNode);
      return child;
    });
    addMethod(this.context, this.own, wrapper, "removeChild", (_this, child) => {
      const childNode = this.nodeFromHandle(child);
      if (!childNode) throw new Error("runtime-error");
      this.document.removeChild(node, childNode);
      return child;
    });
    addMethod(this.context, this.own, wrapper, "remove", () => {
      this.document.remove(node);
      return this.context.undefined;
    });
    addMethod(this.context, this.own, wrapper, "replaceChildren", (_this, ...children) => {
      this.document.replaceChildren(
        node,
        children.flatMap((child) => this.toVirtualNodes(child)),
      );
      return this.context.undefined;
    });
    addMethod(this.context, this.own, wrapper, "append", (_this, ...children) => {
      for (const child of children) {
        for (const childNode of this.toVirtualNodes(child))
          this.document.appendChild(node, childNode);
      }
      return this.context.undefined;
    });
    addMethod(this.context, this.own, wrapper, "prepend", (_this, ...children) => {
      for (const child of [...children].reverse()) {
        for (const childNode of this.toVirtualNodes(child)) {
          this.document.insertBefore(node, childNode, node.children[0] ?? null);
        }
      }
      return this.context.undefined;
    });
    addMethod(this.context, this.own, wrapper, "setAttribute", (_this, name, value) => {
      this.document.setAttribute(node, toString(this.context, name), toString(this.context, value));
      return this.context.undefined;
    });
    addMethod(this.context, this.own, wrapper, "getAttribute", (_this, name) => {
      const value = this.document.getAttribute(node, toString(this.context, name));
      return value === null ? this.context.null : this.context.newString(value);
    });
    addMethod(this.context, this.own, wrapper, "hasAttribute", (_this, name) =>
      this.document.hasAttribute(node, toString(this.context, name))
        ? this.context.true
        : this.context.false,
    );
    addMethod(this.context, this.own, wrapper, "removeAttribute", (_this, name) => {
      this.document.removeAttribute(node, toString(this.context, name));
      return this.context.undefined;
    });
  }

  private installNodeCollections(wrapper: QuickJSHandle, node: VirtualNode): void {
    addMethod(this.context, this.own, wrapper, "querySelector", (_this, selector) => {
      const match = this.document.querySelector(toString(this.context, selector));
      return match ? this.getGuestNodeWrapper(match) : this.context.null;
    });
    addMethod(this.context, this.own, wrapper, "querySelectorAll", (_this, selector) =>
      this.newNodeArray(this.document.querySelectorAll(toString(this.context, selector))),
    );
    addMethod(this.context, this.own, wrapper, "getElementsByTagName", (_this, tagName) =>
      this.newNodeArray(
        this.document
          .getElementsByTagName(toString(this.context, tagName))
          .filter((candidate) => this.isDescendant(node, candidate)),
      ),
    );
  }

  private installNodeEvents(wrapper: QuickJSHandle, node: VirtualNode): void {
    addMethod(this.context, this.own, wrapper, "addEventListener", (_this, type, callback) => {
      const eventType = this.eventType(toString(this.context, type));
      if (!eventType || this.context.typeof(callback) !== "function") return this.context.undefined;
      this.addEventHandler(node, eventType, callback.dup());
      return this.context.undefined;
    });
    addMethod(this.context, this.own, wrapper, "removeEventListener", (_this, type, callback) => {
      const eventType = this.eventType(toString(this.context, type));
      if (!eventType) return this.context.undefined;
      const handlers = node.eventHandlers.get(eventType) ?? [];
      const retained = handlers.filter((handler) => handler.value !== callback.value);
      for (const handler of handlers) {
        if (!retained.includes(handler)) this.releaseHandle(handler);
      }
      node.eventHandlers.set(eventType, retained);
      return this.context.undefined;
    });
    addMethod(this.context, this.own, wrapper, "click", () => {
      this.dispatch(node, { type: "click", nodeId: node.id });
      return this.context.undefined;
    });
    for (const [attribute, eventType] of Object.entries(NODE_EVENT_ATTRIBUTES)) {
      defineSetter(this.context, wrapper, attribute, (value) => {
        const oldHandlers = node.eventHandlers.get(eventType) ?? [];
        for (const handler of oldHandlers) this.releaseHandle(handler);
        const next = this.context.typeof(value) === "function" ? [this.own(value.dup())] : [];
        node.eventHandlers.set(eventType, next);
      });
      defineGetter(this.context, wrapper, attribute, () => this.context.undefined);
    }
  }

  private installDataset(wrapper: QuickJSHandle, node: VirtualNode): void {
    const dataset = this.own(this.context.newObject());
    node.dataset = dataset;
    for (const [name, value] of node.attributes) {
      if (name.startsWith("data-"))
        setProp(this.context, dataset, toDatasetKey(name.slice(5)), value);
    }
    defineGetter(this.context, wrapper, "dataset", () => dataset.dup());
  }

  private installStyle(wrapper: QuickJSHandle, node: VirtualNode): void {
    const style = this.own(this.context.newObject());
    node.style = style;
    for (const [name, value] of this.parseStyle(node.attributes.get("style") ?? "")) {
      setProp(this.context, style, toStyleKey(name), value);
    }
    addMethod(this.context, this.own, style, "setProperty", (_this, name, value) => {
      setProp(
        this.context,
        style,
        toStyleKey(toString(this.context, name)),
        toString(this.context, value),
      );
      return this.context.undefined;
    });
    addMethod(this.context, this.own, style, "getPropertyValue", (_this, name) => {
      const value = this.context.getProp(style, toStyleKey(toString(this.context, name)));
      try {
        return this.context.typeof(value) === "undefined"
          ? this.context.newString("")
          : this.context.newString(toString(this.context, value));
      } finally {
        value.dispose();
      }
    });
    addMethod(this.context, this.own, style, "removeProperty", (_this, name) => {
      setProp(
        this.context,
        style,
        toStyleKey(toString(this.context, name)),
        this.context.undefined,
      );
      return this.context.undefined;
    });
    defineGetter(this.context, wrapper, "style", () => style.dup());
  }

  private installClassList(wrapper: QuickJSHandle, node: VirtualNode): void {
    const classList = this.own(this.context.newObject());
    node.classList = classList;
    addMethod(this.context, this.own, classList, "add", (_this, ...tokens) => {
      const classes = this.classTokens(node);
      for (const token of tokens) classes.add(toString(this.context, token));
      this.writeClassTokens(node, classes);
      return this.context.undefined;
    });
    addMethod(this.context, this.own, classList, "remove", (_this, ...tokens) => {
      const classes = this.classTokens(node);
      for (const token of tokens) classes.delete(toString(this.context, token));
      this.writeClassTokens(node, classes);
      return this.context.undefined;
    });
    addMethod(this.context, this.own, classList, "contains", (_this, token) =>
      this.classTokens(node).has(toString(this.context, token))
        ? this.context.true
        : this.context.false,
    );
    addMethod(this.context, this.own, classList, "toggle", (_this, token, force) => {
      const value = toString(this.context, token);
      const classes = this.classTokens(node);
      const hasForce = force && this.context.typeof(force) !== "undefined";
      const shouldAdd = hasForce ? toBoolean(this.context, force) : !classes.has(value);
      if (shouldAdd) classes.add(value);
      else classes.delete(value);
      this.writeClassTokens(node, classes);
      return shouldAdd ? this.context.true : this.context.false;
    });
    defineGetter(this.context, classList, "value", () =>
      this.context.newString(node.attributes.get("class") ?? ""),
    );
    defineGetter(this.context, wrapper, "classList", () => classList.dup());
  }

  private newNodeArray(nodes: VirtualNode[]): QuickJSHandle {
    const array = this.context.newArray();
    for (const [index, node] of nodes.entries())
      setProp(this.context, array, index, this.getNodeWrapper(node));
    return array;
  }

  private toVirtualNodes(handle: QuickJSHandle): VirtualNode[] {
    const node = this.nodeFromHandle(handle);
    return node ? [node] : [this.document.createTextNode(toString(this.context, handle))];
  }

  private nodeFromHandle(handle: QuickJSHandle): VirtualNode | undefined {
    const id = readHiddenString(this.context, handle, "__previewNodeId");
    return id ? this.document.findNode(id) : undefined;
  }

  private addEventHandler(
    node: VirtualNode,
    type: PreviewEventType,
    callback: QuickJSHandle,
  ): void {
    const handlers = node.eventHandlers.get(type) ?? [];
    handlers.push(this.own(callback));
    node.eventHandlers.set(type, handlers);
    if (handlers.length > this.maxEventQueue) throw new Error(PREVIEW_BUDGET_ERROR);
  }

  private syncDataset(node: VirtualNode, dataset: QuickJSHandle): void {
    for (const [key, value] of readObjectProperties(this.context, dataset)) {
      if (this.context.typeof(value) === "undefined") {
        this.document.removeAttribute(node, `data-${fromDatasetKey(key)}`);
      } else {
        this.document.setAttribute(
          node,
          `data-${fromDatasetKey(key)}`,
          toString(this.context, value),
        );
      }
      value.dispose();
    }
  }

  private syncStyle(node: VirtualNode, style: QuickJSHandle): void {
    const declarations: string[] = [];
    for (const [key, value] of readObjectProperties(this.context, style)) {
      const type = this.context.typeof(value);
      if (type !== "undefined" && type !== "function") {
        declarations.push(`${fromStyleKey(key)}: ${toString(this.context, value)}`);
      }
      value.dispose();
    }
    if (declarations.length) this.document.setAttribute(node, "style", declarations.join("; "));
    else this.document.removeAttribute(node, "style");
  }

  private serializeChildren(node: VirtualNode): string {
    return node.children
      .map((child) => {
        if (child.tagName === "#text") return child.text ?? "";
        const attributes = [...child.attributes.entries()]
          .map(([name, value]) => ` ${name}="${value.replaceAll('"', "&quot;")}"`)
          .join("");
        return `<${child.tagName}${attributes}>${this.serializeChildren(child)}</${child.tagName}>`;
      })
      .join("");
  }

  private parseStyle(value: string): Array<[string, string]> {
    return value
      .split(";")
      .map((declaration) => declaration.split(":"))
      .filter((parts): parts is [string, string] => parts.length >= 2 && Boolean(parts[0].trim()))
      .map(([name, ...rest]) => [toStyleKey(name), rest.join(":").trim()]);
  }

  private classTokens(node: VirtualNode): Set<string> {
    return new Set((node.attributes.get("class") ?? "").split(/\s+/).filter(Boolean));
  }

  private writeClassTokens(node: VirtualNode, classes: Set<string>): void {
    const value = [...classes].join(" ");
    if (value) this.document.setAttribute(node, "class", value);
    else this.document.removeAttribute(node, "class");
  }

  private isDescendant(parent: VirtualNode, candidate: VirtualNode): boolean {
    let current = candidate.parent;
    while (current) {
      if (current === parent) return true;
      current = current.parent;
    }
    return false;
  }

  private eventType(value: string): PreviewEventType | undefined {
    const normalized = value.toLowerCase();
    return ["click", "input", "change", "submit", "keydown"].includes(
      normalized as PreviewEventType,
    )
      ? (normalized as PreviewEventType)
      : undefined;
  }
}
