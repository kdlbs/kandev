import type {
  AttachTaskWorkspaceSourcesRequest,
  WorkspaceRepositorySourceRequest,
  WorkspaceSourceRequest,
} from "@/lib/types/http";
import { getWorkspaceSourceCapabilities } from "./executor-capabilities";
import { looksLikeSupportedRemoteURL } from "./remote-url";

export type WorkspaceSourceRow = {
  key: string;
  kind: "repository" | "folder";
  sourceType?: "saved_repository" | "local_repository" | "remote_repository" | "folder";
  repositoryId?: string;
  localPath?: string;
  remoteUrl?: string;
  provider?: "github" | "gitlab" | "azure_devops";
  providerRepoId?: string;
  providerOwner?: string;
  providerName?: string;
  baseBranch?: string;
  checkoutBranch?: string;
  displayName?: string;
};

type RowKind = NonNullable<WorkspaceSourceRow["sourceType"]> | WorkspaceSourceRow["kind"];

let nextKey = 0;

export function addWorkspaceSourceRow(
  rows: WorkspaceSourceRow[],
  kind: RowKind,
  executorType?: string | null,
): WorkspaceSourceRow[] {
  if (kind === "folder" && !getWorkspaceSourceCapabilities(executorType).canAddFolders) return rows;
  nextKey += 1;
  const key = `workspace-source-${nextKey}`;
  const sourceType = kind === "repository" ? "saved_repository" : kind;
  return [
    ...rows,
    sourceType === "folder"
      ? { key, kind: "folder", sourceType, localPath: "", displayName: "" }
      : { key, kind: "repository", sourceType, baseBranch: "" },
  ];
}

export function removeWorkspaceSourceRow(
  rows: WorkspaceSourceRow[],
  key: string,
): WorkspaceSourceRow[] {
  return rows.filter((row) => row.key !== key);
}

export function updateWorkspaceSourceRow(
  rows: WorkspaceSourceRow[],
  key: string,
  patch: Partial<WorkspaceSourceRow>,
): WorkspaceSourceRow[] {
  return rows.map((row) =>
    row.key === key ? { ...row, ...patch, key: row.key, kind: row.kind } : row,
  );
}

export function getWorkspaceSourceValidation(rows: WorkspaceSourceRow[]): Record<string, string> {
  const errors: Record<string, string> = {};
  const repositoryKeys = new Set<string>();
  const folderPaths = new Set<string>();
  for (const row of rows) {
    const error = validateRow(row, repositoryKeys, folderPaths);
    if (error) errors[row.key] = error;
  }
  return errors;
}

export function buildWorkspaceSourcesPayload(
  rows: WorkspaceSourceRow[],
): AttachTaskWorkspaceSourcesRequest {
  return { sources: rows.map(toRequestSource) };
}

function validateRow(
  row: WorkspaceSourceRow,
  repositoryKeys: Set<string>,
  folderPaths: Set<string>,
): string | null {
  if (row.kind === "folder") {
    const path = canonicalPath(row.localPath);
    if (!path) return "Choose a folder.";
    if (folderPaths.has(path)) return "This folder is already selected.";
    folderPaths.add(path);
    return null;
  }
  const locator = repositoryLocator(row);
  if (row.remoteUrl && !isValidRemoteUrl(row.remoteUrl)) {
    return "Enter a valid remote repository URL and base branch.";
  }
  if (!locator || !row.baseBranch?.trim()) return "Choose a repository and base branch.";
  const duplicateKey = `${locator}\u0000${row.baseBranch.trim()}\u0000${row.checkoutBranch?.trim() ?? ""}`;
  if (repositoryKeys.has(duplicateKey)) return "This repository and branch are already selected.";
  repositoryKeys.add(duplicateKey);
  return null;
}

function repositoryLocator(row: WorkspaceSourceRow): string {
  return row.repositoryId || canonicalPath(row.localPath) || row.remoteUrl?.trim() || "";
}

function canonicalPath(path: string | undefined): string {
  return path?.trim().replace(/\/+$/, "") ?? "";
}

function isValidRemoteUrl(value: string): boolean {
  return looksLikeSupportedRemoteURL(value.trim());
}

function toRequestSource(row: WorkspaceSourceRow): WorkspaceSourceRequest {
  if (row.kind === "folder") {
    return {
      kind: "folder",
      local_path: canonicalPath(row.localPath),
      ...(row.displayName?.trim() ? { display_name: row.displayName.trim() } : {}),
    };
  }
  return {
    kind: "repository",
    ...repositoryLocation(row),
    ...repositoryProvider(row),
    base_branch: row.baseBranch?.trim() ?? "",
    ...(row.checkoutBranch?.trim() ? { checkout_branch: row.checkoutBranch.trim() } : {}),
  };
}

function repositoryLocation(
  row: WorkspaceSourceRow,
): Pick<WorkspaceRepositorySourceRequest, "repository_id" | "local_path" | "remote_url"> {
  if (row.repositoryId) return { repository_id: row.repositoryId };
  if (row.localPath) return { local_path: canonicalPath(row.localPath) };
  return row.remoteUrl ? { remote_url: row.remoteUrl.trim() } : {};
}

function repositoryProvider(
  row: WorkspaceSourceRow,
): Pick<
  WorkspaceRepositorySourceRequest,
  "provider" | "provider_repo_id" | "provider_owner" | "provider_name"
> {
  return {
    ...(row.provider ? { provider: row.provider } : {}),
    ...(row.providerRepoId ? { provider_repo_id: row.providerRepoId } : {}),
    ...(row.providerOwner ? { provider_owner: row.providerOwner } : {}),
    ...(row.providerName ? { provider_name: row.providerName } : {}),
  };
}
