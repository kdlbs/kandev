import { describe, it, expect, vi, afterEach } from "vitest";
import { useState } from "react";
import { render, cleanup } from "@testing-library/react";
import type { Branch } from "@/lib/types/http";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { UseRemoteRepositoriesResult } from "@/hooks/domains/integrations/use-remote-repositories";
import type { PRInfo } from "@/hooks/domains/github/use-pr-info-by-url";
import { RemoteRepoChip } from "./task-create-dialog-remote-repo-chip";

// Branch auto-select tests for RemoteRepoChip, split out of the main chip test
// file to keep both under the 600-line cap. Shares the same lightweight harness
// (duplicated intentionally — these lint rules are per-file).
function makeAccessible(
  overrides: Partial<UseRemoteRepositoriesResult> = {},
): UseRemoteRepositoriesResult {
  return {
    repos: [],
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
    ...overrides,
  };
}

const URL_PR_42 = "https://github.com/acme/site/pull/42";
const HEAD_BRANCH = "feature/pr-branch";
const PR_INFO_FEATURE: PRInfo = {
  prHeadBranch: HEAD_BRANCH,
  prBaseBranch: "main",
  prNumber: 42,
  suggestedTitle: "PR #42: x",
};

afterEach(() => {
  cleanup();
});

function row(overrides: Partial<TaskRemoteRepoRow> = {}): TaskRemoteRepoRow {
  return { key: "remote-0", url: "", branch: "", source: "paste", ...overrides };
}

function renderInProvider(ui: Parameters<typeof render>[0]) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

const noopRemove = () => undefined;

describe("RemoteRepoChip — per-row branch auto-select", () => {
  it("auto-selects the PR head branch when prInfo arrives and row.branch is empty", () => {
    const onBranchChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: URL_PR_42 })}
        branches={[{ name: "main", type: "remote" }]}
        branchesLoading={false}
        accessibleRepos={makeAccessible()}
        prInfo={PR_INFO_FEATURE}
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />,
    );
    expect(onBranchChange).toHaveBeenCalledWith(HEAD_BRANCH);
  });

  it("does NOT overwrite a user-picked branch when PR info arrives later", () => {
    // Regression guard for the "PR info should not clobber user pick" case:
    // once the user has picked or accepted a branch (row.branch non-empty),
    // the chip never overwrites it — re-paste / clear is required to reset.
    const onBranchChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: URL_PR_42, branch: "develop" })}
        branches={[
          { name: "main", type: "remote" },
          { name: "develop", type: "remote" },
        ]}
        branchesLoading={false}
        accessibleRepos={makeAccessible()}
        prInfo={PR_INFO_FEATURE}
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />,
    );
    expect(onBranchChange).not.toHaveBeenCalled();
  });

  it("surfaces a fork PR head branch even when it isn't in the base repo's branch list", () => {
    // Fork PRs: PR head lives only on the contributor's fork, so the base
    // repo's branch list won't contain it. The chip still surfaces the head
    // name so the pill matches the URL the user just pasted.
    const onBranchChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: "https://github.com/acme/site/pull/977" })}
        branches={[
          { name: "main", type: "remote" },
          { name: "develop", type: "remote" },
        ]}
        branchesLoading={false}
        accessibleRepos={makeAccessible()}
        prInfo={{
          prHeadBranch: "fork-only-branch",
          prBaseBranch: "main",
          prNumber: 977,
          suggestedTitle: "PR #977: x",
        }}
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />,
    );
    expect(onBranchChange).toHaveBeenCalledWith("fork-only-branch");
  });
});

describe("RemoteRepoChip — per-row branch auto-select (no PR info)", () => {
  it("falls back to 'main' when there is no PR info and branches have loaded", () => {
    const onBranchChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: "https://github.com/acme/site" })}
        branches={[
          { name: "feature/y", type: "remote" },
          { name: "main", type: "remote" },
        ]}
        branchesLoading={false}
        accessibleRepos={makeAccessible()}
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />,
    );
    expect(onBranchChange).toHaveBeenCalledWith("main");
  });

  it("does nothing when the row has no URL yet", () => {
    const onBranchChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[{ name: "main", type: "remote" }]}
        branchesLoading={false}
        accessibleRepos={makeAccessible()}
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />,
    );
    expect(onBranchChange).not.toHaveBeenCalled();
  });
});

