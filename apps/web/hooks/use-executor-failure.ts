import { useRef, useState } from "react";
import { recheckExecutorFailure } from "@/lib/api/domains/executor-failure-api";
import { applyExecutorRecheck } from "@/lib/executor-failure";
import type { ExecutorFailureEpisode } from "@/lib/types/executor-failure";

export function useExecutorFailure(
  taskId: string,
  episode: ExecutorFailureEpisode | null | undefined,
) {
  const latest = useRef({ taskId, episode });
  latest.current = { taskId, episode };
  const [result, setResult] = useState<ExecutorFailureEpisode | null>(null);
  const inFlight = useRef(new Set<string>());
  const [pendingFor, setPendingFor] = useState<string | null>(null);
  const [unverifiedFor, setUnverifiedFor] = useState<string | null>(null);
  const identity = `${taskId}:${episode?.id ?? ""}`;
  const unverified = unverifiedFor === identity;
  const pending = pendingFor === identity;
  const visible = episode && result ? applyExecutorRecheck(episode, result) : episode;
  async function recheck() {
    if (!visible || inFlight.current.has(identity)) return;
    inFlight.current.add(identity);
    setPendingFor(identity);
    setUnverifiedFor(null);
    try {
      const response = await recheckExecutorFailure(taskId, visible);
      const current = latest.current;
      if (current.taskId === taskId && current.episode?.id === visible.id) {
        setResult(applyExecutorRecheck(current.episode, response));
      }
    } catch {
      if (latest.current.taskId === taskId && latest.current.episode?.id === visible.id) {
        setUnverifiedFor(identity);
      }
    } finally {
      inFlight.current.delete(identity);
      setPendingFor((current) => (current === identity ? null : current));
    }
  }
  return { episode: visible, pending, unverified, recheck };
}
