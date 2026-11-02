import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useEffect, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const state = vi.hoisted(() => ({ office: false, mounts: 0, unmounts: 0 }));
vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => state.office }));
vi.mock("./system-route-shell", () => ({
  SystemRouteShell: ({ tabs, children }: { tabs: ReactNode; children: ReactNode }) => (
    <>
      {tabs}
      {children}
    </>
  ),
}));
vi.mock("./storage/storage-maintenance-settings", () => ({
  StorageMaintenanceSettings: () => <div>Host maintenance</div>,
}));
vi.mock("./retention-settings-card", () => ({
  RetentionSettingsCard: () => {
    useEffect(() => {
      state.mounts++;
      return () => {
        state.unmounts++;
      };
    }, []);
    return <input aria-label="Retention draft" defaultValue="30" />;
  },
}));

import { StorageSettings } from "./storage-settings";

beforeEach(() => {
  state.office = false;
  state.mounts = 0;
  state.unmounts = 0;
  window.history.replaceState({}, "", "/settings/system/storage");
});
afterEach(cleanup);

// @covers AC-SYSTEM-PAGE-DATA-STORAGE-PAGES-002.2/.6/.8
describe("Storage Office availability", () => {
  it.each([
    "",
    "?tab=office-retention",
    "#setting-system-retention",
    "?tab=office-retention#setting-system-retention",
  ])("hides Office retention when Office is disabled: %s", (suffix) => {
    window.history.replaceState({}, "", `/settings/system/storage${suffix}`);
    render(<StorageSettings />);
    expect(screen.queryByRole("tab", { name: "Office retention" })).toBeNull();
    expect(screen.getByRole("tab", { name: "Host" }).getAttribute("aria-selected")).toBe("true");
    expect(screen.queryByTestId("settings-storage-office-retention")).toBeNull();
    expect(state.mounts).toBe(0);
  });

  it("keeps enabled retention drafts through a tab round trip and removes them on disable", () => {
    state.office = true;
    window.history.replaceState(
      {},
      "",
      "/settings/system/storage?tab=host#setting-system-retention",
    );
    const { rerender } = render(<StorageSettings />);
    const officeTab = () => screen.getByRole("tab", { name: "Office retention" });
    expect(officeTab().getAttribute("aria-selected")).toBe("true");
    expect(state.mounts).toBe(1);
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "45" } });
    fireEvent.keyDown(screen.getByRole("tab", { name: "Host" }), { key: "Enter" });
    fireEvent.keyDown(officeTab(), { key: "Enter" });
    expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("45");
    state.office = false;
    rerender(<StorageSettings />);
    expect(screen.queryByRole("tab", { name: "Office retention" })).toBeNull();
    expect(screen.getByRole("tab", { name: "Host" }).getAttribute("aria-selected")).toBe("true");
    expect(state.unmounts).toBe(1);
  });
});
