import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useCallback } from "react";
import type { ResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const responsiveMocks = vi.hoisted(() => ({
  useResponsiveBreakpoint: vi.fn(),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => responsiveMocks);

import { useThreadColumnActivation } from "./use-thread-column-activation";

type ObserverRecord = {
  callback: IntersectionObserverCallback;
  instance: MockIntersectionObserver;
};

const observers: ObserverRecord[] = [];
const TASK_A = "a";
const TASK_B = "b";
const TASK_C = "c";
const TASK_D = "d";
const DETAIL_IDS = "detail-ids";
const PRELOAD_IDS = "preload-ids";
const MOBILE_TASK_ID = "mobile-task";

class MockIntersectionObserver implements IntersectionObserver {
  readonly root: Element | Document | null = null;
  readonly rootMargin = "";
  readonly thresholds: readonly number[] = [];
  readonly observed = new Set<Element>();

  constructor(
    private readonly callback: IntersectionObserverCallback,
    _options?: IntersectionObserverInit,
  ) {
    observers.push({ callback, instance: this });
  }

  observe(element: Element): void {
    this.observed.add(element);
  }

  unobserve(element: Element): void {
    this.observed.delete(element);
  }

  disconnect(): void {
    this.observed.clear();
  }

  takeRecords(): IntersectionObserverEntry[] {
    return [];
  }

  emit(...entries: Array<Partial<IntersectionObserverEntry> & { target: Element }>): void {
    act(() => {
      this.callback(
        entries.map((entry) => entry as IntersectionObserverEntry),
        this as unknown as IntersectionObserver,
      );
    });
  }
}

function desktopBreakpoint(): ResponsiveBreakpoint {
  return { isMobile: false } as ResponsiveBreakpoint;
}

function mobileBreakpoint(): ResponsiveBreakpoint {
  return { isMobile: true } as ResponsiveBreakpoint;
}

function ActivationColumn({
  id,
  registerColumn,
}: {
  id: string;
  registerColumn: (taskId: string, element: HTMLElement | null) => void;
}) {
  const ref = useCallback(
    (element: HTMLElement | null) => registerColumn(id, element),
    [id, registerColumn],
  );
  return <div ref={ref} data-testid={`column-${id}`} />;
}

function ActivationFixture({ ids, focusedTaskId }: { ids: string[]; focusedTaskId?: string }) {
  const activation = useThreadColumnActivation(ids, focusedTaskId);
  return (
    <div ref={activation.boardRef} data-testid="activation-board">
      <output data-testid="preload-ids">{[...activation.preloadTaskIds].join(",")}</output>
      <output data-testid="detail-ids">{[...activation.detailTaskIds].join(",")}</output>
      <output data-testid={MOBILE_TASK_ID}>{activation.mobileTaskId}</output>
      {ids.map((id) => (
        <ActivationColumn key={id} id={id} registerColumn={activation.registerColumn} />
      ))}
    </div>
  );
}

function ids(testId: string): string[] {
  const value = screen.getByTestId(testId).textContent ?? "";
  return value ? value.split(",") : [];
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  observers.length = 0;
});

function mobileGeometry() {
  const board = screen.getByTestId("activation-board");
  const a = screen.getByTestId(`column-${TASK_A}`);
  const b = screen.getByTestId(`column-${TASK_B}`);
  vi.spyOn(board, "getBoundingClientRect").mockReturnValue({ left: 0, right: 300 } as DOMRect);
  const rectA = vi.spyOn(a, "getBoundingClientRect");
  const rectB = vi.spyOn(b, "getBoundingClientRect");
  function move(offset: number) {
    rectA.mockReturnValue({ left: -offset, right: 300 - offset } as DOMRect);
    rectB.mockReturnValue({ left: 300 - offset, right: 600 - offset } as DOMRect);
    fireEvent.scroll(board);
  }
  return { board, a, b, move };
}

describe("phone position feedback", () => {
  let resize: ResizeObserverCallback;
  const disconnectResize = vi.fn();

  beforeEach(() => {
    responsiveMocks.useResponsiveBreakpoint.mockReturnValue(mobileBreakpoint());
    vi.useFakeTimers({ toFake: ["requestAnimationFrame", "cancelAnimationFrame"] });
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(callback: ResizeObserverCallback) {
          resize = callback;
        }
        observe() {}
        disconnect = disconnectResize;
      },
    );
    disconnectResize.mockClear();
  });

  function frame() {
    act(() => vi.advanceTimersToNextFrame());
  }

  // @covers AC-UI-THREADS-DECK-003.13
  it("updates mobile position across the midpoint without changing intersecting membership", () => {
    render(<ActivationFixture ids={[TASK_A, TASK_B]} />);
    const { a, b, move } = mobileGeometry();
    move(30);
    observers[0].instance.emit(
      { target: a, isIntersecting: true, intersectionRatio: 0.9 },
      { target: b, isIntersecting: true, intersectionRatio: 0.1 },
    );
    frame();
    expect(ids(DETAIL_IDS)).toEqual([TASK_A]);

    move(180);
    observers[0].instance.emit(
      { target: a, isIntersecting: true, intersectionRatio: 0.4 },
      { target: b, isIntersecting: true, intersectionRatio: 0.6 },
    );
    frame();
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe(TASK_B);
    expect(ids(DETAIL_IDS)).toEqual([TASK_B]);

    move(90);
    frame();
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe(TASK_A);
  });

  it("reconciles resize, removal, and an empty deck without waiting for visibility", () => {
    const view = render(<ActivationFixture ids={[TASK_A, TASK_B]} />);
    const { board, move } = mobileGeometry();
    move(180);
    frame();
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe(TASK_B);

    vi.mocked(board.getBoundingClientRect).mockReturnValue({ left: 0, right: 50 } as DOMRect);
    act(() => resize([], {} as ResizeObserver));
    frame();
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe(TASK_A);

    view.rerender(<ActivationFixture ids={[TASK_B]} />);
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe(TASK_B);
    view.rerender(<ActivationFixture ids={[]} />);
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe("");
  });

  it("uses deep-link fallback before geometry is available and clears on desktop", () => {
    const view = render(<ActivationFixture ids={[TASK_A, TASK_B]} focusedTaskId={TASK_B} />);
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe(TASK_B);
    responsiveMocks.useResponsiveBreakpoint.mockReturnValue(desktopBreakpoint());
    view.rerender(<ActivationFixture ids={[TASK_A, TASK_B]} focusedTaskId={TASK_B} />);
    expect(screen.getByTestId(MOBILE_TASK_ID).textContent).toBe("");
    expect(disconnectResize).toHaveBeenCalled();
  });

  it("coalesces scroll work and cancels pending frames on unmount", () => {
    const view = render(<ActivationFixture ids={[TASK_A, TASK_B]} />);
    const { board, move } = mobileGeometry();
    move(180);
    move(190);
    expect(vi.getTimerCount()).toBe(1);
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
    expect(disconnectResize).toHaveBeenCalledTimes(1);
    fireEvent.scroll(board);
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("useThreadColumnActivation", () => {
  beforeEach(() => {
    responsiveMocks.useResponsiveBreakpoint.mockReturnValue(desktopBreakpoint());
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
  });

  it("preloads visible columns and one neighbor while detailing every visible desktop column", () => {
    render(<ActivationFixture ids={[TASK_A, TASK_B, TASK_C, TASK_D]} />);
    const observer = observers[0]?.instance;
    const columnA = screen.getByTestId(`column-${TASK_A}`);
    const columnB = screen.getByTestId(`column-${TASK_B}`);
    const columnC = screen.getByTestId(`column-${TASK_C}`);
    expect(observer?.observed).toEqual(
      new Set([columnA, columnB, columnC, screen.getByTestId(`column-${TASK_D}`)]),
    );

    observer?.emit(
      { target: columnB, isIntersecting: true },
      { target: columnC, isIntersecting: true },
    );

    expect(ids(DETAIL_IDS)).toEqual([TASK_B, TASK_C]);
    expect(ids(PRELOAD_IDS)).toEqual([TASK_A, TASK_B, TASK_C, TASK_D]);

    observer?.emit({ target: columnB, isIntersecting: false });

    expect(ids(DETAIL_IDS)).toEqual([TASK_C]);
    expect(ids(PRELOAD_IDS)).toEqual([TASK_B, TASK_C, TASK_D]);
  });

  it("keeps a focused deep-link task in the conservative fallback window", () => {
    vi.stubGlobal("IntersectionObserver", undefined);

    render(<ActivationFixture ids={[TASK_A, TASK_B, TASK_C, TASK_D]} focusedTaskId={TASK_C} />);

    expect(ids(DETAIL_IDS)).toEqual([TASK_C]);
    expect(ids(PRELOAD_IDS)).toEqual([TASK_B, TASK_C, TASK_D]);
  });

  // @covers AC-TASKS-THREADS-ACTIONS-003.5, AC-TASKS-THREADS-ACTIONS-003.6
  it.each([
    { change: "removal", nextIds: [TASK_A, TASK_B] },
    { change: "readmission", nextIds: [TASK_A, TASK_B, TASK_C, TASK_D] },
  ])("retains the surviving visible chat through $change", ({ nextIds }) => {
    const view = render(<ActivationFixture ids={[TASK_A, TASK_B, TASK_C]} />);
    observers[0].instance.emit({
      target: screen.getByTestId(`column-${TASK_B}`),
      isIntersecting: true,
    });
    expect(ids(DETAIL_IDS)).toEqual([TASK_B]);

    view.rerender(<ActivationFixture ids={nextIds} />);

    expect(ids(DETAIL_IDS)).toEqual([TASK_B]);
    expect(observers[0].instance.observed).toEqual(
      new Set(nextIds.map((id) => screen.getByTestId(`column-${id}`))),
    );
  });

  it("gives phone only the nearest visible snap column as detail owner", () => {
    responsiveMocks.useResponsiveBreakpoint.mockReturnValue(mobileBreakpoint());
    render(<ActivationFixture ids={[TASK_A, TASK_B, TASK_C]} />);
    const board = screen.getByTestId("activation-board");
    const columnA = screen.getByTestId(`column-${TASK_A}`);
    const columnB = screen.getByTestId(`column-${TASK_B}`);
    const columnC = screen.getByTestId(`column-${TASK_C}`);
    vi.spyOn(board, "getBoundingClientRect").mockReturnValue({ left: 0, right: 300 } as DOMRect);
    vi.spyOn(columnA, "getBoundingClientRect").mockReturnValue({
      left: -120,
      right: 80,
    } as DOMRect);
    vi.spyOn(columnB, "getBoundingClientRect").mockReturnValue({
      left: 100,
      right: 300,
    } as DOMRect);
    vi.spyOn(columnC, "getBoundingClientRect").mockReturnValue({
      left: 320,
      right: 520,
    } as DOMRect);

    observers[0]?.instance.emit(
      { target: columnA, isIntersecting: true },
      { target: columnB, isIntersecting: true },
      { target: columnC, isIntersecting: false },
    );

    expect(ids(DETAIL_IDS)).toEqual([TASK_B]);
    expect(ids("detail-ids")).toHaveLength(1);
    expect(ids(PRELOAD_IDS)).toEqual([TASK_A, TASK_B, TASK_C]);
  });
});
