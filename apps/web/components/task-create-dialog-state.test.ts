import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { computeDialogDefaultStepId } from "./task-create-dialog-defaults";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { useDialogFormState } from "./task-create-dialog-state";
import { buildRepositoriesPayload } from "./task-create-dialog-helpers";
import { TASK_TITLE_MAX_LENGTH } from "@/lib/task-title";

const BITBUCKET_HEAD_BRANCH = "feature/fix";

// `useBranchesByURL` triggers a real network ensure() when given a URL — stub
// it so the dialog state hook can mount in JSDOM without hitting fetch. The
// stubbed shape mirrors the production hook (branches/loading/ensure).
vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({
    branches: () => [],
    loading: () => false,
    ensure: () => undefined,
  }),
}));

// `usePRInfoByURL` also touches the network on ensure(); stub it to a
// per-test-controlled cache so the title-autofill effect can be exercised
// without an actual fetch. Each test that needs a specific GitHub URL info
// value writes into `prInfoMap` before calling `setUseRemote(true)`.
const prInfoMap = new Map<
  string,
  {
    prHeadBranch?: string;
    prBaseBranch?: string;
    prNumber?: number;
    issueNumber?: number;
    suggestedTitle: string;
  }
>();
vi.mock("@/hooks/domains/github/use-pr-info-by-url", async (importOriginal) => {
  const original =
    await importOriginal<typeof import("@/hooks/domains/github/use-pr-info-by-url")>();
  return {
    ...original,
    usePRInfoByURL: () => ({
      info: (url: string) => prInfoMap.get(url),
      loading: () => false,
      ensure: () => undefined,
      clear: () => undefined,
    }),
  };
});

function snapshot(workflowId: string): WorkflowSnapshotData {
  return {
    workflowId,
    workflowName: workflowId,
    steps: [
      {
        id: `${workflowId}-later`,
        title: "Later",
        color: "gray",
        position: 2,
      },
      {
        id: `${workflowId}-start`,
        title: "Start",
        color: "green",
        position: 1,
        is_start_step: true,
      },
    ],
    tasks: [],
  };
}

describe("computeDialogDefaultStepId", () => {
  it("uses the resolved workflow when falling back to snapshot steps", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: null,
        workflowId: "provided",
        fetchedSteps: null,
        defaultStepId: null,
        effectiveWorkflowId: "provided",
        snapshots: {
          provided: snapshot("provided"),
          single: snapshot("single"),
        },
      }),
    ).toBe("provided-start");
  });

  it("falls back to the lowest-position snapshot step when no start step exists", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: null,
        workflowId: "provided",
        fetchedSteps: null,
        defaultStepId: null,
        effectiveWorkflowId: "provided",
        snapshots: {
          provided: {
            workflowId: "provided",
            workflowName: "provided",
            steps: [
              { id: "provided-2", title: "Two", color: "gray", position: 2 },
              { id: "provided-1", title: "One", color: "green", position: 1 },
            ],
            tasks: [],
          },
        },
      }),
    ).toBe("provided-1");
  });

  it("ignores a stale default step while a newly selected workflow loads", () => {
    expect(
      computeDialogDefaultStepId({
        selectedWorkflowId: "selected",
        workflowId: "original",
        fetchedSteps: null,
        defaultStepId: "original-start",
        effectiveWorkflowId: "selected",
        snapshots: {
          original: snapshot("original"),
          selected: snapshot("selected"),
        },
      }),
    ).toBe("selected-start");
  });
});

