"use client";

import { IconLoader2, IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { getChangeRequestTerminology } from "@/hooks/use-git-operations";
import { Trans, useTranslation } from "react-i18next";

type ChangeRequestTerminology = ReturnType<typeof getChangeRequestTerminology>;

type GenerateButtonProps = {
  onClick: () => void;
  isGenerating: boolean;
  disabled?: boolean;
  tooltip: string;
  notConfiguredTooltip?: string;
  isConfigured?: boolean;
};

export function GenerateButton({
  onClick,
  isGenerating,
  disabled,
  tooltip,
  notConfiguredTooltip,
  isConfigured = true,
}: GenerateButtonProps) {
  const { t } = useTranslation();
  const isDisabled = !isConfigured || disabled || isGenerating;
  const tooltipText = isConfigured
    ? tooltip
    : (notConfiguredTooltip ?? t("integrations:configureAUtilityAgentInSettings"));

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex">
          <Button
            type="button"
            size="icon"
            variant="ghost"
            aria-label={tooltip}
            className="h-7 w-7 cursor-pointer"
            onClick={isConfigured ? onClick : undefined}
            disabled={isDisabled}
          >
            {isGenerating ? (
              <IconLoader2 className="h-4 w-4 animate-spin" />
            ) : (
              <IconSparkles className="h-4 w-4" />
            )}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltipText}</TooltipContent>
    </Tooltip>
  );
}

export function CommitBodyField({
  commitBody,
  onCommitBodyChange,
  onGenerateDescription,
  isGeneratingDescription,
  isUtilityConfigured,
  disabled,
}: {
  commitBody: string;
  onCommitBodyChange: (v: string) => void;
  onGenerateDescription: () => void;
  isGeneratingDescription: boolean;
  isUtilityConfigured: boolean;
  disabled: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label htmlFor="vcs-commit-body" className="text-sm">
        {t("integrations:description")}
      </Label>
      <div className="relative min-w-0">
        <Textarea
          id="vcs-commit-body"
          data-testid="commit-body-input"
          placeholder={t("integrations:addDetailsAboutThisChange")}
          value={commitBody}
          onChange={(e) => onCommitBodyChange(e.target.value)}
          rows={3}
          className="resize-none max-h-[200px] overflow-y-auto pr-10"
        />
        <div className="absolute right-1.5 top-1.5">
          <GenerateButton
            onClick={onGenerateDescription}
            isGenerating={isGeneratingDescription}
            disabled={disabled}
            tooltip={t("integrations:generateCommitDescriptionWithAi")}
            isConfigured={isUtilityConfigured}
          />
        </div>
      </div>
    </div>
  );
}

export function PRTitleField({
  prTitle,
  onPrTitleChange,
  onGenerateTitle,
  isGeneratingTitle,
  isUtilityConfigured,
  terminology,
}: {
  prTitle: string;
  onPrTitleChange: (v: string) => void;
  onGenerateTitle: () => void;
  isGeneratingTitle: boolean;
  isUtilityConfigured: boolean;
  terminology: ChangeRequestTerminology;
}) {
  const { t } = useTranslation();
  return (
    <div className="relative min-w-0">
      <Input
        id="vcs-pr-title"
        aria-label={t("integrations:longNameTitle", { longName: terminology.longName })}
        placeholder={t("integrations:longNameTitlePlaceholder", {
          longName: terminology.longName,
        })}
        value={prTitle}
        onChange={(e) => onPrTitleChange(e.target.value)}
        className="pr-10"
        autoFocus
      />
      <div className="absolute right-1.5 top-1/2 -translate-y-1/2">
        <GenerateButton
          onClick={onGenerateTitle}
          isGenerating={isGeneratingTitle}
          tooltip={t("integrations:generateTitleWithAi", { shortName: terminology.shortName })}
          isConfigured={isUtilityConfigured}
        />
      </div>
    </div>
  );
}

export function PRDescriptionField({
  prBody,
  onPrBodyChange,
  onGenerateDescription,
  isGeneratingDescription,
  isUtilityConfigured,
  terminology,
}: {
  prBody: string;
  onPrBodyChange: (v: string) => void;
  onGenerateDescription: () => void;
  isGeneratingDescription: boolean;
  isUtilityConfigured: boolean;
  terminology: ChangeRequestTerminology;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label htmlFor="vcs-pr-body" className="text-sm">
        {t("integrations:description")}
      </Label>
      <div className="relative min-w-0">
        <Textarea
          id="vcs-pr-body"
          placeholder={t("integrations:describeYourChanges")}
          value={prBody}
          onChange={(e) => onPrBodyChange(e.target.value)}
          rows={6}
          className="resize-none max-h-[200px] overflow-y-auto pr-10"
        />
        <div className="absolute right-1.5 top-1.5">
          <GenerateButton
            onClick={onGenerateDescription}
            isGenerating={isGeneratingDescription}
            tooltip={t("integrations:generateDescriptionWithAi", {
              shortName: terminology.shortName,
            })}
            isConfigured={isUtilityConfigured}
          />
        </div>
      </div>
    </div>
  );
}

export function PRBranchSummary({
  displayBranch,
  baseBranch,
  terminology,
}: {
  displayBranch?: string | null;
  baseBranch?: string;
  terminology: ChangeRequestTerminology;
}) {
  if (!displayBranch) return null;
  return (
    <div className="text-sm text-muted-foreground">
      {baseBranch ? (
        <span>
          <Trans
            i18nKey="integrations:creatingFromInto"
            values={{ shortName: terminology.shortName, displayBranch, baseBranch }}
          >
            Creating {{ shortName: terminology.shortName }} from{" "}
            <span className="font-medium text-foreground">{displayBranch}</span>
            {" → "}
            <span className="font-medium text-foreground">{baseBranch}</span>
          </Trans>
        </span>
      ) : (
        <span>
          <Trans
            i18nKey="integrations:creatingFrom"
            values={{ shortName: terminology.shortName, displayBranch }}
          >
            Creating {{ shortName: terminology.shortName }} from{" "}
            <span className="font-medium text-foreground">{displayBranch}</span>
          </Trans>
        </span>
      )}
    </div>
  );
}

export function ChangeRequestPartialStatus({
  terminology,
}: {
  terminology: ChangeRequestTerminology;
}) {
  const { t } = useTranslation();
  return (
    <div role="status" className="border-l-2 border-amber-500 bg-amber-500/10 px-3 py-2 text-sm">
      {t("integrations:branchWasPushedRetryCreation", {
        longName: terminology.longName.toLowerCase(),
      })}
    </div>
  );
}
