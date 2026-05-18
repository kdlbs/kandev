"use client";

import { memo, useMemo, useState, useEffect, useRef, useCallback } from "react";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Button } from "@kandev/ui/button";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { TaskChangesPanel } from "../task-changes-panel";
import { CommitDiffView } from "../commit-detail-panel";
import type { ReviewSource, SourceCounts } from "@/hooks/domains/session/use-review-sources";
import type { SelectedDiff } from "../task-layout";
import type { DiffSheetMode } from "../changes-diff-target";

const MOBILE_DIFF_SOURCE_FILTER_KEY = "mobile-diff-source-filter";

type MobileDiffSheetProps = {
  mode: DiffSheetMode | null;
  onClose: () => void;
  onOpenFile?: (filePath: string) => void;
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  sourceCounts: SourceCounts;
};

function pickFirstNonEmpty(sourceCounts: SourceCounts): ReviewSource | null {
  if (sourceCounts.uncommitted > 0) return "uncommitted";
  if (sourceCounts.committed > 0) return "committed";
  if (sourceCounts.pr > 0) return "pr";
  return null;
}

function useAutoSelectSource(
  modeKind: string | undefined,
  sourceCounts: SourceCounts,
  activeSource: ReviewSource,
  setActiveSource: (s: ReviewSource) => void,
) {
  const userPickedRef = useRef(false);
  const prevModeKindRef = useRef<string | undefined>(undefined);

  // Reset the "user picked" flag whenever the sheet closes/reopens.
  useEffect(() => {
    if (modeKind !== "all" && prevModeKindRef.current === "all") {
      userPickedRef.current = false;
    }
    prevModeKindRef.current = modeKind;
  }, [modeKind]);

  // Auto-select highest-priority non-empty source when the active source is
  // empty. Stops once the user makes an explicit choice. Does not override a
  // non-empty active source — so switching tabs then seeing uncommitted files
  // arrive does not forcibly reset the user's view.
  useEffect(() => {
    if (modeKind !== "all") return;
    if (userPickedRef.current) return;
    const pick = pickFirstNonEmpty(sourceCounts);
    if (pick === null) return;
    if (sourceCounts[activeSource] > 0) return;
    if (pick === activeSource) return;
    setActiveSource(pick);
  }, [modeKind, sourceCounts, activeSource, setActiveSource]);

  const handleUserPick = useCallback(
    (s: ReviewSource) => {
      userPickedRef.current = true;
      setActiveSource(s);
    },
    [setActiveSource],
  );

  return { handleUserPick };
}

type SourceTab = { key: ReviewSource; label: string; count: number };

function buildSourceTabs(sourceCounts: SourceCounts): SourceTab[] {
  return [
    { key: "uncommitted" as ReviewSource, label: "Uncommitted", count: sourceCounts.uncommitted },
    { key: "committed" as ReviewSource, label: "Committed", count: sourceCounts.committed },
    { key: "pr" as ReviewSource, label: "PR", count: sourceCounts.pr },
  ].filter((t) => t.count > 0);
}

function deriveTitle(mode: DiffSheetMode | null, sourceTabs: SourceTab[]): string {
  if (!mode) return "";
  if (mode.kind === "all") {
    if (sourceTabs.length === 1) return `${sourceTabs[0].label} Changes`;
    return "All Changes";
  }
  if (mode.kind === "file") return "File Changes";
  if (mode.kind === "commit") return "Commit Changes";
  return "";
}

function SheetHeader({
  title,
  showSourceChip,
  sourceLabel,
  onClose,
}: {
  title: string;
  showSourceChip: boolean;
  sourceLabel: string | null;
  onClose: () => void;
}) {
  return (
    <DrawerHeader className="flex items-center justify-between py-2 px-4 border-b shrink-0">
      <DrawerTitle className="text-base flex items-center gap-2">
        <span>{title}</span>
        {showSourceChip && sourceLabel && (
          <span
            className="text-[10px] font-medium uppercase tracking-wide text-muted-foreground bg-muted/60 rounded-sm px-1.5 py-0.5"
            data-testid="mobile-diff-sheet-source-chip"
          >
            {sourceLabel}
          </span>
        )}
      </DrawerTitle>
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
  );
}

