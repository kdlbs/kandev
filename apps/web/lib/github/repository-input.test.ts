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
