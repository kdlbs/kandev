import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BackendReloadSnapshot } from "@/lib/platform/backend-reload-coordinator";

const reloadState = vi.hoisted(() => ({
  snapshot: {
    reloadRequired: false,
    source: null as BackendReloadSnapshot["source"],
    ownerCount: 0,
  },
  listeners: new Set<() => void>(),
}));

vi.mock("@/lib/platform/backend-reload-coordinator", () => ({
  backendReloadCoordinator: {
    getSnapshot: () => reloadState.snapshot,
    subscribe: (listener: () => void) => {
      reloadState.listeners.add(listener);
      return () => reloadState.listeners.delete(listener);
    },
  },
}));

import { BackendReloadRequiredAlert } from "./backend-reload-required-alert";

function setSnapshot(snapshot: typeof reloadState.snapshot): void {
  reloadState.snapshot = snapshot;
  reloadState.listeners.forEach((listener) => listener());
}

beforeEach(() => {
  reloadState.snapshot = { reloadRequired: false, source: null, ownerCount: 0 };
  reloadState.listeners.clear();
});

afterEach(cleanup);

describe("BackendReloadRequiredAlert", () => {
  it("shows one persistent reload action after a proven restart", () => {
    act(() => {
      setSnapshot({ reloadRequired: true, source: "boot_id_changed", ownerCount: 0 });
    });

    render(<BackendReloadRequiredAlert />);

    expect(screen.getByTestId("backend-reload-required-alert")).toBeTruthy();
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(screen.getByText("Reload required")).toBeTruthy();
    expect(
      screen.getByText(
        "Kandev restarted. Reload this page to continue. Reloading discards unsaved changes.",
      ),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Reload page" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });

  it("keeps the global alert hidden while an intentional restart owns recovery", () => {
    setSnapshot({ reloadRequired: true, source: "boot_id_changed", ownerCount: 1 });

    render(<BackendReloadRequiredAlert />);
    expect(screen.queryByTestId("backend-reload-required-alert")).toBeNull();

    act(() => {
      setSnapshot({ reloadRequired: true, source: "boot_id_changed", ownerCount: 0 });
    });
    expect(screen.getByTestId("backend-reload-required-alert")).toBeTruthy();
  });

  it("reloads the current document only after the user activates the action", () => {
    const reload = vi.fn();
    vi.stubGlobal("location", { ...window.location, reload });
    setSnapshot({ reloadRequired: true, source: "boot_id_changed", ownerCount: 0 });

    render(<BackendReloadRequiredAlert />);
    expect(reload).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Reload page" }));
    expect(reload).toHaveBeenCalledOnce();
  });

  it("uses a touch-sized full-width action in the phone layout", () => {
    setSnapshot({ reloadRequired: true, source: "boot_id_changed", ownerCount: 0 });

    render(<BackendReloadRequiredAlert />);

    const action = screen.getByRole("button", { name: "Reload page" });
    expect(action.className).toContain("h-11");
    expect(action.className).toContain("w-full");
  });
});
