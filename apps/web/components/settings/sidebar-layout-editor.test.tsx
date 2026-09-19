import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ComponentProps } from "react";
import { ApiError } from "@/lib/api/client";
import { SettingsSaveProvider } from "./settings-save-provider";
import { SidebarLayoutEditor } from "./sidebar-layout-editor";

const mocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: "workspace-1" },
    userSettings: { sidebarLayoutsByWorkspace: {} },
  },
  catalog: [] as Array<{
    target: { kind: "destination"; id: string };
    label: string;
    available: boolean;
  }>,
  isMobile: false,
  setUserSettings: vi.fn(),
  updateUserSettings: vi.fn(),
  fetchUserSettings: vi.fn(),
}));

const SIDEBAR_LABEL = "Sidebar";
const ADD_SECTION_LABEL = "Add shortcut section";
const SECTION_NAME_LABEL = "Section name";
const MOVE_UP_LABEL = "Move up";
const MOVE_DOWN_LABEL = "Move down";
const LOAD_LATEST_LABEL = "Load latest";
const RETRY_SAVE_LABEL = "Retry save";
const LOADING_LABEL = "Loading";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
  useAppStoreApi: () => ({ getState: () => mocks.state }),
}));

vi.mock("react-i18next", () => ({
  Trans: ({ children }: { children?: unknown }) => children,
  useTranslation: () => ({
    t: (key: string) =>
      ({
        "settings:sidebar": SIDEBAR_LABEL,
        "settings:addShortcutSection": ADD_SECTION_LABEL,
        "settings:sectionName": SECTION_NAME_LABEL,
        "settings:moveUp": MOVE_UP_LABEL,
        "settings:moveDown": MOVE_DOWN_LABEL,
        "settings:sidebarConflict": "Conflict",
        "settings:sidebarLoadLatest": LOAD_LATEST_LABEL,
        "settings:retrySave": RETRY_SAVE_LABEL,
        "common:loading": LOADING_LABEL,
      })[key] ?? key,
  }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile, isFinePointer: true }),
}));

vi.mock("@/hooks/domains/sidebar/use-sidebar-shortcut-catalog", () => ({
  useSidebarShortcutCatalog: () => ({
    workspaceId: mocks.state.workspaces.activeId,
    catalog: mocks.catalog,
    loading: false,
    error: null,
    refresh: vi.fn(),
  }),
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: (...args: unknown[]) => mocks.updateUserSettings(...args),
  fetchUserSettings: (...args: unknown[]) => mocks.fetchUserSettings(...args),
}));

type MockSwitchProps = ComponentProps<"button"> & {
  checked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
};

vi.mock("@kandev/ui/switch", () => ({
  Switch: ({ checked = false, onCheckedChange, ...props }: MockSwitchProps) => (
    <button
      {...props}
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={() => onCheckedChange?.(!checked)}
    />
  ),
}));

