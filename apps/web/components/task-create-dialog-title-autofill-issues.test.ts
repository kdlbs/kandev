import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useDialogFormState } from "./task-create-dialog-state";

const prInfoMap = new Map<
  string,
  {
    issueNumber?: number;
    suggestedTitle: string;
  }
>();

vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({
    branches: () => [],
    loading: () => false,
    ensure: () => undefined,
  }),
}));

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

vi.mock("@/hooks/domains/settings/use-remote-auth-specs", () => ({
  useRemoteAuthSpecs: () => ({ specs: [], loaded: true }),
}));

const ISSUE_URL_1456 = "https://github.com/acme/site/issues/1456";
const ISSUE_TITLE_1456 = "Issue #1456: Fix remote picker";
const USER_TYPED_TITLE = "my own title";

function seedIssueInfo(url: string, issueNumber: number, suggestedTitle: string) {
  prInfoMap.set(url, {
    issueNumber,
    suggestedTitle,
  });
}

describe("useDialogFormState issue title autofill", () => {
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
