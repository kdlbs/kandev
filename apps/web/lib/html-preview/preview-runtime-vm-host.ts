import type { QuickJSContext, QuickJSHandle, QuickJSRuntime } from "quickjs-emscripten";
import { PreviewVirtualDocument, type VirtualNode } from "./preview-runtime-document";
import { PreviewRuntimeNodeApi } from "./preview-runtime-vm-node";
import {
  addEphemeralMethod,
  addMethod,
  defineGetter,
  defineValue,
  readStringArray,
  readTypeOption,
  setProp,
  toNumber,
  toString,
  type OwnHandle,
} from "./preview-runtime-vm-helpers";
import {
  PREVIEW_RUNTIME_PROTOCOL_VERSION,
  PreviewRuntimeError,
  type PreviewEvent,
  type PreviewEventType,
  type PreviewResource,
  type PreviewRuntimeOptions,
  type PreviewSnapshot,
} from "./preview-runtime-types";

type TimerRecord = {
  callback: QuickJSHandle;
  interval: boolean;
};

type BlobRecord = {
  content: string;
  mediaType: string;
};

const EVENT_TYPES: PreviewEventType[] = ["click", "input", "change", "submit", "keydown"];
// i18n-exempt: internal runtime failure marker, never rendered
const PREVIEW_BUDGET_ERROR = "preview budget exceeded";
// i18n-exempt: browser capability identities, never rendered
const DISABLED_GLOBALS = [
  "fetch",
  "XMLHttpRequest",
  "WebSocket",
  "EventSource",
  "Worker",
  "SharedWorker",
  "ServiceWorker",
  "importScripts",
  "require",
  "process",
  "module",
  "WebAssembly",
  "navigator",
  "eval",
  "Function",
];

export type PreviewRuntimeVmHostOptions = {
  runtime: QuickJSRuntime;
  context: QuickJSContext;
  document: PreviewVirtualDocument;
  options: PreviewRuntimeOptions;
  own: OwnHandle;
  releaseHandle: (handle: QuickJSHandle) => void;
  isBudgetExceeded: () => boolean;
};

export class PreviewRuntimeVmHost {
  private readonly runtime: QuickJSRuntime;
  private readonly context: QuickJSContext;
  private readonly document: PreviewVirtualDocument;
  private readonly options: PreviewRuntimeOptions;
  private readonly own: OwnHandle;
  private readonly releaseHandle: (handle: QuickJSHandle) => void;
  private readonly isBudgetExceeded: () => boolean;
  private readonly blobs = new Map<string, BlobRecord>();
  private readonly timers = new Map<number, TimerRecord>();
  private readonly documentReadyHandlers: QuickJSHandle[] = [];
  private readonly nodeApi: PreviewRuntimeNodeApi;
  private nextTimerId = 1;
  private nextBlobId = 1;
  private eventsProcessed = 0;
  private startedAt = 0;
  private documentHandle: QuickJSHandle | undefined;
  private windowHandle: QuickJSHandle | undefined;

  constructor(options: PreviewRuntimeVmHostOptions) {
    this.runtime = options.runtime;
    this.context = options.context;
    this.document = options.document;
    this.options = options.options;
    this.own = options.own;
    this.releaseHandle = options.releaseHandle;
    this.isBudgetExceeded = options.isBudgetExceeded;
    this.nodeApi = new PreviewRuntimeNodeApi({
      context: this.context,
      document: this.document,
      getDocumentHandle: () => this.documentHandle,
      own: this.own,
      releaseHandle: this.releaseHandle,
      dispatch: (node, event) => this.dispatchToNode(node, event),
      maxEventQueue: this.options.maxEventQueue,
    });
  }

  start(startedAt: number): void {
    this.beginOperation(startedAt);
    this.installGlobals();
    this.nodeApi.installExistingNodeWrappers();
    this.compileInlineHandlers();
  }

