import { Badge } from "@kandev/ui/badge";
import type { GitLabMRFile } from "@/lib/types/gitlab";
import { CollapsibleSection } from "@/components/github/pr-shared";

export function MRFilesSection({ files }: { files: GitLabMRFile[] }) {
  return (
    <CollapsibleSection title="Files" count={files.length} defaultOpen={false}>
      {files.length === 0 && (
        <p className="px-2 py-2 text-xs text-muted-foreground">No changed files</p>
      )}
      {files.map((file) => (
        <article
          key={`${file.old_path ?? ""}:${file.filename}`}
          className="overflow-hidden rounded-md border"
        >
          <header className="flex min-w-0 items-center gap-2 bg-muted/40 px-2 py-2 text-xs">
            <Badge variant="outline" className="shrink-0 text-[10px] capitalize">
              {file.status}
            </Badge>
            <span className="min-w-0 flex-1 truncate font-mono" title={file.filename}>
              {file.filename}
            </span>
            <span className="shrink-0 text-green-600">+{file.additions}</span>
            <span className="shrink-0 text-red-600">-{file.deletions}</span>
          </header>
          {file.patch && (
            <pre className="max-h-72 overflow-auto p-2 text-[11px] leading-5">
              <code>{file.patch}</code>
            </pre>
          )}
        </article>
      ))}
    </CollapsibleSection>
  );
}
