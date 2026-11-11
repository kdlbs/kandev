import { describe, expect, it } from "vitest";
import { parseGitHubRepositoryInput } from "./repository-input";

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
