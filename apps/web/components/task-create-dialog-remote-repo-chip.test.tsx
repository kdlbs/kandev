import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { Branch } from "@/lib/types/http";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import { TooltipProvider } from "@kandev/ui/tooltip";

// Mocked hook so the chip's repo-picker popover renders deterministic options
// (and we can flip `unavailable` to true to assert the "Connect GitHub" banner).
type AccessibleRepo = {
  provider: "github" | "gitlab";
  owner: string;
  name: string;
  full_name: string;
  default_branch: string;
  description?: string;
  private: boolean;
};
const accessibleReposState = vi.hoisted(
  (): {
    value: {
      repos: AccessibleRepo[];
      loading: boolean;
      unavailable: boolean;
      error: Error | null;
      search: (q: string) => void;
    };
  } => ({
    value: { repos: [], loading: false, unavailable: false, error: null, search: () => undefined },
  }),
);

vi.mock("@/hooks/domains/github/use-accessible-repos", () => ({
  useAccessibleRepos: () => accessibleReposState.value,
}));

import { RemoteRepoChip, computeTriggerLabel } from "./task-create-dialog-remote-repo-chip";

const TRIGGER_TID = "remote-repo-chip-trigger";
const FULL_NAME = "acme/site";
const URL_ACME_SITE = "https://github.com/acme/site";

afterEach(() => {
  cleanup();
  accessibleReposState.value = {
    repos: [],
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
  };
});

function row(overrides: Partial<TaskRemoteRepoRow> = {}): TaskRemoteRepoRow {
  return { key: "remote-0", url: "", branch: "", source: "paste", ...overrides };
}

function renderInProvider(ui: Parameters<typeof render>[0]) {
  return render(<TooltipProvider>{ui}</TooltipProvider>);
}

const noopBranch = () => undefined;
const noopRemove = () => undefined;

describe("RemoteRepoChip — write paths", () => {
  it("picker selection writes URL + picker metadata (incl. default_branch) via onURLChange", () => {
    accessibleReposState.value = {
      ...accessibleReposState.value,
      repos: [
        {
          provider: "github",
          owner: "acme",
          name: "site",
          full_name: FULL_NAME,
          default_branch: "trunk",
          private: false,
        },
      ],
    };
    const onURLChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={onURLChange}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    fireEvent.click(screen.getByText(FULL_NAME));
    expect(onURLChange).toHaveBeenCalledWith(URL_ACME_SITE, "picker", {
      provider: "github",
      fullName: FULL_NAME,
      defaultBranch: "trunk",
    });
  });

  it("paste input writes URL with source=paste (no metadata) on Enter", () => {
    const onURLChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={onURLChange}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    const input = screen.getByTestId("remote-paste-url-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "https://github.com/acme/api" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(onURLChange).toHaveBeenCalledWith("https://github.com/acme/api", "paste");
  });

  it("paste input also commits on blur", () => {
    const onURLChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={onURLChange}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    const input = screen.getByTestId("remote-paste-url-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "https://github.com/foo/bar" } });
    fireEvent.blur(input);
    expect(onURLChange).toHaveBeenCalledWith("https://github.com/foo/bar", "paste");
  });

  it("calls onRemove when the X button is clicked", () => {
    const onRemove = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: URL_ACME_SITE })}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={onRemove}
      />,
    );
    fireEvent.click(screen.getByTestId("remote-chip-remove"));
    expect(onRemove).toHaveBeenCalledOnce();
  });
});

