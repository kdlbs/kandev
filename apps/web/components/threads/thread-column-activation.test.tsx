import { act, cleanup, render, screen } from "@testing-library/react";
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
  vi.unstubAllGlobals();
  observers.length = 0;
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
