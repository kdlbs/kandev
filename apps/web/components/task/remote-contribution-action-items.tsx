"use client";

import type { ComponentType } from "react";
import { useTranslation } from "react-i18next";
import { IconCloudDownload, IconCloudUpload, IconEye, IconInfoCircle } from "@tabler/icons-react";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";

type ActionKind = "replace" | "use" | "view";
type DescriptionMode = "tooltip" | "inline";

type ActionDefinition = {
  kind: ActionKind;
  labelKey: string;
  descriptionKey: string;
  icon: ComponentType<{ className?: string }>;
  iconClassName: string;
};

const ACTION_DEFINITIONS: ActionDefinition[] = [
  {
    kind: "replace",
    labelKey: "task:replacePRBranch",
    descriptionKey: "task:remoteContributionReplaceDescription",
    icon: IconCloudUpload,
    iconClassName: "text-destructive",
  },
  {
    kind: "use",
    labelKey: "task:usePRVersion",
    descriptionKey: "task:remoteContributionUseDescription",
    icon: IconCloudDownload,
    iconClassName: "text-blue-500",
  },
  {
    kind: "view",
    labelKey: "task:prNumberVersion",
    descriptionKey: "task:remoteContributionViewDescription",
    icon: IconEye,
    iconClassName: "text-muted-foreground",
  },
];

export type RemoteContributionActionItemsProps = {
  disabled: boolean;
  replaceDisabled?: boolean;
  useDisabled?: boolean;
  onReplaceContribution?: () => void;
  onUseContribution?: () => void;
  onViewPRVersion?: () => void;
  descriptionMode?: DescriptionMode;
  itemClassName?: string;
  testIdPrefix?: string;
  prNumber?: number;
};

function actionHandler(
  kind: ActionKind,
  props: RemoteContributionActionItemsProps,
): (() => void) | undefined {
  if (kind === "replace") return props.onReplaceContribution;
  if (kind === "use") return props.onUseContribution;
  return props.onViewPRVersion;
}

function actionDisabled(kind: ActionKind, props: RemoteContributionActionItemsProps): boolean {
  if (kind === "replace") return Boolean(props.replaceDisabled) || !props.onReplaceContribution;
  if (kind === "use") return Boolean(props.useDisabled) || !props.onUseContribution;
  return !props.onViewPRVersion;
}

function actionTestId(prefix: string | undefined, kind: ActionKind): string | undefined {
  if (!prefix) return undefined;
  if (kind === "replace") return `${prefix}-replace-pr-branch`;
  if (kind === "use") return `${prefix}-use-pr-version`;
  return `${prefix}-view-pr-version`;
}

function ActionInfo({ description }: { description: string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          aria-label={description}
          className="ml-1 inline-flex h-5 w-5 shrink-0 cursor-help items-center justify-center rounded text-muted-foreground/70 hover:text-foreground"
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          role="img"
          tabIndex={0}
        >
          <IconInfoCircle className="h-3.5 w-3.5" aria-hidden="true" />
        </span>
      </TooltipTrigger>
      <TooltipContent side="left" sideOffset={4}>
        {description}
      </TooltipContent>
    </Tooltip>
  );
}

function ActionItem({
  definition,
  disabled,
  descriptionMode,
  itemClassName,
  testIdPrefix,
  onSelect,
  prNumber,
}: {
  definition: ActionDefinition;
  disabled: boolean;
  descriptionMode: DescriptionMode;
  itemClassName?: string;
  testIdPrefix?: string;
  onSelect?: () => void;
  prNumber?: number;
}) {
  const { t } = useTranslation();
  const Icon = definition.icon;
  const label = t(definition.labelKey, { number: prNumber ?? "" });
  const description = t(definition.descriptionKey, { number: prNumber ?? "" });
  const labelContent = (
    <span
      className={cn(
        "min-w-0 flex-1",
        descriptionMode === "inline" && "flex flex-col gap-0.5 leading-tight",
      )}
    >
      <span className="truncate">{label}</span>
      {descriptionMode === "inline" && (
        <span className="text-[10px] font-normal leading-tight text-muted-foreground">
          {description}
        </span>
      )}
    </span>
  );

  return (
    <DropdownMenuItem
      className={cn(
        "cursor-pointer gap-3",
        descriptionMode === "inline" && "min-h-11",
        itemClassName,
      )}
      data-testid={actionTestId(testIdPrefix, definition.kind)}
      disabled={disabled}
      onClick={onSelect}
      variant={definition.kind === "replace" ? "destructive" : "default"}
    >
      <Icon className={cn("h-4 w-4", definition.iconClassName)} />
      {labelContent}
      {descriptionMode === "tooltip" && <ActionInfo description={description} />}
    </DropdownMenuItem>
  );
}

export function RemoteContributionActionItems(props: RemoteContributionActionItemsProps) {
  const descriptionMode = props.descriptionMode ?? "tooltip";
  return (
    <>
      {ACTION_DEFINITIONS.map((definition) => (
        <ActionItem
          key={definition.kind}
          definition={definition}
          disabled={props.disabled || actionDisabled(definition.kind, props)}
          descriptionMode={descriptionMode}
          itemClassName={props.itemClassName}
          testIdPrefix={props.testIdPrefix}
          onSelect={actionHandler(definition.kind, props)}
          prNumber={props.prNumber}
        />
      ))}
    </>
  );
}
