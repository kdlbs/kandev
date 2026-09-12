import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useColumnNaturalHeight } from "./use-column-natural-height";

afterEach(() => vi.unstubAllGlobals());

it("reports logical rows plus chrome, remeasures chrome and disconnects outside compact mode", () => {
  const disconnect = vi.fn();
  let resize = () => {};
  const observer = vi.fn(
    class {
      constructor(callback: () => void) {
        resize = callback;
      }
      observe = vi.fn();
      disconnect = disconnect;
    },
  );
  vi.stubGlobal("ResizeObserver", observer);
  const report = vi.fn();
  const { result, rerender, unmount } = renderHook(
    ({ enabled }) => useColumnNaturalHeight("step", enabled ? report : undefined),
    { initialProps: { enabled: false } },
  );
  expect(observer).not.toHaveBeenCalled();
  expect(result.current.onContentHeightChange).toBeUndefined();
  const column = document.createElement("div");
  column.style.padding = "8px";
  column.style.height = "400px";
  const header = document.createElement("div");
  header.style.marginBottom = "12px";
  let headerHeight = 28;
  vi.spyOn(header, "getBoundingClientRect").mockImplementation(
    () => ({ height: headerHeight }) as DOMRect,
  );
  const scroll = document.createElement("div");
  scroll.style.paddingTop = "4px";
  scroll.style.paddingBottom = "0px";
  column.append(header, scroll);
  document.body.append(column);
  result.current.columnRef.current = column;
  result.current.headerRef.current = header;
  rerender({ enabled: true });
  act(() => result.current.onContentHeightChange!(190, scroll));
  expect(report).toHaveBeenLastCalledWith("step", 250);
  headerHeight = 40;
  act(() => resize());
  expect(report).toHaveBeenLastCalledWith("step", 262);
  act(() => result.current.onContentHeightChange!(90, scroll));
  expect(report).toHaveBeenLastCalledWith("step", 162);
  rerender({ enabled: false });
  expect(disconnect).toHaveBeenCalledTimes(1);
  rerender({ enabled: true });
  unmount();
  expect(disconnect).toHaveBeenCalledTimes(2);
  column.remove();
});
