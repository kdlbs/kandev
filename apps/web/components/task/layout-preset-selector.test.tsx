import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getBuiltInLayoutProfile } from "@/lib/layout/layout-profiles";

const mocks = vi.hoisted(() => ({
  finePointer: true,
  savedLayouts: [] as Array<Record<string, unknown>>,
  updateUserSettings: vi.fn(),
  setUserSettings: vi.fn(),
  toastError: vi.fn(),
  applyCustomLayout: vi.fn(),
  applyBuiltInPreset: vi.fn(),
  resetLayout: vi.fn(),
  tooltipOpen: false,
  requestTooltipOpen: null as ((open: boolean) => void) | null,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: { name?: string }) => {
      if (key === "task:delete2") return `Delete ${values?.name ?? "saved layout"}`;
      if (key === "task:deleteLayoutConfirm") return `Delete ${values?.name ?? "saved layout"}?`;
      return key;
    },
  }),
}));

vi.mock("@/lib/i18n", () => ({
  t: (key: string) => key,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isFinePointer: mocks.finePointer }),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: (...args: unknown[]) => mocks.toastError(...args) },
}));

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: (...args: unknown[]) => mocks.updateUserSettings(...args),
}));

vi.mock("@/lib/ssr/user-settings", () => ({
  mapUserSettingsResponse: (response: { settings: { saved_layouts: unknown[] } }) => ({
    savedLayouts: response.settings.saved_layouts,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      userSettings: { savedLayouts: mocks.savedLayouts },
      tasks: { activeTaskId: "task-1", activeSessionId: "session-1" },
      setUserSettings: mocks.setUserSettings,
    }),
  useAppStoreApi: () => ({
    getState: () => ({
      userSettings: { savedLayouts: mocks.savedLayouts },
      setUserSettings: mocks.setUserSettings,
      taskSessionsByTask: { itemsByTaskId: { "task-1": [] } },
    }),
  }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: unknown) => unknown) =>
    selector({
      applyCustomLayout: mocks.applyCustomLayout,
      applyBuiltInPreset: mocks.applyBuiltInPreset,
      resetLayout: mocks.resetLayout,
      captureCurrentLayout: vi.fn(() => getBuiltInLayoutProfile("default").layout),
    }),
}));

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ isLoaded: true, loadSessions: vi.fn() }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({
    children,
    open,
    onOpenChange,
  }: {
    children: React.ReactNode;
    open?: boolean;
    onOpenChange?: (open: boolean) => void;
  }) => {
    mocks.tooltipOpen = open ?? false;
    mocks.requestTooltipOpen = onOpenChange ?? null;
    return <>{children}</>;
  },
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

import { LayoutPresetSelector } from "./layout-preset-selector";

const SAVED_DELETE_TEST_ID = "layout-saved-delete";
const SAVED_DELETE_POPOVER_TEST_ID = "layout-saved-delete-confirm-popover";
const SAVED_DELETE_CONFIRM_TEST_ID = "layout-saved-delete-confirm";

function savedLayout(overrides: Record<string, unknown> = {}) {
  return {
    id: "saved-layout",
    name: "Saved layout",
    is_default: false,
    layout: getBuiltInLayoutProfile("default").layout,
    created_at: "2026-08-20T00:00:00.000Z",
    ...overrides,
  };
}

