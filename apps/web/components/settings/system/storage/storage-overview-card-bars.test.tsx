import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { StorageOverviewResponse } from "@/lib/types/system";
import { StorageOverviewCard } from "./storage-overview-card";

const SYSTEM_TEMPORARY_RESOURCE_TEST_ID = "storage-resource-system-temporary";
const SYSTEM_TEMPORARY_TRIGGER_TEST_ID = `${SYSTEM_TEMPORARY_RESOURCE_TEST_ID}-trigger`;
const WORKSPACES_RESOURCE_TEST_ID = "storage-resource-workspaces";
const WORKSPACES_TRIGGER_TEST_ID = `${WORKSPACES_RESOURCE_TEST_ID}-trigger`;
const WORKSPACES_BAR_TEST_ID = `${WORKSPACES_RESOURCE_TEST_ID}-bar`;
const WORKSPACES_BAR_FILL_TEST_ID = `${WORKSPACES_RESOURCE_TEST_ID}-bar-fill`;
const SYSTEM_TEMPORARY_BAR_FILL_TEST_ID = `${SYSTEM_TEMPORARY_RESOURCE_TEST_ID}-bar-fill`;
const TEMPORARY_ARTIFACTS_BAR_TEST_ID = "storage-resource-temporary-artifacts-bar";
const GO_CACHE_TRIGGER_TEST_ID = "storage-resource-go-cache-trigger";
const GO_CACHE_CLEAN_TEST_ID = "storage-go-cache-clean";
const EXPANDED_ATTRIBUTE = "aria-expanded";
const EXPANDED_ATTRIBUTE_VALUE = "true";
const ACCORDION_CONTENT_SELECTOR = '[data-slot="accordion-content"]';

function expectExpandedContent(resourceTestId: string) {
  const content = screen.getByTestId(resourceTestId).querySelector(ACCORDION_CONTENT_SELECTOR);
  expect(content).not.toBeNull();
  expect(content?.getAttribute("data-state")).toBe("open");
  expect(content?.hasAttribute("hidden")).toBe(false);
}

const barsOverview = {
  settings: {
    enabled: false,
    check_interval_hours: 24,
    idle_for_minutes: 10,
    orphan_grace_hours: 168,
    quarantine_retention_hours: 168,
    workspaces: { enabled: true, dependency_cleanup_enabled: false },
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
    docker_networks: {
      enabled: false,
      stale_hours: 168,
      quarantine_hours: 24,
      orphan_grace_hours: 1,
      probe_enabled: true,
    },
  },
  capabilities: {
    managed_go_cache_path: "/data/cache/go-build",
    go_cache_adoption_available: true,
    temporary_artifacts_available: false,
    docker_available: false,
    docker_host: "",
    host_global_docker_cleanup_allowed: false,
  },
  summary: {
    workspaces: { total_bytes: 8 * 1024 ** 3, active_bytes: 0, candidate_bytes: 0 },
    go_cache: { path: "/data/cache/go-build", size_bytes: 0, owned: true, enabled: false },
    quarantine: { available: true, count: 0, size_bytes: 2 * 1024 ** 3 },
    temporary_artifacts: { available: false, warning: "registry unavailable" },
    system_temporary: {
      status: "partial",
      size_bytes: 4 * 1024 ** 3,
      included_in_total: false,
      roots: [],
    },
    docker: {
      available: false,
      build_cache_bytes: 0,
      unused_image_bytes: 0,
      managed_container_count: 0,
      managed_container_bytes: 0,
    },
  },
  analysis: {
    generation: 1,
    state: "ready",
    started_at: "2026-07-23T11:59:00Z",
    completed_at: "2026-07-23T12:00:00Z",
    duration_ms: 100,
    cache_ttl_seconds: 900,
    refresh_due_at: "2099-07-23T12:15:00Z",
    stale: false,
    error: null,
    partial_summary: null,
    progress: { completed_sources: 8, total_sources: 8, sources: {} },
  },
  analyzed_at: null,
  last_run: null,
} satisfies StorageOverviewResponse;

afterEach(cleanup);

