"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useRouter } from "@/lib/routing/client-router";
import { runWithNavigationBlockerBypassed } from "@/lib/routing/navigation-guard";
import { ApiError } from "@/lib/api/client";
import { toast } from "@/lib/toast/sonner";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useCoordinator } from "@/hooks/domains/settings/use-coordinator";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import {
  buildPatchCoordinatorPayload,
  coordinatorFormFromRecord,
  type CoordinatorFormState,
} from "@/lib/coordinators/coordinator-form";
import { isValidCoordinatorName } from "@/lib/coordinators/validate-form";
import { coordinatorFieldError, type CoordinatorFieldError } from "@/lib/coordinators/field-error";
import { CoordinatorFormFields } from "./coordinator-form-fields";
import { CoordinatorDeleteConfirmDialog } from "./coordinator-delete-confirm-dialog";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { CoordinatorSections } from "./sections/coordinator-sections";
import type { WorkspaceState } from "@/lib/state/slices";

type Workspace = WorkspaceState["items"][number];

const EMPTY_FORM: CoordinatorFormState = {
  name: "",
  agentProfileId: "",
  executorProfileId: "",
  context: "",
};

type CoordinatorEditorPageProps = {
  workspaceId: string;
  coordinatorId: string;
};

function coordinatorsListHref(workspaceId: string): string {
  return `/settings/workspaces/${workspaceId}/coordinators`;
}

type CoordinatorStatusViewProps = {
  status: "loading" | "error" | "not-found";
  workspaceId: string;
  refresh: () => void;
};

