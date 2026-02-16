import { create } from 'zustand';
import type { DockviewApi, AddPanelOptions, SerializedDockview } from 'dockview-react';
import { getSessionLayout, setSessionLayout } from '@/lib/local-storage';

export type FileEditorState = {
    path: string;
    name: string;
    content: string;
    originalContent: string;
    originalHash: string;
    isDirty: boolean;
    isBinary?: boolean;
};

/** Direction relative to a reference panel or group. */
export type PanelDirection = 'left' | 'right' | 'above' | 'below';

/** A deferred panel operation applied after the next layout build / restore. */
export type DeferredPanelAction = {
    id: string;
    component: string;
    title: string;
    /** Where to place the panel. 'tab' adds as a tab in the reference group. */
    placement: 'tab' | PanelDirection;
    /** Panel ID or group alias to position relative to. Defaults to 'chat'. */
    referencePanel?: string;
    params?: Record<string, unknown>;
};

type DockviewStore = {
    api: DockviewApi | null;
    setApi: (api: DockviewApi | null) => void;

    // File editor tracking
    openFiles: Map<string, FileEditorState>;
    setFileState: (path: string, state: FileEditorState) => void;
    updateFileState: (path: string, updates: Partial<FileEditorState>) => void;
    removeFileState: (path: string) => void;
    clearFileStates: () => void;

    // Layout methods
    buildDefaultLayout: (api: DockviewApi) => void;
    resetLayout: () => void;

    // Add-only panel actions (single-instance panels use focusOrAddPanel)
    addChatPanel: () => void;
    addChangesPanel: (groupId?: string) => void;
    addFilesPanel: (groupId?: string) => void;
    addDiffViewerPanel: (path?: string, content?: string, groupId?: string) => void;
    addCommitDetailPanel: (sha: string, groupId?: string) => void;
    addFileEditorPanel: (path: string, name: string, quiet?: boolean) => void;
    addBrowserPanel: (url?: string, groupId?: string) => void;
    addPlanPanel: (groupId?: string) => void;
    addTerminalPanel: (terminalId?: string, groupId?: string) => void;

    // Cross-panel shared state
    selectedDiff: { path: string; content?: string } | null;
    setSelectedDiff: (diff: { path: string; content?: string } | null) => void;

    // Active group tracking
    activeGroupId: string | null;

    // Group IDs for header action routing
    centerGroupId: string;
    rightTopGroupId: string;
    rightBottomGroupId: string;
    sidebarGroupId: string;

    // Per-session layout switching
    isRestoringLayout: boolean;
    currentLayoutSessionId: string | null;
    switchSessionLayout: (oldSessionId: string | null, newSessionId: string) => void;

    // Deferred panel actions — queued before navigation, applied after next layout build / restore
    deferredPanelActions: DeferredPanelAction[];
    queuePanelAction: (action: DeferredPanelAction) => void;
};

const SIDEBAR_GROUP = 'group-sidebar';
const CENTER_GROUP = 'group-center';
const RIGHT_TOP_GROUP = 'group-right-top';
const RIGHT_BOTTOM_GROUP = 'group-right-bottom';

// Layout sizing constants (single source of truth)
export const LAYOUT_SIDEBAR_RATIO = 2.5 / 10;
export const LAYOUT_RIGHT_RATIO = 2.5 / 10;
export const LAYOUT_SIDEBAR_MAX_PX = 350;
export const LAYOUT_RIGHT_MAX_PX = 400;

function focusOrAddPanel(api: DockviewApi, options: AddPanelOptions & { id: string }, quiet = false) {
    const existing = api.getPanel(options.id);
    if (existing) {
        if (!quiet) existing.api.setActive();
        return;
    }
    // Remember the currently active panel so we can restore focus after adding
    const previousActive = quiet ? api.activePanel : null;
    api.addPanel(options);
    // When quiet, restore focus to whatever was active before the add
    if (previousActive) {
        previousActive.api.setActive();
    }
}

/**
 * Drain the deferred panel action queue, applying each action to the dockview API.
 * Call this after a layout build or restore so queued panels appear in the new layout.
 */
function applyDeferredPanelActions(api: DockviewApi, actions: DeferredPanelAction[]): void {
    for (const action of actions) {
        const ref = action.referencePanel ?? 'chat';
        let position: AddPanelOptions['position'];
        if (action.placement === 'tab') {
            const groupId = api.getPanel(ref)?.group?.id;
            if (groupId) {
                position = { referenceGroup: groupId };
            }
        } else {
            position = { referencePanel: ref, direction: action.placement };
        }
        focusOrAddPanel(api, {
            id: action.id,
            component: action.component,
            title: action.title,
            position,
            ...(action.params ? { params: action.params } : {}),
        });
    }
}

/**
 * After fromJSON() restores a layout, apply fixups: re-lock sidebar,
 * fix old panel aliases, and return computed group IDs.
 */
