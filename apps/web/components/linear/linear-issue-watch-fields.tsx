"use client";

import { type Dispatch, type SetStateAction, useEffect, useRef, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Switch } from "@kandev/ui/switch";
import {
  listLinearLabels,
  listLinearProjects,
  listLinearStates,
  listLinearTeams,
  listLinearUsers,
} from "@/lib/api/domains/linear-api";
import {
  PRIORITY_OPTIONS,
  parseMaxInflightTasks,
  SORT_BY_OPTIONS,
  type FormState,
  type LinearPriority,
} from "./linear-issue-watch-form";
import type {
  LinearLabel,
  LinearProject,
  LinearTeam,
  LinearUser,
  LinearWorkflowState,
} from "@/lib/types/linear";
import { useTranslation } from "react-i18next";
import { resolveOptionLabel } from "@/lib/i18n/option-label";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

// teamCacheKey scopes per-team caches to the workspace: two workspaces can
// connect different Linear organizations that both have a team with the same
// key.
function teamCacheKey(workspaceId: string, teamKey: string): string {
  return JSON.stringify([workspaceId, teamKey]);
}

// useTeamsAndStates loads the team list once Linear is configured, plus the
// states, labels, projects, and users for the currently-selected team. Each per-team
// dataset is cached so switching teams renders an empty list (or the cached
// list) without us having to setState in an effect — only the lookup
// expression changes.
export function useTeamsAndStates(workspaceId: string, teamKey: string) {
  const [teams, setTeams] = useState<LinearTeam[]>([]);
  const [statesByTeam, setStatesByTeam] = useState<Record<string, LinearWorkflowState[]>>({});
  const [labelsByTeam, setLabelsByTeam] = useState<Record<string, LinearLabel[]>>({});
  const [usersByTeam, setUsersByTeam] = useState<Record<string, LinearUser[]>>({});
  const [projectsByTeam, setProjectsByTeam] = useState<Record<string, LinearProject[]>>({});
  const fetchedTeams = useRef<Set<string>>(new Set());

  useEffect(() => {
    let cancelled = false;
    listLinearTeams({ workspaceId })
      .then((res) => {
        if (!cancelled) setTeams(res.teams ?? []);
      })
      .catch(() => {
        if (!cancelled) setTeams([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  useEffect(() => {
    // Capture the Set once so cleanup uses the exact instance the effect
    // body added to (ref.current is stable across renders, but eslint can't
    // prove that — so we assign to a local).
    const fetched = fetchedTeams.current;
    if (!teamKey) return;
    const key = teamCacheKey(workspaceId, teamKey);
    if (fetched.has(key)) return;
    // Mark in-flight so concurrent renders don't double-fetch; clear on
    // failure so a subsequent remount can retry instead of being stuck with
    // an empty cached entry.
    fetched.add(key);
    let cancelled = false;
    let anyFailed = false;
    let loaded = false;
    const markFailed = () => {
      anyFailed = true;
    };
    Promise.allSettled([
      listLinearStates(teamKey, { workspaceId })
        .then((res) => {
          if (!cancelled) setStatesByTeam((prev) => ({ ...prev, [key]: res.states ?? [] }));
        })
        .catch(() => {
          markFailed();
          if (!cancelled) setStatesByTeam((prev) => ({ ...prev, [key]: [] }));
        }),
      listLinearLabels(teamKey, { workspaceId })
        .then((res) => {
          if (!cancelled) setLabelsByTeam((prev) => ({ ...prev, [key]: res.labels ?? [] }));
        })
        .catch(() => {
          markFailed();
          if (!cancelled) setLabelsByTeam((prev) => ({ ...prev, [key]: [] }));
        }),
      listLinearUsers(teamKey, { workspaceId })
        .then((res) => {
          if (!cancelled) setUsersByTeam((prev) => ({ ...prev, [key]: res.users ?? [] }));
        })
        .catch(() => {
          markFailed();
          if (!cancelled) setUsersByTeam((prev) => ({ ...prev, [key]: [] }));
        }),
      listLinearProjects(teamKey, { workspaceId })
        .then((res) => {
          if (!cancelled) setProjectsByTeam((prev) => ({ ...prev, [key]: res.projects ?? [] }));
        })
        .catch(() => {
          markFailed();
          if (!cancelled) setProjectsByTeam((prev) => ({ ...prev, [key]: [] }));
        }),
    ]).finally(() => {
      // Track full success so cleanup can keep the cache marker — otherwise
      // every cleanup wipes it and the next visit refetches.
      loaded = !cancelled && !anyFailed;
      // If a fetch failed (and we didn't cancel), drop the marker so the
      // next visit to this team can retry. Success-path marker stays so
      // cached data is reused.
      if (anyFailed) fetched.delete(key);
    });
    return () => {
      cancelled = true;
      // Drop the marker on cleanup ONLY when the fetch hadn't completed
      // successfully — that handles the rapid-switch (A→B→A) case without
      // also evicting a healthy cache after a normal team change.
      if (!loaded) fetched.delete(key);
    };
  }, [workspaceId, teamKey]);

  const cacheKey = teamKey ? teamCacheKey(workspaceId, teamKey) : "";
  const states = teamKey ? (statesByTeam[cacheKey] ?? []) : [];
  const labels = teamKey ? (labelsByTeam[cacheKey] ?? []) : [];
  const users = teamKey ? (usersByTeam[cacheKey] ?? []) : [];
  const projects = teamKey ? (projectsByTeam[cacheKey] ?? []) : [];
  const loadingStates = !!teamKey && statesByTeam[cacheKey] === undefined;
  const loadingLabels = !!teamKey && labelsByTeam[cacheKey] === undefined;
  const loadingUsers = !!teamKey && usersByTeam[cacheKey] === undefined;
  const loadingProjects = !!teamKey && projectsByTeam[cacheKey] === undefined;
  return {
    teams,
    states,
    labels,
    users,
    projects,
    loadingStates,
    loadingLabels,
    loadingUsers,
    loadingProjects,
  };
}

export function StateMultiSelect({
  states,
  loading,
  selected,
  onToggle,
  disabled,
}: {
  states: LinearWorkflowState[];
  loading: boolean;
  selected: string[];
  onToggle: (id: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  if (disabled) {
    // Caller's row description already explains the disabled state — render
    // nothing here to avoid duplicate prose.
    return null;
  }
  if (loading) {
    return <p className="text-xs text-muted-foreground">{t("linear:loadingStates")}</p>;
  }
  if (states.length === 0) {
    return <p className="text-xs text-muted-foreground">{t("linear:noWorkflowStatesAvailable")}</p>;
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {states.map((s) => {
        const active = selected.includes(s.id);
        return (
          <button
            key={s.id}
            type="button"
            onClick={() => onToggle(s.id)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            {/* Workflow state names come from the Linear API — user data. */}
            <Badge variant={active ? "default" : "outline"}>{s.name}</Badge>
          </button>
        );
      })}
    </div>
  );
}

export function PriorityMultiSelect({
  selected,
  onToggle,
}: {
  selected: LinearPriority[];
  onToggle: (p: LinearPriority) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap gap-1.5">
      {PRIORITY_OPTIONS.map((opt) => {
        const active = selected.includes(opt.value);
        return (
          <button
            key={opt.value}
            type="button"
            onClick={() => onToggle(opt.value)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            <Badge variant={active ? "default" : "outline"}>{resolveOptionLabel(t, opt)}</Badge>
          </button>
        );
      })}
    </div>
  );
}

export function LabelMultiSelect({
  labels,
  loading,
  selected,
  onToggle,
  disabled,
}: {
  labels: LinearLabel[];
  loading: boolean;
  selected: string[];
  onToggle: (id: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  if (disabled) {
    // Caller's row description already explains the disabled state — render
    // nothing here to avoid duplicate prose.
    return null;
  }
  if (loading) {
    return <p className="text-xs text-muted-foreground">{t("linear:loadingLabels")}</p>;
  }
  if (labels.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">{t("linear:noLabelsAvailableForThisTeam")}</p>
    );
  }
  return (
    <div className="flex flex-wrap gap-1.5">
      {labels.map((l) => {
        const active = selected.includes(l.id);
        return (
          <button
            key={l.id}
            type="button"
            onClick={() => onToggle(l.id)}
            aria-pressed={active}
            className="cursor-pointer"
          >
            {/* Label names come from the Linear API — user data. */}
            <Badge variant={active ? "default" : "outline"}>{l.name}</Badge>
          </button>
        );
      })}
    </div>
  );
}

export function ProjectMultiSelect({
  projects,
  loading,
  selected,
  onToggle,
  disabled,
}: {
  projects: LinearProject[];
  loading: boolean;
  selected: string[];
  onToggle: (id: string) => void;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  if (disabled) return null;
  if (loading) {
    return <p className="text-xs text-muted-foreground">{t("linear:loadingProjects")}</p>;
  }
  // Selected projects that the team no longer lists (archived, moved, or no
  // longer accessible) stay removable instead of becoming a hidden filter.
  const listed = new Set(projects.map((project) => project.id));
  const unavailable = selected.filter((id) => !listed.has(id));
  if (projects.length === 0 && unavailable.length === 0) {
    return (
      <p className="text-xs text-muted-foreground">{t("linear:noProjectsAvailableForThisTeam")}</p>
    );
  }
  const chipClass = isFinePointer
    ? "cursor-pointer"
    : "inline-flex min-h-11 min-w-11 cursor-pointer items-center";
  return (
    <div className="flex flex-wrap gap-1.5" data-testid="linear-watch-projects">
      {projects.map((project) => {
        const active = selected.includes(project.id);
        return (
          <button
            key={project.id}
            type="button"
            onClick={() => onToggle(project.id)}
            aria-pressed={active}
            className={chipClass}
          >
            {/* Project names come from the Linear API — user data. */}
            <Badge variant={active ? "default" : "outline"}>{project.name}</Badge>
          </button>
        );
      })}
      {unavailable.map((id) => (
        <button
          key={id}
          type="button"
          onClick={() => onToggle(id)}
          aria-pressed
          title={id}
          className={chipClass}
        >
          <Badge variant="default">{t("linear:unavailableProject")}</Badge>
        </button>
      ))}
    </div>
  );
}

type FormSetter = Dispatch<SetStateAction<FormState>>;

// Radix SelectItem rejects an empty-string value, so the "Default (Linear
// order)" option uses a sentinel in the dropdown that we translate back to ""
// (FormState's empty default) at the Select boundary.
const SORT_BY_DEFAULT_SENTINEL = "__default__";

export function SortByField({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1.5">
      <Label>{t("linear:dispatchOrder")}</Label>
      <p className="text-xs text-muted-foreground">{t("linear:dispatchOrderHelp")}</p>
      <Select
        value={form.sortBy || SORT_BY_DEFAULT_SENTINEL}
        onValueChange={(v) =>
          setForm((p) => ({
            ...p,
            sortBy: (v === SORT_BY_DEFAULT_SENTINEL ? "" : v) as FormState["sortBy"],
          }))
        }
      >
        <SelectTrigger className="cursor-pointer">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {SORT_BY_OPTIONS.map((o) => (
            <SelectItem
              key={o.value || SORT_BY_DEFAULT_SENTINEL}
              value={o.value || SORT_BY_DEFAULT_SENTINEL}
            >
              {resolveOptionLabel(t, o)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

// MaxInflightTasksField renders the per-watcher throttle-cap input with inline
// validation. Lives here (rather than the dialog) to keep the dialog file under
// its line ceiling.
export function MaxInflightTasksField({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  const parsed = parseMaxInflightTasks(form.maxInflightTasks);
  const invalid = parsed === "invalid";
  return (
    <div className="space-y-1.5">
      <Label>{t("linear:maxInflightTasks")}</Label>
      <p className="text-xs text-muted-foreground">{t("linear:maxInflightTasksHelp")}</p>
      <Input
        type="number"
        value={form.maxInflightTasks}
        onChange={(e) => setForm((p) => ({ ...p, maxInflightTasks: e.target.value }))}
        min={1}
        step={1}
        placeholder={t("linear:noCap")}
        aria-invalid={invalid}
      />
      {invalid && (
        <p className="text-xs text-destructive">{t("linear:enterAPositiveIntegerOrBlank")}</p>
      )}
    </div>
  );
}

// SettingsFields renders the poll-interval, throttle-cap, and enabled toggle —
// the trailing "Settings" block of the watcher dialog.
export function SettingsFields({ form, setForm }: { form: FormState; setForm: FormSetter }) {
  const { t } = useTranslation();
  return (
    <>
      <div className="space-y-1.5">
        <Label>{t("linear:pollIntervalSeconds")}</Label>
        <p className="text-xs text-muted-foreground">{t("linear:pollIntervalHelp")}</p>
        <Input
          type="number"
          value={form.pollInterval}
          onChange={(e) => setForm((p) => ({ ...p, pollInterval: Number(e.target.value) }))}
          min={60}
          max={3600}
        />
      </div>
      <MaxInflightTasksField form={form} setForm={setForm} />
      <SortByField form={form} setForm={setForm} />
      <div className="flex items-center justify-between">
        <div>
          <Label>{t("linear:enabled")}</Label>
          <p className="text-xs text-muted-foreground">{t("linear:enabledHelp")}</p>
        </div>
        <Switch
          checked={form.enabled}
          onCheckedChange={(v) => setForm((p) => ({ ...p, enabled: v }))}
          className="cursor-pointer"
        />
      </div>
    </>
  );
}
