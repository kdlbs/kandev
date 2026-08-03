"use client";

import { createElement, useCallback, useEffect, useMemo, useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import { IconPlus, IconRefresh, IconTrash } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { CardContent } from "@kandev/ui/card";
import { Input } from "@kandev/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import {
  ScriptEditor,
  computeEditorHeight,
} from "@/components/settings/profile-edit/script-editor";
import type { ScriptPlaceholder } from "@/components/settings/profile-edit/script-editor-completions";
import {
  ACTION_PRESET_ICON_CHOICES,
  iconForActionPreset,
} from "@/components/integrations/action-preset-icons";
import { SettingsCard } from "@/components/settings/settings-card";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { useToast } from "@/components/toast-provider";
import {
  getAzureDevOpsWorkspaceSettings,
  updateAzureDevOpsWorkspaceSettings,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsActionPreset } from "@/lib/types/azure-devops";
import {
  DEFAULT_AZURE_PULL_REQUEST_ACTIONS,
  DEFAULT_AZURE_WORK_ITEM_ACTIONS,
} from "./azure-devops-workspace-defaults";

type Translate = (key: string, values?: Record<string, unknown>) => string;

/**
 * Prompt-template placeholders offered by the ScriptEditor completions.
 *
 * A function rather than a module-scope constant: `t()` at module scope would
 * freeze the descriptions at the boot locale. `key` is the placeholder token
 * the backend substitutes and `example` is sample data — neither is copy.
 * Memoize at every call site: ScriptEditor keys its completion-provider
 * registration on array identity.
 */
function actionPromptPlaceholders(t: Translate): ScriptPlaceholder[] {
  return [
    {
      key: "url",
      description: t("azuredevops:placeholderUrlDescription"),
      example: "https://dev.azure.com/acme/Platform/_workitems/edit/42",
      executor_types: [],
    },
    {
      key: "title",
      description: t("azuredevops:placeholderTitleDescription"),
      example: "Rotate integration credentials",
      executor_types: [],
    },
  ];
}

/**
 * Wire discriminant for the two action kinds. These used to be the English
 * phrases "Work item" / "Pull request", which made the same value both a
 * `===` comparand and display copy; they are now identifiers, and the labels
 * resolve through the key maps below.
 */
type ActionKind = "work-item" | "pull-request";

/** Title-cased kind noun, e.g. "Work item action label 1". */
const KIND_TITLE_KEYS: Record<ActionKind, string> = {
  "work-item": "azuredevops:workItemKindTitle",
  "pull-request": "azuredevops:pullRequestKindTitle",
};

/** Lower-cased kind noun for mid-sentence use, e.g. "Remove work item action 1". */
const KIND_LOWER_KEYS: Record<ActionKind, string> = {
  "work-item": "azuredevops:workItemKindLower",
  "pull-request": "azuredevops:pullRequestKindLower",
};

/** Route these presets drive; a path, so it is interpolated rather than translated. */
const AZURE_DEVOPS_ROUTE = "/azure-devops";

/** Prompt-template tokens, shown verbatim in the hint below the editor. */
const PROMPT_OPEN_BRACES = "{{";
const PROMPT_URL_TOKEN = "{{url}}";
const PROMPT_TITLE_TOKEN = "{{title}}";

// `label` is PERSISTED as part of `AzureDevOpsActionPreset` and is editable in
// the row below, so it must stay locale-neutral — the same contract as
// `newPreset` in components/github/action-presets-section.tsx.
function newAction(): AzureDevOpsActionPreset {
  return {
    id: `preset_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`,
    label: "New action",
    hint: "",
    icon: "sparkle",
    promptTemplate: "",
  };
}

