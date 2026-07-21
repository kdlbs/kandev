import { describe, expect, it } from "vitest";
import { getChangeRequestTerminology } from "./use-git-operations";

describe("getChangeRequestTerminology", () => {
  it("uses merge request terminology for GitLab", () => {
    expect(getChangeRequestTerminology("gitlab")).toEqual({
      longName: "Merge Request",
      shortName: "MR",
    });
  });

  it("keeps pull request terminology for other providers", () => {
    expect(getChangeRequestTerminology("github")).toEqual({
      longName: "Pull Request",
      shortName: "PR",
    });
  });
});
