import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { EditorModel, EditorView, StringValue } from "@vscode/markdown-editor";
import type {
  MarkdownInputFactoryOptions,
  MarkdownInputState,
  MarkdownInputTransport,
} from "./markdown-input-transport";

class InputContext extends EventTarget {
  text = "";
  updateText(start: number, end: number, text: string) {
    this.text = this.text.slice(0, start) + text + this.text.slice(end);
  }
  updateSelection() {}
}
beforeEach(() => {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
  vi.stubGlobal("EditContext", InputContext);
  vi.stubGlobal(
    "DOMPoint",
    class {
      constructor(
        public x = 0,
        public y = 0,
      ) {}
      matrixTransform() {
        return this;
      }
    },
  );
});
afterEach(() => vi.unstubAllGlobals());

it("AC-UI-MARKDOWN-FILE-EDITING-002.15 injects input and accepts revision-owned selection without editing source", () => {
  const model = new EditorModel();
  model.sourceText.set(new StringValue("hello"), undefined, undefined);
  let options!: MarkdownInputFactoryOptions;
  let snapshot!: MarkdownInputState;
  const textUpdates = vi.fn();
  const context = new InputContext();
  context.addEventListener("textupdate", textUpdates);
  const transport = {
    context,
    attach: vi.fn(),
    focus: vi.fn(),
    setSuspended: vi.fn(),
    syncState: vi.fn((value: MarkdownInputState) => {
      snapshot = value;
    }),
    dispose: vi.fn(),
  } satisfies MarkdownInputTransport;
  const view = new EditorView(model, {
    showReadonlyToggle: false,
    inputFactory: (value) => {
      options = value;
      return transport;
    },
  });
  expect(transport.attach).toHaveBeenCalledWith(view.element);
  const initial = snapshot;
  options.onSelection({ ...snapshot, anchor: 4, active: 1 });
  expect(model.selection.get()).toMatchObject({ anchor: 4, active: 1 });
  expect(model.selectionSource.get()).toBe("user");
  expect(model.sourceText.get().value).toBe("hello");
  expect(textUpdates).not.toHaveBeenCalled();
  const control = document.createElement("button");
  const suspension = view.suspendEditContextWhileFocused(control);
  control.dispatchEvent(new FocusEvent("focusin"));
  expect(transport.setSuspended).toHaveBeenLastCalledWith(true);
  control.dispatchEvent(new FocusEvent("focusout"));
  expect(transport.setSuspended).toHaveBeenLastCalledWith(false);
  suspension.dispose();
  model.replaceSourceText(new StringValue("hello again"));
  expect(snapshot.source).toBe("hello again");
  expect(snapshot.sourceRevision).toBeGreaterThan(initial.sourceRevision);
  const current = model.selection.get();
  options.onSelection({ ...initial, anchor: 0, active: 0 });
  expect(model.selection.get()).toBe(current);
  options.onSelection({ ...snapshot, instanceId: Symbol(), anchor: 0, active: 0 });
  expect(model.selection.get()).toBe(current);
  view.focus();
  expect(transport.focus).toHaveBeenCalledOnce();
  view.dispose();
  view.dispose();
  expect(transport.dispose).toHaveBeenCalledOnce();
  options.onSelection({ ...snapshot, anchor: 0, active: 0 });
  expect(model.selection.get()).toBe(current);
});

it("disposes the injected transport when attachment fails", () => {
  const model = new EditorModel();
  const dispose = vi.fn();
  const transport: MarkdownInputTransport = {
    context: new InputContext(),
    attach() {
      throw new Error("attach failed");
    },
    focus() {},
    setSuspended() {},
    syncState() {},
    dispose,
  };
  expect(
    () => new EditorView(model, { showReadonlyToggle: false, inputFactory: () => transport }),
  ).toThrow("attach failed");
  expect(dispose).toHaveBeenCalledOnce();
});

it("retains native selection synchronization frequency when readonly changes", () => {
  const model = new EditorModel();
  const context = new InputContext();
  const selectionUpdate = vi.spyOn(context, "updateSelection");
  const transport: MarkdownInputTransport = {
    context,
    attach() {},
    focus() {},
    setSuspended() {},
    syncState() {},
    dispose() {},
  };
  const view = new EditorView(model, { showReadonlyToggle: false, inputFactory: () => transport });
  selectionUpdate.mockClear();
  model.readonlyMode.set(true, undefined, undefined);
  // The render pass synchronizes once. Transport state publication must add no extra update.
  expect(selectionUpdate).toHaveBeenCalledTimes(1);
  view.dispose();
});
