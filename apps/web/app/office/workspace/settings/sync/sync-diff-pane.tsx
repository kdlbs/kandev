"use client";

import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import type { ImportDiff, SyncDiff } from "@/lib/api/domains/office-api";

type SyncDiffPaneProps = {
  title: string;
  description: string;
  icon: React.ReactNode;
  diff: SyncDiff | null;
  loading: boolean;
  applying: boolean;
  applyLabel: string;
  onApply: () => void;
};

export function SyncDiffPane({
  title,
  description,
  icon,
  diff,
  loading,
  applying,
  applyLabel,
  onApply,
}: SyncDiffPaneProps) {
  const totalChanges = diff ? countChanges(diff) : 0;

  return (
    <div className="rounded-lg border border-border overflow-hidden flex flex-col">
      <div className="px-4 py-3 border-b border-border bg-muted/30">
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-2 min-w-0">
            <div className="text-muted-foreground shrink-0">{icon}</div>
            <h2 className="text-sm font-medium truncate">{title}</h2>
          </div>
          <Button
            size="sm"
            onClick={onApply}
            disabled={applying || loading || totalChanges === 0}
            className="cursor-pointer shrink-0"
          >
            {applying ? "Applying..." : applyLabel}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground mt-1">{description}</p>
      </div>
      <div className="flex-1 p-4 space-y-4">
        <DiffBody diff={diff} loading={loading} totalChanges={totalChanges} />
        {diff?.errors && diff.errors.length > 0 && <ParseErrorsSection errors={diff.errors} />}
      </div>
    </div>
  );
}

function DiffBody({
  diff,
  loading,
  totalChanges,
}: {
  diff: SyncDiff | null;
  loading: boolean;
  totalChanges: number;
}) {
  if (loading && !diff) {
    return <p className="text-sm text-muted-foreground">Loading...</p>;
  }
  if (!diff || totalChanges === 0) {
    return <p className="text-sm text-muted-foreground">No changes detected.</p>;
  }
  return (
    <>
      <DiffSection label="Agents" diff={diff.preview.agents} />
      <DiffSection label="Skills" diff={diff.preview.skills} />
      <DiffSection label="Routines" diff={diff.preview.routines} />
      <DiffSection label="Projects" diff={diff.preview.projects} />
    </>
  );
}

function DiffSection({ label, diff }: { label: string; diff: ImportDiff }) {
  const total =
    (diff.created?.length ?? 0) + (diff.updated?.length ?? 0) + (diff.deleted?.length ?? 0);
  if (total === 0) return null;
  return (
    <div>
      <p className="text-xs font-medium text-muted-foreground mb-1.5 uppercase tracking-wide">
        {label}
      </p>
      <div className="flex flex-wrap gap-1">
        {diff.created?.map((name) => (
          <Badge
            key={`c-${name}`}
            className="bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300"
          >
            + {name}
          </Badge>
        ))}
        {diff.updated?.map((name) => (
          <Badge
            key={`u-${name}`}
            className="bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300"
          >
            ~ {name}
          </Badge>
        ))}
        {diff.deleted?.map((name) => (
          <Badge
            key={`d-${name}`}
            className="bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300"
          >
            − {name}
          </Badge>
        ))}
      </div>
    </div>
  );
}

function ParseErrorsSection({
  errors,
}: {
  errors: { workspace_id: string; file_path: string; error: string }[];
}) {
  return (
    <div>
      <p className="text-xs font-medium text-destructive mb-1.5 uppercase tracking-wide">
        Parse errors ({errors.length})
      </p>
      <ul className="space-y-1">
        {errors.map((err) => (
          <li key={err.file_path} className="text-xs">
            <code className="font-mono">{err.file_path}</code>
            <span className="text-muted-foreground"> — {err.error}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function countChanges(diff: SyncDiff): number {
  const sections = [
    diff.preview.agents,
    diff.preview.skills,
    diff.preview.routines,
    diff.preview.projects,
  ];
  return sections.reduce(
    (acc, s) =>
      acc + (s.created?.length ?? 0) + (s.updated?.length ?? 0) + (s.deleted?.length ?? 0),
    0,
  );
}
