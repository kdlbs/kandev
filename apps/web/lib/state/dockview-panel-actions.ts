import type { DockviewApi } from "dockview-react";
import { focusOrAddPanel } from "./dockview-layout-builders";

type StoreGet = () => {
  api: DockviewApi | null;
  centerGroupId: string;
  rightTopGroupId: string;
  rightBottomGroupId: string;
  selectedDiff: { path: string; content?: string } | null;
};
type StoreSet = (
  partial: Partial<{ selectedDiff: { path: string; content?: string } | null }>,
) => void;

type SimplePanelOpts = {
  id: string;
  component: string;
  title: string;
  params?: Record<string, unknown>;
};

function addSimplePanel(api: DockviewApi, groupId: string, opts: SimplePanelOpts): void {
  focusOrAddPanel(api, { ...opts, position: { referenceGroup: groupId } });
}

export function buildPanelActions(set: StoreSet, get: StoreGet) {
  const getFileName = (path: string) => path.split("/").pop() || path;

  return {
    addChatPanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "chat",
        component: "chat",
        tabComponent: "permanentTab",
        title: "Agent",
        position: { referenceGroup: centerGroupId },
      });
    },
    addChangesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "changes",
        component: "changes",
        title: "Changes",
      });
    },
    addFilesPanel: (groupId?: string) => {
      const { api, rightTopGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? rightTopGroupId, {
        id: "files",
        component: "files",
        title: "Files",
      });
    },
    addDiffViewerPanel: (path?: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      if (path) set({ selectedDiff: { path, content } });
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: "diff-viewer",
        component: "diff-viewer",
        title: "Diff Viewer",
        params: { kind: "all" },
      });
    },
    addFileDiffPanel: (path: string, content?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: `diff:file:${path}`,
        component: "diff-viewer",
        title: `Diff [${getFileName(path)}]`,
        params: { kind: "file", path, content },
      });
    },
    addCommitDetailPanel: (sha: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: `commit:${sha}`,
        component: "commit-detail",
        title: sha.slice(0, 7),
        params: { commitSha: sha },
      });
    },
    addFileEditorPanel: (path: string, name: string, quiet?: boolean) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(
        api,
        {
          id: `file:${path}`,
          component: "file-editor",
          title: name,
          params: { path },
          position: { referenceGroup: centerGroupId },
        },
        quiet,
      );
    },
    addBrowserPanel: (url?: string, groupId?: string) => {
      const { api, centerGroupId } = get();
      if (!api) return;
      const browserId = url ? `browser:${url}` : `browser:${Date.now()}`;
      addSimplePanel(api, groupId ?? centerGroupId, {
        id: browserId,
        component: "browser",
        title: "Browser",
        params: { url: url ?? "" },
      });
    },
  };
}

export function buildExtraPanelActions(get: StoreGet) {
  return {
    addVscodePanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: "VS Code",
        position: { referenceGroup: centerGroupId },
      });
    },
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    openInternalVscode: (_goto: { file: string; line: number; col: number } | null) => {
      const { api } = get();
      if (!api) return;
      const existing = api.getPanel("vscode");
      if (existing) {
        existing.api.setActive();
        return;
      }
      focusOrAddPanel(api, {
        id: "vscode",
        component: "vscode",
        title: "VS Code",
        position: { referencePanel: "chat", direction: "right" },
      });
    },
    addPlanPanel: (groupId?: string) => {
      const { api } = get();
      if (!api) return;
      const position = groupId
        ? { referenceGroup: groupId }
        : { referencePanel: "chat" as const, direction: "right" as const };
      focusOrAddPanel(api, { id: "plan", component: "plan", title: "Plan", position });
    },
    addPRPanel: () => {
      const { api, centerGroupId } = get();
      if (!api) return;
      focusOrAddPanel(api, {
        id: "pr-detail",
        component: "pr-detail",
        title: "Pull Request",
        position: { referenceGroup: centerGroupId },
      });
    },
    addTerminalPanel: (terminalId?: string, groupId?: string) => {
      const { api, rightBottomGroupId } = get();
      if (!api) return;
      const id = terminalId ?? `terminal-${Date.now()}`;
      addSimplePanel(api, groupId ?? rightBottomGroupId, {
        id,
        component: "terminal",
        title: "Terminal",
        params: { terminalId: id },
      });
    },
  };
}
