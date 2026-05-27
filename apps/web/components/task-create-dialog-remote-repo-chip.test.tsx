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

import { RemoteRepoChip } from "./task-create-dialog-remote-repo-chip";

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
  it("picker selection writes URL + picker metadata via onURLChange", () => {
    accessibleReposState.value = {
      ...accessibleReposState.value,
      repos: [
        { provider: "github", owner: "acme", name: "site", full_name: FULL_NAME, private: false },
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
