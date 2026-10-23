import { getMarkdownClientCaretRect } from "./markdown-input-dom";
import type {
  MarkdownInputFactoryOptions,
  MarkdownInputTransport,
} from "./markdown-input-transport";

type NativeContext = MarkdownInputTransport["context"];
type NativeBoundsContext = NativeContext & {
  updateControlBounds?: (bounds: DOMRect) => void;
  updateSelectionBounds?: (anchor: DOMRect, focus: DOMRect) => void;
};
type NativeHost = HTMLElement & { editContext?: NativeContext | null };
type NativeConstructor = new (options: {
  text: string;
  selectionStart: number;
  selectionEnd: number;
}) => NativeContext;

/** Retains the browser's native EditContext and the engine host's focus behavior. */
export function createMarkdownNativeInput(
  options: MarkdownInputFactoryOptions,
): MarkdownInputTransport {
  const Constructor = (globalThis as unknown as { EditContext: NativeConstructor }).EditContext;
  const context = new Constructor({ text: options.text, selectionStart: 0, selectionEnd: 0 });
  let host: NativeHost | undefined;
  let disposed = false;
  const abort = new AbortController();
  const updateBounds = () => {
    if (disposed || !host) return;
    const boundsContext = context as NativeBoundsContext;
    boundsContext.updateControlBounds?.(host.getBoundingClientRect());
    const caret = getMarkdownClientCaretRect(options);
    if (caret) boundsContext.updateSelectionBounds?.(caret, caret);
  };
  return {
    context,
    attach(element) {
      if (disposed) return;
      host = element;
      host.editContext = context;
      const eventOptions = { signal: abort.signal };
      host.ownerDocument.addEventListener("scroll", updateBounds, {
        ...eventOptions,
        capture: true,
        passive: true,
      });
      host.ownerDocument.defaultView?.addEventListener("resize", updateBounds, eventOptions);
    },
    focus() {
      if (!disposed) host?.focus({ preventScroll: true });
    },
    setSuspended(suspended) {
      if (host && !disposed) host.editContext = suspended ? null : context;
    },
    syncState: updateBounds,
    dispose() {
      if (disposed) return;
      disposed = true;
      abort.abort();
      if (host?.editContext === context) host.editContext = null;
      host = undefined;
    },
  };
}
