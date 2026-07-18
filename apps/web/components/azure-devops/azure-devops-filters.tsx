"use client";

import { IconSearch } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import type { AzureDevOpsProject, AzureDevOpsRepository } from "@/lib/types/azure-devops";

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
};

function ProjectFilter({
  idSuffix,
  value,
  projects,
  onChange,
}: {
  idSuffix: FiltersProps["idSuffix"];
  value: string;
  projects: AzureDevOpsProject[];
  onChange: (value: string) => void;
}) {
  const controlId = `azure-devops-filter-project${idSuffix}`;
  return (
    <div className="space-y-1.5">
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
    </div>
  );
}

function PullRequestFilters({
  idSuffix,
  filters,
  repositories,
  onChange,
}: Pick<FiltersProps, "idSuffix" | "filters" | "repositories" | "onChange">) {
  return (
    <>
      <div className="space-y-1.5">
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
      </div>
      <div className="space-y-1.5">
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
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`azure-devops-filter-creator${idSuffix}`}>Creator ID</Label>
        <Input
          id={`azure-devops-filter-creator${idSuffix}`}
          value={filters.creator}
          onChange={(event) => onChange("creator", event.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`azure-devops-filter-reviewer${idSuffix}`}>Reviewer ID</Label>
        <Input
          id={`azure-devops-filter-reviewer${idSuffix}`}
          value={filters.reviewer}
          onChange={(event) => onChange("reviewer", event.target.value)}
        />
      </div>
    </>
  );
}

function WorkItemFilters({
  idSuffix,
  filters,
  onChange,
}: Pick<FiltersProps, "idSuffix" | "filters" | "onChange">) {
  return (
    <>
      <div className="space-y-1.5">
        <Label htmlFor={`azure-devops-filter-wiql${idSuffix}`}>WIQL</Label>
        <Textarea
          id={`azure-devops-filter-wiql${idSuffix}`}
          value={filters.wiql}
          onChange={(event) => onChange("wiql", event.target.value)}
          className="min-h-32 resize-y font-mono text-xs"
        />
      </div>
      <div className="space-y-1.5">
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
      </div>
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
}: FiltersProps) {
  const disabled =
    loading ||
    !filters.projectId ||
    (mode === "work-items" ? !filters.wiql.trim() : !filters.repositoryId);
  return (
    <div className="space-y-4" data-testid={`azure-devops-filters${idSuffix}`}>
      <ProjectFilter
        idSuffix={idSuffix}
        value={filters.projectId}
        projects={projects}
        onChange={(value) => onChange("projectId", value)}
      />
      {mode === "work-items" ? (
        <WorkItemFilters idSuffix={idSuffix} filters={filters} onChange={onChange} />
      ) : (
        <PullRequestFilters
          idSuffix={idSuffix}
          filters={filters}
          repositories={repositories}
          onChange={onChange}
        />
      )}
      <Button
        type="button"
        onClick={onSearch}
        disabled={disabled}
        className="w-full cursor-pointer"
        data-testid={`azure-devops-search-button${idSuffix}`}
      >
        <IconSearch className="h-4 w-4" />
        {loading ? "Loading..." : "Search"}
      </Button>
    </div>
  );
}
