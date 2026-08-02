import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  AzureDevOpsPullRequestWatch,
  AzureDevOpsWorkItemWatch,
} from "@/lib/types/azure-devops";

const mocks = vi.hoisted(() => ({ watches: vi.fn() }));

vi.mock("@/hooks/domains/azure-devops/use-azure-devops-watches", () => ({
  useAzureDevOpsWatches: mocks.watches,
}));

import { AzureDevOpsWatchSettings } from "@/components/azure-devops/azure-devops-watch-settings";

const PULL_REQUEST: AzureDevOpsPullRequestWatch = {
  id: "pr-1",
  projectId: "project-1",
  status: "completed",
  enabled: true,
  pollIntervalSeconds: 300,
  cleanupPolicy: "auto",
} as AzureDevOpsPullRequestWatch;

const WORK_ITEM: AzureDevOpsWorkItemWatch = {
  id: "wi-1",
  projectId: "project-1",
  wiql: "SELECT [System.Id] FROM WorkItems",
  enabled: false,
  pollIntervalSeconds: 600,
  cleanupPolicy: "never",
} as AzureDevOpsWorkItemWatch;

function state(overrides: Partial<AzureDevOpsPullRequestWatch> = {}) {
  return {
    workItems: [WORK_ITEM],
    pullRequests: [{ ...PULL_REQUEST, ...overrides }],
    loading: false,
    error: null,
    refresh: vi.fn(),
    createWorkItem: vi.fn(),
    createPullRequest: vi.fn(),
    updateWorkItem: vi.fn(),
    updatePullRequest: vi.fn(),
    remove: vi.fn(),
    trigger: vi.fn(),
    previewReset: vi.fn(),
    reset: vi.fn(),
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.watches.mockReturnValue(state());
});

afterEach(cleanup);

describe("AzureDevOpsWatchSettings", () => {
  // The status and cleanup-policy values are Azure DevOps wire enums. They must
  // reach the summary through the catalog, not be interpolated raw — otherwise a
  // non-English locale still renders plain `completed` / `auto` on a migrated page.
  it("renders wire enums through localized labels", () => {
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);

    const prCard = screen.getByTestId("azure-pull-request-watch-pr-1");
    expect(prCard.textContent).toContain("Status: completed");
    expect(prCard.textContent).toContain("project-1 · every 300s · cleanup auto");
    expect(prCard.textContent).toContain("Not checked yet");

    const wiCard = screen.getByTestId("azure-work-item-watch-wi-1");
    expect(wiCard.textContent).toContain("project-1 · every 600s · cleanup never");
  });

  it("treats an `all` status filter the same as no status filter", () => {
    mocks.watches.mockReturnValue(state({ status: "all" }));
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);
    expect(screen.getByTestId("azure-pull-request-watch-pr-1").textContent).toContain(
      "Status: any",
    );
  });

  it("echoes an unrecognized status rather than blanking it", () => {
    mocks.watches.mockReturnValue(
      state({ status: "draft" as AzureDevOpsPullRequestWatch["status"] }),
    );
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);
    expect(screen.getByTestId("azure-pull-request-watch-pr-1").textContent).toContain(
      "Status: draft",
    );
  });
});
