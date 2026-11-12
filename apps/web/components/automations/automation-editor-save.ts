import type React from "react";
import { t } from "@/lib/i18n";
import { toast } from "@/lib/toast/sonner";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import type { useRouter } from "@/lib/routing/client-router";
import type { useAutomationTriggerDrafts } from "./automation-trigger-drafts";
import type {
  Automation,
  AutomationTrigger,
  CreateAutomationRequest,
  CreateAutomationResponse,
  UpdateAutomationRequest,
} from "@/lib/types/automation";
import {
  type FormState,
  type CreatedWebhookDetails,
  canonicalPluginTools,
  mcpSelectionForUpdate,
  resolveRepositoryIdsForMode,
  buildCreatePayload,
  buildUpdatePayload,
  buildWebhookUrl,
  reconcileSavedMCPSelection,
  withSavedMCPSelection,
} from "./automation-payload";

import type { RepositorySelection } from "./automation-repository-selection";

export type SaveHandlerOpts = {
  isNew: boolean;
  workspaceId: string;
  form: FormState;
  savedForm: FormState;
  currentId: string | null;
  // supportsMultiRepo mirrors what ConfigSection's picker rendered for this
  // save. See normalizeRepositorySelections.
  supportsMultiRepo: boolean;
  create: (payload: CreateAutomationRequest) => Promise<CreateAutomationResponse>;
  update: (id: string, payload: UpdateAutomationRequest) => Promise<Automation>;
  setSaving: React.Dispatch<React.SetStateAction<boolean>>;
  setCurrentId: React.Dispatch<React.SetStateAction<string | null>>;
  setForm: React.Dispatch<React.SetStateAction<FormState>>;
  // setCreatedWebhook surfaces the URL + secret in a dialog after creating
  // a webhook automation, then the user is redirected to the listings page.
  // Null when no webhook trigger was configured on the new automation.
  setCreatedWebhook: React.Dispatch<React.SetStateAction<CreatedWebhookDetails | null>>;
  onSaved: (form: FormState, triggers: AutomationTrigger[]) => void;
  // onPluginToolsSaveFailed refreshes the options catalog after a rejected
  // save so a changed policy is visible for explicit reselection without
  // discarding other dirty fields.
  onPluginToolsSaveFailed: () => void;
  onMCPSelectionSaveFailed?: () => void;
  triggerActions: ReturnType<typeof useAutomationTriggerDrafts>;
  router: ReturnType<typeof useRouter>;
};

/** Promote accepted registrations without overwriting repository edits made during the request. */
function reconcilePromotedRepositories(
  current: FormState,
  submitted: FormState,
  promoted: RepositorySelection[],
): FormState {
  const unchangedContext =
    current.repositoryMode === submitted.repositoryMode &&
    current.executorProfileId === submitted.executorProfileId &&
    current.taskMode === submitted.taskMode;
  if (current.repositorySelections === submitted.repositorySelections && unchangedContext)
    return { ...current, repositorySelections: promoted };
  return {
    ...current,
    repositorySelections: current.repositorySelections.map((item) => {
      if (item.kind !== "discovered") return item;
      const index = submitted.repositorySelections.findIndex(
        (source) =>
          source.kind === "discovered" && source.path === item.path && source.key === item.key,
      );
      const source = submitted.repositorySelections[index];
      const accepted = promoted[index];
      if (!source || accepted?.kind !== "registered") return item;
      return { ...accepted, branch: item.branch === source.branch ? accepted.branch : item.branch };
    }),
  };
}

/**
 * Save the automation and reconcile server revisions without overwriting edits made during the
 * request.
 */
export function useSaveHandler(opts: SaveHandlerOpts): () => Promise<void> {
  const { isNew, workspaceId, form, savedForm, currentId, create, update, supportsMultiRepo } =
    opts;
  const { setSaving, setCurrentId, setForm, setCreatedWebhook, triggerActions, router, onSaved } =
    opts;
  const { onPluginToolsSaveFailed } = opts;
  return async () => {
    const pluginToolsChanged =
      JSON.stringify(canonicalPluginTools(form.pluginTools)) !==
      JSON.stringify(canonicalPluginTools(savedForm.pluginTools));
    const pluginToolsSubmitted = isNew ? form.pluginTools.length > 0 : pluginToolsChanged;
    setSaving(true);
    try {
      // The picker only ever *renders* repositorySelections[0] once the
      // executor stops supporting multi-repo. It doesn't truncate the
      // underlying array, so resolveNormalizedRepositoryIds re-enforces the
      // same invariant right before persisting.
      const repositoryMode =
        form.taskMode === "managed_conversation" ? "none" : form.repositoryMode;
      const { repositories, selections: promotedSelections } = await resolveRepositoryIdsForMode(
        workspaceId,
        form.repositorySelections,
        repositoryMode,
        {
          supportsMultiRepo,
        },
      );
      const promoteSelections = () => {
        setForm((prev) => reconcilePromotedRepositories(prev, form, promotedSelections));
      };
      const formWithPromotedSelections = { ...form, repositorySelections: promotedSelections };
      if (isNew) {
        const a = await create(
          buildCreatePayload(workspaceId, form, repositories, triggerActions.pending),
        );
        promoteSelections();
        // Webhook automations need their URL + secret communicated to the
        // user; show the dialog and let its close handler do the redirect.
        // Everything else goes straight to the listings page with a toast.
        const hasWebhookTrigger = (a.triggers ?? []).some((t) => t.type === "webhook");
        if (hasWebhookTrigger && a.webhook_secret) {
          const savedTriggers = a.triggers ?? [];
          triggerActions.loadTriggers(savedTriggers);
          triggerActions.clearPending();
          setForm((prev) => reconcileSavedMCPSelection(prev, form, a));
          onSaved(withSavedMCPSelection(formWithPromotedSelections, a), savedTriggers);
          setCurrentId(a.id);
          setCreatedWebhook({ url: buildWebhookUrl(a.id), secret: a.webhook_secret });
        } else {
          toast.success(t("automations:automationCreated"));
          runWithNavigationBlockerBypassed(() =>
            router.push(`/settings/workspaces/${workspaceId}/automations`),
          );
        }
      } else if (currentId) {
        const request = buildUpdatePayload(
          form,
          repositories,
          pluginToolsChanged ? form.pluginTools : undefined,
          mcpSelectionForUpdate(form, savedForm),
          form.taskMode !== savedForm.taskMode,
        );
        const saved = await update(currentId, request).catch((error) => {
          if (request.expected_mcp_selection_revision !== undefined)
            opts.onMCPSelectionSaveFailed?.();
          throw error;
        });
        const persistedForm = withSavedMCPSelection(formWithPromotedSelections, saved);
        setForm((prev) =>
          reconcilePromotedRepositories(
            reconcileSavedMCPSelection(prev, form, saved),
            form,
            promotedSelections,
          ),
        );
        // Automation settings are saved independently of trigger drafts.
        onSaved(persistedForm, triggerActions.baselineTriggers);

        const persistedTriggers = await triggerActions.persistDrafts();
        onSaved(persistedForm, persistedTriggers);
      }
    } catch (err) {
      if (pluginToolsSubmitted) {
        onPluginToolsSaveFailed();
      }
      // The interpolated value is an API/network diagnostic and stays English
      // by design — see docs/i18n.md ("interpolated value" limit).
      const msg = err instanceof Error ? err.message : String(err);
      toast.error(t("automations:failedToSaveAutomation", { error: msg }));
      throw err;
    } finally {
      setSaving(false);
    }
  };
}