describe("useDialogFormState — remoteRepos mode", () => {
  it("seeds one empty remoteRepos row when useRemote toggles on with an empty list", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    expect(result.current.remoteRepos).toHaveLength(0);

    act(() => {
      result.current.setUseRemote(true);
    });

    expect(result.current.remoteRepos).toHaveLength(1);
    expect(result.current.remoteRepos[0]).toMatchObject({ url: "", branch: "", source: "paste" });
  });

  it("preserves the remoteRepos array when switching Remote → Repo → Remote", () => {
    const PASTED_URL = "github.com/owner/repo";
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));

    // Enter Remote mode, fill in a URL.
    act(() => {
      result.current.setUseRemote(true);
    });
    const seededKey = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(seededKey!, { url: PASTED_URL });
    });
    expect(result.current.remoteRepos[0]?.url).toBe(PASTED_URL);

    // Switch back to Repo mode (Remote off). The array must NOT be cleared.
    act(() => {
      result.current.setUseRemote(false);
    });
    expect(result.current.remoteRepos[0]?.url).toBe(PASTED_URL);

    // Flip back to Remote mode — the prior rows are still there.
    act(() => {
      result.current.setUseRemote(true);
    });
    expect(result.current.remoteRepos).toHaveLength(1);
    expect(result.current.remoteRepos[0]?.url).toBe(PASTED_URL);
  });

  it("seeds remoteRepos from initialValues.githubUrl and sets useRemote=true on dialog open", () => {
    const initialValues = {
      title: "",
      githubUrl: "github.com/acme/site",
      branch: "main",
    };
    const { result, rerender } = renderHook(
      ({ open }: { open: boolean }) => useDialogFormState(open, "ws-1", null, initialValues),
      { initialProps: { open: false } },
    );

    // Rising edge: dialog opens with a pre-filled URL.
    rerender({ open: true });

    expect(result.current.useRemote).toBe(true);
    expect(result.current.remoteRepos).toHaveLength(1);
    expect(result.current.remoteRepos[0]).toMatchObject({
      url: "github.com/acme/site",
      branch: "main",
      source: "paste",
    });
  });

  it("prefers provider-neutral remoteUrl while retaining githubUrl compatibility", () => {
    const initialValues = {
      title: "",
      remoteUrl: "https://bitbucket.example.test/bitbucket/scm/PLATFORM/web.git",
      githubUrl: "https://github.com/acme/legacy",
      branch: "main",
    };
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null, initialValues));

    expect(result.current.useRemote).toBe(true);
    expect(result.current.remoteRepos).toEqual([
      expect.objectContaining({
        url: "https://bitbucket.example.test/bitbucket/scm/PLATFORM/web.git",
        branch: "main",
        source: "paste",
      }),
    ]);
  });
});

describe("useDialogFormState — canvas task preset", () => {
  it("restores the repository-free source and launch-only local preference", async () => {
    const { result } = renderHook(() =>
      useDialogFormState(true, "ws-1", null, {
        title: "Create a canvas",
        description: "Build a canvas",
        noRepository: true,
        preferLocalExecutor: true,
      }),
    );

    await waitFor(() => expect(result.current.noRepository).toBe(true));
    expect(result.current.workspacePath).toBe("");
    expect(result.current.preferLocalExecutor).toBe(true);
    expect(result.current.repositories).toEqual([]);
    expect(result.current.useRemote).toBe(false);
  });
});

describe("useDialogFormState — repository policy snapshots", () => {
  it("restores a policy-backed repository row without marking it as edited", () => {
    const { result } = renderHook(() =>
      useDialogFormState(true, "ws-1", null, {
        title: "Existing task",
        repositories: [
          {
            id: "task-repo-1",
            task_id: "task-1",
            repository_id: "repo-1",
            base_branch: "main",
            checkout_branch: "feature/existing",
            branch_policy_id: "policy-1",
            branch_policy_name: "Feature branches",
            branch_policy_base_branch: "main",
            branch_policy_branch_template: "feature/{title}-{suffix}",
            branch_policy_pull_request_target: "main",
            position: 0,
            created_at: "2026-08-24T10:00:00Z",
            updated_at: "2026-08-24T10:00:00Z",
          },
        ],
      }),
    );

    expect(result.current.repositories).toEqual([
      expect.objectContaining({
        repositoryId: "repo-1",
        branch: "main",
        branchPolicyId: "policy-1",
      }),
    ]);
    expect(result.current.repositoriesDirty).toBe(false);

    act(() => {
      result.current.updateRepository(result.current.repositories[0]!.key, { branch: "develop" });
    });

    expect(result.current.repositoriesDirty).toBe(true);
  });
});

