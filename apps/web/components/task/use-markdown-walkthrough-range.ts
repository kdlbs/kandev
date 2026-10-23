import { useMemo } from "react";
import { useOptionalAppStore } from "@/components/state-provider";
import { getWalkthroughEditorRange } from "@/lib/walkthrough-editor-range";
import { useIsWalkthroughOpenForTask } from "@/lib/walkthrough-open-state";
import type { MarkdownWalkthroughTarget } from "@/lib/markdown/walkthrough-range";

type FileScope = { path: string; repo?: string; taskId?: string | null; sessionId?: string | null };

export function useMarkdownWalkthroughRange({
  path,
  repo,
  taskId,
  sessionId,
}: FileScope): MarkdownWalkthroughTarget | null {
  const activeTaskId = useOptionalAppStore((s) => s.tasks.activeTaskId, null);
  const activeSessionId = useOptionalAppStore((s) => s.tasks.activeSessionId, null);
  const open = useIsWalkthroughOpenForTask(taskId);
  const stepIndex = useOptionalAppStore(
    (s) => (taskId ? (s.walkthroughs?.activeStepByTaskId[taskId] ?? 0) : 0),
    0,
  );
  const step = useOptionalAppStore(
    (s) => (taskId ? (s.walkthroughs?.byTaskId[taskId]?.steps[stepIndex] ?? null) : null),
    null,
  );
  return useMemo(() => {
    if (
      !taskId ||
      taskId !== activeTaskId ||
      !open ||
      (sessionId && activeSessionId && sessionId !== activeSessionId)
    )
      return null;
    const range = getWalkthroughEditorRange({ path, repository_name: repo }, step);
    if (!range || !step) return null;
    return {
      ...range,
      path,
      repositoryName: repo,
      taskId,
      sessionId: sessionId ?? undefined,
      stepIndex,
      navigationLine: step.line,
      file: step.file,
      repo: step.repo,
      key: JSON.stringify([
        taskId,
        sessionId,
        repo,
        path,
        stepIndex,
        range.startLine,
        range.endLine,
      ]),
    };
  }, [path, repo, taskId, sessionId, activeTaskId, activeSessionId, open, stepIndex, step]);
}
