import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useEffect } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { MonitorMessage } from "./monitor-message";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { StateProvider, useAppStore } from "@/components/state-provider";

// monitorMessage is a tiny test-helper that constructs the kandev Message
// shape produced by the orchestrator for a Monitor tool_call. The fields
// match what `service_messages.applyToolCallMessageUpdate` writes after
// receiving an in_progress tool_call_update from the ACP adapter.
function monitorMessage(opts: {
  status?: "pending" | "running" | "complete" | "error";
  command?: string;
  taskId?: string;
  eventCount?: number;
  recentEvents?: string[];
  ended?: boolean;
  endReason?: string;
  callId?: string;
}): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "Monitor",
    type: "tool_call",
    created_at: "2026-04-25T10:00:00Z",
    metadata: {
      tool_call_id: opts.callId ?? "tc-monitor",
      title: "Monitor",
      status: opts.status ?? "running",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: { command: opts.command ?? "" },
          output: {
            monitor: {
              kind: "Monitor",
              task_id: opts.taskId ?? "task-1",
              command: opts.command ?? "",
              event_count: opts.eventCount ?? 0,
              recent_events: opts.recentEvents ?? [],
              ended: opts.ended ?? false,
              end_reason: opts.endReason ?? "",
            },
          },
        },
      },
    },
  };
}

function monitorWorkload(
  workId: string,
  sourceCallId: string,
  state: WorkloadRunObservation["state"],
): WorkloadRunObservation {
  return {
    session_id: "s1",
    work_id: workId,
    kind: "monitor",
    title: "Monitor",
    state,
    source_call_id: sourceCallId,
    capabilities: {
      discovery: "snapshot",
      output: "none",
      transcript: false,
      parentage: false,
      reasoning_summary: false,
      attributable_usage: false,
    },
    revision: 1,
  };
}

function BackgroundWorkSetter({
  onSet,
}: {
  onSet: (setWorkloads: (workloads: WorkloadRunObservation[]) => void) => void;
}) {
  const setBackgroundWorkloads = useAppStore((state) => state.setBackgroundWorkloads);
  useEffect(() => {
    onSet((workloads) => setBackgroundWorkloads("s1", workloads));
  }, [onSet, setBackgroundWorkloads]);
  return null;
}

afterEach(() => cleanup());

describe("MonitorMessage", () => {
  it("renders a 'watching' pill while the monitor is in progress", () => {
    const html = renderToStaticMarkup(
      <MonitorMessage comment={monitorMessage({ command: "gh pr checks", eventCount: 0 })} />,
    );
    expect(html).toContain("Monitor");
    expect(html).toContain("gh pr checks");
    expect(html).toContain("watching");
    expect(html).not.toContain("ended");
  });

  it("shows the event count when events have arrived", () => {
    const html = renderToStaticMarkup(
      <MonitorMessage comment={monitorMessage({ eventCount: 3, recentEvents: ["a", "b", "c"] })} />,
    );
    expect(html).toContain("3 events");
    // recent events render under the card body when expanded; the auto-expand
    // logic kicks in while watching, so all three should be in the markup.
    expect(html).toContain(">a<");
    expect(html).toContain(">b<");
    expect(html).toContain(">c<");
  });

  it("uses singular 'event' for a count of 1", () => {
    const html = renderToStaticMarkup(
      <MonitorMessage comment={monitorMessage({ eventCount: 1, recentEvents: ["x"] })} />,
    );
    expect(html).toContain("1 event");
    expect(html).not.toContain("1 events");
  });

  it("flips to 'ended' when the monitor exits cleanly", () => {
    const html = renderToStaticMarkup(
      <MonitorMessage
        comment={monitorMessage({ ended: true, endReason: "exited", status: "complete" })}
      />,
    );
    expect(html).toContain("ended");
    expect(html).not.toContain("watching");
  });

  it("keeps the events tail expanded after the monitor ends", () => {
    // After the run completes the user still wants to see what fired.
    // Auto-expand keys off `recentEvents.length > 0`, not `!ended`.
    const html = renderToStaticMarkup(
      <MonitorMessage
        comment={monitorMessage({
          ended: true,
          endReason: "exited",
          status: "complete",
          eventCount: 3,
          recentEvents: ["queued", "running", "passed"],
        })}
      />,
    );
    expect(html).toContain(">queued<");
    expect(html).toContain(">running<");
    expect(html).toContain(">passed<");
  });

  it("flips to 'ended (session restart)' when the agent process restarted", () => {
    const html = renderToStaticMarkup(
      <MonitorMessage comment={monitorMessage({ ended: true, endReason: "session_restart" })} />,
    );
    expect(html).toContain("ended (session restart)");
  });

  it("renders a baseline card even when no monitor view is attached", () => {
    // Defensive guard: if the adapter didn't seed the view (e.g. very old
    // session before this code shipped), the renderer should still produce
    // the "watching" baseline rather than crashing.
    const empty: Message = {
      id: "msg-1",
      session_id: toSessionId("s1"),
      task_id: toTaskId("t1"),
      author_type: "agent",
      content: "Monitor",
      type: "tool_call",
      created_at: "2026-04-25T10:00:00Z",
      metadata: { tool_call_id: "tc-monitor", title: "Monitor", status: "running" },
    };
    const html = renderToStaticMarkup(<MonitorMessage comment={empty} />);
    expect(html).toContain("Monitor");
    expect(html).toContain("watching");
  });

  it("settles only the Monitor card with the matching source call workload", async () => {
    let setWorkloads: ((workloads: WorkloadRunObservation[]) => void) | undefined;
    render(
      <StateProvider>
        <BackgroundWorkSetter onSet={(setter) => (setWorkloads = setter)} />
        <MonitorMessage comment={monitorMessage({ callId: "tc-first" })} />
        <MonitorMessage comment={monitorMessage({ callId: "tc-second", command: "other check" })} />
      </StateProvider>,
    );

    await waitFor(() => expect(setWorkloads).toBeTypeOf("function"));
    await act(async () => {
      setWorkloads?.([
        monitorWorkload("monitor-first", "tc-first", "completed"),
        monitorWorkload("monitor-second", "tc-second", "running"),
      ]);
    });
    await waitFor(() => {
      const statuses = screen.getAllByTestId("monitor-status-pill");
      expect(statuses.map((status) => status.textContent)).toEqual(["ended", "watching"]);
    });
  });
});
