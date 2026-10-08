import type { ExecutorFailureEpisode } from "./types/executor-failure";

/** A recheck owns only the episode and revision that the user inspected. */
export function applyExecutorRecheck(
  current: ExecutorFailureEpisode,
  response: ExecutorFailureEpisode,
): ExecutorFailureEpisode {
  if (response.id !== current.id || response.revision < current.revision) return current;
  return response;
}

/** Structured reasons describe the cause. Exit 137 alone cannot establish OOM. */
export function executorFailureTitleKey(
  episode: Pick<ExecutorFailureEpisode, "observation" | "current_outcome">,
): string {
  if (
    episode.observation.outcome === "unknown" &&
    episode.observation.reason === "WorkerUnavailable"
  ) {
    return "task:executorFailureConnectionLost";
  }
  if (episode.current_outcome === "unknown") return "task:executorFailureUnverified";
  if (episode.observation.outcome === "missing") return "task:executorFailureMissing";
  if (episode.observation.reason === "Evicted") return "task:executorFailureEvicted";
  if (episode.observation.outcome === "restarted") return "task:executorFailureRestarted";
  return "task:executorFailureStopped";
}

export function providerRecoveryKey(outcome: unknown): string {
  if (outcome === "fresh") return "task:executorFailureConversationFresh";
  if (outcome === "restored") return "task:executorFailureConversationRestored";
  return "task:executorFailureConversationUnknown";
}

/** Select shared task evidence or one specifically owned session episode. */
export function executorFailureForOwner(
  episode: ExecutorFailureEpisode | null | undefined,
  sessionId?: string,
): ExecutorFailureEpisode | null {
  if (!episode || episode.state !== "active") return null;
  if (sessionId) return episode.session_id === sessionId ? episode : null;
  return episode.session_id ? null : episode;
}

/** Shared environments are presented by each attached session composer. */
export function executorFailureForComposer(
  episode: ExecutorFailureEpisode | null | undefined,
  sessionId: string | null,
) {
  if (!sessionId || episode?.state !== "active") return null;
  return !episode.session_id || episode.session_id === sessionId ? episode : null;
}
