import { describe, expect, it } from "vitest";
import { getChangeRequestTerminology } from "@/hooks/use-git-operations";
import { getChangeRequestFailureFeedback } from "./change-request-feedback";

describe("getChangeRequestFailureFeedback", () => {
  it.each([
    ["gitlab", "MR", "merge request"],
    ["github", "PR", "pull request"],
    ["azure_repos", "PR", "pull request"],
  ])("returns provider-aware partial feedback for %s", (provider, shortName, longName) => {
    const feedback = getChangeRequestFailureFeedback(
      {
        success: false,
        branch_pushed: true,
        provider,
        error: "sensitive provider failure",
        output: "sensitive provider output",
      },
      getChangeRequestTerminology("github"),
    );

    expect(feedback).toEqual({
      title: `Branch pushed; ${shortName} not created`,
      description: `Branch was pushed; retry ${longName} creation.`,
      variant: "default",
    });
    expect(JSON.stringify(feedback)).not.toContain("sensitive provider");
  });

  it("keeps ordinary failures distinct", () => {
    expect(
      getChangeRequestFailureFeedback(
        { success: false, provider: "gitlab", error: "Authentication failed" },
        getChangeRequestTerminology("github"),
      ),
    ).toEqual({
      title: "Create MR failed",
      description: "Authentication failed",
      variant: "error",
    });
  });
});
