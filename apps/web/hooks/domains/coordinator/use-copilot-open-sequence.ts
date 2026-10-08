import { useCallback, useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/api/client";
import {
  getCoordinator,
  getCoordinatorProfileUnavailable,
  openConversation,
  type ConversationResponse,
  type ProfileStatus,
} from "@/lib/api/domains/coordinator-api";

/** The three outcome-table rows that show a translated message with a
 *  **Try again** button; the other error rows (gone, profile-unavailable)
 *  retry only on the next open. */
export type OpenSequenceErrorKind = "load-failed" | "conflict" | "open-failed";

export type OpenSequenceState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "profile-unavailable"; agentStatus: ProfileStatus; executorStatus: ProfileStatus }
  | { kind: "gone" }
  | { kind: "error"; error: OpenSequenceErrorKind }
  | { kind: "ready"; session: ConversationResponse };

function isNotFound(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

/**
 * One GET-then-route open, per
 * `docs/specs/coordinator/system-design/copilot-popover.md
 * #opening-the-conversation`'s outcome table. Pure and directly testable;
 * `useCopilotOpenSequence` wraps it with the in-flight dedupe and the
 * viewed-coordinator staleness guard a component needs.
 */
export async function runOpenSequence(
  workspaceId: string,
  coordinatorId: string,
): Promise<OpenSequenceState> {
  let profile: { agent_profile_status?: ProfileStatus; executor_profile_status?: ProfileStatus };
  try {
    profile = await getCoordinator(workspaceId, coordinatorId);
  } catch (error) {
    if (isNotFound(error)) return { kind: "gone" };
    return { kind: "error", error: "load-failed" };
  }
  const agentStatus = profile.agent_profile_status ?? "missing";
  const executorStatus = profile.executor_profile_status ?? "missing";
  if (agentStatus !== "ok" || executorStatus !== "ok") {
    return { kind: "profile-unavailable", agentStatus, executorStatus };
  }
  try {
    const session = await openConversation(workspaceId, coordinatorId);
    return { kind: "ready", session };
  } catch (error) {
    if (isNotFound(error)) return { kind: "gone" };
    const profileUnavailable = getCoordinatorProfileUnavailable(error);
    if (profileUnavailable) {
      return {
        kind: "profile-unavailable",
        agentStatus: profileUnavailable.agent_profile_status,
        executorStatus: profileUnavailable.executor_profile_status,
      };
    }
    if (error instanceof ApiError && error.errorCode === "conversation_conflict") {
      return { kind: "error", error: "conflict" };
    }
    return { kind: "error", error: "open-failed" };
  }
}

export type UseCopilotOpenSequenceResult = {
  state: OpenSequenceState;
  /** The attempt id that produced the current `state`, or `null` before any
   *  call has settled. A caller that remembers the id its own `open()` or
   *  `retry()` call returned can compare it against this value once `state`
   *  next changes, to tell whether that specific call is what produced the
   *  new state — as opposed to a different call, or one that joined an
   *  already in-flight attempt and never started its own. */
  settledAttemptId: number | null;
  /** Runs the sequence. A call while one is already in flight for this
   *  coordinator joins it rather than starting a second one, and returns
   *  `null` in that case since it started no attempt of its own; otherwise
   *  returns the id of the newly-started attempt. Safe to call on every
   *  popover open, including one that is already `ready`: the profile
   *  statuses must be current at each open. */
  open: () => number | null;
  /** Identical to `open`; kept as a separate name for the **Try again**
   *  button's intent. */
  retry: () => number | null;
};

/**
 * Wraps {@link runOpenSequence} with the in-flight dedupe ("one sequence
 * runs at a time per coordinator: an open while one is in flight joins it")
 * and the stale-response guard ("a response that arrives after the viewed
 * coordinator changed is discarded").
 */
export function useCopilotOpenSequence(
  workspaceId: string,
  coordinatorId: string | null,
): UseCopilotOpenSequenceResult {
  const [state, setState] = useState<OpenSequenceState>({ kind: "idle" });
  const [settledAttemptId, setSettledAttemptId] = useState<number | null>(null);
  const coordinatorGenerationRef = useRef(0);
  const attemptCounterRef = useRef(0);
  const inFlightRef = useRef(false);

  useEffect(() => {
    coordinatorGenerationRef.current += 1;
    inFlightRef.current = false;
    setState({ kind: "idle" });
    setSettledAttemptId(null);
  }, [coordinatorId]);

  const run = useCallback((): number | null => {
    if (!coordinatorId || inFlightRef.current) return null;
    inFlightRef.current = true;
    const coordinatorGeneration = coordinatorGenerationRef.current;
    const attemptId = ++attemptCounterRef.current;
    setState({ kind: "loading" });
    void runOpenSequence(workspaceId, coordinatorId).then((result) => {
      if (coordinatorGenerationRef.current !== coordinatorGeneration) return;
      inFlightRef.current = false;
      setState(result);
      setSettledAttemptId(attemptId);
    });
    return attemptId;
  }, [workspaceId, coordinatorId]);

  return { state, settledAttemptId, open: run, retry: run };
}
