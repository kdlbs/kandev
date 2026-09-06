import { useRef, useState } from "react";
import { t as translate } from "@/lib/i18n";
import { exportAllWorkflowsAction, importWorkflowsAction } from "@/app/actions/workspaces";
import type { Workflow, Workspace } from "@/lib/types/http";

export function useWorkflowImportExport(
  workspace: Workspace | null,
  workflowItems: Workflow[],
  router: { refresh: () => void },
  toast: (options: { title: string; description?: string; variant?: "error" }) => void,
) {
  const [isExportDialogOpen, setIsExportDialogOpen] = useState(false);
  const [exportYaml, setExportYaml] = useState("");
  const [isImportDialogOpen, setIsImportDialogOpen] = useState(false);
  const [importYaml, setImportYaml] = useState("");
  const [importLoading, setImportLoading] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleExportAll = async () => {
    if (!workspace) return;
    try {
      // Export only the workflows shown in this settings view (kanban-only —
      // office workflows are filtered out upstream).
      // Workflow import/export is kanban-only by design (ADR-0004).
      const exportIds = workflowItems.map((wf) => wf.id);
      const yamlText = await exportAllWorkflowsAction(workspace.id, exportIds);
      setExportYaml(yamlText);
      setIsExportDialogOpen(true);
    } catch (error) {
      toast({
        title: translate("workflows:failedToExportWorkflows"),
        description: error instanceof Error ? error.message : translate("common:requestFailed"),
        variant: "error",
      });
    }
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (event) => {
      setImportYaml(event.target?.result as string);
    };
    reader.readAsText(file);
    e.target.value = "";
  };

  const handleImport = async () => {
    if (!workspace || !importYaml.trim()) return;
    setImportLoading(true);
    try {
      const result = await importWorkflowsAction(workspace.id, importYaml.trim());
      const created = result.created ?? [];
      const skipped = result.skipped ?? [];
      const parts: string[] = [];
      if (created.length > 0)
        parts.push(translate("workflows:importCreated", { names: created.join(", ") }));
      if (skipped.length > 0)
        parts.push(translate("workflows:importSkipped", { names: skipped.join(", ") }));
      toast({ title: translate("workflows:importCompleteTitle"), description: parts.join(". ") });
      setIsImportDialogOpen(false);
      setImportYaml("");
      if (created.length > 0) router.refresh();
    } catch (error) {
      toast({
        title: translate("workflows:failedToImportWorkflows"),
        description: error instanceof Error ? error.message : translate("workflows:invalidYaml"),
        variant: "error",
      });
    } finally {
      setImportLoading(false);
    }
  };

  return {
    isExportDialogOpen,
    setIsExportDialogOpen,
    exportYaml,
    isImportDialogOpen,
    setIsImportDialogOpen,
    importYaml,
    setImportYaml,
    importLoading,
    fileInputRef,
    handleExportAll,
    handleFileUpload,
    handleImport,
  };
}
