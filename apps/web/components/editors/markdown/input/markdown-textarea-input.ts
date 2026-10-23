import { getMarkdownKeyboardPlatform } from "./markdown-keyboard-platform";
import { hideMarkdownDecoration, positionMarkdownInput } from "./markdown-input-dom";
import {
  MarkdownInputContext,
  nativeMarkdownReplacement,
  projectMarkdownInput,
  type NativeReplacementRange,
} from "./markdown-input-context";
import type {
  MarkdownInputFactoryOptions,
  MarkdownInputState,
  MarkdownInputTransport,
} from "./markdown-input-transport";

export function createMarkdownTextareaInput(
  options: MarkdownInputFactoryOptions,
  label: string,
): MarkdownTextareaInput {
  return new MarkdownTextareaInput(options, label);
}

class MarkdownTextareaInput implements MarkdownInputTransport {
  private readonly nativeContext: MarkdownInputContext;
  readonly context: MarkdownInputTransport["context"];
  private readonly textarea = document.createElement("textarea");
  private state?: MarkdownInputState;
  private disposed = false;
  private suspended = false;
  private composing = false;
  private applyingInput = false;
  private syncing = false;
  private selectionEcho = "";
  private replacementHint?: NativeReplacementRange;
  private readonly pendingSettlements = new Map<symbol, () => void>();
  private readonly defaultSettlementOwner = Symbol();
  private pendingSuspension = false;
  private readonly abort = new AbortController();
  private decorationObserver?: MutationObserver;

  constructor(
    private readonly options: MarkdownInputFactoryOptions,
    label: string,
  ) {
    this.nativeContext = new MarkdownInputContext(options.text);
    this.context = this.nativeContext as unknown as MarkdownInputTransport["context"];
    this.textarea.setAttribute("aria-label", label);
    this.textarea.setAttribute("autocapitalize", "off");
    this.textarea.spellcheck = false;
    this.textarea.wrap = "off";
    this.textarea.style.cssText =
      "position:absolute;width:2px;min-width:2px;height:20px;padding:0;border:0;opacity:0.01;resize:none;overflow:hidden;pointer-events:none;z-index:-1;font-size:16px;";
    const eventOptions = { signal: this.abort.signal };
    this.textarea.addEventListener("input", this.onInput, eventOptions);
    this.textarea.addEventListener("select", this.onSelection, eventOptions);
    this.textarea.addEventListener("selectionchange", this.onSelection, eventOptions);
    this.textarea.addEventListener(
      "compositionstart",
      () => {
        this.composing = true;
      },
      eventOptions,
    );
    this.textarea.addEventListener("compositionend", this.onCompositionEnd, eventOptions);
    this.textarea.addEventListener(
      "blur",
      () => {
        if (!this.composing) this.renderState();
      },
      eventOptions,
    );
    this.textarea.addEventListener("beforeinput", this.onBeforeInput, eventOptions);
  }

  attach(host: HTMLElement): void {
    if (this.disposed) return;
    const shadow = host.attachShadow({ mode: "open" });
    shadow.append(document.createElement("slot"), this.textarea);
    const eventOptions = { signal: this.abort.signal };
    host.addEventListener("focus", this.onHostFocus, eventOptions);
    host.ownerDocument.addEventListener("scroll", this.updateGeometry, {
      ...eventOptions,
      capture: true,
      passive: true,
    });
    host.ownerDocument.defaultView?.addEventListener("resize", this.updateGeometry, eventOptions);
    this.decorationObserver = new MutationObserver(() => hideMarkdownDecoration(host));
    this.decorationObserver.observe(host, { childList: true, subtree: true });
    hideMarkdownDecoration(host);
  }

  private readonly updateGeometry = (): void => {
    if (!this.disposed) positionMarkdownInput(this.options, this.textarea);
  };

  private readonly onHostFocus = (): void => {
    this.focus();
  };
  focus(): void {
    if (this.disposed || this.suspended) return;
    this.renderState();
    this.textarea.focus({ preventScroll: true });
  }

  setSuspended(suspended: boolean): void {
    if (this.composing) {
      this.pendingSuspension = suspended;
      if (suspended) this.textarea.blur();
      return;
    }
    this.suspended = suspended;
    if (suspended) this.textarea.blur();
  }

  runWhenSettled(callback: () => void, key = this.defaultSettlementOwner): void {
    if (this.disposed) return;
    if (this.composing) this.pendingSettlements.set(key, callback);
    else callback();
  }

  private readonly onCompositionEnd = (): void => {
    this.composing = false;
    this.onInput();
    const callbacks = [...this.pendingSettlements.values()];
    this.pendingSettlements.clear();
    for (const callback of callbacks) callback();
    this.suspended = this.pendingSuspension;
    this.pendingSuspension = false;
    this.renderState();
  };