describe("RemoteRepoChip — paste/picker race", () => {
  it("picker click after typing in paste input does not trigger paste commit", () => {
    // Race: user types into paste input, then clicks a picker option. The
    // input's onBlur fires first (focus moves to the option button). Without
    // the guard, blur would commit the typed value AND close the popover,
    // and the subsequent picker click would be dropped.
    accessibleReposState.value = {
      ...accessibleReposState.value,
      repos: [
        {
          provider: "github",
          owner: "acme",
          name: "site",
          full_name: FULL_NAME,
          default_branch: "main",
          private: false,
        },
      ],
    };
    const onURLChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={onURLChange}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    const input = screen.getByTestId("remote-paste-url-input") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "https://github.com/typed/value" } });
    // Simulate focus moving to the picker option before the click lands —
    // this is the exact ordering the browser produces (blur → click).
    const option = screen.getByText(FULL_NAME).closest("button") as HTMLButtonElement;
    fireEvent.blur(input, { relatedTarget: option });
    fireEvent.click(option);
    expect(onURLChange).toHaveBeenCalledTimes(1);
    expect(onURLChange).toHaveBeenCalledWith(URL_ACME_SITE, "picker", {
      provider: "github",
      fullName: FULL_NAME,
      defaultBranch: "main",
    });
  });
});

describe("RemoteRepoChip — branch pill", () => {
  it("is disabled when the URL is empty", () => {
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    const branchTrigger = screen.getByTestId("remote-branch-chip-trigger") as HTMLButtonElement;
    expect(branchTrigger.disabled).toBe(true);
  });

  it("enables once URL is present and branches load", () => {
    const branches: Branch[] = [
      { name: "main", type: "remote", remote: "origin" },
      { name: "develop", type: "remote", remote: "origin" },
    ];
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: URL_ACME_SITE })}
        branches={branches}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    const branchTrigger = screen.getByTestId("remote-branch-chip-trigger") as HTMLButtonElement;
    expect(branchTrigger.disabled).toBe(false);
  });

  it("is enabled when the row already has a branch even if branch options haven't loaded yet", () => {
    // Picker pre-fill sets `row.branch` before the branch list fetch finishes.
    // The pill must show the value as the active selection rather than
    // greying out and confusing the user into thinking pre-fill failed.
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: URL_ACME_SITE, branch: "trunk" })}
        branches={[]}
        branchesLoading={true}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    const branchTrigger = screen.getByTestId("remote-branch-chip-trigger") as HTMLButtonElement;
    expect(branchTrigger.disabled).toBe(false);
    expect(branchTrigger.textContent).toContain("trunk");
  });
});

describe("RemoteRepoChip — option description", () => {
  it("renders the description as a second line when present", () => {
    accessibleReposState.value = {
      ...accessibleReposState.value,
      repos: [
        {
          provider: "github",
          owner: "acme",
          name: "site",
          full_name: FULL_NAME,
          default_branch: "main",
          description: "The acme corporate website",
          private: false,
        },
      ],
    };
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    expect(screen.getByTestId("remote-repo-option-description").textContent).toContain(
      "The acme corporate website",
    );
  });

  it("omits the description line entirely when description is missing or empty", () => {
    accessibleReposState.value = {
      ...accessibleReposState.value,
      repos: [
        {
          provider: "github",
          owner: "acme",
          name: "site",
          full_name: FULL_NAME,
          default_branch: "main",
          private: false,
        },
        {
          provider: "github",
          owner: "acme",
          name: "blank",
          full_name: "acme/blank",
          default_branch: "main",
          description: "",
          private: false,
        },
      ],
    };
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    expect(screen.queryByTestId("remote-repo-option-description")).toBeNull();
  });
});

describe("RemoteRepoChip — popover content", () => {
  it("renders the 'Connect GitHub' banner when useAccessibleRepos returns unavailable=true", () => {
    accessibleReposState.value = { ...accessibleReposState.value, unavailable: true };
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    expect(screen.getByText(/Connect a GitHub account/i)).toBeTruthy();
    const link = screen.getByRole("link", { name: /settings/i }) as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/settings/integrations/github");
  });

  it("renders 'private' badge next to private repo options", () => {
    accessibleReposState.value = {
      ...accessibleReposState.value,
      repos: [
        {
          provider: "github",
          owner: "acme",
          name: "secret",
          full_name: "acme/secret",
          default_branch: "main",
          private: true,
        },
      ],
    };
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    expect(screen.getByText(/private/i)).toBeTruthy();
  });
});

