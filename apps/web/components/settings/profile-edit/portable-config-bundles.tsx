"use client";

import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { IconAlertTriangle, IconInfoCircle } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { AgentConfigBundle } from "@/lib/api/domains/agent-config-api";

type AgentConfigOptionsProps = {
  agentId: string;
  bundles: AgentConfigBundle[];
  selectedIds: string[];
  baselineSelectedIds: string[];
  onChange: (ids: string[]) => void;
  isSSH?: boolean;
};

const PORTABLE_CONFIG_LABEL_KEYS: Record<string, string> = {
  "claude.settings": "executors:portableConfigBundleClaudeSettings",
  "codex.config": "executors:portableConfigBundleCodexConfig",
  "opencode.config": "executors:portableConfigBundleOpenCodeConfig",
  "mock.settings": "executors:portableConfigBundleMockSettings",
};

export function AgentConfigOptions({
  agentId,
  bundles,
  selectedIds,
  baselineSelectedIds,
  onChange,
  isSSH = false,
}: AgentConfigOptionsProps) {
  const { t } = useTranslation();
  if (bundles.length === 0) return null;

  const selected = new Set(selectedIds);
  const baseline = new Set(baselineSelectedIds);
  const bundleIds = new Set(bundles.map((bundle) => bundle.id));
  const isDirty = [...bundleIds].some(
    (bundleId) => selected.has(bundleId) !== baseline.has(bundleId),
  );

  const toggle = (bundleId: string, checked: boolean) => {
    const next = new Set(selected);
    if (checked) next.add(bundleId);
    else next.delete(bundleId);
    onChange([...next]);
  };

  return (
    <div
      role="group"
      aria-label={t("executors:portableConfigTitle")}
      className="grid gap-2"
      data-testid={`agent-config-options-${agentId}`}
      data-agent-id={agentId}
      data-settings-dirty={isDirty}
    >
      {bundles.map((bundle, index) => {
        const labelKey = PORTABLE_CONFIG_LABEL_KEYS[bundle.id];
        const label = labelKey
          ? t(labelKey)
          : t("executors:portableConfigBundleGeneric", { id: bundle.id });
        const bundleIsDirty = selected.has(bundle.id) !== baseline.has(bundle.id);
        return (
          <div
            key={bundle.id}
            className="flex min-h-11 items-start gap-2 rounded-md border border-border p-3 transition-colors hover:bg-muted/40"
            data-testid={`portable-config-bundle-${bundle.id}`}
            data-settings-dirty={bundleIsDirty}
          >
            <label className="flex min-w-0 flex-1 cursor-pointer items-start gap-3">
              <Checkbox
                checked={selected.has(bundle.id)}
                disabled={!bundle.available && !selected.has(bundle.id)}
                onCheckedChange={(checked) => toggle(bundle.id, checked === true)}
                aria-label={label}
                className="mt-0.5"
              />
              <span className="min-w-0 flex-1">
                <span className="flex flex-wrap items-center gap-2 text-sm font-medium">
                  <span>{label}</span>
                  <Badge variant={bundle.available ? "secondary" : "outline"}>
                    {bundle.available
                      ? t("executors:portableConfigAvailable")
                      : t("executors:portableConfigNotFound")}
                  </Badge>
                </span>
                <span className="mt-1 block space-y-0.5 text-xs text-muted-foreground">
                  {bundle.files.map((file) => (
                    <span key={`${bundle.id}:${file.target_path}`} className="block break-all">
                      <Trans
                        i18nKey="executors:portableConfigFileMapping"
                        values={{ source: file.source_path, target: file.target_path }}
                      />
                    </span>
                  ))}
                </span>
                {!bundle.available && (
                  <span className="mt-1 block text-xs text-amber-600 dark:text-amber-400">
                    {t("executors:portableConfigNotFoundHint")}
                  </span>
                )}
              </span>
            </label>
            {index === 0 && (
              <PortableConfigInfo isSSH={isSSH} testId={`agent-config-info-${agentId}`} />
            )}
          </div>
        );
      })}
    </div>
  );
}

function PortableConfigInfo({ isSSH, testId }: { isSSH: boolean; testId: string }) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const [open, setOpen] = useState(false);
  const button = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={t("executors:portableConfigInfoLabel")}
      aria-haspopup={usesTouchDrawer ? "dialog" : undefined}
      aria-expanded={usesTouchDrawer ? open : undefined}
      data-testid={testId}
      className={usesTouchDrawer ? "h-11 w-11 shrink-0" : "h-7 w-7 shrink-0"}
    >
      <IconAlertTriangle className="size-4 text-amber-500" aria-hidden="true" />
    </Button>
  );
  const content = (
    <div className="space-y-2 text-xs">
      <p>{t("executors:portableConfigInfoCopyTiming")}</p>
      <p>{t("executors:portableConfigInfoErrors")}</p>
      <p>{t("executors:portableConfigInfoScope")}</p>
      {isSSH && <p>{t("executors:portableConfigInfoSsh")}</p>}
    </div>
  );
  const trigger = usesTouchDrawer ? (
    <DrawerTrigger asChild>{button}</DrawerTrigger>
  ) : (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>{button}</TooltipTrigger>
        <TooltipContent
          side="bottom"
          align="start"
          sideOffset={8}
          className="max-w-[min(24rem,calc(100vw-2rem))]"
        >
          {content}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );

  return (
    <Drawer open={open} onOpenChange={setOpen}>
      {trigger}
      <DrawerContent>
        <DrawerHeader>
          <DrawerTitle className="flex items-center gap-2">
            <IconInfoCircle className="size-4" aria-hidden="true" />
            {t("executors:portableConfigInfoTitle")}
          </DrawerTitle>
          <DrawerDescription>{t("executors:portableConfigInfoLabel")}</DrawerDescription>
        </DrawerHeader>
        <div className="min-h-0 overflow-y-auto px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
          {content}
        </div>
      </DrawerContent>
    </Drawer>
  );
}
