"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconRefresh, IconSearch } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { listSentryProjects, searchSentryIssues } from "@/lib/api/domains/sentry-api";
import type {
  SentryIssue,
  SentryLevel,
  SentryProject,
  SentrySearchFilter,
  SentryStatus,
} from "@/lib/types/sentry";
import {
  SentryErrorMessage,
  SentryIssueRow,
  levelBadgeClass,
  statusBadgeClass,
} from "./sentry-issue-common";

const LEVELS: SentryLevel[] = ["fatal", "error", "warning", "info", "debug"];
const STATUSES: SentryStatus[] = ["unresolved", "resolved", "ignored"];
const PERIODS = ["1h", "24h", "7d", "14d", "30d"] as const;
type Period = (typeof PERIODS)[number];

type FilterState = {
  orgSlug: string;
  projectSlug: string;
  environment: string;
  query: string;
  statsPeriod: Period;
  levels: SentryLevel[];
  statuses: SentryStatus[];
};

const initialFilter: FilterState = {
  orgSlug: "",
  projectSlug: "",
  environment: "",
  query: "",
  statsPeriod: "24h",
  levels: [],
  statuses: ["unresolved"],
};

type SentryIssueDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId?: string;
};

export function SentryIssueDialog({ open, onOpenChange, workspaceId }: SentryIssueDialogProps) {
  const dialog = useDialogState(open, workspaceId);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="!max-w-[min(1080px,95vw)] w-[95vw] max-h-[90vh] overflow-hidden flex flex-col gap-0 p-0 sm:rounded-lg">
        <DialogTitle className="sr-only">Browse Sentry issues</DialogTitle>
        <FiltersBar state={dialog} />
        <ResultsBody state={dialog} />
      </DialogContent>
    </Dialog>
  );
}

type DialogState = ReturnType<typeof useDialogState>;

