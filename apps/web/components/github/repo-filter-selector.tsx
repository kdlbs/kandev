"use client";

import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { IconX, IconLoader2, IconPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandItem,
  CommandGroup,
} from "@kandev/ui/command";
import { Label } from "@kandev/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Switch } from "@kandev/ui/switch";
import { cn } from "@/lib/utils";
import {
  fetchGitHubWorkspaceSettings,
  listUserOrgs,
  searchOrgRepos,
} from "@/lib/api/domains/github-api";
import type {
  RepoFilter,
  GitHubOrg,
  GitHubRepoInfo,
  GitHubWorkspaceSettings,
} from "@/lib/types/github";

type RepoFilterSelectorProps = {
  allRepos: boolean;
  selectedRepos: RepoFilter[];
  onAllReposChange: (checked: boolean) => void;
  onSelectedReposChange: (repos: RepoFilter[]) => void;
  workspaceId?: string;
};

function useGitHubOrgs() {
  const [orgs, setOrgs] = useState<GitHubOrg[]>([]);
  const [loading, setLoading] = useState(true);
  const fetchedRef = useRef(false);

  useEffect(() => {
    if (fetchedRef.current) return;
    fetchedRef.current = true;
    listUserOrgs()
      .then((r) => setOrgs(r.orgs ?? []))
      .catch(() => setOrgs([]))
      .finally(() => setLoading(false));
  }, []);

  return { orgs, loading };
}

function useRepoSearch(org: string, query: string) {
  const [searchState, setSearchState] = useState<{
    results: GitHubRepoInfo[];
    loading: boolean;
    org: string;
  }>({ results: [], loading: false, org: "" });
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => {
    clearTimeout(timerRef.current);
    if (!org) return;
    timerRef.current = setTimeout(() => {
      setSearchState((prev) => ({ ...prev, loading: true, org }));
      searchOrgRepos(org, query || undefined)
        .then((r) => setSearchState({ results: r.repos ?? [], loading: false, org }))
        .catch(() => setSearchState({ results: [], loading: false, org }));
    }, 300);
    return () => clearTimeout(timerRef.current);
  }, [org, query]);

  // Clear results when org becomes empty (derived, not in effect)
  const results = org ? searchState.results : [];
  const loading = org ? searchState.loading : false;

  return { results, loading };
}

