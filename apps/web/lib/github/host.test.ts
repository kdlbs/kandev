// @covers AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.2, AC-INTEGRATIONS-GITHUB-ENTERPRISE-001.3
import { describe, expect, it } from "vitest";
import { normalizeGitHubHost, githubOrigin } from "./host";

describe("GitHub web host identity", () => {
  it.each([
    ["github.com", "github.com"],
    ["git", "git"],
    ["https://GIT.Company.Example:443/", "git.company.example"],
    ["git.company.example:8443", "git.company.example:8443"],
    ["tenant.ghe.com", "tenant.ghe.com"],
  ])("normalizes %s", (input, expected) => expect(normalizeGitHubHost(input)).toBe(expected));
  it.each([
    "",
    "git.company.example:",
    "http://git.example.com",
    "https://git.example.com/path",
    "https://user@git.example.com",
    "127.0.0.1",
    "api.github.com",
    "ghe.com",
    "api.tenant.ghe.com",
    "github.com:8443",
    "tenant.ghe.com:8443",
    "git.example.com?token=x",
    "git.example.com#fragment",
    "git. example.com",
  ])("rejects %s", (input) => expect(normalizeGitHubHost(input)).toBeNull());
  it("retains a GHES port in links", () =>
    expect(githubOrigin("git.example.com:8443")).toBe("https://git.example.com:8443"));
});
