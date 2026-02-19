"use client";

import { createContext, useContext, memo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";

type TaskArchivedState = {
  isArchived: boolean;
  archivedTaskId?: string;
  archivedTaskTitle?: string;
  archivedTaskRepositoryPath?: string;
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
  message = "Workspace not available — this task is archived",
}: {
  message?: string;
}) {
  return (
    <PanelRoot>
      <PanelBody>
        <div className="flex items-center justify-center h-full text-muted-foreground text-xs">
          {message}
        </div>
      </PanelBody>
    </PanelRoot>
  );
});
