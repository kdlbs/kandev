"use client";

import type { ReactNode } from "react";
import type { TaskPlanRevision } from "@/lib/types/http";
import {
  RevisionInlineConfirmation,
  RevisionRestoreAction,
} from "./task-plan-revision-restore-actions";

type TaskPlanRevisionRowProps = {
  revision: TaskPlanRevision;
  isCurrent: boolean;
  isSaving: boolean;
  rowConfirmTarget: TaskPlanRevision | null;
  isFinePointer: boolean;
  onRevertRequest: (revision: TaskPlanRevision) => void;
  onRevertCancel: () => void;
  onRevert: (revision: TaskPlanRevision) => Promise<void>;
  children: ReactNode;
};

export function TaskPlanRevisionRow({
  revision,
  isCurrent,
  isSaving,
  rowConfirmTarget,
  isFinePointer,
  onRevertRequest,
  onRevertCancel,
  onRevert,
  children,
}: TaskPlanRevisionRowProps) {
  return (
    <li
      className="px-3 py-2.5 hover:bg-accent/30"
      data-testid="plan-revision-row"
      data-revision-id={revision.id}
      data-revision-number={revision.revision_number}
    >
      <div className="flex min-w-0 items-center gap-3">
        {children}
        <RevisionRestoreAction
          revision={revision}
          isCurrent={isCurrent}
          isSaving={isSaving}
          rowConfirmTarget={rowConfirmTarget}
          isFinePointer={isFinePointer}
          onRevertRequest={onRevertRequest}
          onRevertCancel={onRevertCancel}
          onRevert={onRevert}
        />
      </div>
      <RevisionInlineConfirmation
        revision={revision}
        isCurrent={isCurrent}
        isSaving={isSaving}
        isFinePointer={isFinePointer}
        rowConfirmTarget={rowConfirmTarget}
        onRevertCancel={onRevertCancel}
        onRevert={onRevert}
      />
    </li>
  );
}