// eslint-disable-next-line max-lines-per-function -- related editor regression cases share one mocked editor harness.
describe("SidebarLayoutEditor", () => {
  afterEach(() => {
    cleanup();
  });

  beforeEach(() => {
    mocks.updateUserSettings.mockReset();
    mocks.fetchUserSettings.mockReset();
    mocks.setUserSettings.mockReset();
    mocks.catalog = [];
    mocks.isMobile = false;
    mocks.state.workspaces.activeId = "workspace-1";
    mocks.state.userSettings = { sidebarLayoutsByWorkspace: {} };
  });

  it("keeps an empty shortcut section editable and exposes explicit move controls", () => {
    render(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    expect(screen.getByRole("heading", { name: SIDEBAR_LABEL })).toBeTruthy();
    fireEvent.change(screen.getByLabelText(SECTION_NAME_LABEL), { target: { value: "Pinned" } });
    fireEvent.click(screen.getAllByRole("button", { name: ADD_SECTION_LABEL }).at(-1)!);

    expect(screen.getByDisplayValue("Pinned")).toBeTruthy();
    expect(screen.getAllByRole("button", { name: MOVE_UP_LABEL }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: MOVE_DOWN_LABEL }).length).toBeGreaterThan(0);
  });

  it("keeps edits made while a save is pending", async () => {
    let resolveSave: ((value: unknown) => void) | undefined;
    mocks.updateUserSettings.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSave = resolve;
      }),
    );
    render(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    const newSectionInputs = () => screen.getAllByLabelText(SECTION_NAME_LABEL);
    fireEvent.change(newSectionInputs().at(-1)!, { target: { value: "First" } });
    fireEvent.click(screen.getByRole("button", { name: ADD_SECTION_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "settings:saveChanges" }));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1));

    fireEvent.change(newSectionInputs().at(-1)!, { target: { value: "Second" } });
    fireEvent.click(screen.getByRole("button", { name: ADD_SECTION_LABEL }));
    await act(async () => {
      resolveSave?.({
        settings: {
          sidebar_layouts_by_workspace: {
            "workspace-1": {
              version: 1,
              revision: 1,
              nodes: [
                { id: "home", kind: "builtin", visible: true, destination_id: "home" },
                { id: "new-task", kind: "builtin", visible: true, destination_id: "new_task" },
                {
                  id: "automations",
                  kind: "builtin",
                  visible: true,
                  destination_id: "automations",
                },
                { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
                {
                  id: "integrations",
                  kind: "builtin",
                  visible: true,
                  destination_id: "integrations",
                },
                { id: "first", kind: "shortcuts", visible: true, name: "First", shortcuts: [] },
              ],
            },
          },
        },
      });
    });

    expect(screen.getByDisplayValue("Second")).toBeTruthy();
  });

  it("retains a dirty draft when the active workspace changes", () => {
    const view = render(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    fireEvent.change(screen.getAllByLabelText(SECTION_NAME_LABEL).at(-1)!, {
      target: { value: "Workspace one draft" },
    });
    fireEvent.click(screen.getByRole("button", { name: ADD_SECTION_LABEL }));

    mocks.state.workspaces.activeId = "workspace-2";
    view.rerender(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );
    mocks.state.workspaces.activeId = "workspace-1";
    view.rerender(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    expect(screen.getByDisplayValue("Workspace one draft")).toBeTruthy();
  });

  it("offers same-group move controls in the focused phone editor", () => {
    mocks.isMobile = true;
    mocks.catalog = [
      { target: { kind: "destination", id: "github" }, label: "GitHub", available: true },
      { target: { kind: "destination", id: "linear" }, label: "Linear", available: true },
    ];
    mocks.state.userSettings = {
      sidebarLayoutsByWorkspace: {
        "workspace-1": {
          version: 1,
          revision: 1,
          nodes: [
            { id: "home", kind: "builtin", visible: true, destination_id: "home" },
            { id: "new-task", kind: "builtin", visible: true, destination_id: "new_task" },
            { id: "automations", kind: "builtin", visible: true, destination_id: "automations" },
            { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
            { id: "integrations", kind: "builtin", visible: true, destination_id: "integrations" },
            {
              id: "group-1",
              kind: "shortcuts",
              visible: true,
              name: "Pinned",
              shortcuts: [
                { id: "github", target: { kind: "destination", id: "github" } },
                { id: "linear", target: { kind: "destination", id: "linear" } },
              ],
            },
          ],
        },
      },
    };
    render(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    fireEvent.click(
      within(screen.getByTestId("sidebar-layout-node-group-1")).getByRole("button", {
        name: "settings:editSection",
      }),
    );
    expect(screen.getAllByRole("button", { name: MOVE_UP_LABEL }).length).toBeGreaterThan(0);
    expect(screen.getAllByRole("button", { name: MOVE_DOWN_LABEL }).length).toBeGreaterThan(0);
  });

  it("blocks editing when the server returns a newer layout version", () => {
    mocks.state.userSettings = {
      sidebarLayoutsByWorkspace: {
        "workspace-1": {
          version: 99,
          revision: 4,
          nodes: [],
        },
      },
    };

    render(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    expect(screen.getByText("settings:sidebarUnsupportedVersion")).toBeTruthy();
    expect(screen.getByRole("button", { name: ADD_SECTION_LABEL })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("loads the latest revision before retrying a same-workspace conflict", async () => {
    mocks.updateUserSettings.mockRejectedValueOnce(new ApiError("conflict", 409, {}));
    mocks.updateUserSettings.mockResolvedValueOnce({
      settings: {
        sidebar_layouts_by_workspace: {
          "workspace-1": {
            version: 1,
            revision: 8,
            nodes: [
              { id: "home", kind: "builtin", visible: true, destination_id: "home" },
              { id: "new-task", kind: "builtin", visible: true, destination_id: "new_task" },
              { id: "automations", kind: "builtin", visible: true, destination_id: "automations" },
              { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
              {
                id: "integrations",
                kind: "builtin",
                visible: true,
                destination_id: "integrations",
              },
            ],
          },
        },
      },
    });
    mocks.fetchUserSettings.mockResolvedValue({
      settings: {
        sidebar_layouts_by_workspace: {
          "workspace-1": {
            version: 1,
            revision: 7,
            nodes: [
              { id: "home", kind: "builtin", visible: true, destination_id: "home" },
              { id: "new-task", kind: "builtin", visible: true, destination_id: "new_task" },
              { id: "automations", kind: "builtin", visible: true, destination_id: "automations" },
              { id: "canvases", kind: "builtin", visible: true, destination_id: "canvases" },
              {
                id: "integrations",
                kind: "builtin",
                visible: true,
                destination_id: "integrations",
              },
            ],
          },
        },
      },
    });

    render(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );
    fireEvent.change(screen.getAllByLabelText(SECTION_NAME_LABEL).at(-1)!, {
      target: { value: "Pinned" },
    });
    fireEvent.click(screen.getByRole("button", { name: ADD_SECTION_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "settings:saveChanges" }));

    await waitFor(() =>
      expect(screen.getByRole("button", { name: LOAD_LATEST_LABEL })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: LOAD_LATEST_LABEL }));
    await waitFor(() => expect(mocks.fetchUserSettings).toHaveBeenCalledTimes(1));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: RETRY_SAVE_LABEL })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: RETRY_SAVE_LABEL }));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(2));
    expect(mocks.updateUserSettings.mock.calls[1]?.[0]).toMatchObject({
      sidebar_layout_state: { expected_revision: 7 },
    });
  });

  it("scopes save recovery status to the active workspace", async () => {
    let resolveLatest: ((value: unknown) => void) | undefined;
    const conflict = new ApiError("conflict", 409, {});
    mocks.updateUserSettings.mockRejectedValueOnce(conflict).mockRejectedValueOnce(conflict);
    mocks.fetchUserSettings.mockReturnValue(
      new Promise((resolve) => {
        resolveLatest = resolve;
      }),
    );
    const view = render(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    fireEvent.change(screen.getAllByLabelText(SECTION_NAME_LABEL).at(-1)!, {
      target: { value: "Workspace one" },
    });
    fireEvent.click(screen.getByRole("button", { name: ADD_SECTION_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "settings:saveChanges" }));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: LOAD_LATEST_LABEL })).toBeTruthy(),
    );
    fireEvent.click(screen.getByRole("button", { name: LOAD_LATEST_LABEL }));

    await waitFor(() => expect(mocks.fetchUserSettings).toHaveBeenCalledTimes(1));
    mocks.state.workspaces.activeId = "workspace-2";
    view.rerender(
      <SettingsSaveProvider>
        <SidebarLayoutEditor />
      </SettingsSaveProvider>,
    );

    fireEvent.change(screen.getAllByLabelText(SECTION_NAME_LABEL).at(-1)!, {
      target: { value: "Workspace two" },
    });
    fireEvent.click(screen.getByRole("button", { name: ADD_SECTION_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: RETRY_SAVE_LABEL }));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.getByRole("button", { name: LOAD_LATEST_LABEL })).toBeTruthy(),
    );

    resolveLatest?.({ settings: { sidebar_layouts_by_workspace: {} } });
  });
});