// Handles the three non-editable states (B3) so the main component below
// stays focused on the loaded editor.
function CoordinatorStatusView({ status, workspaceId, refresh }: CoordinatorStatusViewProps) {
  const { t } = useTranslation();

  if (status === "loading") {
    return (
      <div
        className="py-12 text-center text-muted-foreground"
        data-testid="coordinator-editor-loading"
      >
        {t("coordinator:loadingCoordinators")}
      </div>
    );
  }

  if (status === "error") {
    return (
      <div
        className="space-y-4 py-12 text-center text-muted-foreground"
        data-testid="coordinator-editor-load-error"
      >
        <p>{t("coordinator:loadError")}</p>
        <Button type="button" variant="outline" className="cursor-pointer" onClick={refresh}>
          {t("coordinator:retry")}
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-4 py-12 text-center" data-testid="coordinator-not-found">
      <p className="font-medium">{t("coordinator:notFoundTitle")}</p>
      <p className="text-sm text-muted-foreground">{t("coordinator:notFoundDescription")}</p>
      <Link
        href={coordinatorsListHref(workspaceId)}
        data-testid="all-coordinators-link"
        className="text-sm text-primary hover:underline"
      >
        {t("coordinator:allCoordinators")}
      </Link>
    </div>
  );
}

// Owns form state, the settings-save contributor, and delete handling so the
// component below stays focused on rendering.
function useCoordinatorEditorForm(workspaceId: string, coordinatorId: string) {
  const { t } = useTranslation();
  const router = useRouter();
  const { coordinator, status, refresh, patch, remove } = useCoordinator(
    workspaceId,
    coordinatorId,
  );
  const workspace = useAppStore(
    (state) => state.workspaces.items.find((item: Workspace) => item.id === workspaceId) ?? null,
  );
  const canManage = hasScope(workspace?.scopes, SCOPE.workspaceManage);

  const [form, setForm] = useState<CoordinatorFormState>(EMPTY_FORM);
  const [savedForm, setSavedForm] = useState<CoordinatorFormState>(EMPTY_FORM);
  const [fieldError, setFieldError] = useState<CoordinatorFieldError | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  // Populates the form once per loaded coordinator, not on every `coordinator`
  // reference change: patch() and refresh() both replace that reference after
  // a save, and resetting form/savedForm from that response would discard any
  // edit the user made during the round trip. handleSave below advances
  // savedForm to the submitted snapshot instead of relying on a reset here.
  const initializedForRef = useRef<string | null>(null);
  useEffect(() => {
    if (status !== "ready" || !coordinator) return;
    if (initializedForRef.current === coordinatorId) return;
    const next = coordinatorFormFromRecord(coordinator);
    setForm(next);
    setSavedForm(next);
    initializedForRef.current = coordinatorId;
  }, [status, coordinator, coordinatorId]);

  const patchPayload = buildPatchCoordinatorPayload(form, savedForm);
  const isDirty = canManage && status === "ready" && Object.keys(patchPayload).length > 0;
  const canSave =
    isValidCoordinatorName(form.name) &&
    Boolean(form.agentProfileId) &&
    Boolean(form.executorProfileId);

  const handleSave = async () => {
    setFieldError(null);
    const submitted: CoordinatorFormState = { ...form, name: form.name.trim() };
    try {
      await patch(patchPayload);
      setSavedForm(submitted);
      refresh();
    } catch (error) {
      if (error instanceof ApiError && error.status === 404) {
        refresh();
      } else if (error instanceof ApiError && error.status === 400) {
        setFieldError(coordinatorFieldError(error));
      } else {
        toast.error(t("coordinator:failedToSaveCoordinator"));
      }
      throw error;
    }
  };

  const handleDiscard = () => {
    setForm(savedForm);
    setFieldError(null);
  };

  useSettingsSaveContributor({
    id: `coordinator:${coordinatorId}`,
    revision: JSON.stringify(form),
    isDirty,
    canSave,
    invalidReason: canSave ? undefined : t("coordinator:nameRequiredReason"),
    save: handleSave,
    discard: handleDiscard,
  });

  const handleDelete = async () => {
    if (deleting) return;
    setDeleting(true);
    try {
      await remove();
      // The coordinator is already gone: the dirty-navigation guard
      // (registered whenever this page's save contributor is dirty) must not
      // prompt to confirm leaving a record that no longer exists. Mirrors
      // automation-editor.tsx's useRemoveAutomation.
      runWithNavigationBlockerBypassed(() => router.replace(coordinatorsListHref(workspaceId)));
    } catch {
      toast.error(t("coordinator:failedToDeleteCoordinator"));
    } finally {
      setDeleting(false);
    }
  };

  return {
    coordinator,
    status,
    refresh,
    canManage,
    form,
    setForm,
    savedForm,
    fieldError,
    deleteDialogOpen,
    setDeleteDialogOpen,
    deleting,
    handleDelete,
  };
}

export function CoordinatorEditorPage({ workspaceId, coordinatorId }: CoordinatorEditorPageProps) {
  const { t } = useTranslation();
  useSettingsData(true);
  const phase2 = useFeature("coordinatorPhase2");
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const executors = useAppStore((state) => state.executors.items);
  const {
    coordinator,
    status,
    refresh,
    canManage,
    form,
    setForm,
    savedForm,
    fieldError,
    deleteDialogOpen,
    setDeleteDialogOpen,
    deleting,
    handleDelete,
  } = useCoordinatorEditorForm(workspaceId, coordinatorId);

  if (status === "loading" || status === "error" || status === "not-found") {
    return <CoordinatorStatusView status={status} workspaceId={workspaceId} refresh={refresh} />;
  }

  const identity = (
    <div className="space-y-6">
      <CoordinatorFormFields
        form={form}
        onChange={(key, value) => setForm((prev) => ({ ...prev, [key]: value }))}
        disabled={!canManage}
        agentProfiles={agentProfiles}
        executors={executors}
        agentProfileStatus={coordinator?.agent_profile_status}
        executorProfileStatus={coordinator?.executor_profile_status}
        fieldError={fieldError}
      />
      <div className="space-y-1 text-sm text-muted-foreground">
        <p>{t("coordinator:contextRestartNote")}</p>
        <p>{t("coordinator:autoApproveIgnoredNote")}</p>
      </div>
      {canManage && (
        <>
          <Button
            type="button"
            variant="destructive"
            data-testid="delete-coordinator-button"
            className="cursor-pointer"
            onClick={() => setDeleteDialogOpen(true)}
          >
            {t("coordinator:deleteCoordinator")}
          </Button>
          <CoordinatorDeleteConfirmDialog
            open={deleteDialogOpen}
            coordinatorName={savedForm.name}
            isDeleting={deleting}
            onOpenChange={(open) => {
              if (!deleting) setDeleteDialogOpen(open);
            }}
            onConfirm={handleDelete}
          />
        </>
      )}
    </div>
  );

  return (
    <div className="max-w-2xl space-y-6" data-testid="coordinator-editor-page">
      <Link
        href={coordinatorsListHref(workspaceId)}
        data-testid="all-coordinators-link"
        className="text-sm text-primary hover:underline"
      >
        {t("coordinator:allCoordinators")}
      </Link>
      {phase2 ? (
        <CoordinatorSections
          workspaceId={workspaceId}
          coordinatorId={coordinatorId}
          canManage={canManage}
          identity={identity}
        />
      ) : (
        identity
      )}
    </div>
  );
}
