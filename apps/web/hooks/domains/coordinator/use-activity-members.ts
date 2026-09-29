"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listWorkspaceMembers } from "@/lib/api/domains/team-access-api";
import type { ActivityItem } from "@/lib/api/domains/coordinator-activity-api";

export type MemberNamesStatus = "loading" | "loaded" | "failed";

/** How a recorded person renders: a name, no person, or "a former member". */
export type PersonName = { kind: "none" } | { kind: "named"; name: string } | { kind: "former" };

export const SECOND_READ_AFTER_MS = 30_000;

type MemberState = { status: MemberNamesStatus; names: ReadonlyMap<string, string> };

const LOADING: MemberState = { status: "loading", names: new Map() };

export function resolvePerson(state: MemberState, userId: string | null): PersonName {
  if (!userId || state.status !== "loaded") return { kind: "none" };
  const name = state.names.get(userId);
  if (name === undefined) return { kind: "former" };
  return name ? { kind: "named", name } : { kind: "none" };
}

function recordedPeople(rows: ActivityItem[]): string[] {
  const ids: string[] = [];
  for (const row of rows) {
    if (row.actor_user_id) ids.push(row.actor_user_id);
    if (row.undone_by) ids.push(row.undone_by);
  }
  return ids;
}

/**
 * The workspace member list as the only name source: read on mount and at most
 * once more per mount, when a list arrival leaves a recorded person absent (or
 * the first read failed) and the first read started over 30 seconds ago.
 */
export function useActivityMembers(workspaceId: string) {
  const [state, setState] = useState<MemberState>(LOADING);
  const stateRef = useRef<MemberState>(LOADING);
  const firstStartedAt = useRef(0);
  const secondUsed = useRef(false);
  const inFlight = useRef(false);
  const generation = useRef(0);

  const read = useCallback(
    (gen: number) => {
      inFlight.current = true;
      listWorkspaceMembers(workspaceId)
        .then((res) => {
          if (generation.current !== gen) return;
          const names = new Map(res.members.map((m) => [m.user_id, m.display_name ?? ""]));
          stateRef.current = { status: "loaded", names };
          setState(stateRef.current);
        })
        .catch(() => {
          if (generation.current !== gen) return;
          stateRef.current = { status: "failed", names: stateRef.current.names };
          setState(stateRef.current);
        })
        .finally(() => {
          if (generation.current === gen) inFlight.current = false;
        });
    },
    [workspaceId],
  );

  useEffect(() => {
    const gen = ++generation.current;
    stateRef.current = LOADING;
    setState(LOADING);
    secondUsed.current = false;
    firstStartedAt.current = Date.now();
    read(gen);
    return () => {
      generation.current += 1;
      inFlight.current = false;
    };
  }, [read]);

  const noteArrival = useCallback(
    (rows: ActivityItem[]) => {
      const people = recordedPeople(rows);
      const current = stateRef.current;
      const absent =
        current.status === "failed" ||
        (current.status === "loaded" && people.some((id) => !current.names.has(id)));
      if (secondUsed.current || inFlight.current || people.length === 0 || !absent) return;
      if (Date.now() - firstStartedAt.current <= SECOND_READ_AFTER_MS) return;
      secondUsed.current = true;
      read(generation.current);
    },
    [read],
  );

  const resolve = useCallback((userId: string | null) => resolvePerson(state, userId), [state]);

  return { resolve, noteArrival, status: state.status };
}
