"use client";

import { useMemo, useState } from "react";
import { IconChevronDown, IconX } from "@tabler/icons-react";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import type { JiraProject, JiraStatusCategory } from "@/lib/types/jira";
import { STATUS_CATEGORY_OPTIONS, type AssigneeFilter } from "./filter-model";

type PillShellProps = {
  label: string;
  summary: string | null;
  active: boolean;
  onClear?: () => void;
  children: React.ReactNode;
};

function PillShell({ label, summary, active, onClear, children }: PillShellProps) {
  const [open, setOpen] = useState(false);
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
  value: JiraStatusCategory[];
  onChange: (cats: JiraStatusCategory[]) => void;
};

export function StatusPill({ value, onChange }: StatusPillProps) {
  const selected = new Set(value);
  const toggle = (cat: JiraStatusCategory) => {
    if (selected.has(cat)) onChange(value.filter((c) => c !== cat));
    else onChange([...value, cat]);
  };
  const summary =
    value.length > 0
      ? joinSummary(
          value.map((c) => STATUS_CATEGORY_OPTIONS.find((o) => o.value === c)?.label ?? c),
          2,
        )
      : null;

  return (
    <PillShell
      label="Status"
      summary={summary}
      active={value.length > 0}
      onClear={() => onChange([])}
    >
      <div className="py-1">
        {STATUS_CATEGORY_OPTIONS.map((o) => (
          <label
            key={o.value}
            className="flex items-center gap-2 px-3 py-1.5 cursor-pointer hover:bg-muted/50"
          >
            <Checkbox checked={selected.has(o.value)} onCheckedChange={() => toggle(o.value)} />
            <span className="text-sm">{o.label}</span>
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
