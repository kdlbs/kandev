import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({
  finePointer: true,
  useLayoutSettings: vi.fn(),
  deleteSelected: vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: { name?: string }) => (values?.name ? `${key}:${values.name}` : key),
  }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: mocks.finePointer }),
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: vi.fn(),
}));

vi.mock("@/components/settings/settings-target", () => ({
  SettingsTarget: ({ children, ...props }: { children: React.ReactNode }) => (
    <div {...props}>{children}</div>
  ),
}));

vi.mock("./layout-editor", () => ({
  LayoutEditor: () => <div data-testid="layout-editor" />,
}));

vi.mock("./layout-profile-list", () => ({
  LayoutProfileList: () => <div data-testid="layout-profile-list" />,
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("./use-layout-settings", () => ({
  useLayoutSettings: () => mocks.useLayoutSettings(),
}));

import { LayoutSettings } from "./layout-settings";

const DELETE_PROFILE_TEST_ID = "layout-profile-delete";

function controller() {
  const selectedCustom = {
    id: "custom-layout",
    name: "Custom layout",
    is_default: false,
    layout: {},
    created_at: "2026-08-20T00:00:00.000Z",
  };
  return {
    profiles: [selectedCustom],
    selection: { kind: "custom" as const, id: selectedCustom.id },
    selectedCustom,
    selectedBuiltIn: null,
    selectedBuiltInOverride: null,
    compatibility: { status: "editable" as const, profile: selectedCustom, layout: {}, issues: [] },
    editorLayout: {},
    selectedName: selectedCustom.name,
    selectedIsDefault: false,
    selectedSavedDefault: false,
    defaultActionDisabled: false,
    defaultActionLabel: "Use as default",
    editorReset: 0,
    isDirty: false,
    profilesKey: "profiles",
    error: null,
    save: vi.fn(),
    cancel: vi.fn(),
    create: vi.fn(),
    duplicate: vi.fn(),
    setSelection: vi.fn(),
    updateSelected: vi.fn(),
    updateLayout: vi.fn(),
    setDefault: vi.fn(),
    resetBuiltIn: vi.fn(),
    deleteSelected: mocks.deleteSelected,
  };
}

beforeEach(() => {
  mocks.finePointer = true;
  mocks.deleteSelected.mockReset();
  mocks.useLayoutSettings.mockReturnValue(controller());
});

afterEach(cleanup);

describe("LayoutSettings deletion confirmation", () => {
  it("anchors confirmation to the selected profile action on fine pointers", async () => {
    render(<LayoutSettings />);

    const trigger = screen.getByTestId(DELETE_PROFILE_TEST_ID);
    fireEvent.click(trigger);

    const popover = screen.getByTestId("layout-profile-delete-confirm-popover");
    expect(screen.queryByRole("alertdialog")).toBeNull();

    fireEvent.click(within(popover).getByRole("button", { name: "settings:cancel" }));
    expect(mocks.deleteSelected).not.toHaveBeenCalled();
    await waitFor(() => expect(document.activeElement).toBe(trigger));

    fireEvent.click(trigger);
    fireEvent.click(
      within(screen.getByTestId("layout-profile-delete-confirm-popover")).getByTestId(
        "layout-profile-delete-confirm",
      ),
    );
    await waitFor(() => expect(mocks.deleteSelected).toHaveBeenCalledTimes(1));
  });

  it("morphs into touch-sized inline actions on coarse pointers", async () => {
    mocks.finePointer = false;
    render(<LayoutSettings />);

    fireEvent.click(screen.getByTestId(DELETE_PROFILE_TEST_ID));

    const inline = screen.getByTestId("layout-profile-delete-inline-confirmation");
    expect(screen.queryByTestId("layout-profile-delete-confirm-popover")).toBeNull();
    expect(within(inline).getByRole("button", { name: "settings:cancel" })).toBeTruthy();
    expect(within(inline).getByTestId("layout-profile-delete-confirm").className).toContain(
      "min-w-11",
    );

    fireEvent.click(within(inline).getByRole("button", { name: "settings:cancel" }));
    expect(mocks.deleteSelected).not.toHaveBeenCalled();
    await waitFor(() =>
      expect(document.activeElement).toBe(screen.getByTestId(DELETE_PROFILE_TEST_ID)),
    );

    fireEvent.click(screen.getByTestId(DELETE_PROFILE_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId("layout-profile-delete-inline-confirmation")).getByTestId(
        "layout-profile-delete-confirm",
      ),
    );
    await waitFor(() => expect(mocks.deleteSelected).toHaveBeenCalledTimes(1));
  });

  it("does not expose deletion for built-in profiles", () => {
    mocks.useLayoutSettings.mockReturnValue({
      ...controller(),
      selectedCustom: null,
      selection: { kind: "built-in", id: "default" },
      selectedName: "Default",
    });

    render(<LayoutSettings />);

    expect(screen.queryByTestId(DELETE_PROFILE_TEST_ID)).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
});
