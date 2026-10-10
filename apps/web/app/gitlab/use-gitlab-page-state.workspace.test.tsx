import type { ReactNode } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ListToolbar } from "@/components/gitlab/my-gitlab/list-toolbar";
import { MRList } from "@/components/gitlab/my-gitlab/mr-list";
import { resetKnownProjectsStore } from "@/components/gitlab/my-gitlab/use-known-projects";
import { __resetSnapshotForTests } from "@/components/gitlab/my-gitlab/use-saved-presets";
import type { Issue, IssueSearchPage, MR, MRSearchPage } from "@/lib/types/gitlab";
import { useGitLabPageState } from "./use-gitlab-page-state";

const requests = vi.hoisted(() => ({ mrs: vi.fn(), issues: vi.fn(), settings: vi.fn() }));

vi.mock("@/lib/api/domains/gitlab-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/gitlab-api")>()),
  searchUserMRs: requests.mrs,
  searchUserIssues: requests.issues,
}));
vi.mock("@/lib/api/domains/settings-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/settings-api")>()),
  fetchUserSettings: requests.settings,
}));

const A = "workspace-west";
const B = "workspace-east";
const FIRST = "west/service";
const SECOND = "west/frontend";
const OTHER = "east/application";
const ASSIGNED = "assigned";
const RETURN_PROJECT = "west/return";
const CREATED = "2026-10-01T12:00:00Z";

function mergeRequest(project: string): MR {
  return {
    id: 42,
    iid: 7,
    project_id: 3,
    title: `Review ${project}`,
    url: `https://gitlab.example/${project}/-/merge_requests/7`,
    web_url: `https://gitlab.example/${project}/-/merge_requests/7`,
    state: "opened",
    head_branch: "feature",
    head_sha: "abc123",
    base_branch: "main",
    author_username: "developer",
    project_namespace: project.split("/")[0],
    project_path: project,
    body: "",
    draft: false,
    merge_status: "can_be_merged",
    has_conflicts: false,
    additions: 2,
    deletions: 1,
    reviewers: [],
    assignees: [],
    created_at: CREATED,
    updated_at: CREATED,
  };
}

function issue(project: string): Issue {
  return {
    id: 61,
    iid: 9,
    project_id: 3,
    title: `Implement ${project}`,
    body: "",
    url: `https://gitlab.example/${project}/-/issues/9`,
    web_url: `https://gitlab.example/${project}/-/issues/9`,
    state: "opened",
    author_username: "developer",
    project_namespace: project.split("/")[0],
    project_path: project,
    labels: [],
    assignees: [],
    milestone: "Release",
    created_at: CREATED,
    updated_at: CREATED,
  };
}

function mrPage(projects: string[], page = 1): MRSearchPage {
  return { mrs: projects.map(mergeRequest), total_count: 50, page, per_page: 25 };
}

function issuePage(projects: string[], page = 1): IssueSearchPage {
  return { issues: projects.map(issue), total_count: 50, page, per_page: 25 };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

function pageProject(workspaceId: string, page: number) {
  if (workspaceId !== A) return OTHER;
  return page === 1 ? FIRST : SECOND;
}

function Providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </StateProvider>
  );
}

function mountState(workspaceId = A, enabled = true) {
  return renderHook(
    (props: { workspaceId: string; enabled: boolean }) =>
      useGitLabPageState(props.enabled, props.workspaceId),
    { initialProps: { workspaceId, enabled }, wrapper: Providers },
  );
}

function BrowseResults({ workspaceId }: { workspaceId: string }) {
  const state = useGitLabPageState(true, workspaceId);
  return (
    <>
      <ListToolbar
        title={state.title}
        count={state.search.total}
        loading={state.search.loading}
        lastFetchedAt={state.search.lastFetchedAt}
        customQuery={state.customQuery}
        committedQuery={state.committedQuery}
        onCustomQueryChange={state.setCustomQuery}
        onCommitCustomQuery={state.commitCustomQuery}
        projectFilter={state.projectFilter}
        onProjectFilterChange={state.setProjectFilter}
        projectOptions={state.projectOptions}
        onRefresh={state.search.refresh}
        showMilestoneFilter={state.showMilestoneFilter}
        milestone={state.milestone}
        committedMilestone={state.committedMilestone}
        onMilestoneChange={state.setMilestone}
        onCommitMilestone={state.onCommitMilestone}
      />
      <MRList
        items={state.search.items as MR[]}
        loading={state.search.loading}
        error={state.search.error}
      />
    </>
  );
}

