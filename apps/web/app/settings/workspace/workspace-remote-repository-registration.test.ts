import { describe, expect, it } from "vitest";
import { remoteRepositoryRegistrationPayload } from "./workspace-remote-repository-registration";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";

function row(overrides: Partial<TaskRemoteRepoRow> = {}): TaskRemoteRepoRow {
  return { key: "remote-0", url: "", branch: "", source: "paste", ...overrides };
}

describe("remoteRepositoryRegistrationPayload", () => {
  it("sends only the locator and branch for a GitHub picker selection", () => {
    expect(
      remoteRepositoryRegistrationPayload(
        row({
          url: "https://github.com/acme/site",
          branch: "main",
          source: "picker",
          provider: "github",
          providerRepoId: "acme/site",
          providerOwner: "acme",
          providerName: "site",
          fullName: "acme/site",
        }),
      ),
    ).toEqual({ remote_url: "https://github.com/acme/site", default_branch: "main" });
  });

  it("forwards the provider descriptor hints for a plugin provider selection", () => {
    expect(
      remoteRepositoryRegistrationPayload(
        row({
          url: "https://git.example.test/curtis/tooling",
          remoteUrl: "https://git.example.test/curtis/tooling.git",
          branch: "dev",
          source: "picker",
          provider: "forgejo",
          providerHost: "https://git.example.test",
          providerScope: "scope-1",
          providerRepoId: "42",
          providerOwner: "curtis",
          providerName: "tooling",
        }),
      ),
    ).toEqual({
      remote_url: "https://git.example.test/curtis/tooling.git",
      default_branch: "dev",
      provider: "forgejo",
      provider_host: "https://git.example.test",
      provider_scope: "scope-1",
      provider_repo_id: "42",
      provider_owner: "curtis",
      provider_name: "tooling",
    });
  });

  it("omits the branch when the row has none and trims a pasted URL", () => {
    expect(
      remoteRepositoryRegistrationPayload(row({ url: "  https://gitlab.com/acme/api  " })),
    ).toEqual({ remote_url: "https://gitlab.com/acme/api" });
  });
});
