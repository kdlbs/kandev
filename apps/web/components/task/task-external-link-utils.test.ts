import { describe, expect, it } from "vitest";
import { buildLinkedIssueTitle } from "./task-external-link-utils";

describe("buildLinkedIssueTitle", () => {
  it("prepends the external issue key to a normal title", () => {
    expect(buildLinkedIssueTitle("Fix login", "PROJ-12")).toBe("PROJ-12: Fix login");
  });

  it("uses the key alone when the task title is empty", () => {
    expect(buildLinkedIssueTitle("   ", "ENG-20")).toBe("ENG-20");
  });

  it("replaces an existing leading Jira/Linear/Sentry-style prefix", () => {
    expect(buildLinkedIssueTitle("PROJ-12: Fix login", "ENG-20")).toBe("ENG-20: Fix login");
    expect(buildLinkedIssueTitle("API-99: Debug production crash", "SVC-7")).toBe(
      "SVC-7: Debug production crash",
    );
  });
});
