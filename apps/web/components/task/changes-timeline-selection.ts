import type { ChangedFile } from "./changes-panel-helpers";

export type ChangesFileSection = "unstaged" | "staged";

export type ChangedFileTarget = Pick<ChangedFile, "path" | "repositoryName" | "changeLayer">;

const SELECTION_KEY_PREFIX = "changes-file";

export function changesFileSelectionKey(section: ChangesFileSection, file: ChangedFile): string {
  return JSON.stringify([
    SELECTION_KEY_PREFIX,
    section,
    file.repositoryName ?? "",
    file.changeLayer ?? "",
    file.path,
  ]);
}

export function changedFileTargetFromSelectionKey(key: string): ChangedFileTarget | null {
  try {
    const value: unknown = JSON.parse(key);
    if (
      !Array.isArray(value) ||
      value.length !== 5 ||
      value[0] !== SELECTION_KEY_PREFIX ||
      (value[1] !== "unstaged" && value[1] !== "staged") ||
      typeof value[2] !== "string" ||
      typeof value[3] !== "string" ||
      typeof value[4] !== "string"
    ) {
      return null;
    }

    return {
      path: value[4],
      ...(value[2] ? { repositoryName: value[2] } : {}),
      ...(value[3] ? { changeLayer: value[3] as ChangedFile["changeLayer"] } : {}),
    };
  } catch {
    return null;
  }
}

export function changedFileTargetsFromSelectionKeys(keys: Iterable<string>): ChangedFileTarget[] {
  const targets: ChangedFileTarget[] = [];
  for (const key of keys) {
    const target = changedFileTargetFromSelectionKey(key);
    if (target) targets.push(target);
  }
  return targets;
}

export function groupChangedFileTargetsByRepository(
  targets: ChangedFileTarget[],
): Array<{ repositoryName?: string; paths: string[] }> {
  const pathsByRepository = new Map<string, { paths: string[]; seen: Set<string> }>();
  for (const target of targets) {
    const repositoryName = target.repositoryName ?? "";
    let group = pathsByRepository.get(repositoryName);
    if (!group) {
      group = { paths: [], seen: new Set() };
      pathsByRepository.set(repositoryName, group);
    }
    if (group.seen.has(target.path)) continue;
    group.seen.add(target.path);
    group.paths.push(target.path);
  }
  return [...pathsByRepository].map(([repositoryName, { paths }]) => ({
    ...(repositoryName ? { repositoryName } : {}),
    paths,
  }));
}
