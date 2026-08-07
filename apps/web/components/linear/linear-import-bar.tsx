"use client";

import { IconHexagon } from "@tabler/icons-react";
import { getLinearIssue } from "@/lib/api/domains/linear-api";
import type { LinearIssue } from "@/lib/types/linear";
import { LINEAR_KEY_RE } from "./linear-issue-common";
import { useLinearAvailable } from "@/hooks/domains/linear/use-linear-availability";
import { ValidatedPopover } from "@/components/integrations/validated-popover";
import { useTranslation } from "react-i18next";

type LinearImportBarProps = {
  workspaceId: string | null;
  disabled?: boolean;
  onImport: (issue: LinearIssue) => void;
};

export function LinearImportBar({ workspaceId, disabled, onImport }: LinearImportBarProps) {
  const { t } = useTranslation();
  const available = useLinearAvailable(workspaceId);
  if (!available || !workspaceId) return null;

  return (
    <ValidatedPopover
      triggerStyle="ghost-icon"
      triggerIcon={<IconHexagon className="h-4 w-4" />}
      triggerAriaLabel={t("linear:importFromLinear")}
      triggerDisabled={disabled}
      testIdPrefix="linear-import"
      tooltip={t("linear:importFromLinearIssueUrlOr")}
      align="start"
      headline={t("linear:importLinearIssue")}
      placeholder={t("linear:eng123OrPasteIssueUrl")}
      extractKey={(raw) => raw.toUpperCase().match(LINEAR_KEY_RE)?.[0] ?? null}
      validationHint={t("linear:pasteALinearIssueUrlOr")}
      fetch={(key) => getLinearIssue(key, { workspaceId })}
      onSuccess={(_key, issue) => onImport(issue)}
      submitLabel={t("linear:import")}
      submittingLabel={t("linear:loading")}
    />
  );
}
