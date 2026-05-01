"use client";

import { IconHexagon } from "@tabler/icons-react";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import type { LinearIssue } from "@/lib/types/linear";
import { LINEAR_KEY_RE } from "./linear-issue-common";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";

type LinearImportBarProps = {
  workspaceId: string | null;
  disabled?: boolean;
  onImport: (issue: LinearIssue) => void;
};

export function LinearImportBar({ workspaceId, disabled, onImport }: LinearImportBarProps) {
  const available = useLinearAvailable(workspaceId);
  if (!available || !workspaceId) return null;

  return (
    <ValidatedPopover
      triggerStyle="ghost-icon"
      triggerIcon={<IconHexagon className="h-4 w-4" />}
      triggerAriaLabel="Import from Linear"
      triggerDisabled={disabled}
      tooltip="Import from Linear issue URL or identifier"
      align="start"
      headline="Import Linear issue"
      placeholder="ENG-123 or paste issue URL"
      extractKey={(raw) => raw.toUpperCase().match(LINEAR_KEY_RE)?.[0] ?? null}
      validationHint="Paste a Linear issue URL or identifier (ENG-123)"
      fetch={(key) => getLinearIssue(workspaceId, key)}
      onSuccess={(_key, issue) => onImport(issue)}
      submitLabel="Import"
      submittingLabel="Loading..."
    />
  );
}
