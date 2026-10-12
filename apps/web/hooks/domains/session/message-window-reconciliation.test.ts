import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/ids";
import { reconcileLatestMessageWindow } from "./message-window-reconciliation";

function message(id: string, minute: number, fraction = ""): Message {
  return {
    id,
    task_id: taskId("task-1"),
    session_id: sessionId("session-1"),
    author_type: "agent",
    content: id,
    type: "message",
    created_at: `2026-08-30T09:${String(minute).padStart(2, "0")}:00${fraction}Z`,
  } as Message;
}

describe("reconcileLatestMessageWindow", () => {
  // @covers AC-UI-TASK-PROMPT-TRANSCRIPT-VISIBILITY-001.10
  it("drops a disjoint stale prefix and retains a row received during the request", () => {
    const stale = [message("stale-1", 0), message("stale-2", 1)];
    const fetched = [message("fetched-3", 3), message("fetched-4", 4)];
    const live = message("live-5", 5);

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: stale,
      cachedAtResponse: [...stale, live],
      fetched,
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["fetched-3", "fetched-4", "live-5"]);
    expect(result.oldestCursor).toBe("fetched-3");
  });

  it("keeps cached older pages when the fetched suffix overlaps them", () => {
    const cached = [message("cached-1", 1), message("shared-3", 3)];
    const fetched = [message("shared-3", 3), message("fetched-4", 4)];
    const live = message("live-6", 6);

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: cached,
      cachedAtResponse: [...cached, live],
      fetched,
    });

    expect(result.messages.map(({ id }) => id)).toEqual([
      "cached-1",
      "shared-3",
      "fetched-4",
      "live-6",
    ]);
    expect(result.oldestCursor).toBe("cached-1");
  });

  it("leaves an older live row for the next page when the fetched window is disjoint", () => {
    const stale = [message("stale-1", 0), message("stale-2", 1)];
    const fetched = [message("fetched-3", 3), message("fetched-4", 4)];
    const olderLive = message("live-old-2", 2);
    const newerLive = message("live-5", 5);

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: stale,
      cachedAtResponse: [...stale, olderLive, newerLive],
      fetched,
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["fetched-3", "fetched-4", "live-5"]);
    expect(result.oldestCursor).toBe("fetched-3");
  });

  it("removes stale rows from an authoritative window while retaining older pages", () => {
    const older = message("older-1", 1);
    const stale = message("stale-3", 3);
    const fetched = [message("fresh-3", 3), message("fresh-4", 4)];

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [older, stale],
      cachedAtResponse: [older, stale],
      fetched,
      authoritative: true,
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["older-1", "fresh-3", "fresh-4"]);
    expect(result.oldestCursor).toBe("older-1");
  });

  it("sorts timestamps using sub-millisecond precision before the id tie-breaker", () => {
    const earlier = message("z-earlier", 0, ".000001");
    const later = message("a-later", 0, ".000002");

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [],
      cachedAtResponse: [],
      fetched: [later, earlier],
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["z-earlier", "a-later"]);
  });

  it("uses the id tie-breaker after truncating timestamps to backend microseconds", () => {
    const first = message("z-tie", 0, ".0000011");
    const second = message("a-tie", 0, ".0000012");

    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [],
      cachedAtResponse: [],
      fetched: [first, second],
    });

    expect(result.messages.map(({ id }) => id)).toEqual(["a-tie", "z-tie"]);
  });

  it("keeps the current cache when an empty response has no replacement boundary", () => {
    const cached = [message("cached-1", 1)];

    expect(
      reconcileLatestMessageWindow({
        cachedAtRequest: cached,
        cachedAtResponse: cached,
        fetched: [],
      }),
    ).toEqual({ messages: cached, oldestCursor: "cached-1" });
  });
});

const SERVER_RUNNING = "server running";
const FIRST_REVISION = "2026-10-07T05:00:00Z";
const SECOND_REVISION = "2026-10-07T05:00:01Z";
const THIRD_REVISION = "2026-10-07T05:00:02Z";