function useWorkspaceRepoScope(workspaceId: string | undefined) {
  const [settings, setSettings] = useState<GitHubWorkspaceSettings | null>(null);

  useEffect(() => {
    if (!workspaceId) {
      setSettings(null);
      return;
    }
    let cancelled = false;
    void fetchGitHubWorkspaceSettings(workspaceId)
      .then((next) => {
        if (!cancelled) setSettings(next);
      })
      .catch(() => {
        if (!cancelled) setSettings(null);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  return settings;
}

function repoAllowedByScope(settings: GitHubWorkspaceSettings | null, owner: string, repo: string) {
  if (!settings || settings.repo_scope_mode === "all") return true;
  const ownerLower = owner.trim().toLowerCase();
  const repoLower = repo.trim().toLowerCase();
  if (settings.repo_scope_mode === "orgs") {
    return (settings.repo_scope_orgs ?? []).some((allowed) => {
      return allowed.trim().toLowerCase() === ownerLower;
    });
  }
  return (settings.repo_scope_repos ?? []).some((allowed) => {
    return (
      allowed.owner.trim().toLowerCase() === ownerLower &&
      allowed.name.trim().toLowerCase() === repoLower
    );
  });
}

function orgAllowedByScope(settings: GitHubWorkspaceSettings | null, org: string) {
  if (!settings || settings.repo_scope_mode === "all") return true;
  const orgLower = org.trim().toLowerCase();
  if (settings.repo_scope_mode === "orgs") {
    return (settings.repo_scope_orgs ?? []).some((allowed) => {
      return allowed.trim().toLowerCase() === orgLower;
    });
  }
  return false;
}

function SelectedFilters({
  repos,
  onRemove,
  disabled,
}: {
  repos: RepoFilter[];
  onRemove: (r: RepoFilter) => void;
  disabled: boolean;
}) {
  if (repos.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-1.5 mt-2">
      {repos.map((r) => {
        const label = r.name === "" ? `${r.owner}/*` : `${r.owner}/${r.name}`;
        return (
          <Badge
            key={label}
            variant="secondary"
            className={cn("text-xs gap-1 pr-1", disabled && "opacity-50")}
          >
            {label}
            <button
              type="button"
              className="ml-0.5 hover:text-foreground cursor-pointer"
              onClick={() => onRemove(r)}
              disabled={disabled}
            >
              <IconX className="h-3 w-3" />
            </button>
          </Badge>
        );
      })}
    </div>
  );
}

function OrgBadges({
  orgs,
  loading,
  selectedRepos,
  disabled,
  onToggleOrg,
}: {
  orgs: GitHubOrg[];
  loading: boolean;
  selectedRepos: RepoFilter[];
  disabled: boolean;
  onToggleOrg: (login: string) => void;
}) {
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <IconLoader2 className="h-3 w-3 animate-spin" />
        Loading organizations...
      </div>
    );
  }
  if (orgs.length === 0) return null;

  return (
    <div className="flex flex-wrap gap-1.5">
      {orgs.map((org) => {
        const isSelected = selectedRepos.some((r) => r.owner === org.login && r.name === "");
        return (
          <Badge
            key={org.login}
            variant={isSelected ? "default" : "outline"}
            className={cn(
              "text-xs cursor-pointer select-none",
              disabled && "opacity-50 pointer-events-none",
            )}
            onClick={() => onToggleOrg(org.login)}
          >
            {org.login}
          </Badge>
        );
      })}
    </div>
  );
}

function RepoSearchCombobox({
  disabled,
  scope,
  onAdd,
}: {
  disabled: boolean;
  scope: GitHubWorkspaceSettings | null;
  onAdd: (owner: string, name: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");

  const slashIdx = value.indexOf("/");
  const org = slashIdx > 0 ? value.slice(0, slashIdx) : "";
  const query = slashIdx > 0 ? value.slice(slashIdx + 1) : "";
  const { results, loading: searchLoading } = useRepoSearch(org, query);
  const filteredResults = useMemo(
    () => results.filter((repo) => repoAllowedByScope(scope, repo.owner, repo.name)).slice(0, 10),
    [results, scope],
  );

  const handleSelect = useCallback(
    (fullName: string) => {
      const [owner, ...rest] = fullName.split("/");
      const name = rest.join("/");
      if (owner && name) {
        onAdd(owner, name);
        setValue("");
        setOpen(false);
      }
    },
    [onAdd],
  );

  return (
    <Popover open={open && !disabled} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={disabled}
          className="cursor-pointer text-xs gap-1"
        >
          <IconPlus className="h-3 w-3" />
          Add repository
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-72 p-0" align="start" portal={false}>
        <Command shouldFilter={false}>
          <CommandInput value={value} onValueChange={setValue} placeholder="owner/repo" />
          <CommandList>
            {searchLoading && (
              <div className="flex items-center gap-2 px-3 py-3 text-xs text-muted-foreground">
                <IconLoader2 className="h-3 w-3 animate-spin" />
                Searching...
              </div>
            )}
            {!searchLoading && org && filteredResults.length === 0 && (
              <CommandEmpty>No repos found for &quot;{org}&quot;</CommandEmpty>
            )}
            {!searchLoading && !org && value.length > 0 && (
              <CommandEmpty>Type owner/repo to search</CommandEmpty>
            )}
            {filteredResults.length > 0 && (
              <CommandGroup>
                {filteredResults.map((repo) => (
                  <CommandItem
                    key={repo.full_name}
                    value={repo.full_name}
                    onSelect={handleSelect}
                    className="cursor-pointer"
                  >
                    {repo.full_name}
                    {repo.private && (
                      <span className="ml-auto text-xs text-muted-foreground">private</span>
                    )}
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

export function RepoFilterSelector({
  allRepos,
  selectedRepos,
  onAllReposChange,
  onSelectedReposChange,
  workspaceId,
}: RepoFilterSelectorProps) {
  const { orgs, loading: orgsLoading } = useGitHubOrgs();
  const scope = useWorkspaceRepoScope(workspaceId);
  const scopedOrgs = useMemo(
    () => orgs.filter((org) => orgAllowedByScope(scope, org.login)),
    [orgs, scope],
  );
  const showOrgBadges = !scope || scope.repo_scope_mode !== "repos";

  const toggleOrg = useCallback(
    (login: string) => {
      const exists = selectedRepos.some((r) => r.owner === login && r.name === "");
      if (exists) {
        onSelectedReposChange(selectedRepos.filter((r) => !(r.owner === login && r.name === "")));
      } else {
        onSelectedReposChange([...selectedRepos, { owner: login, name: "" }]);
      }
    },
    [selectedRepos, onSelectedReposChange],
  );

  const addRepo = useCallback(
    (owner: string, name: string) => {
      const exists = selectedRepos.some((r) => r.owner === owner && r.name === name);
      if (!exists) {
        onSelectedReposChange([...selectedRepos, { owner, name }]);
      }
    },
    [selectedRepos, onSelectedReposChange],
  );

  const removeFilter = useCallback(
    (filter: RepoFilter) => {
      onSelectedReposChange(
        selectedRepos.filter((r) => !(r.owner === filter.owner && r.name === filter.name)),
      );
    },
    [selectedRepos, onSelectedReposChange],
  );

  return (
    <div className="space-y-3">
      <div>
        <Label>Repositories</Label>
        <p className="text-xs text-muted-foreground">
          Which allowed GitHub repositories to monitor for this watch.
        </p>
      </div>

      <div className="flex items-center gap-2">
        <Switch
          id="all-repos-toggle"
          checked={allRepos}
          onCheckedChange={onAllReposChange}
          className="cursor-pointer"
        />
        <Label htmlFor="all-repos-toggle" className="font-normal cursor-pointer">
          All repositories allowed by this workspace
        </Label>
      </div>

      {!allRepos && (
        <>
          {showOrgBadges && (
            <div className="space-y-1.5">
              <Label className="text-xs text-muted-foreground font-normal">Organizations</Label>
              <OrgBadges
                orgs={scopedOrgs}
                loading={orgsLoading}
                selectedRepos={selectedRepos}
                disabled={allRepos}
                onToggleOrg={toggleOrg}
              />
            </div>
          )}

          <RepoSearchCombobox disabled={allRepos} scope={scope} onAdd={addRepo} />

          <SelectedFilters repos={selectedRepos} onRemove={removeFilter} disabled={allRepos} />
        </>
      )}
    </div>
  );
}
