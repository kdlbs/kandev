"use client";

import { useCallback, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { IconDownload, IconUpload } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@kandev/ui/dialog";
import { useAppStore } from "@/components/state-provider";
import * as orchestrateApi from "@/lib/api/domains/orchestrate-api";

type ImportDiff = { created: string[]; updated: string[]; deleted: string[] };
type ImportPreview = {
  agents: ImportDiff;
  skills: ImportDiff;
  routines: ImportDiff;
  projects: ImportDiff;
};

export function ConfigSection() {
  const activeWorkspaceId = useAppStore((s) => s.workspaces?.activeId ?? "");
  const router = useRouter();
  const [importDialogOpen, setImportDialogOpen] = useState(false);
  const [preview, setPreview] = useState<ImportPreview | null>(null);
  const [bundle, setBundle] = useState<Record<string, unknown> | null>(null);
  const [applying, setApplying] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const handleExport = useCallback(() => {
    router.push("/orchestrate/company/settings/export");
  }, [router]);

  const handleFileSelect = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file || !activeWorkspaceId) return;
      try {
        const text = await file.text();
        const parsed = JSON.parse(text);
        setBundle(parsed);
        const res = await orchestrateApi.previewImport(activeWorkspaceId, parsed);
        setPreview(res.preview);
        setImportDialogOpen(true);
      } catch {
        /* ignore parse errors */
      }
    },
    [activeWorkspaceId],
  );

  const handleApply = useCallback(async () => {
    if (!activeWorkspaceId || !bundle) return;
    setApplying(true);
    try {
      await orchestrateApi.applyImport(activeWorkspaceId, bundle);
      setImportDialogOpen(false);
      setPreview(null);
      setBundle(null);
    } catch {
      /* ignore */
    } finally {
      setApplying(false);
    }
  }, [activeWorkspaceId, bundle]);

  return (
    <section className="space-y-4">
      <h2 className="text-sm font-semibold">Configuration</h2>
      <div className="flex gap-2">
        <Button variant="outline" onClick={handleExport} className="cursor-pointer">
          <IconDownload className="h-4 w-4 mr-1" />
          Export
        </Button>
        <Button
          variant="outline"
          onClick={() => fileRef.current?.click()}
          className="cursor-pointer"
        >
          <IconUpload className="h-4 w-4 mr-1" />
          Import
        </Button>
        <input
          ref={fileRef}
          type="file"
          accept=".json,.zip"
          className="hidden"
          onChange={handleFileSelect}
        />
      </div>

      <ImportPreviewDialog
        open={importDialogOpen}
        onOpenChange={setImportDialogOpen}
        preview={preview}
        applying={applying}
        onApply={handleApply}
      />
    </section>
  );
}

function ImportPreviewDialog({
  open,
  onOpenChange,
  preview,
  applying,
  onApply,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  preview: ImportPreview | null;
  applying: boolean;
  onApply: () => void;
}) {
  if (!preview) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Import Preview</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <DiffSection label="Agents" diff={preview.agents} />
          <DiffSection label="Skills" diff={preview.skills} />
          <DiffSection label="Routines" diff={preview.routines} />
          <DiffSection label="Projects" diff={preview.projects} />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} className="cursor-pointer">
            Cancel
          </Button>
          <Button onClick={onApply} disabled={applying} className="cursor-pointer">
            {applying ? "Applying..." : "Apply"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DiffSection({ label, diff }: { label: string; diff: ImportDiff }) {
  const total =
    (diff.created?.length ?? 0) + (diff.updated?.length ?? 0) + (diff.deleted?.length ?? 0);
  if (total === 0) return null;

  return (
    <div>
      <p className="text-sm font-medium mb-1">{label}</p>
      <div className="flex flex-wrap gap-1">
        {diff.created?.map((name) => (
          <Badge
            key={name}
            className="bg-green-100 text-green-700 dark:bg-green-900/50 dark:text-green-300"
          >
            + {name}
          </Badge>
        ))}
        {diff.updated?.map((name) => (
          <Badge
            key={name}
            className="bg-yellow-100 text-yellow-700 dark:bg-yellow-900/50 dark:text-yellow-300"
          >
            ~ {name}
          </Badge>
        ))}
        {diff.deleted?.map((name) => (
          <Badge
            key={name}
            className="bg-red-100 text-red-700 dark:bg-red-900/50 dark:text-red-300"
          >
            - {name}
          </Badge>
        ))}
      </div>
    </div>
  );
}
