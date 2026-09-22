/* eslint-disable sonarjs/no-duplicate-string -- Repeated wire fixtures keep queue ownership assertions readable. */
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { TaskStatusSummaryLaunchQueue } from "@/lib/types/task-status-summary";
import {
  hasWorkflowParkingMarker,
  LaunchQueueStatus,
  ParkedSessionNote,
} from "./launch-queue-status";

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
        "task:launchQueueWaitingGlobalCapacity": "Waiting for global session capacity.",
        "task:launchQueueGlobalScope": "Global session limit",
        "task:launchQueueGlobalScopeHelp": "All workspaces",
        "task:launchQueueConfigureCapacity": "Configure global session limit",
        "task:launchQueueOwnershipUnavailable": "Launch ownership is unavailable.",
        "task:launchQueueReplayError": "The queued launch needs attention.",
        "task:launchQueueReplayErrorStopped": "The queued launch needs attention.",
        "task:launchQueueCapacity": `${values.inUse ?? ""} of ${values.limit ?? ""} sessions in use. Checked ${values.checkedAt ?? ""}.`,
        "task:launchQueueCapacityStale": `${values.inUse ?? ""} of ${values.limit ?? ""} sessions in use. Capacity data is stale. Last checked ${values.checkedAt ?? ""}.`,
        "task:launchQueueCapacityDisconnected": `${values.inUse ?? ""} of ${values.limit ?? ""} sessions in use. Capacity connection is unavailable. Last checked ${values.checkedAt ?? ""}.`,
        "task:launchQueueCapacityUnavailable": "Capacity is unavailable.",
        "task:launchQueueCapacityUnavailableStopped": "Capacity information is unavailable.",
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
  cn: (...values: unknown[]) => values.filter(Boolean).join(" "),
  formatRelativeTime: (value: string) => (value ? "just now" : ""),
}));

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const QUEUE_WITH_CAPACITY: TaskStatusSummaryLaunchQueue = {
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
};

const QUEUE_STALE_CAPACITY: TaskStatusSummaryLaunchQueue = {
  ...QUEUE_WITH_CAPACITY,
  queued_at: "2026-09-17T09:59:00Z",
  capacity: { ...QUEUE_WITH_CAPACITY.capacity!, observed_at: "2026-09-16T20:15:44Z" },
};

const QUEUE_CURRENT_CAPACITY: TaskStatusSummaryLaunchQueue = {
  ...QUEUE_WITH_CAPACITY,
  queued_at: "2026-09-17T09:59:00Z",
  capacity: { ...QUEUE_WITH_CAPACITY.capacity!, observed_at: "2026-09-17T09:59:59Z" },
};

const QUEUE_STOPPED_REPLAY: TaskStatusSummaryLaunchQueue = {
  session_id: "luna-session",
  agent_profile_id: "luna-profile",
  queued_at: "2026-09-17T09:59:00Z",
  reason: "replay_error",
  retrying: false,
};

describe("LaunchQueueStatus", () => {
  it("renders the destination and bounded capacity details outside the transcript", () => {
    render(<LaunchQueueStatus queue={QUEUE_WITH_CAPACITY} />);

    expect(screen.getByTestId("task-launch-queue-status").getAttribute("role")).toBeNull();
    expect(screen.getByTestId("task-launch-queue-live-status").getAttribute("role")).toBe("status");
    expect(screen.getByText("Destination: Luna")).toBeTruthy();
    expect(screen.getByText(/5 of 5 sessions in use/)).toBeTruthy();
    expect(screen.getByText("Global session limit")).toBeTruthy();
    expect(screen.getByText("All workspaces")).toBeTruthy();
    expect(screen.getByTestId("launch-queue-session-capacity-link").getAttribute("href")).toBe(
      "/settings/preferences/task-behavior#setting-session-capacity",
    );
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
    render(<LaunchQueueStatus queue={QUEUE_STALE_CAPACITY} />);

    expect(screen.getByText(/Capacity data is stale/)).toBeTruthy();
  });

  it("marks known capacity unavailable while disconnected and keeps ownership visible", () => {
    render(<LaunchQueueStatus isConnected={false} queue={QUEUE_WITH_CAPACITY} />);

    expect(screen.getByText("Destination: Luna")).toBeTruthy();
    expect(screen.getByText(/Capacity connection is unavailable/)).toBeTruthy();
  });

  it("uses the generic destination label when the profile is not loaded", () => {
    render(
      <LaunchQueueStatus
        queue={{
          session_id: "opaque-session-id",
          agent_profile_id: "opaque-profile-id",
          queued_at: "2026-09-16T20:15:44Z",
          reason: "session_capacity",
          retrying: true,
        }}
      />,
    );

    expect(screen.getByText("Destination: the queued session")).toBeTruthy();
    expect(screen.queryByText(/opaque-profile-id|opaque-session-id/)).toBeNull();
  });

  it("rerenders the same queue as its capacity sample ages", () => {
    vi.useFakeTimers();
    const now = new Date("2026-09-17T10:00:00Z");
    vi.setSystemTime(now);
    render(<LaunchQueueStatus queue={QUEUE_CURRENT_CAPACITY} />);

    expect(screen.getByText(/Checked just now/)).toBeTruthy();
    act(() => vi.advanceTimersByTime(41_000));
    expect(screen.getByText(/Capacity data is stale/)).toBeTruthy();
  });
});

describe("LaunchQueueStatus retry copy", () => {
  it("does not promise a retry when the queue has stopped retrying", () => {
    render(<LaunchQueueStatus queue={QUEUE_STOPPED_REPLAY} />);

    expect(screen.getByText("The queued launch needs attention.")).toBeTruthy();
    expect(screen.getByText("Automatic retry stopped.")).toBeTruthy();
    expect(screen.queryByText(/will retry/)).toBeNull();
    expect(screen.queryByTestId("launch-queue-session-capacity-link")).toBeNull();
  });
});

describe("workflow parking presentation", () => {
  it.each([
    [undefined, false],
    [{}, false],
    [
      {
        workflow_parking: {
          stamp: "",
          parked_at: "2026-09-16T20:00:00Z",
          source_session_id: "source",
        },
      },
      false,
    ],
    [{ workflow_parking: { stamp: "stamp", parked_at: "", source_session_id: "source" } }, false],
    [
      {
        workflow_parking: {
          stamp: "stamp",
          parked_at: "2026-09-16T20:00:00Z",
          source_session_id: "",
        },
      },
      false,
    ],
    [
      {
        workflow_parking: {
          stamp: "stamp",
          parked_at: "2026-09-16T20:00:00Z",
          source_session_id: "source",
        },
      },
      true,
    ],
  ])("validates %j as %s", (metadata, expected) => {
    expect(hasWorkflowParkingMarker(metadata as Record<string, unknown> | undefined)).toBe(
      expected,
    );
  });

  it("renders the note only when the selected session is parked", () => {
    const { rerender } = render(<ParkedSessionNote visible={false} />);
    expect(screen.queryByTestId("task-parked-session-note")).toBeNull();
    rerender(<ParkedSessionNote visible />);
    expect(screen.getByTestId("task-parked-session-note")).toBeTruthy();
  });
});
