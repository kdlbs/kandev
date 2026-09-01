import { afterEach, describe, expect, it, vi } from "vitest";
import {
  captureQuickChatLauncherFocus,
  registerQuickChatCloseHandler,
  requestQuickChatClose,
  restoreQuickChatLauncherFocus,
} from "./quick-chat-focus";

const SILENT_FOCUS_ATTRIBUTE = "data-quick-chat-silent-focus";

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe("quick chat launcher focus", () => {
  it("routes an external close request through the registered modal lifecycle", () => {
    const close = vi.fn();
    const unregister = registerQuickChatCloseHandler(close);

    expect(requestQuickChatClose()).toBe(true);
    expect(close).toHaveBeenCalledTimes(1);

    unregister();
    expect(requestQuickChatClose()).toBe(false);
  });

  // @covers AC-UI-QUICK-TERMINAL-001.9
  it("restores focus through the animation frame after closing", () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 0;
    });
    const launcher = document.createElement("button");
    document.body.append(launcher);
    launcher.focus();

    captureQuickChatLauncherFocus();
    launcher.blur();
    restoreQuickChatLauncherFocus();

    expect(document.activeElement).toBe(launcher);
    expect(launcher.getAttribute(SILENT_FOCUS_ATTRIBUTE)).toBe("true");

    launcher.blur();

    expect(launcher.getAttribute(SILENT_FOCUS_ATTRIBUTE)).toBeNull();
  });

  it("restores focus from a non-launcher origin without a silent marker", () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 0;
    });
    const origin = document.createElement("input");
    document.body.append(origin);
    origin.focus();

    captureQuickChatLauncherFocus({ silent: false });
    origin.blur();
    restoreQuickChatLauncherFocus();

    expect(document.activeElement).toBe(origin);
    expect(origin.getAttribute(SILENT_FOCUS_ATTRIBUTE)).toBeNull();
  });

  it("does not focus a launcher that was removed while the dialog was open", () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 0;
    });
    const launcher = document.createElement("button");
    document.body.append(launcher);
    launcher.focus();

    captureQuickChatLauncherFocus();
    launcher.remove();
    restoreQuickChatLauncherFocus();

    expect(document.activeElement).not.toBe(launcher);
    expect(launcher.getAttribute(SILENT_FOCUS_ATTRIBUTE)).toBeNull();
  });
});