function ActionIconSelect({
  value,
  dirty,
  onChange,
}: {
  value: string;
  dirty: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className="h-11 w-full cursor-pointer sm:h-8"
        aria-label={t("azuredevops:icon")}
        data-settings-dirty={dirty}
      >
        <SelectValue>
          {createElement(iconForActionPreset(value), { className: "h-4 w-4" })}
        </SelectValue>
      </SelectTrigger>
      <SelectContent>
        {ACTION_PRESET_ICON_CHOICES.map((choice) => {
          const ChoiceIcon = choice.icon;
          return (
            <SelectItem key={choice.key} value={choice.key} className="cursor-pointer">
              <span className="flex items-center gap-2">
                <ChoiceIcon className="h-3.5 w-3.5" />
                {t(choice.labelKey)}
              </span>
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}

function Field({
  label,
  className,
  children,
}: {
  label: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <label className={`flex min-w-0 flex-col gap-0.5 ${className ?? ""}`}>
      <span className="text-[10px] text-muted-foreground">{label}</span>
      {children}
    </label>
  );
}

function ActionPromptPanel({
  action,
  baseline,
  onPatch,
}: {
  action: AzureDevOpsActionPreset;
  baseline?: AzureDevOpsActionPreset;
  onPatch: (patch: Partial<AzureDevOpsActionPreset>) => void;
}) {
  const { t } = useTranslation();
  // ScriptEditor keys its completion-provider registration on array identity, so
  // a fresh array per render would re-register on every keystroke.
  const placeholders = useMemo(() => actionPromptPlaceholders(t), [t]);
  return (
    <div className="space-y-1 px-3 pb-3 sm:px-2 sm:pb-2">
      <div
        className="overflow-hidden rounded-md border"
        data-settings-dirty={action.promptTemplate !== baseline?.promptTemplate}
        data-settings-dirty-level="container"
      >
        <ScriptEditor
          value={action.promptTemplate}
          onChange={(promptTemplate) => onPatch({ promptTemplate })}
          language="markdown"
          height={computeEditorHeight(action.promptTemplate)}
          lineNumbers="off"
          placeholders={placeholders}
        />
      </div>
      <p className="text-[11px] text-muted-foreground/60">
        {/* The three tokens are prompt syntax, passed as values so neither
            i18next interpolation nor the pseudo-locale rewrites them. */}
        <Trans
          i18nKey="azuredevops:promptPlaceholdersHint"
          values={{ open: PROMPT_OPEN_BRACES, url: PROMPT_URL_TOKEN, title: PROMPT_TITLE_TOKEN }}
        >
          Type {PROMPT_OPEN_BRACES} to see available placeholders. <code>{PROMPT_URL_TOKEN}</code>{" "}
          and <code>{PROMPT_TITLE_TOKEN}</code> are substituted when the action runs.
        </Trans>
      </p>
    </div>
  );
}

function ActionRow({
  kind,
  index,
  action,
  baseline,
  expanded,
  onToggle,
  onPatch,
  onRemove,
}: {
  kind: ActionKind;
  index: number;
  action: AzureDevOpsActionPreset;
  baseline?: AzureDevOpsActionPreset;
  expanded: boolean;
  onToggle: () => void;
  onPatch: (patch: Partial<AzureDevOpsActionPreset>) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const kindTitle = t(KIND_TITLE_KEYS[kind]);
  return (
    <div
      className="rounded-md border"
      data-settings-dirty={JSON.stringify(action) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-[5rem_10rem_minmax(0,1fr)_auto_auto] sm:items-end sm:p-2">
        <Field label={t("azuredevops:icon")}>
          <ActionIconSelect
            value={action.icon}
            dirty={action.icon !== baseline?.icon}
            onChange={(icon) => onPatch({ icon })}
          />
        </Field>
        <Field label={t("azuredevops:label")}>
          <Input
            className="h-11 w-full sm:h-8"
            value={action.label}
            aria-label={t("azuredevops:actionLabelAria", { kind: kindTitle, index: index + 1 })}
            data-settings-dirty={action.label !== baseline?.label}
            onChange={(event) => onPatch({ label: event.target.value })}
          />
        </Field>
        <Field label={t("azuredevops:hint")} className="col-span-2 sm:col-span-1">
          <Input
            className="h-11 w-full sm:h-8"
            value={action.hint}
            aria-label={t("azuredevops:actionHintAria", { kind: kindTitle, index: index + 1 })}
            placeholder={t("azuredevops:hintOptional")}
            data-settings-dirty={action.hint !== baseline?.hint}
            onChange={(event) => onPatch({ hint: event.target.value })}
          />
        </Field>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-11 cursor-pointer text-xs sm:h-8"
          onClick={onToggle}
        >
          {expanded ? t("azuredevops:hidePrompt") : t("azuredevops:editPrompt")}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-11 w-full cursor-pointer text-destructive sm:h-8 sm:w-8"
          onClick={onRemove}
          aria-label={t("azuredevops:removeActionAria", {
            kind: t(KIND_LOWER_KEYS[kind]),
            index: index + 1,
          })}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </div>
      {expanded && <ActionPromptPanel action={action} baseline={baseline} onPatch={onPatch} />}
    </div>
  );
}

function ActionEditor({
  kind,
  actions,
  baseline,
  onChange,
}: {
  kind: ActionKind;
  actions: AzureDevOpsActionPreset[];
  baseline: AzureDevOpsActionPreset[];
  onChange: (actions: AzureDevOpsActionPreset[]) => void;
}) {
  const { t } = useTranslation();
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const patch = (index: number, values: Partial<AzureDevOpsActionPreset>) =>
    onChange(
      actions.map((action, current) => (current === index ? { ...action, ...values } : action)),
    );
  const add = () => {
    const action = newAction();
    onChange([...actions, action]);
    setExpandedId(action.id);
  };
  return (
    <div className="space-y-2">
      {actions.map((action, index) => (
        <ActionRow
          key={action.id}
          kind={kind}
          index={index}
          action={action}
          baseline={baseline.find((candidate) => candidate.id === action.id)}
          expanded={expandedId === action.id}
          onToggle={() => setExpandedId((id) => (id === action.id ? null : action.id))}
          onPatch={(values) => patch(index, values)}
          onRemove={() => onChange(actions.filter((_, current) => current !== index))}
        />
      ))}
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-11 w-full cursor-pointer sm:h-8 sm:w-auto"
        onClick={add}
      >
        <IconPlus className="h-4 w-4" />{" "}
        {kind === "pull-request"
          ? t("azuredevops:addPrAction")
          : t("azuredevops:addWorkItemAction")}
      </Button>
    </div>
  );
}

function useActionDrafts(workspaceId: string, t: Translate) {
  const [workItems, setWorkItems] = useState(DEFAULT_AZURE_WORK_ITEM_ACTIONS);
  const [pullRequests, setPullRequests] = useState(DEFAULT_AZURE_PULL_REQUEST_ACTIONS);
  const [baseline, setBaseline] = useState({ workItems, pullRequests });
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [resetRequested, setResetRequested] = useState(false);
  const dirty = useMemo(
    () =>
      resetRequested || JSON.stringify({ workItems, pullRequests }) !== JSON.stringify(baseline),
    [baseline, pullRequests, resetRequested, workItems],
  );

  useEffect(() => {
    let current = true;
    setLoading(true);
    setLoadError(null);
    void getAzureDevOpsWorkspaceSettings(workspaceId)
      .then((settings) => {
        if (!current) return;
        setWorkItems(settings.workItemActions);
        setPullRequests(settings.pullRequestActions);
        setBaseline({
          workItems: settings.workItemActions,
          pullRequests: settings.pullRequestActions,
        });
      })
      .catch((error: unknown) => {
        if (!current) return;
        setLoadError(
          error instanceof Error ? error.message : t("azuredevops:failedToLoadQuickActions"),
        );
      })
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [t, workspaceId]);

  const save = useCallback(async () => {
    const response = await updateAzureDevOpsWorkspaceSettings(
      workspaceId,
      resetRequested
        ? { workItemActions: null, pullRequestActions: null }
        : { workItemActions: workItems, pullRequestActions: pullRequests },
    );
    setWorkItems(response.workItemActions);
    setPullRequests(response.pullRequestActions);
    setBaseline({
      workItems: response.workItemActions,
      pullRequests: response.pullRequestActions,
    });
    setResetRequested(false);
  }, [pullRequests, resetRequested, workItems, workspaceId]);
  const reset = useCallback(() => {
    setWorkItems(DEFAULT_AZURE_WORK_ITEM_ACTIONS);
    setPullRequests(DEFAULT_AZURE_PULL_REQUEST_ACTIONS);
    setResetRequested(true);
  }, []);
  const discard = useCallback(() => {
    setWorkItems(baseline.workItems);
    setPullRequests(baseline.pullRequests);
    setResetRequested(false);
  }, [baseline]);

  return {
    workItems,
    pullRequests,
    setWorkItems,
    setPullRequests,
    baseline,
    loading,
    loadError,
    dirty,
    save,
    reset,
    discard,
  };
}

function validActions(actions: AzureDevOpsActionPreset[]): boolean {
  return (
    actions.length > 0 &&
    actions.every((action) => action.label.trim() && action.promptTemplate.trim())
  );
}

export function AzureDevOpsQuickActionsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation();
  const drafts = useActionDrafts(workspaceId, t);
  const { toast } = useToast();
  const valid = validActions(drafts.workItems) && validActions(drafts.pullRequests);
  const save = useCallback(async () => {
    try {
      await drafts.save();
      toast({ description: t("azuredevops:quickActionsSaved"), variant: "success" });
    } catch (error) {
      toast({
        description:
          error instanceof Error ? error.message : t("azuredevops:failedToSaveQuickActions"),
        variant: "error",
      });
      throw error;
    }
  }, [drafts, t, toast]);

  useSettingsSaveContributor({
    id: `azure-devops-actions:${workspaceId}`,
    revision: JSON.stringify([drafts.workItems, drafts.pullRequests]),
    isDirty: drafts.dirty,
    canSave: !drafts.loading && !drafts.loadError && valid,
    invalidReason: drafts.loadError ?? (valid ? undefined : t("azuredevops:quickActionInvalid")),
    save,
    discard: drafts.discard,
  });

  return (
    <SettingsSection
      title={t("azuredevops:quickActions")}
      description={t("azuredevops:quickActionsDescription", { path: AZURE_DEVOPS_ROUTE })}
      action={
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-11 w-full cursor-pointer sm:h-8 sm:w-auto"
          disabled={drafts.loading || !!drafts.loadError}
          onClick={drafts.reset}
        >
          <IconRefresh className="h-4 w-4" /> {t("common:reset")}
        </Button>
      }
    >
      <SettingsCard isDirty={drafts.dirty} data-testid="azure-devops-quick-actions-card">
        <CardContent className="pt-4 sm:pt-6">
          {drafts.loadError && (
            <p role="alert" className="mb-4 text-sm text-destructive">
              {t("azuredevops:couldNotLoadQuickActions", { error: drafts.loadError })}
            </p>
          )}
          <fieldset disabled={drafts.loading || !!drafts.loadError} className="contents">
            <Tabs defaultValue="pull-request">
              <TabsList className="w-full sm:w-auto">
                <TabsTrigger value="pull-request" className="flex-1 cursor-pointer sm:flex-none">
                  {t("azuredevops:pullRequestsTab")}
                </TabsTrigger>
                <TabsTrigger value="work-item" className="flex-1 cursor-pointer sm:flex-none">
                  {t("azuredevops:workItemsTab")}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="pull-request">
                <ActionEditor
                  kind="pull-request"
                  actions={drafts.pullRequests}
                  baseline={drafts.baseline.pullRequests}
                  onChange={drafts.setPullRequests}
                />
              </TabsContent>
              <TabsContent value="work-item">
                <ActionEditor
                  kind="work-item"
                  actions={drafts.workItems}
                  baseline={drafts.baseline.workItems}
                  onChange={drafts.setWorkItems}
                />
              </TabsContent>
            </Tabs>
          </fieldset>
        </CardContent>
      </SettingsCard>
    </SettingsSection>
  );
}
