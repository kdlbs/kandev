"use client";

import React, { useCallback, useEffect, useMemo } from "react";
import { PRDetailPanelComponent } from "@/components/github/pr-detail-panel";
import { useAppStore } from "@/components/state-provider";
import { useActiveTaskHasRepos } from "@/hooks/domains/kanban/use-active-task-has-repos";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { useSessionChangesCount } from "@/hooks/domains/session/use-session-changes-count";
import type { ReviewSource } from "@/hooks/domains/session/use-review-sources";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { useFileEditors } from "@/hooks/use-file-editors";
import { createDebugLogger, isDebug } from "@/lib/debug/log";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { isPassthroughSession } from "@/lib/session/is-passthrough-session";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { BrowserPanel } from "./browser-panel";
import type { OpenDiffOptions } from "./changes-diff-target";
import { ChangesPanel } from "./changes-panel";
import { CommitDetailPanel } from "./commit-detail-panel";
import { FileEditorPanel } from "./file-editor-panel";
import { FilesPanel } from "./files-panel";
import { PanelBody, PanelRoot } from "./panel-primitives";
import { PassthroughToolbar } from "./passthrough-toolbar";
import { TaskChangesPanel } from "./task-changes-panel";
import { TaskChatPanel } from "./task-chat-panel";
import { TaskPlanPanel } from "./task-plan-panel";
import { TerminalPanel } from "./terminal-panel";
import { VscodePanel } from "./vscode-panel";

const debugChangesVisibility = createDebugLogger("changes:visibility");

export const CHAT_PANEL_FALLBACK_LABEL = "Agent";

export function resolveChatPanelTitle(agentLabel: string | null | undefined): string {
  return agentLabel || CHAT_PANEL_FALLBACK_LABEL;
}

function useChatSessionTitle(panelId: string, sessionId: string | null) {
  const agentProfileId = useAppStore((state) => {
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    return session?.agent_profile_id ?? null;
  });
  const { agentProfiles } = useSettingsData(Boolean(agentProfileId));
  const agentLabel = useMemo(() => {
    if (!agentProfileId) return null;
    const profile = agentProfiles.find((p) => p.id === agentProfileId);
    if (!profile) return null;
    const parts = profile.label.split(" \u2022 ");
    return parts[1] || parts[0] || profile.label;
  }, [agentProfileId, agentProfiles]);
  useEffect(() => {
    setPanelTitle(panelId, resolveChatPanelTitle(agentLabel));
  }, [panelId, agentLabel]);
}

function ChatContent({ panelId, params }: { panelId: string; params: Record<string, unknown> }) {
  const paramSessionId = params?.sessionId as string | undefined;
  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = paramSessionId ?? storeSessionId;
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const { openFile } = useFileEditors();
  const isPassthrough = useAppStore((state) =>
    sessionId ? isPassthroughSession(state.taskSessions.items[sessionId]) : false,
  );
  useChatSessionTitle(panelId, sessionId);

  if (isPassthrough) {
    return (
      <PanelRoot>
        <PanelBody padding={false} scroll={false}>
          <PassthroughToolbar sessionId={sessionId} taskId={taskId} />
        </PanelBody>
      </PanelRoot>
    );
  }
  return (
    <TaskChatPanel
      sessionId={sessionId}
      taskId={sessionId ? taskId : null}
      onOpenFile={openFile}
      onOpenFileAtLine={openFile}
      hideSessionsDropdown
    />
  );
}

function DiffViewerContent({
  panelId,
  params,
}: {
  panelId: string;
  params: Record<string, unknown>;
}) {
  const selectedDiff = useDockviewStore((s) => s.selectedDiff);
  const setSelectedDiff = useDockviewStore((s) => s.setSelectedDiff);
  const { openFile } = useFileEditors();
  const panelKind = (params?.kind as string) ?? "all";
  const selectedPath = panelKind === "file" ? (params?.path as string) : undefined;
  const sourceFilter = ((params?.source as string) || "all") as "all" | ReviewSource;
  const panelSelectedDiff = panelKind === "all" ? selectedDiff : null;
  const handleClosePanel = useCallback(() => {
    const dockApi = useDockviewStore.getState().api;
    const panel = dockApi?.getPanel(panelId);
    if (dockApi && panel) dockApi.removePanel(panel);
  }, [panelId]);

  return (
    <TaskChangesPanel
      mode={panelKind as "all" | "file"}
      filePath={selectedPath}
      sourceFilter={sourceFilter}
      selectedDiff={panelSelectedDiff}
      onClearSelected={() => setSelectedDiff(null)}
      onOpenFile={openFile}
      onBecameEmpty={handleClosePanel}
    />
  );
}

function describeTaskRepositoriesForDebug(
  task: { repositoryId?: string; repositories?: Array<{ repository_id: string }> } | null,
  taskId: string | null,
) {
  if (!taskId) return { source: "none", repositoryId: "-", repoCount: -1, repoIds: "-" };
  if (task) {
    return {
      source: "taskQuery",
      repositoryId: task.repositoryId ?? "-",
      repoCount: task.repositories?.length ?? -1,
      repoIds: task.repositories?.map((repo) => repo.repository_id).join(",") || "-",
    };
  }
  return { source: "missing", repositoryId: "-", repoCount: -1, repoIds: "-" };
}

