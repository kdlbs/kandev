import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { IconList } from "@tabler/icons-react";

const mocks = vi.hoisted(() => ({
  state: {
    workspaces: { activeId: "workspace-1" },
    automations: { items: [] },
  },
  canvasesEnabled: false,
  canvases: vi.fn(),
  automationRefresh: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof mocks.state) => unknown) => selector(mocks.state),
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => mocks.canvasesEnabled,
}));

vi.mock("@/hooks/use-app-destinations", () => ({
  useStaticDestinations: () => [
    { id: "home", label: "Home", icon: IconList, section: "primary", href: "/" },
  ],
  useAppDestinations: () => [],
}));

vi.mock("@/components/runs/use-workspace-automations", () => ({
  useWorkspaceAutomations: () => ({
    automations: [],
    loading: false,
    error: null,
    refresh: mocks.automationRefresh,
  }),
}));

vi.mock("@/lib/api/domains/canvas-api", () => ({
  listWorkspaceCanvases: (...args: unknown[]) => mocks.canvases(...args),
}));

vi.mock("@/lib/canvas-lifecycle", () => ({
  useCanvasLifecycleRevision: () => 0,
}));

vi.mock("@/components/runs/use-live-refresh", () => ({ useLiveRefresh: vi.fn() }));
vi.mock("@/hooks/use-foreground-refresh", () => ({ useForegroundRefresh: vi.fn() }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

import { useSidebarShortcutCatalog } from "./use-sidebar-shortcut-catalog";

describe("useSidebarShortcutCatalog", () => {
  beforeEach(() => {
    mocks.canvasesEnabled = false;
    mocks.canvases.mockReset();
    mocks.automationRefresh.mockReset();
  });

  it("settles the disabled canvas source without hiding built-in choices", async () => {
    const { result } = renderHook(() => useSidebarShortcutCatalog());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(mocks.canvases).not.toHaveBeenCalled();
    expect(result.current.canvasLoading).toBe(false);
    expect(result.current.canvasError).toBeNull();
    expect(result.current.catalog.some((entry) => entry.target.id === "home")).toBe(true);
    expect(result.current.definitionsError).toBeNull();
  });

  it("keeps a canvas failure independent from automation definitions", async () => {
    mocks.canvasesEnabled = true;
    mocks.canvases.mockRejectedValue(new Error("canvas unavailable"));

    const { result } = renderHook(() => useSidebarShortcutCatalog());

    await waitFor(() => expect(result.current.canvasError).toBe("canvas unavailable"));

    expect(result.current.error).toBeNull();
    expect(result.current.loading).toBe(false);
    expect(result.current.catalog.some((entry) => entry.target.id === "home")).toBe(true);
  });

  it("does not fetch canvases while the navigation surface is inactive", async () => {
    mocks.canvasesEnabled = true;

    const { result } = renderHook(() => useSidebarShortcutCatalog({ active: false }));

    await waitFor(() => expect(result.current.canvasLoading).toBe(false));

    expect(mocks.canvases).not.toHaveBeenCalled();
    expect(result.current.canvasError).toBeNull();
  });
});
