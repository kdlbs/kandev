import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ImproveKandevDialog } from "./improve-kandev-dialog";
import { IMPROVE_KANDEV_WORKSPACE_NAME } from "./improve-kandev-dialog-model";
import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";

const ACTIVE_WORKSPACE = { id: "ws-active", name: "Active Workspace" };
const IMPROVE_WORKSPACE = { id: "ws-improve", name: IMPROVE_KANDEV_WORKSPACE_NAME };
const WORKSPACE_CHOICE_CONFIRM_TESTID = "improve-kandev-create-workspace-confirm";

const mocks = vi.hoisted(() => ({
  bootstrap: vi.fn(),
  listRepositories: vi.fn(),
  listWorkflowSteps: vi.fn(),
  health: vi.fn(),
  toast: vi.fn(),
  setRepositories: vi.fn(),
}));

const storeState = {
  workspaces: { items: [ACTIVE_WORKSPACE] as Array<{ id: string; name: string }> },
  setRepositories: mocks.setRepositories,
};

function setStoreWorkspaces(items: Array<{ id: string; name: string }>) {
  storeState.workspaces.items = items;
}

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (s: typeof storeState) => unknown) => selector(storeState),
}));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));
vi.mock("@/components/routing/app-link", () => ({
  default: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
}));
vi.mock("@/lib/api/domains/improve-kandev-api", () => ({
  bootstrapImproveKandev: mocks.bootstrap,
}));
vi.mock("@/lib/api/domains/workspace-api", () => ({
  listRepositories: mocks.listRepositories,
}));
vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: mocks.listWorkflowSteps,
}));
vi.mock("@/lib/api/domains/health-api", () => ({
  fetchSystemHealth: mocks.health,
}));
vi.mock("./improve-kandev-dialog-create", () => ({
  CreateModeView: (props: { workspaceId: string | null; bootstrap: { kind: string } }) => (
    <div
      data-testid="create-mode-view"
      data-workspace={props.workspaceId ?? ""}
      data-bootstrap={props.bootstrap.kind}
    >
      create view
    </div>
  ),
}));

const bootstrapResponse: ImproveKandevBootstrapResponse = {
  workspace_id: IMPROVE_WORKSPACE.id,
  repository_id: "r1",
  workflow_id: "w1",
  issue_workflow_id: "w2",
  branch: "main",
  bundle_dir: "/tmp/bundle",
  bundle_file: "/tmp/bundle/diagnostic-bundle.zip",
  github_login: "octocat",
  has_write_access: false,
  fork_status: "unknown",
};

function renderDialog() {
  return render(<ImproveKandevDialog open onOpenChange={() => {}} workspaceId="ws-active" />);
}

beforeEach(() => {
  window.localStorage.clear();
  setStoreWorkspaces([ACTIVE_WORKSPACE]);
  mocks.bootstrap.mockReset();
  mocks.listRepositories.mockReset();
  mocks.listWorkflowSteps.mockReset();
  mocks.health.mockReset();
  mocks.toast.mockReset();
  mocks.setRepositories.mockReset();
  mocks.health.mockResolvedValue({ issues: [] });
  mocks.bootstrap.mockResolvedValue(bootstrapResponse);
  mocks.listWorkflowSteps.mockResolvedValue({ steps: [] });
  mocks.listRepositories.mockResolvedValue({ repositories: [{ id: "r1", name: "kandev" }] });
});

afterEach(() => cleanup());

