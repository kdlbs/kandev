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

  it("tolerates a query string suffix (canonical share URL)", () => {
    // The GitHub UI pins active tabs via ?tab=… and similar; the picker
    // should accept these without stripping the tail by hand.
    expect(parseGitHubRepoUrl("https://github.com/acme/site?tab=readme")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("tolerates a fragment suffix", () => {
    expect(parseGitHubRepoUrl("https://github.com/acme/site#readme")).toEqual({
      owner: "acme",
      repo: "site",
    });
  });

  it("tolerates a trailing slash followed by a fragment", () => {
    expect(parseGitHubRepoUrl("https://github.com/acme/site/#section")).toEqual({
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

  it("returns null when the URL is embedded in surrounding text", () => {
    // Regression: the regex was unanchored, so anything containing a valid
    // github.com/<owner>/<repo> substring (chat messages, log lines, etc.)
    // matched and got treated as a clean repo URL. Anchoring at ^ rules out
    // these false positives — input must START with the URL pattern.
    expect(parseGitHubRepoUrl("some prefix text github.com/owner/repo")).toBeNull();
    expect(parseGitHubRepoUrl("see https://github.com/owner/repo for details")).toBeNull();
    expect(parseGitHubRepoUrl("malicious.com/github.com/owner/repo")).toBeNull();
  });
});
