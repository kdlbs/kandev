import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ImproveKandevDialog } from "./improve-kandev-dialog";
import {
  IMPROVE_KANDEV_SKIP_INTRO_KEY,
  IMPROVE_KANDEV_WORKSPACE_NAME,
} from "./improve-kandev-dialog-model";
import type { ImproveKandevBootstrapResponse } from "@/lib/api/domains/improve-kandev-api";

const ACTIVE_WORKSPACE = { id: "ws-active", name: "Active Workspace" };
const IMPROVE_WORKSPACE = { id: "ws-improve", name: IMPROVE_KANDEV_WORKSPACE_NAME };
const WORKSPACE_CHOICE_CONFIRM_TESTID = "improve-kandev-create-workspace-confirm";
// The health check's own remediation sentence, rendered verbatim after the copy.
const GH_AUTH_MESSAGE = "Run gh auth login.";

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

  it("re-runs bootstrap when the dialog reopens with the dedicated workspace present (skip-intro)", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE, IMPROVE_WORKSPACE]);
    window.localStorage.setItem(IMPROVE_KANDEV_SKIP_INTRO_KEY, "true");
    const { rerender } = render(
      <ImproveKandevDialog open onOpenChange={() => {}} workspaceId="ws-active" />,
    );

    await waitFor(() => expect(mocks.bootstrap).toHaveBeenCalledTimes(1));

    // Close, then reopen — a re-open must re-run bootstrap instead of sitting
    // at the idle "Preparing kandev repository" banner with submit blocked.
    rerender(<ImproveKandevDialog open={false} onOpenChange={() => {}} workspaceId="ws-active" />);
    rerender(<ImproveKandevDialog open onOpenChange={() => {}} workspaceId="ws-active" />);

    await waitFor(() => expect(mocks.bootstrap).toHaveBeenCalledTimes(2));
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

function githubIssue(id: string, message: string) {
  return {
    id,
    category: "github",
    title: "GitHub",
    message,
    severity: "warning",
    fix_url: "/settings/integrations/github",
    fix_label: "Configure GitHub",
  };
}

// The dialog renders through a Radix portal, so its markup lands on
// document.body rather than the render container.
async function waitForProceedEnabled() {
  await waitFor(() => {
    const proceed = screen.getByTestId("improve-kandev-proceed");
    expect(proceed.hasAttribute("disabled")).toBe(false);
  });
}

describe("ImproveKandevDialog GitHub gate", () => {
  // The gate must ask "is there any GitHub credential at all", not "is every
  // workspace's connection healthy". A user on GitHub in one workspace and
  // another code host elsewhere carries this issue permanently, and it used to
  // lock the dialog behind a warning about workspaces this flow never touches.
  it("does not block on a connection that is unhealthy in another workspace", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE, IMPROVE_WORKSPACE]);
    mocks.health.mockResolvedValue({
      issues: [
        githubIssue(
          "github_workspace_connections_unhealthy",
          "1 of 2 configured GitHub workspace connections need attention (1 invalid, 0 suspended, 0 revoked).",
        ),
      ],
    });
    renderDialog();

    await waitForProceedEnabled();
    expect(screen.queryByText(/needs the/)).toBeNull();
  });

  it("does not block on an exhausted rate limit", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE, IMPROVE_WORKSPACE]);
    mocks.health.mockResolvedValue({
      issues: [githubIssue("github_rate_limit_core", "PR/issue checks are paused.")],
    });
    renderDialog();

    await waitForProceedEnabled();
  });

  it("still blocks when no GitHub connection is configured anywhere", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE, IMPROVE_WORKSPACE]);
    mocks.health.mockResolvedValue({
      issues: [
        githubIssue("github_not_authenticated", "Configure a GitHub connection for a workspace."),
      ],
    });
    renderDialog();

    await waitFor(() => expect(screen.queryByTestId("improve-kandev-proceed")).toBeNull());
  });

  // Rendered through the component, not a hand-fed <Trans>: the catalog string
  // interpolates the binary name inside its <code> element, so a values object
  // missing `binary` ships the literal "{{binary}}" to the user. A test that
  // supplies the values itself cannot see that.
  it("renders the gh-auth notice with no un-interpolated placeholder", async () => {
    setStoreWorkspaces([ACTIVE_WORKSPACE, IMPROVE_WORKSPACE]);
    mocks.health.mockResolvedValue({
      issues: [githubIssue("github_not_authenticated", GH_AUTH_MESSAGE)],
    });
    renderDialog();

    await waitFor(() => expect(document.body.textContent).toContain(GH_AUTH_MESSAGE));
    expect(document.body.textContent).not.toContain("{{");
    expect(
      Array.from(document.body.querySelectorAll("code")).map((node) => node.textContent),
    ).toContain("gh");
  });
});

import { Trans } from "react-i18next";
describe("improve-kandev dialog <Trans> copy", () => {
  it("renders the gh-auth notice byte-identically to the old literal", () => {
    const { container } = render(
      <Trans
        i18nKey="common:theFinalStepOpensAPullRequest"
        values={{ binary: "gh", message: GH_AUTH_MESSAGE }}
      >
        The final step of this workflow opens a pull request, which needs the <code>gh</code> CLI to
        be authenticated. {GH_AUTH_MESSAGE}
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
