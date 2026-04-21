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
