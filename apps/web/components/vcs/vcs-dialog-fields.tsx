"use client";

import { IconLoader2, IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

type GenerateButtonProps = {
  onClick: () => void;
  isGenerating: boolean;
  disabled?: boolean;
  tooltip: string;
  notConfiguredTooltip?: string;
  isConfigured?: boolean;
  size?: "icon" | "sm";
  showLabel?: boolean;
};

export function GenerateButton({
  onClick,
  isGenerating,
  disabled,
  tooltip,
  notConfiguredTooltip = "Configure a utility agent in settings to enable AI generation",
  isConfigured = true,
  size = "icon",
  showLabel,
}: GenerateButtonProps) {
  const isDisabled = !isConfigured || disabled || isGenerating;
  const tooltipText = isConfigured ? tooltip : notConfiguredTooltip;

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {/* Wrap in span so tooltip works even when button is disabled */}
        <span className="inline-flex">
          <Button
            type="button"
            size={size}
            variant="ghost"
            className={
              size === "icon" ? "h-7 w-7 cursor-pointer" : "h-6 px-2 cursor-pointer gap-1.5 text-xs"
            }
            onClick={isConfigured ? onClick : undefined}
            disabled={isDisabled}
          >
            {isGenerating ? (
              <IconLoader2 className="h-4 w-4 animate-spin" />
            ) : (
              <IconSparkles className="h-4 w-4" />
            )}
            {showLabel && "Generate"}
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
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label htmlFor="vcs-commit-body" className="text-sm">
          Description
        </Label>
        <GenerateButton
          onClick={onGenerateDescription}
          isGenerating={isGeneratingDescription}
          disabled={disabled}
          tooltip="Generate commit description with AI"
          isConfigured={isUtilityConfigured}
          size="sm"
          showLabel
        />
      </div>
      <Textarea
        id="vcs-commit-body"
        data-testid="commit-body-input"
        placeholder="Add details about this change..."
        value={commitBody}
        onChange={(e) => onCommitBodyChange(e.target.value)}
        rows={3}
        className="resize-none max-h-[200px] overflow-y-auto"
      />
    </div>
  );
}

export function PRTitleField({
  prTitle,
  onPrTitleChange,
  onGenerateTitle,
  isGeneratingTitle,
  isUtilityConfigured,
}: {
  prTitle: string;
  onPrTitleChange: (v: string) => void;
  onGenerateTitle: () => void;
  isGeneratingTitle: boolean;
  isUtilityConfigured: boolean;
}) {
  return (
    <div className="relative">
      <Input
        id="vcs-pr-title"
        placeholder="Pull request title..."
        value={prTitle}
        onChange={(e) => onPrTitleChange(e.target.value)}
        className="pr-10"
        autoFocus
      />
      <div className="absolute right-1.5 top-1/2 -translate-y-1/2">
        <GenerateButton
          onClick={onGenerateTitle}
          isGenerating={isGeneratingTitle}
          tooltip="Generate PR title with AI"
          isConfigured={isUtilityConfigured}
        />
      </div>
    </div>
  );
}

export function PRBranchSummary({
  displayBranch,
  baseBranch,
}: {
  displayBranch?: string | null;
  baseBranch?: string;
}) {
  if (!displayBranch) return null;
  return (
    <div className="text-sm text-muted-foreground">
      {baseBranch ? (
        <span>
          Creating PR from <span className="font-medium text-foreground">{displayBranch}</span>
          {" → "}
          <span className="font-medium text-foreground">{baseBranch}</span>
        </span>
      ) : (
        <span>
          Creating PR from <span className="font-medium text-foreground">{displayBranch}</span>
        </span>
      )}
    </div>
  );
}