export function applyLayoutFixups(api: DockviewApi): {
    centerGroupId: string;
    rightTopGroupId: string;
    rightBottomGroupId: string;
    sidebarGroupId: string;
} {
    // Re-lock sidebar group and ensure header is visible for workspace tab
    const sidebarPanel = api.getPanel('sidebar');
    if (sidebarPanel) {
        sidebarPanel.group.locked = 'no-drop-target';
        sidebarPanel.group.header.hidden = false;
    }

    // Fix up panel titles from old saved layouts
    const oldChanges = api.getPanel('diff-files');
    if (oldChanges) oldChanges.api.setTitle('Changes');
    const oldFiles = api.getPanel('all-files');
    if (oldFiles) oldFiles.api.setTitle('Files');

    const chatPanel = api.getPanel('chat');
    const changesPanel = api.getPanel('changes') ?? oldChanges;
    const terminalPanel = api.panels.find(
        (p) => p.id.startsWith('terminal-') || p.id === 'terminal-default'
    );

    return {
        centerGroupId: chatPanel?.group?.id ?? CENTER_GROUP,
        rightTopGroupId: changesPanel?.group?.id ?? RIGHT_TOP_GROUP,
        rightBottomGroupId: terminalPanel?.group?.id ?? RIGHT_BOTTOM_GROUP,
        sidebarGroupId: sidebarPanel?.group?.id ?? SIDEBAR_GROUP,
    };
}

