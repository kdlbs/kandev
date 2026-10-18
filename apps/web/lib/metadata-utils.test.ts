import { describe, it, expect } from "vitest";
import {
  isPRReviewFromMetadata,
  isIssueWatchFromMetadata,
  issueFieldsFromMetadata,
} from "./metadata-utils";

describe("isPRReviewFromMetadata", () => {
  it("returns true when review_watch_id is a non-empty string", () => {
    expect(isPRReviewFromMetadata({ review_watch_id: "rw-abc" })).toBe(true);
  });

  it("returns false when review_watch_id is absent", () => {
    expect(isPRReviewFromMetadata({ other_key: "val" })).toBe(false);
  });

  it("returns false when review_watch_id is an empty string", () => {
    expect(isPRReviewFromMetadata({ review_watch_id: "" })).toBe(false);
  });

  it("returns false for null metadata", () => {
    expect(isPRReviewFromMetadata(null)).toBe(false);
  });

  it("returns false for non-object metadata", () => {
    expect(isPRReviewFromMetadata("string" as unknown as Record<string, unknown>)).toBe(false);
  });
});

describe("isIssueWatchFromMetadata", () => {
  it("returns true when issue_watch_id is a non-empty string", () => {
    expect(isIssueWatchFromMetadata({ issue_watch_id: "iw-xyz" })).toBe(true);
  });

  it("returns false when issue_watch_id is absent", () => {
    expect(isIssueWatchFromMetadata({ review_watch_id: "rw-abc" })).toBe(false);
  });

  it("returns false when issue_watch_id is an empty string", () => {
    expect(isIssueWatchFromMetadata({ issue_watch_id: "" })).toBe(false);
  });

  it("returns false for null metadata", () => {
    expect(isIssueWatchFromMetadata(null)).toBe(false);
  });
});

describe("issueFieldsFromMetadata", () => {
  const ISSUE_URL = "https://github.com/a/b/issues/7";

  it("extracts issueUrl and issueNumber when both are present", () => {
    expect(issueFieldsFromMetadata({ issue_url: ISSUE_URL, issue_number: 7 })).toEqual({
      issueUrl: ISSUE_URL,
      issueNumber: 7,
    });
  });

  it("returns only issueUrl when issue_number is absent", () => {
    expect(issueFieldsFromMetadata({ issue_url: ISSUE_URL })).toEqual({
      issueUrl: ISSUE_URL,
    });
  });

  it("returns only issueNumber when issue_url is absent", () => {
    expect(issueFieldsFromMetadata({ issue_number: 42 })).toEqual({ issueNumber: 42 });
  });

  it("returns empty object when neither field is present", () => {
    expect(issueFieldsFromMetadata({ other: "val" })).toEqual({});
  });

  it("ignores issue_number if it is a string instead of a number", () => {
    expect(issueFieldsFromMetadata({ issue_url: "https://x", issue_number: "7" })).toEqual({
      issueUrl: "https://x",
    });
  });

  it("returns empty object for null metadata", () => {
    expect(issueFieldsFromMetadata(null)).toEqual({});
  });
});

// @covers AC-UI-PROVIDER-WATCHER-TYPES-001.1 through .4
const watcherKeys: [string, boolean, boolean][] = [
  ["issue_watch_id", true, false],
  ["review_watch_id", false, true],
  ["gitlab_issue_watch_id", true, false],
  ["gitlab_review_watch_id", false, true],
  ["jira_issue_watch_id", true, false],
  ["linear_issue_watch_id", true, false],
  ["sentry_issue_watch_id", true, false],
  ["azure_devops_work_item_watch_id", true, false],
  ["azure_devops_pr_watch_id", false, true],
];
it.each(watcherKeys)("recognizes only usable %s values", (key, issue, review) => {
  expect(isIssueWatchFromMetadata({ [key]: "watch" })).toBe(issue);
  expect(isPRReviewFromMetadata({ [key]: "watch" })).toBe(review);
  for (const value of [null, "", "undefined", false, true, 0, 42, ["id"], { id: "watch" }]) {
    expect(isIssueWatchFromMetadata({ [key]: value })).toBe(false);
    expect(isPRReviewFromMetadata({ [key]: value })).toBe(false);
  }
});
it("does not infer watcher keys or mask a usable ID with an empty one", () => {
  expect(
    isIssueWatchFromMetadata({ custom_issue_watch_id: "watch", issue_url: "https://example.test" }),
  ).toBe(false);
  expect(
    isPRReviewFromMetadata({ custom_review_watch_id: "watch", pr_url: "https://example.test" }),
  ).toBe(false);
  expect(isIssueWatchFromMetadata({ issue_watch_id: "", gitlab_issue_watch_id: "watch" })).toBe(
    true,
  );
  expect(isPRReviewFromMetadata({ review_watch_id: null, azure_devops_pr_watch_id: "watch" })).toBe(
    true,
  );
  expect(isIssueWatchFromMetadata({ issue_watch_id: " " })).toBe(true);
  expect(isPRReviewFromMetadata({ review_watch_id: "null" })).toBe(true);
});
