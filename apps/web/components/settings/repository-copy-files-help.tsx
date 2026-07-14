"use client";

import { IconChevronDown, IconInfoCircle } from "@tabler/icons-react";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type { Repository } from "@/lib/types/http";

type CopyFilesFieldProps = {
  repositoryId: string;
  copyFiles: string;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

export function CopyFilesField({ repositoryId, copyFiles, onUpdate }: CopyFilesFieldProps) {
  const inputId = `copy-files-${repositoryId}`;
  const helpId = `copy-files-help-${repositoryId}`;
  return (
    <div className="space-y-2">
      <Label htmlFor={inputId}>Copy Files</Label>
      <Textarea
        id={inputId}
        data-testid={`copy-files-input-${repositoryId}`}
        aria-describedby={helpId}
        value={copyFiles}
        onChange={(e) => onUpdate(repositoryId, { copy_files: e.target.value })}
        placeholder=".env, .env.*, apps/**/.env, .env.local:symlink"
        rows={2}
        className="font-mono text-sm"
      />
      <p id={helpId} className="text-xs text-muted-foreground">
        Gitignored paths copied into new worktrees. Append{" "}
        <code className="px-1 py-0.5 bg-muted rounded">:symlink</code> to an entry to link it back
        to the main repo. Use <code className="px-1 py-0.5 bg-muted rounded">::symlink</code> for a
        literal filename ending in <code className="px-1 py-0.5 bg-muted rounded">:symlink</code>.
      </p>
      <p data-testid="copy-files-remote-fallback" className="text-xs text-muted-foreground">
        Remote executors copy file contents instead of creating symlinks.
      </p>
      <CopyFilesDetails />
    </div>
  );
}

function CopyFilesDetails() {
  return (
    <details className="group text-xs text-muted-foreground">
      <summary className="flex min-h-11 w-fit cursor-pointer list-none items-center gap-1.5 py-2 font-medium text-foreground">
        <IconInfoCircle className="h-4 w-4 shrink-0" aria-hidden="true" />
        Pattern syntax
        <IconChevronDown
          className="h-4 w-4 shrink-0 transition-transform group-open:rotate-180"
          aria-hidden="true"
        />
      </summary>
      <div className="max-w-sm space-y-2 pb-1">
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
            <code className="px-1 py-0.5 bg-muted rounded">{"{a,b}"}</code> brace alternation, e.g.{" "}
            <code className="px-1 py-0.5 bg-muted rounded">.env{"{,.local}"}</code>
          </li>
        </ul>
        <p className="text-muted-foreground">
          Files over 5 MiB are skipped when copying to remote executors. Local worktrees copy them
          without a size cap.
        </p>
      </div>
    </details>
  );
}
