import { describe, expect, it } from "vitest";
import { parseGitHubRepoUrl } from "./parse-url";

describe("parseGitHubRepoUrl", () => {
  it("parses a plain https URL", () => {
    expect(parseGitHubRepoUrl("https://github.com/acme/site")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("parses an http URL", () => {
    expect(parseGitHubRepoUrl("http://github.com/acme/site")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("parses a URL without a scheme", () => {
    expect(parseGitHubRepoUrl("github.com/acme/site")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("parses a URL with www.", () => {
    expect(parseGitHubRepoUrl("https://www.github.com/acme/site")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("strips a .git suffix", () => {
    expect(parseGitHubRepoUrl("https://github.com/acme/site.git")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("tolerates a trailing slash", () => {
    expect(parseGitHubRepoUrl("https://github.com/acme/site/")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("tolerates a .git suffix and trailing slash together", () => {
    expect(parseGitHubRepoUrl("https://github.com/acme/site.git/")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("accepts hyphens, dots, and underscores in owner and repo", () => {
    expect(parseGitHubRepoUrl("https://github.com/my-org_1.x/repo-name_2.x")).toEqual({
      owner: "my-org_1.x",
      repo: "repo-name_2.x",
    });
  });

  it("trims surrounding whitespace before matching", () => {
    expect(parseGitHubRepoUrl("   https://github.com/acme/site   ")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("returns null for empty input", () => {
    expect(parseGitHubRepoUrl("")).toBeNull();
    expect(parseGitHubRepoUrl("   ")).toBeNull();
  });

  it("returns null for a non-GitHub host", () => {
    expect(parseGitHubRepoUrl("https://gitlab.com/acme/site")).toBeNull();
  });

  it("returns null for a malformed URL", () => {
    expect(parseGitHubRepoUrl("https://github.com/acme")).toBeNull();
    expect(parseGitHubRepoUrl("not a url at all")).toBeNull();
  });
});