describe("useDialogFormState — provider-neutral initial values", () => {
  it("seeds an authorized provider descriptor before asynchronous URL inspection", () => {
    const initialValues = {
      title: "",
      remoteUrl: "https://bitbucket.example.test/projects/PLATFORM/repos/web/pull-requests/42",
      checkoutBranch: BITBUCKET_HEAD_BRANCH,
      remoteRepository: {
        providerId: "bitbucket",
        providerHost: "https://bitbucket.example.test",
        ownerOrProject: "PLATFORM",
        repositoryId: "web-42",
        repositoryName: "web",
        cloneUrl: "https://bitbucket.example.test/scm/PLATFORM/web.git",
        defaultBranch: "trunk",
        baseBranch: "trunk",
        headBranch: BITBUCKET_HEAD_BRANCH,
        pullRequest: { number: 42, title: "Fix" },
      },
    };
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null, initialValues));

    expect(result.current.remoteRepos[0]).toMatchObject({
      url: initialValues.remoteUrl,
      remoteUrl: "https://bitbucket.example.test/scm/PLATFORM/web.git",
      provider: "bitbucket",
      providerHost: "https://bitbucket.example.test",
      providerRepoId: "web-42",
      providerOwner: "PLATFORM",
      providerName: "web",
      branch: BITBUCKET_HEAD_BRANCH,
      prNumber: 42,
      prBaseBranch: "trunk",
      prHeadBranch: BITBUCKET_HEAD_BRANCH,
    });
  });
});

describe("useDialogFormState — remote PR metadata", () => {
  it("clears seeded PR metadata when a remote repo URL changes", () => {
    const initialValues = {
      title: "",
      githubUrl: PR_URL_42,
      branch: "feature/x",
      checkoutBranch: "feature/x",
      prNumber: 42,
      prBaseBranch: "main",
    };
    const { result, rerender } = renderHook(
      ({ open }: { open: boolean }) => useDialogFormState(open, "ws-1", null, initialValues),
      { initialProps: { open: false } },
    );

    rerender({ open: true });
    const key = result.current.remoteRepos[0]?.key;
    expect(result.current.remoteRepos[0]).toMatchObject({
      url: PR_URL_42,
      branch: "feature/x",
      prNumber: 42,
      prBaseBranch: "main",
      prHeadBranch: "feature/x",
    });

    act(() => {
      result.current.updateRemoteRepo(key!, { url: "https://github.com/acme/site/pull/99" });
    });

    expect(result.current.remoteRepos[0]).toMatchObject({
      url: "https://github.com/acme/site/pull/99",
      branch: "feature/x",
    });
    expect(result.current.remoteRepos[0]?.prNumber).toBeUndefined();
    expect(result.current.remoteRepos[0]?.prBaseBranch).toBeUndefined();
    expect(result.current.remoteRepos[0]?.prHeadBranch).toBeUndefined();
  });

  it("preserves PR metadata supplied with a remote repo URL change", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));

    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;

    act(() => {
      result.current.updateRemoteRepo(key!, {
        url: PR_URL_42,
        branch: "feature/x",
        prNumber: 42,
        prBaseBranch: "main",
        prHeadBranch: "feature/x",
      });
    });

    expect(result.current.remoteRepos[0]).toMatchObject({
      url: PR_URL_42,
      branch: "feature/x",
      prNumber: 42,
      prBaseBranch: "main",
      prHeadBranch: "feature/x",
    });
  });
});