function useDialogState(open: boolean, workspaceId?: string) {
  const [filter, setFilter] = useState<FilterState>(initialFilter);
  const [projects, setProjects] = useState<SentryProject[]>([]);
  const [issues, setIssues] = useState<SentryIssue[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [isLast, setIsLast] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [configLoaded, setConfigLoaded] = useState(false);
  // Monotonic request id: only the most recent search may write results, so an
  // out-of-order response from a superseded search can't clobber the list.
  const searchSeq = useRef(0);

  useBrowseProjects({
    open,
    workspaceId,
    loaded: configLoaded,
    setFilter,
    setLoaded: setConfigLoaded,
    setProjects,
  });

  const updateFilter = useCallback(
    <K extends keyof FilterState>(key: K, value: FilterState[K]) =>
      setFilter((prev) => ({ ...prev, [key]: value })),
    [],
  );

  const search = useCallback(
    async (nextCursorValue?: string) => {
      if (!filter.orgSlug) {
        setError("Organization slug is required");
        return;
      }
      const seq = ++searchSeq.current;
      setLoading(true);
      setError(null);
      try {
        const payload = toSearchFilter(filter);
        const res = await searchSentryIssues(
          payload,
          nextCursorValue,
          workspaceId ? { workspaceId } : undefined,
        );
        // A newer search started while this one was in flight — drop the result.
        if (seq !== searchSeq.current) return;
        const page = res.issues ?? [];
        // nextCursorValue set => "Load more": append. Empty => fresh search: replace.
        setIssues((prev) => (nextCursorValue ? [...prev, ...page] : page));
        setNextCursor(res.nextPageToken);
        setIsLast(res.isLast);
      } catch (err) {
        if (seq !== searchSeq.current) return;
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (seq === searchSeq.current) setLoading(false);
      }
    },
    [workspaceId, filter],
  );

  return {
    filter,
    updateFilter,
    projects,
    issues,
    nextCursor,
    isLast,
    loading,
    error,
    search,
  };
}

function toSearchFilter(filter: FilterState): SentrySearchFilter {
  return {
    orgSlug: filter.orgSlug,
    projectSlug: filter.projectSlug || undefined,
    environment: filter.environment || undefined,
    query: filter.query || undefined,
    statsPeriod: filter.statsPeriod,
    levels: filter.levels.length ? filter.levels : undefined,
    statuses: filter.statuses.length ? filter.statuses : undefined,
  };
}

type BrowseProjectsArgs = {
  open: boolean;
  workspaceId: string | undefined;
  loaded: boolean;
  setFilter: (f: (prev: FilterState) => FilterState) => void;
  setLoaded: (v: boolean) => void;
  setProjects: (p: SentryProject[]) => void;
};

function useBrowseProjects({
  open,
  workspaceId,
  loaded,
  setFilter,
  setLoaded,
  setProjects,
}: BrowseProjectsArgs) {
  useEffect(() => {
    // Reset when the dialog closes so reopening (component stays mounted)
    // refetches projects rather than showing a stale snapshot.
    if (!open) {
      setLoaded(false);
      return;
    }
    if (loaded) return;
    let cancelled = false;
    (async () => {
      try {
        const res = await listSentryProjects(workspaceId ? { workspaceId } : undefined).catch(
          () => ({
            projects: [] as SentryProject[],
          }),
        );
        if (cancelled) return;
        const projects = res.projects ?? [];
        setProjects(projects);
        // Auto-select the sole org so the required org field is pre-filled when
        // the token only sees one organization.
        const orgs = Array.from(new Set(projects.map((p) => p.orgSlug).filter(Boolean)));
        if (orgs.length === 1) {
          setFilter((prev) => (prev.orgSlug ? prev : { ...prev, orgSlug: orgs[0] }));
        }
      } finally {
        if (!cancelled) setLoaded(true);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, loaded, setFilter, setLoaded, setProjects]);
}

function FiltersBar({ state }: { state: DialogState }) {
  return (
    <div className="border-b px-5 py-4 space-y-3 shrink-0">
      <FilterTopRow state={state} />
      <FilterChipRow
        label="Level"
        options={LEVELS}
        selected={state.filter.levels}
        onToggle={(next) => state.updateFilter("levels", next as SentryLevel[])}
        chipClass={(v) => levelBadgeClass(v as SentryLevel)}
      />
      <FilterChipRow
        label="Status"
        options={STATUSES}
        selected={state.filter.statuses}
        onToggle={(next) => state.updateFilter("statuses", next as SentryStatus[])}
        chipClass={(v) => statusBadgeClass(v as SentryStatus)}
      />
      <SearchActionRow state={state} />
    </div>
  );
}

function FilterTopRow({ state }: { state: DialogState }) {
  const { filter, updateFilter, projects } = state;
  return (
    <div className="grid gap-2 sm:grid-cols-[1fr_1fr_180px_120px]">
      <LabeledInput
        id="sentry-search-org"
        label="Organization"
        value={filter.orgSlug}
        onChange={(v) => updateFilter("orgSlug", v)}
        placeholder="my-org"
      />
      <ProjectFilterSelect
        value={filter.projectSlug}
        projects={projects}
        onChange={(v) => updateFilter("projectSlug", v)}
      />
      <LabeledInput
        id="sentry-search-env"
        label="Environment"
        value={filter.environment}
        onChange={(v) => updateFilter("environment", v)}
        placeholder="production"
      />
      <PeriodSelect value={filter.statsPeriod} onChange={(v) => updateFilter("statsPeriod", v)} />
    </div>
  );
}

type LabeledInputProps = {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
};

function LabeledInput({ id, label, value, onChange, placeholder }: LabeledInputProps) {
  return (
    <div className="space-y-1">
      <Label htmlFor={id} className="text-xs text-muted-foreground">
        {label}
      </Label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-8 text-xs"
      />
    </div>
  );
}

type ProjectFilterSelectProps = {
  value: string;
  projects: SentryProject[];
  onChange: (v: string) => void;
};

function ProjectFilterSelect({ value, projects, onChange }: ProjectFilterSelectProps) {
  return (
    <div className="space-y-1">
      <Label className="text-xs text-muted-foreground">Project</Label>
      <Select value={value || "__all__"} onValueChange={(v) => onChange(v === "__all__" ? "" : v)}>
        <SelectTrigger className="h-8 text-xs">
          <SelectValue placeholder="All projects" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">All projects</SelectItem>
          {projects.map((p) => (
            <SelectItem key={p.id} value={p.slug}>
              {p.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function PeriodSelect({ value, onChange }: { value: Period; onChange: (v: Period) => void }) {
  return (
    <div className="space-y-1">
      <Label className="text-xs text-muted-foreground">Period</Label>
      <Select value={value} onValueChange={(v) => onChange(v as Period)}>
        <SelectTrigger className="h-8 text-xs">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {PERIODS.map((p) => (
            <SelectItem key={p} value={p}>
              {p}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

type ChipRowProps = {
  label: string;
  options: readonly string[];
  selected: string[];
  onToggle: (next: string[]) => void;
  chipClass: (value: string) => string;
};

function FilterChipRow({ label, options, selected, onToggle, chipClass }: ChipRowProps) {
  const toggle = (value: string) => {
    if (selected.includes(value)) onToggle(selected.filter((v) => v !== value));
    else onToggle([...selected, value]);
  };
  return (
    <div className="flex items-center gap-2 flex-wrap">
      <span className="text-xs text-muted-foreground w-20">{label}</span>
      {options.map((opt) => {
        const active = selected.includes(opt);
        return (
          <Badge
            key={opt}
            variant="outline"
            onClick={() => toggle(opt)}
            className={`cursor-pointer text-[10px] uppercase px-2 py-0.5 ${
              active ? chipClass(opt) : "opacity-60"
            }`}
          >
            {opt}
          </Badge>
        );
      })}
    </div>
  );
}

function SearchActionRow({ state }: { state: DialogState }) {
  const { filter, updateFilter, loading, search } = state;
  return (
    <div className="flex items-end gap-2">
      <div className="flex-1 space-y-1">
        <Label htmlFor="sentry-search-query" className="text-xs text-muted-foreground">
          Query
        </Label>
        <Input
          id="sentry-search-query"
          value={filter.query}
          onChange={(e) => updateFilter("query", e.target.value)}
          placeholder="is:unresolved release:1.2.3"
          className="h-8 text-xs"
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              void search();
            }
          }}
        />
      </div>
      <Button
        type="button"
        size="sm"
        onClick={() => void search()}
        disabled={loading || !filter.orgSlug}
        className="cursor-pointer gap-1.5"
      >
        {loading ? (
          <IconRefresh className="h-4 w-4 animate-spin" />
        ) : (
          <IconSearch className="h-4 w-4" />
        )}
        Search
      </Button>
    </div>
  );
}

function ResultsBody({ state }: { state: DialogState }) {
  const { issues, loading, error, isLast, nextCursor, search } = state;
  const empty = useMemo(() => !loading && !error && issues.length === 0, [loading, error, issues]);

  return (
    <div className="flex-1 overflow-y-auto px-5 py-4 space-y-2">
      {error && <SentryErrorMessage error={error} compact />}
      {empty && (
        <div className="text-sm text-muted-foreground py-12 text-center">
          No issues yet. Run a search to begin.
        </div>
      )}
      {issues.map((issue) => (
        <a
          key={issue.id}
          href={issue.permalink}
          target="_blank"
          rel="noreferrer"
          className="block cursor-pointer hover:bg-muted/30 rounded-md"
        >
          <SentryIssueRow issue={issue} />
        </a>
      ))}
      {!isLast && nextCursor && (
        <div className="flex justify-center pt-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void search(nextCursor)}
            disabled={loading}
            className="cursor-pointer"
          >
            Load more
          </Button>
        </div>
      )}
    </div>
  );
}
