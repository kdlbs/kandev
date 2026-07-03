"use client";

import { useMemo, useState } from "react";
import { IconChevronDown, IconX } from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import type { JiraProject, JiraStatus } from "@/lib/types/jira";
import { type AssigneeFilter } from "./filter-model";

type PillShellProps = {
  label: string;
  summary: string | null;
  active: boolean;
  onClear?: () => void;
  // When disabled the pill renders greyed-out and non-interactive; `disabledHint`
  // is surfaced via the title tooltip so the user learns why (e.g. pick a
  // project first).
  disabled?: boolean;
  disabledHint?: string;
  children: React.ReactNode;
};

function DisabledPill({
  label,
  summary,
  disabledHint,
}: Pick<PillShellProps, "label" | "summary" | "disabledHint">) {
  return (
    <div
      data-testid={`jira-filter-pill-${label.toLowerCase()}`}
      data-disabled="true"
      className="inline-flex items-stretch rounded-md border text-xs overflow-hidden bg-background opacity-50"
      title={disabledHint}
    >
      <span className="px-2.5 py-1.5 flex items-center gap-1.5 cursor-not-allowed">
        <span className="text-muted-foreground">{label}</span>
        {summary && <span className="font-medium">{summary}</span>}
        <IconChevronDown className="h-3 w-3 text-muted-foreground" />
      </span>
    </div>
  );
}

function PillShell({
  label,
  summary,
  active,
  onClear,
  disabled,
  disabledHint,
  children,
}: PillShellProps) {
  const [open, setOpen] = useState(false);
  if (disabled) {
    return <DisabledPill label={label} summary={summary} disabledHint={disabledHint} />;
  }
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <div
        className={`inline-flex items-stretch rounded-md border text-xs overflow-hidden ${
          active ? "border-primary/40 bg-primary/5" : "bg-background"
        }`}
      >
        <PopoverTrigger asChild>
          <button
            type="button"
            data-testid={`jira-filter-pill-${label.toLowerCase()}`}
            className="cursor-pointer px-2.5 py-1.5 flex items-center gap-1.5 hover:bg-muted/50 transition-colors"
          >
            <span className="text-muted-foreground">{label}</span>
            {summary && <span className="font-medium">{summary}</span>}
            <IconChevronDown className="h-3 w-3 text-muted-foreground" />
          </button>
        </PopoverTrigger>
        {active && onClear && (
          <button
            type="button"
            onClick={onClear}
            className="cursor-pointer px-1.5 border-l hover:bg-muted flex items-center"
            title={`Clear ${label.toLowerCase()}`}
          >
            <IconX className="h-3 w-3 text-muted-foreground" />
          </button>
        )}
      </div>
      <PopoverContent align="start" className="w-64 p-0">
        {children}
      </PopoverContent>
    </Popover>
  );
}

function joinSummary(items: string[], max: number): string {
  if (items.length <= max) return items.join(", ");
  return `${items.slice(0, max).join(", ")} +${items.length - max}`;
}

type ProjectPillProps = {
  projects: JiraProject[];
  value: string[];
  onChange: (keys: string[]) => void;
};

export function ProjectPill({ projects, value, onChange }: ProjectPillProps) {
  const [query, setQuery] = useState("");
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return projects;
    return projects.filter(
      (p) => p.key.toLowerCase().includes(q) || p.name.toLowerCase().includes(q),
    );
  }, [projects, query]);

  const selected = new Set(value);
  const toggle = (key: string) => {
    if (selected.has(key)) onChange(value.filter((k) => k !== key));
    else onChange([...value, key]);
  };

  return (
    <PillShell
      label="Project"
      summary={value.length > 0 ? joinSummary(value, 2) : null}
      active={value.length > 0}
      onClear={() => onChange([])}
    >
      <div className="p-2 border-b">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search projects…"
          className="h-7 text-xs"
        />
      </div>
      <div className="max-h-64 overflow-y-auto py-1">
        {filtered.length === 0 && (
          <div className="px-3 py-2 text-xs text-muted-foreground">No projects match.</div>
        )}
        {filtered.map((p) => (
          <label
            key={p.key}
            data-testid={`jira-project-option-${p.key}`}
            className="flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-muted/50"
          >
            <Checkbox checked={selected.has(p.key)} onCheckedChange={() => toggle(p.key)} />
            <span className="font-mono text-xs">{p.key}</span>
            <span className="text-xs text-muted-foreground truncate">{p.name}</span>
          </label>
        ))}
      </div>
    </PillShell>
  );
}

