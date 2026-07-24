import type { ReactNode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StorageOverviewResponse } from "@/lib/types/system";
import { SettingsSaveProvider } from "../../settings-save-provider";
import { StorageMaintenanceSettings } from "./storage-maintenance-settings";

const mocks = vi.hoisted(() => ({
  useStorageMaintenance: vi.fn(),
  useSystemJob: vi.fn(),
}));
const IDLE_PERIOD_TEST_ID = "storage-idle-period";
const SAVE_BUTTON_NAME = "Save changes";

vi.mock("@/hooks/domains/system/use-storage-maintenance", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/hooks/domains/system/use-storage-maintenance")>()),
  useStorageMaintenance: mocks.useStorageMaintenance,
}));

vi.mock("@/hooks/domains/system/use-system-jobs", () => ({
  useSystemJob: mocks.useSystemJob,
  useSystemJobs: () => [],
}));

const overview = {
  settings: {
    enabled: false,
    check_interval_hours: 24,
    idle_for_minutes: 10,
    orphan_grace_hours: 168,
    quarantine_retention_hours: 168,
    workspaces: { enabled: true },
    kandev_containers: { enabled: true },
    go_cache: { enabled: false, max_bytes: 16106127360, adopted_path: "" },
    docker: {
      dedicated_daemon_acknowledged: false,
      build_cache_enabled: false,
      build_cache_keep_bytes: 10737418240,
      build_cache_unused_hours: 168,
      unused_images_enabled: false,
      unused_images_hours: 168,
    },
  },
  capabilities: {
    managed_go_cache_path: "/data/cache/go-build",
    go_cache_adoption_available: true,
    docker_available: true,
    docker_host: "",
    host_global_docker_cleanup_allowed: true,
  },
  summary: {
    workspaces: { active_bytes: 0, candidate_bytes: 0 },
    go_cache: { path: "/data/cache/go-build", size_bytes: 0, owned: true, enabled: false },
    quarantine: { count: 0, size_bytes: 0 },
    docker: {
      available: true,
      build_cache_bytes: 0,
      unused_image_bytes: 0,
      managed_container_count: 0,
      managed_container_bytes: 0,
    },
  },
  analyzed_at: "2026-07-23T12:00:00Z",
  last_run: null,
} satisfies StorageOverviewResponse;

function controller(currentOverview: StorageOverviewResponse) {
  return {
    overview: currentOverview,
    runs: [],
    quarantine: [],
    pendingAction: null,
    error: null,
    analysisJob: undefined,
    cleanupJob: undefined,
    deleteJob: undefined,
    analyze: vi.fn(),
    runNow: vi.fn(),
    save: vi.fn().mockResolvedValue(undefined),
    adopt: vi.fn(),
    restore: vi.fn(),
    permanentlyDelete: vi.fn(),
    reload: vi.fn(),
  };
}

function Providers({ children }: { children: ReactNode }) {
  return (
    <SettingsSaveProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </SettingsSaveProvider>
  );
}

