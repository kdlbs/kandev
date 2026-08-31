import { describe, expect, it } from "vitest";
import { getChangeRequestTerminology } from "@/hooks/use-git-operations";
import {
  getChangeRequestFailureFeedback,
  getChangeRequestSuccessFeedback,
} from "./change-request-feedback";

describe("getChangeRequestSuccessFeedback", () => {
  it("warns without inviting duplicate creation when association failed", () => {
    expect(
      getChangeRequestSuccessFeedback(
        {
          success: true,
          branch_pushed: true,
          pr_url: "https://bitbucket.test/pr/42",
          provider: "bitbucket",
          linked: false,
          association_error: "Task association could not be saved",
        },
        false,
        getChangeRequestTerminology("github"),
      ),
    ).toEqual({
      title: "PR created; task link needs attention",
      description: "Task association could not be saved. Use Link in the task menu to retry.",
      variant: "default",
    });
  });
});

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

  it("maps empty-remote publication failures to recovery copy", () => {
    expect(
      getChangeRequestFailureFeedback(
        {
          success: false,
          provider: "github",
          error: "raw push output",
          error_code: "empty_remote_branch_publish_failed",
        },
        getChangeRequestTerminology("github"),
      ),
    ).toEqual({
      title: "Create PR failed",
      description: "The base branch was published, but the task branch was not. Try Push again.",
      variant: "error",
    });
  });
});
