import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { MergeRepo } from "@/lib/api/domains/coordinator-merge-api";

type HookState = {
  status: "loading" | "ready" | "error" | "off";
  repos: MergeRepo[];
  protection: unknown[];
  draft: Record<string, unknown>;
  saveError: string | null;
  refreshing: boolean;
  refreshFailed: boolean;
};
const choosePolicy = vi.fn();
const replacePrincipal = vi.fn();
const refreshProtection = vi.fn();
const retry = vi.fn();
let state: HookState;

vi.mock("@/hooks/domains/coordinator/use-merge-settings", () => ({
  useMergeSettings: () => ({
    ...state,
    retry,
    choosePolicy,
    replacePrincipal,
    refreshProtection,
  }),
}));

import { MergeSection } from "./merge-section";

function repo(id: string, policy: MergeRepo["policy"], active = true): MergeRepo {
  return { repository_id: id, name: `repo-${id}`, policy, automatic_active: active };
}

beforeEach(() => {
  vi.clearAllMocks();
  state = {
    status: "ready",
    repos: [repo("r1", "denied")],
    protection: [],
    draft: {},
    saveError: null,
    refreshing: false,
    refreshFailed: false,
  };
});
afterEach(cleanup);

function mount(canManage = true) {
  return render(<MergeSection workspaceId="w1" coordinatorId="c1" canManage={canManage} />);
}

describe("MergeSection", () => {
  it("states the four merge statements", () => {
    mount();
    const text = screen.getByTestId("merge-statements").textContent ?? "";
    expect(text).toContain("denied until a manager chooses another policy");
    expect(text).toContain("can change between the last check");
    expect(text).toContain("report no checks cannot be merged");
    expect(text).toContain("merge queue is not supported");
  });

  it("lists each repository and changes its policy", () => {
    mount();
    expect(screen.getByTestId("merge-repo-r1").textContent).toContain("repo-r1");
    fireEvent.click(screen.getByLabelText("Automatic"));
    expect(choosePolicy).toHaveBeenCalledWith(state.repos[0], "automatic");
  });

  it("shows the loading and failure states", () => {
    state.status = "loading";
    const { unmount } = mount();
    expect(screen.getByTestId("merge-loading")).toBeTruthy();
    unmount();
    state.status = "error";
    mount();
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(retry).toHaveBeenCalledTimes(1);
  });

  it("offers Replace execution manager only for an inactive automatic principal", () => {
    state.repos = [repo("r1", "automatic", true), repo("r2", "automatic", false)];
    mount();
    expect(screen.queryByTestId("merge-replace-r1")).toBeNull();
    fireEvent.click(screen.getByTestId("merge-replace-r2"));
    expect(replacePrincipal).toHaveBeenCalledWith(state.repos[1]);
  });

  it("refreshes protection and shows the save error", () => {
    state.saveError = "principal_valid";
    state.protection = [
      {
        repository_id: "r1",
        branch: "main",
        pipeline: "unknown",
        approvals: "enforced",
        threads: "not_enforced",
        observed_at: "2026-05-01T10:00:00Z",
      },
    ];
    mount();
    expect(screen.getByTestId("merge-save-error").textContent).toContain("still authorized");
    expect(
      screen.getAllByTestId("merge-protection-cell").map((c) => c.getAttribute("data-value")),
    ).toEqual(["unknown", "enforced", "not_enforced"]);
    fireEvent.click(screen.getByTestId("merge-protection-refresh"));
    expect(refreshProtection).toHaveBeenCalledTimes(1);
  });

  it("renders nothing when merge is off", () => {
    state.status = "off";
    const { container } = mount();
    expect(container.firstChild).toBeNull();
  });
});