function toolRevision(label: string, updatedAt: string | undefined): Message {
  return {
    ...message("shared-tool", 3),
    type: "tool_call",
    content: label,
    raw_content: `raw ${label}`,
    updated_at: updatedAt,
    metadata: {
      status: label === SERVER_RUNNING ? "running" : "complete",
      normalized: { generic: { output: label } },
    },
  };
}

const revisionCases = [
  {
    name: "older history after a live completion",
    cached: SECOND_REVISION,
    fetched: FIRST_REVISION,
    changed: true,
    winner: "cache",
  },
  {
    name: "older history than the cache at issuance",
    cached: SECOND_REVISION,
    fetched: FIRST_REVISION,
    changed: false,
    winner: "cache",
  },
  {
    name: "newer server after a live update",
    cached: SECOND_REVISION,
    fetched: THIRD_REVISION,
    changed: true,
    winner: "server",
  },
  {
    name: "newer server without a live update",
    cached: FIRST_REVISION,
    fetched: SECOND_REVISION,
    changed: false,
    winner: "server",
  },
  {
    name: "equal revisions after a live update",
    cached: SECOND_REVISION,
    fetched: SECOND_REVISION,
    changed: true,
    winner: "cache",
  },
  {
    name: "equal revisions without a live update",
    cached: SECOND_REVISION,
    fetched: SECOND_REVISION,
    changed: false,
    winner: "server",
  },
  {
    name: "equal instants in different time zones",
    cached: SECOND_REVISION,
    fetched: "2026-10-07T06:00:01+01:00",
    changed: true,
    winner: "cache",
  },
  {
    name: "older nanosecond revision within one microsecond",
    cached: "2026-10-07T05:00:01.000000002Z",
    fetched: "2026-10-07T05:00:01.000000001Z",
    changed: true,
    winner: "cache",
  },
  {
    name: "newer nanosecond revision within one microsecond",
    cached: "2026-10-07T05:00:01.000000001Z",
    fetched: "2026-10-07T05:00:01.000000002Z",
    changed: true,
    winner: "server",
  },
  {
    name: "missing server revision against a valid unchanged cache",
    cached: SECOND_REVISION,
    fetched: undefined,
    changed: false,
    winner: "cache",
  },
  {
    name: "missing cache revision during a live update",
    cached: undefined,
    fetched: SECOND_REVISION,
    changed: true,
    winner: "cache",
  },
  {
    name: "missing revisions during a live update",
    cached: undefined,
    fetched: undefined,
    changed: true,
    winner: "cache",
  },
  {
    name: "unversioned unchanged cache accepts server hydration",
    cached: undefined,
    fetched: SECOND_REVISION,
    changed: false,
    winner: "server",
  },
  {
    name: "unversioned unchanged copies permit hydration",
    cached: undefined,
    fetched: undefined,
    changed: false,
    winner: "server",
  },
  {
    name: "malformed server date cannot replace a valid cache",
    cached: SECOND_REVISION,
    fetched: "0",
    changed: false,
    winner: "cache",
  },
  {
    name: "calendar-normalized server date cannot replace a valid cache",
    cached: SECOND_REVISION,
    fetched: "2026-02-30T05:00:01Z",
    changed: false,
    winner: "cache",
  },
  {
    name: "invalid cache revision during a live update",
    cached: "0",
    fetched: SECOND_REVISION,
    changed: true,
    winner: "cache",
  },
  {
    name: "invalid unchanged cache permits server hydration",
    cached: "2026-02-30T05:00:01Z",
    fetched: SECOND_REVISION,
    changed: false,
    winner: "server",
  },
] as const;

