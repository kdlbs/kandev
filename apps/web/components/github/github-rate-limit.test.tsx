import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { GitHubPRDiscoveryHealth } from "@/lib/types/github";
import { GitHubPRDiscoveryHealthWarning } from "./github-pr-discovery-health";

afterEach(cleanup);

const degradedHealth: GitHubPRDiscoveryHealth = {
  state: "degraded",
  failed_target_count: 2,
  category: "invalid_query",
  last_failure_at: "2026-09-11T12:00:00Z",
  retry_at: "2026-09-11T12:01:00Z",
  revision: 3,
  credential_generation: 4,
};

describe("GitHub PR discovery health disclosure", () => {
  it("shows operation, category, failure time, affected targets, and retry state", () => {
    render(<GitHubPRDiscoveryHealthWarning health={degradedHealth} />);

    expect(screen.getByTestId("github-pr-discovery-health")).toBeTruthy();
    expect(screen.getByText("PR discovery failed")).toBeTruthy();
    expect(screen.getByText(/Reason: Invalid query/)).toBeTruthy();
    expect(screen.getByText(/Affected targets: 2/)).toBeTruthy();
    expect(screen.getByText(/Retry after/)).toBeTruthy();
  });

  it("does not show a warning for healthy discovery", () => {
    render(
      <GitHubPRDiscoveryHealthWarning
        health={{ ...degradedHealth, state: "healthy", failed_target_count: 0 }}
      />,
    );

    expect(screen.queryByTestId("github-pr-discovery-health")).toBeNull();
  });
});
