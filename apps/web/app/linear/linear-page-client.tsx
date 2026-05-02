"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { IconExternalLink, IconHexagon, IconPlus, IconSearch } from "@tabler/icons-react";
import { Alert, AlertDescription } from "@kandev/ui/alert";
import { Avatar, AvatarFallback, AvatarImage } from "@kandev/ui/avatar";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from "@kandev/ui/pagination";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { PageTopbar } from "@/components/page-topbar";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import {
  formatRelative,
  LinearErrorMessage,
  stateBadgeClass,
} from "@/components/linear/linear-issue-common";
import { LinearIssueDialog } from "@/components/linear/linear-issue-dialog";
import { LinearQuickTaskLauncher } from "@/components/linear/linear-quick-task-launcher";
import { getLinearConfig, listLinearTeams, searchLinearIssues } from "@/lib/api/domains/linear-api";
import type { LinearIssue, LinearTeam } from "@/lib/types/linear";
import type { Workflow, WorkflowStep } from "@/lib/types/http";

const PAGE_SIZE = 25;

type LinearPageClientProps = {
  workspaceId?: string;
  workflows: Workflow[];
  steps: WorkflowStep[];
};

function NotConfiguredNotice({ workspaceId }: { workspaceId?: string }) {
  return (
    <div className="p-6 max-w-2xl">
      <Alert>
        <AlertDescription>
          Linear is not configured for this workspace.{" "}
          <Link
            href={workspaceId ? `/settings/workspace/${workspaceId}/linear` : "/settings"}
            className="underline font-medium cursor-pointer"
          >
            Configure Linear
          </Link>{" "}
          to see your issues here.
        </AlertDescription>
      </Alert>
    </div>
  );
}

function useLinearPageData(workspaceId?: string) {
  const [loaded, setLoaded] = useState(false);
  const [configured, setConfigured] = useState(false);
  const [teams, setTeams] = useState<LinearTeam[]>([]);

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!workspaceId) {
        setLoaded(true);
        return;
      }
      try {
        const cfg = await getLinearConfig(workspaceId);
        if (cancelled) return;
        const ok = !!cfg && cfg.hasSecret;
        setConfigured(ok);
        if (ok) {
          try {
            const list = await listLinearTeams(workspaceId);
            if (!cancelled) setTeams(list.teams ?? []);
          } catch {
            // Non-fatal: team filter just stays empty.
          }
        }
      } finally {
        if (!cancelled) setLoaded(true);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return { loaded, configured, teams };
}

type SearchState = {
  items: LinearIssue[];
  loading: boolean;
  error: string | null;
  page: number;
  pageSize: number;
  isLast: boolean;
  goNext: () => void;
  goPrev: () => void;
};

// useIssueSearch is page-based: it caches each page's cursor in `tokensRef` so
// the user can step backward without re-querying from the first page. Linear
// returns no total count, so the UI shows a row range plus a Page N indicator.
function useIssueSearch(
  workspaceId: string | undefined,
  query: string,
  teamKey: string,
  assigned: string,
): SearchState {
  const [items, setItems] = useState<LinearIssue[]>([]);
  const [page, setPage] = useState(1);
  const [isLast, setIsLast] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // tokens[i] is the page_token for page i+1; tokens[0] is always "".
  const tokensRef = useRef<string[]>([""]);
  const reqRef = useRef(0);

  const fetchPage = useCallback(
    (p: number) =>
      searchLinearIssues(workspaceId!, {
        query: query || undefined,
        teamKey: teamKey || undefined,
        assigned: assigned || undefined,
        pageToken: tokensRef.current[p - 1] || undefined,
        maxResults: PAGE_SIZE,
      }),
    [workspaceId, query, teamKey, assigned],
  );

  const run = useCallback(
    async (p: number) => {
      if (!workspaceId) return;
      const reqId = ++reqRef.current;
      setLoading(true);
      setError(null);
      try {
        const res = await fetchPage(p);
        if (reqId !== reqRef.current) return;
        setItems(res.issues ?? []);
        setIsLast(res.isLast ?? true);
        if (!res.isLast && res.nextPageToken) {
          tokensRef.current[p] = res.nextPageToken;
        }
      } catch (err) {
        if (reqId !== reqRef.current) return;
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (reqId === reqRef.current) setLoading(false);
      }
    },
    [workspaceId, fetchPage],
  );

  // Reset cursor stack and snap back to page 1 when filters change.
  useEffect(() => {
    tokensRef.current = [""];
    setPage(1);
  }, [workspaceId, query, teamKey, assigned]);

  // 250ms debounce keeps the keyboard input from firing a request per char.
  useEffect(() => {
    if (!workspaceId) return;
    const id = setTimeout(() => void run(page), 250);
    return () => clearTimeout(id);
  }, [run, page, workspaceId]);

  return {
    items,
    loading,
    error,
    page,
    pageSize: PAGE_SIZE,
    isLast,
    goNext: () => {
      if (!isLast) setPage((p) => p + 1);
    },
    goPrev: () => {
      setPage((p) => Math.max(1, p - 1));
    },
  };
}

