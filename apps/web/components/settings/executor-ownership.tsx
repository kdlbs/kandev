"use client";

import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { CardContent } from "@kandev/ui/card";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useAssignablePeople } from "@/hooks/domains/users/use-assignable-people";
import { updateExecutor, updateExecutorProfile } from "@/lib/api/domains/settings-api";
import {
  executorOwnerChoices,
  reconcileExecutorOwnerDraft,
  acceptExecutorOwnerSave,
} from "@/lib/auth/executor-ownership";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { SettingsCard } from "./settings-card";
import { SettingsCardHeader } from "./settings-card-header";
import { ExecutorOwnerSelect } from "./executor-owner-select";
import { useSettingsSaveContributor } from "./settings-save-provider";

export function ExecutorOwnerChoice({
  value,
  onChange,
  parentOwner = "",
  creating = false,
  baseline = value,
  label,
}: {
  value: string;
  onChange: (owner: string) => void;
  parentOwner?: string;
  creating?: boolean;
  baseline?: string;
  label?: string;
}) {
  const { t } = useTranslation();
  const manager = useIsAdmin();
  const user = useAppStore((state) => state.auth.user);
  const { people } = useAssignablePeople(undefined, { enabled: manager && Boolean(user) });
  const choices = executorOwnerChoices({
    userId: user?.id ?? "",
    currentOwner: baseline,
    parentOwner,
    manager,
    creating,
    people,
  });
  const ownerLabel = (owner: string) => {
    if (!owner) return t("executors:ownershipShared");
    if (owner === user?.id) return t("executors:ownershipMe");
    return people.find((person) => person.id === owner)?.name ?? owner;
  };
  const title = label ?? t("executors:ownershipProfile");
  const disabled = !user || (!manager && !creating && baseline !== user.id);
  return (
    <SettingsCard isDirty={value !== baseline}>
      <SettingsCardHeader title={title} description={t("executors:ownershipDescription")} />
      <CardContent className="space-y-3">
        <ExecutorOwnerSelect
          value={value}
          onChange={onChange}
          title={title}
          disabled={disabled}
          options={choices.map((owner) => ({ value: owner, label: ownerLabel(owner) }))}
        />
        {parentOwner && (
          <p className="text-sm text-muted-foreground">{t("executors:ownershipInherited")}</p>
        )}
      </CardContent>
    </SettingsCard>
  );
}

export function ExecutorOwnershipCard({
  executor,
  profile,
}: {
  executor: Executor;
  profile?: ExecutorProfile;
}) {
  const { t } = useTranslation();
  const store = useAppStoreApi();
  const baseline = (profile ?? executor).owner_user_id ?? "";
  const [draft, setDraft] = useState({ value: baseline, baseline });
  const { value, baseline: saved } = draft;
  const setValue = (value: string) => setDraft((current) => ({ ...current, value }));
  useEffect(() => {
    setDraft((current) => reconcileExecutorOwnerDraft(current, baseline));
  }, [baseline]);
  const save = async () => {
    const submitted = value;
    if (profile) await updateExecutorProfile(executor.id, profile.id, { owner_user_id: submitted });
    else await updateExecutor(executor.id, { owner_user_id: submitted });
    setDraft((current) => acceptExecutorOwnerSave(current, submitted));
    const current = store.getState().executors.items;
    store.getState().setExecutors(
      current.map((item) => {
        if (item.id !== executor.id) return item;
        if (!profile) return { ...item, owner_user_id: submitted };
        return {
          ...item,
          profiles: item.profiles?.map((entry) =>
            entry.id === profile.id ? { ...entry, owner_user_id: submitted } : entry,
          ),
        };
      }),
    );
  };
  useSettingsSaveContributor({
    id: `executor-ownership:${profile?.id ?? executor.id}`,
    revision: value,
    isDirty: value !== saved,
    save,
    discard: () => setValue(saved),
  });
  return (
    <ExecutorOwnerChoice
      value={value}
      onChange={setValue}
      baseline={saved}
      parentOwner={profile ? (executor.owner_user_id ?? "") : ""}
      label={t(profile ? "executors:ownershipProfile" : "executors:ownershipExecutor")}
    />
  );
}
