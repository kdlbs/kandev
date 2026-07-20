"use client";

import { useEffect, useMemo, useState } from "react";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { createTaskMR } from "@/lib/api/domains/gitlab-api";
import type { Repository } from "@/lib/types/http";

const NO_REPOSITORY = "__none__";

type TaskRepositoryLink = { repository_id: string };
type RepositoryOption = { id: string; label: string };

function useRepositoryOptions(
  repositories: Repository[],
  taskRepositories: TaskRepositoryLink[],
): RepositoryOption[] {
  return useMemo(() => {
    const repositoryByID = new Map<string, Repository>(
      repositories.map((repository) => [repository.id, repository]),
    );
    return taskRepositories.map((link) => {
      const repository = repositoryByID.get(link.repository_id);
      const providerLabel =
        repository?.provider_owner && repository.provider_name
          ? `${repository.provider_owner}/${repository.provider_name}`
          : repository?.name;
      return { id: link.repository_id, label: providerLabel || link.repository_id };
    });
  }, [repositories, taskRepositories]);
}

function RepositoryField({
  options,
  value,
  onChange,
}: {
  options: RepositoryOption[];
  value: string;
  onChange: (value: string) => void;
}) {
  if (options.length <= 1) return null;
  return (
    <div className="space-y-2">
      <Label htmlFor="gitlab-mr-repository">Task repository</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id="gitlab-mr-repository">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((repository) => (
            <SelectItem key={repository.id} value={repository.id}>
              {repository.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function LinkDialogFields({
  mrURL,
  onMRURLChange,
  options,
  repositoryID,
  onRepositoryChange,
}: {
  mrURL: string;
  onMRURLChange: (value: string) => void;
  options: RepositoryOption[];
  repositoryID: string;
  onRepositoryChange: (value: string) => void;
}) {
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label htmlFor="gitlab-mr-url">Merge request URL</Label>
        <Input
          id="gitlab-mr-url"
          value={mrURL}
          onChange={(event) => onMRURLChange(event.target.value)}
          placeholder="https://gitlab.example.com/group/project/-/merge_requests/42"
          autoComplete="off"
        />
      </div>
      <RepositoryField options={options} value={repositoryID} onChange={onRepositoryChange} />
    </div>
  );
}

export function TaskMRLinkDialog({
  open,
  onOpenChange,
  taskId,
  workspaceId,
  taskRepositories,
  repositories,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  workspaceId: string;
  taskRepositories: TaskRepositoryLink[];
  repositories: Repository[];
}) {
  const [mrURL, setMRURL] = useState("");
  const [repositoryID, setRepositoryID] = useState(NO_REPOSITORY);
  const [submitting, setSubmitting] = useState(false);
  const setTaskMR = useAppStore((state) => state.setTaskMR);
  const { toast } = useToast();
  const options = useRepositoryOptions(repositories, taskRepositories);
  const defaultRepositoryID = options[0]?.id ?? NO_REPOSITORY;

  useEffect(() => {
    if (open) return;
    setMRURL("");
    setRepositoryID(defaultRepositoryID);
  }, [defaultRepositoryID, open]);

  const submit = async () => {
    if (!mrURL.trim() || submitting) return;
    setSubmitting(true);
    try {
      const association = await createTaskMR(
        {
          task_id: taskId,
          repository_id: repositoryID === NO_REPOSITORY ? undefined : repositoryID,
          mr_url: mrURL.trim(),
        },
        workspaceId,
      );
      setTaskMR(workspaceId, taskId, association);
      onOpenChange(false);
    } catch (error) {
      toast({
        title: "Failed to link merge request",
        description:
          error instanceof Error ? error.message : "GitLab rejected the merge request link.",
        variant: "error",
      });
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Link GitLab merge request</DialogTitle>
          <DialogDescription>
            Paste a merge request URL from this workspace&apos;s configured GitLab host.
          </DialogDescription>
        </DialogHeader>
        <LinkDialogFields
          mrURL={mrURL}
          onMRURLChange={setMRURL}
          options={options}
          repositoryID={repositoryID}
          onRepositoryChange={setRepositoryID}
        />
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            className="cursor-pointer"
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={() => void submit()}
            disabled={!mrURL.trim() || submitting}
            className="cursor-pointer"
          >
            {submitting ? "Linking…" : "Link merge request"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
