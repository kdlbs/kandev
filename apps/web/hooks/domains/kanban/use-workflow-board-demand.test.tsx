import { act, render, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useWorkflowBoardDemand } from "./use-workflow-board-demand";

let observe: ReturnType<typeof vi.fn>;
let notify: IntersectionObserverCallback;
let observerOptions: IntersectionObserverInit | undefined;
const ids = Array.from({ length: 16 }, (_, index) => `workflow-${index}`);
const onDemandChange = vi.fn();
function Boards({ workflows = ids, collapsed = [] as string[], focused = false }) {
  const ref = useWorkflowBoardDemand({
    workflowIds: workflows,
    collapsedIds: collapsed,
    focused,
    onDemandChange,
  });
  return (
    <div ref={ref}>
      {workflows.map((id) => (
        <div key={id} data-workflow-id={id} data-testid={id} />
      ))}
    </div>
  );
}
function entry(target: HTMLElement, isIntersecting: boolean): IntersectionObserverEntry {
  return {
    target,
    isIntersecting,
    boundingClientRect: target.getBoundingClientRect(),
    intersectionRect: target.getBoundingClientRect(),
    intersectionRatio: isIntersecting ? 1 : 0,
    rootBounds: null,
    time: 0,
  };
}
function installObserver() {
  observe = vi.fn();
  vi.stubGlobal(
    "IntersectionObserver",
    class {
      constructor(callback: IntersectionObserverCallback, options?: IntersectionObserverInit) {
        notify = callback;
        observerOptions = options;
      }
      observe = observe;
      disconnect = vi.fn();
    },
  );
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  onDemandChange.mockClear();
});

describe("workflow board demand", () => {
  it("starts with one board, then follows visible and adjacent lanes", () => {
    installObserver();
    const result = render(<Boards />);
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[0]]);
    expect(observerOptions?.root).toBe(result.container.firstChild);
    expect(observerOptions?.rootMargin).toBe("0px 0px 300px 0px");
    act(() =>
      notify(
        [
          entry(result.getByTestId(ids[0]), true),
          entry(result.getByTestId(ids[1]), true),
          entry(result.getByTestId(ids[15]), false),
        ],
        {} as IntersectionObserver,
      ),
    );
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[0], ids[1]]);
    act(() =>
      notify(
        [
          entry(result.getByTestId(ids[0]), false),
          entry(result.getByTestId(ids[1]), false),
          entry(result.getByTestId(ids[15]), true),
        ],
        {} as IntersectionObserver,
      ),
    );
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[15]]);
  });
  it("never demands collapsed boards and releases them after collapse", () => {
    installObserver();
    const result = render(<Boards collapsed={[ids[0]]} />);
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[1]]);
    expect(observe).not.toHaveBeenCalledWith(result.getByTestId(ids[0]));
    result.rerender(<Boards collapsed={[ids[0], ids[1]]} />);
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[2]]);
  });
  it("ignores queued observations after a lane is collapsed", () => {
    installObserver();
    const result = render(<Boards />);
    const retiredNotify = notify;
    result.rerender(<Boards collapsed={[ids[0]]} />);
    act(() => retiredNotify([entry(result.getByTestId(ids[0]), true)], {} as IntersectionObserver));
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[1]]);
  });

  it("demands only the focused phone board and follows selection", () => {
    installObserver();
    const result = render(<Boards workflows={[ids[0]]} focused />);
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[0]]);
    expect(observe).not.toHaveBeenCalled();
    result.rerender(<Boards workflows={[ids[15]]} focused />);
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[15]]);
    result.unmount();
    expect(onDemandChange).toHaveBeenLastCalledWith([]);
  });
  it("follows scroll in the bounded fallback when observation is unavailable", () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    const result = render(<Boards />);
    const root = result.container.firstChild as HTMLElement;
    root.getBoundingClientRect = () => ({ top: 0, bottom: 500 }) as DOMRect;
    for (const id of ids)
      result.getByTestId(id).getBoundingClientRect = () =>
        ({ top: 10000, bottom: 10224 }) as DOMRect;
    result.getByTestId(ids[15]).getBoundingClientRect = () =>
      ({ top: 450, bottom: 674 }) as DOMRect;
    fireEvent.scroll(root);
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[15]]);
  });
  it("keeps a bounded fallback when IntersectionObserver is unavailable", () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    render(<Boards />);
    expect(onDemandChange).toHaveBeenLastCalledWith([ids[0]]);
  });
});
