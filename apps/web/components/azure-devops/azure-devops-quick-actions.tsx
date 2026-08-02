"use client";

import { createElement, useCallback, useEffect, useMemo, useState } from "react";
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

const ACTION_PROMPT_PLACEHOLDERS: ScriptPlaceholder[] = [
  {
    key: "url",
    description: "URL of the Azure DevOps work item or pull request",
    example: "https://dev.azure.com/acme/Platform/_workitems/edit/42",
    executor_types: [],
  },
  {
    key: "title",
    description: "Title of the Azure DevOps work item or pull request",
    example: "Rotate integration credentials",
    executor_types: [],
  },
];

type ActionKind = "Work item" | "Pull request";

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
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger
        className="h-11 w-full cursor-pointer sm:h-8"
        aria-label="Icon"
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
                {choice.label}
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
  return (
    <div
      className="rounded-md border"
      data-settings-dirty={JSON.stringify(action) !== JSON.stringify(baseline)}
      data-settings-dirty-level="container"
    >
      <div className="grid grid-cols-2 gap-2 p-3 sm:grid-cols-[5rem_10rem_minmax(0,1fr)_auto_auto] sm:items-end sm:p-2">
        <Field label="Icon">
          <ActionIconSelect
            value={action.icon}
            dirty={action.icon !== baseline?.icon}
            onChange={(icon) => onPatch({ icon })}
          />
        </Field>
        <Field label="Label">
          <Input
            className="h-11 w-full sm:h-8"
            value={action.label}
            aria-label={`${kind} action label ${index + 1}`}
            data-settings-dirty={action.label !== baseline?.label}
            onChange={(event) => onPatch({ label: event.target.value })}
          />
        </Field>
        <Field label="Hint" className="col-span-2 sm:col-span-1">
          <Input
            className="h-11 w-full sm:h-8"
            value={action.hint}
            aria-label={`${kind} action hint ${index + 1}`}
            placeholder="Hint (optional)"
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
          {expanded ? "Hide prompt" : "Edit prompt"}
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-11 w-full cursor-pointer text-destructive sm:h-8 sm:w-8"
          onClick={onRemove}
          aria-label={`Remove ${kind.toLowerCase()} action ${index + 1}`}
        >
          <IconTrash className="h-4 w-4" />
        </Button>
      </div>
      {expanded && (
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
              placeholders={ACTION_PROMPT_PLACEHOLDERS}
            />
          </div>
          <p className="text-[11px] text-muted-foreground/60">
            Type {"{{"} to see available placeholders. <code>{"{{url}}"}</code> and{" "}
            <code>{"{{title}}"}</code> are substituted when the action runs.
          </p>
        </div>
      )}
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
        <IconPlus className="h-4 w-4" /> Add {kind === "Pull request" ? "PR" : "work item"} action
      </Button>
    </div>
  );
}

function useActionDrafts(workspaceId: string) {
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
        setLoadError(error instanceof Error ? error.message : "Failed to load quick actions.");
      })
      .finally(() => current && setLoading(false));
    return () => {
      current = false;
    };
  }, [workspaceId]);

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
  const drafts = useActionDrafts(workspaceId);
  const { toast } = useToast();
  const valid = validActions(drafts.workItems) && validActions(drafts.pullRequests);
  const save = useCallback(async () => {
    try {
      await drafts.save();
      toast({ description: "Azure DevOps quick actions saved", variant: "success" });
    } catch (error) {
      toast({
        description: error instanceof Error ? error.message : "Failed to save quick actions",
        variant: "error",
      });
      throw error;
    }
  }, [drafts, toast]);

  useSettingsSaveContributor({
    id: `azure-devops-actions:${workspaceId}`,
    revision: JSON.stringify([drafts.workItems, drafts.pullRequests]),
    isDirty: drafts.dirty,
    canSave: !drafts.loading && !drafts.loadError && valid,
    invalidReason:
      drafts.loadError ?? (valid ? undefined : "Every quick action needs a label and prompt."),
    save,
    discard: drafts.discard,
  });

  return (
    <SettingsSection
      title="Quick actions"
      description="Prompts shown on /azure-devops when starting a task from a pull request or work item."
      action={
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="h-11 w-full cursor-pointer sm:h-8 sm:w-auto"
          disabled={drafts.loading || !!drafts.loadError}
          onClick={drafts.reset}
        >
          <IconRefresh className="h-4 w-4" /> Reset
        </Button>
      }
    >
      <SettingsCard isDirty={drafts.dirty} data-testid="azure-devops-quick-actions-card">
        <CardContent className="pt-4 sm:pt-6">
          {drafts.loadError && (
            <p role="alert" className="mb-4 text-sm text-destructive">
              Could not load existing quick actions: {drafts.loadError}
            </p>
          )}
          <fieldset disabled={drafts.loading || !!drafts.loadError} className="contents">
            <Tabs defaultValue="pull-request">
              <TabsList className="w-full sm:w-auto">
                <TabsTrigger value="pull-request" className="flex-1 cursor-pointer sm:flex-none">
                  Pull requests
                </TabsTrigger>
                <TabsTrigger value="work-item" className="flex-1 cursor-pointer sm:flex-none">
                  Work items
                </TabsTrigger>
              </TabsList>
              <TabsContent value="pull-request">
                <ActionEditor
                  kind="Pull request"
                  actions={drafts.pullRequests}
                  baseline={drafts.baseline.pullRequests}
                  onChange={drafts.setPullRequests}
                />
              </TabsContent>
              <TabsContent value="work-item">
                <ActionEditor
                  kind="Work item"
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
