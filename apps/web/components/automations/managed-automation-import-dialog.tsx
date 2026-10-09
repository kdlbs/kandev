"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconUpload } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@kandev/ui/dialog";
import type {
  CreateAutomationRequest,
  ManagedConversationDestination,
} from "@/lib/types/automation";
import { ManagedConversationDestinationSelector } from "./managed-conversation-destination-selector";
import type { ManagedAutomationImportEntry } from "./managed-automation-import";
import { useManagedAutomationImport } from "./use-managed-automation-import";

type ImportState = ReturnType<typeof useManagedAutomationImport>;

type EntryCardProps = {
  entry: ManagedAutomationImportEntry;
  workspaceId: string;
  destination?: ManagedConversationDestination;
  imported: boolean;
  importing: boolean;
  anyImporting: boolean;
  onDestinationChange: (value: ManagedConversationDestination) => void;
  onImport: () => void;
};

function ManagedAutomationImportEntryCard({
  entry,
  workspaceId,
  destination,
  imported,
  importing,
  anyImporting,
  onDestinationChange,
  onImport,
}: EntryCardProps) {
  const { t } = useTranslation();
  return (
    <section
      className="space-y-3 rounded-md border p-3"
      data-testid={`managed-automation-import-entry-${entry.sourceIndex}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="font-medium">{entry.name}</h3>
        {imported ? <Badge>{t("automations:importStatusImported")}</Badge> : null}
      </div>
      <p className="text-sm text-muted-foreground">
        {t("automations:importSourceDestination", {
          plugin: entry.sourcePluginId,
          instance: entry.sourceInstanceKey,
        })}
      </p>
      {!imported ? (
        <>
          <ManagedConversationDestinationSelector
            workspaceId={workspaceId}
            value={destination}
            onChange={onDestinationChange}
            isDirty={Boolean(destination)}
          />
          <Button
            type="button"
            className="min-h-11 w-full sm:w-auto"
            data-testid={`managed-automation-import-submit-${entry.sourceIndex}`}
            disabled={!destination || anyImporting}
            onClick={onImport}
          >
            {importing ? t("automations:importingSchedule") : t("automations:importSchedule")}
          </Button>
        </>
      ) : null}
    </section>
  );
}

function ManagedAutomationImportBody({
  workspaceId,
  state,
}: {
  workspaceId: string;
  state: ImportState;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4">
      <label htmlFor="managed-automation-import-file" className="text-sm font-medium">
        {t("automations:importFileLabel")}
      </label>
      <input
        ref={state.fileInput}
        id="managed-automation-import-file"
        type="file"
        accept=".yaml,.yml,application/yaml,text/yaml"
        data-testid="managed-automation-import-file"
        className="min-h-11 w-full rounded-md border p-2 text-sm file:mr-3 file:min-h-8"
        onChange={(event) => void state.loadFile(event.currentTarget.files?.[0])}
      />
      {state.errorKey ? (
        <p
          role="alert"
          data-testid="managed-automation-import-error"
          className="text-sm text-destructive"
        >
          {t(state.errorKey)}
        </p>
      ) : null}
      {state.skippedCount > 0 ? (
        <p
          className="text-sm text-muted-foreground"
          data-testid="managed-automation-import-skipped"
        >
          {t("automations:importSkippedOtherModes", { count: state.skippedCount })}
        </p>
      ) : null}
      <div className="space-y-3">
        {state.entries.map((entry) => (
          <ManagedAutomationImportEntryCard
            key={entry.sourceIndex}
            entry={entry}
            workspaceId={workspaceId}
            destination={state.destinations[entry.sourceIndex]}
            imported={state.imported.has(entry.sourceIndex)}
            importing={state.importingIndex === entry.sourceIndex}
            anyImporting={state.importingIndex !== null}
            onDestinationChange={(destination) =>
              state.setDestination(entry.sourceIndex, destination)
            }
            onImport={() => void state.importEntry(entry)}
          />
        ))}
      </div>
    </div>
  );
}

export function ManagedAutomationImportDialog({
  workspaceId,
  create,
}: {
  workspaceId: string;
  create: (payload: CreateAutomationRequest) => Promise<unknown>;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const state = useManagedAutomationImport(workspaceId, create);
  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) state.reset();
      }}
    >
      <DialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          data-testid="import-managed-automation-button"
          className="min-h-11"
        >
          <IconUpload className="mr-2 size-4" aria-hidden />
          {t("automations:importAutomations")}
        </Button>
      </DialogTrigger>
      <DialogContent
        className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl"
        data-testid="managed-automation-import-dialog"
      >
        <DialogHeader>
          <DialogTitle>{t("automations:importManagedSchedulesTitle")}</DialogTitle>
          <DialogDescription>
            {t("automations:importManagedSchedulesDescription")}
          </DialogDescription>
        </DialogHeader>
        <ManagedAutomationImportBody workspaceId={workspaceId} state={state} />
      </DialogContent>
    </Dialog>
  );
}
