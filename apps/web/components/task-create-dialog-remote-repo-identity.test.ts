import { describe, expect, it } from "vitest";
import type { TaskRemoteRepoRow } from "./task-create-dialog-types";
import {
  remoteRepositoryMatchesSelection,
  selectedRemoteRepositoryIdentity,
} from "./task-create-dialog-remote-repo-identity";

const GITHUB_REPOSITORY_URL = "https://github.com/acme/site";

function row(overrides: Partial<TaskRemoteRepoRow> = {}): TaskRemoteRepoRow {
  return { key: "remote-0", url: "", branch: "", source: "paste", ...overrides };
}

describe("Remote repository identities", () => {
  it("prefers provider-qualified IDs when metadata is complete", () => {
    expect(
      selectedRemoteRepositoryIdentity(
        row({
          url: "https://github.com/another/repository",
          source: "picker",
          provider: "github",
          providerRepoId: "acme/site",
        }),
      ),
    ).toBe("github:id:acme/site");
  });

  it.each([
    ["GitHub SSH", "git@github.com:acme/site.git", GITHUB_REPOSITORY_URL],
    [
      "www GitHub tree",
      "https://www.github.com/acme/site/tree/feature/docs",
      GITHUB_REPOSITORY_URL,
    ],
    ["GitHub pull request", "https://github.com/acme/site/pull/42/files", GITHUB_REPOSITORY_URL],
    ["GitHub blob", "https://github.com/acme/site/blob/main/src/index.ts", GITHUB_REPOSITORY_URL],
    ["GitLab SSH", "git@gitlab.com:acme/site.git", "https://gitlab.com/acme/site"],
    [
      "Azure DevOps SSH",
      "git@ssh.dev.azure.com:v3/acme/Platform/api",
      "https://dev.azure.com/acme/Platform/_git/api",
    ],
  ])("equates %s fallback URLs with their picker repository", (_label, fallback, picker) => {
    expect(selectedRemoteRepositoryIdentity(row({ url: fallback }))).toBe(
      selectedRemoteRepositoryIdentity(row({ url: picker })),
    );
  });

  it.each([
    ["HTTPS", "https://github.com/AcMe/SiTe"],
    ["SSH", "git@github.com:AcMe/SiTe.git"],
  ])("normalizes GitHub %s owner and repository case", (_transport, url) => {
    expect(selectedRemoteRepositoryIdentity(row({ url }))).toBe(
      selectedRemoteRepositoryIdentity(row({ url: GITHUB_REPOSITORY_URL })),
    );
  });

  it("keeps distinct fallback repositories separate", () => {
    expect(selectedRemoteRepositoryIdentity(row({ url: GITHUB_REPOSITORY_URL }))).not.toBe(
      selectedRemoteRepositoryIdentity(row({ url: "https://github.com/acme/docs" })),
    );
  });

  it("does not match an identical raw provider ID from a different provider", () => {
    expect(
      remoteRepositoryMatchesSelection(
        {
          provider: "gitlab",
          id: "acme/site",
          owner: "acme",
          name: "site",
          fullName: "acme/site",
          url: "https://gitlab.com/acme/site",
          defaultBranch: "main",
          private: false,
        },
        "github:id:acme/site",
      ),
    ).toBe(false);
  });
});