  beginOperation(startedAt: number): void {
    this.startedAt = startedAt;
    this.eventsProcessed = 0;
  }

  runScripts(): void {
    for (const [index, script] of this.document.scripts.entries()) {
      this.evaluateScript(script, `preview-inline-${index + 1}.js`);
      this.flushJobsAndTimers();
    }
    this.fireDocumentReady();
    this.flushJobsAndTimers();
  }

  dispatch(event: PreviewEvent): PreviewSnapshot {
    this.ensureBudget();
    const node = this.document.findNode(event.nodeId);
    if (!node || !EVENT_TYPES.includes(event.type))
      throw new PreviewRuntimeError("malformed-message");
    if (event.value !== undefined) this.document.setAttribute(node, "value", event.value);
    if (event.checked !== undefined) {
      if (event.checked) this.document.setAttribute(node, "checked", "");
      else this.document.removeAttribute(node, "checked");
    }
    this.dispatchToNode(node, event);
    this.flushJobsAndTimers();
    return this.createSnapshot();
  }

  snapshot(): PreviewSnapshot {
    return this.createSnapshot();
  }

  dispose(): void {
    for (const timer of this.timers.values()) this.releaseHandle(timer.callback);
    this.timers.clear();
    for (const handler of this.documentReadyHandlers) this.releaseHandle(handler);
    this.documentReadyHandlers.length = 0;
    this.blobs.clear();
    this.documentHandle = undefined;
    this.windowHandle = undefined;
  }

  private installGlobals(): void {
    const documentHandle = this.own(this.context.newObject());
    const windowHandle = this.own(this.context.newObject());
    this.documentHandle = documentHandle;
    this.windowHandle = windowHandle;
    this.installDocumentApi(documentHandle, windowHandle);
    this.installWindowApi(windowHandle, documentHandle);
    this.installTimers(this.context.global);
    this.installConsole(this.context.global);
    this.installBlobApi(this.context.global, windowHandle);
    this.installNoopObject(this.context.global, "location", ["assign", "replace", "reload"]);
    this.installNoopObject(this.context.global, "history", [
      "back",
      "forward",
      "go",
      "pushState",
      "replaceState",
    ]);
    setProp(this.context, this.context.global, "document", documentHandle);
    setProp(this.context, this.context.global, "window", windowHandle);
    setProp(this.context, this.context.global, "self", windowHandle);
    setProp(this.context, this.context.global, "parent", windowHandle);
    setProp(this.context, this.context.global, "top", windowHandle);
    setProp(this.context, windowHandle, "document", documentHandle);
    setProp(this.context, windowHandle, "self", windowHandle);
    setProp(this.context, windowHandle, "parent", windowHandle);
    setProp(this.context, windowHandle, "top", windowHandle);
    for (const name of DISABLED_GLOBALS) {
      setProp(this.context, this.context.global, name, this.context.undefined);
      setProp(this.context, windowHandle, name, this.context.undefined);
    }
  }

