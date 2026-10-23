import { afterEach, expect, it, vi } from "vitest";
import { createMarkdownNativeInput } from "./markdown-native-input";

afterEach(() => vi.unstubAllGlobals());
it("keeps native text and host focus while suspension and disposal detach only its own context", () => {
  vi.stubGlobal(
    "EditContext",
    class extends EventTarget {
      constructor(readonly options: unknown) {
        super();
      }
    },
  );
  const host = document.createElement("div") as HTMLDivElement & { editContext?: unknown };
  const focus = vi.spyOn(host, "focus");
  const input = createMarkdownNativeInput({
    host,
    text: "exact\r\n",
    getCaretRect: () => undefined,
    onSelection: vi.fn(),
  });
  input.attach(host);
  expect(host.editContext).toBe(input.context);
  input.focus();
  expect(focus).toHaveBeenCalledWith({ preventScroll: true });
  input.setSuspended(true);
  expect(host.editContext).toBeNull();
  input.setSuspended(false);
  expect(host.editContext).toBe(input.context);
  input.dispose();
  input.dispose();
  expect(host.editContext).toBeNull();
  input.attach(host);
  input.focus();
  expect(host.editContext).toBeNull();
  expect(focus).toHaveBeenCalledTimes(1);
});

it("updates native control and selection bounds from the current client caret geometry", () => {
  const updateSelectionBounds = vi.fn();
  const updateControlBounds = vi.fn();
  vi.stubGlobal(
    "EditContext",
    class extends EventTarget {
      updateSelectionBounds = updateSelectionBounds;
      updateControlBounds = updateControlBounds;
    },
  );
  const host = document.createElement("div");
  vi.spyOn(host, "getBoundingClientRect").mockReturnValue(new DOMRect(100, 200, 400, 300));
  const input = createMarkdownNativeInput({
    host,
    text: "abc",
    getCaretRect: () => ({ left: 10, top: 20, width: 0, height: 18 }) as never,
    onSelection: vi.fn(),
  });
  input.attach(host);
  input.syncState({
    source: "abc",
    anchor: 0,
    active: 0,
    readOnly: false,
    instanceId: Symbol(),
    sourceRevision: 1,
  });
  expect(updateControlBounds).toHaveBeenCalledWith(
    expect.objectContaining({ x: 100, y: 200, width: 400, height: 300 }),
  );
  expect(updateSelectionBounds).toHaveBeenCalledWith(
    expect.objectContaining({ x: 110, y: 220, height: 18 }),
    expect.objectContaining({ x: 110, y: 220, height: 18 }),
  );
  input.dispose();
});
