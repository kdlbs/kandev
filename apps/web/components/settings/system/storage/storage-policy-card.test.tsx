import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
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
  workspaces: { enabled: true, dependency_cleanup_enabled: false },
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

const testIds = {
  goCacheMax: "storage-go-cache-max",
  dockerBuildCacheKeep: "storage-docker-build-cache-keep-bytes",
  dockerBuildCacheUnused: "storage-docker-build-cache-unused-hours",
  dockerImagesUnused: "storage-docker-unused-images-hours",
};
const dirtyAttribute = "data-settings-dirty";
const ADOPTION_PATH_TEST_ID = "storage-go-cache-adopt-path";
const ADOPT_BUTTON_TEST_ID = "storage-go-cache-adopt";

afterEach(cleanup);

function renderCard(
  pending = false,
  onChange = vi.fn(),
  currentSettings: StorageMaintenanceSettings = settings,
  savedSettings: StorageMaintenanceSettings = settings,
  onCleanDependencies?: () => void,
) {
  render(
    <TooltipProvider>
      <StoragePolicyCard
        settings={currentSettings}
        savedSettings={savedSettings}
        capabilities={capabilities}
        pending={pending}
        onChange={onChange}
        onAdopt={vi.fn()}
        onCleanDependencies={onCleanDependencies}
      />
    </TooltipProvider>,
  );
  return onChange;
}