beforeEach(() => {
  resetKnownProjectsStore();
  __resetSnapshotForTests();
  requests.settings.mockReset().mockResolvedValue({ settings: { gitlab_saved_presets: [] } });
  requests.mrs
    .mockReset()
    .mockImplementation(({ workspaceId, page }: { workspaceId: string; page: number }) =>
      Promise.resolve(mrPage([pageProject(workspaceId, page)], page)),
    );
  requests.issues
    .mockReset()
    .mockImplementation(({ workspaceId, page }: { workspaceId: string; page: number }) =>
      Promise.resolve(issuePage([pageProject(workspaceId, page)], page)),
    );
});
afterEach(() => cleanup());

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.5
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.8
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.10
describe("GitLab project options workspace scope", () => {
  it("excludes previous workspace projects after the current MR response settles", async () => {
    const view = mountState();
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
    view.rerender({ workspaceId: B, enabled: true });
    await waitFor(() => expect(view.result.current.search.rawItems[0]?.project_path).toBe(OTHER));
    expect(view.result.current.projectOptions).toEqual([OTHER]);
  });

  it("offers only current workspace projects in the real toolbar", async () => {
    requests.mrs.mockImplementation(({ workspaceId }: { workspaceId: string }) =>
      Promise.resolve(mrPage(workspaceId === A ? [FIRST] : [OTHER, "east/worker"])),
    );
    const view = render(<BrowseResults workspaceId={A} />, { wrapper: Providers });
    await screen.findByRole("link", { name: `Review ${FIRST}` });
    view.rerender(<BrowseResults workspaceId={B} />);
    await screen.findByRole("link", { name: `Review ${OTHER}` });
    fireEvent.keyDown(screen.getByTestId("gitlab-project-filter-trigger"), { key: "ArrowDown" });
    const option = await screen.findByRole("option", { name: OTHER });
    expect(screen.queryByRole("option", { name: FIRST })).toBeNull();
    fireEvent.click(option);
    await waitFor(() =>
      expect(screen.queryByRole("link", { name: "Review east/worker" })).toBeNull(),
    );
    expect(screen.getByRole("link", { name: `Review ${OTHER}` })).toBeTruthy();
  });

  it("retains earlier-page projects in the same workspace", async () => {
    const view = mountState();
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
    act(() => view.result.current.search.setPage(2));
    await waitFor(() => expect(view.result.current.search.rawItems[0]?.project_path).toBe(SECOND));
    expect(view.result.current.projectOptions).toEqual([SECOND, FIRST]);
  });
});

describe("GitLab project options workspace transitions", () => {
  it.each(["mr", "issue"] as const)(
    "starts %s results in B without an earlier workspace",
    async (kind) => {
      const view = mountState(B);
      if (kind === "issue")
        act(() => view.result.current.onSelect({ kind, source: "preset", id: ASSIGNED }));
      await waitFor(() => expect(view.result.current.projectOptions).toEqual([OTHER]));
    },
  );

  it.each(["mr", "issue"] as const)(
    "isolates %s accumulated pages through A-B-A and B-A",
    async (kind) => {
      const view = mountState();
      if (kind === "issue")
        act(() => view.result.current.onSelect({ kind, source: "preset", id: ASSIGNED }));
      await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
      act(() => view.result.current.search.setPage(2));
      await waitFor(() => expect(view.result.current.projectOptions).toEqual([SECOND, FIRST]));
      view.rerender({ workspaceId: B, enabled: true });
      await waitFor(() => expect(view.result.current.search.rawItems[0]?.project_path).toBe(OTHER));
      expect(view.result.current.projectOptions).toEqual([OTHER]);
      const response = kind === "mr" ? mrPage([RETURN_PROJECT]) : issuePage([RETURN_PROJECT]);
      (kind === "mr" ? requests.mrs : requests.issues).mockResolvedValue(response);
      view.rerender({ workspaceId: A, enabled: true });
      await waitFor(() =>
        expect(view.result.current.search.rawItems[0]?.project_path).toBe(RETURN_PROJECT),
      );
      expect(view.result.current.projectOptions).toEqual([RETURN_PROJECT]);
    },
  );

  it.each(["mr", "issue"] as const)(
    "clears %s options for an empty replacement workspace",
    async (kind) => {
      const view = mountState();
      if (kind === "issue")
        act(() => view.result.current.onSelect({ kind, source: "preset", id: ASSIGNED }));
      await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
      (kind === "mr" ? requests.mrs : requests.issues).mockResolvedValue(
        kind === "mr" ? mrPage([]) : issuePage([]),
      );
      view.rerender({ workspaceId: B, enabled: true });
      await waitFor(() => expect(view.result.current.search.loading).toBe(false));
      expect(view.result.current.search.rawItems).toEqual([]);
      expect(view.result.current.projectOptions).toEqual([]);
    },
  );
});