describe("ImproveKandevDialog bootstrap workspace wiring", () => {
  it("bootstraps with the dedicated workspace and lists repositories for the returned workspace", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE, IMPROVE_WORKSPACE]);
    // Skip-intro so the dialog opens directly in create mode, where
    // bootstrap runs (the workspace exists, so there is no choice gate).
    window.localStorage.setItem("kandev.improveKandev.skipIntro", "true");
    renderDialog();

    await waitFor(() => expect(mocks.bootstrap).toHaveBeenCalled());
    expect(mocks.bootstrap).toHaveBeenCalledWith("ws-active", { createWorkspace: true });
    await waitFor(() =>
      expect(mocks.listRepositories).toHaveBeenCalledWith(IMPROVE_WORKSPACE.id, undefined, {
        cache: "no-store",
      }),
    );
    await waitFor(() =>
      expect(mocks.setRepositories).toHaveBeenCalledWith(IMPROVE_WORKSPACE.id, [
        { id: "r1", name: "kandev" },
      ]),
    );
    // The dialog hands the active workspace to CreateModeView; the create view
    // itself switches to the bootstrap's workspace_id once ready (covered by
    // the e2e isolation test).
    await waitFor(() =>
      expect(screen.getByTestId("create-mode-view").dataset.workspace).toBe("ws-active"),
    );
  });

  it("shows the workspace-creation choice when the dedicated workspace is missing and defers bootstrap", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE]);
    window.localStorage.setItem("kandev.improveKandev.skipIntro", "true");
    renderDialog();

    expect(mocks.bootstrap).not.toHaveBeenCalled();
    const confirm = screen.getByTestId(WORKSPACE_CHOICE_CONFIRM_TESTID);
    expect(confirm).toBeTruthy();

    act(() => fireEvent.click(confirm));
    await waitFor(() => expect(mocks.bootstrap).toHaveBeenCalledTimes(1));
    expect(mocks.bootstrap).toHaveBeenCalledWith("ws-active", { createWorkspace: true });
  });

  it("passes create_workspace=false when the user declines workspace creation", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE]);
    window.localStorage.setItem("kandev.improveKandev.skipIntro", "true");
    renderDialog();

    const choice = screen.getByTestId("improve-kandev-create-workspace");
    const checkbox = choice.querySelector('[role="checkbox"]');
    expect(checkbox).toBeTruthy();
    act(() => fireEvent.click(checkbox as Element));

    act(() => fireEvent.click(screen.getByTestId(WORKSPACE_CHOICE_CONFIRM_TESTID)));
    await waitFor(() => expect(mocks.bootstrap).toHaveBeenCalledTimes(1));
    expect(mocks.bootstrap).toHaveBeenCalledWith("ws-active", { createWorkspace: false });
  });

  it("offers the workspace-creation checkbox in the intro when the workspace is missing", () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE]);
    renderDialog();

    expect(screen.getByTestId("improve-kandev-create-workspace")).toBeTruthy();
  });
});

import { Trans } from "react-i18next";
describe("improve-kandev dialog <Trans> copy", () => {
  it("renders the gh-auth notice byte-identically to the old literal", () => {
    const { container } = render(
      <Trans
        i18nKey="common:theFinalStepOpensAPullRequest"
        values={{ binary: "gh", message: "Run gh auth login." }}
      >
        The final step of this workflow opens a pull request, which needs the <code>gh</code> CLI to
        be authenticated. {"Run gh auth login."}
      </Trans>,
    );

    expect(container.textContent).toBe(
      "The final step of this workflow opens a pull request, which needs the gh CLI to be " +
        "authenticated. Run gh auth login.",
    );
    expect(container.querySelector("code")?.textContent).toBe("gh");
  });

  it("renders the fork bullet with the repo slug intact", () => {
    const { container } = render(
      <Trans i18nKey="common:theAgentForksKandevToYourAccount" values={{ repo: "kdlbs/kandev" }}>
        The agent forks <code>kdlbs/kandev</code> to your GitHub account and opens a PR from your
        fork, credited to you
      </Trans>,
    );

    expect(container.textContent).toBe(
      "The agent forks kdlbs/kandev to your GitHub account and opens a PR from your fork, " +
        "credited to you",
    );
    expect(container.querySelector("code")?.textContent).toBe("kdlbs/kandev");
  });

  it("keeps the contributor line one sentence per branch", () => {
    const { container } = render(
      <Trans
        i18nKey="common:contributingAsLogin"
        values={{ login: "octocat", access: "You have write access." }}
      >
        Contributing as <code>@octocat</code>. {"You have write access."}
      </Trans>,
    );

    expect(container.textContent).toBe("Contributing as @octocat. You have write access.");
  });
});