describe("RemoteRepoChip — trigger label", () => {
  it("displays picker label (owner/name) when row has picker metadata", () => {
    renderInProvider(
      <RemoteRepoChip
        row={row({
          url: URL_ACME_SITE,
          source: "picker",
          provider: "github",
          fullName: FULL_NAME,
        })}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    expect(screen.getByTestId(TRIGGER_TID).textContent).toContain(FULL_NAME);
  });

  it("displays the raw URL when source is 'paste'", () => {
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: "https://github.com/foo/bar", source: "paste" })}
        branches={[]}
        branchesLoading={false}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    // Short URLs render verbatim; the middle-ellipsis only fires past ~30 chars.
    expect(screen.getByTestId(TRIGGER_TID).textContent).toContain("github.com/foo/bar");
  });
});

describe("RemoteRepoChip — per-row branch auto-select", () => {
  it("auto-selects the PR head branch when prInfo arrives and row.branch is empty", () => {
    const onBranchChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: "https://github.com/acme/site/pull/42" })}
        branches={[{ name: "main", type: "remote" }]}
        branchesLoading={false}
        prInfo={{
          prHeadBranch: "feature/pr-branch",
          prBaseBranch: "main",
          prNumber: 42,
          suggestedTitle: "PR #42: x",
        }}
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />,
    );
    expect(onBranchChange).toHaveBeenCalledWith("feature/pr-branch");
  });

  it("does NOT overwrite a user-picked branch when PR info arrives later", () => {
    // Regression guard for the "PR info should not clobber user pick" case:
    // once the user has picked or accepted a branch (row.branch non-empty),
    // the chip never overwrites it — re-paste / clear is required to reset.
    const onBranchChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row({ url: "https://github.com/acme/site/pull/42", branch: "develop" })}
        branches={[
          { name: "main", type: "remote" },
          { name: "develop", type: "remote" },
        ]}
        branchesLoading={false}
        prInfo={{
          prHeadBranch: "feature/pr-branch",
          prBaseBranch: "main",
          prNumber: 42,
          suggestedTitle: "PR #42: x",
        }}
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
        onURLChange={vi.fn()}
        onBranchChange={onBranchChange}
        onRemove={noopRemove}
      />,
    );
    expect(onBranchChange).not.toHaveBeenCalled();
  });
});

describe("computeTriggerLabel", () => {
  it("returns the empty-state placeholder when url is empty", () => {
    expect(computeTriggerLabel(row())).toBe("Pick or paste a repo");
  });

  it("returns picker fullName when source is 'picker' and metadata is present", () => {
    expect(
      computeTriggerLabel(
        row({
          url: "https://github.com/octocat/hello-world",
          source: "picker",
          provider: "github",
          fullName: "octocat/hello-world",
        }),
      ),
    ).toBe("octocat/hello-world");
  });

  it("returns short paste URLs verbatim (no ellipsis under threshold)", () => {
    const label = computeTriggerLabel(row({ url: "github.com/x/y", source: "paste" }));
    expect(label).toBe("github.com/x/y");
    expect(label).not.toContain("…");
  });

  it("middle-truncates long paste URLs while preserving first and last chars", () => {
    const long = "github.com/some-very-long-org/some-very-long-repo-name";
    const stripped = "github.com/some-very-long-org/some-very-long-repo-name"; // no scheme to strip
    const label = computeTriggerLabel(row({ url: long, source: "paste" }));
    expect(label).toContain("…");
    expect(label.length).toBeLessThan(stripped.length);
    expect(label.startsWith(stripped[0]!)).toBe(true);
    expect(label.endsWith(stripped[stripped.length - 1]!)).toBe(true);
  });

  it("strips https:// and www. prefixes from paste labels", () => {
    expect(
      computeTriggerLabel(row({ url: "https://www.github.com/foo/bar", source: "paste" })),
    ).toBe("github.com/foo/bar");
  });
});
