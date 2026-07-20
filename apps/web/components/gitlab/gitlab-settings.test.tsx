import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { GitLabIntegrationPage } from "./gitlab-settings";
import type { GitLabActionPresets, ReviewWatch } from "@/lib/types/gitlab";

const fetchGitLabStatusMock = vi.fn();
const setGitLabConfigMock = vi.fn();
const resetActionPresetsMock = vi.fn();
const updateActionPresetsMock = vi.fn();
const deleteReviewWatchMock = vi.fn();
const workspaceId = "workspace-1";
const defaultHost = "https://gitlab.com";
const appState = {
  gitlabReviewWatches: { items: [] as ReviewWatch[], loaded: false, loading: false },
  gitlabIssueWatches: { items: [], loaded: false, loading: false },
  gitlabActionPresets: {
    byWorkspaceId: {} as Record<string, GitLabActionPresets>,
    loading: false,
  },
  setGitLabReviewWatches: vi.fn(),
  setGitLabReviewWatchesLoading: vi.fn(),
  addGitLabReviewWatch: vi.fn(),
  updateGitLabReviewWatchInStore: vi.fn(),
  removeGitLabReviewWatch: vi.fn(),
  setGitLabIssueWatches: vi.fn(),
  setGitLabIssueWatchesLoading: vi.fn(),
  addGitLabIssueWatch: vi.fn(),
  updateGitLabIssueWatchInStore: vi.fn(),
  removeGitLabIssueWatch: vi.fn(),
  setGitLabActionPresets: vi.fn(),
  setGitLabActionPresetsLoading: vi.fn(),
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: typeof appState) => unknown) => selector(appState),
}));

