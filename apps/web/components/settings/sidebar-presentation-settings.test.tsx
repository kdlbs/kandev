import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { createDefaultUserSettings } from "@/lib/ssr/user-settings";
import type { SettingsSaveContributor } from "./settings-save-provider";

const mocks = vi.hoisted(() => ({
  update: vi.fn(),
  contributor: null as SettingsSaveContributor | null,
}));
let settings = createDefaultUserSettings();
const store = {
  getState: () => ({
    userSettings: settings,
    setUserSettings: (next: typeof settings) => {
      settings = next;
    },
  }),
};
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: ReturnType<typeof store.getState>) => unknown) =>
    select(store.getState()),
  useAppStoreApi: () => store,
}));
vi.mock("@/lib/api/domains/settings-api", () => ({ updateUserSettings: mocks.update }));
vi.mock("./settings-save-provider", () => ({
  useSettingsSaveContributor: (value: SettingsSaveContributor) => {
    mocks.contributor = value;
  },
  SettingsSaveDirtyScope: ({ children }: { children: (dirty: boolean) => React.ReactNode }) =>
    children(false),
}));
import { SidebarPresentationSettings } from "./sidebar-presentation-settings";
beforeEach(() => {
  settings = createDefaultUserSettings();
  mocks.update.mockReset();
});
afterEach(cleanup);
const CHECKED_STATE_ATTRIBUTE = "data-state";

it("drafts account preferences without a workspace and discards to latest saved settings", async () => {
  const view = render(<SidebarPresentationSettings />);
  const toggle = screen.getByTestId("sidebar-fast-actions-setting");
  fireEvent.click(toggle);
  expect(mocks.contributor?.isDirty).toBe(true);
  expect(mocks.update).not.toHaveBeenCalled();
  expect(settings.sidebarFastActionsEnabled).toBe(false);
  settings = { ...settings, sidebarNewTaskStyle: "compact" };
  view.rerender(<SidebarPresentationSettings />);
  expect(toggle.getAttribute(CHECKED_STATE_ATTRIBUTE)).toBe("checked");
  expect(screen.getByTestId("sidebar-new-task-style-setting").textContent).toBe(
    "Compact New Task row",
  );
  await act(async () => {
    await mocks.contributor?.discard();
  });
  expect(toggle.getAttribute(CHECKED_STATE_ATTRIBUTE)).toBe("unchecked");
  expect(mocks.contributor?.isDirty).toBe(false);
});

it("preserves a newer edit during a save and leaves failed drafts intact", async () => {
  let resolve: (response: unknown) => void = () => {};
  mocks.update.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const view = render(<SidebarPresentationSettings />);
  const toggle = screen.getByTestId("sidebar-fast-actions-setting");
  fireEvent.click(toggle);
  const save = mocks.contributor!.save(mocks.contributor!.revision);
  expect(mocks.update).toHaveBeenCalledWith({ sidebar_fast_actions_enabled: true });
  fireEvent.click(toggle);
  await act(async () => {
    resolve({ settings: { sidebar_fast_actions_enabled: true, sidebar_new_task_style: "simple" } });
    await save;
  });
  view.rerender(<SidebarPresentationSettings />);
  expect(toggle.getAttribute(CHECKED_STATE_ATTRIBUTE)).toBe("unchecked");
  expect(mocks.contributor?.isDirty).toBe(true);
  mocks.update.mockRejectedValueOnce(new Error("offline"));
  await act(async () => {
    await expect(mocks.contributor!.save(mocks.contributor!.revision)).rejects.toThrow("offline");
  });
  expect(toggle.getAttribute(CHECKED_STATE_ATTRIBUTE)).toBe("unchecked");
  expect(mocks.contributor?.isDirty).toBe(true);
});
