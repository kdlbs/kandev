import { type Task } from "@/lib/types/http";
import { createDemoFiles } from "./demo-files";
import { createDemoMultiRepoFiles } from "./demo-multi-repo-files";
import { findTask, notify, persist, state } from "./runtime-state";
import { DEMO_IDS, demoRepository } from "./scenario";

export function buildFileTree(path: string, workspaceFiles: Record<string, string>) {
  const normalizedPath = normalizeFilePath(path);
  const prefix = normalizedPath ? `${normalizedPath}/` : "";
  const childNames = new Set<string>();
  for (const filePath of Object.keys(workspaceFiles)) {
    if (!filePath.startsWith(prefix)) continue;
    const relative = filePath.slice(prefix.length);
    if (relative) childNames.add(relative.split("/")[0]);
  }
  const children = Array.from(childNames, (name) => {
    const childPath = prefix + name;
    const isDirectory = Object.keys(workspaceFiles).some((filePath) =>
      filePath.startsWith(`${childPath}/`),
    );
    return {
      name,
      path: childPath,
      is_dir: isDirectory,
      size: isDirectory ? undefined : (workspaceFiles[childPath]?.length ?? 0),
    };
  });
  return {
    name: normalizedPath.split("/").pop() || demoRepository.name,
    path: normalizedPath,
    is_dir: true,
    children,
  };
}

export function workspaceFilesForSession(sessionId: string) {
  const session = state.sessions.find((candidate) => candidate.id === sessionId);
  const task = session ? findTask(session.task_id) : undefined;
  const taskId = task?.id ?? "demo-task-checkout";
  const workspaces = (state.filesByTask ??= {});
  return (workspaces[taskId] ??= seedTaskWorkspace(task));
}

export function workspaceFilePath(payload: Record<string, unknown>) {
  return withRepositoryPrefix(normalizeFilePath(payload.path), normalizeFilePath(payload.repo));
}

export function withRepositoryPrefix(path: string, repositoryPrefix: string) {
  if (!repositoryPrefix || path === repositoryPrefix || path.startsWith(`${repositoryPrefix}/`)) {
    return path;
  }
  return `${repositoryPrefix}/${path}`;
}

export function normalizeFilePath(value: unknown) {
  return String(value || "")
    .replaceAll("\\", "/")
    .replace(/^\.\//, "")
    .replace(/^\/+|\/+$/g, "");
}

export function simpleHash(content: string) {
  let hash = 0;
  for (let index = 0; index < content.length; index += 1) {
    hash = (hash * 31 + content.charCodeAt(index)) | 0;
  }
  return `demo-${Math.abs(hash).toString(16)}`;
}

export function notifyFileChange(sessionId: string, path: string, operation: string) {
  persist();
  notify("session.workspace.file.changes", {
    session_id: sessionId,
    changes: [
      {
        session_id: sessionId,
        task_id: state.sessions.find((session) => session.id === sessionId)?.task_id ?? "",
        agent_id: DEMO_IDS.agent,
        timestamp: new Date().toISOString(),
        path,
        operation,
      },
    ],
  });
}

export function seedTaskWorkspace(task: Task | undefined): Record<string, string> {
  if ((task?.repositories?.length ?? 0) > 1) return createDemoMultiRepoFiles();
  if (task?.repositories?.[0]?.repository_id === DEMO_IDS.apiRepository) {
    return Object.fromEntries(
      Object.entries(createDemoMultiRepoFiles())
        .filter(([path]) => path.startsWith("acme-api/"))
        .map(([path, content]) => [path.slice("acme-api/".length), content]),
    );
  }
  return createDemoFiles();
}
