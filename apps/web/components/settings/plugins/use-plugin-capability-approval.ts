"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import {
  getPluginCapabilityApprovalContext,
  revokePluginCapabilityApproval,
  updatePluginCapabilityApproval,
} from "@/lib/api/domains/plugins-api";
import { generateUUID } from "@/lib/uuid";
import type { PluginCapabilityApprovalContext } from "@/lib/types/plugins";

type ApprovalState = {
  context: PluginCapabilityApprovalContext | null;
  selectedCapabilityIds: string[];
  setSelectedCapabilityIds: Dispatch<SetStateAction<string[]>>;
  reason: string;
  setReason: Dispatch<SetStateAction<string>>;
  loading: boolean;
  errorKey: string | null;
  setErrorKey: Dispatch<SetStateAction<string | null>>;
  status: string;
  setStatus: Dispatch<SetStateAction<string>>;
  reload: (preserveError?: boolean) => Promise<void>;
};

export function usePluginCapabilityApproval(pluginId: string, workspaceId: string) {
  const { t } = useTranslation();
  const state = useApprovalState(pluginId, workspaceId);
  const selection = useApprovalSelection(state);
  const actions = useApprovalActions({
    pluginId,
    workspaceId,
    state,
    canSave: selection.canSave,
    t,
  });

  return {
    ...state,
    ...actions,
    ...selection,
    canSave: selection.canSave && !actions.saving && !actions.revoking,
    canRevoke: hasRevokeReason(state),
  };
}

function useApprovalSelection(state: ApprovalState) {
  const toggleCapability = useCallback(
    (capabilityId: string, checked: boolean) => {
      state.setSelectedCapabilityIds((current) => {
        const next = new Set(current);
        if (checked) next.add(capabilityId);
        else next.delete(capabilityId);
        return Array.from(next).sort();
      });
    },
    [state.setSelectedCapabilityIds],
  );
  const unchanged = sameAsApprovedSelection(state);
  const canSave =
    state.context !== null &&
    !state.loading &&
    !unchanged &&
    state.selectedCapabilityIds.length > 0 &&
    state.reason.trim().length > 0;
  return { toggleCapability, canSave };
}

function sameAsApprovedSelection(state: ApprovalState): boolean {
  const approval = state.context?.approval;
  if (!state.context || approval?.state !== "active" || state.context.requires_review) return false;
  if (approval.manifest_digest !== state.context.manifest_digest) return false;
  const approved = [...approval.capability_ids].sort();
  return (
    approved.length === state.selectedCapabilityIds.length &&
    approved.every((capability, index) => capability === state.selectedCapabilityIds[index])
  );
}

function hasRevokeReason(state: ApprovalState): boolean {
  return state.context?.approval?.state === "active" && state.reason.trim().length > 0;
}

function useApprovalState(pluginId: string, workspaceId: string): ApprovalState {
  const [context, setContext] = useState<PluginCapabilityApprovalContext | null>(null);
  const [selectedCapabilityIds, setSelectedCapabilityIds] = useState<string[]>([]);
  const [reason, setReason] = useState("");
  const [loading, setLoading] = useState(true);
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const [status, setStatus] = useState("");
  const requestSequence = useRef(0);
  const reload = useCallback(
    async (preserveError = false) => {
      if (!workspaceId) {
        setContext(null);
        setSelectedCapabilityIds([]);
        setLoading(false);
        return;
      }
      const sequence = ++requestSequence.current;
      setLoading(true);
      setStatus("");
      if (!preserveError) setErrorKey(null);
      try {
        const response = await getPluginCapabilityApprovalContext(pluginId, workspaceId);
        if (sequence !== requestSequence.current) return;
        setContext(response);
        const declared = new Set(response.declared_capability_ids);
        setSelectedCapabilityIds(
          response.approval?.state === "active"
            ? response.approval.capability_ids.filter((id) => declared.has(id)).sort()
            : [],
        );
      } catch {
        if (sequence !== requestSequence.current) return;
        setContext(null);
        setErrorKey("plugins:failedToLoadCapabilityApproval");
      } finally {
        if (sequence === requestSequence.current) setLoading(false);
      }
    },
    [pluginId, workspaceId],
  );

  useEffect(() => {
    void reload();
    return () => {
      requestSequence.current += 1;
    };
  }, [reload]);

  return {
    context,
    selectedCapabilityIds,
    setSelectedCapabilityIds,
    reason,
    setReason,
    loading,
    errorKey,
    setErrorKey,
    status,
    setStatus,
    reload,
  };
}

function useApprovalActions({
  pluginId,
  workspaceId,
  state,
  canSave,
  t,
}: {
  pluginId: string;
  workspaceId: string;
  state: ApprovalState;
  canSave: boolean;
  t: TFunction;
}) {
  const [saving, setSaving] = useState(false);
  const [revoking, setRevoking] = useState(false);
  const save = useCallback(async () => {
    if (!state.context || !canSave || saving || revoking) return;
    setSaving(true);
    state.setErrorKey(null);
    state.setStatus(t("plugins:savingCapabilityApproval"));
    try {
      await updatePluginCapabilityApproval(pluginId, {
        workspace_id: workspaceId,
        expected_revision: state.context.approval?.revision ?? 0,
        manifest_digest: state.context.manifest_digest,
        capability_ids: [...state.selectedCapabilityIds].sort(),
        reason: state.reason.trim(),
        audit_id: generateUUID(),
      });
      state.setReason("");
      await state.reload();
      state.setStatus(t("plugins:capabilityApprovalSaved"));
    } catch (error) {
      state.setErrorKey(
        error instanceof ApiError && error.status === 409
          ? "plugins:staleCapabilityApproval"
          : "plugins:failedToSaveCapabilityApproval",
      );
      if (error instanceof ApiError && error.status === 409) await state.reload(true);
      state.setStatus("");
    } finally {
      setSaving(false);
    }
  }, [canSave, pluginId, revoking, saving, state, t, workspaceId]);
  const revoke = useCallback(async () => {
    const approval = state.context?.approval;
    if (!approval || approval.state !== "active" || revoking || saving || !state.reason.trim())
      return;
    setRevoking(true);
    state.setErrorKey(null);
    state.setStatus(t("plugins:revokingCapabilityApproval"));
    try {
      await revokePluginCapabilityApproval(pluginId, {
        workspace_id: workspaceId,
        expected_revision: approval.revision,
        reason: state.reason.trim(),
        audit_id: generateUUID(),
      });
      state.setReason("");
      await state.reload();
      state.setStatus(t("plugins:capabilityApprovalRevoked"));
    } catch (error) {
      state.setErrorKey(
        error instanceof ApiError && error.status === 409
          ? "plugins:staleCapabilityApproval"
          : "plugins:failedToRevokeCapabilityApproval",
      );
      if (error instanceof ApiError && error.status === 409) await state.reload(true);
      state.setStatus("");
    } finally {
      setRevoking(false);
    }
  }, [pluginId, revoking, saving, state, t, workspaceId]);

  return { saving, revoking, save, revoke };
}
