"use client";

import { useEffect, useMemo, useState } from "react";
import { listDirectoryUsers, listWorkspaceMembers } from "@/lib/api/domains/team-access-api";
import type { DirectoryUser, WorkspaceMember } from "@/lib/types/team-access";

export type Person = { id: string; name: string };

/**
 * One in-flight directory request for the whole page.
 *
 * The board renders a name per card, so an uncached per-consumer fetch would
 * mean one request per card. The directory is global and small (id + display
 * name for active users), so it is fetched once and shared. The cost is
 * staleness: a user created after this load renders as a raw id until the next
 * one, which is the same tradeoff the agent-profile list already makes.
 */
let directoryCache: Promise<DirectoryUser[]> | null = null;

function loadDirectory(): Promise<DirectoryUser[]> {
  directoryCache ??= listDirectoryUsers()
    .then((res) => (res.users ?? []) as DirectoryUser[])
    .catch(() => {
      // Do not cache a failure: a refused or dropped request should be retried
      // by the next consumer rather than blanking every name for the session.
      directoryCache = null;
      return [];
    });
  return directoryCache;
}

/** Test seam: drops the shared directory cache. */
export function resetDirectoryCacheForTests(): void {
  directoryCache = null;
}

/**
 * Display names for user ids, from the shared directory.
 *
 * Use this for read-only surfaces (a board card, a chip). Anything that offers
 * a choice of assignee wants {@link useAssignablePeople}, which also folds in
 * the workspace member list.
 */
export function useDirectoryNames(): { nameFor: (userId: string) => string } {
  const [users, setUsers] = useState<DirectoryUser[]>([]);

  useEffect(() => {
    let cancelled = false;
    loadDirectory().then((list) => {
      if (!cancelled) setUsers(list);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  const byId = useMemo(() => new Map(users.map((u) => [u.id, u.display_name || u.id])), [users]);

  // Falls back to the id so an assignee the directory does not carry (a
  // disabled account) still renders as something rather than as unassigned.
  return { nameFor: (userId: string) => byId.get(userId) ?? userId };
}

/**
 * The people a task can be assigned to, and the names to show for them.
 *
 * Two sources, because neither alone is the set. The workspace member list
 * misses everyone who reaches an `org`-visible workspace without a member row,
 * which is the common case and would otherwise render a colleague as a raw
 * user id. The directory covers those, but a private workspace can hold a
 * member the directory omits.
 *
 * Reach is deliberately NOT computed here. It depends on visibility,
 * membership and the org boundary, and the server is the authority: assigning
 * someone who cannot see the workspace is refused with a message written to be
 * shown as-is. Guessing in the browser would only add a second, divergent rule.
 */
export function useAssignablePeople(
  workspaceId?: string | null,
  options?: { enabled?: boolean },
): {
  people: Person[];
  nameFor: (userId: string) => string;
} {
  // Callers render nothing when there is no identity to assign to, but hooks
  // run before that early return, so without this the picker would fetch on
  // every task page of an install that has authentication switched off.
  const enabled = options?.enabled ?? true;
  const [people, setPeople] = useState<Person[]>([]);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    // The directory is not workspace-scoped, so it is loaded even with no
    // active workspace: the task routes do not always populate one, and
    // gating both calls on it leaves every name showing as a raw id.
    Promise.all([
      loadDirectory(),
      workspaceId
        ? listWorkspaceMembers(workspaceId).catch(() => ({ members: [], total: 0 }))
        : Promise.resolve({ members: [], total: 0 }),
    ]).then(([directory, members]) => {
      if (cancelled) return;
      const byId = new Map<string, string>();
      for (const user of directory) byId.set(user.id, user.display_name || user.id);
      for (const member of (members.members ?? []) as WorkspaceMember[]) {
        if (!byId.has(member.user_id)) {
          byId.set(member.user_id, member.display_name || member.user_id);
        }
      }
      setPeople(Array.from(byId, ([id, name]) => ({ id, name })));
    });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, enabled]);

  const byId = useMemo(() => new Map(people.map((p) => [p.id, p.name])), [people]);

  return {
    people,
    nameFor: (userId: string) => byId.get(userId) ?? userId,
  };
}
