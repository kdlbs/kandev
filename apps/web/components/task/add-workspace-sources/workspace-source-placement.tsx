"use client";

import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { useTranslation } from "react-i18next";
import type {
  WorkspaceRepositoryPlacement,
  WorkspaceRepositoryPlacementPreview,
} from "@/lib/types/http";
import { cn } from "@/lib/utils";

type Props = {
  placement?: WorkspaceRepositoryPlacement | null;
  onPlacementChange: (placement: WorkspaceRepositoryPlacement) => void;
  preview?: WorkspaceRepositoryPlacementPreview | null;
  previewLoading?: boolean;
};

const placements: WorkspaceRepositoryPlacement[] = [
  "kandev_directory",
  "current_root",
  "expand_root",
];

export function WorkspaceSourcePlacement({
  placement,
  onPlacementChange,
  preview,
  previewLoading = false,
}: Props) {
  const { t } = useTranslation();
  const options = placements.map((value) => {
    const serverOption = preview?.supported_placements.find((option) => option.placement === value);
    if (serverOption)
      return {
        ...serverOption,
        reason: serverOption.enabled
          ? undefined
          : placementDisabledReason(t, value, serverOption.reason),
      };
    return {
      placement: value,
      enabled: value !== "expand_root",
      reason: placementDisabledReason(t, value),
    };
  });

  return (
    <fieldset className="space-y-3 rounded border p-3" data-testid="workspace-source-placement">
      <legend className="text-sm font-medium">{t("task:workspaceSourcePlacementTitle")}</legend>
      <p className="text-sm text-muted-foreground">
        {t("task:workspaceSourcePlacementDescription")}
      </p>
      <RadioGroup
        value={placement ?? ""}
        onValueChange={(value) => {
          if (options.some((option) => option.placement === value && option.enabled)) {
            onPlacementChange(value as WorkspaceRepositoryPlacement);
          }
        }}
        className="grid gap-2 md:grid-cols-3"
        aria-label={t("task:workspaceSourcePlacementTitle")}
      >
        {options.map((option) => (
          <label
            key={option.placement}
            className={cn(
              "flex min-h-11 cursor-pointer items-start gap-2 rounded border p-3",
              option.enabled ? "hover:border-primary/60" : "cursor-not-allowed opacity-60",
              placement === option.placement && option.enabled && "border-primary bg-primary/5",
            )}
          >
            <RadioGroupItem
              value={option.placement}
              disabled={!option.enabled}
              className="mt-0.5"
            />
            <span className="min-w-0">
              <span className="block text-sm font-medium">
                {placementTitle(t, option.placement)}
              </span>
              <span className="block text-xs text-muted-foreground">
                {placementDescription(t, option.placement)}
              </span>
              {!option.enabled && option.reason && (
                <span className="mt-1 block text-xs text-muted-foreground">{option.reason}</span>
              )}
            </span>
          </label>
        ))}
      </RadioGroup>
      {!placement && (
        <p className="text-xs text-muted-foreground" role="alert">
          {t("task:workspaceSourcePlacementSelectionRequired")}
        </p>
      )}
      {previewLoading && (
        <p className="text-xs text-muted-foreground">
          {t("task:workspaceSourcePlacementPreviewing")}
        </p>
      )}
      {preview && preview.sources.length > 0 && (
        <div
          className="space-y-1 rounded bg-muted/40 p-2 text-xs"
          data-testid="workspace-source-placement-preview"
        >
          <p className="font-medium">{t("task:workspaceSourcePlacementPreviewTitle")}</p>
          {preview.sources.map((source) => (
            <p
              key={`${source.kind ?? "repository"}:${source.repository_id ?? source.source_name}:${source.workspace_relative_path}`}
              className="truncate"
            >
              {source.source_name ?? source.repository_name}:{" "}
              <code>{source.workspace_relative_path}</code>
            </p>
          ))}
        </div>
      )}
    </fieldset>
  );
}

function placementTitle(
  translate: (key: string) => string,
  placement: WorkspaceRepositoryPlacement,
): string {
  switch (placement) {
    case "kandev_directory":
      return translate("task:workspaceSourcePlacementKandev");
    case "current_root":
      return translate("task:workspaceSourcePlacementCurrentRoot");
    case "expand_root":
      return translate("task:workspaceSourcePlacementExpand");
  }
  return translate("task:workspaceSourcePlacementTitle");
}

function placementDescription(
  translate: (key: string) => string,
  placement: WorkspaceRepositoryPlacement,
): string {
  switch (placement) {
    case "kandev_directory":
      return translate("task:workspaceSourcePlacementKandevDescription");
    case "current_root":
      return translate("task:workspaceSourcePlacementCurrentDescription");
    case "expand_root":
      return translate("task:workspaceSourcePlacementExpandDescription");
  }
  return translate("task:workspaceSourcePlacementDescription");
}

function placementDisabledReason(
  translate: (key: string) => string,
  placement: WorkspaceRepositoryPlacement,
  serverReason?: string,
): string | undefined {
  if (placement === "expand_root") {
    return translate("task:workspaceSourcePlacementExpansionUnavailable");
  }
  return serverReason;
}