export const useDockviewStore = create<DockviewStore>((set, get) => ({
    api: null,
    setApi: (api) => set({ api }),

    activeGroupId: null,

    selectedDiff: null,
    setSelectedDiff: (diff) => set({ selectedDiff: diff }),

    openFiles: new Map(),
    setFileState: (path, state) => {
        set((prev) => {
            const next = new Map(prev.openFiles);
            next.set(path, state);
            return { openFiles: next };
        });
    },
    updateFileState: (path, updates) => {
        set((prev) => {
            const existing = prev.openFiles.get(path);
            if (!existing) return prev;
            const next = new Map(prev.openFiles);
            next.set(path, { ...existing, ...updates });
            return { openFiles: next };
        });
    },
    removeFileState: (path) => {
        set((prev) => {
            const next = new Map(prev.openFiles);
            next.delete(path);
            return { openFiles: next };
        });
    },
    clearFileStates: () => {
        set({ openFiles: new Map() });
    },

    centerGroupId: CENTER_GROUP,
    rightTopGroupId: RIGHT_TOP_GROUP,
    rightBottomGroupId: RIGHT_BOTTOM_GROUP,
    sidebarGroupId: SIDEBAR_GROUP,

    isRestoringLayout: false,
    currentLayoutSessionId: null,

    deferredPanelActions: [],
    queuePanelAction: (action) => set((prev) => ({
        deferredPanelActions: [...prev.deferredPanelActions, action],
    })),

    switchSessionLayout: (oldSessionId, newSessionId) => {
        const { api, currentLayoutSessionId } = get();
        if (!api) return;
        // Idempotent: skip if we already switched to this session
        if (currentLayoutSessionId === newSessionId) return;

        set({ isRestoringLayout: true, currentLayoutSessionId: newSessionId });

        try {
            // Save current layout for the old session
            if (oldSessionId) {
                try {
                    setSessionLayout(oldSessionId, api.toJSON());
                } catch {
                    // Ignore save errors
                }
            }

            // Try to restore the new session's layout
            let restored = false;
            const savedLayout = getSessionLayout(newSessionId);
            if (savedLayout) {
                try {
                    api.fromJSON(savedLayout as SerializedDockview);
                    set(applyLayoutFixups(api));
                    restored = true;
                } catch {
                    // Restore failed, fall through to default
                }
            }

            if (!restored) {
                get().buildDefaultLayout(api);
            }
        } finally {
            set({ isRestoringLayout: false });
        }
    },

    buildDefaultLayout: (api) => {
        // Clear everything
        api.clear();

        const totalWidth = api.width;
        const sidebarWidth = Math.min(Math.round(totalWidth * LAYOUT_SIDEBAR_RATIO), LAYOUT_SIDEBAR_MAX_PX);
        const rightWidth = Math.min(Math.round(totalWidth * LAYOUT_RIGHT_RATIO), LAYOUT_RIGHT_MAX_PX);

        // Chat panel added first (takes full width, then others split off from it)
        api.addPanel({
            id: 'chat',
            component: 'chat',
            tabComponent: 'permanentTab',
            title: 'Chat',
        });

        // Sidebar panel — split off to the left
        const sidebarPanel = api.addPanel({
            id: 'sidebar',
            component: 'sidebar',
            title: 'Sidebar',
            position: { direction: 'left', referencePanel: 'chat' },
            initialWidth: sidebarWidth,
        });
        const sidebarGroup = sidebarPanel.group;
        sidebarGroup.locked = 'no-drop-target';

        // Changes panel (right top group seed)
        const changesPanel = api.addPanel({
            id: 'changes',
            component: 'changes',
            title: 'Changes',
            position: { direction: 'right', referencePanel: 'chat' },
            initialWidth: rightWidth,
        });

        // Files panel (same group as changes)
        const rightTopGroupId = changesPanel.group.id;
        api.addPanel({
            id: 'files',
            component: 'files',
            title: 'Files',
            position: { referenceGroup: rightTopGroupId },
        });

        // Terminal panel (right bottom group seed)
        api.addPanel({
            id: 'terminal-default',
            component: 'terminal',
            title: 'Terminal',
            params: { terminalId: 'shell-default' },
            position: { direction: 'below', referencePanel: 'changes' },
        });

        // Store group IDs from the actual panels
        const chatPanel = api.getPanel('chat');
        const terminalPanel = api.getPanel('terminal-default');

        set({
            centerGroupId: chatPanel?.group?.id ?? CENTER_GROUP,
            rightTopGroupId: rightTopGroupId ?? RIGHT_TOP_GROUP,
            rightBottomGroupId: terminalPanel?.group?.id ?? RIGHT_BOTTOM_GROUP,
            sidebarGroupId: sidebarGroup.id,
        });

        // Apply any queued panel actions (e.g., plan panel from plan mode task creation)
        const pending = get().deferredPanelActions;
        if (pending.length > 0) {
            set({ deferredPanelActions: [] });
            applyDeferredPanelActions(api, pending);
        }
    },

    resetLayout: () => {
        const { api } = get();
        if (!api) return;
        get().buildDefaultLayout(api);
    },

    addChatPanel: () => {
        const { api, centerGroupId } = get();
        if (!api) return;
        focusOrAddPanel(api, {
            id: 'chat',
            component: 'chat',
            tabComponent: 'permanentTab',
            title: 'Chat',
            position: { referenceGroup: centerGroupId },
        });
    },

    addChangesPanel: (groupId) => {
        const { api, rightTopGroupId } = get();
        if (!api) return;
        focusOrAddPanel(api, {
            id: 'changes',
            component: 'changes',
            title: 'Changes',
            position: { referenceGroup: groupId ?? rightTopGroupId },
        });
    },

    addFilesPanel: (groupId) => {
        const { api, rightTopGroupId } = get();
        if (!api) return;
        focusOrAddPanel(api, {
            id: 'files',
            component: 'files',
            title: 'Files',
            position: { referenceGroup: groupId ?? rightTopGroupId },
        });
    },

    addDiffViewerPanel: (path, content, groupId) => {
        const { api, centerGroupId } = get();
        if (!api) return;
        if (path) {
            set({ selectedDiff: { path, content } });
        }
        focusOrAddPanel(api, {
            id: 'diff-viewer',
            component: 'diff-viewer',
            title: 'Diff Viewer',
            position: { referenceGroup: groupId ?? centerGroupId },
        });
    },

    addCommitDetailPanel: (sha, groupId) => {
        const { api, centerGroupId } = get();
        if (!api) return;
        const panelId = `commit:${sha}`;
        focusOrAddPanel(api, {
            id: panelId,
            component: 'commit-detail',
            title: sha.slice(0, 7),
            params: { commitSha: sha },
            position: { referenceGroup: groupId ?? centerGroupId },
        });
    },

    addFileEditorPanel: (path, name, quiet) => {
        const { api, centerGroupId } = get();
        if (!api) return;
        const panelId = `file:${path}`;
        focusOrAddPanel(api, {
            id: panelId,
            component: 'file-editor',
            title: name,
            params: { path },
            position: { referenceGroup: centerGroupId },
        }, quiet);
    },

    addBrowserPanel: (url, groupId) => {
        const { api, centerGroupId } = get();
        if (!api) return;
        const browserId = url ? `browser:${url}` : `browser:${Date.now()}`;
        focusOrAddPanel(api, {
            id: browserId,
            component: 'browser',
            title: 'Browser',
            params: { url: url ?? '' },
            position: { referenceGroup: groupId ?? centerGroupId },
        });
    },

    addPlanPanel: (groupId) => {
        const { api } = get();
        if (!api) return;
        if (groupId) {
            // Header "+" menu: add as tab in the specified group
            focusOrAddPanel(api, {
                id: 'plan',
                component: 'plan',
                title: 'Plan',
                position: { referenceGroup: groupId },
            });
        } else {
            // Toolbar toggle: split right from the chat panel
            focusOrAddPanel(api, {
                id: 'plan',
                component: 'plan',
                title: 'Plan',
                position: { referencePanel: 'chat', direction: 'right' },
            });
        }
    },

    addTerminalPanel: (terminalId, groupId) => {
        const { api, rightBottomGroupId } = get();
        if (!api) return;
        const id = terminalId ?? `terminal-${Date.now()}`;
        focusOrAddPanel(api, {
            id,
            component: 'terminal',
            title: 'Terminal',
            params: { terminalId: id },
            position: { referenceGroup: groupId ?? rightBottomGroupId },
        });
    },
}));

/**
 * Perform a layout switch between sessions. Call this after setActiveSession()
 * so that remounted components (e.g. TerminalPanel) read the new session.
 */
export function performLayoutSwitch(oldSessionId: string | null, newSessionId: string): void {
    useDockviewStore.getState().switchSessionLayout(oldSessionId, newSessionId);
}
