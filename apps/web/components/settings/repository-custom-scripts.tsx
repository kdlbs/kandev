import { IconPlus, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import type { RepositoryScript } from "@/lib/types/http";
import { UnsavedChangesBadge } from "./unsaved-indicator";

type RepositoryCustomScriptsProps = {
  repositoryId: string;
  scripts: RepositoryScript[];
  savedScripts?: RepositoryScript[];
  areScriptsDirty: boolean;
  onAddScript: (repoId: string) => void;
  onUpdateScript: (repoId: string, scriptId: string, updates: Partial<RepositoryScript>) => void;
  onDeleteScript: (repoId: string, scriptId: string) => void;
};

export function RepositoryCustomScripts({
  repositoryId,
  scripts,
  savedScripts,
  areScriptsDirty,
  onAddScript,
  onUpdateScript,
  onDeleteScript,
}: RepositoryCustomScriptsProps) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <Label className="flex items-center gap-2">
          <span>Custom Scripts</span>
          {areScriptsDirty && <UnsavedChangesBadge />}
        </Label>
        <Button type="button" variant="outline" size="sm" onClick={() => onAddScript(repositoryId)}>
          <IconPlus className="h-4 w-4 mr-1" />
          Add Script
        </Button>
      </div>
      <div className="space-y-3">
        {scripts.length === 0 ? (
          <p className="text-sm text-muted-foreground">No scripts yet.</p>
        ) : (
          scripts.map((script) => (
            <RepositoryCustomScript
              key={script.id}
              repositoryId={repositoryId}
              script={script}
              savedScript={savedScripts?.find((candidate) => candidate.id === script.id)}
              onUpdate={onUpdateScript}
              onDelete={onDeleteScript}
            />
          ))
        )}
      </div>
    </div>
  );
}

function RepositoryCustomScript({
  repositoryId,
  script,
  savedScript,
  onUpdate,
  onDelete,
}: {
  repositoryId: string;
  script: RepositoryScript;
  savedScript?: RepositoryScript;
  onUpdate: RepositoryCustomScriptsProps["onUpdateScript"];
  onDelete: RepositoryCustomScriptsProps["onDeleteScript"];
}) {
  const nameIsDirty = !savedScript || script.name !== savedScript.name;
  const commandIsDirty = !savedScript || script.command !== savedScript.command;
  return (
    <div
      className="grid gap-2"
      data-settings-dirty={nameIsDirty || commandIsDirty}
      data-settings-dirty-level="container"
    >
      <div className="flex items-center gap-2">
        <Input
          value={script.name ?? ""}
          onChange={(event) => onUpdate(repositoryId, script.id, { name: event.target.value })}
          placeholder="Script name"
          data-settings-dirty={nameIsDirty}
        />
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={() => onDelete(repositoryId, script.id)}
        >
          <IconX className="h-4 w-4" />
        </Button>
      </div>
      <Textarea
        value={script.command ?? ""}
        onChange={(event) => onUpdate(repositoryId, script.id, { command: event.target.value })}
        placeholder="#!/bin/bash&#10;npm run dev"
        rows={3}
        className="font-mono text-sm"
        data-settings-dirty={commandIsDirty}
      />
    </div>
  );
}