  private installDocumentApi(documentHandle: QuickJSHandle, windowHandle: QuickJSHandle): void {
    addMethod(this.context, this.own, documentHandle, "createElement", (_this, tagName) => {
      return this.nodeApi.getGuestNodeWrapper(
        this.document.createElement(toString(this.context, tagName)),
      );
    });
    addMethod(this.context, this.own, documentHandle, "createTextNode", (_this, value) => {
      return this.nodeApi.getGuestNodeWrapper(
        this.document.createTextNode(toString(this.context, value)),
      );
    });
    addMethod(this.context, this.own, documentHandle, "getElementById", (_this, id) => {
      const node = this.document.getElementById(toString(this.context, id));
      return node ? this.nodeApi.getGuestNodeWrapper(node) : this.context.null;
    });
    addMethod(this.context, this.own, documentHandle, "querySelector", (_this, selector) => {
      const node = this.document.querySelector(toString(this.context, selector));
      return node ? this.nodeApi.getGuestNodeWrapper(node) : this.context.null;
    });
    addMethod(this.context, this.own, documentHandle, "querySelectorAll", (_this, selector) =>
      this.newNodeArray(this.document.querySelectorAll(toString(this.context, selector))),
    );
    addMethod(this.context, this.own, documentHandle, "getElementsByTagName", (_this, tagName) =>
      this.newNodeArray(this.document.getElementsByTagName(toString(this.context, tagName))),
    );
    addMethod(
      this.context,
      this.own,
      documentHandle,
      "addEventListener",
      (_this, type, callback) => {
        if (toString(this.context, type).toLowerCase() !== "domcontentloaded")
          return this.context.undefined;
        if (this.context.typeof(callback) !== "function") return this.context.undefined;
        this.addDocumentReadyHandler(callback);
        return this.context.undefined;
      },
    );
    defineGetter(this.context, documentHandle, "body", () =>
      this.nodeApi.getGuestNodeWrapper(this.document.body),
    );
    defineGetter(this.context, documentHandle, "head", () =>
      this.nodeApi.getGuestNodeWrapper(this.document.head),
    );
    defineGetter(this.context, documentHandle, "documentElement", () =>
      this.nodeApi.getGuestNodeWrapper(this.document.html),
    );
    defineGetter(this.context, documentHandle, "defaultView", () => windowHandle.dup());
    defineValue(this.context, documentHandle, "readyState", "complete");
  }

  private installWindowApi(windowHandle: QuickJSHandle, documentHandle: QuickJSHandle): void {
    addMethod(this.context, this.own, windowHandle, "open", () => {
      throw new Error("unsupported-capability");
    });
    addMethod(this.context, this.own, windowHandle, "addEventListener", (_this, type, callback) => {
      if (toString(this.context, type).toLowerCase() !== "load") return this.context.undefined;
      if (this.context.typeof(callback) === "function") {
        this.addDocumentReadyHandler(callback);
      }
      return this.context.undefined;
    });
    this.installNoopObject(windowHandle, "location", ["assign", "replace", "reload"]);
    this.installNoopObject(windowHandle, "history", [
      "back",
      "forward",
      "go",
      "pushState",
      "replaceState",
    ]);
    setProp(this.context, windowHandle, "document", documentHandle);
  }

  private installTimers(global: QuickJSHandle): void {
    addMethod(this.context, this.own, global, "setTimeout", (_this, callback) => {
      if (this.context.typeof(callback) !== "function") throw new Error("runtime-error");
      if (this.timers.size >= this.options.maxTimers) throw new Error(PREVIEW_BUDGET_ERROR);
      const id = this.nextTimerId++;
      this.timers.set(id, { callback: this.own(callback.dup()), interval: false });
      return this.context.newNumber(id);
    });
    addMethod(this.context, this.own, global, "setInterval", (_this, callback) => {
      if (this.context.typeof(callback) !== "function") throw new Error("runtime-error");
      if (this.timers.size >= this.options.maxTimers) throw new Error(PREVIEW_BUDGET_ERROR);
      const id = this.nextTimerId++;
      this.timers.set(id, { callback: this.own(callback.dup()), interval: true });
      return this.context.newNumber(id);
    });
    addMethod(this.context, this.own, global, "clearTimeout", (_this, id) => {
      this.clearTimer(toNumber(this.context, id));
      return this.context.undefined;
    });
    addMethod(this.context, this.own, global, "clearInterval", (_this, id) => {
      this.clearTimer(toNumber(this.context, id));
      return this.context.undefined;
    });
  }

  private installConsole(global: QuickJSHandle): void {
    const consoleHandle = this.own(this.context.newObject());
    for (const method of ["debug", "info", "log", "warn", "error"]) {
      addMethod(this.context, this.own, consoleHandle, method, () => this.context.undefined);
    }
    setProp(this.context, global, "console", consoleHandle);
  }

