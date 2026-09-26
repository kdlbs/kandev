import { afterEach, describe, expect, it, vi } from "vitest";
import {
  clearNavigationBlockerForTests,
  setNavigationBlocker,
} from "@/lib/routing/navigation-guard";
import { SETTINGS_TARGET_REQUEST_EVENT, type SettingsTargetRequestDetail } from "./target";
import { navigateToSettingsDiscovery } from "./navigation";

const FONT_SIZE_TARGET_ID = "font-size";
const TERMINAL_FONT_SIZE_HREF = `/settings/general/terminal#${FONT_SIZE_TARGET_ID}`;

afterEach(() => {
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", "/");
  vi.restoreAllMocks();
});

describe("navigateToSettingsDiscovery", () => {
  it("uses guarded app routing when the owning page changes", () => {
    window.history.replaceState({}, "", "/settings/general/appearance");
    const push = vi.fn();

    navigateToSettingsDiscovery(TERMINAL_FONT_SIZE_HREF, push);

    expect(push).toHaveBeenCalledWith(TERMINAL_FONT_SIZE_HREF);
  });

  it("updates and requests a same-page fragment without invoking the leave blocker", () => {
    window.history.replaceState({}, "", "/settings/general/terminal");
    const push = vi.fn();
    const blocker = vi.fn();
    setNavigationBlocker(blocker);
    const requests: string[] = [];
    const listener = (event: Event) => {
      requests.push((event as CustomEvent<SettingsTargetRequestDetail>).detail.targetId);
    };
    window.addEventListener(SETTINGS_TARGET_REQUEST_EVENT, listener);

    navigateToSettingsDiscovery(TERMINAL_FONT_SIZE_HREF, push);

    expect(push).not.toHaveBeenCalled();
    expect(blocker).not.toHaveBeenCalled();
    expect(window.location.hash).toBe(`#${FONT_SIZE_TARGET_ID}`);
    expect(requests).toEqual([FONT_SIZE_TARGET_ID]);
    window.removeEventListener(SETTINGS_TARGET_REQUEST_EVENT, listener);
  });

  it("emits another request when the current fragment is selected again", () => {
    window.history.replaceState({}, "", TERMINAL_FONT_SIZE_HREF);
    const requests: string[] = [];
    const listener = (event: Event) => {
      requests.push((event as CustomEvent<SettingsTargetRequestDetail>).detail.targetId);
    };
    window.addEventListener(SETTINGS_TARGET_REQUEST_EVENT, listener);

    navigateToSettingsDiscovery(TERMINAL_FONT_SIZE_HREF, vi.fn());
    navigateToSettingsDiscovery(TERMINAL_FONT_SIZE_HREF, vi.fn());

    expect(requests).toEqual([FONT_SIZE_TARGET_ID, FONT_SIZE_TARGET_ID]);
    window.removeEventListener(SETTINGS_TARGET_REQUEST_EVENT, listener);
  });
});
