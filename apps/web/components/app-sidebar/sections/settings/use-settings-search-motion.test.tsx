import { StrictMode } from "react";
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useSettingsSearchMotion } from "./use-settings-search-motion";

type Handle = {
  cancel: () => void;
  effect: unknown;
  finish: () => void;
  listenerCount: () => number;
};

const originalAnimate = Object.getOwnPropertyDescriptor(Element.prototype, "animate");

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  if (originalAnimate) Object.defineProperty(Element.prototype, "animate", originalAnimate);
  else Reflect.deleteProperty(Element.prototype, "animate");
});

function Results({ keys }: { keys: string[] }) {
  const ref = useSettingsSearchMotion(keys.join(","));
  return (
    <div ref={ref}>
      {keys.map((key) => (
        <div key={key} data-settings-search-motion-key={key}>
          {key}
        </div>
      ))}
    </div>
  );
}

function installAnimations() {
  const handles: Handle[] = [];
  const animate = vi.fn(() => {
    const listeners = new Set<() => void>();
    let onfinish: (() => void) | null = null;
    let queuedFinish: (() => void) | undefined;
    const handle = {
      cancel: vi.fn(),
      effect: {} as unknown,
      get onfinish() {
        return onfinish;
      },
      set onfinish(listener: (() => void) | null) {
        onfinish = listener;
        if (listener) queuedFinish = listener;
      },
      addEventListener: (_type: string, fn: () => void) => {
        listeners.add(fn);
        queuedFinish = fn;
      },
      finish: () => {
        listeners.clear();
        queuedFinish?.();
      },
      listenerCount: () => listeners.size + (onfinish ? 1 : 0),
    };
    handles.push(handle);
    return handle;
  });
  Object.defineProperty(Element.prototype, "animate", { value: animate, configurable: true });
  return { handles, animate };
}

it("releases a finished entrance before its row is removed", () => {
  const { handles } = installAnimations();
  const view = render(<Results keys={["a"]} />);
  view.rerender(<Results keys={["a", "b"]} />);
  const entrance = handles.at(-1)!;
  expect(entrance.effect).not.toBeNull();
  entrance.finish();
  expect(entrance.effect).toBeNull();
  expect(entrance.cancel).toHaveBeenCalledOnce();
  expect(entrance.listenerCount()).toBe(0);
  view.rerender(<Results keys={["a"]} />);
  view.unmount();
  expect(entrance.cancel).toHaveBeenCalledOnce();
});

it.each(["before", "after"])(
  "keeps a replacement owned when the old finish arrives %s replacement",
  (order) => {
    const { handles } = installAnimations();
    let top = 0;
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
      () => ({ top }) as DOMRect,
    );
    const view = render(<Results keys={["a"]} />);
    const previous = handles[0];
    if (order === "before") previous.finish();
    top = 20;
    view.rerender(<Results keys={["a", "b"]} />);
    const replacement = handles[1];
    expect(previous.effect).toBeNull();
    expect(previous.listenerCount()).toBe(0);
    if (order === "after") previous.finish();
    expect(replacement.effect).not.toBeNull();
    expect(replacement.cancel).not.toHaveBeenCalled();
    replacement.finish();
    expect(replacement.effect).toBeNull();
    expect(replacement.cancel).toHaveBeenCalledOnce();
    view.unmount();
    expect(handles.every((handle) => handle.effect === null)).toBe(true);
    expect(replacement.cancel).toHaveBeenCalledOnce();
  },
);

it("releases active rows on removal and unmount despite late finish events", () => {
  const { handles } = installAnimations();
  const view = render(<Results keys={["a", "b"]} />);
  view.rerender(<Results keys={["b"]} />);
  expect(handles.every((handle) => handle.effect === null)).toBe(true);
  view.rerender(<Results keys={["b", "c"]} />);
  const active = handles.at(-1)!;
  expect(active.effect).not.toBeNull();
  view.unmount();
  for (const handle of handles) {
    handle.finish();
    expect(handle.effect).toBeNull();
    expect(handle.listenerCount()).toBe(0);
    expect(handle.cancel).toHaveBeenCalledOnce();
  }
});

it("releases the StrictMode startup effect without replay on unmount", () => {
  const { handles, animate } = installAnimations();
  const view = render(
    <StrictMode>
      <Results keys={["a"]} />
    </StrictMode>,
  );
  expect(animate).toHaveBeenCalledOnce();
  expect(handles[0].effect).toBeNull();
  expect(handles[0].listenerCount()).toBe(0);
  view.unmount();
  expect(handles[0].cancel).toHaveBeenCalledOnce();
});

it("keeps a replacement reachable for unmount after a stale finish", () => {
  const { handles } = installAnimations();
  let top = 0;
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    () => ({ top }) as DOMRect,
  );
  const view = render(<Results keys={["a"]} />);
  const previous = handles[0];
  top = 20;
  view.rerender(<Results keys={["a", "b"]} />);
  const replacement = handles[1];
  previous.finish();
  expect(replacement.effect).not.toBeNull();
  view.unmount();
  expect(replacement.effect).toBeNull();
  expect(replacement.cancel).toHaveBeenCalledOnce();
});
