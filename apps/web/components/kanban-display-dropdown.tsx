'use client';

import { Button } from '@kandev/ui/button';
import { Checkbox } from '@kandev/ui/checkbox';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@kandev/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import { IconAdjustmentsHorizontal } from '@tabler/icons-react';
import { useKanbanDisplaySettings } from '@/hooks/use-kanban-display-settings';
import type { Workspace, Repository } from '@/lib/types/http';
import type { WorkflowsState } from '@/lib/state/slices';
import { Badge } from '@kandev/ui/badge';

function getRepositoryPlaceholder(repositoriesLoading: boolean, repositoriesEmpty: boolean): string {
  if (repositoriesLoading) return 'Loading repositories...';
  if (repositoriesEmpty) return 'No repositories';
  return 'Select repository';
}

function WorkspaceSection({ activeWorkspaceId, workspaces, onWorkspaceChange }: {
  activeWorkspaceId: string | null;
  workspaces: Workspace[];
  onWorkspaceChange: (id: string | null) => void;
}) {
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">Workspace</DropdownMenuLabel>
      <Select
        value={activeWorkspaceId ?? ''}
        onValueChange={(value) => onWorkspaceChange(value || null)}
      >
        <SelectTrigger className="w-full border-border">
          <SelectValue placeholder="Select workspace" />
        </SelectTrigger>
        <SelectContent>
          {workspaces.map((workspace: Workspace) => (
            <SelectItem key={workspace.id} value={workspace.id}>
              {workspace.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function WorkflowSection({ activeWorkflowId, workflows, onWorkflowChange }: {
  activeWorkflowId: string | null;
  workflows: WorkflowsState['items'];
  onWorkflowChange: (id: string | null) => void;
}) {
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">Workflow</DropdownMenuLabel>
      <Select
        value={activeWorkflowId ?? 'all'}
        onValueChange={(value) => onWorkflowChange(value === 'all' ? null : value)}
      >
        <SelectTrigger className="w-full border-border">
          <SelectValue placeholder="Select workflow" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Workflows</SelectItem>
          {workflows.map((workflow: WorkflowsState['items'][number]) => (
            <SelectItem key={workflow.id} value={workflow.id}>
              {workflow.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function RepositorySection({ repositoryValue, repositories, repositoriesLoading, onRepositoryChange }: {
  repositoryValue: string;
  repositories: Repository[];
  repositoriesLoading: boolean;
  onRepositoryChange: (value: string | 'all') => void;
}) {
  return (
    <div className="space-y-1.5">
      <DropdownMenuLabel className="px-0 text-foreground">Repository</DropdownMenuLabel>
      <Select
        value={repositoryValue}
        onValueChange={(value) => onRepositoryChange(value as string | 'all')}
        disabled={repositories.length === 0}
      >
        <SelectTrigger className="w-full border-border">
          <SelectValue placeholder={getRepositoryPlaceholder(repositoriesLoading, repositories.length === 0)} />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All repositories</SelectItem>
          {repositories.map((repo: Repository) => (
            <SelectItem key={repo.id} value={repo.id}>
              {repo.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

export function KanbanDisplayDropdown() {
  const {
    workspaces,
    workflows,
    activeWorkspaceId,
    activeWorkflowId,
    repositories,
    repositoriesLoading,
    allRepositoriesSelected,
    selectedRepositoryId,
    enablePreviewOnClick,
    onWorkspaceChange,
    onWorkflowChange,
    onRepositoryChange,
    onTogglePreviewOnClick,
  } = useKanbanDisplaySettings();

  const repositoryValue = allRepositoriesSelected ? 'all' : selectedRepositoryId ?? 'all';

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="outline" className="cursor-pointer">
          <IconAdjustmentsHorizontal className="h-4 w-4 mr-2" />
          Display
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-[280px] p-3">
        <div className="space-y-3">
          <WorkspaceSection activeWorkspaceId={activeWorkspaceId} workspaces={workspaces} onWorkspaceChange={onWorkspaceChange} />
          <DropdownMenuSeparator />
          <WorkflowSection activeWorkflowId={activeWorkflowId} workflows={workflows} onWorkflowChange={onWorkflowChange} />
          <DropdownMenuSeparator />
          <RepositorySection repositoryValue={repositoryValue} repositories={repositories} repositoriesLoading={repositoriesLoading} onRepositoryChange={onRepositoryChange} />
          <DropdownMenuSeparator />
          <div className="space-y-1.5">
            <DropdownMenuLabel className="px-0 text-foreground">Preview Panel</DropdownMenuLabel>
            <label className="flex items-center gap-2 cursor-pointer">
              <Checkbox
                checked={enablePreviewOnClick ?? false}
                onCheckedChange={(checked) => {
                  onTogglePreviewOnClick?.(!!checked);
                }}
              />
              <span className="text-sm text-foreground">
                Open preview on click {" "}
                <Badge variant="secondary" className="mr-1">beta</Badge>
              </span>
            </label>
            <p className="text-xs text-muted-foreground pl-6">
              When enabled, clicking a task opens the preview panel. When disabled, clicking navigates directly to the session.
            </p>
          </div>
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