type StatusPillProps = {
  // Available statuses for the selected project(s). Empty means either no
  // project is selected or the selected project(s) expose no statuses; the pill
  // is disabled either way, with a hint chosen from hasProjectSelected.
  options: JiraStatus[];
  value: string[];
  onChange: (statuses: string[]) => void;
  // Whether at least one project is selected. Distinguishes "pick a project
  // first" from "this project has no statuses" so the disabled hint isn't
  // misleading once a project is chosen.
  hasProjectSelected: boolean;
};

const NO_PROJECT_HINT = "Select a project to filter by status";
const NO_STATUSES_HINT = "No statuses available for the selected project";

export function StatusPill({ options, value, onChange, hasProjectSelected }: StatusPillProps) {
  const [query, setQuery] = useState("");
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return options;
    return options.filter((o) => o.name.toLowerCase().includes(q));
  }, [options, query]);

  const selected = new Set(value);
  const toggle = (name: string) => {
    if (selected.has(name)) onChange(value.filter((n) => n !== name));
    else onChange([...value, name]);
  };
  const summary = value.length > 0 ? joinSummary(value, 2) : null;

  return (
    <PillShell
      label="Status"
      summary={summary}
      active={value.length > 0}
      onClear={() => onChange([])}
      disabled={options.length === 0}
      disabledHint={hasProjectSelected ? NO_STATUSES_HINT : NO_PROJECT_HINT}
    >
      <div className="p-2 border-b">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search statuses…"
          className="h-7 text-xs"
        />
      </div>
      <div className="max-h-64 overflow-y-auto py-1">
        {filtered.length === 0 && (
          <div className="px-3 py-2 text-xs text-muted-foreground">No statuses match.</div>
        )}
        {filtered.map((o) => (
          <label
            key={o.id}
            data-testid={`jira-status-option-${o.name}`}
            className="flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-muted/50"
          >
            <Checkbox checked={selected.has(o.name)} onCheckedChange={() => toggle(o.name)} />
            <span className="text-sm truncate">{o.name}</span>
          </label>
        ))}
      </div>
    </PillShell>
  );
}

type AssigneePillProps = {
  value: AssigneeFilter;
  onChange: (a: AssigneeFilter) => void;
};

const ASSIGNEE_OPTIONS: { value: AssigneeFilter; label: string }[] = [
  { value: "anyone", label: "Anyone" },
  { value: "me", label: "Me" },
  { value: "unassigned", label: "Unassigned" },
];

export function AssigneePill({ value, onChange }: AssigneePillProps) {
  const active = value !== "anyone";
  const summary = active ? (ASSIGNEE_OPTIONS.find((o) => o.value === value)?.label ?? null) : null;
  return (
    <PillShell
      label="Assignee"
      summary={summary}
      active={active}
      onClear={() => onChange("anyone")}
    >
      <div className="py-1">
        {ASSIGNEE_OPTIONS.map((o) => (
          <button
            key={o.value}
            type="button"
            onClick={() => onChange(o.value)}
            className={`w-full text-left px-3 py-1.5 text-sm cursor-pointer hover:bg-muted/50 ${
              value === o.value ? "font-medium" : ""
            }`}
          >
            {o.label}
          </button>
        ))}
      </div>
    </PillShell>
  );
}
