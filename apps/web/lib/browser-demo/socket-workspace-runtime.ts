import { DEMO_TERMINAL_ID, demoTerminal, respond } from "./runtime-state";
import {
  buildFileTree,
  normalizeFilePath,
  notifyFileChange,
  simpleHash,
  withRepositoryPrefix,
  workspaceFilePath,
  workspaceFilesForSession,
} from "./workspace-runtime";

export function routeWorkspaceTreeGet(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "workspace.tree.get") {
    const sessionId = String(payload.session_id || "");
    respond(socketId, id, {
      root: buildFileTree(String(payload.path || ""), workspaceFilesForSession(sessionId)),
    });
    return true;
  }
  return false;
}

export function routeWorkspaceFileGet(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "workspace.file.get" || action === "workspace.file.get_at_ref") {
    const path = workspaceFilePath(payload);
    const content = workspaceFilesForSession(String(payload.session_id || ""))[path];
    if (content === undefined) {
      // i18n-exempt: demo protocol fixtures retain backend wire errors and seeded prompts
      respond(socketId, id, { message: `File not found: ${path}` }, true);
      return true;
    }
    respond(socketId, id, { path, content, size: content.length, is_binary: false });
    return true;
  }
  return false;
}

export function routeWorkspaceFilesSearch(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "workspace.files.search") {
    const query = String(payload.query || "").toLowerCase();
    const limit = Number(payload.limit ?? 20);
    const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
    const matches = Object.keys(workspaceFiles)
      .filter((path) => path.toLowerCase().includes(query))
      .slice(0, limit);
    respond(socketId, id, { files: matches });
    return true;
  }
  return false;
}

export function routeWorkspaceFileUpdate(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "workspace.file.update") {
    const path = workspaceFilePath(payload);
    const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
    const desiredContent = payload.desired_content;
    if (typeof desiredContent !== "string") {
      // i18n-exempt: demo protocol fixtures retain backend wire errors and seeded prompts
      respond(socketId, id, { message: "The demo editor requires desired_content" }, true);
      return true;
    }
    workspaceFiles[path] = desiredContent;
    respond(socketId, id, {
      path,
      success: true,
      new_hash: simpleHash(desiredContent),
      resolution: "applied",
    });
    notifyFileChange(String(payload.session_id || ""), path, "write");
    return true;
  }
  return false;
}

export function routeWorkspaceFileCreate(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "workspace.file.create") {
    const path = workspaceFilePath(payload);
    workspaceFilesForSession(String(payload.session_id || ""))[path] = "";
    respond(socketId, id, { path, success: true });
    notifyFileChange(String(payload.session_id || ""), path, "create");
    return true;
  }
  return false;
}

export function routeWorkspaceFileDelete(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "workspace.file.delete") {
    const path = workspaceFilePath(payload);
    const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
    for (const filePath of Object.keys(workspaceFiles)) {
      if (filePath === path || filePath.startsWith(`${path}/`)) delete workspaceFiles[filePath];
    }
    respond(socketId, id, { path, success: true });
    notifyFileChange(String(payload.session_id || ""), path, "remove");
    return true;
  }
  return false;
}

export function routeWorkspaceFileRename(
  socketId: string,
  id: string,
  action: string,
  payload: Record<string, unknown>,
): boolean {
  if (action === "workspace.file.rename") {
    const repositoryPrefix = normalizeFilePath(payload.repo);
    const oldPath = withRepositoryPrefix(normalizeFilePath(payload.old_path), repositoryPrefix);
    const newPath = withRepositoryPrefix(normalizeFilePath(payload.new_path), repositoryPrefix);
    const workspaceFiles = workspaceFilesForSession(String(payload.session_id || ""));
    for (const filePath of Object.keys(workspaceFiles)) {
      if (filePath !== oldPath && !filePath.startsWith(`${oldPath}/`)) continue;
      const renamedPath = `${newPath}${filePath.slice(oldPath.length)}`;
      workspaceFiles[renamedPath] = workspaceFiles[filePath];
      delete workspaceFiles[filePath];
    }
    respond(socketId, id, { old_path: oldPath, new_path: newPath, success: true });
    notifyFileChange(String(payload.session_id || ""), newPath, "rename");
    return true;
  }
  return false;
}

export function routeUserShellList(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (action === "user_shell.list") {
    respond(socketId, id, { shells: [demoTerminal()] });
    return true;
  }
  return false;
}

export function routeUserShellCreate(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (action === "user_shell.create") {
    respond(socketId, id, {
      terminal_id: DEMO_TERMINAL_ID,
      kind: "ordinary",
      seq: 1,
      display_name: "Terminal 1",
      state: "open",
      pty_status: "running",
      closable: true,
    });
    return true;
  }
  return false;
}

export function routeUserShellDestroy(
  socketId: string,
  id: string,
  action: string,
  _payload: Record<string, unknown>,
): boolean {
  if (
    action === "user_shell.destroy" ||
    action === "user_shell.stop" ||
    action === "user_shell.rename" ||
    action === "user_shell.park" ||
    action === "user_shell.resume"
  ) {
    respond(socketId, id, { success: true });
    return true;
  }
  return false;
}
