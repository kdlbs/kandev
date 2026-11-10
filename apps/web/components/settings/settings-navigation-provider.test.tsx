import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { StrictMode, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { replaceTaskUrl } from "@/lib/links";
import { useRouter } from "@/lib/routing/client-router";
import {
  clearNavigationBlockerForTests,
  setNavigationBlocker,
  type NavigationIntent,
} from "@/lib/routing/navigation-guard";
import { SettingsNavigationProvider, useSettingsReturnHref } from "./settings-navigation-provider";

const INITIAL_TASK_HREF = "/t/task-a?sessionId=first";

function wrapper({ children }: { children: ReactNode }) {
  return (
    <StrictMode>
      <SettingsNavigationProvider>{children}</SettingsNavigationProvider>
    </StrictMode>
  );
}
function setup() {
  return renderHook(() => ({ href: useSettingsReturnHref(), router: useRouter() }), { wrapper });
}

beforeEach(() => {
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", INITIAL_TASK_HREF);
});
afterEach(() => {
  cleanup();
  clearNavigationBlockerForTests();
});

describe("Settings navigation lifetime", () => {
  // @covers AC-UI-SETTINGS-RETURN-001.1, AC-UI-SETTINGS-RETURN-001.2
  it("captures a silent session URL change before same-tick settings redirects", () => {
    const { result } = setup();
    act(() => {
      replaceTaskUrl("task-a", "selected-session");
      result.current.router.push("/settings");
      result.current.router.replace("/settings/preferences/appearance");
      result.current.router.push("/settings/agents");
    });
    expect(result.current.href).toBe("/t/task-a?sessionId=selected-session");
  });

  // @covers AC-UI-SETTINGS-RETURN-001.3
  it("retains the origin when a close is cancelled and clears it on a permitted close", () => {
    const { result } = setup();
    act(() => result.current.router.push("/settings"));
    let intent: NavigationIntent | undefined;
    const unblock = setNavigationBlocker((pending) => {
      intent = pending;
    });
    act(() => result.current.router.push(result.current.href!));
    expect(window.location.pathname).toBe("/settings");
    expect(result.current.href).toBe(INITIAL_TASK_HREF);
    act(() => intent!.cancel());
    expect(result.current.href).toBe(INITIAL_TASK_HREF);
    act(() => result.current.router.push(result.current.href!));
    act(() => intent!.proceed());
    expect(window.location.pathname).toBe("/t/task-a");
    expect(result.current.href).toBeNull();
    unblock();
  });

  it("does not capture a blocked Settings entry", () => {
    const { result } = setup();
    const unblock = setNavigationBlocker((intent) => intent.cancel());
    act(() => result.current.router.push("/settings"));
    expect(result.current.href).toBeNull();
    expect(window.location.pathname).toBe("/t/task-a");
    unblock();
  });

  // @covers AC-UI-SETTINGS-RETURN-001.4
  it("has no inherited origin after a Settings remount", () => {
    const first = setup();
    act(() => first.result.current.router.push("/settings"));
    first.unmount();
    const second = setup();
    expect(second.result.current.href).toBeNull();
    act(() => second.result.current.router.replace("/settings/agents"));
    expect(second.result.current.href).toBeNull();
  });

  // @covers AC-UI-SETTINGS-RETURN-001.5
  it("uses a new origin after explicit Home navigation and a second visit", () => {
    const { result } = setup();
    act(() => {
      result.current.router.push("/settings");
      result.current.router.push("/?home=overview");
    });
    expect(result.current.href).toBeNull();
    act(() => {
      result.current.router.push("/t/task-b?sessionId=second#plan");
      result.current.router.push("/settings/agents");
    });
    expect(result.current.href).toBe("/t/task-b?sessionId=second#plan");
  });

  // @covers AC-UI-SETTINGS-RETURN-001.3
  it("preserves the origin after cancelling Back across a fragment entry", async () => {
    window.history.replaceState({}, "", "/t/task-b#plan");
    const { result } = setup();
    act(() => {
      result.current.router.push(INITIAL_TASK_HREF);
      result.current.router.push("/settings");
    });
    let intent: NavigationIntent | undefined;
    const unblock = setNavigationBlocker((pending) => {
      intent = pending;
    });
    const traverse = window.history.go.bind(window.history);
    let restorationDelta = 0;
    const go = vi.spyOn(window.history, "go").mockImplementation((delta = 0) => {
      if (delta > 0) restorationDelta = delta;
      else traverse(delta);
    });
    try {
      act(() => window.history.go(-2));
      await waitFor(() => expect(intent).toBeDefined());
      expect(window.location.pathname + window.location.hash).toBe("/t/task-b#plan");
      // happy-dom does not emit the native traversal's separate hashchange.
      act(() => window.dispatchEvent(new HashChangeEvent("hashchange")));
      act(() => traverse(restorationDelta));
      await waitFor(() => expect(window.location.pathname).toBe("/settings"));
      act(() => intent!.cancel());
      expect(result.current.href).toBe(INITIAL_TASK_HREF);
    } finally {
      go.mockRestore();
      unblock();
    }
  });

  it("tracks browser Back and Forward without a Settings loop", async () => {
    const { result } = setup();
    act(() => result.current.router.push("/settings"));
    act(() => result.current.router.back());
    await waitFor(() => expect(window.location.pathname).toBe("/t/task-a"));
    expect(result.current.href).toBeNull();
    act(() => result.current.router.forward());
    await waitFor(() => expect(result.current.href).toBe(INITIAL_TASK_HREF));
  });
});
