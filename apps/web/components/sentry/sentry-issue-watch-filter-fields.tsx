"use client";

import { useState, useEffect, useCallback, useMemo } from "react";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { listSentryProjects } from "@/lib/api/domains/sentry-api";
import {
  LevelMultiSelect,
  StatusMultiSelect,
  ProjectMultiSelect,
} from "./sentry-issue-watch-multiselect";
import {
  STATS_PERIOD_OPTIONS,
  type FormState,
  orgSelectItems,
  projectMultiSelectItems,
} from "./sentry-issue-watch-form";
import type { SentryLevel, SentryProject, SentryStatus } from "@/lib/types/sentry";

export type FormSetter = React.Dispatch<React.SetStateAction<FormState>>;

// useSentryProjects loads the workspace/instance's projects and narrows them
// to the currently-selected org — Sentry's auth-token endpoint already
// filters to the user's accessible orgs, so this is a client-side scope, not
// a second network filter.
function useSentryProjects(workspaceId: string, instanceId: string, orgSlug: string) {
  const [projects, setProjects] = useState<SentryProject[]>([]);
  useEffect(() => {
    if (!workspaceId || !instanceId) {
      setProjects([]);
      return;
    }
    setProjects([]);
    let cancelled = false;
    listSentryProjects(workspaceId, instanceId)
      .then((res) => {
        if (!cancelled) setProjects(res.projects ?? []);
      })
      .catch(() => {
        if (!cancelled) setProjects([]);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId, instanceId]);
  return useMemo(
    () => (orgSlug ? projects.filter((p) => p.orgSlug === orgSlug) : projects),
    [projects, orgSlug],
  );
}

export function SelectField(props: {
  label: string;
  description?: string;
  value: string;
  onChange: (v: string) => void;
  placeholder: string;
  items: { id: string; label: string }[];
  disabled?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{props.label}</Label>
      {props.description && <p className="text-xs text-muted-foreground">{props.description}</p>}
      <Select
        value={props.value || undefined}
        onValueChange={props.onChange}
        disabled={props.disabled}
      >
        <SelectTrigger className="cursor-pointer">
          <SelectValue placeholder={props.placeholder} />
        </SelectTrigger>
        <SelectContent>
          {props.items.map((item) => (
            <SelectItem key={item.id} value={item.id}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function ProjectMultiSelectField(props: {
  label: string;
  description?: string;
  selected: string[];
  onChange: (v: string[]) => void;
  placeholder: string;
  items: { id: string; label: string }[];
  disabled?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <Label>{props.label}</Label>
      {props.description && <p className="text-xs text-muted-foreground">{props.description}</p>}
      <ProjectMultiSelect
        options={props.items}
        selected={props.selected}
        onChange={props.onChange}
        placeholder={props.placeholder}
        disabled={props.disabled}
      />
    </div>
  );
}

function OrgProjectRow({
  form,
  setForm,
  projects,
  orgs,
}: {
  form: FormState;
  setForm: FormSetter;
  projects: SentryProject[];
  orgs: string[];
}) {
  const onOrgChange = (v: string) =>
    // The selected projects may belong to a different org — clear them so the
    // project picker re-populates within the new org.
    setForm((p) => ({ ...p, orgSlug: v, projectSlugs: [] }));
  const orgItems = orgSelectItems(orgs, form.orgSlug);
  const projectItems = projectMultiSelectItems(projects, form.projectSlugs);
  return (
    <div className="grid grid-cols-2 gap-4">
      <SelectField
        label="Organization slug"
        description="The Sentry org to poll."
        value={form.orgSlug}
        onChange={onOrgChange}
        placeholder={orgItems.length === 0 ? "No organizations available" : "Select organization"}
        items={orgItems}
        disabled={orgItems.length === 0}
      />
      <ProjectMultiSelectField
        label="Project slug"
        description="The Sentry project(s) to poll. Select one or more."
        selected={form.projectSlugs}
        onChange={(v: string[]) => setForm((p) => ({ ...p, projectSlugs: v }))}
        placeholder={projectItems.length === 0 ? "No projects available" : "Select projects"}
        items={projectItems}
        disabled={projectItems.length === 0}
      />
    </div>
  );
}

export function FilterFields({
  form,
  setForm,
  orgs,
}: {
  form: FormState;
  setForm: FormSetter;
  orgs: string[];
}) {
  const projects = useSentryProjects(form.workspaceId, form.sentryInstanceId, form.orgSlug);
  const toggleLevel = useCallback(
    (level: SentryLevel) =>
      setForm((p) => ({
        ...p,
        levels: p.levels.includes(level)
          ? p.levels.filter((l) => l !== level)
          : [...p.levels, level],
      })),
    [setForm],
  );
  const toggleStatus = useCallback(
    (status: SentryStatus) =>
      setForm((p) => ({
        ...p,
        statuses: p.statuses.includes(status)
          ? p.statuses.filter((s) => s !== status)
          : [...p.statuses, status],
      })),
    [setForm],
  );
  return (
    <>
      <OrgProjectRow form={form} setForm={setForm} projects={projects} orgs={orgs} />
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1.5">
          <Label>Environment</Label>
          <p className="text-xs text-muted-foreground">Optional — restrict to one environment.</p>
          <Input
            value={form.environment}
            onChange={(e) => setForm((p) => ({ ...p, environment: e.target.value }))}
            placeholder="production"
          />
        </div>
        <SelectField
          label="Stats period"
          description="How far back to look for matching issues."
          value={form.statsPeriod}
          onChange={(v) => setForm((p) => ({ ...p, statsPeriod: v }))}
          placeholder="(any)"
          items={STATS_PERIOD_OPTIONS.map((o) => ({ id: o.value, label: o.label }))}
        />
      </div>
      <div className="space-y-1.5">
        <Label>Levels</Label>
        <p className="text-xs text-muted-foreground">
          Click to toggle. Matches issues at ANY of the selected levels.
        </p>
        <LevelMultiSelect selected={form.levels} onToggle={toggleLevel} />
      </div>
      <div className="space-y-1.5">
        <Label>Statuses</Label>
        <p className="text-xs text-muted-foreground">
          Click to toggle. Matches issues at ANY of the selected statuses.
        </p>
        <StatusMultiSelect selected={form.statuses} onToggle={toggleStatus} />
      </div>
      <div className="space-y-1.5">
        <Label>Query</Label>
        <p className="text-xs text-muted-foreground">Free-text Sentry search query (optional).</p>
        <Input
          value={form.query}
          onChange={(e) => setForm((p) => ({ ...p, query: e.target.value }))}
          placeholder="is:unresolved transaction:/api/checkout"
        />
      </div>
    </>
  );
}