function SourceTabBar({
  tabs,
  activeSource,
  onPick,
}: {
  tabs: SourceTab[];
  activeSource: ReviewSource;
  onPick: (s: ReviewSource) => void;
}) {
  return (
    <div className="px-4 py-2 border-b shrink-0">
      <Tabs value={activeSource} onValueChange={(v) => onPick(v as ReviewSource)}>
        <TabsList className="h-8">
          {tabs.map((tab) => (
            <TabsTrigger key={tab.key} value={tab.key} className="text-xs px-2">
              {tab.label} ({tab.count})
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>
    </div>
  );
}

function renderPanel(
  mode: DiffSheetMode | null,
  activeSource: ReviewSource,
  selectedDiff: SelectedDiff | null,
  onClearSelected: () => void,
  onOpenFile?: (filePath: string) => void,
): React.ReactNode {
  if (!mode) return null;
  if (mode.kind === "commit") {
    return <CommitDiffView sha={mode.sha} repo={mode.repo} onOpenFile={onOpenFile} wordWrap />;
  }
  const panelMode = mode.kind;
  const filePath = mode.kind === "file" ? mode.path : undefined;
  const fileRepositoryName = mode.kind === "file" ? mode.repositoryName : undefined;
  const effectiveSourceFilter = mode.kind === "all" ? activeSource : (mode.sourceFilter ?? "all");
  return (
    <TaskChangesPanel
      mode={panelMode}
      filePath={filePath}
      fileRepositoryName={fileRepositoryName}
      selectedDiff={selectedDiff}
      onClearSelected={onClearSelected}
      onOpenFile={onOpenFile}
      sourceFilter={effectiveSourceFilter}
      wordWrap
    />
  );
}

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
  sourceCounts,
}: MobileDiffSheetProps) {
  const [activeSource, setActiveSource] = useState<ReviewSource>(() => {
    if (typeof window === "undefined") return "uncommitted";
    const saved = localStorage.getItem(MOBILE_DIFF_SOURCE_FILTER_KEY);
    if (saved === "uncommitted" || saved === "committed" || saved === "pr") return saved;
    return "uncommitted";
  });
  const { handleUserPick } = useAutoSelectSource(
    mode?.kind,
    sourceCounts,
    activeSource,
    setActiveSource,
  );
  useEffect(() => {
    if (mode?.kind !== "all") return;
    localStorage.setItem(MOBILE_DIFF_SOURCE_FILTER_KEY, activeSource);
  }, [mode?.kind, activeSource]);
  const sourceTabs = useMemo(() => buildSourceTabs(sourceCounts), [sourceCounts]);
  const activeSourceLabel = useMemo(
    () => sourceTabs.find((t) => t.key === activeSource)?.label ?? null,
    [sourceTabs, activeSource],
  );
  const title = useMemo(() => deriveTitle(mode, sourceTabs), [mode, sourceTabs]);
  const panelContent = useMemo(
    () => renderPanel(mode, activeSource, selectedDiff, onClearSelected, onOpenFile),
    [mode, activeSource, selectedDiff, onClearSelected, onOpenFile],
  );

  return (
    <Drawer open={mode !== null} onOpenChange={(open) => (!open ? onClose() : undefined)}>
      <DrawerContent className="h-full max-h-screen flex flex-col rounded-none">
        <SheetHeader
          title={title}
          showSourceChip={mode?.kind === "all" && sourceTabs.length === 1}
          sourceLabel={activeSourceLabel}
          onClose={onClose}
        />
        {mode?.kind === "all" && sourceTabs.length > 1 && (
          <SourceTabBar tabs={sourceTabs} activeSource={activeSource} onPick={handleUserPick} />
        )}
        <div className="flex-1 min-h-0 overflow-y-auto" data-vaul-no-drag>
          {panelContent}
        </div>
      </DrawerContent>
    </Drawer>
  );
});
