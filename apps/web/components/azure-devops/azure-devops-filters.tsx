"use client";

import { useState } from "react";
import { IconAdjustments, IconChevronDown, IconSearch } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent } from "@kandev/ui/collapsible";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import type { AzureDevOpsProject, AzureDevOpsRepository } from "@/lib/types/azure-devops";
import { cn } from "@/lib/utils";

export type AzureDevOpsBrowseMode = "work-items" | "pull-requests";

export type AzureDevOpsFiltersState = {
  projectId: string;
  repositoryId: string;
  wiql: string;
  top: number;
  status: string;
  creator: string;
  reviewer: string;
};

type FiltersProps = {
  idSuffix: "" | "-mobile";
  mode: AzureDevOpsBrowseMode;
  filters: AzureDevOpsFiltersState;
  projects: AzureDevOpsProject[];
  repositories: AzureDevOpsRepository[];
  loading: boolean;
  onChange: <K extends keyof AzureDevOpsFiltersState>(
    key: K,
    value: AzureDevOpsFiltersState[K],
  ) => void;
  onSearch: () => void;
  compact?: boolean;
};

function Field({ compact, children }: { compact?: boolean; children: React.ReactNode }) {
  return <div className={cn("space-y-1.5", compact && "min-w-44 flex-1")}>{children}</div>;
}

function ProjectFilter({
  idSuffix,
  value,
  projects,
  onChange,
  compact,
}: {
  idSuffix: FiltersProps["idSuffix"];
  value: string;
  projects: AzureDevOpsProject[];
  onChange: (value: string) => void;
  compact?: boolean;
}) {
  const controlId = `azure-devops-filter-project${idSuffix}`;
  return (
    <Field compact={compact}>
      <Label htmlFor={controlId}>Project</Label>
      <Select value={value || undefined} onValueChange={onChange}>
        <SelectTrigger id={controlId} className="w-full">
          <SelectValue placeholder="Select project" />
        </SelectTrigger>
        <SelectContent>
          {projects.map((project) => (
            <SelectItem key={project.id} value={project.id}>
              {project.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}

function PullRequestFilters({
  idSuffix,
  filters,
  repositories,
  onChange,
  compact,
}: Pick<FiltersProps, "idSuffix" | "filters" | "repositories" | "onChange" | "compact">) {
  const [advanced, setAdvanced] = useState(false);
  return (
    <>
      <Field compact={compact}>
        <Label htmlFor={`azure-devops-filter-repository${idSuffix}`}>Repository</Label>
        <Select
          value={filters.repositoryId || undefined}
          onValueChange={(value) => onChange("repositoryId", value)}
          disabled={repositories.length === 0}
        >
          <SelectTrigger id={`azure-devops-filter-repository${idSuffix}`} className="w-full">
            <SelectValue placeholder="Select repository" />
          </SelectTrigger>
          <SelectContent>
            {repositories.map((repository) => (
              <SelectItem key={repository.id} value={repository.id}>
                {repository.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <Field compact={compact}>
        <Label htmlFor={`azure-devops-filter-status${idSuffix}`}>Status</Label>
        <Select value={filters.status} onValueChange={(value) => onChange("status", value)}>
          <SelectTrigger id={`azure-devops-filter-status${idSuffix}`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="active">Active</SelectItem>
            <SelectItem value="completed">Completed</SelectItem>
            <SelectItem value="abandoned">Abandoned</SelectItem>
            <SelectItem value="all">All</SelectItem>
          </SelectContent>
        </Select>
      </Field>
      <AdvancedFilters open={advanced} onOpenChange={setAdvanced} compact={compact}>
        <Field compact={compact}>
          <Label htmlFor={`azure-devops-filter-creator${idSuffix}`}>Creator ID</Label>
          <Input
            id={`azure-devops-filter-creator${idSuffix}`}
            value={filters.creator}
            onChange={(event) => onChange("creator", event.target.value)}
          />
        </Field>
        <Field compact={compact}>
          <Label htmlFor={`azure-devops-filter-reviewer${idSuffix}`}>Reviewer ID</Label>
          <Input
            id={`azure-devops-filter-reviewer${idSuffix}`}
            value={filters.reviewer}
            onChange={(event) => onChange("reviewer", event.target.value)}
          />
        </Field>
      </AdvancedFilters>
    </>
  );
}

function AdvancedFilters({
  open,
  onOpenChange,
  compact,
  children,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  compact?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Collapsible open={open} onOpenChange={onOpenChange} className={cn(compact && "contents")}>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="cursor-pointer self-end"
        onClick={() => onOpenChange(!open)}
        aria-expanded={open}
      >
        <IconAdjustments className="h-4 w-4" />
        Advanced
        <IconChevronDown className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-180")} />
      </Button>
      <CollapsibleContent className={cn(compact && "basis-full")}>
        <div className={cn("space-y-4 pt-3", compact && "flex flex-wrap gap-3 space-y-0")}>
          {children}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function WorkItemFilters({
  idSuffix,
  filters,
  onChange,
  compact,
}: Pick<FiltersProps, "idSuffix" | "filters" | "onChange" | "compact">) {
  const [advanced, setAdvanced] = useState(false);
  return (
    <>
      <Field compact={compact}>
        <Label htmlFor={`azure-devops-filter-limit${idSuffix}`}>Result limit</Label>
        <Select
          value={String(filters.top)}
          onValueChange={(value) => onChange("top", Number(value))}
        >
          <SelectTrigger id={`azure-devops-filter-limit${idSuffix}`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {[25, 50, 100, 200].map((value) => (
              <SelectItem key={value} value={String(value)}>
                {value}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Field>
      <AdvancedFilters open={advanced} onOpenChange={setAdvanced} compact={compact}>
        <div className="min-w-0 flex-1 space-y-1.5">
          <Label htmlFor={`azure-devops-filter-wiql${idSuffix}`}>WIQL</Label>
          <Textarea
            id={`azure-devops-filter-wiql${idSuffix}`}
            value={filters.wiql}
            onChange={(event) => onChange("wiql", event.target.value)}
            className="min-h-24 resize-y font-mono text-xs"
          />
        </div>
      </AdvancedFilters>
    </>
  );
}

export function AzureDevOpsFilters({
  idSuffix,
  mode,
  filters,
  projects,
  repositories,
  loading,
  onChange,
  onSearch,
  compact,
}: FiltersProps) {
  const disabled =
    loading ||
    !filters.projectId ||
    (mode === "work-items" ? !filters.wiql.trim() : !filters.repositoryId);
  return (
    <div
      className={cn("space-y-4", compact && "flex flex-wrap items-end gap-3 space-y-0")}
      data-testid={`azure-devops-filters${idSuffix}`}
    >
      <ProjectFilter
        idSuffix={idSuffix}
        value={filters.projectId}
        projects={projects}
        onChange={(value) => onChange("projectId", value)}
        compact={compact}
      />
      {mode === "work-items" ? (
        <WorkItemFilters
          idSuffix={idSuffix}
          filters={filters}
          onChange={onChange}
          compact={compact}
        />
      ) : (
        <PullRequestFilters
          idSuffix={idSuffix}
          filters={filters}
          repositories={repositories}
          onChange={onChange}
          compact={compact}
        />
      )}
      <Button
        type="button"
        onClick={onSearch}
        disabled={disabled}
        className={cn("w-full cursor-pointer", compact && "w-auto self-end")}
        data-testid={`azure-devops-search-button${idSuffix}`}
      >
        <IconSearch className="h-4 w-4" />
        {loading ? "Loading..." : "Search"}
      </Button>
    </div>
  );
}
