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

  it("replaces the prefix on a previously-truncated title without exceeding the limit", () => {
    // Simulate a title already truncated by a previous link (60 chars, "…" tail).
    // Equal-length keys keep the composed title at exactly the limit, so this
    // locks prefix replacement, not truncation (see the longer-key case below).
    const prev = `DO-916: ${"x".repeat(51)}…`;
    const got = buildLinkedIssueTitle(prev, "ENG-20");
    expect(Array.from(got)).toHaveLength(60);
    expect(got).toBe(`ENG-20: ${"x".repeat(51)}…`);
  });

  it("truncates when re-linking to a longer key", () => {
    // "DO-9: " (6 chars) → "PROJ-12345: " (12 chars) makes the re-composed
    // title exceed the limit, so the truncation path fires after the prefix swap.
    const got = buildLinkedIssueTitle(`DO-9: ${"x".repeat(54)}`, "PROJ-12345");
    expect(Array.from(got)).toHaveLength(60);
    expect(got).toBe(`PROJ-12345: ${"x".repeat(47)}…`);
  });

  it("truncates a composed title that exceeds the 60-character limit", () => {
    const got = buildLinkedIssueTitle("x".repeat(60), "DO-916");
    expect(Array.from(got)).toHaveLength(60);
    expect(got.startsWith("DO-916: ")).toBe(true);
    expect(got.endsWith("…")).toBe(true);
  });

  it("leaves a composed title at the 60-character limit unchanged", () => {
    const title = "y".repeat(52);
    expect(buildLinkedIssueTitle(title, "ENG-20")).toBe(`ENG-20: ${title}`);
  });

  it("counts Unicode code points when truncating", () => {
    const got = buildLinkedIssueTitle("🙂".repeat(60), "DO-916");
    expect(Array.from(got)).toHaveLength(60);
    expect(got.startsWith("DO-916: ")).toBe(true);
    expect(got.endsWith("…")).toBe(true);
  });
});
