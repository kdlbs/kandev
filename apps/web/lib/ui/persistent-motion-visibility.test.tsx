import { afterEach, describe, expect, it, vi } from "vitest";
import { createPersistentMotionVisibility } from "@kandev/ui/persistent-motion-visibility";

type MockIntersectionObserverInstance = {
  callback: IntersectionObserverCallback;
  disconnect: ReturnType<typeof vi.fn>;
  observe: ReturnType<typeof vi.fn>;
  emit: (isIntersecting: boolean, intersectionRatio?: number) => void;
};

const intersectionObservers: MockIntersectionObserverInstance[] = [];
const originalVisibilityState = Object.getOwnPropertyDescriptor(document, "visibilityState");

class MockIntersectionObserver {
  callback: IntersectionObserverCallback;
  disconnect = vi.fn();
  observe = vi.fn();

  constructor(callback: IntersectionObserverCallback) {
    this.callback = callback;
    intersectionObservers.push(this);
  }

  emit(isIntersecting: boolean, intersectionRatio = isIntersecting ? 1 : 0) {
    this.callback(
      [
        {
          isIntersecting,
          intersectionRatio,
        } as IntersectionObserverEntry,
      ],
      this as unknown as IntersectionObserver,
    );
  }
}

function makeAnimation() {
  let playState: AnimationPlayState = "running";
  return {
    get playState() {
      return playState;
    },
    pause: vi.fn(() => {
      playState = "paused";
    }),
    play: vi.fn(() => {
      playState = "running";
    }),
  } as unknown as Animation;
}

function setDocumentVisibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, "visibilityState", {
    configurable: true,
    value: state,
  });
  document.dispatchEvent(new Event("visibilitychange"));
}

afterEach(() => {
  intersectionObservers.length = 0;
  vi.unstubAllGlobals();
  if (originalVisibilityState) {
    Object.defineProperty(document, "visibilityState", originalVisibilityState);
  } else {
    Reflect.deleteProperty(document, "visibilityState");
  }
  vi.restoreAllMocks();
});

describe("createPersistentMotionVisibility", () => {
  it("pauses owned Web Animations and CSS fallback when the document is hidden", () => {
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    const target = document.createElement("div");
    const element = document.createElement("span");
    element.style.animationPlayState = "running";
    const animation = makeAnimation();
    const controller = createPersistentMotionVisibility(target);
    controller.register(element, animation);

    setDocumentVisibility("hidden");

    expect(animation.pause).toHaveBeenCalledOnce();
    expect(element.style.animationPlayState).toBe("paused");

    setDocumentVisibility("visible");

    expect(animation.play).toHaveBeenCalledOnce();
    expect(element.style.animationPlayState).toBe("running");
    controller.dispose();
  });

  it("pauses offscreen motion and resumes a newly registered effect", () => {
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    const target = document.createElement("div");
    const element = document.createElement("span");
    const hiddenElement = document.createElement("span");
    const firstAnimation = makeAnimation();
    const controller = createPersistentMotionVisibility(target);
    controller.register(element, firstAnimation);

    intersectionObservers[0]?.emit(false);
    expect(firstAnimation.pause).toHaveBeenCalledOnce();

    const hiddenAnimation = makeAnimation();
    controller.register(hiddenElement, hiddenAnimation);
    expect(hiddenAnimation.pause).toHaveBeenCalledOnce();

    intersectionObservers[0]?.emit(true);

    expect(firstAnimation.play).toHaveBeenCalledOnce();
    expect(hiddenAnimation.play).toHaveBeenCalledOnce();
    controller.dispose();
  });

  it("combines owner visibility with document and intersection state", () => {
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    const target = document.createElement("div");
    const element = document.createElement("span");
    const animation = makeAnimation();
    const controller = createPersistentMotionVisibility(target, { ownerVisible: false });
    controller.register(element, animation);

    expect(animation.pause).toHaveBeenCalledOnce();
    controller.setOwnerVisible(true);
    expect(animation.play).toHaveBeenCalledOnce();

    setDocumentVisibility("hidden");
    controller.setOwnerVisible(false);
    setDocumentVisibility("visible");
    expect(animation.play).toHaveBeenCalledOnce();

    controller.setOwnerVisible(true);
    expect(animation.play).toHaveBeenCalledTimes(2);
    controller.dispose();
  });

  it("rejects duplicate registrations without changing the original CSS state", () => {
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    const target = document.createElement("div");
    const element = document.createElement("span");
    const controller = createPersistentMotionVisibility(target);
    const registration = controller.register(element);

    intersectionObservers[0]?.emit(false);
    expect(element.style.animationPlayState).toBe("paused");
    expect(() => controller.register(element)).toThrow();

    intersectionObservers[0]?.emit(true);
    expect(element.style.animationPlayState).toBe("");
    registration.unregister();
    controller.dispose();
  });

  it("treats an intersecting zero-ratio edge target as visible", () => {
    vi.stubGlobal("IntersectionObserver", MockIntersectionObserver);
    const target = document.createElement("div");
    const element = document.createElement("span");
    const animation = makeAnimation();
    const controller = createPersistentMotionVisibility(target);
    controller.register(element, animation);

    intersectionObservers[0]?.emit(false);
    intersectionObservers[0]?.emit(true, 0);

    expect(controller.isVisible()).toBe(true);
    expect(animation.pause).toHaveBeenCalledOnce();
    expect(animation.play).toHaveBeenCalledOnce();
    controller.dispose();
  });

  it("keeps visible feedback usable without IntersectionObserver and cleans up", () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    const target = document.createElement("div");
    const element = document.createElement("span");
    const animation = makeAnimation();
    const controller = createPersistentMotionVisibility(target);
    const registration = controller.register(element, animation);

    expect(animation.pause).not.toHaveBeenCalled();
    setDocumentVisibility("hidden");
    expect(animation.pause).toHaveBeenCalledOnce();

    registration.unregister();
    controller.dispose();
    setDocumentVisibility("visible");
    expect(animation.play).not.toHaveBeenCalled();
  });
});
