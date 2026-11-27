"use client";

import { NewTaskButton } from "@/components/app-sidebar/new-task-button";
import { useAppStore } from "@/components/state-provider";
import { requestNewTaskCreation } from "@/lib/desktop/new-task-request";

export function MobileNewTaskRow({ onNavigate }: { onNavigate: () => void }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  return (
    <NewTaskButton
      className="h-11 px-3 text-sm"
      testId="mobile-new-task-button"
      disabled={!workspaceId}
      onClick={() => {
        onNavigate();
        requestAnimationFrame(requestNewTaskCreation);
      }}
    />
  );
}