function openSavedLayouts() {
  const trigger = screen.getByTestId("layout-preset-trigger");
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.pointerUp(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(trigger);
  return waitFor(() =>
    expect(screen.getAllByTestId(SAVED_DELETE_TEST_ID).length).toBeGreaterThan(0),
  );
}

function deleteAction(layoutId: string) {
  const action = screen
    .getAllByTestId(SAVED_DELETE_TEST_ID)
    .find((candidate) => candidate.getAttribute("data-layout-id") === layoutId);
  expect(action).toBeTruthy();
  return action!;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  mocks.finePointer = true;
  mocks.savedLayouts = [savedLayout()];
  mocks.updateUserSettings.mockReset();
  mocks.setUserSettings.mockReset();
  mocks.toastError.mockReset();
  mocks.applyCustomLayout.mockReset();
  mocks.applyBuiltInPreset.mockReset();
  mocks.resetLayout.mockReset();
  mocks.tooltipOpen = false;
  mocks.requestTooltipOpen = null;
  mocks.setUserSettings.mockImplementation(
    (settings: { savedLayouts?: Array<Record<string, unknown>> }) => {
      if (settings.savedLayouts) mocks.savedLayouts = settings.savedLayouts;
    },
  );
  mocks.updateUserSettings.mockResolvedValue({ settings: { saved_layouts: [] } });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("LayoutPresetSelector saved-layout deletion", () => {
  it("anchors desktop confirmation to the menu action and keeps cancel local", async () => {
    render(<LayoutPresetSelector />);
    await openSavedLayouts();

    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));

    const popover = screen.getByTestId(SAVED_DELETE_POPOVER_TEST_ID);
    expect(popover).toBeTruthy();
    expect(popover.hasAttribute("data-confirmation-boundary")).toBe(true);
    expect(screen.queryByRole("alertdialog")).toBeNull();

    fireEvent.click(
      within(screen.getByTestId(SAVED_DELETE_POPOVER_TEST_ID)).getByRole("button", {
        name: "common:cancel",
      }),
    );
    expect(mocks.updateUserSettings).not.toHaveBeenCalled();
  });
  it("morphs the menu row into touch-sized actions without stacking an overlay", async () => {
    mocks.finePointer = false;
    render(<LayoutPresetSelector />);
    await openSavedLayouts();

    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));

    const inline = screen.getByTestId("layout-saved-delete-inline-confirmation");
    expect(screen.queryByTestId(SAVED_DELETE_POPOVER_TEST_ID)).toBeNull();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(within(inline).getByTestId("layout-saved-delete-confirm").className).toContain(
      "min-w-11",
    );

    fireEvent.click(within(inline).getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    await waitFor(() =>
      expect(mocks.updateUserSettings).toHaveBeenCalledWith({ saved_layouts: [] }),
    );
  });
});

describe("LayoutPresetSelector saved-layout persistence", () => {
  it("serializes deletions and derives each payload from the latest saved layouts", async () => {
    const first = savedLayout({ id: "first-layout", name: "First layout" });
    const second = savedLayout({ id: "second-layout", name: "Second layout" });
    const retained = savedLayout({ id: "retained-layout", name: "Retained layout" });
    mocks.savedLayouts = [first, second, retained];
    const firstResponse = deferred<{
      settings: { saved_layouts: Array<Record<string, unknown>> };
    }>();
    const secondResponse = deferred<{
      settings: { saved_layouts: Array<Record<string, unknown>> };
    }>();
    mocks.updateUserSettings
      .mockReturnValueOnce(firstResponse.promise)
      .mockReturnValueOnce(secondResponse.promise);

    render(<LayoutPresetSelector />);
    await openSavedLayouts();
    fireEvent.click(deleteAction("first-layout"));
    fireEvent.click(screen.getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1));

    await openSavedLayouts();
    fireEvent.click(deleteAction("second-layout"));
    fireEvent.click(screen.getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1);

    firstResponse.resolve({ settings: { saved_layouts: [second, retained] } });
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(2));
    expect(mocks.updateUserSettings).toHaveBeenLastCalledWith({ saved_layouts: [retained] });
    secondResponse.resolve({ settings: { saved_layouts: [retained] } });
  });

  it("suppresses tooltip reopening after confirmation and clears the close timer on unmount", async () => {
    const setTimeoutSpy = vi.spyOn(globalThis, "setTimeout");
    const clearTimeoutSpy = vi.spyOn(globalThis, "clearTimeout");
    const { unmount } = render(<LayoutPresetSelector />);
    await openSavedLayouts();
    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));
    fireEvent.click(screen.getByTestId(SAVED_DELETE_CONFIRM_TEST_ID));
    await waitFor(() => expect(mocks.updateUserSettings).toHaveBeenCalledTimes(1));

    const timerCallIndex = setTimeoutSpy.mock.calls.findIndex(([, delay]) => delay === 200);
    expect(timerCallIndex).toBeGreaterThanOrEqual(0);
    const timerId = setTimeoutSpy.mock.results[timerCallIndex]?.value;
    act(() => mocks.requestTooltipOpen?.(true));
    expect(mocks.tooltipOpen).toBe(false);

    unmount();
    expect(clearTimeoutSpy).toHaveBeenCalledWith(timerId);
  });

  it("keeps the existing localized error toast when deletion fails", async () => {
    mocks.updateUserSettings.mockRejectedValueOnce(new Error("delete failed"));
    render(<LayoutPresetSelector />);
    await openSavedLayouts();
    fireEvent.click(screen.getByTestId(SAVED_DELETE_TEST_ID));
    fireEvent.click(
      within(screen.getByTestId(SAVED_DELETE_POPOVER_TEST_ID)).getByTestId(
        SAVED_DELETE_CONFIRM_TEST_ID,
      ),
    );

    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledTimes(1));
    expect(mocks.toastError).toHaveBeenCalledWith("task:failedToDeleteLayout", {
      description: "delete failed",
    });
  });
});
