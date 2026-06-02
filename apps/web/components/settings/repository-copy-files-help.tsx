"use client";

import { IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import type { Repository } from "@/lib/types/http";

type CopyFilesFieldProps = {
  repositoryId: string;
  copyFiles: string;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

export function CopyFilesField({ repositoryId, copyFiles, onUpdate }: CopyFilesFieldProps) {
  return (
    <div className="space-y-2">
      <Label className="flex items-center gap-1.5">
        Copy Files <CopyFilesHelpTip />
      </Label>
      <Textarea
        value={copyFiles}
        onChange={(e) => onUpdate(repositoryId, { copy_files: e.target.value })}
        placeholder=".env, .env.*, apps/**/.env, config/{local,dev}.yml"
        rows={2}
        className="font-mono text-sm"
      />
      <p className="text-xs text-muted-foreground">Gitignored paths copied into new worktrees.</p>
    </div>
  );
}

function CopyFilesHelpTip() {
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <IconInfoCircle className="h-3.5 w-3.5 text-muted-foreground/50 hover:text-muted-foreground cursor-help shrink-0" />
        </TooltipTrigger>
        <TooltipContent className="max-w-sm space-y-2 text-xs">
          <p>
            Paths are resolved relative to the repository root and seeded into every new worktree,
            preserving their relative location. Existing files in the worktree are not overwritten.
          </p>
          <p className="font-medium">Supported patterns:</p>
          <ul className="space-y-1 pl-3 list-disc">
            <li>
              <code className="px-1 py-0.5 bg-muted rounded">.env</code> literal file or directory
              (directories copy recursively)
            </li>
            <li>
              <code className="px-1 py-0.5 bg-muted rounded">*</code>,{" "}
              <code className="px-1 py-0.5 bg-muted rounded">?</code>,{" "}
              <code className="px-1 py-0.5 bg-muted rounded">[abc]</code> single-segment wildcards
            </li>
            <li>
              <code className="px-1 py-0.5 bg-muted rounded">**</code> matches any number of
              directories, e.g. <code className="px-1 py-0.5 bg-muted rounded">**/.env</code>
            </li>
            <li>
              <code className="px-1 py-0.5 bg-muted rounded">{"{a,b}"}</code> brace alternation,
              e.g. <code className="px-1 py-0.5 bg-muted rounded">.env{"{,.local}"}</code>
            </li>
          </ul>
          <p className="text-muted-foreground">
            Files over 5 MiB are skipped when copying to remote executors. Local worktrees copy them
            without a size cap.
          </p>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