  private installBlobApi(global: QuickJSHandle, windowHandle: QuickJSHandle): void {
    const blobConstructor = this.own(
      this.context.newConstructorFunction("Blob", (parts, options) => {
        const token = `blob:preview-runtime-${this.nextBlobId++}`;
        const content = readStringArray(this.context, parts);
        const mediaType =
          options && this.context.typeof(options) === "object"
            ? readTypeOption(this.context, options)
            : "text/plain";
        this.blobs.set(token, { content, mediaType });
        const blob = this.context.newObject();
        setProp(this.context, blob, "__previewBlobToken", token);
        return blob;
      }),
    );
    setProp(this.context, global, "Blob", blobConstructor);
    const urlHandle = this.own(this.context.newObject());
    addMethod(this.context, this.own, urlHandle, "createObjectURL", (_this, blob) => {
      const token = this.readHiddenString(blob, "__previewBlobToken");
      if (!token || !this.blobs.has(token)) throw new Error("unsupported-capability");
      return this.context.newString(token);
    });
    addMethod(this.context, this.own, urlHandle, "revokeObjectURL", (_this, url) => {
      const token = toString(this.context, url);
      if (this.blobs.has(token)) this.blobs.delete(token);
      return this.context.undefined;
    });
    setProp(this.context, global, "URL", urlHandle);
    setProp(this.context, windowHandle, "URL", urlHandle);
  }

  private installNoopObject(target: QuickJSHandle, name: string, methods: string[]): void {
    const object = this.own(this.context.newObject());
    for (const method of methods)
      addMethod(this.context, this.own, object, method, () => this.context.undefined);
    setProp(this.context, target, name, object);
  }

  private compileInlineHandlers(): void {
    for (const handler of this.document.inlineHandlers) {
      this.ensureBudget();
      const result = this.context.evalCode(
        `(function(event){\n${handler.source}\n})`,
        `preview-handler-${handler.node.id}.js`,
      );
      const functionHandle = this.context.unwrapResult(result);
      this.addEventHandler(handler.node, handler.type, functionHandle);
    }
  }

  private evaluateScript(source: string, filename: string): void {
    this.ensureBudget();
    const result = this.context.evalCode(source, filename);
    const value = this.context.unwrapResult(result);
    value.dispose();
    this.ensureBudget();
  }

  private dispatchToNode(node: VirtualNode, event: PreviewEvent): void {
    const eventHandle = this.createEventHandle(event, node);
    try {
      let current: VirtualNode | null = node;
      while (current) {
        this.ensureEventBudget();
        setProp(this.context, eventHandle, "currentTarget", this.nodeApi.getNodeWrapper(current));
        const handlers = [...(current.eventHandlers.get(event.type) ?? [])];
        for (const handler of handlers) {
          this.ensureEventBudget();
          const result = this.context.callFunction(
            handler,
            this.nodeApi.getNodeWrapper(current),
            eventHandle,
          );
          const value = this.context.unwrapResult(result);
          value.dispose();
        }
        current = current.parent;
      }
    } finally {
      eventHandle.dispose();
    }
  }

  private createEventHandle(event: PreviewEvent, target: VirtualNode): QuickJSHandle {
    const eventHandle = this.context.newObject();
    setProp(this.context, eventHandle, "type", event.type);
    setProp(this.context, eventHandle, "target", this.nodeApi.getNodeWrapper(target));
    setProp(this.context, eventHandle, "nodeId", event.nodeId);
    if (event.value !== undefined) setProp(this.context, eventHandle, "value", event.value);
    if (event.checked !== undefined) setProp(this.context, eventHandle, "checked", event.checked);
    if (event.key !== undefined) setProp(this.context, eventHandle, "key", event.key);
    addEphemeralMethod(this.context, eventHandle, "preventDefault", () => this.context.undefined);
    addEphemeralMethod(this.context, eventHandle, "stopPropagation", () => this.context.undefined);
    return eventHandle;
  }