describe("GitLab project options selected filters and loading", () => {
  it("retains only the explicit selected filter alongside current workspace projects", async () => {
    const view = mountState();
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
    act(() => view.result.current.setProjectFilter("selected/external"));
    view.rerender({ workspaceId: B, enabled: true });
    await waitFor(() => expect(view.result.current.search.rawItems[0]?.project_path).toBe(OTHER));
    expect(view.result.current.projectFilter).toBe("selected/external");
    expect(view.result.current.projectOptions).toEqual([OTHER, "selected/external"]);
    expect(view.result.current.search.items).toEqual([]);
  });

  it("preserves equal-context reuse and earlier options while a page loads", async () => {
    const pending = deferred<MRSearchPage>();
    requests.mrs.mockResolvedValueOnce(mrPage([FIRST])).mockReturnValueOnce(pending.promise);
    const view = mountState();
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
    view.rerender({ workspaceId: A, enabled: true });
    expect(view.result.current.projectOptions).toEqual([FIRST]);
    act(() => view.result.current.search.setPage(2));
    await waitFor(() => expect(view.result.current.search.loading).toBe(true));
    expect(view.result.current.projectOptions).toEqual([FIRST]);
    await act(async () => {
      pending.resolve(mrPage([SECOND], 2));
      await pending.promise;
    });
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([SECOND, FIRST]));
  });
});

describe("GitLab project options obsolete responses", () => {
  it("clears old membership while B loads and ignores an obsolete A response", async () => {
    const old = deferred<MRSearchPage>();
    const replacement = deferred<MRSearchPage>();
    requests.mrs
      .mockResolvedValueOnce(mrPage([FIRST]))
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(replacement.promise);
    const view = mountState();
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
    let refresh!: Promise<void>;
    act(() => {
      refresh = view.result.current.search.refresh();
    });
    view.rerender({ workspaceId: B, enabled: true });
    expect(view.result.current.search.loading).toBe(true);
    expect(view.result.current.projectOptions).toEqual([]);
    await act(async () => {
      replacement.resolve(mrPage([OTHER]));
      await replacement.promise;
    });
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([OTHER]));
    await act(async () => {
      old.resolve(mrPage(["west/obsolete"]));
      await refresh;
    });
    expect(view.result.current.projectOptions).toEqual([OTHER]);
    expect(view.result.current.search.rawItems[0]?.project_path).toBe(OTHER);
  });

  it("preserves same-context membership through empty refresh, error and disabling", async () => {
    const view = mountState();
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
    requests.mrs.mockResolvedValueOnce(mrPage([]));
    await act(async () => {
      await view.result.current.search.refresh();
    });
    expect(view.result.current.search.items).toEqual([]);
    expect(view.result.current.projectOptions).toEqual([FIRST]);
    requests.mrs.mockRejectedValueOnce(new Error("Provider unavailable"));
    await act(async () => {
      await view.result.current.search.refresh();
    });
    expect(view.result.current.search.error).toBe("Provider unavailable");
    expect(view.result.current.projectOptions).toEqual([FIRST]);
    view.rerender({ workspaceId: A, enabled: false });
    expect(view.result.current.search.rawItems).toEqual([]);
    expect(view.result.current.search.loading).toBe(false);
    expect(view.result.current.projectOptions).toEqual([FIRST]);
  });
});

describe("GitLab project options browse context", () => {
  it("resets projects when query, kind or milestone changes within the workspace", async () => {
    const view = mountState();
    await waitFor(() => expect(view.result.current.projectOptions).toEqual([FIRST]));
    requests.mrs.mockResolvedValue(mrPage(["west/query"]));
    act(() => view.result.current.setCustomQuery("labels=release:ready"));
    act(() => view.result.current.commitCustomQuery());
    await waitFor(() => expect(view.result.current.projectOptions).toEqual(["west/query"]));
    requests.issues.mockResolvedValue(issuePage(["west/issues"]));
    act(() => view.result.current.onSelect({ kind: "issue", source: "preset", id: ASSIGNED }));
    await waitFor(() => expect(view.result.current.projectOptions).toEqual(["west/issues"]));
    requests.issues.mockResolvedValue(issuePage(["west/milestone"]));
    act(() => view.result.current.setMilestone("Release:ready"));
    act(() => view.result.current.onCommitMilestone());
    await waitFor(() => expect(view.result.current.projectOptions).toEqual(["west/milestone"]));
  });
});
