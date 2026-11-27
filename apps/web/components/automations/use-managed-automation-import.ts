import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import type {
  CreateAutomationRequest,
  ManagedConversationDestination,
} from "@/lib/types/automation";
import {
  ManagedAutomationImportError,
  parseManagedAutomationImport,
  type ManagedAutomationImportEntry,
} from "./managed-automation-import";

const importErrorKey = {
  invalid_document: "automations:importInvalidDocument",
  no_managed_automations: "automations:importNoManagedSchedules",
  invalid_schedule: "automations:importInvalidSchedule",
} as const;

function createImportPayload(
  workspaceId: string,
  entry: ManagedAutomationImportEntry,
  destination: ManagedConversationDestination,
): CreateAutomationRequest {
  return {
    workspace_id: workspaceId,
    name: entry.name,
    description: entry.description,
    workflow_id: "",
    workflow_step_id: "",
    agent_profile_id: "",
    executor_profile_id: "",
    repository_ids: [],
    repositories: [],
    prompt: entry.prompt,
    task_title_template: "",
    max_concurrent_runs: 1,
    continuation_policy: "new_task",
    task_mode: "managed_conversation",
    managed_destination: destination,
    repository_mode: "none",
    enabled: entry.enabled,
    triggers: [entry.trigger],
  };
}

export function useManagedAutomationImport(
  workspaceId: string,
  create: (payload: CreateAutomationRequest) => Promise<unknown>,
) {
  const { t } = useTranslation();
  const fileInput = useRef<HTMLInputElement>(null);
  const [entries, setEntries] = useState<ManagedAutomationImportEntry[]>([]);
  const [skippedCount, setSkippedCount] = useState(0);
  const [destinations, setDestinations] = useState<Record<number, ManagedConversationDestination>>(
    {},
  );
  const [imported, setImported] = useState<Set<number>>(() => new Set());
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const [importingIndex, setImportingIndex] = useState<number | null>(null);

  const reset = () => {
    setEntries([]);
    setSkippedCount(0);
    setDestinations({});
    setImported(new Set());
    setErrorKey(null);
    if (fileInput.current) fileInput.current.value = "";
  };

  const loadFile = async (file?: File) => {
    if (!file) return;
    reset();
    try {
      const result = parseManagedAutomationImport(await file.text());
      setEntries(result.entries);
      setSkippedCount(result.skippedCount);
    } catch (error) {
      setErrorKey(
        error instanceof ManagedAutomationImportError
          ? importErrorKey[error.code]
          : "automations:importInvalidDocument",
      );
    }
  };

  const importEntry = async (entry: ManagedAutomationImportEntry) => {
    const destination = destinations[entry.sourceIndex];
    if (!destination || importingIndex !== null) return;
    setImportingIndex(entry.sourceIndex);
    try {
      await create(createImportPayload(workspaceId, entry, destination));
      setImported((current) => new Set(current).add(entry.sourceIndex));
      toast.success(t("automations:importCreated", { name: entry.name }));
    } catch (error) {
      toast.error(t("automations:importFailed", { name: entry.name }), {
        description: error instanceof Error ? error.message : t("common:requestFailed"),
      });
    } finally {
      setImportingIndex(null);
    }
  };

  return {
    fileInput,
    entries,
    skippedCount,
    destinations,
    imported,
    errorKey,
    importingIndex,
    reset,
    loadFile,
    importEntry,
    setDestination: (index: number, value: ManagedConversationDestination) =>
      setDestinations((current) => ({ ...current, [index]: value })),
  };
}