  private fireDocumentReady(): void {
    const handlers = this.documentReadyHandlers.splice(0);
    for (const handler of handlers) {
      try {
        const result = this.context.callFunction(handler, this.windowHandle ?? this.context.global);
        const value = this.context.unwrapResult(result);
        value.dispose();
      } finally {
        this.releaseHandle(handler);
      }
    }
  }

  private flushJobsAndTimers(): void {
    this.ensureBudget();
    while (this.runtime.hasPendingJob()) {
      this.ensureEventBudget();
      const result = this.runtime.executePendingJobs(
        Math.max(1, this.options.maxEventQueue - this.eventsProcessed),
      );
      if (result.error) {
        result.error.dispose();
        throw new Error("runtime-error");
      }
      this.eventsProcessed += result.value;
    }
    while (this.timers.size) {
      this.ensureEventBudget();
      const [id, timer] = this.timers.entries().next().value as [number, TimerRecord];
      this.timers.delete(id);
      const result = this.context.callFunction(timer.callback, this.context.undefined);
      const value = this.context.unwrapResult(result);
      value.dispose();
      this.releaseHandle(timer.callback);
      if (timer.interval) this.eventsProcessed += 1;
    }
  }

  private createSnapshot(): PreviewSnapshot {
    this.nodeApi.syncGuestObjects();
    const snapshot: PreviewSnapshot = {
      protocolVersion: PREVIEW_RUNTIME_PROTOCOL_VERSION,
      root: this.document.snapshot(new Set(this.blobs.keys())),
      resources: [...this.blobs.entries()].map(
        ([token, record]) =>
          ({
            token,
            content: record.content,
            mediaType: record.mediaType,
          }) satisfies PreviewResource,
      ),
      diagnostics: [],
    };
    if (JSON.stringify(snapshot).length > this.options.maxSnapshotBytes) {
      throw new Error(PREVIEW_BUDGET_ERROR);
    }
    return snapshot;
  }

  private ensureBudget(): void {
    if (this.isBudgetExceeded() || Date.now() - this.startedAt > this.options.wallClockBudgetMs) {
      throw new PreviewRuntimeError("budget-exceeded");
    }
  }

  private ensureEventBudget(): void {
    this.eventsProcessed += 1;
    if (this.eventsProcessed > this.options.maxEventQueue) throw new Error(PREVIEW_BUDGET_ERROR);
    this.ensureBudget();
  }

  private addEventHandler(
    node: VirtualNode,
    type: PreviewEventType,
    callback: QuickJSHandle,
  ): void {
    const handlers = node.eventHandlers.get(type) ?? [];
    if (handlers.length >= this.options.maxEventQueue) {
      this.releaseHandle(callback);
      throw new Error(PREVIEW_BUDGET_ERROR);
    }
    handlers.push(this.own(callback));
    node.eventHandlers.set(type, handlers);
  }

  private addDocumentReadyHandler(callback: QuickJSHandle): void {
    if (this.documentReadyHandlers.length >= this.options.maxEventQueue)
      throw new Error(PREVIEW_BUDGET_ERROR);
    this.documentReadyHandlers.push(this.own(callback.dup()));
  }

  private clearTimer(id: number): void {
    const timer = this.timers.get(id);
    if (!timer) return;
    this.timers.delete(id);
    this.releaseHandle(timer.callback);
  }

  private readHiddenString(value: QuickJSHandle, key: string): string | undefined {
    const property = this.context.getProp(value, key);
    try {
      return this.context.typeof(property) === "string"
        ? toString(this.context, property)
        : undefined;
    } finally {
      property.dispose();
    }
  }

  private newNodeArray(nodes: VirtualNode[]): QuickJSHandle {
    const array = this.context.newArray();
    for (const [index, node] of nodes.entries()) {
      setProp(this.context, array, index, this.nodeApi.getNodeWrapper(node));
    }
    return array;
  }
}
