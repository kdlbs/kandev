"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  IconBrandGitlab,
  IconBug,
  IconCheck,
  IconClock,
  IconExternalLink,
  IconGitMerge,
  IconRefresh,
  IconX,
} from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Spinner } from "@kandev/ui/spinner";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@kandev/ui/tabs";
import { fetchGitLabStatus, searchUserIssues, searchUserMRs } from "@/lib/api/domains/gitlab-api";
import type { GitLabStatus, Issue, MR } from "@/lib/types/gitlab";

// Filter values map to GitLab's "scope" query param semantics.
const MR_FILTERS = [
  { value: "assigned_to_me", label: "Assigned" },
  { value: "created_by_me", label: "Authored" },
  { value: "review_requested", label: "Review requested" },
] as const;

const ISSUE_FILTERS = [
  { value: "assigned_to_me", label: "Assigned" },
  { value: "created_by_me", label: "Created" },
] as const;

function MRStateBadge({ mr }: { mr: MR }) {
  const state = mr.state === "opened" ? "open" : mr.state;
  if (state === "merged") {
    return (
      <Badge variant="secondary" className="gap-1 bg-purple-500/10 text-purple-500">
        <IconCheck className="h-3 w-3" /> Merged
      </Badge>
    );
  }
  if (state === "closed") {
    return (
      <Badge variant="outline" className="gap-1">
        <IconX className="h-3 w-3" /> Closed
      </Badge>
    );
  }
  if (mr.draft) {
    return (
      <Badge variant="outline" className="gap-1 text-muted-foreground">
        <IconClock className="h-3 w-3" /> Draft
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1 text-emerald-500">
      <IconGitMerge className="h-3 w-3" /> Open
    </Badge>
  );
}

function IssueStateBadge({ issue }: { issue: Issue }) {
  if (issue.state === "closed") {
    return (
      <Badge variant="outline" className="gap-1">
        <IconCheck className="h-3 w-3" /> Closed
      </Badge>
    );
  }
  return (
    <Badge variant="outline" className="gap-1 text-emerald-500">
      <IconBug className="h-3 w-3" /> Open
    </Badge>
  );
}

function NotConnectedNotice() {
  return (
    <Card>
      <CardContent className="py-10 text-center space-y-2">
        <IconBrandGitlab className="h-10 w-10 mx-auto text-muted-foreground" />
        <p className="text-sm font-medium">GitLab not connected</p>
        <p className="text-xs text-muted-foreground">
          Connect your GitLab account to browse merge requests and issues from this page.
        </p>
        <Button asChild size="sm" variant="outline" className="cursor-pointer mt-2">
          <Link href="/settings/integrations/gitlab">Open settings</Link>
        </Button>
      </CardContent>
    </Card>
  );
}

function MRList({ filter, query }: { filter: string; query: string }) {
  const [mrs, setMRs] = useState<MR[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    /* eslint-disable react-hooks/set-state-in-effect -- reset to loading on filter/query change */
    setLoading(true);
    setError(null);
    /* eslint-enable react-hooks/set-state-in-effect */
    searchUserMRs({ filter, customQuery: query, perPage: 50 })
      .then((res) => {
        if (!cancelled) setMRs(res?.mrs ?? []);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Failed to load MRs");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [filter, query]);

  if (loading) {
    return (
      <div className="flex justify-center py-6">
        <Spinner className="h-4 w-4" />
      </div>
    );
  }
  if (error) {
    return <p className="text-sm text-destructive py-4">{error}</p>;
  }
  if (mrs.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-6 text-center">No merge requests found.</p>
    );
  }
  return (
    <ul className="space-y-1.5">
      {mrs.map((mr) => (
        <li key={`${mr.project_path}-${mr.iid}`}>
          <Link
            href={mr.web_url}
            target="_blank"
            rel="noopener noreferrer"
            className="block p-3 rounded-md border hover:border-primary/40 transition-colors cursor-pointer"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-sm font-medium truncate">
                  <span className="truncate">{mr.title}</span>
                  <IconExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
                </div>
                <p className="text-xs text-muted-foreground mt-1 truncate">
                  {mr.project_path} !{mr.iid} · {mr.head_branch} → {mr.base_branch}
                </p>
              </div>
              <MRStateBadge mr={mr} />
            </div>
          </Link>
        </li>
      ))}
    </ul>
  );
}

function IssueList({ filter, query }: { filter: string; query: string }) {
  const [issues, setIssues] = useState<Issue[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    /* eslint-disable react-hooks/set-state-in-effect -- reset to loading on filter/query change */
    setLoading(true);
    setError(null);
    /* eslint-enable react-hooks/set-state-in-effect */
    searchUserIssues({ filter, customQuery: query, perPage: 50 })
      .then((res) => {
        if (!cancelled) setIssues(res?.issues ?? []);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : "Failed to load issues");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [filter, query]);

  if (loading) {
    return (
      <div className="flex justify-center py-6">
        <Spinner className="h-4 w-4" />
      </div>
    );
  }
  if (error) {
    return <p className="text-sm text-destructive py-4">{error}</p>;
  }
  if (issues.length === 0) {
    return <p className="text-sm text-muted-foreground py-6 text-center">No issues found.</p>;
  }
  return (
    <ul className="space-y-1.5">
      {issues.map((issue) => (
        <li key={`${issue.project_path}-${issue.iid}`}>
          <Link
            href={issue.web_url}
            target="_blank"
            rel="noopener noreferrer"
            className="block p-3 rounded-md border hover:border-primary/40 transition-colors cursor-pointer"
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-sm font-medium truncate">
                  <span className="truncate">{issue.title}</span>
                  <IconExternalLink className="h-3 w-3 shrink-0 text-muted-foreground" />
                </div>
                <p className="text-xs text-muted-foreground mt-1 truncate">
                  {issue.project_path} #{issue.iid}
                  {issue.labels.length > 0 && ` · ${issue.labels.slice(0, 3).join(", ")}`}
                </p>
              </div>
              <IssueStateBadge issue={issue} />
            </div>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function GitLabPageClient({ workspaceId: _workspaceId }: { workspaceId?: string }) {
  const [status, setStatus] = useState<GitLabStatus | null>(null);
  const [statusLoading, setStatusLoading] = useState(true);
  const [mrFilter, setMrFilter] = useState<string>(MR_FILTERS[0].value);
  const [issueFilter, setIssueFilter] = useState<string>(ISSUE_FILTERS[0].value);
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");

  useEffect(() => {
    const t = setTimeout(() => setDebounced(search), 200);
    return () => clearTimeout(t);
  }, [search]);

  const reloadStatus = useCallback(() => {
    setStatusLoading(true);
    fetchGitLabStatus({ init: { cache: "no-store" } })
      .then(setStatus)
      .catch(() => setStatus(null))
      .finally(() => setStatusLoading(false));
  }, []);

  useEffect(() => {
    let cancelled = false;
    fetchGitLabStatus({ init: { cache: "no-store" } })
      .then((s) => {
        if (!cancelled) setStatus(s);
      })
      .catch(() => {
        if (!cancelled) setStatus(null);
      })
      .finally(() => {
        if (!cancelled) setStatusLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  // Pass debounced search through as the custom_query. GitLab understands
  // free-text search via the `search` URL param; mapping the raw text to it
  // keeps the surface simple — power users can still type the param-style
  // string like "labels=bug&author_username=me" and the backend forwards
  // it as customQuery.
  const customQuery = useMemo(() => {
    const q = debounced.trim();
    if (!q) return "";
    if (q.includes("=")) return q; // assume raw key=value
    return `search=${encodeURIComponent(q)}`;
  }, [debounced]);

  const connected = status?.authenticated || status?.token_configured;

  return (
    <div className="space-y-4 p-4 md:p-6">
      <PageHeader status={status} statusLoading={statusLoading} onRefresh={reloadStatus} />
      {!statusLoading && !connected ? (
        <NotConnectedNotice />
      ) : (
        <BrowseTabs
          search={search}
          setSearch={setSearch}
          mrFilter={mrFilter}
          setMrFilter={setMrFilter}
          issueFilter={issueFilter}
          setIssueFilter={setIssueFilter}
          customQuery={customQuery}
        />
      )}
    </div>
  );
}

function PageHeader({
  status,
  statusLoading,
  onRefresh,
}: {
  status: GitLabStatus | null;
  statusLoading: boolean;
  onRefresh: () => void;
}) {
  return (
    <header className="flex items-center justify-between gap-3">
      <div className="flex items-center gap-3">
        <IconBrandGitlab className="h-5 w-5 text-orange-500" />
        <div>
          <h2 className="text-xl font-bold">GitLab</h2>
          <p className="text-xs text-muted-foreground">
            {status?.host ?? "https://gitlab.com"} · merge requests and issues
          </p>
        </div>
      </div>
      <Button
        variant="outline"
        size="sm"
        onClick={onRefresh}
        disabled={statusLoading}
        className="gap-1 cursor-pointer"
      >
        <IconRefresh className="h-3 w-3" />
        Refresh
      </Button>
    </header>
  );
}

function BrowseTabs({
  search,
  setSearch,
  mrFilter,
  setMrFilter,
  issueFilter,
  setIssueFilter,
  customQuery,
}: {
  search: string;
  setSearch: (v: string) => void;
  mrFilter: string;
  setMrFilter: (v: string) => void;
  issueFilter: string;
  setIssueFilter: (v: string) => void;
  customQuery: string;
}) {
  return (
    <>
      <Input
        placeholder="Search (free text, or paste GitLab query like labels=bug&state=opened)"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="font-mono text-sm"
      />

      <Tabs defaultValue="mrs">
        <TabsList>
          <TabsTrigger value="mrs" className="gap-1.5">
            <IconGitMerge className="h-3.5 w-3.5" />
            Merge requests
          </TabsTrigger>
          <TabsTrigger value="issues" className="gap-1.5">
            <IconBug className="h-3.5 w-3.5" />
            Issues
          </TabsTrigger>
        </TabsList>
        <TabsContent value="mrs" className="space-y-3">
          <FilterButtons
            value={mrFilter}
            onChange={setMrFilter}
            options={MR_FILTERS as readonly { value: string; label: string }[]}
          />
          <MRList filter={mrFilter} query={customQuery} />
        </TabsContent>
        <TabsContent value="issues" className="space-y-3">
          <FilterButtons
            value={issueFilter}
            onChange={setIssueFilter}
            options={ISSUE_FILTERS as readonly { value: string; label: string }[]}
          />
          <IssueList filter={issueFilter} query={customQuery} />
        </TabsContent>
      </Tabs>
    </>
  );
}

function FilterButtons({
  value,
  onChange,
  options,
}: {
  value: string;
  onChange: (v: string) => void;
  options: readonly { value: string; label: string }[];
}) {
  return (
    <div className="flex gap-1.5 flex-wrap">
      {options.map((f) => (
        <Button
          key={f.value}
          size="sm"
          variant={value === f.value ? "default" : "outline"}
          onClick={() => onChange(f.value)}
          className="cursor-pointer"
        >
          {f.label}
        </Button>
      ))}
    </div>
  );
}
