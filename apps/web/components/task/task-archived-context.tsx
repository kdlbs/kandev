"use client";

import { createContext, useContext, memo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useTranslation } from "react-i18next";

type TaskArchivedState = {
  isArchived: boolean;
  archivedTaskId?: string;
  archivedTaskTitle?: string;
  /**
   * The archived task's repository as a stable `owner/repo` slug (or name).
   * Sidebar rows render this, so it must be the same identity ordinary rows
   * carry, never a local clone path.
   */
  archivedTaskRepositoryLabel?: string;
  archivedTaskUpdatedAt?: string;
};

const TaskArchivedContext = createContext<TaskArchivedState>({ isArchived: false });

export const TaskArchivedProvider = TaskArchivedContext.Provider;

export function useIsTaskArchived() {
  return useContext(TaskArchivedContext).isArchived;
}

export function useArchivedTaskState() {
  return useContext(TaskArchivedContext);
}

export const ArchivedPanelPlaceholder = memo(function ArchivedPanelPlaceholder({
  message,
}: {
  message?: string;
}) {
  const { t } = useTranslation();
  // Resolved in the body: a parameter default runs before the hook is in scope.
  const text = message ?? t("task:workspaceUnavailableArchived");
  return (
    <PanelRoot>
      <PanelBody>
        <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
          {text}
        </div>
      </PanelBody>
    </PanelRoot>
  );
});
