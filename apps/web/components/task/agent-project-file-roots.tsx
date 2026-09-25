"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useAgentProjectTaskContext } from "./agent-project-task-context";
import { AgentProjectContextPanel } from "./agent-project-context-panel";
import { cn } from "@/lib/utils";

export function AgentProjectFileRoots({ children }: { children: React.ReactNode }) {
  const { t } = useTranslation();
  const projectTask = useAgentProjectTaskContext();
  const enabled = useFeature("agentProjects");
  const { isMobile } = useResponsiveBreakpoint();
  const [selectedRoot, setSelectedRoot] = useState<"context" | "workspace">(
    projectTask?.tier === "coordinator" ? "context" : "workspace",
  );
  useEffect(() => {
    setSelectedRoot(projectTask?.tier === "coordinator" ? "context" : "workspace");
  }, [projectTask?.taskId, projectTask?.tier]);

  if (!enabled || !projectTask) return children;

  return (
    <div className="flex h-full min-h-0 flex-col" data-testid="agent-project-file-roots">
      <div
        role="tablist"
        aria-label={t("projects:fileRoots")}
        className="flex shrink-0 gap-1 border-b p-1"
      >
        {(["context", "workspace"] as const).map((root) => (
          <button
            key={root}
            type="button"
            role="tab"
            aria-selected={selectedRoot === root}
            className={cn(
              "rounded px-3 text-sm font-medium text-muted-foreground hover:bg-muted/70 hover:text-foreground",
              selectedRoot === root && "bg-muted text-foreground",
              isMobile ? "min-h-11 flex-1" : "min-h-7",
            )}
            onClick={() => setSelectedRoot(root)}
          >
            {t(root === "context" ? "projects:context" : "projects:workspace")}
          </button>
        ))}
      </div>
      <div role="tabpanel" className="min-h-0 flex-1">
        {selectedRoot === "context" ? (
          <AgentProjectContextPanel
            workspaceId={projectTask.workspaceId}
            projectId={projectTask.projectId}
          />
        ) : (
          children
        )}
      </div>
    </div>
  );
}