function AssigneeCell({ issue }: { issue: LinearIssue }) {
  if (!issue.assigneeName) {
    return <span className="text-xs text-muted-foreground">Unassigned</span>;
  }
  return (
    <div className="flex items-center gap-1.5 min-w-0">
      <Avatar size="sm" className="size-5">
        {issue.assigneeIcon && <AvatarImage src={issue.assigneeIcon} alt={issue.assigneeName} />}
        <AvatarFallback className="text-[10px]">{issue.assigneeName.charAt(0)}</AvatarFallback>
      </Avatar>
      <span className="text-xs text-muted-foreground truncate">{issue.assigneeName}</span>
    </div>
  );
}

function IssueRow({
  issue,
  onOpen,
  onStartTask,
}: {
  issue: LinearIssue;
  onOpen: (i: LinearIssue) => void;
  onStartTask: (i: LinearIssue) => void;
}) {
  const relative = formatRelative(issue.updated);
  return (
    <div className="flex items-start gap-3 py-3 border-b last:border-b-0">
      <button
        type="button"
        onClick={() => onOpen(issue)}
        className="flex-1 min-w-0 space-y-1 text-left cursor-pointer rounded -mx-2 px-2 py-1 hover:bg-muted/50 transition-colors"
        title="Open issue details"
      >
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="font-mono">{issue.identifier}</span>
          {issue.priorityLabel && <span>· {issue.priorityLabel}</span>}
        </div>
        <div className="text-sm font-medium truncate" title={issue.title}>
          {issue.title}
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          {issue.stateName && (
            <Badge variant="outline" className={stateBadgeClass(issue.stateCategory)}>
              {issue.stateName}
            </Badge>
          )}
          <AssigneeCell issue={issue} />
          {relative && <span className="text-xs text-muted-foreground">· updated {relative}</span>}
        </div>
      </button>
      <div className="flex items-center gap-1 shrink-0">
        <Button asChild variant="ghost" size="icon-sm" className="cursor-pointer">
          <a href={issue.url} target="_blank" rel="noreferrer" title="Open in Linear">
            <IconExternalLink className="h-3.5 w-3.5" />
          </a>
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="cursor-pointer h-7 px-2 gap-1 text-xs"
          onClick={() => onStartTask(issue)}
        >
          <IconPlus className="h-3.5 w-3.5" />
          Start task
        </Button>
      </div>
    </div>
  );
}

function FilterControls({
  query,
  setQuery,
  teamKey,
  setTeamKey,
  assigned,
  setAssigned,
  teams,
}: {
  query: string;
  setQuery: (v: string) => void;
  teamKey: string;
  setTeamKey: (v: string) => void;
  assigned: string;
  setAssigned: (v: string) => void;
  teams: LinearTeam[];
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 px-6 py-3 border-b">
      <div className="relative flex-1 min-w-[280px]">
        <IconSearch className="absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
        <Input
          placeholder="Search title or description"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          className="pl-8"
        />
      </div>
      <Select
        value={teamKey || "__all__"}
        onValueChange={(v) => setTeamKey(v === "__all__" ? "" : v)}
      >
        <SelectTrigger className="w-44">
          <SelectValue placeholder="All teams" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__all__">All teams</SelectItem>
          {teams.map((t) => (
            <SelectItem key={t.id} value={t.key}>
              {t.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <Select
        value={assigned || "__any__"}
        onValueChange={(v) => setAssigned(v === "__any__" ? "" : v)}
      >
        <SelectTrigger className="w-40">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="__any__">Any assignee</SelectItem>
          <SelectItem value="me">Assigned to me</SelectItem>
          <SelectItem value="unassigned">Unassigned</SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}

function PaginationBar({
  page,
  pageSize,
  itemCount,
  isLast,
  onNext,
  onPrev,
}: {
  page: number;
  pageSize: number;
  itemCount: number;
  isLast: boolean;
  onNext: () => void;
  onPrev: () => void;
}) {
  if (page === 1 && isLast) return null;
  const start = (page - 1) * pageSize + 1;
  const end = (page - 1) * pageSize + itemCount;
  const prevDisabled = page <= 1;
  const nextDisabled = isLast;
  return (
    <div className="flex items-center justify-between px-6 py-3 border-t shrink-0">
      <div className="text-xs text-muted-foreground tabular-nums">
        {itemCount === 0 ? "No results" : `${start}–${end}`}
      </div>
      <Pagination className="mx-0 w-auto justify-end">
        <PaginationContent>
          <PaginationItem>
            <PaginationPrevious
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!prevDisabled) onPrev();
              }}
              aria-disabled={prevDisabled}
              className={prevDisabled ? "pointer-events-none opacity-50" : "cursor-pointer"}
            />
          </PaginationItem>
          <PaginationItem>
            <span className="px-3 text-sm tabular-nums">Page {page}</span>
          </PaginationItem>
          <PaginationItem>
            <PaginationNext
              href="#"
              onClick={(e) => {
                e.preventDefault();
                if (!nextDisabled) onNext();
              }}
              aria-disabled={nextDisabled}
              className={nextDisabled ? "pointer-events-none opacity-50" : "cursor-pointer"}
            />
          </PaginationItem>
        </PaginationContent>
      </Pagination>
    </div>
  );
}

function PageShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col h-full">
      <PageTopbar title="Linear" icon={<IconHexagon className="h-4 w-4" />} />
      {children}
    </div>
  );
}