function ChangesContent({ panelId }: { panelId: string }) {
  const addDiffViewerPanel = useDockviewStore((s) => s.addDiffViewerPanel);
  const addFileDiffPanel = useDockviewStore((s) => s.addFileDiffPanel);
  const addCommitDetailPanel = useDockviewStore((s) => s.addCommitDetailPanel);
  const { openFile } = useFileEditors();

  // Dynamic title with file count - use environment-stable sessionId so the
  // tab title doesn't re-fetch on same-environment session tab switches.
  const activeSessionId = useEnvironmentSessionId();
  const totalCount = useSessionChangesCount(activeSessionId);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeUiSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const activeTask = useTaskById(activeTaskId);

  // Repo-less tasks have no git changes ever - auto-close the panel so users
  // don't see a permanently empty Changes tab. Gate on a confirmed `false`:
  // `null` means the task hasn't loaded yet, and removing the panel during
  // that window is unrecoverable in the same session.
  const taskHasRepos = useActiveTaskHasRepos();
  useEffect(() => {
    const dockApi = useDockviewStore.getState().api;
    const panel = dockApi?.getPanel(panelId);
    if (isDebug()) {
      const repoDebug = describeTaskRepositoriesForDebug(activeTask, activeTaskId);
      let action = "keep";
      if (taskHasRepos === false) {
        action = panel ? "remove" : "remove-missing-panel";
      }
      debugChangesVisibility("auto-close decision", {
        panelId,
        taskHasRepos: taskHasRepos === null ? "unknown" : String(taskHasRepos),
        action,
        activeTaskId: activeTaskId ?? "-",
        sessionId: activeUiSessionId ?? "-",
        taskSource: repoDebug.source,
        repositoryId: repoDebug.repositoryId,
        repoCount: repoDebug.repoCount,
        repoIds: repoDebug.repoIds,
        livePanelIds: dockApi?.panels.map((p) => p.id).join(",") ?? "-",
      });
    }
    if (taskHasRepos !== false) return;
    if (dockApi && panel) dockApi.removePanel(panel);
  }, [activeTask, activeTaskId, activeUiSessionId, taskHasRepos, panelId]);

  useEffect(() => {
    const title = totalCount > 0 ? `Changes (${totalCount})` : "Changes";
    setPanelTitle(panelId, title);
  }, [totalCount, panelId]);

  const handleEditFile = useCallback((path: string) => openFile(path), [openFile]);
  const handleOpenDiffFile = useCallback(
    (path: string, options?: OpenDiffOptions) =>
      addFileDiffPanel(path, { source: options?.source, repositoryName: options?.repositoryName }),
    [addFileDiffPanel],
  );
  const handleOpenCommitDetail = useCallback(
    (sha: string, repo?: string) => addCommitDetailPanel(sha, { repo }),
    [addCommitDetailPanel],
  );
  const handleOpenDiffAll = useCallback(() => addDiffViewerPanel(), [addDiffViewerPanel]);
  const handleOpenReview = useCallback(() => {
    window.dispatchEvent(new CustomEvent("open-review-dialog"));
  }, []);

  return (
    <ChangesPanel
      onOpenDiffFile={handleOpenDiffFile}
      onEditFile={handleEditFile}
      onOpenCommitDetail={handleOpenCommitDetail}
      onOpenDiffAll={handleOpenDiffAll}
      onOpenReview={handleOpenReview}
    />
  );
}

function FilesContent() {
  const { openFile } = useFileEditors();
  const handleOpenFile = useCallback(
    (file: { path: string; name: string; content: string }) => openFile(file.path),
    [openFile],
  );
  return <FilesPanel onOpenFile={handleOpenFile} />;
}

function PlanContent() {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  return <TaskPlanPanel taskId={taskId} visible />;
}

const COMPONENT_ALIASES: Record<string, string> = {
  "diff-files": "changes",
  "all-files": "files",
};

function resolveComponent(component: string): string {
  return COMPONENT_ALIASES[component] ?? component;
}

export function renderPanel(
  panelId: string,
  component: string,
  params: Record<string, unknown>,
): React.ReactNode {
  const resolved = resolveComponent(component);

  switch (resolved) {
    case "sidebar":
      return null;
    case "chat":
      return <ChatContent panelId={panelId} params={params} />;
    case "diff-viewer":
      return <DiffViewerContent panelId={panelId} params={params} />;
    case "file-editor":
      return <FileEditorPanel panelId={panelId} params={params} />;
    case "commit-detail":
      return <CommitDetailPanel panelId={panelId} params={params} />;
    case "changes":
      return <ChangesContent panelId={panelId} />;
    case "files":
      return <FilesContent />;
    case "terminal":
      return <TerminalPanel panelId={panelId} params={params} />;
    case "browser":
      return <BrowserPanel panelId={panelId} params={params} />;
    case "vscode":
      return <VscodePanel panelId={panelId} />;
    case "plan":
      return <PlanContent />;
    case "pr-detail":
      return <PRDetailPanelComponent panelId={panelId} params={params} />;
    default:
      return <div className="p-4 text-muted-foreground">Unknown panel: {component}</div>;
  }
}
