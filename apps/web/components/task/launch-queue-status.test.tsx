/* eslint-disable sonarjs/no-duplicate-string -- Repeated wire fixtures keep queue ownership assertions readable. */
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LaunchQueueStatus } from "./launch-queue-status";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, providedValues?: Record<string, unknown>) => {
      const values = providedValues ?? {};
      const labels: Record<string, string> = {
        "task:launchQueueTitle": "Automatic launch",
        "task:launchQueueLabel": "Queued",
        "task:launchQueueIndicator": "Automatic launch queued",
        "task:launchQueueDestination": `Destination: ${values.destination ?? ""}`,
        "task:launchQueueWaitingCapacity": "Waiting for session capacity.",
        "task:launchQueueOwnershipUnavailable": "Launch ownership is unavailable.",
        "task:launchQueueReplayError": "The queued launch needs attention.",
        "task:launchQueueCapacity": `${values.inUse ?? ""} of ${values.limit ?? ""} sessions in use. Checked ${values.checkedAt ?? ""}.`,
        "task:launchQueueCapacityStale": `${values.inUse ?? ""} of ${values.limit ?? ""} sessions in use. Capacity data is stale. Last checked ${values.checkedAt ?? ""}.`,
        "task:launchQueueCapacityDisconnected": `${values.inUse ?? ""} of ${values.limit ?? ""} sessions in use. Capacity connection is unavailable. Last checked ${values.checkedAt ?? ""}.`,
        "task:launchQueueCapacityUnavailable": "Capacity is unavailable.",
        "task:launchQueueSince": `Queued since ${values.time ?? ""}.`,
        "task:launchQueueAutomaticRetry": "Kandev will retry automatically.",
        "task:launchQueueRetryPending": "Retry pending.",
        "task:launchQueueRetryStopped": "Automatic retry stopped.",
        "task:launchQueueUnknownDestination": "the queued session",
      };
      return labels[key] ?? key;
    },
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: (
    selector: (state: {
      agentProfiles: { items: Array<{ id: string; label: string }> };
      connection: { status: string };
    }) => unknown,
    fallback: unknown,
  ) =>
    selector({
      agentProfiles: { items: [{ id: "luna-profile", label: "Luna" }] },
      connection: { status: "connected" },
    }) || fallback,
}));

vi.mock("@/lib/utils", () => ({
  formatRelativeTime: (value: string) => (value ? "just now" : ""),
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("LaunchQueueStatus", () => {
  it("renders the destination and bounded capacity details outside the transcript", () => {
    render(
      <LaunchQueueStatus
        queue={{
          session_id: "luna-session",
          agent_profile_id: "luna-profile",
          queued_at: "2026-09-16T20:15:44Z",
          reason: "session_capacity",
          retrying: true,
          capacity: {
            in_use: 5,
            limit: 5,
            observed_at: "2026-09-16T20:15:44Z",
          },
        }}
      />,
    );

    expect(screen.getByTestId("task-launch-queue-status").getAttribute("role")).toBe("status");
    expect(screen.getByText("Destination: Luna")).toBeTruthy();
    expect(screen.getByText(/5 of 5 sessions in use/)).toBeTruthy();
    expect(screen.getByText(/retry automatically/)).toBeTruthy();
    expect(screen.queryByText(/position|ETA/i)).toBeNull();
  });

  it("does not render when the queue projection is cleared", () => {
    render(<LaunchQueueStatus queue={null} />);
    expect(screen.queryByTestId("task-launch-queue-status")).toBeNull();
  });

  it("marks an old capacity sample stale after the freshness window", () => {
    vi.useFakeTimers();
    const now = new Date("2026-09-17T10:00:00Z");
    vi.setSystemTime(now);
    render(
      <LaunchQueueStatus
        queue={{
          session_id: "luna-session",
          agent_profile_id: "luna-profile",
          queued_at: "2026-09-17T09:59:00Z",
          reason: "session_capacity",
          retrying: true,
          capacity: {
            in_use: 5,
            limit: 5,
            observed_at: "2026-09-16T20:15:44Z",
          },
        }}
      />,
    );

    expect(screen.getByText(/Capacity data is stale/)).toBeTruthy();
  });

  it("marks known capacity unavailable while disconnected and keeps ownership visible", () => {
    render(
      <LaunchQueueStatus
        isConnected={false}
        queue={{
          session_id: "luna-session",
          agent_profile_id: "luna-profile",
          queued_at: "2026-09-16T20:15:44Z",
          reason: "session_capacity",
          retrying: true,
          capacity: {
            in_use: 5,
            limit: 5,
            observed_at: "2026-09-16T20:15:44Z",
          },
        }}
      />,
    );

    expect(screen.getByText("Destination: Luna")).toBeTruthy();
    expect(screen.getByText(/Capacity connection is unavailable/)).toBeTruthy();
  });

  it("rerenders the same queue as its capacity sample ages", () => {
    vi.useFakeTimers();
    const now = new Date("2026-09-17T10:00:00Z");
    vi.setSystemTime(now);
    render(
      <LaunchQueueStatus
        queue={{
          session_id: "luna-session",
          agent_profile_id: "luna-profile",
          queued_at: "2026-09-17T09:59:00Z",
          reason: "session_capacity",
          retrying: true,
          capacity: {
            in_use: 5,
            limit: 5,
            observed_at: "2026-09-17T09:59:59Z",
          },
        }}
      />,
    );

    expect(screen.getByText(/Checked just now/)).toBeTruthy();
    act(() => vi.advanceTimersByTime(41_000));
    expect(screen.getByText(/Capacity data is stale/)).toBeTruthy();
  });
});
