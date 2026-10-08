import type { TFunction } from "i18next";
import type { useToast } from "@/components/toast-provider";
import type { FileTreeNode } from "@/lib/types/backend";
import type { useFileBrowserTree } from "./file-browser-hooks";
import {
  computeMoveTargets,
  findNodeByPath,
  insertNodeInTree,
  removeNodeFromTree,
  renameNodeInTree,
} from "./file-tree-utils";

type MoveFilesParams = {
  sources: string[];
  targetPath: string;
  treeState: ReturnType<typeof useFileBrowserTree>;
  setSelectedPaths: (paths: Set<string>) => void;
  onRenameFile: (oldPath: string, newPath: string) => Promise<boolean>;
};
type MoveTarget = { oldPath: string; newPath: string; node: FileTreeNode | null };

function retainUnaffectedNodes(
  previous: FileTreeNode,
  next: FileTreeNode,
  paths: string[],
): FileTreeNode {
  const prefix = previous.path ? `${previous.path}/` : "";
  if (!paths.some((path) => path.startsWith(prefix))) return previous;
  if (!next.children) return next;
  const previousChildren = new Map(previous.children?.map((child) => [child.path, child]));
  return {
    ...next,
    children: next.children.map((child) => {
      const existing = previousChildren.get(child.path);
      return existing ? retainUnaffectedNodes(existing, child, paths) : child;
    }),
  };
}

function publishAcceptedMove(tree: FileTreeNode, target: MoveTarget): FileTreeNode {
  const { oldPath, newPath, node } = target;
  const slash = newPath.lastIndexOf("/");
  const parent = slash < 0 ? "" : newPath.slice(0, slash);
  if (
    !node ||
    findNodeByPath(tree, oldPath) !== node ||
    findNodeByPath(tree, newPath) ||
    !findNodeByPath(tree, parent)?.is_dir
  )
    return tree;
  const next = insertNodeInTree(
    removeNodeFromTree(tree, oldPath),
    parent,
    renameNodeInTree(node, oldPath, newPath),
  );
  // Sibling settlement preserves identities outside the edited ancestry.
  return retainUnaffectedNodes(tree, next, [oldPath, newPath]);
}

export function executeMoveFiles(
  params: MoveFilesParams,
  toast: ReturnType<typeof useToast>["toast"],
  t: TFunction,
) {
  const { sources, targetPath, treeState, setSelectedPaths, onRenameFile } = params;
  const tree = treeState.tree;
  if (!tree || !treeState.isCurrentTree()) return;
  const targets = computeMoveTargets(tree, sources, targetPath).map((target) => ({
    ...target,
    node: findNodeByPath(tree, target.oldPath),
  }));
  setSelectedPaths(new Set());
  if (targets.length === 0) return;
  void Promise.all(
    targets.map(async (target) => {
      try {
        const ok = await onRenameFile(target.oldPath, target.newPath);
        if (ok && treeState.isCurrentTree()) {
          treeState.invalidateChanges([{ path: target.oldPath }, { path: target.newPath }]);
          treeState.setTree((current) =>
            current && treeState.isCurrentTree() ? publishAcceptedMove(current, target) : current,
          );
        }
        return { ok, unexpected: false };
      } catch {
        return { ok: false, unexpected: true };
      }
    }),
  ).then((results) => {
    if (!treeState.isCurrentTree()) return;
    const paths = new Set(targets.flatMap(({ oldPath, newPath }) => [oldPath, newPath]));
    treeState.refreshChanges(Array.from(paths, (path) => ({ path })));
    if (results.some(({ ok }) => !ok)) {
      toast({
        title: t("task:moveFailed"),
        description: t(
          results.some(({ unexpected }) => unexpected)
            ? "task:moveFailedUnexpected"
            : "task:moveFailedFiles",
        ),
        variant: "error",
      });
    }
  });
}
