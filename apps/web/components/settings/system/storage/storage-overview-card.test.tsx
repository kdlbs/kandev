import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StorageOverviewResponse } from "@/lib/types/system";
import { StorageOverviewCard } from "./storage-overview-card";

const degradedOverview = {
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
    docker_available: false,
    docker_host: "",
    host_global_docker_cleanup_allowed: false,
  },
  summary: {
    workspaces: { active_bytes: 0, candidate_bytes: 0 },
    go_cache: { path: "/data/cache/go-build", size_bytes: 0, owned: true, enabled: false },
    quarantine: { available: false, warning: "quarantine database unavailable" },
    docker: {
      available: false,
      build_cache_bytes: 0,
      unused_image_bytes: 0,
      managed_container_count: 0,
      managed_container_bytes: 0,
    },
  },
  last_run: null,
} satisfies StorageOverviewResponse;

afterEach(cleanup);

describe("StorageOverviewCard", () => {
  it("renders a degraded quarantine warning without inventing zero usage", () => {
    render(<StorageOverviewCard overview={degradedOverview} onRunGoCache={vi.fn()} />);

    const trigger = screen.getByTestId("storage-resource-quarantine-trigger");
    expect(trigger.textContent).toContain("Unavailable");
    expect(trigger.textContent).not.toContain("0 B");
    fireEvent.click(trigger);
    expect(screen.getByText("quarantine database unavailable")).toBeTruthy();
  });

  it("renders unavailable Docker resources without inventing zero usage", () => {
    render(<StorageOverviewCard overview={degradedOverview} onRunGoCache={vi.fn()} />);

    const dockerResourceIds = [
      "managed-containers",
      "docker-image-layers",
      "docker-build-cache",
      "docker-unused-images",
    ];
    for (const resourceId of dockerResourceIds) {
      const trigger = screen.getByTestId(`storage-resource-${resourceId}-trigger`);
      expect(trigger.textContent).toContain("Unavailable");
      expect(trigger.textContent).not.toContain("0 B");
    }
  });

  it("renders total workspace, unmanaged Go cache, and Docker layer usage", () => {
    const overview = {
      ...degradedOverview,
      summary: {
        ...degradedOverview.summary,
        workspaces: {
          total_bytes: 8 * 1024 ** 3,
          active_bytes: 2 * 1024 ** 3,
          candidate_bytes: 5 * 1024 ** 3,
        },
        go_cache: {
          ...degradedOverview.summary.go_cache,
          unmanaged_path: "/root/.cache/go-build",
          unmanaged_size_bytes: 25 * 1024 ** 3,
        },
        docker: {
          ...degradedOverview.summary.docker,
          available: true,
          image_layer_bytes: 14 * 1024 ** 3,
        },
      },
    } satisfies StorageOverviewResponse;

    render(<StorageOverviewCard overview={overview} onRunGoCache={vi.fn()} />);

    expect(screen.getByTestId("storage-resource-workspaces-trigger").textContent).toContain("8 GB");
    expect(screen.getByTestId("storage-resource-unmanaged-go-cache-trigger").textContent).toContain(
      "25 GB",
    );
    expect(
      screen.getByTestId("storage-resource-docker-image-layers-trigger").textContent,
    ).toContain("14 GB");
  });
});
