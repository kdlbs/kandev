"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import {
  fetchSessionCapacitySettings,
  updateSessionCapacitySettings,
} from "@/lib/api/domains/settings-api";
import type {
  SessionCapacitySettingsResponse,
  SessionCapacitySettingsSource,
  SessionCapacityPolicy,
  SessionCapacitySettingsPatch,
} from "@/lib/types/system";
import { useSettingsSaveContributor } from "../settings-save-provider";

import {
  capacityGradePatch,
  gradeLimitDrafts,
  gradeDraftSignature,
  type GradeLimitDraft,
} from "./session-capacity-grade-draft";

const MAX_SESSIONS_LIMIT = 2147483647;

export function parseSessionCapacityMaximum(value: string): number | null {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const parsed = Number(trimmed);
  if (!Number.isSafeInteger(parsed) || parsed < 1 || parsed > MAX_SESSIONS_LIMIT) return null;
  return parsed;
}

export function sessionCapacitySourceLabelKey(source: SessionCapacitySettingsSource): string {
  switch (source) {
    case "setting":
      return "system:sessionCapacitySourceSetting";
    case "environment":
      return "system:sessionCapacitySourceEnvironment";
    default:
      return "system:sessionCapacitySourceDefault";
  }
}

type LoadState = {
  snapshot: SessionCapacitySettingsResponse | null;
  setSnapshot: (value: SessionCapacitySettingsResponse) => void;
  enabledDraft: boolean;
  setEnabledDraft: Dispatch<SetStateAction<boolean>>;
  maxDraft: string;
  setMaxDraft: Dispatch<SetStateAction<string>>;
  policyDraft: SessionCapacityPolicy;
  setPolicyDraft: Dispatch<SetStateAction<SessionCapacityPolicy>>;
  gradesDraft: GradeLimitDraft[];
  setGradesDraft: Dispatch<SetStateAction<GradeLimitDraft[]>>;
  loading: boolean;
  loadFailed: boolean;
  reload: () => Promise<void>;
};