vi.mock("@/components/integrations/workspace-scoped-section", () => ({
  WorkspaceScopedSection: ({ children }: { children: (workspaceId: string) => React.ReactNode }) =>
    children(workspaceId),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: vi.fn(),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => ({
  clearGitLabToken: vi.fn(),
  configureGitLabHost: vi.fn(),
  setGitLabConfig: (...args: unknown[]) => setGitLabConfigMock(...args),
  fetchGitLabStatus: (...args: unknown[]) => fetchGitLabStatusMock(...args),
  listReviewWatches: vi.fn().mockResolvedValue({ watches: [] }),
  listIssueWatches: vi.fn().mockResolvedValue({ watches: [] }),
  createReviewWatch: vi.fn(),
  updateReviewWatch: vi.fn(),
  deleteReviewWatch: (...args: unknown[]) => deleteReviewWatchMock(...args),
  triggerReviewWatch: vi.fn(),
  triggerAllReviewWatches: vi.fn(),
  previewResetReviewWatch: vi.fn(),
  resetReviewWatch: vi.fn(),
  createIssueWatch: vi.fn(),
  updateIssueWatch: vi.fn(),
  deleteIssueWatch: vi.fn(),
  triggerIssueWatch: vi.fn(),
  triggerAllIssueWatches: vi.fn(),
  previewResetIssueWatch: vi.fn(),
  resetIssueWatch: vi.fn(),
  getActionPresets: vi.fn().mockResolvedValue({ workspace_id: "workspace-1", mr: [], issue: [] }),
  updateActionPresets: (...args: unknown[]) => updateActionPresetsMock(...args),
  resetActionPresets: (...args: unknown[]) => resetActionPresetsMock(...args),
}));

afterEach(() => {
  cleanup();
  fetchGitLabStatusMock.mockReset();
  setGitLabConfigMock.mockReset().mockResolvedValue({
    workspace_id: workspaceId,
    host: defaultHost,
    auth_method: "glab_cli",
  });
  deleteReviewWatchMock.mockReset().mockResolvedValue({ deleted: true });
  resetActionPresetsMock.mockReset().mockResolvedValue({
    workspace_id: workspaceId,
    mr: [],
    issue: [],
  });
  updateActionPresetsMock
    .mockReset()
    .mockImplementation((_workspaceId, body) =>
      Promise.resolve({ workspace_id: workspaceId, mr: body.mr ?? [], issue: body.issue ?? [] }),
    );
  appState.gitlabReviewWatches.items = [];
  appState.gitlabActionPresets.byWorkspaceId = {};
  vi.mocked(useSettingsSaveContributor).mockClear();
});

describe("GitLabIntegrationPage", () => {
  it("marks the changed host and its owning card dirty", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: false,
      auth_method: "none",
      connection_error: "",
      glab_version: "",
      host: defaultHost,
      token_configured: false,
      username: "",
    });

    render(<GitLabIntegrationPage workspaceId={workspaceId} />);

    const host = await screen.findByDisplayValue(defaultHost);
    expect(screen.getByText("Merge request review watches")).toBeTruthy();
    expect(screen.getByText("Issue watches")).toBeTruthy();
    expect(fetchGitLabStatusMock).toHaveBeenCalledWith({
      cache: "no-store",
      workspaceId,
    });
    const card = host.closest('[data-slot="card"]');
    expect(card?.getAttribute("data-settings-dirty")).toBe("false");

    fireEvent.change(host, { target: { value: "https://gitlab.example.com" } });

    expect(host.getAttribute("data-settings-dirty")).toBe("true");
    await waitFor(() => expect(card?.getAttribute("data-settings-dirty")).toBe("true"));
  });

  it("submits a fresh self-managed host and PAT together", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: false,
      auth_method: "none",
      connection_error: "",
      host: defaultHost,
      token_configured: false,
      username: "",
    });
    render(<GitLabIntegrationPage workspaceId={workspaceId} />);

    fireEvent.change(await screen.findByDisplayValue(defaultHost), {
      target: { value: "http://gitlab.internal" },
    });
    fireEvent.change(screen.getByPlaceholderText("glpat-xxxxxxxxxxxxxxxxxxxx"), {
      target: { value: "glpat-fresh" },
    });

    const tokenContributor = vi
      .mocked(useSettingsSaveContributor)
      .mock.calls.map(([contributor]) => contributor)
      .reverse()
      .find((contributor) => contributor.id === "gitlab-credentials");
    if (tokenContributor) await tokenContributor.save(tokenContributor.revision);

    expect(setGitLabConfigMock).toHaveBeenCalledWith(
      {
        host: "http://gitlab.internal",
        auth_method: "pat",
        token: "glpat-fresh",
      },
      { workspaceId },
    );
  });
});

describe("GitLabIntegrationPage workspace actions", () => {
  it("uses an accessible destructive confirmation before deleting a watch", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: true,
      auth_method: "pat",
      connection_error: "",
      host: defaultHost,
      token_configured: true,
      username: "alice",
    });
    appState.gitlabReviewWatches.items = [
      {
        id: "review-1",
        workspace_id: workspaceId,
        workflow_id: "workflow",
        workflow_step_id: "step",
        projects: [],
        agent_profile_id: "",
        executor_profile_id: "",
        prompt: "review",
        review_scope: "user",
        custom_query: "",
        enabled: true,
        poll_interval_seconds: 300,
        cleanup_policy: "auto",
        created_at: "2026-01-01",
        updated_at: "2026-01-01",
      },
    ];
    render(<GitLabIntegrationPage workspaceId={workspaceId} />);

    fireEvent.click((await screen.findAllByRole("button", { name: "Delete watch" }))[0]!);
    expect(screen.getByRole("alertdialog", { name: /delete gitlab review watch/i })).toBeTruthy();
    expect(screen.getByText(/delete every task created by this watch/i)).toBeTruthy();
    expect(deleteReviewWatchMock).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Delete watch" }));
    await waitFor(() =>
      expect(deleteReviewWatchMock).toHaveBeenCalledWith("review-1", workspaceId),
    );
  });

  it("can switch the workspace connection to glab CLI authentication", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: false,
      auth_method: "none",
      connection_error: "",
      host: defaultHost,
      token_configured: false,
      username: "",
    });
    render(<GitLabIntegrationPage workspaceId={workspaceId} />);

    fireEvent.click(await screen.findByRole("combobox", { name: "Authentication method" }));
    fireEvent.click(screen.getByRole("option", { name: "glab CLI" }));
    const contributor = vi
      .mocked(useSettingsSaveContributor)
      .mock.calls.map(([value]) => value)
      .reverse()
      .find((value) => value.id === "gitlab-credentials");
    if (contributor) await contributor.save(contributor.revision);

    expect(setGitLabConfigMock).toHaveBeenCalledWith(
      { host: defaultHost, auth_method: "glab_cli" },
      { workspaceId },
    );
    expect(screen.queryByPlaceholderText("glpat-xxxxxxxxxxxxxxxxxxxx")).toBeNull();
  });

  it("labels configured but rejected credentials as reconnect required", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: false,
      auth_method: "pat",
      connection_error: "",
      host: defaultHost,
      token_configured: true,
      username: "",
    });
    render(<GitLabIntegrationPage workspaceId={workspaceId} />);
    expect(await screen.findByText("Reconnect required")).toBeTruthy();
  });
});