function DisabledNotice({ workspaceId }: { workspaceId: string }) {
  return (
    <div className="p-6 max-w-2xl">
      <Alert>
        <AlertDescription>
          Linear integration is disabled for this workspace.{" "}
          <Link
            href={`/settings/workspace/${workspaceId}/linear`}
            className="underline font-medium cursor-pointer"
          >
            Re-enable it in settings
          </Link>
          .
        </AlertDescription>
      </Alert>
    </div>
  );
}

function ResultsArea({
  search,
  empty,
  workspaceId,
  onOpen,
  onStartTask,
}: {
  search: SearchState;
  empty: boolean;
  workspaceId: string;
  onOpen: (issue: LinearIssue) => void;
  onStartTask: (issue: LinearIssue) => void;
}) {
  return (
    <div className="flex-1 overflow-y-auto px-6 py-3">
      {search.error && !search.loading && (
        <div className="py-8 flex justify-center">
          <LinearErrorMessage error={search.error} workspaceId={workspaceId} />
        </div>
      )}
      {!search.error && empty && (
        <div className="text-sm text-muted-foreground py-12 text-center">
          No issues match your filters.
        </div>
      )}
      {search.items.map((issue) => (
        <IssueRow key={issue.id} issue={issue} onOpen={onOpen} onStartTask={onStartTask} />
      ))}
      {search.loading && search.items.length === 0 && (
        <div className="text-sm text-muted-foreground py-12 text-center">Loading issues…</div>
      )}
    </div>
  );
}

export function LinearPageClient({ workspaceId, workflows, steps }: LinearPageClientProps) {
  const available = useLinearAvailable(workspaceId);
  const { loaded, configured, teams } = useLinearPageData(workspaceId);

  const [query, setQuery] = useState("");
  const [teamKey, setTeamKey] = useState("");
  const [assigned, setAssigned] = useState("me");
  const search = useIssueSearch(workspaceId, query, teamKey, assigned);

  const [openIssue, setOpenIssue] = useState<LinearIssue | null>(null);
  const [launchIssue, setLaunchIssue] = useState<LinearIssue | null>(null);

  const empty = useMemo(
    () => loaded && configured && !search.loading && search.items.length === 0,
    [loaded, configured, search.loading, search.items.length],
  );

  if (!workspaceId) {
    return (
      <PageShell>
        <NotConfiguredNotice />
      </PageShell>
    );
  }
  if (loaded && !configured) {
    return (
      <PageShell>
        <NotConfiguredNotice workspaceId={workspaceId} />
      </PageShell>
    );
  }
  if (!available && loaded && configured) {
    return (
      <PageShell>
        <DisabledNotice workspaceId={workspaceId} />
      </PageShell>
    );
  }

  return (
    <PageShell>
      <FilterControls
        query={query}
        setQuery={setQuery}
        teamKey={teamKey}
        setTeamKey={setTeamKey}
        assigned={assigned}
        setAssigned={setAssigned}
        teams={teams}
      />
      <ResultsArea
        search={search}
        empty={empty}
        workspaceId={workspaceId}
        onOpen={setOpenIssue}
        onStartTask={setLaunchIssue}
      />
      <PaginationBar
        page={search.page}
        pageSize={search.pageSize}
        itemCount={search.items.length}
        isLast={search.isLast}
        onNext={search.goNext}
        onPrev={search.goPrev}
      />
      <LinearIssueDialog
        open={openIssue !== null}
        onOpenChange={(open) => !open && setOpenIssue(null)}
        workspaceId={workspaceId}
        identifier={openIssue?.identifier ?? null}
        initialIssue={openIssue}
        onStartTask={setLaunchIssue}
      />
      <LinearQuickTaskLauncher
        workspaceId={workspaceId}
        workflows={workflows}
        steps={steps}
        issue={launchIssue}
        onClose={() => setLaunchIssue(null)}
      />
    </PageShell>
  );
}
