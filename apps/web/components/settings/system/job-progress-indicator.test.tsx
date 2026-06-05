import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SystemJob } from "@/lib/types/system";

const mocks = vi.hoisted(() => ({
  useSystemJob: vi.fn(),
  useSystemJobs: vi.fn(),
}));

vi.mock("@/hooks/domains/system/use-system-jobs", () => ({
  useSystemJob: mocks.useSystemJob,
  useSystemJobs: mocks.useSystemJobs,
}));

import { JobProgressIndicator } from "./job-progress-indicator";

function job(overrides: Partial<SystemJob> = {}): SystemJob {
  return {
    id: "job-1",
    kind: "self-update",
    state: "succeeded",
    started_at: "2026-05-30T00:00:00.000Z",
    ...overrides,
  };
}

describe("JobProgressIndicator", () => {
  beforeEach(() => {
    mocks.useSystemJob.mockReset();
    mocks.useSystemJobs.mockReset();
  });

  afterEach(() => {
    cleanup();
  });

  it("renders a pinned job fetched by id when the websocket list missed it", () => {
    mocks.useSystemJob.mockReturnValue(job({ id: "job-42" }));
    mocks.useSystemJobs.mockReturnValue([]);

    render(<JobProgressIndicator kind="self-update" jobId="job-42" />);

    const indicator = screen.getByTestId("system-job-self-update");
    expect(indicator.getAttribute("data-state")).toBe("succeeded");
    expect(indicator.textContent).toContain("Done");
  });

  it("falls back to the latest kind-matched job when no id is pinned", () => {
    mocks.useSystemJob.mockReturnValue(undefined);
    mocks.useSystemJobs.mockReturnValue([job({ id: "job-2", state: "failed" })]);

    render(<JobProgressIndicator kind="self-update" />);

    expect(screen.getByTestId("system-job-self-update").getAttribute("data-state")).toBe("failed");
  });

  it("allows a flow-specific success label", () => {
    mocks.useSystemJob.mockReturnValue(job({ id: "job-42", state: "succeeded" }));
    mocks.useSystemJobs.mockReturnValue([]);

    render(
      <JobProgressIndicator kind="self-update" jobId="job-42" successLabel="Restarting service" />,
    );

    expect(screen.getByText("Restarting service")).not.toBeNull();
    expect(screen.queryByText("Done")).toBeNull();
  });
});