describe("useDialogFormState — remoteRepos key allocation", () => {
  // Regression: the per-hook counter starts at 0 and increments locally, so
  // a hydrated state that already contains `remote-1` (e.g. from the seed
  // effect or initialValues) would collide on the next addRemoteRepo() —
  // the new row would also be named `remote-1`, breaking React keys.
  it("addRemoteRepo skips keys already present in the rows array", () => {
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));

    // Flip into Remote mode so the seed effect injects `remote-0`.
    act(() => {
      result.current.setUseRemote(true);
    });
    expect(result.current.remoteRepos).toHaveLength(1);
    expect(result.current.remoteRepos[0]?.key).toBe("remote-0");

    // Manually hydrate with a row whose key matches what the local counter
    // is about to hand out (remote-1).
    act(() => {
      result.current.setRemoteRepos([
        { key: "remote-1", url: "github.com/a/b", branch: "main", source: "paste" },
      ]);
    });

    act(() => {
      result.current.addRemoteRepo();
    });

    const keys = result.current.remoteRepos.map((r) => r.key);
    // No duplicates: hydrated `remote-1` still present, but the new row
    // skipped past it instead of colliding.
    expect(new Set(keys).size).toBe(keys.length);
    expect(result.current.remoteRepos).toHaveLength(2);
    expect(result.current.remoteRepos[0]?.key).toBe("remote-1");
    expect(result.current.remoteRepos[1]?.key).not.toBe("remote-1");
  });
});

describe("buildRepositoriesPayload — remoteRepos rows", () => {
  it("filters out rows with empty url before mapping to repos[]", () => {
    const payload = buildRepositoriesPayload({
      useRemote: true,
      remoteRepos: [
        { key: "remote-0", url: "github.com/owner/repo-a", branch: "main", source: "paste" },
        { key: "remote-1", url: "", branch: "", source: "paste" },
        { key: "remote-2", url: "  ", branch: "", source: "paste" },
        { key: "remote-3", url: "github.com/owner/repo-b", branch: "develop", source: "paste" },
      ],
      repositories: [],
      discoveredRepositories: [],
    });
    expect(payload).toHaveLength(2);
    expect(payload[0]).toMatchObject({
      github_url: "github.com/owner/repo-a",
    });
    expect(payload[1]).toMatchObject({
      github_url: "github.com/owner/repo-b",
      base_branch: "develop",
    });
  });
});

const PR_URL_42 = "https://github.com/acme/site/pull/42";
const PR_TITLE_42 = "PR #42: Test PR";
const USER_TYPED_TITLE = "my own title";
const ISSUE_URL_1456 = "https://github.com/acme/site/issues/1456";
const ISSUE_TITLE_1456 = "Issue #1456: Fix remote picker";

function seedPRInfo(url: string, prNumber: number, suggestedTitle: string) {
  prInfoMap.set(url, {
    prHeadBranch: "feature/x",
    prBaseBranch: "main",
    prNumber,
    suggestedTitle,
  });
}

function seedIssueInfo(url: string, issueNumber: number, suggestedTitle: string) {
  prInfoMap.set(url, {
    issueNumber,
    suggestedTitle,
  });
}

describe("useDialogFormState — title autofill from first row GitHub URL info", () => {
  beforeEach(() => {
    prInfoMap.clear();
  });

  it("seeds the task title from the first row's PR info when title is empty", () => {
    seedPRInfo(PR_URL_42, 42, PR_TITLE_42);
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: PR_URL_42 });
    });
    expect(result.current.taskName).toBe(PR_TITLE_42);
    expect(result.current.hasTitle).toBe(true);
  });

  it("truncates a long remote PR title to the shared limit", () => {
    const longTitle = "PR #42: " + "x".repeat(100);
    seedPRInfo(PR_URL_42, 42, longTitle);
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: PR_URL_42 });
    });

    expect(result.current.taskName).toHaveLength(TASK_TITLE_MAX_LENGTH);
    expect(result.current.taskName.endsWith("…")).toBe(true);
  });

  it("does NOT overwrite a title the user typed themselves", () => {
    seedPRInfo(PR_URL_42, 42, PR_TITLE_42);
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setTaskName(USER_TYPED_TITLE);
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: PR_URL_42 });
    });
    expect(result.current.taskName).toBe(USER_TYPED_TITLE);
  });
});

