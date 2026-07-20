import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, fireEvent, cleanup } from "@testing-library/react";
import type { Branch } from "@/lib/types/http";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { UseRemoteRepositoriesResult } from "@/hooks/domains/integrations/use-remote-repositories";

// Each test passes a stubbed `accessibleRepos` prop to the chip. The hook
// itself now lives at the chips-row level (see chips-row test); the chip is
// pure presentational glue over the result. Defaults to an empty/idle state
// and individual tests override the slice they care about.
type AccessibleRepo = {
  provider: "github" | "gitlab" | "azure_devops";
  owner: string;
  name: string;
  full_name: string;
  default_branch: string;
  description?: string;
  private: boolean;
};
type AccessibleOverrides = Omit<Partial<UseRemoteRepositoriesResult>, "repos"> & {
  repos?: AccessibleRepo[];
};
function makeAccessible(overrides: AccessibleOverrides = {}): UseRemoteRepositoriesResult {
  const repos = (overrides.repos ?? []).map((repo) => ({
    provider: repo.provider,
    id: repo.full_name,
    owner: repo.owner,
    name: repo.name,
    fullName: repo.full_name,
    url: remoteTestURL(repo),
    defaultBranch: repo.default_branch,
    private: repo.private,
  }));
  return {
    loading: false,
    unavailable: false,
    error: null,
    search: () => undefined,
    ...overrides,
    repos,
  };
}

function remoteTestURL(repo: AccessibleRepo): string {
  if (repo.provider === "azure_devops") {
    return `https://dev.azure.com/acme/${repo.owner}/_git/${repo.name}`;
  }
  return `https://${repo.provider}.com/${repo.owner}/${repo.name}`;
}

import { RemoteRepoChip } from "./task-create-dialog-remote-repo-chip";

const TRIGGER_TID = "remote-repo-chip-trigger";
const INPUT_TID = "remote-repo-input";
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
    expect(onURLChange).toHaveBeenCalledWith(
      URL_ACME_SITE,
      "picker",
      expect.objectContaining({
        provider: "github",
        fullName: FULL_NAME,
        defaultBranch: "trunk",
      }),
    );
  });

  it("selects an Azure DevOps repository with provider metadata", () => {
    const accessibleRepos = makeAccessible({
      repos: [
        {
          provider: "azure_devops",
          owner: "Platform",
          name: "api",
          full_name: "Platform/api",
          default_branch: "main",
          private: true,
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
    fireEvent.click(screen.getByRole("button", { name: /Platform\/api/i }));
    expect(onURLChange).toHaveBeenCalledWith(
      "https://dev.azure.com/acme/Platform/_git/api",
      "picker",
      expect.objectContaining({ provider: "azure_devops", fullName: "Platform/api" }),
    );
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

describe("RemoteRepoChip — unified search", () => {
  it("uses ordinary text as repository search without committing it on blur", () => {
    const onURLChange = vi.fn();
    const search = vi.fn();
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={makeAccessible({ search })}
        onURLChange={onURLChange}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    const input = screen.getByTestId(INPUT_TID) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "acme" } });
    fireEvent.blur(input);
    expect(search).toHaveBeenLastCalledWith("acme");
    expect(input.getAttribute("aria-invalid")).toBeNull();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(onURLChange).not.toHaveBeenCalled();
  });

  it("picker click after searching commits only the selected repository", () => {
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
    const input = screen.getByTestId(INPUT_TID) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "acme" } });
    const option = screen.getByText(FULL_NAME).closest("button") as HTMLButtonElement;
    fireEvent.blur(input, { relatedTarget: option });
    fireEvent.click(option);
    expect(onURLChange).toHaveBeenCalledTimes(1);
    expect(onURLChange).toHaveBeenCalledWith(
      URL_ACME_SITE,
      "picker",
      expect.objectContaining({
        provider: "github",
        fullName: FULL_NAME,
        defaultBranch: "main",
      }),
    );
  });
});

describe("RemoteRepoChip — picker focus", () => {
  it("does not commit a typed URL on blur when focus moves to a repository option", () => {
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
    const input = screen.getByTestId(INPUT_TID) as HTMLInputElement;
    fireEvent.change(input, { target: { value: URL_ACME_SITE } });
    const option = screen.getByText(FULL_NAME).closest("button") as HTMLButtonElement;
    fireEvent.blur(input, { relatedTarget: option });
    fireEvent.click(option);

    expect(onURLChange).toHaveBeenCalledTimes(1);
    expect(onURLChange).toHaveBeenCalledWith(
      URL_ACME_SITE,
      "picker",
      expect.objectContaining({
        provider: "github",
        fullName: FULL_NAME,
        defaultBranch: "main",
      }),
    );
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
    expect(loadingNode.parentElement?.className).toContain("h-56");
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

  it("does not render a loading spinner with the GitHub connection banner", () => {
    renderInProvider(
      <RemoteRepoChip
        row={row()}
        branches={[]}
        branchesLoading={false}
        accessibleRepos={makeAccessible({ unavailable: true, loading: true })}
        onURLChange={vi.fn()}
        onBranchChange={noopBranch}
        onRemove={noopRemove}
      />,
    );
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    expect(screen.getByText(/Connect a source control provider/i)).toBeTruthy();
    expect(screen.queryByTestId("remote-repo-picker-loading")).toBeNull();
  });

  it("shows only the connection banner for invalid URL input when GitHub is unavailable", () => {
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
    const input = screen.getByTestId(INPUT_TID) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "https://bitbucket.org/acme/api" } });
    fireEvent.keyDown(input, { key: "Enter" });

    expect(screen.getByText(/Connect a source control provider/i)).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(input.getAttribute("aria-invalid")).toBeNull();
  });
});

describe("RemoteRepoChip — popover content", () => {
  it("renders one top-level input for both repository search and GitHub URLs", () => {
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
    fireEvent.click(screen.getByTestId(TRIGGER_TID));
    const input = screen.getByTestId(INPUT_TID) as HTMLInputElement;
    expect(input.placeholder).toBe("Search repositories or paste a remote URL");
    expect(screen.queryByTestId("remote-repo-search")).toBeNull();
    expect(screen.queryByTestId("remote-paste-url-input")).toBeNull();
  });

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
    expect(content.className).toContain("overflow-hidden");
  });

  it("renders the provider settings banner when no integration is available", () => {
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
    expect(screen.getByText(/Connect a source control provider/i)).toBeTruthy();
    const link = screen.getByRole("link", { name: /settings/i }) as HTMLAnchorElement;
    expect(link.getAttribute("href")).toBe("/settings/integrations");
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
