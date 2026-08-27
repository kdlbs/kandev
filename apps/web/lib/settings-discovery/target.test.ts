import { afterEach, describe, expect, it, vi } from "vitest";
import {
  SETTINGS_TARGET_ATTRIBUTE,
  SETTINGS_TARGET_HIGHLIGHT_ATTRIBUTE,
  createSettingsTargetRegistry,
  revealSettingsTarget,
  settingsTargetFromHash,
  settingsTargetSelector,
} from "./target";

afterEach(() => {
  document.body.innerHTML = "";
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("settings target identity", () => {
  it("decodes valid fragments and ignores empty or malformed fragments", () => {
    expect(settingsTargetFromHash("#setting-terminal-font-size")).toBe(
      "setting-terminal-font-size",
    );
    expect(settingsTargetFromHash("#setting%3Aterminal")).toBe("setting:terminal");
    expect(settingsTargetFromHash("")).toBeNull();
    expect(settingsTargetFromHash("#")).toBeNull();
    expect(settingsTargetFromHash("#%E0%A4%A")).toBeNull();
  });

  it("escapes target ids used in DOM selectors", () => {
    expect(settingsTargetSelector("setting:terminal")).toBe(
      `[${SETTINGS_TARGET_ATTRIBUTE}="setting\\:terminal"]`,
    );
  });
});

describe("settings target registry", () => {
  it("reveals a target already registered", () => {
    const reveal = vi.fn();
    const registry = createSettingsTargetRegistry(reveal);
    const target = document.createElement("div");

    registry.register("font-size", target);

    expect(registry.request("font-size")).toBe(true);
    expect(reveal).toHaveBeenCalledWith(target);
  });

  it("keeps a missing request pending until asynchronous content registers", () => {
    const reveal = vi.fn();
    const registry = createSettingsTargetRegistry(reveal);
    const target = document.createElement("div");

    expect(registry.request("late-control")).toBe(false);
    registry.register("late-control", target);

    expect(reveal).toHaveBeenCalledWith(target);
  });

  it("re-reveals a target when the same fragment is requested twice", () => {
    const reveal = vi.fn();
    const registry = createSettingsTargetRegistry(reveal);
    const target = document.createElement("div");
    registry.register("font-size", target);

    registry.request("font-size");
    registry.request("font-size");

    expect(reveal).toHaveBeenCalledTimes(2);
  });
});

describe("revealSettingsTarget", () => {
  it("centers, focuses the first control, then removes its one-shot highlight", () => {
    vi.useFakeTimers();
    const target = document.createElement("div");
    const input = document.createElement("input");
    target.appendChild(input);
    target.scrollIntoView = vi.fn();
    document.body.appendChild(target);

    revealSettingsTarget(target, { highlightDurationMs: 900, reducedMotion: false });

    expect(target.scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "center" });
    expect(document.activeElement).toBe(input);
    expect(target.getAttribute(SETTINGS_TARGET_HIGHLIGHT_ATTRIBUTE)).toBe("true");

    vi.advanceTimersByTime(900);
    expect(target.hasAttribute(SETTINGS_TARGET_HIGHLIGHT_ATTRIBUTE)).toBe(false);
  });

  it("honors an explicit focus marker and reduced motion", () => {
    const target = document.createElement("div");
    const first = document.createElement("button");
    const marked = document.createElement("button");
    marked.setAttribute("data-settings-target-focus", "");
    target.append(first, marked);
    target.scrollIntoView = vi.fn();
    document.body.appendChild(target);

    revealSettingsTarget(target, { reducedMotion: true });

    expect(target.scrollIntoView).toHaveBeenCalledWith({ behavior: "auto", block: "center" });
    expect(document.activeElement).toBe(marked);
  });

  it("re-centers the target when surrounding content grows while settling", () => {
    vi.useFakeTimers();
    let callback: ResizeObserverCallback | undefined;
    const observed: Element[] = [];
    const disconnect = vi.fn(() => {
      callback = undefined;
    });
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(cb: ResizeObserverCallback) {
          callback = cb;
        }
        observe = (el: Element) => void observed.push(el);
        unobserve = vi.fn();
        disconnect = disconnect;
      },
    );

    const parent = document.createElement("div");
    const target = document.createElement("div");
    parent.appendChild(target);
    document.body.appendChild(parent);
    target.scrollIntoView = vi.fn();

    revealSettingsTarget(target, { reducedMotion: true });
    expect(observed).toContain(parent);

    const resize = (el: Element, height: number) =>
      callback?.([{ target: el, contentRect: { width: 100, height } }] as never, {} as never);

    resize(parent, 100); // baseline notification — no shift yet
    expect(target.scrollIntoView).toHaveBeenCalledTimes(1);

    resize(parent, 400); // async content grew above the target
    expect(target.scrollIntoView).toHaveBeenCalledTimes(2);
    expect(target.scrollIntoView).toHaveBeenLastCalledWith({ behavior: "auto", block: "center" });

    vi.advanceTimersByTime(3000); // settle window elapsed
    resize(parent, 800);
    expect(target.scrollIntoView).toHaveBeenCalledTimes(2);
    expect(disconnect).toHaveBeenCalled();
  });

  it("stops re-centering as soon as the user interacts", () => {
    vi.useFakeTimers();
    let callback: ResizeObserverCallback | undefined;
    const disconnect = vi.fn(() => {
      callback = undefined;
    });
    vi.stubGlobal(
      "ResizeObserver",
      class {
        constructor(cb: ResizeObserverCallback) {
          callback = cb;
        }
        observe = vi.fn();
        unobserve = vi.fn();
        disconnect = disconnect;
      },
    );

    const target = document.createElement("div");
    document.body.appendChild(target);
    target.scrollIntoView = vi.fn();

    revealSettingsTarget(target, { reducedMotion: true });
    const resize = (height: number) =>
      callback?.([{ target, contentRect: { width: 100, height } }] as never, {} as never);
    resize(100);

    window.dispatchEvent(new Event("wheel"));
    expect(disconnect).toHaveBeenCalled();

    resize(400);
    expect(target.scrollIntoView).toHaveBeenCalledTimes(1);
  });
});
