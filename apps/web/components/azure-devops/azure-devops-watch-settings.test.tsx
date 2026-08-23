import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/lib/i18n";
import type {
  AzureDevOpsPullRequestWatch,
  AzureDevOpsWorkItemWatch,
} from "@/lib/types/azure-devops";

const PR_CARD = "azure-pull-request-watch-pr-1";
const WI_CARD = "azure-work-item-watch-wi-1";

const mocks = vi.hoisted(() => ({ watches: vi.fn(), promptEditor: vi.fn() }));

vi.mock("@/hooks/domains/azure-devops/use-azure-devops-watches", () => ({
  useAzureDevOpsWatches: mocks.watches,
}));
vi.mock("@/components/settings/settings-prompt-editor", () => ({
  SettingsPromptEditor: (props: Record<string, unknown>) => {
    mocks.promptEditor(props);
    return <div data-testid={props.testId as string} />;
  },
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

function state(
  overrides: Partial<AzureDevOpsPullRequestWatch> = {},
  actionOverrides: Record<string, unknown> = {},
) {
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
    ...actionOverrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mocks.watches.mockReturnValue(state());
});

afterEach(async () => {
  cleanup();
  await i18n.changeLanguage("en");
  Object.defineProperty(window, "confirm", { configurable: true, value: undefined });
});

function installNativeConfirm() {
  const nativeConfirm = vi.fn();
  Object.defineProperty(window, "confirm", {
    configurable: true,
    value: nativeConfirm,
  });
  return nativeConfirm;
}

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

  it("confirms watch deletion locally without calling browser confirm", async () => {
    const remove = vi.fn();
    const nativeConfirm = installNativeConfirm();
    mocks.watches.mockReturnValue(state({}, { remove }));
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);

    fireEvent.click(
      within(screen.getByTestId(PR_CARD)).getByTestId("azure-pull-request-watch-delete-pr-1"),
    );
    const confirmation = await screen.findByRole("dialog", { name: "Delete this watch?" });
    expect(nativeConfirm).not.toHaveBeenCalled();
    fireEvent.click(within(confirmation).getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith("pull-request", "pr-1"));
  });

  it("keeps Azure reset in ResetWatchDialog while removing native pre-confirm", async () => {
    const previewReset = vi.fn().mockResolvedValue({ taskCount: 2 });
    const nativeConfirm = installNativeConfirm();
    mocks.watches.mockReturnValue(state({}, { previewReset }));
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);

    fireEvent.click(within(screen.getByTestId(PR_CARD)).getByRole("button", { name: "Reset" }));

    expect(await screen.findByTestId("reset-watch-dialog")).toBeTruthy();
    expect(previewReset).toHaveBeenCalledWith("pull-request", "pr-1");
    expect(nativeConfirm).not.toHaveBeenCalled();
  });

  it("uses typed Azure DevOps placeholders with saved-prompt references", () => {
    render(<AzureDevOpsWatchSettings workspaceId="workspace-a" />);

    fireEvent.click(screen.getByTestId("azure-pull-request-watch-edit-pr-1"));

    expect(mocks.promptEditor).toHaveBeenCalledWith(
      expect.objectContaining({
        promptReferences: true,
        testId: "azure-pull-request-watch-prompt-editor",
        placeholders: expect.arrayContaining([
          expect.objectContaining({ key: "pull_request.url" }),
          expect.objectContaining({ key: "pull_request.target_branch" }),
        ]),
      }),
    );
  });
});
