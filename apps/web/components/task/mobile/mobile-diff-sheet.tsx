"use client";

import { memo, useMemo } from "react";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { TaskChangesPanel } from "../task-changes-panel";
import type { SelectedDiff } from "../task-layout";

type DiffSheetMode =
  | { kind: "all" }
  | { kind: "file"; path: string }
  | { kind: "commit"; sha: string; repo?: string };

type MobileDiffSheetProps = {
  mode: DiffSheetMode | null;
  onClose: () => void;
  onOpenFile?: (filePath: string) => void;
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
};

/**
 * Full-screen mobile diff viewer sheet. Shows either merged diffs (mode=all),
 * single-file diffs (mode=file), or commit diffs (mode=commit).
 * Uses Drawer with full-screen height to give diff viewer maximum space.
 */
export const MobileDiffSheet = memo(function MobileDiffSheet({
  mode,
  onClose,
  onOpenFile,
  selectedDiff,
  onClearSelected,
}: MobileDiffSheetProps) {
  const open = mode !== null;

  const title = useMemo(() => {
    if (!mode) return "";
    if (mode.kind === "all") return "All Changes";
    if (mode.kind === "file") return "File Changes";
    if (mode.kind === "commit") return "Commit Changes";
    return "";
  }, [mode]);

  const taskChangesPanelProps = useMemo(() => {
    if (!mode) return { mode: "all" as const };
    if (mode.kind === "all") return { mode: "all" as const };
    if (mode.kind === "file") {
      return { mode: "file" as const, filePath: mode.path };
    }
    // For commit mode, we'd need CommitDetailPanel content
    // For now, just show the single-file view
    return { mode: "file" as const, filePath: "" };
  }, [mode]);

  return (
    <Drawer open={open} onOpenChange={onClose}>
      <DrawerContent className="h-full max-h-screen flex flex-col rounded-none">
        <DrawerHeader className="flex items-center justify-between py-2 px-4 border-b shrink-0">
          <DrawerTitle className="text-base">{title}</DrawerTitle>
          <Button
            variant="ghost"
            size="sm"
            className="px-2"
            onClick={onClose}
            data-testid="mobile-diff-sheet-close"
          >
            Close
          </Button>
        </DrawerHeader>
        <div className="flex-1 min-h-0 overflow-y-auto">
          <TaskChangesPanel
            mode={taskChangesPanelProps.mode}
            filePath={taskChangesPanelProps.filePath}
            selectedDiff={selectedDiff}
            onClearSelected={onClearSelected}
            onOpenFile={onOpenFile}
            sourceFilter="all"
          />
        </div>
      </DrawerContent>
    </Drawer>
  );
});
