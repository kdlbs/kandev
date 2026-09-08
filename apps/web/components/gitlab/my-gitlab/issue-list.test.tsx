import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { Issue } from "@/lib/types/gitlab";
import { IssueList } from "./issue-list";

afterEach(() => cleanup());

function fakeIssue(overrides: Partial<Issue> = {}): Issue {
  return {
    id: 1,
    iid: 1,
    project_id: 1,
    title: "Fix the thing",
    body: "",
    url: "",
    web_url: "",
    state: "opened",
    author_username: "alice",
    project_namespace: "acme",
    project_path: "acme/api",
    labels: [],
    assignees: [],
    milestone: "",
    created_at: new Date().toISOString(),
    updated_at: "",
    ...overrides,
  };
}

describe("IssueList milestone chip", () => {
  it("renders no chip when the issue has no milestone, and leaves the label chips unmoved (Scenario 14)", () => {
    render(
      <IssueList
        items={[fakeIssue({ labels: ["bug", "frontend"] })]}
        loading={false}
        error={null}
      />,
    );
    expect(screen.queryByTestId("gitlab-issue-milestone")).toBeNull();
    expect(screen.getByText("bug")).not.toBeNull();
    expect(screen.getByText("frontend")).not.toBeNull();
  });

  it("positions the milestone chip after the project#iid/author/time metadata and before the first label chip (Scenario 14)", () => {
    render(
      <IssueList
        items={[fakeIssue({ milestone: "Next", labels: ["bug", "frontend"] })]}
        loading={false}
        error={null}
      />,
    );
    const row = screen.getByTestId("issue-row");
    const metadata = within(row).getByText(/acme\/api#1/);
    const chip = within(row).getByTestId("gitlab-issue-milestone");
    const firstLabel = within(row).getByText("bug");

    expect(chip.textContent).toBe("Next");
    // DOM order: metadata, then the milestone chip, then the first label chip.
    expect(
      // eslint-disable-next-line no-bitwise -- Node.compareDocumentPosition bitmask, not a numeric flag
      metadata.compareDocumentPosition(chip) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
    expect(
      // eslint-disable-next-line no-bitwise -- Node.compareDocumentPosition bitmask, not a numeric flag
      chip.compareDocumentPosition(firstLabel) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).not.toBe(0);
  });

  it("does not consume a label slot: a milestone plus six labels renders the chip, four labels, and a +2 overflow indicator (Scenario 26)", () => {
    render(
      <IssueList
        items={[
          fakeIssue({
            milestone: "Next",
            labels: ["l1", "l2", "l3", "l4", "l5", "l6"],
          }),
        ]}
        loading={false}
        error={null}
      />,
    );
    const row = screen.getByTestId("issue-row");
    expect(within(row).getByTestId("gitlab-issue-milestone").textContent).toBe("Next");
    for (const label of ["l1", "l2", "l3", "l4"]) {
      expect(within(row).getByText(label)).not.toBeNull();
    }
    expect(within(row).queryByText("l5")).toBeNull();
    expect(within(row).queryByText("l6")).toBeNull();
    expect(within(row).getByText("+2")).not.toBeNull();
  });
});
