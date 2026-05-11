"use client";

import { memo, useMemo, useState, useEffect, useRef } from "react";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { TaskChangesPanel } from "../task-changes-panel";
import { CommitDiffView } from "../commit-detail-panel";
import { useAppStore } from "@/components/state-provider";
import { useReviewSources } from "@/hooks/domains/session/use-review-sources";
import type { ReviewSource } from "@/hooks/domains/session/use-review-sources";
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
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const { sourceCounts } = useReviewSources(activeSessionId);

  const [activeSource, setActiveSource] = useState<ReviewSource>("uncommitted");
  const prevModeKindRef = useRef<string | undefined>(undefined);

  // Auto-select the first non-empty source when the all-mode sheet opens.
  // Use requestAnimationFrame to defer setState out of the effect body.
  useEffect(() => {
    if (mode?.kind !== "all" || prevModeKindRef.current === "all") {
      prevModeKindRef.current = mode?.kind;
      return;
    }
    prevModeKindRef.current = "all";
    requestAnimationFrame(() => {
      if (sourceCounts.uncommitted > 0) setActiveSource("uncommitted");
      else if (sourceCounts.committed > 0) setActiveSource("committed");
      else setActiveSource("pr");
    });
  }, [mode?.kind, sourceCounts.uncommitted, sourceCounts.committed]);

  const title = useMemo(() => {
    if (!mode) return "";
    if (mode.kind === "all") return "All Changes";
    if (mode.kind === "file") return "File Changes";
    if (mode.kind === "commit") return "Commit Changes";
    return "";
  }, [mode]);

  const taskChangesPanelProps = useMemo(() => {
    if (!mode || mode.kind === "all") return { mode: "all" as const };
    if (mode.kind === "file") return { mode: "file" as const, filePath: mode.path };
    return null;
  }, [mode]);

  const sourceTabs = useMemo<Array<{ key: ReviewSource; label: string; count: number }>>(
    () =>
      [
        {
          key: "uncommitted" as ReviewSource,
          label: "Uncommitted",
          count: sourceCounts.uncommitted,
        },
        { key: "committed" as ReviewSource, label: "Committed", count: sourceCounts.committed },
        { key: "pr" as ReviewSource, label: "PR", count: sourceCounts.pr },
      ].filter((t) => t.count > 0),
    [sourceCounts],
  );

  function renderPanelContent() {
    if (mode?.kind === "commit") {
      return <CommitDiffView sha={mode.sha} repo={mode.repo} onOpenFile={onOpenFile} />;
    }
    if (!taskChangesPanelProps) return null;
    return (
      <TaskChangesPanel
        mode={taskChangesPanelProps.mode}
        filePath={taskChangesPanelProps.filePath}
        selectedDiff={selectedDiff}
        onClearSelected={onClearSelected}
        onOpenFile={onOpenFile}
        sourceFilter={mode?.kind === "all" ? activeSource : "all"}
      />
    );
  }

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

        {mode?.kind === "all" && sourceTabs.length > 1 && (
          <div className="px-4 py-2 border-b shrink-0">
            <Tabs value={activeSource} onValueChange={(v) => setActiveSource(v as ReviewSource)}>
              <TabsList className="h-8">
                {sourceTabs.map((tab) => (
                  <TabsTrigger key={tab.key} value={tab.key} className="text-xs px-2">
                    {tab.label} ({tab.count})
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
          </div>
        )}

        <div className="flex-1 min-h-0 overflow-y-auto">{renderPanelContent()}</div>
      </DrawerContent>
    </Drawer>
  );
});
