import { describe, expect, it } from "vitest";
import { remoteRepositoryBrowserUrl } from "./remote-repository-browser-url";

describe("remoteRepositoryBrowserUrl", () => {
  it("removes one terminal .git suffix and trailing slash", () => {
    expect(remoteRepositoryBrowserUrl("https://gitlab.example.com/team/agent.git/", "gitlab")).toBe(
      "https://gitlab.example.com/team/agent",
    );
  });

  it("does not advertise a Bitbucket Server clone URL as its browser page", () => {
    expect(
      remoteRepositoryBrowserUrl(
        "https://bitbucket.example.test/scm/TEAM/fixture.git",
        "bitbucket",
      ),
    ).toBeNull();
  });

  it.each([
    undefined,
    "",
    "git@github.com:owner/repo.git",
    "http://github.com/owner/repo",
    "https://user:secret@github.com/owner/repo",
    "https://github.com/owner/repo?tab=code",
    "https://github.com/owner/repo#readme",
    "not a URL",
  ])("returns no browser destination for an unsafe or missing URL: %s", (remoteUrl) => {
    expect(remoteRepositoryBrowserUrl(remoteUrl, "github")).toBeNull();
  });

  it("fails closed for plugin providers without a browser URL contract", () => {
    expect(
      remoteRepositoryBrowserUrl("https://code.example.test/owner/repo.git", "source-control"),
    ).toBeNull();
  });
});
