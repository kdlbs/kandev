"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@/lib/toast/sonner";
import { ApiError } from "@/lib/api/client";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { WorkspaceRouting } from "@/lib/state/slices/office/types";
import type {
  AuthorExclusionStepChange,
  AuthorExclusionWarning,
} from "@/lib/types/author-exclusion";

export type RoutingSaveArgs = {
  /** The page's routing draft; null until the config has loaded. */
  draft: WorkspaceRouting | null;
  /** The stored routing config the draft is compared with and reset to. */
  saved: WorkspaceRouting;
  exclusionChanges: AuthorExclusionStepChange[];
  currentWarnings: AuthorExclusionWarning[];
  setDraft: (cfg: WorkspaceRouting) => void;
  resetExclusion: () => void;
  updateRouting: (cfg: WorkspaceRouting) => Promise<AuthorExclusionWarning[]>;
  updateAuthorExclusion: (
    changes: AuthorExclusionStepChange[],
  ) => Promise<AuthorExclusionWarning[]>;
  onSaved: () => void;
};

/**
 * Registers the routing page with the shared settings save bar. One save
 * writes the routing draft first, then the Review on a different provider
 * switches, so the warnings returned by the last write reflect both. A failed
 * write is rethrown so the save bar reports it.
 */
export function useRoutingSave(args: RoutingSaveArgs) {
  const { t } = useTranslation();
  const [saving, setSaving] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<string[]>([]);
  const { draft, saved, exclusionChanges, setDraft, resetExclusion } = args;

  const routingDirty = useMemo(
    () => !!draft && JSON.stringify(draft) !== JSON.stringify(saved),
    [draft, saved],
  );
  const isDirty = routingDirty || exclusionChanges.length > 0;
  const revision = JSON.stringify({ draft, exclusionChanges });

  const save = useCallback(async () => {
    if (!draft) return;
    setSaving(true);
    setFieldErrors([]);
    try {
      let warnings = args.currentWarnings;
      if (routingDirty) warnings = await args.updateRouting(draft);
      if (exclusionChanges.length > 0) {
        warnings = await args.updateAuthorExclusion(exclusionChanges);
      }
      args.onSaved();
      if (warnings.length > 0) toast.warning(t("office:routingSavedWithExclusionWarnings"));
      else toast.success(t("office:routingSettingsSaved"));
    } catch (err) {
      const errs = extractValidationDetails(err);
      if (errs.length > 0) setFieldErrors(errs);
      toast.error(err instanceof Error ? err.message : t("office:failedToSave"));
      throw err;
    } finally {
      setSaving(false);
    }
  }, [args, draft, routingDirty, exclusionChanges, t]);

  const discard = useCallback(() => {
    setDraft(saved);
    resetExclusion();
    setFieldErrors([]);
  }, [saved, setDraft, resetExclusion]);

  useSettingsSaveContributor({
    id: "office-routing",
    revision,
    isDirty,
    save,
    discard,
  });

  return { saving, fieldErrors, discard };
}

/**
 * Composes the BACKEND's own field names and error messages. Protocol, not copy
 * — the strings here are echoed from the API response, never authored in the UI.
 */
function extractValidationDetails(err: unknown): string[] {
  if (!(err instanceof ApiError)) return [];
  const body = err.body;
  if (!body || typeof body !== "object") return [];
  const out: string[] = [];
  const obj = body as {
    error?: string;
    field?: string;
    details?: Array<{ provider_id?: string; field?: string; message?: string }>;
  };
  if (obj.error && obj.field) out.push(`${obj.field}: ${obj.error}`);
  if (Array.isArray(obj.details)) {
    for (const d of obj.details) {
      if (d?.message) {
        const prefix = d.provider_id ? `${d.provider_id}: ` : "";
        out.push(`${prefix}${d.message}`);
      }
    }
  }
  return out;
}
