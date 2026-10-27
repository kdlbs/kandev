import { useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import type { WorkloadRunObservation } from "@/lib/types/background-work";

export function useClearedOutput(
  sessionId: string,
  workload: WorkloadRunObservation,
  search: string,
) {
  const outputWindow = useAppStore(
    (state) => state.backgroundWork?.outputWindowsBySessionId?.[sessionId]?.[workload.work_id],
  );
  const [clearAnchor, setClearAnchor] = useState<{
    sessionId: string;
    workId: string;
    runId: string | undefined;
    generation: number | undefined;
    end: number;
  } | null>(null);
  const displayOutput = useMemo(() => {
    let text = workload.output || "";
    if (
      clearAnchor &&
      clearAnchor.sessionId === sessionId &&
      clearAnchor.workId === workload.work_id &&
      clearAnchor.runId === workload.run_id &&
      clearAnchor.generation === outputWindow?.generation
    ) {
      text = text.slice(
        Math.max(0, Math.min(text.length, clearAnchor.end - (outputWindow?.evicted ?? 0))),
      );
    }
    if (!search.trim()) return text;
    const q = search.toLowerCase();
    return text
      .split("\n")
      .filter((line) => line.toLowerCase().includes(q))
      .join("\n");
  }, [
    workload.output,
    workload.work_id,
    workload.run_id,
    sessionId,
    clearAnchor,
    outputWindow,
    search,
  ]);

  const clear = () =>
    setClearAnchor({
      sessionId,
      workId: workload.work_id,
      runId: workload.run_id,
      generation: outputWindow?.generation,
      end: (outputWindow?.evicted ?? 0) + (workload.output || "").length,
    });
  return { displayOutput, clear };
}
