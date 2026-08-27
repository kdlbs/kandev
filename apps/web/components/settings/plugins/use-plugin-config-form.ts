"use client";

import { useEffect, useMemo, useState } from "react";
import { toast } from "@/lib/toast/sonner";
// Module-level `t`: these resolve when the save/load callback runs.
import { t } from "@/lib/i18n";
import { getPluginConfig, updatePluginConfig } from "@/lib/api/domains/plugins-api";
import {
  SECRET_MASK,
  buildInitialValues,
  missingRequiredFields,
  parseConfigSchema,
  serializeConfigValues,
  type PluginConfigField,
} from "@/lib/plugins/config-schema";
import type { PluginRecord } from "@/lib/types/plugins";

type SaveStatus = "idle" | "loading" | "success" | "error";
type FormValues = Record<string, string | boolean>;

function maskSecretsIn(
  source: FormValues,
  fields: ReturnType<typeof parseConfigSchema>,
): FormValues {
  const masked = { ...source };
  for (const field of fields) {
    const current = masked[field.name];
    if (field.secret && typeof current === "string" && current !== "") {
      masked[field.name] = SECRET_MASK;
    }
  }
  return masked;
}

/**
 * Whether the form is showing a required value that is not stored yet: the key
 * is absent from the config, but buildInitialValues put something persistable
 * in the field (a schema default, or false for a boolean).
 *
 * Those cases are otherwise undirtiable. The displayed value equals its own
 * baseline, so Save never enables and the operator cannot persist the value
 * already in front of them, leaving the list's "Setup required" badge with no
 * way to clear. A required field that is simply blank is excluded: there is
 * nothing to save yet, and the missing-field validation already covers it.
 */
function hasUnsavedRequiredDefaults(
  fields: PluginConfigField[],
  config: Record<string, unknown>,
  displayed: FormValues,
): boolean {
  return fields.some((field) => {
    if (!field.required) return false;
    if (Object.prototype.hasOwnProperty.call(config, field.name)) return false;
    const value = displayed[field.name];
    return typeof value === "boolean" || (typeof value === "string" && value.trim() !== "");
  });
}

/**
 * Load/edit/save state for one plugin's schema-driven settings form.
 * Mirrors use-plugin-actions' local-hook pattern: fetch + toast wiring lives
 * here, the components stay presentational. Saving PATCHes the full config
 * (secret fields carrying the mask keep their stored value server-side) and
 * then re-fetches the masked config so the form reflects what is stored.
 */
export function usePluginConfigForm(plugin: PluginRecord | null) {
  const fields = useMemo(() => parseConfigSchema(plugin?.config_schema), [plugin?.config_schema]);
  const [values, setValues] = useState<FormValues>({});
  const [initialValues, setInitialValues] = useState<FormValues>({});
  // True when a required key is absent from the stored config while the form
  // shows a value for it (a schema default, or false for a boolean). Without
  // this the baseline equals what is displayed, isDirty stays false, and the
  // operator cannot save the value the form is already showing them: the
  // "Setup required" badge on the plugin list would have no way to clear.
  const [requiredKeysUnsaved, setRequiredKeysUnsaved] = useState(false);
  const [configLoading, setConfigLoading] = useState(false);
  const [configError, setConfigError] = useState<string | null>(null);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");

  const pluginId = plugin?.id ?? null;
  const hasFields = fields.length > 0;

  useEffect(() => {
    if (!pluginId || !hasFields) return;
    let cancelled = false;
    setConfigLoading(true);
    setConfigError(null);
    getPluginConfig(pluginId, { cache: "no-store" })
      .then((config) => {
        if (cancelled) return;
        const initial = buildInitialValues(fields, config);
        setValues(initial);
        setInitialValues(initial);
        setRequiredKeysUnsaved(hasUnsavedRequiredDefaults(fields, config, initial));
      })
      .catch((err) => {
        if (!cancelled) {
          setConfigError(err instanceof Error ? err.message : t("plugins:failedToLoadSettings"));
        }
      })
      .finally(() => {
        if (!cancelled) setConfigLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // fields is derived solely from plugin.config_schema; pluginId is the
    // real reload trigger.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pluginId, hasFields]);

  const isDirty = useMemo(
    () =>
      requiredKeysUnsaved ||
      fields.some((field) => values[field.name] !== initialValues[field.name]),
    [fields, values, initialValues, requiredKeysUnsaved],
  );
  const missing = useMemo(() => missingRequiredFields(fields, values), [fields, values]);

  const handleChange = (name: string, value: string | boolean) => {
    setValues((prev) => ({ ...prev, [name]: value }));
    setSaveStatus("idle");
  };

  const handleSave = async () => {
    if (!pluginId) return;
    if (missing.length > 0) {
      // The field names come from the plugin's config_schema, so only the
      // "Required:" frame is copy.
      const message = t("plugins:requiredFields", { fields: missing.join(", ") });
      toast.error(message);
      throw new Error(message);
    }
    setSaveStatus("loading");
    try {
      await updatePluginConfig(pluginId, serializeConfigValues(fields, values));
    } catch (err) {
      setSaveStatus("error");
      toast.error(err instanceof Error ? err.message : t("plugins:failedToSaveSettings"));
      throw err;
    }
    // The config IS persisted from here on — a refetch failure (e.g. a
    // transient hiccup while the plugin restarts) must not be reported as a
    // save failure, and the typed cleartext secret must not stay on screen.
    try {
      const refreshed = await getPluginConfig(pluginId, { cache: "no-store" });
      const initial = buildInitialValues(fields, refreshed);
      setValues(initial);
      setInitialValues(initial);
      setRequiredKeysUnsaved(hasUnsavedRequiredDefaults(fields, refreshed, initial));
      toast.success(t("plugins:settingsSaved"));
    } catch {
      const masked = maskSecretsIn(values, fields);
      setValues(masked);
      setInitialValues(masked);
      // The PATCH went through, so the required keys are stored even though
      // the re-read did not come back.
      setRequiredKeysUnsaved(false);
      toast.warning(t("plugins:settingsSavedReloadFailed"));
    }
    setSaveStatus("success");
  };

  return {
    fields,
    values,
    initialValues,
    configLoading,
    configError,
    saveStatus,
    isDirty,
    canSave: missing.length === 0,
    invalidReason:
      missing.length > 0 ? t("plugins:requiredFields", { fields: missing.join(", ") }) : undefined,
    revision: JSON.stringify(values),
    handleChange,
    handleSave,
    discard: () => {
      setValues(initialValues);
      setSaveStatus("idle");
    },
  };
}
