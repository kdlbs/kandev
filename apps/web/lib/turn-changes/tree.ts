import type { TurnFileChange, TurnRepositoryChange } from "@/lib/types/turn-changes";

export type TurnChangeTreeFile = { kind: "file"; file: TurnFileChange };
export type TurnChangeTreeFolder = {
  kind: "folder";
  name: string;
  path: string;
  folders: TurnChangeTreeFolder[];
  files: TurnChangeTreeFile[];
};
export type TurnChangeTreeRepository = {
  kind: "repository";
  id: string;
  label: string;
  folders: TurnChangeTreeFolder[];
  files: TurnChangeTreeFile[];
};

type MutableFolder = Omit<TurnChangeTreeFolder, "folders" | "files"> & {
  folders: Map<string, MutableFolder>;
  files: TurnChangeTreeFile[];
};

function freezeFolder(folder: MutableFolder): TurnChangeTreeFolder {
  return {
    ...folder,
    folders: [...folder.folders.values()]
      .sort((a, b) => a.name.localeCompare(b.name))
      .map(freezeFolder),
    files: folder.files.sort((a, b) => a.file.path.localeCompare(b.file.path)),
  };
}

export function buildTurnChangeTree(
  repositories: TurnRepositoryChange[],
  filesByRepository: Record<string, TurnFileChange[]>,
  repositoryFallback: string,
): TurnChangeTreeRepository[] {
  return repositories.map((repository, index) => {
    const folders = new Map<string, MutableFolder>();
    const files: TurnChangeTreeFile[] = [];
    for (const file of filesByRepository[repository.id] ?? []) {
      const segments = file.path.split("/").filter(Boolean);
      let level = folders;
      let parentPath = "";
      for (const [segmentIndex, segment] of segments.slice(0, -1).entries()) {
        parentPath = parentPath ? `${parentPath}/${segment}` : segment;
        let folder = level.get(segment);
        if (!folder) {
          folder = {
            kind: "folder",
            name: segment,
            path: parentPath,
            folders: new Map(),
            files: [],
          };
          level.set(segment, folder);
        }
        level = folder.folders;
        if (segmentIndex === segments.length - 2) break;
      }
      const row = { kind: "file" as const, file };
      const parent = segments.length > 1 ? parentPath.split("/").at(-1) : undefined;
      if (!parent) files.push(row);
      else levelOfPath(folders, parentPath)?.files.push(row);
    }
    const fallbackFiles = (filesByRepository[repository.id] ?? []).filter(
      (file) => !file.path.includes("/"),
    );
    files.splice(
      0,
      files.length,
      ...fallbackFiles.map((file) => ({ kind: "file" as const, file })),
    );
    return {
      kind: "repository" as const,
      id: repository.id,
      label: repository.display_name?.trim() || `${repositoryFallback} ${index + 1}`,
      folders: [...folders.values()].sort((a, b) => a.name.localeCompare(b.name)).map(freezeFolder),
      files: files.sort((a, b) => a.file.path.localeCompare(b.file.path)),
    };
  });
}

function levelOfPath(roots: Map<string, MutableFolder>, path: string): MutableFolder | undefined {
  const parts = path.split("/");
  let current = roots.get(parts[0]);
  for (const part of parts.slice(1)) current = current?.folders.get(part);
  return current;
}

export function countTurnChangeTreeFiles(folder: TurnChangeTreeFolder): number {
  return (
    folder.files.length +
    folder.folders.reduce((sum, child) => sum + countTurnChangeTreeFiles(child), 0)
  );
}

export function summarizeTurnChangeTreeFolder(folder: TurnChangeTreeFolder): {
  loadedFiles: number;
  addedLines: number;
  deletedLines: number;
  unknownCountFiles: number;
} {
  const files = [
    ...folder.files.map(({ file }) => file),
    ...folder.folders.flatMap((child) => flattenFolderFiles(child)),
  ];
  let addedLines = 0;
  let deletedLines = 0;
  let unknownCountFiles = 0;
  for (const file of files) {
    if (file.added_lines == null || file.deleted_lines == null) {
      unknownCountFiles++;
      continue;
    }
    addedLines += file.added_lines;
    deletedLines += file.deleted_lines;
  }
  return { loadedFiles: files.length, addedLines, deletedLines, unknownCountFiles };
}

export function turnChangeRepositoryOptionName(
  repositories: TurnRepositoryChange[],
  repositoryChangeId: string,
  checkoutId: string,
): string {
  const repository = repositories.find((item) => item.id === repositoryChangeId);
  const name = repository?.display_name?.trim() || repository?.checkout_id || checkoutId;
  return `${name} (${repository?.checkout_id || checkoutId})`;
}

function flattenFolderFiles(folder: TurnChangeTreeFolder): TurnFileChange[] {
  return [
    ...folder.files.map(({ file }) => file),
    ...folder.folders.flatMap((child) => flattenFolderFiles(child)),
  ];
}
