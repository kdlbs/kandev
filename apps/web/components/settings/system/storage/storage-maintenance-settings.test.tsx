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

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
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
    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

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
