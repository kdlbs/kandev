import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StorageMaintenanceSettings } from "@/lib/types/system";
import { StoragePolicyCard } from "./storage-policy-card";

const settings: StorageMaintenanceSettings = {
  enabled: false,
  check_interval_hours: 24,
  idle_for_minutes: 10,
  orphan_grace_hours: 168,
  quarantine_retention_hours: 168,
  workspaces: { enabled: true },
  kandev_containers: { enabled: true },
  go_cache: { enabled: false, max_bytes: 16106127360, adopted_path: "" },
  docker: {
    dedicated_daemon_acknowledged: true,
    build_cache_enabled: true,
    build_cache_keep_bytes: 10737418240,
    build_cache_unused_hours: 168,
    unused_images_enabled: true,
    unused_images_hours: 168,
  },
};

const capabilities = {
  managed_go_cache_path: "/data/cache/go-build",
  go_cache_adoption_available: true,
  docker_available: true,
  docker_host: "",
  host_global_docker_cleanup_allowed: true,
};

afterEach(cleanup);

function renderCard(pending = false, onChange = vi.fn()) {
  render(
    <TooltipProvider>
      <StoragePolicyCard
        settings={settings}
        capabilities={capabilities}
        pending={pending}
        onChange={onChange}
        onSave={vi.fn()}
        onDedicatedConfirm={vi.fn()}
        onAdopt={vi.fn()}
      />
    </TooltipProvider>,
  );
  return onChange;
}

describe("StoragePolicyCard", () => {
  it("edits every Docker cleanup threshold", () => {
    const onChange = renderCard();

    expect((screen.getByTestId("storage-go-cache-max") as HTMLInputElement).value).toBe("15");
    expect(
      (screen.getByTestId("storage-docker-build-cache-keep-bytes") as HTMLInputElement).value,
    ).toBe("10");

    fireEvent.change(screen.getByTestId("storage-go-cache-max"), {
      target: { value: "20" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      go_cache: { ...settings.go_cache, max_bytes: 21_474_836_480 },
    });

    fireEvent.change(screen.getByTestId("storage-docker-build-cache-keep-bytes"), {
      target: { value: "2" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      docker: { ...settings.docker, build_cache_keep_bytes: 2147483648 },
    });

    fireEvent.change(screen.getByTestId("storage-docker-build-cache-unused-hours"), {
      target: { value: "72" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      docker: { ...settings.docker, build_cache_unused_hours: 72 },
    });

    fireEvent.change(screen.getByTestId("storage-docker-unused-images-hours"), {
      target: { value: "96" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      docker: { ...settings.docker, unused_images_hours: 96 },
    });
  });

  it("disables policy controls while an action is pending", () => {
    renderCard(true);

    const testIds = [
      "storage-scheduling-enabled",
      "storage-go-cache-enabled",
      "storage-check-interval",
      "storage-idle-period",
      "storage-orphan-grace",
      "storage-quarantine-retention",
      "storage-go-cache-max",
      "storage-go-cache-adopt-path",
      "storage-go-cache-adopt",
      "storage-docker-dedicated",
      "storage-docker-build-cache",
      "storage-docker-build-cache-keep-bytes",
      "storage-docker-build-cache-unused-hours",
      "storage-docker-unused-images",
      "storage-docker-unused-images-hours",
      "storage-save-settings",
    ];
    for (const testId of testIds) {
      expect((screen.getByTestId(testId) as HTMLButtonElement | HTMLInputElement).disabled).toBe(
        true,
      );
    }
    expect(
      (screen.getByLabelText("Clean orphan task workspaces") as HTMLButtonElement).disabled,
    ).toBe(true);
    expect((screen.getByLabelText("Clean Kandev containers") as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("groups related settings and provides help for every policy option", () => {
    renderCard();

    for (const heading of [
      "Schedule",
      "Workspaces and containers",
      "Go build cache",
      "Docker cleanup",
      "Quarantine safety",
    ]) {
      expect(screen.getByText(heading)).toBeTruthy();
    }
    expect(screen.getAllByLabelText(/^More information about /)).toHaveLength(16);
  });
});