describe("useDialogFormState — title autofill after title ownership", () => {
  beforeEach(() => {
    prInfoMap.clear();
  });

  it("does NOT re-apply autofill after the user clears the title (user took ownership)", () => {
    // Regression: clearing an auto-filled title used to reset the ref to ""
    // and trigger a re-application on the next render, so the user could
    // never actually clear the field — every keystroke or render brought
    // the suggested title right back.
    seedPRInfo(PR_URL_42, 42, PR_TITLE_42);
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: PR_URL_42 });
    });
    expect(result.current.taskName).toBe(PR_TITLE_42);
    act(() => {
      result.current.setTaskName("");
    });
    // Even after re-render, autofill MUST NOT reapply for this URL.
    expect(result.current.taskName).toBe("");
  });

  it("re-applies autofill when the user switches to a different PR URL", () => {
    // Once the user pastes a fresh PR URL, the previous "user-cleared" lock
    // for the earlier URL must lift — the fresh URL is a new autofill
    // opportunity.
    seedPRInfo(PR_URL_42, 42, PR_TITLE_42);
    const NEW_PR_URL = "https://github.com/acme/site/pull/99";
    seedPRInfo(NEW_PR_URL, 99, "PR #99: Another PR");
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: PR_URL_42 });
    });
    expect(result.current.taskName).toBe(PR_TITLE_42);
    act(() => {
      result.current.setTaskName("");
    });
    expect(result.current.taskName).toBe("");

    // Switch to a different PR URL → fresh autofill opportunity.
    act(() => {
      result.current.updateRemoteRepo(key!, { url: NEW_PR_URL });
    });
    expect(result.current.taskName).toBe("PR #99: Another PR");
  });

  it("autofills from the first non-empty row when an earlier placeholder is empty", () => {
    const SECOND_PR_URL = "https://github.com/acme/api/pull/99";
    prInfoMap.set(SECOND_PR_URL, {
      prHeadBranch: "feature/y",
      prBaseBranch: "main",
      prNumber: 99,
      suggestedTitle: "PR #99: Second PR",
    });
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    // Add a second row with a PR URL; row 0 stays empty.
    act(() => {
      result.current.addRemoteRepo();
    });
    const secondKey = result.current.remoteRepos[1]?.key;
    act(() => {
      result.current.updateRemoteRepo(secondKey!, { url: SECOND_PR_URL });
    });
    expect(result.current.taskName).toBe("PR #99: Second PR");
  });
});

describe("useDialogFormState — title autofill from first row GitHub issue info", () => {
  beforeEach(() => {
    prInfoMap.clear();
  });

  it("seeds the task title from the first row's issue info when title is empty", () => {
    seedIssueInfo(ISSUE_URL_1456, 1456, ISSUE_TITLE_1456);
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: ISSUE_URL_1456 });
    });
    expect(result.current.taskName).toBe(ISSUE_TITLE_1456);
    expect(result.current.hasTitle).toBe(true);
  });

  it("does NOT overwrite a title the user typed themselves", () => {
    seedIssueInfo(ISSUE_URL_1456, 1456, ISSUE_TITLE_1456);
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setTaskName(USER_TYPED_TITLE);
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: ISSUE_URL_1456 });
    });
    expect(result.current.taskName).toBe(USER_TYPED_TITLE);
  });

  it("does NOT re-apply autofill after the user clears the title", () => {
    seedIssueInfo(ISSUE_URL_1456, 1456, ISSUE_TITLE_1456);
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: ISSUE_URL_1456 });
    });
    expect(result.current.taskName).toBe(ISSUE_TITLE_1456);
    act(() => {
      result.current.setTaskName("");
    });
    expect(result.current.taskName).toBe("");
  });

  it("re-applies autofill when the user switches to a different issue URL", () => {
    const newIssueURL = "https://github.com/acme/site/issues/1457";
    seedIssueInfo(ISSUE_URL_1456, 1456, ISSUE_TITLE_1456);
    seedIssueInfo(newIssueURL, 1457, "Issue #1457: Add issue URL paste");
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: ISSUE_URL_1456 });
    });
    expect(result.current.taskName).toBe(ISSUE_TITLE_1456);
    act(() => {
      result.current.setTaskName("");
    });
    expect(result.current.taskName).toBe("");

    act(() => {
      result.current.updateRemoteRepo(key!, { url: newIssueURL });
    });
    expect(result.current.taskName).toBe("Issue #1457: Add issue URL paste");
  });
});
