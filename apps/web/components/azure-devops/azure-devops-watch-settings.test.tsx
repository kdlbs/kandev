import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/lib/i18n";
import type {
  AzureDevOpsPullRequestWatch,
  AzureDevOpsWorkItemWatch,
} from "@/lib/types/azure-devops";

const PR_CARD = "azure-pull-request-watch-pr-1";
const WI_CARD = "azure-work-item-watch-wi-1";

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

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("en");
});

describe("AzureDevOpsWatchSettings", () => {
  it("renders the watch summary", () => {
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);

    const prCard = screen.getByTestId(PR_CARD);
    expect(prCard.textContent).toContain("Status: completed");
    expect(prCard.textContent).toContain("project-1 · every 300s · cleanup auto");
    expect(prCard.textContent).toContain("Not checked yet");

    const wiCard = screen.getByTestId(WI_CARD);
    expect(wiCard.textContent).toContain("project-1 · every 600s · cleanup never");
  });

  // `status` and `cleanupPolicy` are Azure DevOps wire enums whose values happen
  // to equal their own English labels, so an English assertion cannot tell a
  // catalog lookup from raw interpolation — it passes either way. Only a
  // non-English locale distinguishes them, which is the whole point of the
  // pseudo-locale.
  it("resolves both wire enums through the catalog rather than interpolating them raw", async () => {
    await i18n.changeLanguage("pseudo");
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);

    const prCard = screen.getByTestId(PR_CARD);
    expect(prCard.textContent).toContain("Śţàţũś: ćōḿƥĺēţēď");
    expect(prCard.textContent).toContain("project-1 · ēvēŕŷ 300ś · ćĺēàńũƥ àũţō");
    // The wire values must not leak through anywhere on the card.
    expect(prCard.textContent).not.toContain("completed");
    expect(prCard.textContent).not.toContain("auto");

    const wiCard = screen.getByTestId(WI_CARD);
    expect(wiCard.textContent).toContain("ćĺēàńũƥ ńēvēŕ");
    expect(wiCard.textContent).not.toContain("never");
    // `projectId` is Azure DevOps data and the WIQL is a query language; both
    // stay verbatim under pseudo.
    expect(wiCard.textContent).toContain("project-1");
    expect(wiCard.textContent).toContain("SELECT [System.Id] FROM WorkItems");
  });

  it("treats an `all` status filter the same as no status filter", async () => {
    mocks.watches.mockReturnValue(state({ status: "all" }));
    await i18n.changeLanguage("pseudo");
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);
    expect(screen.getByTestId(PR_CARD).textContent).toContain("Śţàţũś: àńŷ");
  });

  it("echoes an unrecognized status rather than blanking it", async () => {
    mocks.watches.mockReturnValue(
      state({ status: "draft" as AzureDevOpsPullRequestWatch["status"] }),
    );
    await i18n.changeLanguage("pseudo");
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);
    expect(screen.getByTestId(PR_CARD).textContent).toContain("Śţàţũś: draft");
  });
});
