import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { FileEditorState } from "@/lib/state/dockview-store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId, PREVIEW_FILE_EDITOR_ID } from "@/lib/state/dockview-panel-actions";
import { setActiveTabForSession } from "@/lib/local-storage";
import { isWorkspaceTreePath, workspaceRelativeFilePath } from "@/lib/workspace-file-path";

export type RenameAdapter = {
  capture: (oldPath: string, newPath: string) => { collision: boolean; publish: () => boolean };
};
export type RenameFile = Pick<FileEditorState, "path" | "repo" | "name">;
type Reservation = { token: symbol; scope: string; paths: string[] };
const reservations = new WeakMap<StoreApi<AppState>, Reservation[]>();
type PendingOpen = { scope: string; path: string; invalid: boolean };
const pendingOpens = new WeakMap<StoreApi<AppState>, Set<PendingOpen>>();
const releaseListeners = new WeakMap<StoreApi<AppState>, Set<() => void>>();
let retargeting = false;

export function isFileRenameRetargeting() {
  return retargeting;
}
export function renameScope(store: StoreApi<AppState>, sessionId: string): string {
  return store.getState().environmentIdBySessionId[sessionId] ?? `session:${sessionId}`;
}
export function treePath(file: Pick<RenameFile, "path" | "repo">): string | null {
  if (
    !file.path ||
    !isWorkspaceTreePath(file.path) ||
    (file.repo && !isWorkspaceTreePath(file.repo))
  )
    return null;
  return file.repo ? `${file.repo}/${file.path}` : file.path;
}
export function isWithin(path: string, parent: string) {
  return path === parent || path.startsWith(`${parent}/`);
}
export function retargetFile(
  file: RenameFile,
  oldPath: string,
  newPath: string,
): RenameFile | null {
  const current = treePath(file);
  if (!current || !isWithin(current, oldPath)) return null;
  const next = newPath + current.slice(oldPath.length);
  const repo = file.repo && next.startsWith(`${file.repo}/`) ? file.repo : undefined;
  const path = repo ? next.slice(repo.length + 1) : next;
  return { path, name: path.split("/").pop() || path, repo };
}
export function isRenameReserved(
  store: StoreApi<AppState>,
  session: string,
  path: string,
  repo?: string,
) {
  const full = treePath({ path, repo });
  if (!full) return false;
  const scope = renameScope(store, session);
  return (reservations.get(store) ?? []).some(
    (entry) => entry.scope === scope && entry.paths.some((p) => isWithin(full, p)),
  );
}
export function trackRenameOpen(
  store: StoreApi<AppState>,
  session: string,
  path: string,
  repo?: string,
) {
  const item = {
    scope: renameScope(store, session),
    path: treePath({ path, repo }) ?? "",
    invalid: false,
  };
  const entries = pendingOpens.get(store) ?? new Set<PendingOpen>();
  entries.add(item);
  pendingOpens.set(store, entries);
  return { current: () => !item.invalid, dispose: () => entries.delete(item) };
}
export async function waitForRenameRelease(
  store: StoreApi<AppState>,
  session: string,
  paths: Array<Pick<RenameFile, "path" | "repo">>,
) {
  while (paths.some(({ path, repo }) => isRenameReserved(store, session, path, repo))) {
    await new Promise<void>((resolve) => {
      const listeners = releaseListeners.get(store) ?? new Set<() => void>();
      const done = () => {
        listeners.delete(done);
        resolve();
      };
      listeners.add(done);
      releaseListeners.set(store, listeners);
      if (!paths.some(({ path, repo }) => isRenameReserved(store, session, path, repo))) done();
    });
  }
}
export function reserveRename(
  store: StoreApi<AppState>,
  scope: string,
  oldPath: string,
  newPath: string,
): (() => void) | null {
  const entries = reservations.get(store) ?? [];
  if (
    entries.some(
      (entry) =>
        entry.scope === scope &&
        entry.paths.some((p) => [oldPath, newPath].some((q) => isWithin(p, q) || isWithin(q, p))),
    )
  )
    return null;
  for (const open of pendingOpens.get(store) ?? []) {
    if (open.scope === scope && [oldPath, newPath].some((p) => isWithin(open.path, p)))
      open.invalid = true;
  }
  const item = { token: Symbol(), scope, paths: [oldPath, newPath] };
  reservations.set(store, [...entries, item]);
  return () => {
    reservations.set(
      store,
      (reservations.get(store) ?? []).filter((entry) => entry.token !== item.token),
    );
    for (const listener of [...(releaseListeners.get(store) ?? [])]) listener();
  };
}

