import type { PropsWithChildren } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import type { Issue, IssueSearchPage, MR, MRSearchPage } from "@/lib/types/gitlab";
import type { searchUserIssues, searchUserMRs } from "@/lib/api/domains/gitlab-api";
import { MRList } from "./mr-list";
import { ResultsPagination } from "./results-pagination";
import { ISSUE_PRESETS, MR_PRESETS } from "./presets";
import { useGitLabSearch } from "./use-gitlab-search";

const transport = vi.hoisted(() => ({
  mrs: vi.fn<typeof searchUserMRs>(),
  issues: vi.fn<typeof searchUserIssues>(),
}));

vi.mock(import("@/lib/api/domains/gitlab-api"), async (importOriginal) => ({
  ...(await importOriginal()),
  searchUserMRs: transport.mrs,
  searchUserIssues: transport.issues,
}));

const WORKSPACE_A = "gitlab-workspace-a";
const WORKSPACE_B = "gitlab-workspace-b";
const ASSIGNED = "assigned";
const REVIEW_REQUESTED = "review_requested";
const CUSTOM_QUERY = "labels=regression";
const TRANSPORT_FAILURE = "Search unavailable";
const PROJECT_A = "alpha/service";
const PROJECT_B = "beta/service";
const FIXTURE_AT = "2026-10-01T12:00:00Z";
const KINDS = ["mr", "issue"] as const;
type Kind = (typeof KINDS)[number];
type Options = Parameters<typeof useGitLabSearch>[0];
type MRParams = Parameters<typeof searchUserMRs>[0];
type IssueParams = Parameters<typeof searchUserIssues>[0];
const RESET_INPUTS: Array<[string, Partial<Options>]> = [
  ["preset", { preset: REVIEW_REQUESTED }],
  ["custom query", { customQuery: CUSTOM_QUERY }],
  ["kind", { kind: "issue", presets: ISSUE_PRESETS }],
];

function makeMR(workspace: string, iid: number): MR {
  const project = workspace === WORKSPACE_A ? PROJECT_A : PROJECT_B;
  const url = `https://gitlab.example/${project}/-/merge_requests/${iid}`;
  return {
    id: iid,
    iid,
    project_id: 7,
    title: `${workspace} merge request ${iid}`,
    url,
    web_url: url,
    state: "opened",
    head_branch: "feature",
    head_sha: "abcdef",
    base_branch: "main",
    author_username: "reviewer",
    project_namespace: project.split("/")[0],
    project_path: project,
    body: "Workspace pagination fixture",
    draft: false,
    merge_status: "can_be_merged",
    has_conflicts: false,
    additions: 1,
    deletions: 0,
    reviewers: [],
    assignees: [],
    created_at: FIXTURE_AT,
    updated_at: FIXTURE_AT,
  };
}

function makeIssue(workspace: string, iid: number): Issue {
  const project = workspace === WORKSPACE_A ? PROJECT_A : PROJECT_B;
  const url = `https://gitlab.example/${project}/-/issues/${iid}`;
  return {
    id: iid,
    iid,
    project_id: 7,
    title: `${workspace} issue ${iid}`,
    body: "Workspace issue fixture",
    url,
    web_url: url,
    state: "opened",
    author_username: "reporter",
    project_namespace: project.split("/")[0],
    project_path: project,
    labels: [],
    assignees: [],
    milestone: "Next",
    created_at: FIXTURE_AT,
    updated_at: FIXTURE_AT,
  };
}

const A_MRS = Array.from({ length: 61 }, (_, index) => makeMR(WORKSPACE_A, index + 1));
const A_ISSUES = Array.from({ length: 61 }, (_, index) => makeIssue(WORKSPACE_A, index + 1));
const B_MR = makeMR(WORKSPACE_B, 101);
const B_ISSUE = makeIssue(WORKSPACE_B, 201);

// The fake server slices independent datasets using the incoming page, without
// knowing why the caller selected it or which workspace was visited before.
function mrPage({ workspaceId, page = 1, perPage = 25 }: MRParams): MRSearchPage {
  const rows = workspaceId === WORKSPACE_A ? A_MRS : [B_MR];
  return {
    mrs: rows.slice((page - 1) * perPage, page * perPage),
    total_count: rows.length,
    page,
    per_page: perPage,
  };
}

function issuePage({ workspaceId, page = 1, perPage = 25 }: IssueParams): IssueSearchPage {
  const rows = workspaceId === WORKSPACE_A ? A_ISSUES : [B_ISSUE];
  return {
    issues: rows.slice((page - 1) * perPage, page * perPage),
    total_count: rows.length,
    page,
    per_page: perPage,
  };
}

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T) => void;
  reject: (reason: Error) => void;
};
const settleHeldPages: Array<() => void> = [];