function useSessionCapacityLoad(): LoadState {
  const [snapshot, setSnapshot] = useState<SessionCapacitySettingsResponse | null>(null);
  const [enabledDraft, setEnabledDraft] = useState(false);
  const [maxDraft, setMaxDraft] = useState("");
  const [policyDraft, setPolicyDraft] = useState<SessionCapacityPolicy>("automatic");
  const [gradesDraft, setGradesDraft] = useState<GradeLimitDraft[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const loadVersion = useRef(0);

  const reload = useCallback(async () => {
    const version = ++loadVersion.current;
    setLoading(true);
    setLoadFailed(false);
    try {
      const response = await fetchSessionCapacitySettings();
      if (version !== loadVersion.current) return;
      setSnapshot(response);
      setEnabledDraft(response.settings.enabled);
      setMaxDraft(String(response.settings.max_sessions));
      setPolicyDraft(response.settings.policy ?? "automatic");
      setGradesDraft(gradeLimitDrafts(response.settings.grade_limits));
    } catch {
      if (version === loadVersion.current) setLoadFailed(true);
    } finally {
      if (version === loadVersion.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    return () => {
      loadVersion.current += 1;
    };
  }, [reload]);

  return {
    snapshot,
    setSnapshot,
    enabledDraft,
    setEnabledDraft,
    maxDraft,
    setMaxDraft,
    policyDraft,
    setPolicyDraft,
    gradesDraft,
    setGradesDraft,
    loading,
    loadFailed,
    reload,
  };
}

type ContributorOptions = {
  load: LoadState;
  parsed: number | null;
  savedEnabled: boolean | undefined;
  savedMaximum: number | undefined;
  isAdmin: boolean;
  isLocked: boolean;
  invalidReason: string | undefined;
  onSaveFailed: (failed: boolean) => void;
  scopePatch: SessionCapacitySettingsPatch | null;
};

function sessionCapacityInvalidReason(
  t: (key: string, values?: Record<string, unknown>) => string,
  isAdmin: boolean,
  isLocked: boolean,
  enabled: boolean,
  parsed: number | null,
): string | undefined {
  if (!isAdmin) return t("system:sessionCapacityAdminOnly");
  if (isLocked) {
    return t("system:sessionCapacityEnvironmentLocked", {
      variable: "KANDEV_MAX_CONCURRENT_SESSIONS",
    });
  }
  if (enabled && parsed === null) return t("system:sessionCapacityValidation");
  return undefined;
}

function useSessionCapacityContributor({
  load,
  parsed,
  savedEnabled,
  savedMaximum,
  isAdmin,
  isLocked,
  invalidReason,
  onSaveFailed,
  scopePatch,
}: ContributorOptions) {
  const {
    snapshot,
    setSnapshot,
    enabledDraft,
    setEnabledDraft,
    maxDraft,
    setMaxDraft,
    policyDraft,
    setPolicyDraft,
    gradesDraft,
    setGradesDraft,
  } = load;
  const isDirty =
    savedEnabled !== undefined &&
    savedMaximum !== undefined &&
    (enabledDraft !== savedEnabled ||
      maxDraft !== String(savedMaximum) ||
      policyDraft !== (snapshot?.settings.policy ?? "automatic") ||
      gradeDraftSignature(gradesDraft) !==
        gradeDraftSignature(gradeLimitDrafts(snapshot?.settings.grade_limits)));
  const canSave =
    snapshot !== null &&
    isAdmin &&
    !isLocked &&
    (!enabledDraft || parsed !== null) &&
    savedMaximum !== undefined &&
    scopePatch !== null;

  useSettingsSaveContributor({
    id: "system-session-capacity",
    revision: `${enabledDraft}:${maxDraft}:${policyDraft}:${gradeDraftSignature(gradesDraft)}`,
    isDirty,
    canSave,
    invalidReason,
    save: async () => {
      if (!canSave || savedMaximum === undefined || scopePatch === null)
        throw new Error(invalidReason);
      const submittedPolicy = policyDraft;
      const submittedGrades = gradeDraftSignature(gradesDraft);
      const submittedEnabled = enabledDraft;
      const submittedMaximumDraft = maxDraft;
      const submittedMaximum = parsed ?? savedMaximum;
      onSaveFailed(false);
      try {
        const response = await updateSessionCapacitySettings({
          enabled: submittedEnabled,
          max_sessions: submittedMaximum,
          ...scopePatch,
        });
        setSnapshot(response);
        setPolicyDraft((current) =>
          current === submittedPolicy ? (response.settings.policy ?? "automatic") : current,
        );
        setGradesDraft((current) =>
          gradeDraftSignature(current) === submittedGrades
            ? gradeLimitDrafts(response.settings.grade_limits)
            : current,
        );
        setEnabledDraft((current) =>
          current === submittedEnabled ? response.settings.enabled : current,
        );
        setMaxDraft((current) =>
          current === submittedMaximumDraft ? String(response.settings.max_sessions) : current,
        );
      } catch (error) {
        onSaveFailed(true);
        throw error;
      }
    },
    discard: () => {
      if (savedEnabled !== undefined) setEnabledDraft(savedEnabled);
      if (savedMaximum !== undefined) setMaxDraft(String(savedMaximum));
      setPolicyDraft(snapshot?.settings.policy ?? "automatic");
      setGradesDraft(gradeLimitDrafts(snapshot?.settings.grade_limits));
      onSaveFailed(false);
    },
  });

  return { isDirty, canSave };
}

export function useSessionCapacitySettings() {
  const { t } = useTranslation();
  const role = useAppStore((state) => state.auth.user?.role);
  const [saveFailed, setSaveFailed] = useState(false);
  const load = useSessionCapacityLoad();
  const savedEnabled = load.snapshot?.settings.enabled;
  const savedMaximum = load.snapshot?.settings.max_sessions;
  const parsed = parseSessionCapacityMaximum(load.maxDraft);
  const isAdmin = role === undefined || role === "admin";
  const isLocked = load.snapshot?.effective.locked === true;
  const scopePatch = load.snapshot
    ? capacityGradePatch(
        load.snapshot.settings,
        load.enabledDraft,
        load.policyDraft,
        load.gradesDraft,
      )
    : null;
  const invalidReason =
    sessionCapacityInvalidReason(t, isAdmin, isLocked, load.enabledDraft, parsed) ??
    (load.enabledDraft && scopePatch === null
      ? t("system:sessionCapacityGradeValidation")
      : undefined);
  const contributor = useSessionCapacityContributor({
    load,
    parsed,
    savedEnabled,
    savedMaximum,
    isAdmin,
    isLocked,
    invalidReason,
    onSaveFailed: setSaveFailed,
    scopePatch,
  });

  return {
    ...load,
    ...contributor,
    parsed,
    savedEnabled,
    savedMaximum,
    isAdmin,
    isLocked,
    invalidReason,
    saveFailed,
    scopeInvalid: load.enabledDraft && scopePatch === null,
    gradeSupported:
      load.snapshot?.settings.policy !== undefined &&
      load.snapshot?.settings.grade_limits !== undefined,
  };
}