type Panel = NonNullable<
  ReturnType<NonNullable<ReturnType<typeof useDockviewStore.getState>["api"]>["getPanel"]>
>;
type Captured = {
  key: string;
  file: FileEditorState;
  panel: Panel | undefined;
  target: RenameFile;
};
function panelFor(
  key: string,
  api: NonNullable<ReturnType<typeof useDockviewStore.getState>["api"]>,
) {
  const pinned = api.getPanel(`file:${key}`);
  if (pinned) return pinned;
  const file = useDockviewStore.getState().openFiles.get(key);
  const legacy = file?.repo ? api.getPanel(`file:${file.path}`) : undefined;
  if (legacy?.params?.path === file?.path && legacy?.params?.repo === file?.repo) return legacy;
  const preview = api.getPanel(PREVIEW_FILE_EDITOR_ID);
  return preview?.params?.previewItemId === key ? preview : undefined;
}
type DockApi = NonNullable<ReturnType<typeof useDockviewStore.getState>["api"]>;

function isExternalFile(file: Pick<RenameFile, "path" | "repo">, root?: string) {
  if (!root || file.repo) return false;
  const posix = (path: string) => path.startsWith("/") && !path.startsWith("//");
  const windows = (path: string) => /^[A-Za-z]:[\\/]/.test(path);
  const isWindows = windows(root) && windows(file.path);
  if (!isWindows && !(posix(root) && posix(file.path))) return false;
  const canonical = (path: string, rootPath: boolean) => {
    if (path.includes("\0") || (!isWindows && path.includes("\\"))) return false;
    const normalized = isWindows ? path.replaceAll("\\", "/") : path;
    const rest = normalized.slice(isWindows ? 3 : 1);
    if (!rootPath && (!rest || rest.endsWith("/"))) return false;
    const relative = rootPath ? rest.replace(/\/$/, "") : rest;
    return (
      relative === "" || relative.split("/").every((part) => part && part !== "." && part !== "..")
    );
  };
  return (
    canonical(root, true) &&
    canonical(file.path, false) &&
    workspaceRelativeFilePath(file.path, root) === null
  );
}

function collectOpenFiles(api: DockApi, oldPath: string, newPath: string, root?: string) {
  const captured: Captured[] = [];
  const targets = new Set<string>();
  let collision = false;
  for (const [key, file] of useDockviewStore.getState().openFiles) {
    const full = treePath(file);
    if (!full) {
      if (!isExternalFile(file, root)) collision = true;
      continue;
    }
    const target = retargetFile(file, oldPath, newPath);
    if (!target) {
      if (isWithin(full, newPath)) collision = true;
      continue;
    }
    const targetTree = treePath(target)!;
    const panel = panelFor(key, api);
    if (targets.has(targetTree) || !panel || !file.instanceId || !file.originalHash)
      collision = true;
    targets.add(targetTree);
    captured.push({ key, file, panel, target });
  }
  return { captured, collision };
}

function panelConflicts(
  panel: Panel,
  oldPath: string,
  newPath: string,
  captured: Captured[],
  root?: string,
) {
  if (panel.view?.contentComponent !== "file-editor") return false;
  const path = panel.params?.path;
  const repo = panel.params?.repo;
  if (typeof path !== "string" || (repo !== undefined && typeof repo !== "string")) return true;
  const full = treePath({ path, repo });
  if (!full) return !isExternalFile({ path, repo }, root);
  return (
    (isWithin(full, newPath) && !isWithin(full, oldPath)) ||
    (isWithin(full, oldPath) && !captured.some((entry) => entry.panel === panel))
  );
}

export function captureDockviewRename(
  oldPath: string,
  newPath: string,
  session: string,
  root?: string,
) {
  const { api } = useDockviewStore.getState();
  if (!api) return { collision: false, publish: () => true };
  const { captured, collision } = collectOpenFiles(api, oldPath, newPath, root);
  return {
    collision:
      collision ||
      (api.panels ?? []).some((panel) => panelConflicts(panel, oldPath, newPath, captured, root)),
    publish: () => publishDockviewRename(api, captured, session),
  };
}