function deferPage<T>(fallback: T): Deferred<T> {
  let resolve!: Deferred<T>["resolve"];
  let reject!: Deferred<T>["reject"];
  const promise = new Promise<T>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  settleHeldPages.push(() => resolve(fallback));
  return { promise, resolve, reject };
}

function Providers({ children }: PropsWithChildren) {
  return (
    <StateProvider>
      <TooltipProvider>{children}</TooltipProvider>
    </StateProvider>
  );
}

function options(kind: Kind = "mr", overrides: Partial<Options> = {}): Options {
  return {
    workspaceId: WORKSPACE_A,
    kind,
    presets: kind === "mr" ? MR_PRESETS : ISSUE_PRESETS,
    preset: ASSIGNED,
    customQuery: "",
    ...overrides,
  };
}

function renderSearch(initialProps: Options = options()) {
  return renderHook((props: Options) => useGitLabSearch(props), {
    initialProps,
    wrapper: Providers,
  });
}

function MRResults({ workspaceId }: { workspaceId: string }) {
  const search = useGitLabSearch(options("mr", { workspaceId }));
  return (
    <>
      <MRList items={search.items as MR[]} loading={search.loading} error={search.error} />
      <ResultsPagination
        page={search.page}
        pageSize={search.pageSize}
        total={search.total}
        onPageChange={search.setPage}
      />
    </>
  );
}

