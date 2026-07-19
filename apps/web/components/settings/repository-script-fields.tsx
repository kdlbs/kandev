"use client";

import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { CopyFilesField } from "@/components/settings/repository-copy-files-help";
import { RepositoryStartupPromptField } from "@/components/settings/repository-startup-prompt-field";
import type { Repository } from "@/lib/types/http";

type RepositoryScriptFieldsProps = {
  repositoryId: string;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
  setupScript: string;
  cleanupScript: string;
  devScript: string;
  copyFiles: string;
  startupPrompt: string;
};

export function RepositoryScriptFields({
  repositoryId,
  onUpdate,
  setupScript,
  cleanupScript,
  devScript,
  copyFiles,
  startupPrompt,
}: RepositoryScriptFieldsProps) {
  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label>Setup Script</Label>
          <Textarea
            value={setupScript}
            onChange={(e) => onUpdate(repositoryId, { setup_script: e.target.value })}
            placeholder="#!/bin/bash&#10;# any manual setup you need"
            rows={3}
            className="font-mono text-sm"
          />
          <p className="text-xs text-muted-foreground">
            Runs when the repo is cloned or a git worktree is created.
          </p>
        </div>
        <div className="space-y-2">
          <Label>Cleanup Script</Label>
          <Textarea
            value={cleanupScript}
            onChange={(e) => onUpdate(repositoryId, { cleanup_script: e.target.value })}
            placeholder="#!/bin/bash&#10;# any manual clean up you need"
            rows={3}
            className="font-mono text-sm"
          />
          <p className="text-xs text-muted-foreground">
            Runs when the task is completed to clean up the workspace.
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <Label>Dev Script</Label>
        <Textarea
          value={devScript}
          onChange={(e) => onUpdate(repositoryId, { dev_script: e.target.value })}
          placeholder="#!/bin/bash&#10;npm run dev -- --port $PORT"
          rows={3}
          className="font-mono text-sm"
        />
        <p className="text-xs text-muted-foreground">
          Used to start the preview dev server for this repository. Use{" "}
          <code className="px-1 py-0.5 bg-muted rounded">$PORT</code> for automatic port allocation.
        </p>
      </div>

      <RepositoryStartupPromptField
        repositoryId={repositoryId}
        startupPrompt={startupPrompt}
        onUpdate={onUpdate}
      />

      <CopyFilesField repositoryId={repositoryId} copyFiles={copyFiles} onUpdate={onUpdate} />
    </>
  );
}
