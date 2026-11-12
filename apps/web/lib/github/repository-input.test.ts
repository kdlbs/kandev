import { describe, expect, it } from "vitest";
import { parseGitHubLogin, parseGitHubRepositoryInput } from "./repository-input";

describe("parseGitHubRepositoryInput", () => {
  it("parses an owner/name slug", () => {
    expect(parseGitHubRepositoryInput(" kdlbs/kandev ")).toEqual({
      owner: "kdlbs",
      name: "kandev",
    });
  });

  it("parses a GitHub repository URL", () => {
    expect(parseGitHubRepositoryInput("https://github.com/kdlbs/kandev.git")).toEqual({
      owner: "kdlbs",
      name: "kandev",
    });
  });

  it.each(["", "kandev", "kdlbs/", "/kandev", "a/b/c", "kd lbs/kandev", "https://example.com/a/b"])(
    "rejects %j",
    (value) => {
      expect(parseGitHubRepositoryInput(value)).toBeNull();
    },
  );
});

describe("parseGitHubLogin", () => {
  it.each([
    ["kdlbs", "kdlbs"],
    [" Octo-Cat ", "Octo-Cat"],
    ["a1", "a1"],
  ])("accepts %j", (value, expected) => {
    expect(parseGitHubLogin(value)).toBe(expected);
  });

  it.each(["", "-kdlbs", "kd lbs", "kdlbs/kandev", "kd_lbs", "a".repeat(40)])(
    "rejects %j",
    (value) => {
      expect(parseGitHubLogin(value)).toBeNull();
    },
  );
});

describe("parseGitHubRepositoryInput host matching", () => {
  it("rejects a repository URL from a host other than the workspace host", () => {
    expect(parseGitHubRepositoryInput("https://acme.ghe.com/kdlbs/kandev")).toBeNull();
    expect(
      parseGitHubRepositoryInput("https://github.com/kdlbs/kandev", "git.company.example"),
    ).toBeNull();
  });

  it("accepts a repository URL on the workspace host", () => {
    expect(
      parseGitHubRepositoryInput("https://git.company.example/acme/widget", "git.company.example"),
    ).toEqual({ owner: "acme", name: "widget" });
    expect(parseGitHubRepositoryInput("https://acme.ghe.com/a/b", "acme.ghe.com")).toEqual({
      owner: "a",
      name: "b",
    });
  });
});
