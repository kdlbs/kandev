"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { Dispatch, SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import { useRouter } from "@/lib/routing/client-router";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { serializeSettingsRevision } from "@/components/settings/settings-save-revision";
import { useToast } from "@/components/toast-provider";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  buildExecutorProfileValues,
  parseExecutorProfileSchema,
  serializeExecutorProfileValues,
  validateExecutorProfileValues,
  type ExecutorProfileFieldErrors,
} from "@/lib/plugins/executor-profile-schema";
import {
  executorProviderUnavailableReason,
  executorProviderRetentionText,
} from "@/lib/executor-provider-display";
import {
  deleteExecutorProfile,
  listExecutorProfiles,
  updateExecutorProfile,
} from "@/lib/api/domains/settings-api";
import { upsertExecutorProfile } from "@/components/settings/profile-edit/profile-edit-page-chrome";
import type { Executor, ExecutorProfile } from "@/lib/types/http";

type Translate = (key: string, options?: Record<string, unknown>) => string;

function sameValues(left: Record<string, string>, right: Record<string, string>): boolean {
  return (
    Object.keys(left).length === Object.keys(right).length &&
    Object.keys(left).every((key) => left[key] === right[key])
  );
}

export function usePluginExecutorProfileData(
  executor: Executor,
  profile: ExecutorProfile,
  t: Translate,
) {
  const setExecutors = useAppStore((state) => state.setExecutors);
  const appStore = useAppStoreApi();
  const [currentProfile, setCurrentProfile] = useState(profile);
  const [name, setName] = useState(profile.name);
  const [initialName, setInitialName] = useState(profile.name);
  const parsed = useMemo(
    () => parseExecutorProfileSchema(currentProfile.provider?.profile_schema),
    [currentProfile.provider?.profile_schema],
  );
  const [values, setValues] = useState(() =>
    buildExecutorProfileValues(parsed.fields, profile.config),
  );
  const [initialValues, setInitialValues] = useState(() =>
    buildExecutorProfileValues(parsed.fields, profile.config),
  );
  const [clearedSecrets, setClearedSecrets] = useState<Record<string, boolean>>({});
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setLoadError(null);
    listExecutorProfiles(executor.id, { cache: "no-store" })
      .then((response) => {
        if (cancelled) return;
        const fresh = response.profiles.find((item) => item.id === profile.id);
        if (!fresh) {
          setLoadError(t("executors:providerProfileLoadFailed"));
          return;
        }
        const freshSchema = parseExecutorProfileSchema(fresh.provider?.profile_schema);
        const freshValues = buildExecutorProfileValues(freshSchema.fields, fresh.config);
        setCurrentProfile(fresh);
        setName(fresh.name);
        setInitialName(fresh.name);
        setValues(freshValues);
        setInitialValues(freshValues);
        setClearedSecrets({});
        setExecutors(upsertExecutorProfile(appStore.getState().executors.items, executor, fresh));
      })
      .catch((reason) => {
        if (!cancelled) {
          setLoadError(
            reason instanceof Error ? reason.message : t("executors:providerProfileLoadFailed"),
          );
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // Route identifiers define the reload boundary; changing state does not refetch.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [executor.id, profile.id]);

  return {
    appStore,
    setExecutors,
    currentProfile,
    setCurrentProfile,
    name,
    setName,
    initialName,
    setInitialName,
    parsed,
    values,
    setValues,
    initialValues,
    setInitialValues,
    clearedSecrets,
    setClearedSecrets,
    loading,
    loadError,
  };
}

function profileInvalidReason(
  name: string,
  fields: ReturnType<typeof usePluginExecutorProfileData>["parsed"]["fields"],
  errors: ExecutorProfileFieldErrors,
  t: Translate,
): string | undefined {
  if (!name.trim()) return t("executors:providerProfileNameRequired");
  const field = fields.find((candidate) => errors[candidate.name]);
  if (!field) return undefined;
  return t("executors:providerFieldInvalid", { field: field.label });
}

async function persistPluginExecutorProfile({
  executor,
  profile,
  data,
  validationErrors,
  invalidReason,
  setSaving,
  setError,
  setFieldErrors,
  t,
  toast,
}: {
  executor: Executor;
  profile: ExecutorProfile;
  data: ReturnType<typeof usePluginExecutorProfileData>;
  validationErrors: ExecutorProfileFieldErrors;
  invalidReason?: string;
  setSaving: Dispatch<SetStateAction<boolean>>;
  setError: Dispatch<SetStateAction<string | null>>;
  setFieldErrors: Dispatch<SetStateAction<ExecutorProfileFieldErrors>>;
  t: Translate;
  toast: ReturnType<typeof useToast>["toast"];
}) {
  if (Object.keys(validationErrors).length > 0 || !data.name.trim()) {
    setFieldErrors(validationErrors);
    throw new Error(invalidReason ?? t("executors:providerProfileNameRequired"));
  }
  setSaving(true);
  setError(null);
  try {
    const updated = await updateExecutorProfile(executor.id, profile.id, {
      name: data.name.trim(),
      config: serializeExecutorProfileValues(data.parsed.fields, data.values, data.clearedSecrets),
    });
    const updatedSchema = parseExecutorProfileSchema(updated.provider?.profile_schema);
    const updatedValues = buildExecutorProfileValues(updatedSchema.fields, updated.config);
    data.setCurrentProfile(updated);
    data.setName(updated.name);
    data.setInitialName(updated.name);
    data.setValues(updatedValues);
    data.setInitialValues(updatedValues);
    data.setClearedSecrets({});
    setFieldErrors({});
    data.setExecutors(
      upsertExecutorProfile(data.appStore.getState().executors.items, executor, updated),
    );
    toast({ title: t("executors:profileSaved"), variant: "success" });
  } catch (reason) {
    const message = reason instanceof Error ? reason.message : t("executors:failedToSaveProfile");
    setError(message);
    throw reason instanceof Error ? reason : new Error(message);
  } finally {
    setSaving(false);
  }
}

function useRegisterPluginProfileSaveContributor({
  profile,
  revision,
  dirty,
  data,
  unavailableReason,
  invalidReason,
  save,
  discard,
}: {
  profile: ExecutorProfile;
  revision: string;
  dirty: boolean;
  data: ReturnType<typeof usePluginExecutorProfileData>;
  unavailableReason: string | null;
  invalidReason?: string;
  save: () => Promise<void>;
  discard: () => void;
}) {
  useSettingsSaveContributor({
    id: `executor-profile:${profile.id}`,
    revision,
    isDirty: dirty && !data.loading,
    canSave:
      !data.loading &&
      !data.loadError &&
      !unavailableReason &&
      data.parsed.supported &&
      !invalidReason,
    invalidReason,
    save,
    discard,
  });
}

function usePluginProfileFieldHandlers(
  data: ReturnType<typeof usePluginExecutorProfileData>,
  setFieldErrors: Dispatch<SetStateAction<ExecutorProfileFieldErrors>>,
) {
  const onValueChange = useCallback(
    (field: string, value: string) => {
      data.setValues((current) => ({ ...current, [field]: value }));
      if (value !== "") data.setClearedSecrets((current) => ({ ...current, [field]: false }));
      setFieldErrors((current) => {
        const next = { ...current };
        delete next[field];
        return next;
      });
    },
    [data],
  );
  return {
    onValueChange,
    onClearSecret: (field: string) =>
      data.setClearedSecrets((current) => ({ ...current, [field]: true })),
    onRestoreSecret: (field: string) =>
      data.setClearedSecrets((current) => ({ ...current, [field]: false })),
  };
}

export function usePluginExecutorProfileSave({
  executor,
  profile,
  data,
  t,
}: {
  executor: Executor;
  profile: ExecutorProfile;
  data: ReturnType<typeof usePluginExecutorProfileData>;
  t: Translate;
}) {
  const { toast } = useToast();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<ExecutorProfileFieldErrors>({});
  const fieldHandlers = usePluginProfileFieldHandlers(data, setFieldErrors);
  const configuredSecrets = data.currentProfile.secret_fields ?? {};
  const validationErrors = useMemo(
    () =>
      validateExecutorProfileValues(
        data.parsed.fields,
        data.values,
        configuredSecrets,
        data.clearedSecrets,
      ),
    [configuredSecrets, data.clearedSecrets, data.parsed.fields, data.values],
  );
  const dirty =
    data.name !== data.initialName ||
    !sameValues(data.values, data.initialValues) ||
    Object.values(data.clearedSecrets).some(Boolean);
  const unavailableReason = executorProviderUnavailableReason(
    data.currentProfile.provider,
    true,
    t,
  );
  const retentionText = executorProviderRetentionText(data.currentProfile.provider, t);
  const invalidReason = profileInvalidReason(data.name, data.parsed.fields, validationErrors, t);
  const revision = serializeSettingsRevision({
    name: data.name,
    values: data.values,
    clearedSecrets: data.clearedSecrets,
  });

  const discard = useCallback(() => {
    data.setName(data.initialName);
    data.setValues(data.initialValues);
    data.setClearedSecrets({});
    setFieldErrors({});
    setError(null);
  }, [data]);

  const save = useCallback(
    () =>
      persistPluginExecutorProfile({
        executor,
        profile,
        data,
        validationErrors,
        invalidReason,
        setSaving,
        setError,
        setFieldErrors,
        t,
        toast,
      }),
    [data, executor, invalidReason, profile, t, toast, validationErrors],
  );

  useRegisterPluginProfileSaveContributor({
    profile,
    revision,
    dirty,
    data,
    unavailableReason,
    invalidReason,
    save,
    discard,
  });

  return {
    configuredSecrets,
    validationErrors,
    dirty,
    unavailableReason,
    retentionText,
    saving,
    error,
    fieldErrors,
    setFieldErrors,
    ...fieldHandlers,
  };
}

export function usePluginExecutorProfileDelete(executor: Executor, profile: ExecutorProfile) {
  const router = useRouter();
  const appStore = useAppStoreApi();
  const setExecutors = useAppStore((state) => state.setExecutors);
  const { t } = useTranslation();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleDelete = useCallback(async () => {
    setDeleting(true);
    try {
      await deleteExecutorProfile(executor.id, profile.id);
      setExecutors(
        appStore
          .getState()
          .executors.items.map((item) =>
            item.id === executor.id
              ? { ...item, profiles: item.profiles?.filter((entry) => entry.id !== profile.id) }
              : item,
          ),
      );
      router.push("/settings/executors");
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t("executors:failedToSaveProfile"));
      setDeleting(false);
      setDeleteOpen(false);
    }
  }, [appStore, executor.id, profile.id, router, setExecutors, t]);

  return { deleteOpen, setDeleteOpen, deleting, error, handleDelete };
}
