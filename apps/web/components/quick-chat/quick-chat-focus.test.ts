import { afterEach, describe, expect, it, vi } from "vitest";
import { captureQuickChatLauncherFocus, restoreQuickChatLauncherFocus } from "./quick-chat-focus";

afterEach(() => {
  vi.unstubAllGlobals();
  document.body.replaceChildren();
});

describe("quick chat launcher focus", () => {
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
  });
});