describe("External Go cache path", () => {
  it("hydrates the external cache path from persisted settings", () => {
    const persistedSettings = {
      ...settings,
      go_cache: { ...settings.go_cache, adopted_path: "/var/cache/go-build" },
    };
    renderCard(false, vi.fn(), persistedSettings, persistedSettings);

    expect((screen.getByTestId(ADOPTION_PATH_TEST_ID) as HTMLInputElement).value).toBe(
      "/var/cache/go-build",
    );
  });

  it("follows a persisted external cache path change while the input is clean", () => {
    const firstSettings = {
      ...settings,
      go_cache: { ...settings.go_cache, adopted_path: "/var/cache/first" },
    };
    const secondSettings = {
      ...settings,
      go_cache: { ...settings.go_cache, adopted_path: "/var/cache/second" },
    };
    const { rerender } = render(
      <TooltipProvider>
        <StoragePolicyCard
          settings={firstSettings}
          savedSettings={firstSettings}
          capabilities={capabilities}
          pending={false}
          onChange={vi.fn()}
          onAdopt={vi.fn()}
        />
      </TooltipProvider>,
    );

    rerender(
      <TooltipProvider>
        <StoragePolicyCard
          settings={secondSettings}
          savedSettings={secondSettings}
          capabilities={capabilities}
          pending={false}
          onChange={vi.fn()}
          onAdopt={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect((screen.getByTestId(ADOPTION_PATH_TEST_ID) as HTMLInputElement).value).toBe(
      "/var/cache/second",
    );
  });

  it("preserves an external cache path being edited during a persisted update", () => {
    const firstSettings = {
      ...settings,
      go_cache: { ...settings.go_cache, adopted_path: "/var/cache/first" },
    };
    const secondSettings = {
      ...settings,
      go_cache: { ...settings.go_cache, adopted_path: "/var/cache/second" },
    };
    const { rerender } = render(
      <TooltipProvider>
        <StoragePolicyCard
          settings={firstSettings}
          savedSettings={firstSettings}
          capabilities={capabilities}
          pending={false}
          onChange={vi.fn()}
          onAdopt={vi.fn()}
        />
      </TooltipProvider>,
    );
    fireEvent.change(screen.getByTestId(ADOPTION_PATH_TEST_ID), {
      target: { value: "/var/cache/draft" },
    });

    rerender(
      <TooltipProvider>
        <StoragePolicyCard
          settings={secondSettings}
          savedSettings={secondSettings}
          capabilities={capabilities}
          pending={false}
          onChange={vi.fn()}
          onAdopt={vi.fn()}
        />
      </TooltipProvider>,
    );

    expect((screen.getByTestId(ADOPTION_PATH_TEST_ID) as HTMLInputElement).value).toBe(
      "/var/cache/draft",
    );
  });
});

describe("StoragePolicyCard", () => {
  it("shows the dependency allowlist and keeps cleanup opt-in", () => {
    const onChange = renderCard();
    const allowlist = screen.getByTestId("storage-dependency-allowlist");
    const expectedDirectories = [
      "node_modules",
      "bower_components",
      ".pnpm-store",
      ".yarn/cache",
      ".yarn/unplugged",
      ".venv",
      "venv",
      ".tox",
      ".nox",
      "__pypackages__",
      "Pods",
      ".gradle",
    ];

    expect(
      within(allowlist)
        .getAllByRole("listitem")
        .map((item) => item.textContent?.trim()),
    ).toEqual(expectedDirectories);
    expect(allowlist.textContent).not.toContain("vendor");

    const toggle = screen.getByTestId(
      "storage-workspace-dependencies-enabled",
    ) as HTMLButtonElement;
    expect(toggle.getAttribute("aria-checked")).toBe("false");
    fireEvent.click(toggle);
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      workspaces: { ...settings.workspaces, dependency_cleanup_enabled: true },
    });
  });

  it("keeps dependency cleanup enabled when orphan cleanup is toggled", () => {
    const currentSettings = {
      ...settings,
      workspaces: { enabled: true, dependency_cleanup_enabled: true },
    };
    const onChange = renderCard(false, vi.fn(), currentSettings, currentSettings);

    fireEvent.click(screen.getByLabelText("Clean orphan task workspaces"));

    expect(onChange).toHaveBeenLastCalledWith({
      ...currentSettings,
      workspaces: { ...currentSettings.workspaces, enabled: false },
    });
  });

  it("marks dependency cleanup changes on the workspace policy card", () => {
    const currentSettings = {
      ...settings,
      workspaces: { enabled: true, dependency_cleanup_enabled: true },
    };
    renderCard(false, vi.fn(), currentSettings, settings);

    expect(
      screen.getByTestId("storage-policy-section-workspaces").getAttribute(dirtyAttribute),
    ).toBe("true");
    expect(
      screen.getByTestId("storage-workspace-dependencies-enabled").getAttribute(dirtyAttribute),
    ).toBe("true");
  });

  it("edits every Docker cleanup threshold", () => {
    const onChange = renderCard();

    expect((screen.getByTestId(testIds.goCacheMax) as HTMLInputElement).value).toBe("15");
    expect((screen.getByTestId(testIds.dockerBuildCacheKeep) as HTMLInputElement).value).toBe("10");

    fireEvent.change(screen.getByTestId(testIds.goCacheMax), {
      target: { value: "20" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      go_cache: { ...settings.go_cache, max_bytes: 21_474_836_480 },
    });

    fireEvent.change(screen.getByTestId(testIds.dockerBuildCacheKeep), {
      target: { value: "2" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      docker: { ...settings.docker, build_cache_keep_bytes: 2147483648 },
    });

    fireEvent.change(screen.getByTestId(testIds.dockerBuildCacheUnused), {
      target: { value: "72" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      docker: { ...settings.docker, build_cache_unused_hours: 72 },
    });

    fireEvent.change(screen.getByTestId(testIds.dockerImagesUnused), {
      target: { value: "96" },
    });
    expect(onChange).toHaveBeenLastCalledWith({
      ...settings,
      docker: { ...settings.docker, unused_images_hours: 96 },
    });
  });
});

describe("StoragePolicyCard dependency action", () => {
  it("runs dependency cleanup only when the saved opt-in is enabled", () => {
    const onCleanDependencies = vi.fn();
    const enabledSettings = {
      ...settings,
      workspaces: { enabled: true, dependency_cleanup_enabled: true },
    };
    renderCard(false, vi.fn(), enabledSettings, enabledSettings, onCleanDependencies);

    const button = screen.getByTestId("storage-clean-workspace-dependencies") as HTMLButtonElement;
    expect(button.disabled).toBe(false);
    fireEvent.click(button);
    expect(onCleanDependencies).toHaveBeenCalledTimes(1);
  });

  it("disables dependency cleanup while opt-in changes are unsaved or pending", () => {
    const onCleanDependencies = vi.fn();
    const enabledSettings = {
      ...settings,
      workspaces: { enabled: true, dependency_cleanup_enabled: true },
    };
    renderCard(false, vi.fn(), enabledSettings, settings, onCleanDependencies);
    expect(
      (screen.getByTestId("storage-clean-workspace-dependencies") as HTMLButtonElement).disabled,
    ).toBe(true);

    cleanup();
    renderCard(true, vi.fn(), enabledSettings, enabledSettings, onCleanDependencies);
    expect(
      (screen.getByTestId("storage-clean-workspace-dependencies") as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(onCleanDependencies).not.toHaveBeenCalled();
  });
});

describe("StoragePolicyCard interactions", () => {
  it("disables policy controls while an action is pending", () => {
    renderCard(true);

    const pendingTestIds = [
      "storage-scheduling-enabled",
      "storage-go-cache-enabled",
      "storage-check-interval",
      "storage-idle-period",
      "storage-orphan-grace",
      "storage-quarantine-retention",
      testIds.goCacheMax,
      "storage-go-cache-adopt-path",
      ADOPT_BUTTON_TEST_ID,
      "storage-docker-dedicated",
      "storage-docker-build-cache",
      testIds.dockerBuildCacheKeep,
      testIds.dockerBuildCacheUnused,
      "storage-docker-unused-images",
      testIds.dockerImagesUnused,
    ];
    for (const testId of pendingTestIds) {
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

  it("disables child fields when their cleanup option is off", () => {
    renderCard(false, vi.fn(), {
      ...settings,
      enabled: false,
      workspaces: { ...settings.workspaces, enabled: false },
      go_cache: { ...settings.go_cache, enabled: false },
      docker: {
        ...settings.docker,
        build_cache_enabled: false,
        unused_images_enabled: false,
      },
    });

    for (const testId of [
      "storage-check-interval",
      "storage-idle-period",
      "storage-orphan-grace",
      testIds.goCacheMax,
      "storage-go-cache-adopt-path",
      "storage-go-cache-adopt",
      testIds.dockerBuildCacheKeep,
      testIds.dockerBuildCacheUnused,
      testIds.dockerImagesUnused,
    ]) {
      expect((screen.getByTestId(testId) as HTMLButtonElement | HTMLInputElement).disabled).toBe(
        true,
      );
    }
    expect((screen.getByTestId("storage-quarantine-retention") as HTMLInputElement).disabled).toBe(
      false,
    );
  });

  it("renders each maintenance group as a separate card", () => {
    renderCard();

    for (const section of ["schedule", "workspaces", "go-cache", "docker", "quarantine"]) {
      expect(
        screen.getByTestId(`storage-policy-section-${section}`).getAttribute("data-slot"),
      ).toBe("card");
    }
  });

  it("marks the changed field and owning policy card as dirty", () => {
    renderCard(false, vi.fn(), { ...settings, idle_for_minutes: 31 });

    expect(screen.getByTestId("storage-idle-period").getAttribute(dirtyAttribute)).toBe("true");
    expect(screen.getByTestId("storage-policy-section-schedule").getAttribute(dirtyAttribute)).toBe(
      "true",
    );
    expect(screen.getByTestId("storage-policy-section-docker").getAttribute(dirtyAttribute)).toBe(
      "false",
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
    expect(screen.getAllByLabelText(/^More information about /)).toHaveLength(18);
  });
});

// The adoption button picks one of three reasons and the managed-cache row
// interpolates a path. Both are now catalog lookups rather than literals, so a
// missing key or a renamed placeholder degrades to a raw key or a dropped path
// rather than failing anything else.
describe("Go cache section copy", () => {
  const enabledGoCache = { ...settings, go_cache: { ...settings.go_cache, enabled: true } };

  /**
   * The reason renders inside a Radix tooltip, which jsdom will not open from a
   * synthetic mouse event — focus the wrapper span the disabled button sits in,
   * as apps/web/CLAUDE.md prescribes for this shape.
   */
  async function adoptionDisabledReason(): Promise<string> {
    const trigger = screen.getByTestId(ADOPT_BUTTON_TEST_ID).parentElement;
    if (!trigger) throw new Error("adopt button has no tooltip trigger");
    fireEvent.focus(trigger);
    const tooltip = await screen.findAllByRole("tooltip");
    return tooltip[0]?.textContent ?? "";
  }

  it("explains each reason adoption is unavailable", async () => {
    renderCard(true, vi.fn(), enabledGoCache, enabledGoCache);
    expect(await adoptionDisabledReason()).toBe("Wait for the current storage action to finish.");

    cleanup();
    renderCard();
    expect(await adoptionDisabledReason()).toBe("Enable the managed Go cache first.");

    cleanup();
    renderCard(false, vi.fn(), enabledGoCache, enabledGoCache);
    expect(await adoptionDisabledReason()).toBe("Enter an absolute cache path first.");

    // A whitespace-only path is still no path.
    fireEvent.change(screen.getByTestId(ADOPTION_PATH_TEST_ID), { target: { value: "   " } });
    expect(await adoptionDisabledReason()).toBe("Enter an absolute cache path first.");

    fireEvent.change(screen.getByTestId(ADOPTION_PATH_TEST_ID), { target: { value: "/tmp/go" } });
    expect((screen.getByTestId(ADOPT_BUTTON_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
  });

  it("interpolates the managed cache path rather than concatenating it", () => {
    renderCard();
    expect(
      screen.getByText(`New host-local executions use ${capabilities.managed_go_cache_path}.`),
    ).toBeTruthy();
  });
});
