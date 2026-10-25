"use client";

import type { ComponentType } from "react";
import { useTranslation } from "react-i18next";
import {
  IconCloudDownload,
  IconCloudUpload,
  IconEye,
  IconGitCompare,
  IconInfoCircle,
} from "@tabler/icons-react";
import { DropdownMenuItem } from "@kandev/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@kandev/ui/lib/utils";

type ActionKind = "compare" | "replace" | "use" | "view";
type DescriptionMode = "tooltip" | "inline";
type ActionSurface = "menu" | "drawer";

type ActionDefinition = {
  kind: ActionKind;
  labelKey: string;
  descriptionKey: string;
  icon: ComponentType<{ className?: string }>;
  iconClassName: string;
};

const ACTION_DEFINITIONS: ActionDefinition[] = [
  {
    kind: "compare",
    labelKey: "task:compareContributionVersions",
    descriptionKey: "task:remoteContributionCompareDescription",
    icon: IconGitCompare,
    iconClassName: "text-primary",
  },
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
  onCompareVersions?: () => void;
  compareDisabled?: boolean;
  descriptionMode?: DescriptionMode;
  surface?: ActionSurface;
  replaceLabelKey?: string;
  useLabelKey?: string;
  replaceDescriptionKey?: string;
  useDescriptionKey?: string;
  compareDescriptionKey?: string;
  itemClassName?: string;
  testIdPrefix?: string;
  prNumber?: number;
  viewLabelKey?: string;
};

function actionHandler(
  kind: ActionKind,
  props: RemoteContributionActionItemsProps,
): (() => void) | undefined {
  if (kind === "compare") return props.onCompareVersions;
  if (kind === "replace") return props.onReplaceContribution;
  if (kind === "use") return props.onUseContribution;
  return props.onViewPRVersion;
}

function actionDisabled(kind: ActionKind, props: RemoteContributionActionItemsProps): boolean {
  if (kind === "compare") return Boolean(props.compareDisabled) || !props.onCompareVersions;
  if (kind === "replace") return Boolean(props.replaceDisabled) || !props.onReplaceContribution;
  if (kind === "use") return Boolean(props.useDisabled) || !props.onUseContribution;
  return !props.onViewPRVersion;
}

function actionTestId(prefix: string | undefined, kind: ActionKind): string | undefined {
  if (!prefix) return undefined;
  if (kind === "compare") return `${prefix}-compare-versions`;
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

type ActionItemProps = {
  definition: ActionDefinition;
  disabled: boolean;
  descriptionMode: DescriptionMode;
  itemClassName?: string;
  testIdPrefix?: string;
  onSelect?: () => void;
  prNumber?: number;
  surface: ActionSurface;
  viewLabelKey?: string;
  replaceLabelKey?: string;
  useLabelKey?: string;
  replaceDescriptionKey?: string;
  useDescriptionKey?: string;
  compareDescriptionKey?: string;
};

function actionLabelKey(
  definition: ActionDefinition,
  props: Pick<ActionItemProps, "viewLabelKey" | "replaceLabelKey" | "useLabelKey">,
): string {
  switch (definition.kind) {
    case "view":
      return props.viewLabelKey ?? definition.labelKey;
    case "replace":
      return props.replaceLabelKey ?? definition.labelKey;
    case "use":
      return props.useLabelKey ?? definition.labelKey;
    default:
      return definition.labelKey;
  }
}

function actionDescriptionKey(
  definition: ActionDefinition,
  props: Pick<
    ActionItemProps,
    "replaceDescriptionKey" | "useDescriptionKey" | "compareDescriptionKey"
  >,
): string {
  switch (definition.kind) {
    case "replace":
      return props.replaceDescriptionKey ?? definition.descriptionKey;
    case "use":
      return props.useDescriptionKey ?? definition.descriptionKey;
    case "compare":
      return props.compareDescriptionKey ?? definition.descriptionKey;
    default:
      return definition.descriptionKey;
  }
}

function ActionItemContent({
  definition,
  descriptionMode,
  label,
  description,
}: Pick<ActionItemProps, "definition" | "descriptionMode"> & {
  label: string;
  description: string;
}) {
  const Icon = definition.icon;
  return (
    <>
      <Icon className={cn("h-4 w-4 shrink-0", definition.iconClassName)} />
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
      {descriptionMode === "tooltip" && <ActionInfo description={description} />}
    </>
  );
}

function actionItemClassName(
  definition: ActionDefinition,
  descriptionMode: DescriptionMode,
  surface: ActionSurface,
  itemClassName?: string,
): string {
  return cn(
    "cursor-pointer gap-3",
    descriptionMode === "inline" && "min-h-11",
    surface === "drawer" && "flex w-full items-start rounded-md px-3 py-2.5 text-left",
    definition.kind === "replace" && "text-destructive",
    itemClassName,
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
  surface,
  viewLabelKey,
  replaceLabelKey,
  useLabelKey,
  replaceDescriptionKey,
  useDescriptionKey,
  compareDescriptionKey,
}: ActionItemProps) {
  const { t } = useTranslation();
  const translationOptions = { number: prNumber ?? "" };
  const label = t(
    actionLabelKey(definition, { viewLabelKey, replaceLabelKey, useLabelKey }),
    translationOptions,
  );
  const descriptionKey = actionDescriptionKey(definition, {
    replaceDescriptionKey,
    useDescriptionKey,
    compareDescriptionKey,
  });
  const description = t(descriptionKey, { number: prNumber ?? "" });
  const className = actionItemClassName(definition, descriptionMode, surface, itemClassName);
  const content = (
    <ActionItemContent
      definition={definition}
      descriptionMode={descriptionMode}
      label={label}
      description={description}
    />
  );
  if (surface === "drawer") {
    return (
      <button
        type="button"
        className={className}
        data-testid={actionTestId(testIdPrefix, definition.kind)}
        disabled={disabled}
        onClick={onSelect}
      >
        {content}
      </button>
    );
  }
  return (
    <DropdownMenuItem
      className={className}
      data-testid={actionTestId(testIdPrefix, definition.kind)}
      disabled={disabled}
      onClick={onSelect}
      variant={definition.kind === "replace" ? "destructive" : "default"}
    >
      {content}
    </DropdownMenuItem>
  );
}

export function RemoteContributionActionItems(props: RemoteContributionActionItemsProps) {
  const descriptionMode = props.descriptionMode ?? "tooltip";
  const surface = props.surface ?? "menu";
  return (
    <>
      {ACTION_DEFINITIONS.filter(
        (definition) => definition.kind !== "compare" || props.onCompareVersions,
      ).map((definition) => (
        <ActionItem
          key={definition.kind}
          definition={definition}
          disabled={props.disabled || actionDisabled(definition.kind, props)}
          descriptionMode={descriptionMode}
          itemClassName={props.itemClassName}
          testIdPrefix={props.testIdPrefix}
          onSelect={actionHandler(definition.kind, props)}
          prNumber={props.prNumber}
          surface={surface}
          viewLabelKey={props.viewLabelKey}
          replaceLabelKey={props.replaceLabelKey}
          useLabelKey={props.useLabelKey}
          replaceDescriptionKey={props.replaceDescriptionKey}
          useDescriptionKey={props.useDescriptionKey}
          compareDescriptionKey={props.compareDescriptionKey}
        />
      ))}
    </>
  );
}
