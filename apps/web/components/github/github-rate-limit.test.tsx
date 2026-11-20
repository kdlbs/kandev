import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GitHubPRDiscoveryHealth } from "@/lib/types/github";
import { GitHubPRDiscoveryHealthWarning } from "./github-pr-discovery-health";

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date("2026-09-11T12:00:00Z"));
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

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

  // @covers AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.3 .8
  it("hides only retry text when the deadline is not in the future", () => {
    for (const retry_at of [
      "2026-09-11T12:00:00Z",
      "2026-09-11T11:59:00Z",
      "not-a-date",
      "2099-02-30T12:00:00Z",
      "2099-02-02",
      "2099-02-02T12:00:00",
      undefined,
    ]) {
      cleanup();
      render(<GitHubPRDiscoveryHealthWarning health={{ ...degradedHealth, retry_at }} />);

      expect(screen.getByText("PR discovery failed")).toBeTruthy();
      expect(screen.getByText(/Reason: Invalid query/)).toBeTruthy();
      expect(screen.getByText(/Affected targets: 2/)).toBeTruthy();
      expect(screen.queryByText(/Retry after/)).toBeNull();
    }
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