// eslint-disable-next-line max-lines-per-function -- related bar and focus regressions share one fixture.
describe("StorageOverviewCard relative bars", () => {
  it("renders decorative relative bars and visible partial status", () => {
    render(<StorageOverviewCard overview={barsOverview} onRunGoCache={vi.fn()} />);

    expect(screen.getByTestId("storage-analysis-bars-description").textContent).toContain(
      "Bars compare category sizes",
    );
    expect(screen.getByTestId(WORKSPACES_BAR_TEST_ID).getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByTestId(WORKSPACES_BAR_TEST_ID).getAttribute("role")).toBeNull();
    expect(screen.getByTestId(WORKSPACES_BAR_FILL_TEST_ID).getAttribute("style")).toBe(
      "width: 100%;",
    );
    const partialBadge = screen.getByTestId(`${SYSTEM_TEMPORARY_RESOURCE_TEST_ID}-partial`);
    const systemTemporaryTitle = screen.getByTestId(`${SYSTEM_TEMPORARY_RESOURCE_TEST_ID}-title`);
    const systemTemporaryValue = screen.getByTestId("storage-analysis-source-system_temporary");
    expect(systemTemporaryTitle.contains(partialBadge)).toBe(true);
    expect(systemTemporaryValue.contains(partialBadge)).toBe(false);
    expect(screen.getByTestId(SYSTEM_TEMPORARY_BAR_FILL_TEST_ID).getAttribute("style")).toBe(
      "width: 50%;",
    );
    expect(screen.queryByTestId(TEMPORARY_ARTIFACTS_BAR_TEST_ID)).toBeNull();
  });

  it("keeps expanded rows and focus attached to stable resource IDs after reordering", () => {
    const { rerender } = render(
      <StorageOverviewCard overview={barsOverview} onRunGoCache={vi.fn()} />,
    );
    const systemTemporaryTrigger = screen.getByTestId(SYSTEM_TEMPORARY_TRIGGER_TEST_ID);
    const workspacesTrigger = screen.getByTestId(WORKSPACES_TRIGGER_TEST_ID);
    fireEvent.click(systemTemporaryTrigger);
    fireEvent.click(workspacesTrigger);
    systemTemporaryTrigger.focus();
    expect(systemTemporaryTrigger.getAttribute(EXPANDED_ATTRIBUTE)).toBe(EXPANDED_ATTRIBUTE_VALUE);
    expect(workspacesTrigger.getAttribute(EXPANDED_ATTRIBUTE)).toBe(EXPANDED_ATTRIBUTE_VALUE);
    expectExpandedContent(SYSTEM_TEMPORARY_RESOURCE_TEST_ID);
    expectExpandedContent(WORKSPACES_RESOURCE_TEST_ID);

    rerender(
      <StorageOverviewCard
        overview={{
          ...barsOverview,
          summary: {
            ...barsOverview.summary,
            workspaces: { total_bytes: 1 * 1024 ** 3, active_bytes: 0, candidate_bytes: 0 },
            system_temporary: {
              ...barsOverview.summary.system_temporary,
              size_bytes: 16 * 1024 ** 3,
            },
          },
        }}
        onRunGoCache={vi.fn()}
      />,
    );

    expect(
      screen.getByTestId(SYSTEM_TEMPORARY_TRIGGER_TEST_ID).getAttribute(EXPANDED_ATTRIBUTE),
    ).toBe(EXPANDED_ATTRIBUTE_VALUE);
    expect(screen.getByTestId(WORKSPACES_TRIGGER_TEST_ID).getAttribute(EXPANDED_ATTRIBUTE)).toBe(
      EXPANDED_ATTRIBUTE_VALUE,
    );
    expectExpandedContent(SYSTEM_TEMPORARY_RESOURCE_TEST_ID);
    expectExpandedContent(WORKSPACES_RESOURCE_TEST_ID);
    expect(document.activeElement).toBe(screen.getByTestId(SYSTEM_TEMPORARY_TRIGGER_TEST_ID));
  });

  it("keeps a focused expanded-row action attached after its row moves", () => {
    const actionOverview = {
      ...barsOverview,
      summary: {
        ...barsOverview.summary,
        go_cache: {
          ...barsOverview.summary.go_cache,
          size_bytes: 16 * 1024 ** 3,
          owned: true,
        },
      },
    } satisfies StorageOverviewResponse;
    const { rerender } = render(
      <StorageOverviewCard overview={actionOverview} onRunGoCache={vi.fn()} />,
      { wrapper: TooltipProvider },
    );

    const goCacheTrigger = screen.getByTestId(GO_CACHE_TRIGGER_TEST_ID);
    fireEvent.click(goCacheTrigger);
    const cleanButton = screen.getByTestId(GO_CACHE_CLEAN_TEST_ID);
    cleanButton.focus();
    expect(goCacheTrigger.getAttribute(EXPANDED_ATTRIBUTE)).toBe(EXPANDED_ATTRIBUTE_VALUE);
    expect(document.activeElement).toBe(cleanButton);

    rerender(
      <StorageOverviewCard
        overview={{
          ...actionOverview,
          summary: {
            ...actionOverview.summary,
            go_cache: {
              ...actionOverview.summary.go_cache,
              size_bytes: 1,
              warning: "cache measurement warning",
            },
            system_temporary: {
              ...actionOverview.summary.system_temporary,
              size_bytes: 20 * 1024 ** 3,
            },
          },
        }}
        onRunGoCache={vi.fn()}
      />,
    );

    expect(screen.getByTestId(GO_CACHE_TRIGGER_TEST_ID).getAttribute(EXPANDED_ATTRIBUTE)).toBe(
      EXPANDED_ATTRIBUTE_VALUE,
    );
    const disabledCleanButton = screen.getByTestId(GO_CACHE_CLEAN_TEST_ID);
    expect(disabledCleanButton.parentElement?.getAttribute("tabindex")).toBe("0");
    expect(document.activeElement).toBe(disabledCleanButton.parentElement);
  });
});