// Controlled harness: mirrors the real parent by writing onBranchChange back
// into row.branch, so the auto-select effect sees the value it set on the next
// render. Lets us simulate the list-resolves-then-PR-arrives sequence.
function ControlledChip(props: {
  url: string;
  initialBranch?: string;
  branches: Branch[];
  prInfo?: PRInfo;
  onBranchChange: (b: string) => void;
}) {
  const [branch, setBranch] = useState(props.initialBranch ?? "");
  return (
    <RemoteRepoChip
      row={row({ url: props.url, branch })}
      branches={props.branches}
      branchesLoading={false}
      accessibleRepos={makeAccessible()}
      prInfo={props.prInfo}
      onURLChange={vi.fn()}
      onBranchChange={(b) => {
        setBranch(b);
        props.onBranchChange(b);
      }}
      onRemove={noopRemove}
    />
  );
}

describe("RemoteRepoChip — PR head outranks list default (resolve-order independence)", () => {
  it("applies the PR head even when the branch list resolved first and set 'main'", () => {
    // Item-2 regression: list resolves first → auto-select writes 'main'; then
    // prInfo arrives. The PR head must REPLACE the list-derived 'main' default,
    // not be skipped because row.branch is already non-empty.
    const onBranchChange = vi.fn();
    const branches: Branch[] = [{ name: "main", type: "remote" }];
    const chip = (prInfo?: PRInfo) => (
      <ControlledChip
        url={URL_PR_42}
        branches={branches}
        prInfo={prInfo}
        onBranchChange={onBranchChange}
      />
    );
    const { rerender } = renderInProvider(chip());
    // After the first render, the auto-selector should have written 'main'.
    expect(onBranchChange).toHaveBeenLastCalledWith("main");
    // Now prInfo arrives → PR head replaces the list-derived 'main'.
    rerender(<TooltipProvider>{chip(PR_INFO_FEATURE)}</TooltipProvider>);
    expect(onBranchChange).toHaveBeenLastCalledWith(HEAD_BRANCH);
  });

  it("does NOT overwrite a genuine user pick when prInfo arrives later", () => {
    // The user picked 'develop' (initialBranch differs from anything the
    // auto-selector would have written), then prInfo arrives — the pick stays.
    const onBranchChange = vi.fn();
    const branches: Branch[] = [
      { name: "main", type: "remote" },
      { name: "develop", type: "remote" },
    ];
    const chip = (prInfo?: PRInfo) => (
      <ControlledChip
        url={URL_PR_42}
        initialBranch="develop"
        branches={branches}
        prInfo={prInfo}
        onBranchChange={onBranchChange}
      />
    );
    const { rerender } = renderInProvider(chip());
    expect(onBranchChange).not.toHaveBeenCalled();
    rerender(<TooltipProvider>{chip(PR_INFO_FEATURE)}</TooltipProvider>);
    expect(onBranchChange).not.toHaveBeenCalled();
  });
});

describe("RemoteRepoChip — branch ownership resets on URL change", () => {
  it("does not auto-overwrite a branch carried into a newly selected URL", () => {
    // Regression for lastUrlRef: switching URLs must reset auto-set ownership,
    // so a value equal to the prior URL's auto-set branch isn't clobbered to the
    // new URL's list default.
    const onBranchChange = vi.fn();
    const chip = (url: string, branch: string, names: string[]) => (
      <RemoteRepoChip
        row={row({ url, branch })}
        branches={names.map((name): Branch => ({ name, type: "remote" }))}
        branchesLoading={false}
        accessibleRepos={makeAccessible()}
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />
    );
    const { rerender } = renderInProvider(chip("https://github.com/acme/a", "", ["main"]));
    expect(onBranchChange).toHaveBeenLastCalledWith("main");
    onBranchChange.mockClear();
    // URL B carries "main" but its list has no "main"; ownership reset → preserve.
    rerender(
      <TooltipProvider>
        {chip("https://github.com/acme/b", "main", ["develop", "trunk"])}
      </TooltipProvider>,
    );
    expect(onBranchChange).not.toHaveBeenCalled();
  });
});