function replacePinnedPanel(api: DockApi, panel: Panel, target: RenameFile, key: string) {
  const group = panel.group;
  const previousActive = api.activePanel;
  const selected = group.activePanel === panel;
  const descriptor = panel.toJSON();
  const replacement = api.addPanel({
    id: `file:${key}`,
    component: panel.view.contentComponent,
    tabComponent: panel.view.tabComponent,
    title: target.name,
    params: { ...panel.params, path: target.path, repo: target.repo },
    position: { referenceGroup: group.id, index: group.panels.indexOf(panel) },
    inactive: true,
    renderer: descriptor.renderer,
    minimumWidth: descriptor.minimumWidth,
    maximumWidth: descriptor.maximumWidth,
    minimumHeight: descriptor.minimumHeight,
    maximumHeight: descriptor.maximumHeight,
  });
  api.removePanel(panel);
  if (selected) replacement.api.setActive();
  if (previousActive && previousActive !== panel) previousActive.api.setActive();
}

function rollbackEntry(api: DockApi, entry: Captured, file: FileEditorState, key: string) {
  const { panel, target, key: oldKey } = entry;
  if (!panel || api.getPanel(panel.id) !== panel || panel.params?.path === target.path) return;
  const rollback = new Map(useDockviewStore.getState().openFiles);
  const retained = rollback.get(key);
  if (!retained || retained.instanceId !== file.instanceId) return;
  rollback.delete(key);
  rollback.set(oldKey, { ...retained, path: file.path, name: file.name, repo: file.repo });
  useDockviewStore.setState({ openFiles: rollback });
}

function publishEntry(api: DockApi, entry: Captured) {
  const { panel, target, key: oldKey } = entry;
  if (!panel) return true;
  const key = buildRepoScopedItemId(target.path, target.repo);
  const next = new Map(useDockviewStore.getState().openFiles);
  const file = next.get(oldKey);
  if (!file || file.instanceId !== entry.file.instanceId) return true;
  next.delete(oldKey);
  next.set(key, { ...file, ...target });
  try {
    useDockviewStore.setState({ openFiles: next });
    if (panel.id === PREVIEW_FILE_EDITOR_ID) {
      panel.api.updateParameters({
        ...panel.params,
        path: target.path,
        repo: target.repo,
        previewItemId: key,
      });
      panel.setTitle(target.name);
    } else replacePinnedPanel(api, panel, target, key);
    return true;
  } catch {
    rollbackEntry(api, entry, file, key);
    return false;
  }
}

function currentOwnedPanel(api: DockApi, entry: Captured): Panel | undefined {
  const current = panelFor(entry.key, api);
  if (current === entry.panel) return current;
  const original = entry.panel;
  if (
    original?.id === PREVIEW_FILE_EDITOR_ID &&
    current?.id === `file:${entry.key}` &&
    current.params?.path === entry.file.path &&
    current.params?.repo === entry.file.repo
  )
    return current;
  return undefined;
}

function publishDockviewRename(
  api: NonNullable<ReturnType<typeof useDockviewStore.getState>["api"]>,
  captured: Captured[],
  session: string,
) {
  if (useDockviewStore.getState().api !== api) return true;
  const live = useDockviewStore.getState().openFiles;
  const entries = captured.flatMap((entry) => {
    if (live.get(entry.key)?.instanceId !== entry.file.instanceId) return [];
    const panel = currentOwnedPanel(api, entry);
    return panel ? [{ ...entry, panel }] : [];
  });
  if (!entries.length) return true;
  const keys = new Set(entries.map((entry) => entry.key));
  const targetPaths = new Set(entries.map((entry) => treePath(entry.target)));
  if ([...live].some(([key, file]) => !keys.has(key) && targetPaths.has(treePath(file))))
    return false;
  retargeting = true;
  try {
    for (const entry of entries) {
      if (!publishEntry(api, entry)) return false;
    }
  } finally {
    retargeting = false;
    if (api.activePanel) setActiveTabForSession(session, api.activePanel.id);
    useDockviewStore.setState({ openFiles: new Map(useDockviewStore.getState().openFiles) });
  }
  return true;
}
