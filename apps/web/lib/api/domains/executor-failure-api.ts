import { fetchJson } from "../client";
import type { ExecutorFailureEpisode } from "@/lib/types/executor-failure";

export function recheckExecutorFailure(taskId: string, episode: ExecutorFailureEpisode) {
  return fetchJson<ExecutorFailureEpisode>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/executor-failure/recheck`,
    {
      init: {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ episode_id: episode.id, revision: episode.revision }),
      },
    },
  );
}
