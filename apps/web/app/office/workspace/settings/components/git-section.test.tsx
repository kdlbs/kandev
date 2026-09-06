import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { GitStatusData } from "@/lib/api/domains/office-api";

const getGitStatusMock = vi.fn<(workspaceId: string) => Promise<GitStatusData>>();
const gitCloneMock = vi.fn();
const gitPullMock = vi.fn();
const gitPushMock = vi.fn();

vi.mock("@/lib/api/domains/office-api", () => ({
  getGitStatus: (workspaceId: string) => getGitStatusMock(workspaceId),
  gitClone: (...args: unknown[]) => gitCloneMock(...args),
  gitPull: (...args: unknown[]) => gitPullMock(...args),
  gitPush: (...args: unknown[]) => gitPushMock(...args),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string } }) => unknown) =>
    selector({ workspaces: { activeId: "workspace-1" } }),
}));

vi.mock("@/lib/toast/sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

let configSyncActive = false;
vi.mock("@/hooks/domains/office/use-office-config-sync-active", () => ({
  useOfficeConfigSyncActive: () => configSyncActive,
}));

import { GitSection } from "./git-section";

const PULL_TEST_ID = "office-git-pull";

function notGit(): GitStatusData {
  return {
    is_git: false,
    is_dirty: false,
    has_remote: false,
    ahead: 0,
    behind: 0,
    commit_count: 0,
  };
}

function gitStatus(overrides: Partial<GitStatusData> = {}): GitStatusData {
  return {
    is_git: true,
    branch: "main",
    is_dirty: false,
    has_remote: true,
    ahead: 0,
    behind: 0,
    commit_count: 3,
    ...overrides,
  };
}

describe("GitSection — config sync mutual exclusion (AC-OFFICE-CONFIG-SYNC-006.6)", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    configSyncActive = false;
  });

  afterEach(cleanup);

  it("hides the guard reason and leaves Clone gated only by the URL field when config sync is inactive", async () => {
    getGitStatusMock.mockResolvedValue(notGit());
    render(<GitSection />);
    await waitFor(() => expect(screen.getByTestId("office-git-clone")).toBeTruthy());

    // Still disabled (no repo URL entered), but not because of the guard.
    expect(screen.queryByTestId("office-git-clone-disabled-reason")).toBeNull();
  });

  it("disables Clone with a reason when config sync is active", async () => {
    configSyncActive = true;
    getGitStatusMock.mockResolvedValue(notGit());
    render(<GitSection />);
    await waitFor(() => expect(screen.getByTestId("office-git-clone")).toBeTruthy());

    expect((screen.getByTestId("office-git-clone") as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("office-git-clone-disabled-reason")).toBeTruthy();
  });

  it("disables Pull but leaves Push available when config sync is active", async () => {
    configSyncActive = true;
    getGitStatusMock.mockResolvedValue(gitStatus());
    render(<GitSection />);
    await waitFor(() => expect(screen.getByTestId(PULL_TEST_ID)).toBeTruthy());

    expect((screen.getByTestId(PULL_TEST_ID) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId("office-git-pull-disabled-reason")).toBeTruthy();
    expect((screen.getByRole("button", { name: /push/i }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("keeps Pull enabled when config sync is inactive", async () => {
    getGitStatusMock.mockResolvedValue(gitStatus());
    render(<GitSection />);
    await waitFor(() => expect(screen.getByTestId(PULL_TEST_ID)).toBeTruthy());

    expect((screen.getByTestId(PULL_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
    expect(screen.queryByTestId("office-git-pull-disabled-reason")).toBeNull();
  });
});
