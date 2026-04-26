"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { listIssues } from "@/lib/api/domains/orchestrate-api";
import { NewIssueDialog } from "../components/new-issue-dialog";
import { IssuesToolbar } from "./issues-toolbar";
import { IssuesContent } from "./issues-content";
import { useIssuesTree } from "./use-issues-tree";

const STORAGE_KEY_PREFIX = "kandev-issues-filters-";

function loadPersistedFilters(workspaceId: string | null) {
  if (!workspaceId || typeof window === "undefined") return {};
  try {
    const raw = localStorage.getItem(`${STORAGE_KEY_PREFIX}${workspaceId}`);
    return raw ? JSON.parse(raw) : {};
  } catch {
    return {};
  }
}

function persistFilters(workspaceId: string | null, filters: Record<string, unknown>) {
  if (!workspaceId || typeof window === "undefined") return;
  try {
    localStorage.setItem(`${STORAGE_KEY_PREFIX}${workspaceId}`, JSON.stringify(filters));
  } catch {
    // ignore storage errors
  }
}

export function IssuesList() {
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const issues = useAppStore((s) => s.orchestrate.issues.items);
  const filters = useAppStore((s) => s.orchestrate.issues.filters);
  const viewMode = useAppStore((s) => s.orchestrate.issues.viewMode);
  const sortField = useAppStore((s) => s.orchestrate.issues.sortField);
  const sortDir = useAppStore((s) => s.orchestrate.issues.sortDir);
  const groupBy = useAppStore((s) => s.orchestrate.issues.groupBy);
  const nestingEnabled = useAppStore((s) => s.orchestrate.issues.nestingEnabled);
  const isLoading = useAppStore((s) => s.orchestrate.issues.isLoading);
  const agents = useAppStore((s) => s.orchestrate.agentInstances);

  const setIssues = useAppStore((s) => s.setIssues);
  const setIssueFilters = useAppStore((s) => s.setIssueFilters);
  const setIssueViewMode = useAppStore((s) => s.setIssueViewMode);
  const setIssueSortField = useAppStore((s) => s.setIssueSortField);
  const setIssueSortDir = useAppStore((s) => s.setIssueSortDir);
  const setIssueGroupBy = useAppStore((s) => s.setIssueGroupBy);
  const toggleNesting = useAppStore((s) => s.toggleNesting);
  const setIssuesLoading = useAppStore((s) => s.setIssuesLoading);

  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set());
  const [newIssueOpen, setNewIssueOpen] = useState(false);

  const agentMap = new Map(agents.map((a) => [a.id, a.name]));

  // Load persisted filters on mount
  useEffect(() => {
    const persisted = loadPersistedFilters(workspaceId);
    if (Object.keys(persisted).length > 0) {
      setIssueFilters(persisted);
    }
  }, [workspaceId, setIssueFilters]);

  // Fetch issues
  useEffect(() => {
    if (!workspaceId) return;
    let cancelled = false;
    setIssuesLoading(true);
    listIssues(workspaceId)
      .then((res) => {
        if (!cancelled) setIssues(res.issues ?? []);
      })
      .catch(() => {
        // error handled silently
      })
      .finally(() => {
        if (!cancelled) setIssuesLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, setIssues, setIssuesLoading]);

  const handleFilterChange = useCallback(
    (patch: Record<string, unknown>) => {
      setIssueFilters(patch);
      persistFilters(workspaceId, { ...filters, ...patch });
    },
    [setIssueFilters, filters, workspaceId],
  );

  const handleSearchChange = useCallback(
    (search: string) => {
      setIssueFilters({ search });
    },
    [setIssueFilters],
  );

  const handleToggleExpand = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  const flatNodes = useIssuesTree({
    issues, filters, sortField, sortDir, nestingEnabled, expandedIds,
  });

  return (
    <div className="space-y-4 p-6">
      <IssuesToolbar
        viewMode={viewMode}
        nestingEnabled={nestingEnabled}
        filters={filters}
        sortField={sortField}
        sortDir={sortDir}
        groupBy={groupBy}
        onViewModeChange={setIssueViewMode}
        onToggleNesting={toggleNesting}
        onFilterChange={handleFilterChange}
        onSortFieldChange={setIssueSortField}
        onSortDirChange={setIssueSortDir}
        onGroupByChange={setIssueGroupBy}
        onSearchChange={handleSearchChange}
        onNewIssue={() => setNewIssueOpen(true)}
      />

      <IssuesContent
        viewMode={viewMode}
        isLoading={isLoading}
        flatNodes={flatNodes}
        expandedIds={expandedIds}
        onToggleExpand={handleToggleExpand}
        agentMap={agentMap}
      />

      <NewIssueDialog open={newIssueOpen} onOpenChange={setNewIssueOpen} />
    </div>
  );
}
