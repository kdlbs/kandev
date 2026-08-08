"use client";

import React, { useCallback, useEffect } from "react";
import { ReviewDetailPanelComponent } from "./review-detail-panel";
import { MRDetailPanelComponent } from "@/components/gitlab/mr-detail-panel";
import { useAppStore } from "@/components/state-provider";
import { useSessionChangesCount } from "@/hooks/domains/session/use-session-changes-count";
import type { ReviewSource } from "@/hooks/domains/session/use-review-sources";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { useFileEditors } from "@/hooks/use-file-editors";
import { usePanelActive } from "@/hooks/use-panel-active";
import { t } from "@/lib/i18n";
import { setPanelTitle } from "@/lib/layout/panel-portal-manager";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { BrowserPanel } from "./browser-panel";
import type { CommitDetailTarget, OpenDiffOptions } from "./changes-diff-target";
import { ChangesPanel } from "./changes-panel";
import { CommitDetailPanel } from "./commit-detail-panel";
import { FileEditorPanel } from "./file-editor-panel";
import { FilesPanel } from "./files-panel";
import { PanelBody, PanelRoot } from "./panel-primitives";
import { PassthroughToolbar } from "./passthrough-toolbar";
import { PluginTaskPanel } from "./plugin-task-panel";
import { TaskChangesPanel } from "./task-changes-panel";
import { TaskChatPanel } from "./task-chat-panel";
import { TaskPlanPanel } from "./task-plan-panel";
import { TerminalPanel } from "./terminal-panel";
import { TodosContent } from "./todos-panel-content";
import { VscodePanel } from "./vscode-panel";

export const CHAT_PANEL_FALLBACK_LABEL = "Agent";

export function resolveChatPanelTitle(agentLabel: string | null | undefined): string {
  return agentLabel || CHAT_PANEL_FALLBACK_LABEL;
}

function useChatSessionTitle(panelId: string, sessionId: string | null) {
  const agentLabel = useAppStore((state) => {
    if (!sessionId) return null;
    const session = state.taskSessions.items[sessionId];
    // User-supplied session name wins over the derived profile label,
    // matching the session tab title precedence (resolveSessionTabTitle).
    if (session?.name) return session.name;
    if (!session?.agent_profile_id) return null;
    const profile = state.agentProfiles.items.find(
      (p: { id: string }) => p.id === session.agent_profile_id,
    );
    if (!profile) return null;
    const parts = profile.label.split(" \u2022 ");
    return parts[1] || parts[0] || profile.label;
  });
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
    sessionId ? state.taskSessions.items[sessionId]?.is_passthrough === true : false,
  );
  useChatSessionTitle(panelId, sessionId);
  const isVisible = usePanelActive(panelId);

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
      isVisible={isVisible}
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
  const selectedRepositoryName =
    panelKind === "file" ? (params?.repositoryName as string | undefined) : undefined;
  const selectedPRKey = panelKind === "file" ? (params?.prKey as string | undefined) : undefined;
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
      fileRepositoryName={selectedRepositoryName}
      prKey={selectedPRKey}
      sourceFilter={sourceFilter}
      selectedDiff={panelSelectedDiff}
      onClearSelected={() => setSelectedDiff(null)}
      onOpenFile={openFile}
      onBecameEmpty={handleClosePanel}
    />
  );
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

  useEffect(() => {
    const title = totalCount > 0 ? `Changes (${totalCount})` : "Changes";
    setPanelTitle(panelId, title);
  }, [totalCount, panelId]);

  const handleEditFile = useCallback(
    (path: string, repo?: string) => openFile(path, repo),
    [openFile],
  );
  const handleOpenDiffFile = useCallback(
    (path: string, options?: OpenDiffOptions) =>
      addFileDiffPanel(path, {
        source: options?.source,
        repositoryName: options?.repositoryName,
        prKey: options?.prKey,
      }),
    [addFileDiffPanel],
  );
  const handleOpenCommitDetail = useCallback(
    (target: CommitDetailTarget) => addCommitDetailPanel(target),
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

/**
 * One renderer per component name — a lookup table rather than a growing
 * switch, so adding a panel type (like "plugin-panel") never trips the
 * function-complexity lint ceiling (R3, docs/plans/plugins).
 */
type PanelRenderer = (panelId: string, params: Record<string, unknown>) => React.ReactNode;

const PANEL_RENDERERS: Record<string, PanelRenderer> = {
  sidebar: () => null,
  chat: (panelId, params) => <ChatContent panelId={panelId} params={params} />,
  "diff-viewer": (panelId, params) => <DiffViewerContent panelId={panelId} params={params} />,
  "file-editor": (panelId, params) => <FileEditorPanel panelId={panelId} params={params} />,
  "commit-detail": (panelId, params) => <CommitDetailPanel panelId={panelId} params={params} />,
  changes: (panelId) => <ChangesContent panelId={panelId} />,
  files: () => <FilesContent />,
  terminal: (panelId, params) => <TerminalPanel panelId={panelId} params={params} />,
  browser: (panelId, params) => <BrowserPanel panelId={panelId} params={params} />,
  vscode: (panelId) => <VscodePanel panelId={panelId} />,
  plan: () => <PlanContent />,
  todos: () => <TodosContent />,
  "pr-detail": (panelId, params) => (
    <ReviewDetailPanelComponent panelId={panelId} params={params} />
  ),
  "mr-detail": (panelId, params) => (
    <MRDetailPanelComponent
      panelId={panelId}
      params={{ mrKey: typeof params.mrKey === "string" ? params.mrKey : undefined }}
    />
  ),
  "plugin-panel": (panelId, params) => (
    <PluginTaskPanel
      pluginId={typeof params.pluginId === "string" ? params.pluginId : ""}
      panelKey={typeof params.panelKey === "string" ? params.panelKey : ""}
      panelId={panelId}
      presentation="desktop"
    />
  ),
};

export function renderPanel(
  panelId: string,
  component: string,
  params: Record<string, unknown>,
): React.ReactNode {
  const renderer = PANEL_RENDERERS[resolveComponent(component)];
  if (renderer) return renderer(panelId, params);
  return <div className="p-4 text-muted-foreground">{t("common:unknownPanel", { component })}</div>;
}
