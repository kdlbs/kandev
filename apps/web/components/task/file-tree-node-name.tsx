import { useCallback, useEffect, useRef, useState } from "react";
import { Input } from "@kandev/ui/input";
import { cn } from "@/lib/utils";
import type { FileInfo } from "@/lib/state/store";
import type { FileTreeNode } from "@/lib/types/backend";
import type { useFileRename } from "./file-context-menu";

type GitFileStatus = FileInfo["status"] | undefined;

/** Inline rename input or static file name */
export function TreeNodeName({
  node,
  displayName,
  isActive,
  gitStatus,
  rename,
}: {
  node: FileTreeNode;
  displayName?: string;
  isActive: boolean;
  gitStatus: GitFileStatus;
  rename: ReturnType<typeof useFileRename>;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const blurEnabledRef = useRef(false);
  const [blurReadySequence, setBlurReadySequence] = useState<number | null>(null);

  useEffect(() => {
    if (rename.isRenaming) {
      blurEnabledRef.current = false;
      inputRef.current?.focus();
      inputRef.current?.select();
      const sequence = rename.renameSequence;
      const blurTimer = setTimeout(() => {
        blurEnabledRef.current = true;
        setBlurReadySequence(sequence);
      }, 400);
      return () => {
        clearTimeout(blurTimer);
      };
    }
  }, [rename.isRenaming, rename.renameSequence]);

  const handleBlur = useCallback(() => {
    if (blurEnabledRef.current) {
      rename.handleConfirmRename();
    }
  }, [rename]);

  if (rename.isRenaming) {
    return (
      <Input
        ref={inputRef}
        controlSize="none"
        value={rename.renameValue}
        data-blur-commit-ready={blurReadySequence === rename.renameSequence ? "true" : "false"}
        onChange={(e) => rename.setRenameValue(e.target.value)}
        onKeyDown={rename.handleRenameKeyDown}
        onBlur={handleBlur}
        onClick={(e) => e.stopPropagation()}
        className="h-5 text-xs px-1 py-0 flex-1 min-w-0"
      />
    );
  }
  return (
    <span
      className={cn(
        "min-w-0 flex-1 truncate group-hover:text-foreground",
        isActive ? "text-foreground" : "text-muted-foreground",
        node.is_dir ? "font-medium" : getGitStatusTextClass(gitStatus),
      )}
    >
      {displayName ?? node.name}
    </span>
  );
}

export function getGitStatusTextClass(status: GitFileStatus): string {
  switch (status) {
    case "added":
    case "untracked":
      return "text-green-700 dark:text-green-600";
    case "modified":
      return "text-yellow-600";
    default:
      return "";
  }
}
