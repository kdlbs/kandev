import { describe, expect, it } from "vitest";
import {
  canChooseCheckoutBranchForSource,
  getWorkspaceSourceCapabilities,
  hasCloneableSavedRepository,
} from "./executor-capabilities";

describe("getWorkspaceSourceCapabilities", () => {
  it.each(["local", "local_pc", "worktree"] as const)("offers folders for %s", (executorType) => {
    expect(getWorkspaceSourceCapabilities(executorType).canAddFolders).toBe(true);
  });

  it.each(["local_docker", "remote_docker", "ssh", "sprites"] as const)(
    "hides folders for %s",
    (executorType) => {
      expect(getWorkspaceSourceCapabilities(executorType).canAddFolders).toBe(false);
    },
  );

  it.each(["local", "local_pc"] as const)(
    "uses the live checkout and does not offer a checkout branch for %s",
    (executorType) => {
      expect(getWorkspaceSourceCapabilities(executorType)).toMatchObject({
        canChooseCheckoutBranch: false,
        requiresCloneableLocalRepository: false,
      });
    },
  );

  it.each(["worktree", "local_docker", "remote_docker", "ssh", "sprites"] as const)(
    "offers a checkout branch for owned clones on %s",
    (executorType) => {
      expect(getWorkspaceSourceCapabilities(executorType)).toMatchObject({
        canChooseCheckoutBranch: true,
      });
    },
  );

  it.each(["local_docker", "remote_docker", "ssh", "sprites"] as const)(
    "requires a cloneable origin for a selected local repository on %s",
    (executorType) => {
      expect(getWorkspaceSourceCapabilities(executorType)).toMatchObject({
        requiresCloneableLocalRepository: true,
      });
    },
  );

  it.each([
    ["a valid remote URL", { remote_url: "https://git.example.com/acme/api.git" }, true],
    [
      "a supported provider owner and name",
      { provider: "github", provider_owner: "acme", provider_name: "api" },
      true,
    ],
    ["a provider ID without owner and name", { provider: "github", provider_repo_id: "42" }, false],
    ["no remote locator", {}, false],
  ])("recognizes %s as cloneable: %s", (_, repository, expected) => {
    expect(hasCloneableSavedRepository(repository)).toBe(expected);
  });

  it("offers checkout selection for local Git sources in owned runtimes", () => {
    expect(canChooseCheckoutBranchForSource("local_repository", "worktree")).toBe(true);
    expect(canChooseCheckoutBranchForSource("local_repository", "local_pc")).toBe(false);
  });
});
