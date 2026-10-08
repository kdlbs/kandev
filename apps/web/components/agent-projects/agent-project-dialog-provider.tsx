"use client";

import { createContext, useContext, useMemo, useState, type ReactNode } from "react";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { linkToTask } from "@/lib/links";
import type { AgentProject } from "@/lib/types/http-agent-projects";
import { AgentProjectActionDialog } from "./agent-project-action-dialog";
import { AgentProjectFormDialog } from "./agent-project-form-dialog";

type ProjectActionTarget = { project: AgentProject; action: "archive" | "delete" };

type AgentProjectDialogControls = {
  openCreate: () => void;
  editProject: (project: AgentProject) => void;
  openAction: (project: AgentProject, action: ProjectActionTarget["action"]) => void;
};

const AgentProjectDialogContext = createContext<AgentProjectDialogControls | null>(null);

export function AgentProjectDialogProvider({ children }: { children: ReactNode }) {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const router = useRouter();
  const [createOpen, setCreateOpen] = useState(false);
  const [editingProject, setEditingProject] = useState<AgentProject | null>(null);
  const [actionTarget, setActionTarget] = useState<ProjectActionTarget | null>(null);
  const controls = useMemo<AgentProjectDialogControls>(
    () => ({
      openCreate: () => setCreateOpen(true),
      editProject: setEditingProject,
      openAction: (project, action) => setActionTarget({ project, action }),
    }),
    [],
  );
  const openCoordinator = (taskId: string) => router.push(linkToTask(taskId));

  return (
    <AgentProjectDialogContext.Provider value={controls}>
      {children}
      {workspaceId ? (
        <>
          <AgentProjectFormDialog
            open={createOpen}
            onOpenChange={setCreateOpen}
            workspaceId={workspaceId}
            onOpenCoordinator={openCoordinator}
          />
          <AgentProjectFormDialog
            open={Boolean(editingProject)}
            onOpenChange={(open) => {
              if (!open) setEditingProject(null);
            }}
            workspaceId={workspaceId}
            project={editingProject ?? undefined}
          />
          <AgentProjectActionDialog
            open={Boolean(actionTarget)}
            onOpenChange={(open) => {
              if (!open) setActionTarget(null);
            }}
            workspaceId={workspaceId}
            project={actionTarget?.project ?? null}
            action={actionTarget?.action ?? "archive"}
          />
        </>
      ) : null}
    </AgentProjectDialogContext.Provider>
  );
}

export function useAgentProjectDialogs(): AgentProjectDialogControls {
  const controls = useContext(AgentProjectDialogContext);
  if (!controls) {
    // i18n-exempt: developer-only invariant for incorrect provider wiring.
    throw new Error("Agent Project dialogs require AgentProjectDialogProvider.");
  }
  return controls;
}