describe("GitLabIntegrationPage quick actions", () => {
  it("exposes workspace quick-action reset controls", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: true,
      auth_method: "pat",
      connection_error: "",
      host: defaultHost,
      token_configured: true,
      username: "alice",
    });
    render(<GitLabIntegrationPage workspaceId={workspaceId} />);
    expect(await screen.findByText("Quick actions")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Reset quick actions to defaults" }));
    await waitFor(() => expect(resetActionPresetsMock).toHaveBeenCalledWith(workspaceId));
  });

  it("updates the workspace quick-action prompt through the shared save control", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: true,
      auth_method: "pat",
      connection_error: "",
      host: defaultHost,
      token_configured: true,
      username: "alice",
    });
    appState.gitlabActionPresets.byWorkspaceId[workspaceId] = {
      workspace_id: workspaceId,
      mr: [
        {
          id: "review",
          label: "Review",
          hint: "Inspect",
          icon: "eye",
          prompt_template: "Review {{url}}",
        },
      ],
      issue: [],
    };
    render(<GitLabIntegrationPage workspaceId={workspaceId} />);
    fireEvent.change(await screen.findByRole("textbox", { name: "Action label 1" }), {
      target: { value: "Deep review" },
    });
    const contributor = vi
      .mocked(useSettingsSaveContributor)
      .mock.calls.map(([value]) => value)
      .reverse()
      .find((value) => value.id === `gitlab-action-presets:${workspaceId}`);
    if (contributor) await contributor.save(contributor.revision);
    expect(updateActionPresetsMock).toHaveBeenCalledWith(
      workspaceId,
      expect.objectContaining({ mr: [expect.objectContaining({ label: "Deep review" })] }),
    );
  });

  it("prevents saving a quick action with a blank label", async () => {
    fetchGitLabStatusMock.mockResolvedValue({
      authenticated: true,
      auth_method: "pat",
      connection_error: "",
      host: defaultHost,
      token_configured: true,
      username: "alice",
    });
    appState.gitlabActionPresets.byWorkspaceId[workspaceId] = {
      workspace_id: workspaceId,
      mr: [
        {
          id: "review",
          label: "Review",
          hint: "Inspect",
          icon: "eye",
          prompt_template: "Review {{url}}",
        },
      ],
      issue: [],
    };
    render(<GitLabIntegrationPage workspaceId={workspaceId} />);
    fireEvent.change(await screen.findByRole("textbox", { name: "Action label 1" }), {
      target: { value: "   " },
    });
    const contributor = vi
      .mocked(useSettingsSaveContributor)
      .mock.calls.map(([value]) => value)
      .reverse()
      .find((value) => value.id === `gitlab-action-presets:${workspaceId}`);
    expect(contributor?.canSave).toBe(false);
    expect(contributor?.invalidReason).toContain("label and prompt");
  });
});