beforeEach(() => {
  transport.mrs.mockReset().mockImplementation(async (params) => mrPage(params));
  transport.issues.mockReset().mockImplementation(async (params) => issuePage(params));
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    settleHeldPages.splice(0).forEach((settle) => settle());
  });
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.5
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
describe("GitLab workspace page ownership", () => {
  it.each([
    ["MR", "mr"],
    ["issue", "issue"],
  ] as const)("resets %s page for a smaller workspace", async (_label, kind) => {
    const view = renderSearch(options(kind));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    act(() => view.result.current.setPage(3));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(51));

    view.rerender(options(kind, { workspaceId: WORKSPACE_B }));
    await waitFor(() => expect(view.result.current.loading).toBe(false));
    expect(view.result.current.page).toBe(1);
    expect(view.result.current.items).toEqual([kind === "mr" ? B_MR : B_ISSUE]);
    expect(view.result.current.total).toBe(1);
    if (kind === "mr") {
      expect(transport.issues).not.toHaveBeenCalled();
    } else {
      expect(transport.mrs).not.toHaveBeenCalled();
    }
  });

  it.each(KINDS)("keeps same-workspace page navigation on equal inputs (%s)", async (kind) => {
    const view = renderSearch(options(kind));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    act(() => view.result.current.setPage(3));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(51));
    const requests = transport.mrs.mock.calls.length + transport.issues.mock.calls.length;

    view.rerender(options(kind));
    await act(async () => Promise.resolve());
    expect(view.result.current.page).toBe(3);
    expect(view.result.current.items).toEqual(kind === "mr" ? A_MRS.slice(50) : A_ISSUES.slice(50));
    expect(transport.mrs.mock.calls.length + transport.issues.mock.calls.length).toBe(requests);
  });

  it.each(KINDS)("starts page one on a %s workspace roundtrip", async (kind) => {
    const view = renderSearch(options(kind));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    act(() => view.result.current.setPage(3));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(51));
    view.rerender(options(kind, { workspaceId: WORKSPACE_B }));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(kind === "mr" ? 101 : 201));

    view.rerender(options(kind));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    expect(view.result.current.page).toBe(1);
    expect(view.result.current.total).toBe(61);
  });
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.8
// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
describe("GitLab real browse results and pagination", () => {
  it("shows the new workspace row through real list and pagination", async () => {
    const view = render(<MRResults workspaceId={WORKSPACE_A} />, { wrapper: Providers });
    await screen.findByRole("link", { name: A_MRS[0].title });
    const navigation = screen.getByRole("navigation", { name: "pagination" });
    fireEvent.click(within(navigation).getByRole("link", { name: "3" }));
    await screen.findByRole("link", { name: A_MRS[50].title });
    expect(within(navigation).getByRole("link", { name: "3" }).getAttribute("aria-current")).toBe(
      "page",
    );

    view.rerender(<MRResults workspaceId={WORKSPACE_B} />);
    const link = await screen.findByRole("link", { name: B_MR.title });
    expect(link.getAttribute("href")).toBe(B_MR.web_url);
    expect(screen.queryByRole("link", { name: A_MRS[50].title })).toBeNull();
    expect(screen.queryByRole("navigation", { name: "pagination" })).toBeNull();
    expect(screen.getAllByTestId("mr-row")).toHaveLength(1);
  });

  it("shows a first-page row on initial small workspace", async () => {
    render(<MRResults workspaceId={WORKSPACE_B} />, { wrapper: Providers });
    const link = await screen.findByRole("link", { name: B_MR.title });
    expect(link.getAttribute("href")).toBe(B_MR.web_url);
    expect(screen.queryByRole("navigation", { name: "pagination" })).toBeNull();
    expect(screen.getAllByTestId("mr-row")).toHaveLength(1);
  });
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
describe("GitLab reset response ownership", () => {
  it.each([
    ["mr", "success"],
    ["mr", "failure"],
    ["issue", "success"],
    ["issue", "failure"],
  ] as const)(
    "ignores old-workspace and superseded B-page %s %s after B page one",
    async (kind, outcome) => {
      const heldWorkspaces: string[] = [];
      const settleLate: Array<() => void> = [];
      transport.mrs.mockImplementation((params) => {
        if (params.page === 3) {
          const page = deferPage(mrPage(params));
          heldWorkspaces.push(params.workspaceId);
          settleLate.push(() => {
            if (outcome === "failure") page.reject(new Error(TRANSPORT_FAILURE));
            else
              page.resolve({
                mrs: [makeMR(params.workspaceId, 999)],
                total_count: 99,
                page: 3,
                per_page: 25,
              });
          });
          return page.promise;
        }
        return Promise.resolve(mrPage(params));
      });
      transport.issues.mockImplementation((params) => {
        if (params.page === 3) {
          const page = deferPage(issuePage(params));
          heldWorkspaces.push(params.workspaceId);
          settleLate.push(() => {
            if (outcome === "failure") page.reject(new Error(TRANSPORT_FAILURE));
            else
              page.resolve({
                issues: [makeIssue(params.workspaceId, 999)],
                total_count: 99,
                page: 3,
                per_page: 25,
              });
          });
          return page.promise;
        }
        return Promise.resolve(issuePage(params));
      });
      const view = renderSearch(options(kind));
      const expected = [kind === "mr" ? B_MR : B_ISSUE];
      await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
      act(() => view.result.current.setPage(3));
      await waitFor(() => expect(heldWorkspaces).toContain(WORKSPACE_A));
      expect(view.result.current.loading).toBe(true);

      view.rerender(options(kind, { workspaceId: WORKSPACE_B }));
      expect(view.result.current.items).toEqual([]);
      await waitFor(() => expect(view.result.current.items).toEqual(expected));
      // Both superseded scopes must reach the transport boundary for this
      // late-response test; no request count is assumed for either scope.
      expect(heldWorkspaces).toContain(WORKSPACE_B);
      const acceptedAt = view.result.current.lastFetchedAt;
      expect(acceptedAt).toBeInstanceOf(Date);

      await act(async () => {
        settleLate.forEach((settle) => settle());
      });
      expect(view.result.current.items).toEqual(expected);
      expect(view.result.current.rawItems).toEqual(expected);
      expect(view.result.current.page).toBe(1);
      expect(view.result.current.total).toBe(1);
      expect(view.result.current.loading).toBe(false);
      expect(view.result.current.error).toBeNull();
      expect(view.result.current.lastFetchedAt).toBe(acceptedAt);
    },
  );
});

describe("GitLab pending workspace visibility", () => {
  // @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
  it("masks old items while the replacement page is pending", async () => {
    const heldB: Deferred<MRSearchPage>[] = [];
    transport.mrs.mockImplementation((params) => {
      if (params.workspaceId !== WORKSPACE_B) return Promise.resolve(mrPage(params));
      const page = deferPage(mrPage(params));
      heldB.push(page);
      return page.promise;
    });
    const view = renderSearch();
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    act(() => view.result.current.setPage(3));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(51));

    view.rerender(options("mr", { workspaceId: WORKSPACE_B }));
    expect(view.result.current.items).toEqual([]);
    expect(view.result.current.rawItems).toEqual([]);
    expect(view.result.current.total).toBe(0);
    expect(view.result.current.error).toBeNull();
    expect(view.result.current.loading).toBe(true);
    await act(async () => {
      heldB.forEach((page) => page.resolve({ mrs: [B_MR], total_count: 1, page: 1, per_page: 25 }));
    });
    await waitFor(() => expect(view.result.current.items).toEqual([B_MR]));
  });
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
describe("GitLab existing page reset inputs", () => {
  it.each(RESET_INPUTS)("still resets page one when %s changes", async (_label, change) => {
    const view = renderSearch();
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    act(() => view.result.current.setPage(3));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(51));
    view.rerender(options("mr", change));
    await waitFor(() => expect(view.result.current.page).toBe(1));
    await waitFor(() => expect(view.result.current.loading).toBe(false));
    expect(view.result.current.items[0]?.iid).toBe(1);
    if (change.kind === "issue") {
      expect(view.result.current.items[0]?.title).toBe(A_ISSUES[0].title);
    }
  });
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
describe("GitLab current result and refresh controls", () => {
  it.each(KINDS)("settles a current %s empty page", async (kind) => {
    transport.mrs.mockResolvedValue({ mrs: [], total_count: 0, page: 1, per_page: 25 });
    transport.issues.mockResolvedValue({ issues: [], total_count: 0, page: 1, per_page: 25 });
    const view = renderSearch(options(kind));
    await waitFor(() => expect(view.result.current.loading).toBe(false));
    expect(view.result.current.items).toEqual([]);
    expect(view.result.current.total).toBe(0);
    expect(view.result.current.error).toBeNull();
    expect(view.result.current.lastFetchedAt).toBeInstanceOf(Date);
  });

  it.each(KINDS)("surfaces a current %s failure and refreshes successfully", async (kind) => {
    transport.mrs.mockRejectedValueOnce(new Error(TRANSPORT_FAILURE));
    transport.issues.mockRejectedValueOnce(new Error(TRANSPORT_FAILURE));
    const view = renderSearch(options(kind));
    await waitFor(() => expect(view.result.current.error).toBe(TRANSPORT_FAILURE));
    expect(view.result.current.items).toEqual([]);
    expect(view.result.current.total).toBe(0);
    expect(view.result.current.loading).toBe(false);
    await act(async () => {
      await view.result.current.refresh();
    });
    expect(view.result.current.items[0]?.iid).toBe(1);
    expect(view.result.current.error).toBeNull();
    expect(view.result.current.loading).toBe(false);
  });

  it.each(KINDS)("refreshes the selected %s page after a workspace reset", async (kind) => {
    const view = renderSearch(options(kind));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    act(() => view.result.current.setPage(3));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(51));
    view.rerender(options(kind, { workspaceId: WORKSPACE_B }));
    await waitFor(() =>
      expect(view.result.current.items).toEqual([kind === "mr" ? B_MR : B_ISSUE]),
    );
    await act(async () => {
      await view.result.current.refresh();
    });
    const calls = kind === "mr" ? transport.mrs.mock.calls : transport.issues.mock.calls;
    expect(calls.at(-1)?.[0]).toMatchObject({ workspaceId: WORKSPACE_B, page: 1, perPage: 25 });

    view.rerender(options(kind));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
    act(() => view.result.current.setPage(2));
    await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(26));
    await act(async () => {
      await view.result.current.refresh();
    });
    const laterCalls = kind === "mr" ? transport.mrs.mock.calls : transport.issues.mock.calls;
    expect(laterCalls.at(-1)?.[0]).toMatchObject({
      workspaceId: WORKSPACE_A,
      page: 2,
      perPage: 25,
    });
    expect(view.result.current.page).toBe(2);
    expect(view.result.current.items[0]?.iid).toBe(26);
  });
});

// @covers AC-INTEGRATIONS-GITLAB-INTEGRATION-001.9
describe("GitLab disabled workspace transitions", () => {
  it.each(KINDS)(
    "resets disabled %s without fetching and loads page one when enabled",
    async (kind) => {
      const view = renderSearch(options(kind));
      await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(1));
      act(() => view.result.current.setPage(3));
      await waitFor(() => expect(view.result.current.items[0]?.iid).toBe(51));
      const requests = transport.mrs.mock.calls.length + transport.issues.mock.calls.length;

      view.rerender(options(kind, { workspaceId: WORKSPACE_B, enabled: false }));
      await act(async () => {
        await view.result.current.refresh();
      });
      expect(transport.mrs.mock.calls.length + transport.issues.mock.calls.length).toBe(requests);
      expect(view.result.current.page).toBe(1);
      expect(view.result.current.items).toEqual([]);
      expect(view.result.current.loading).toBe(false);

      view.rerender(options(kind, { workspaceId: WORKSPACE_B, enabled: true }));
      await waitFor(() =>
        expect(view.result.current.items).toEqual([kind === "mr" ? B_MR : B_ISSUE]),
      );
      expect(view.result.current.page).toBe(1);
    },
  );

  it("admits no fetch or refresh without a workspace and recovers when one is selected", async () => {
    const view = renderSearch(options("mr", { workspaceId: "" }));
    await act(async () => {
      await view.result.current.refresh();
    });
    expect(transport.mrs).not.toHaveBeenCalled();
    expect(transport.issues).not.toHaveBeenCalled();
    expect(view.result.current.items).toEqual([]);
    expect(view.result.current.loading).toBe(false);
    view.rerender(options("mr", { workspaceId: WORKSPACE_B }));
    await waitFor(() => expect(view.result.current.items).toEqual([B_MR]));
    expect(view.result.current.page).toBe(1);
  });
});