// @covers AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.1, AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.2, AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.3
describe.each(["overlap", "disjoint", "authoritative"] as const)("%s row freshness", (branch) => {
  const applicableCases =
    branch === "disjoint" ? revisionCases.filter((scenario) => scenario.changed) : revisionCases;
  it.each(applicableCases)("selects the complete accepted row for $name", (scenario) => {
    const cached = toolRevision("live result", scenario.cached);
    const baseline = scenario.changed ? toolRevision("baseline", FIRST_REVISION) : cached;
    const server = toolRevision(SERVER_RUNNING, scenario.fetched);
    const cachedAtRequest = branch === "disjoint" ? [message("stale-prefix", 1)] : [baseline];
    const result = reconcileLatestMessageWindow({
      cachedAtRequest,
      cachedAtResponse: [cached],
      fetched: [server],
      authoritative: branch === "authoritative",
    });

    expect(result.messages).toHaveLength(1);
    expect(result.messages[0]).toBe(scenario.winner === "cache" ? cached : server);
    expect(result.oldestCursor).toBe("shared-tool");
  });
});

// @covers AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.4, AC-UI-TRANSCRIPT-HISTORY-FRESHNESS-001.5
describe("freshness and window membership", () => {
  it.each([false, true])(
    "keeps a newer same-ID arrival outside the request baseline (authoritative=%s)",
    (authoritative) => {
      const prefix = message("stale-prefix", 1);
      const live = toolRevision("completed arrival", SECOND_REVISION);
      const server = toolRevision(SERVER_RUNNING, FIRST_REVISION);
      const result = reconcileLatestMessageWindow({
        cachedAtRequest: [prefix],
        cachedAtResponse: [prefix, live],
        fetched: [server],
        authoritative,
      });
      expect(result.messages.find((row) => row.id === live.id)).toBe(live);
      expect(result.messages.map((row) => row.id)).toEqual(
        authoritative ? [prefix.id, live.id] : [live.id],
      );
      expect(result.oldestCursor).toBe(authoritative ? prefix.id : live.id);
    },
  );

  it("removes an absent authoritative row while keeping a newer match, older page, pending row and live addition", () => {
    const older = message("older-page", 1);
    const baseline = toolRevision(SERVER_RUNNING, FIRST_REVISION);
    const live = toolRevision("completed result", SECOND_REVISION);
    const pending = { ...message("optimistic", 5), metadata: { client_queue_id: "queue-1" } };
    const removed = message("removed-on-server", 6);
    const addition = message("live-addition", 7);
    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [older, baseline, pending, removed],
      cachedAtResponse: [older, live, pending, removed, addition],
      fetched: [message("server-added", 4), baseline],
      authoritative: true,
    });
    expect(result.messages.map((row) => row.id)).toEqual([
      older.id,
      live.id,
      "server-added",
      pending.id,
      addition.id,
    ]);
    expect(result.messages[1]).toBe(live);
    expect(result.oldestCursor).toBe(older.id);
  });

  it("clears completed and newly arrived rows on authoritative empty history while retaining pending locals", () => {
    const baseline = toolRevision(SERVER_RUNNING, FIRST_REVISION);
    const live = toolRevision("completed result", SECOND_REVISION);
    const pending = { ...message("optimistic", 4), metadata: { client_queue_id: "queue-1" } };
    const addition = message("live-addition", 5);
    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [baseline, pending],
      cachedAtResponse: [live, pending, addition],
      fetched: [],
      authoritative: true,
    });
    expect(result).toEqual({ messages: [pending], oldestCursor: pending.id });
  });

  it("retains the changed cache and its identity for non-authoritative empty history", () => {
    const baseline = toolRevision(SERVER_RUNNING, FIRST_REVISION);
    const cache = [toolRevision("completed result", SECOND_REVISION), message("live-addition", 5)];
    const result = reconcileLatestMessageWindow({
      cachedAtRequest: [baseline],
      cachedAtResponse: cache,
      fetched: [],
    });
    expect(result.messages).toBe(cache);
    expect(result.oldestCursor).toBe(baseline.id);
  });
});
