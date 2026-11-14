import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ThreadSessionStatusIcon } from "./thread-session-switcher";

const LABEL = "Agent is working";
const STATUS = { kind: "working", labelKey: "threads:statusWorking", hasAttention: false } as const;
const originalVisibility = Object.getOwnPropertyDescriptor(document, "visibilityState");
const observers: FakeObserver[] = [];

class FakeObserver {
  observe = vi.fn();
  disconnect = vi.fn();
  constructor(private callback: IntersectionObserverCallback) {
    observers.push(this);
  }
  emit(isIntersecting: boolean) {
    this.callback([{ isIntersecting } as IntersectionObserverEntry], this as never);
  }
}

function visibility(value: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", { configurable: true, value });
  document.dispatchEvent(new Event("visibilitychange"));
}

function renderWorking(ownerVisible = true) {
  const view = render(
    <ThreadSessionStatusIcon
      status={STATUS}
      label={LABEL}
      testId="working"
      ownerVisible={ownerVisible}
    />,
  );
  return { ...view, icon: screen.getByTestId("working") as unknown as SVGSVGElement };
}

beforeEach(() => {
  vi.stubGlobal("IntersectionObserver", FakeObserver);
  visibility("visible");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  observers.length = 0;
  if (originalVisibility) Object.defineProperty(document, "visibilityState", originalVisibility);
  else Reflect.deleteProperty(document, "visibilityState");
});

describe("Threads status visibility", () => {
  it("pauses the existing SVG offscreen and restores its CSS play state", () => {
    const { container, icon } = renderWorking();
    expect(container.firstElementChild).toBe(icon);
    expect(icon.tagName.toLowerCase()).toBe("svg");
    expect(observers).toHaveLength(1);
    expect(observers[0].observe).toHaveBeenCalledWith(icon);
    observers[0].emit(false);
    expect(icon.style.animationPlayState).toBe("paused");
    expect(icon.getAttribute("aria-label")).toBe(LABEL);
    observers[0].emit(true);
    expect(icon.style.animationPlayState).toBe("");
  });

  it("combines document and viewport visibility", () => {
    const { icon } = renderWorking();
    visibility("hidden");
    expect(icon.style.animationPlayState).toBe("paused");
    observers[0].emit(false);
    visibility("visible");
    expect(icon.style.animationPlayState).toBe("paused");
    observers[0].emit(true);
    expect(icon.style.animationPlayState).toBe("");
  });

  it("starts paused in a hidden document and releases ownership when status settles", () => {
    visibility("hidden");
    const removeListener = vi.spyOn(document, "removeEventListener");
    const { icon, rerender } = renderWorking();
    expect(icon.style.animationPlayState).toBe("paused");
    rerender(
      <ThreadSessionStatusIcon
        status={{ ...STATUS, kind: "finished" }}
        label="Finished"
        testId="finished"
      />,
    );
    expect(observers[0].disconnect).toHaveBeenCalledOnce();
    expect(removeListener).toHaveBeenCalledWith("visibilitychange", expect.any(Function));
    expect(icon.isConnected).toBe(false);
    expect(icon.style.animationPlayState).toBe("");
    visibility("visible");
    expect(screen.getByTestId("finished").getAttribute("aria-label")).toBe("Finished");
  });
});

it("keeps a known-hidden owner paused without observing its SVG", () => {
  const { icon, rerender } = renderWorking(false);
  expect(icon.style.animationPlayState).toBe("paused");
  expect(observers).toHaveLength(0);
  rerender(<ThreadSessionStatusIcon status={STATUS} label={LABEL} testId="working" ownerVisible />);
  expect(screen.getByTestId("working")).toBe(icon);
  expect(observers).toHaveLength(1);
  expect(observers[0].observe).toHaveBeenCalledWith(icon);
  observers[0].emit(true);
  expect(icon.style.animationPlayState).toBe("");
  rerender(
    <ThreadSessionStatusIcon status={STATUS} label={LABEL} testId="working" ownerVisible={false} />,
  );
  expect(screen.getByTestId("working")).toBe(icon);
  expect(icon.style.animationPlayState).toBe("paused");
  expect(observers).toHaveLength(1);
  expect(observers[0].disconnect).toHaveBeenCalledOnce();
});
