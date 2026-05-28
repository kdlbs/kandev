import { beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { computeDialogDefaultStepId } from "./task-create-dialog-defaults";
import type { WorkflowSnapshotData } from "@/lib/state/slices/kanban/types";
import { useDialogFormState } from "./task-create-dialog-state";
import { buildRepositoriesPayload } from "./task-create-dialog-helpers";

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
// without an actual fetch. Each test that needs a specific PR-info value
// writes into `prInfoMap` before calling `setUseRemote(true)`.
const prInfoMap = new Map<
  string,
  { prHeadBranch: string; prBaseBranch: string; prNumber: number; suggestedTitle: string }
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

describe("useDialogFormState — title autofill from first row PR info", () => {
  const PR_URL = "https://github.com/acme/site/pull/42";

  beforeEach(() => {
    prInfoMap.clear();
  });

  it("seeds the task title from the first row's PR info when title is empty", () => {
    prInfoMap.set(PR_URL, {
      prHeadBranch: "feature/x",
      prBaseBranch: "main",
      prNumber: 42,
      suggestedTitle: "PR #42: Test PR",
    });
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: PR_URL });
    });
    expect(result.current.taskName).toBe("PR #42: Test PR");
    expect(result.current.hasTitle).toBe(true);
  });

  it("does NOT overwrite a title the user typed themselves", () => {
    prInfoMap.set(PR_URL, {
      prHeadBranch: "feature/x",
      prBaseBranch: "main",
      prNumber: 42,
      suggestedTitle: "PR #42: Test PR",
    });
    const { result } = renderHook(() => useDialogFormState(true, "ws-1", null));
    act(() => {
      result.current.setTaskName("my own title");
      result.current.setUseRemote(true);
    });
    const key = result.current.remoteRepos[0]?.key;
    act(() => {
      result.current.updateRemoteRepo(key!, { url: PR_URL });
    });
    expect(result.current.taskName).toBe("my own title");
  });

  it("does NOT autofill from a non-first row's PR info", () => {
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
    expect(result.current.taskName).toBe("");
  });
});
