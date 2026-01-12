'use client';

import { useEffect, useRef, useState, FormEvent } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogClose,
} from '@kandev/ui/dialog';
import { Label } from '@kandev/ui/label';
import { Input } from '@kandev/ui/input';
import { Textarea } from '@kandev/ui/textarea';
import { Button } from '@kandev/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@kandev/ui/select';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@kandev/ui/tooltip';
import type { Repository, Task } from '@/lib/types/http';
import { getBackendConfig } from '@/lib/config';
import { createTask, listRepositories, listRepositoryBranches, updateTask } from '@/lib/http/client';

interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string | null;
  boardId: string | null;
  defaultColumnId: string | null;
  columns: Array<{ id: string; title: string }>;
  editingTask?: { id: string; title: string; description?: string; columnId: string; state?: Task['state'] } | null;
  onSuccess?: (task: Task, mode: 'create' | 'edit') => void;
  initialValues?: {
    title: string;
    description?: string;
    repositoryId?: string;
    branch?: string;
    state?: Task['state'];
  };
  submitLabel?: string;
}

export function TaskCreateDialog({
  open,
  onOpenChange,
  workspaceId,
  boardId,
  defaultColumnId,
  columns,
  editingTask,
  onSuccess,
  initialValues,
  submitLabel = 'Create',
}: TaskCreateDialogProps) {
  const isEditMode = submitLabel !== 'Create';
  const [title, setTitle] = useState(initialValues?.title ?? '');
  const [description, setDescription] = useState(initialValues?.description ?? '');
  const [repositoryId, setRepositoryId] = useState(initialValues?.repositoryId ?? '');
  const [branch, setBranch] = useState(initialValues?.branch ?? '');
  const [taskState, setTaskState] = useState<Task['state']>(initialValues?.state ?? 'CREATED');
  const [startAgent, setStartAgent] = useState(false);
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repositoriesLoading, setRepositoriesLoading] = useState(false);
  const [branchesByRepo, setBranchesByRepo] = useState<Record<string, string[]>>({});
  const [branchesLoading, setBranchesLoading] = useState(false);
  const descriptionRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    if (!open) return;
    setTitle(initialValues?.title ?? '');
    setDescription(initialValues?.description ?? '');
    setRepositoryId(initialValues?.repositoryId ?? '');
    setBranch(initialValues?.branch ?? '');
    setTaskState(initialValues?.state ?? editingTask?.state ?? 'CREATED');
    setStartAgent(false);
  }, [
    editingTask?.state,
    initialValues?.branch,
    initialValues?.description,
    initialValues?.repositoryId,
    initialValues?.state,
    initialValues?.title,
    open,
  ]);

  useEffect(() => {
    if (!open || !workspaceId) return;
    setRepositoriesLoading(true);
    listRepositories(getBackendConfig().apiBaseUrl, workspaceId)
      .then((response) => {
        setRepositories(response.repositories);
        if (editingTask?.description && !description) {
          setDescription(editingTask.description);
        }
      })
      .catch(() => {
        setRepositories([]);
      })
      .finally(() => {
        setRepositoriesLoading(false);
      });
  }, [description, editingTask?.description, open, workspaceId]);

  useEffect(() => {
    if (!repositoryId) return;
    if (branchesByRepo[repositoryId]) return;
    setBranchesLoading(true);
    listRepositoryBranches(getBackendConfig().apiBaseUrl, repositoryId)
      .then((response) => {
        setBranchesByRepo((prev) => ({ ...prev, [repositoryId]: response.branches }));
      })
      .catch(() => {
        setBranchesByRepo((prev) => ({ ...prev, [repositoryId]: [] }));
      })
      .finally(() => {
        setBranchesLoading(false);
      });
  }, [branchesByRepo, repositoryId]);

  useEffect(() => {
    const textarea = descriptionRef.current;
    if (!textarea) return;
    textarea.style.height = 'auto';
    textarea.style.height = `${textarea.scrollHeight}px`;
  }, [description]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    const trimmedTitle = title.trim();
    if (!trimmedTitle) return;
    if (isEditMode && editingTask) {
      try {
        const updatedTask = await updateTask(getBackendConfig().apiBaseUrl, editingTask.id, {
          title: trimmedTitle,
          description: description.trim(),
          state: taskState,
        });
        onSuccess?.(updatedTask, 'edit');
      } finally {
        setTitle('');
        setDescription('');
        setRepositoryId('');
        setBranch('');
        setTaskState('CREATED');
        setStartAgent(false);
        onOpenChange(false);
      }
      return;
    }
    if (!workspaceId || !boardId) return;
    const columnId = editingTask?.columnId ?? defaultColumnId;
    if (!columnId) return;
    let targetColumnId = columnId;
    let targetState: Task['state'] | undefined = taskState ?? 'CREATED';
    if (startAgent && !isEditMode) {
      const progressColumn = columns.find((column) =>
        column.title.toLowerCase().includes('progress')
      );
      targetColumnId = progressColumn?.id ?? columnId;
      targetState = 'IN_PROGRESS';
    }
    const repository = repositories.find((repo) => repo.id === repositoryId);
    try {
      const task = await createTask(getBackendConfig().apiBaseUrl, {
        workspace_id: workspaceId,
        board_id: boardId,
        column_id: targetColumnId,
        title: trimmedTitle,
        description: description.trim(),
        repository_url: repository?.local_path ?? '',
        branch: branch || '',
        agent_type: startAgent ? 'default' : '',
        state: targetState,
      });
      onSuccess?.(task, 'create');
    } finally {
      setTitle('');
      setDescription('');
      setRepositoryId('');
      setBranch('');
      setTaskState('CREATED');
      setStartAgent(false);
      onOpenChange(false);
    }
  };

  const handleCancel = () => {
    setTitle('');
    setDescription('');
    setRepositoryId('');
    setBranch('');
    setTaskState('CREATED');
    setStartAgent(false);
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-none w-[900px] sm:!max-w-none">
        <DialogHeader>
          <DialogTitle>{submitLabel === 'Create' ? 'Create Task' : 'Edit Task'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label>Title</Label>
            <Input
              autoFocus
              required
              placeholder="Enter task title..."
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label>Prompt</Label>
            <Textarea
              ref={descriptionRef}
              placeholder="Write a prompt for the agent..."
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              className="min-h-[96px] resize-none"
            />
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <div className="space-y-2">
              <Label>Repository</Label>
              <Select
                value={repositoryId}
                onValueChange={(value) => {
                  setRepositoryId(value);
                  setBranch('');
                }}
                disabled={isEditMode}
              >
                <SelectTrigger className="w-full">
                  <SelectValue placeholder={repositoriesLoading ? 'Loading repositories...' : 'Select repository'} />
                </SelectTrigger>
                <SelectContent>
                  {repositories.map((repo) => (
                    <SelectItem key={repo.id} value={repo.id}>
                      {repo.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label>Base Branch</Label>
              <Select value={branch} onValueChange={setBranch} disabled={isEditMode || !repositoryId}>
                <SelectTrigger className="w-full">
                  <SelectValue
                    placeholder={
                      !repositoryId
                        ? 'Select repository first'
                        : branchesLoading
                          ? 'Loading branches...'
                          : 'Select branch'
                    }
                  />
                </SelectTrigger>
                <SelectContent>
                  {(branchesByRepo[repositoryId] ?? []).map((branchName) => (
                    <SelectItem key={branchName} value={branchName}>
                      {branchName}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                A worktree will be created from this base branch.
              </p>
            </div>
          </div>
          <div className="space-y-2">
            <Label>State</Label>
            <Select value={taskState} onValueChange={(value) => setTaskState(value as Task['state'])}>
              <SelectTrigger className="w-full">
                <SelectValue placeholder="Select state" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="CREATED">Created</SelectItem>
                <SelectItem value="SCHEDULING">Scheduling</SelectItem>
                <SelectItem value="TODO">Todo</SelectItem>
                <SelectItem value="IN_PROGRESS">In Progress</SelectItem>
                <SelectItem value="REVIEW">Review</SelectItem>
                <SelectItem value="BLOCKED">Blocked</SelectItem>
                <SelectItem value="WAITING_FOR_INPUT">Waiting for input</SelectItem>
                <SelectItem value="COMPLETED">Completed</SelectItem>
                <SelectItem value="FAILED">Failed</SelectItem>
                <SelectItem value="CANCELLED">Cancelled</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <input
              id="start-agent"
              type="checkbox"
              checked={startAgent}
              onChange={(e) => setStartAgent(e.target.checked)}
              disabled={isEditMode}
              className="h-4 w-4 rounded border border-input text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            />
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Label htmlFor="start-agent" className="cursor-pointer">
                    Start Agent
                  </Label>
                </TooltipTrigger>
                <TooltipContent>Start the agent on task creation.</TooltipContent>
              </Tooltip>
            </TooltipProvider>
          </div>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline" onClick={handleCancel}>
                Cancel
              </Button>
            </DialogClose>
            <Button type="submit">{submitLabel}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