  syncState(state: MarkdownInputState): void {
    if (this.disposed) return;
    this.state = state;
    this.nativeContext.text = state.source;
    if (!this.composing && !this.applyingInput) this.renderState();
    else this.updateGeometry();
  }

  private renderState(): void {
    const state = this.state;
    if (!state || this.disposed || this.composing) return;
    this.syncing = true;
    const projection = projectMarkdownInput(state.source);
    if (this.textarea.value !== projection.value) this.textarea.value = projection.value;
    const anchor = projection.toNative(state.anchor);
    const active = projection.toNative(state.active);
    this.textarea.setSelectionRange(
      Math.min(anchor, active),
      Math.max(anchor, active),
      anchor > active ? "backward" : "forward",
    );
    this.selectionEcho = this.selectionKey();
    this.textarea.readOnly = state.readOnly;
    this.syncing = false;
    this.updateGeometry();
  }

  private selectionKey(): string {
    return `${this.textarea.selectionStart}:${this.textarea.selectionEnd}:${this.textarea.selectionDirection}`;
  }

  private readonly onSelection = (): void => {
    if (
      !this.state ||
      this.disposed ||
      this.suspended ||
      this.syncing ||
      this.applyingInput ||
      this.composing ||
      this.selectionKey() === this.selectionEcho
    )
      return;
    const projection = projectMarkdownInput(this.nativeContext.text);
    if (projection.value !== this.textarea.value) return;
    const start = projection.toSource(this.textarea.selectionStart);
    const end = projection.toSource(this.textarea.selectionEnd);
    const backward = this.textarea.selectionDirection === "backward";
    this.selectionEcho = this.selectionKey();
    this.options.onSelection({
      anchor: backward ? end : start,
      active: backward ? start : end,
      instanceId: this.state.instanceId,
      sourceRevision: this.state.sourceRevision,
    });
  };

  private readonly onInput = (): void => {
    if (!this.state || this.disposed || this.suspended || this.state.readOnly) {
      this.renderState();
      return;
    }
    const edit = nativeMarkdownReplacement(
      this.nativeContext.text,
      this.textarea.value,
      this.replacementHint,
    );
    this.replacementHint = undefined;
    if (edit.start === edit.end && !edit.text) return;
    this.applyingInput = true;
    this.nativeContext.updateText(edit.start, edit.end, edit.text);
    const event = Object.assign(new Event("textupdate"), {
      updateRangeStart: edit.start,
      updateRangeEnd: edit.end,
      text: edit.text,
    });
    this.nativeContext.dispatchEvent(event);
    this.applyingInput = false;
    this.selectionEcho = "";
    this.onSelectionAfterInput();
  };

  private onSelectionAfterInput(): void {
    if (!this.state) return;
    const projection = projectMarkdownInput(this.nativeContext.text);
    const start = projection.toSource(this.textarea.selectionStart);
    const end = projection.toSource(this.textarea.selectionEnd);
    const backward = this.textarea.selectionDirection === "backward";
    this.options.onSelection({
      anchor: backward ? end : start,
      active: backward ? start : end,
      instanceId: this.state.instanceId,
      sourceRevision: this.state.sourceRevision,
    });
  }

  private readonly onBeforeInput = (event: InputEvent): void => {
    if (this.suspended || this.state?.readOnly) {
      event.preventDefault();
      return;
    }
    this.replacementHint = { start: this.textarea.selectionStart, end: this.textarea.selectionEnd };
    if (this.replacementHint.start === this.replacementHint.end) {
      if (event.inputType === "deleteContentBackward")
        this.replacementHint.start = Math.max(0, this.replacementHint.start - 1);
      if (event.inputType === "deleteContentForward")
        this.replacementHint.end = Math.min(
          this.textarea.value.length,
          this.replacementHint.end + 1,
        );
    }
    if (event.inputType !== "historyUndo" && event.inputType !== "historyRedo") return;
    const mac = getMarkdownKeyboardPlatform() === "macos";
    this.options.host.dispatchEvent(
      new KeyboardEvent("keydown", {
        key: "z",
        ctrlKey: !mac,
        metaKey: mac,
        shiftKey: event.inputType === "historyRedo",
        bubbles: true,
        cancelable: true,
      }),
    );
    event.preventDefault();
  };

  dispose(): void {
    if (this.disposed) return;
    this.disposed = true;
    this.pendingSettlements.clear();
    this.abort.abort();
    this.decorationObserver?.disconnect();
    this.textarea.remove();
  }
}
