"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { useTaskChanges } from "@/hooks/domains/coordinator/use-task-changes";
import { ChangeState } from "./change-state";
import { DependencyEditor } from "./dependency-editor";
import type { QueueMergeContextValue } from "./queue-merge-context";

type Panel = "changes" | "predecessors";

function PanelBody({
  panel,
  taskId,
  merge,
}: {
  panel: Panel;
  taskId: string;
  merge: QueueMergeContextValue;
}) {
  const changes = useTaskChanges(
    merge.workspaceId,
    merge.coordinatorId,
    taskId,
    panel === "changes",
  );
  if (panel === "changes") {
    return (
      <ChangeState changes={changes.changes} status={changes.status} onRetry={changes.retry} />
    );
  }
  return (
    <DependencyEditor
      workspaceId={merge.workspaceId}
      coordinatorId={merge.coordinatorId}
      taskId={taskId}
      canManage={merge.canManage}
    />
  );
}

/** The change state and predecessor controls under a Queue row, one panel open at a time. */
export function QueueRowMerge({
  taskId,
  merge,
}: {
  taskId: string;
  merge: QueueMergeContextValue;
}) {
  const { t } = useTranslation();
  const [panel, setPanel] = useState<Panel | null>(null);
  const toggle = (next: Panel) => setPanel((current) => (current === next ? null : next));
  return (
    <div className="space-y-2 px-2 pb-2" data-testid={`queue-merge-${taskId}`}>
      <div className="flex flex-col gap-2 sm:flex-row">
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-0"
          aria-expanded={panel === "changes"}
          onClick={() => toggle("changes")}
          data-testid={`queue-changes-toggle-${taskId}`}
        >
          {t("coordinator:mergeChangesToggle")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="min-h-11 cursor-pointer sm:min-h-0"
          aria-expanded={panel === "predecessors"}
          onClick={() => toggle("predecessors")}
          data-testid={`queue-deps-toggle-${taskId}`}
        >
          {t("coordinator:mergePredecessorsToggle")}
        </Button>
      </div>
      {panel && <PanelBody key={panel} panel={panel} taskId={taskId} merge={merge} />}
    </div>
  );
}