describe("StorageMaintenanceSettings", () => {
  afterEach(cleanup);

  beforeEach(() => {
    mocks.useSystemJob.mockReturnValue(undefined);
    mocks.useStorageMaintenance.mockReturnValue(controller(overview));
  });

  it("shows analysis completion inside the Analyze button", () => {
    const analysisJob = {
      id: "analysis-1",
      kind: "storage-analysis",
      state: "succeeded",
      started_at: "2026-07-16T00:00:00Z",
    } as const;
    mocks.useSystemJob.mockReturnValue(analysisJob);
    mocks.useStorageMaintenance.mockReturnValue({
      ...controller(overview),
      analysisJob,
    });

    render(<StorageMaintenanceSettings />, { wrapper: Providers });

    const analyzeButton = screen.getByTestId("storage-analyze");
    expect(analyzeButton.textContent?.trim()).toBe("Analysis complete");
    expect(analyzeButton.getAttribute("data-job-state")).toBe("succeeded");
    expect(screen.queryByTestId("storage-analysis-job")).toBeNull();
  });

  it("keeps the Analyze button disabled while its job is active", () => {
    const analysisJob = {
      id: "analysis-1",
      kind: "storage-analysis",
      state: "running",
      started_at: "2026-07-16T00:00:00Z",
    } as const;
    mocks.useStorageMaintenance.mockReturnValue({
      ...controller(overview),
      analysisJob,
    });

    render(<StorageMaintenanceSettings />, { wrapper: Providers });

    const analyzeButton = screen.getByTestId("storage-analyze") as HTMLButtonElement;
    expect(analyzeButton.textContent?.trim()).toBe("Analyzing...");
    expect(analyzeButton.disabled).toBe(true);
  });

  it.each(["analyze", "run", "restore", "delete"] as const)(
    "keeps policy editing and shared Save available while %s is pending",
    async (pendingAction) => {
      const editableOverview = { ...overview, settings: { ...overview.settings, enabled: true } };
      const currentController = { ...controller(editableOverview), pendingAction };
      mocks.useStorageMaintenance.mockReturnValue(currentController);
      render(<StorageMaintenanceSettings />, { wrapper: Providers });

      const idlePeriod = screen.getByTestId(IDLE_PERIOD_TEST_ID) as HTMLInputElement;
      expect(idlePeriod.disabled).toBe(false);
      fireEvent.change(idlePeriod, { target: { value: "31" } });

      const save = screen.getByRole("button", { name: SAVE_BUTTON_NAME }) as HTMLButtonElement;
      expect(save.disabled).toBe(false);
      fireEvent.click(save);
      await waitFor(() =>
        expect(currentController.save).toHaveBeenCalledWith(
          { ...editableOverview.settings, idle_for_minutes: 31 },
          undefined,
        ),
      );
    },
  );

  it("blocks a pending adoption with its specific save reason", () => {
    mocks.useStorageMaintenance.mockReturnValue({
      ...controller(overview),
      pendingAction: "adopt",
    });
    render(<StorageMaintenanceSettings />, { wrapper: Providers });

    fireEvent.change(screen.getByTestId(IDLE_PERIOD_TEST_ID), { target: { value: "31" } });

    expect(
      (screen.getByRole("button", { name: SAVE_BUTTON_NAME }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(screen.getByText("Wait for Go cache adoption to finish.")).toBeTruthy();
  });

  it("keeps the contributor valid while its own save is pending", () => {
    const editableOverview = { ...overview, settings: { ...overview.settings, enabled: true } };
    mocks.useStorageMaintenance.mockReturnValue({
      ...controller(editableOverview),
      pendingAction: "save",
    });
    render(<StorageMaintenanceSettings />, { wrapper: Providers });

    fireEvent.change(screen.getByTestId(IDLE_PERIOD_TEST_ID), { target: { value: "31" } });

    expect(
      (screen.getByRole("button", { name: SAVE_BUTTON_NAME }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("preserves a dirty policy draft when refreshed overview data arrives", () => {
    const { rerender } = render(<StorageMaintenanceSettings />, { wrapper: Providers });
    const idlePeriod = screen.getByTestId(IDLE_PERIOD_TEST_ID) as HTMLInputElement;
    fireEvent.change(idlePeriod, { target: { value: "31" } });

    mocks.useStorageMaintenance.mockReturnValue(
      controller({
        ...overview,
        settings: { ...overview.settings, check_interval_hours: 48 },
      }),
    );
    rerender(<StorageMaintenanceSettings />);

    expect((screen.getByTestId(IDLE_PERIOD_TEST_ID) as HTMLInputElement).value).toBe("31");
  });
});

describe("StorageMaintenanceSettings pending policy", () => {
  afterEach(cleanup);

  it("rebases an adopted Go cache path into a dirty policy draft before shared save", async () => {
    const currentController = controller(overview);
    mocks.useStorageMaintenance.mockReturnValue(currentController);
    const { rerender } = render(<StorageMaintenanceSettings />, { wrapper: Providers });
    fireEvent.change(screen.getByTestId(IDLE_PERIOD_TEST_ID), { target: { value: "31" } });

    const adoptedPath = "/mnt/shared/go-build";
    currentController.overview = {
      ...overview,
      settings: {
        ...overview.settings,
        go_cache: { ...overview.settings.go_cache, adopted_path: adoptedPath },
      },
    };
    rerender(<StorageMaintenanceSettings />);

    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON_NAME }));
    await waitFor(() =>
      expect(currentController.save).toHaveBeenCalledWith(
        {
          ...overview.settings,
          idle_for_minutes: 31,
          go_cache: { ...overview.settings.go_cache, adopted_path: adoptedPath },
        },
        undefined,
      ),
    );
  });

  it.each(["analyze", "run", "restore", "delete"] as const)(
    "disables storage action controls while %s is pending without disabling shared Save",
    (pendingAction) => {
      const currentController = { ...controller(overview), pendingAction };
      mocks.useStorageMaintenance.mockReturnValue(currentController);
      render(<StorageMaintenanceSettings />, { wrapper: Providers });

      expect((screen.getByTestId("storage-analyze") as HTMLButtonElement).disabled).toBe(true);
      expect((screen.getByTestId("storage-run-now") as HTMLButtonElement).disabled).toBe(true);
      fireEvent.change(screen.getByTestId(IDLE_PERIOD_TEST_ID), { target: { value: "31" } });
      expect(
        (screen.getByRole("button", { name: SAVE_BUTTON_NAME }) as HTMLButtonElement).disabled,
      ).toBe(false);
    },
  );

  it("blocks Go cache adoption while a save is pending", () => {
    mocks.useStorageMaintenance.mockReturnValue({
      ...controller(overview),
      pendingAction: "save",
    });
    render(<StorageMaintenanceSettings />, { wrapper: Providers });

    expect((screen.getByTestId("storage-go-cache-adopt") as HTMLButtonElement).disabled).toBe(true);
  });
});

describe("StorageMaintenanceSettings coordinated save", () => {
  afterEach(cleanup);

  beforeEach(() => {
    mocks.useSystemJob.mockReturnValue(undefined);
  });

  it("stages policy edits until the shared save action runs", async () => {
    const currentController = controller(overview);
    mocks.useStorageMaintenance.mockReturnValue(currentController);
    render(<StorageMaintenanceSettings />, { wrapper: Providers });

    fireEvent.change(screen.getByTestId(IDLE_PERIOD_TEST_ID), { target: { value: "31" } });

    expect(currentController.save).not.toHaveBeenCalled();
    expect(screen.getByTestId(IDLE_PERIOD_TEST_ID).getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    expect(
      screen.getByTestId("storage-policy-section-schedule").getAttribute("data-settings-dirty"),
    ).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON_NAME }));
    await waitFor(() =>
      expect(currentController.save).toHaveBeenCalledWith(
        { ...overview.settings, idle_for_minutes: 31 },
        undefined,
      ),
    );
  });

  it("stages the Docker acknowledgement and confirms it through the shared save", async () => {
    const currentController = controller(overview);
    mocks.useStorageMaintenance.mockReturnValue(currentController);
    render(<StorageMaintenanceSettings />, { wrapper: Providers });

    fireEvent.click(screen.getByTestId("storage-docker-dedicated"));
    fireEvent.change(screen.getByLabelText("Type DEDICATED to confirm"), {
      target: { value: "DEDICATED" },
    });
    fireEvent.click(screen.getByTestId("storage-docker-confirm"));

    expect(currentController.save).not.toHaveBeenCalled();
    expect(screen.getByTestId("storage-docker-dedicated").getAttribute("data-settings-dirty")).toBe(
      "true",
    );
    fireEvent.click(screen.getByRole("button", { name: SAVE_BUTTON_NAME }));

    await waitFor(() =>
      expect(currentController.save).toHaveBeenCalledWith(
        {
          ...overview.settings,
          docker: {
            ...overview.settings.docker,
            dedicated_daemon_acknowledged: true,
          },
        },
        "DEDICATED",
      ),
    );
  });
});
