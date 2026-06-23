import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { Branch } from "@/lib/types/http";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { UseAccessibleReposResult } from "@/hooks/domains/github/use-accessible-repos";

// Each test passes a stubbed `accessibleRepos` prop to the chip. The hook
// itself now lives at the chips-row level (see chips-row test); the chip is
// pure presentational glue over the result. Defaults to an empty/idle state
// and individual tests override the slice they care about.
type AccessibleRepo = {
  provider: "github" | "gitlab";
  owner: string;
  name: string;
  full_name: string;
  default_branch: string;
  description?: string;
  private: boolean;
};
function makeAccessible(
  overrides: Partial<UseAccessibleReposResult> = {},
): UseAccessibleReposResult {
  return {
    repos: [] as AccessibleRepo[],
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
    ...overrides,
  };
}

import { RemoteRepoChip, computeTriggerLabel } from "./task-create-dialog-remote-repo-chip";

const TRIGGER_TID = "remote-repo-chip-trigger";
const FULL_NAME = "acme/site";
const URL_ACME_SITE = "https://github.com/acme/site";

afterEach(() => {
  cleanup();
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
    const accessibleRepos = makeAccessible({
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
    });
    const onURLChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={accessibleRepos}
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
        accessibleRepos={makeAccessible()}
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
        accessibleRepos={makeAccessible()}
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
        accessibleRepos={makeAccessible()}
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
    const accessibleRepos = makeAccessible({
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
    });
    const onURLChange = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={accessibleRepos}
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
        accessibleRepos={makeAccessible()}
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
        accessibleRepos={makeAccessible()}
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
        accessibleRepos={makeAccessible()}
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

describe("RemoteRepoChip — option layout", () => {
  it("never renders an option description line, even when the repo has a description", () => {
    // Inverted from the original "renders description as a second line" test —
    // the description was dropped from the picker to keep each row compact and
    // one-line, after the picker switched to client-side filtering over a
    // single fetch (descriptions added visual noise without aiding the
    // case-insensitive full_name substring match).
    const accessibleRepos = makeAccessible({
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
    });
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={accessibleRepos}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    expect(screen.queryByTestId("remote-repo-option-description")).toBeNull();
    expect(screen.queryByText("The acme corporate website")).toBeNull();
    // The owner/name line is still rendered.
    expect(screen.getByText(FULL_NAME)).toBeTruthy();
  });
});

describe("RemoteRepoChip — picker loading state", () => {
  it("renders an inline spinner while the initial fetch is loading and no repos are yet available", () => {
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={makeAccessible({ loading: true })}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    const loadingNode = screen.getByTestId("remote-repo-picker-loading");
    expect(loadingNode.textContent).toContain("Loading repositories");
  });

  it("does NOT render the spinner once repos have loaded (even if loading flips true again later)", () => {
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={makeAccessible({
          loading: true,
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
        })}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    expect(screen.queryByTestId("remote-repo-picker-loading")).toBeNull();
    expect(screen.getByText(FULL_NAME)).toBeTruthy();
  });
});

describe("RemoteRepoChip — popover content", () => {
  it("portals and constrains the repo picker so dialog overflow cannot clip it", () => {
    renderInProvider(
      <div data-testid="clipping-host" className="overflow-hidden">
        <RemoteRepoChip
          row={row()}
          branches={[]}
          branchesLoading={false}
          accessibleRepos={makeAccessible()}
          onURLChange={vi.fn()}
          onBranchChange={noopBranch}
          onRemove={noopRemove}
        />
      </div>,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    const content = screen.getByTestId("remote-repo-popover-content");
    expect(screen.getByTestId("clipping-host").contains(content)).toBe(false);
    expect(content.className).toContain("max-w-[calc(100vw-2rem)]");
    expect(content.className).toContain("max-h-[min(420px,calc(100vh-12rem))]");
    expect(content.className).toContain("overflow-y-auto");
  });

  it("renders the 'Connect GitHub' banner when accessibleRepos.unavailable=true", () => {
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={makeAccessible({ unavailable: true })}
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
    const accessibleRepos = makeAccessible({
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
    });
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={accessibleRepos}
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
        accessibleRepos={makeAccessible()}
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
        accessibleRepos={makeAccessible()}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    // Short URLs render verbatim; the middle-ellipsis only fires past ~30 chars.
    expect(screen.getByTestId(TRIGGER_TID).textContent).toContain("github.com/foo/bar");
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
