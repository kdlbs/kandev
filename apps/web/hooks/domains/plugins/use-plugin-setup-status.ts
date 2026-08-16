"use client";

import { useEffect, useMemo, useState } from "react";
import { getPluginConfig } from "@/lib/api/domains/plugins-api";
import {
  buildInitialValues,
  missingRequiredFields,
  parseConfigSchema,
  type PluginConfigField,
} from "@/lib/plugins/config-schema";
import type { PluginRecord } from "@/lib/types/plugins";

type SetupProbe = {
  id: string;
  version: string;
  fields: PluginConfigField[];
};

/**
 * The installed plugins worth probing: an unconfigured plugin can only exist
 * where the manifest declares a required field the operator has to fill in.
 * Boolean fields are excluded for the same reason the settings form excludes
 * them from its required check — `false` is a legitimate value, so they are
 * never "missing".
 */
function buildProbes(plugins: PluginRecord[]): SetupProbe[] {
  const probes: SetupProbe[] = [];
  for (const plugin of plugins) {
    if (plugin.status === "uninstalled") continue;
    const fields = parseConfigSchema(plugin.config_schema).filter(
      (field) => field.required && field.type !== "boolean",
    );
    if (fields.length === 0) continue;
    probes.push({ id: plugin.id, version: plugin.version, fields });
  }
  return probes;
}

/**
 * Ids of installed plugins whose required settings are still unset, so the
 * list can mark them "Setup required" and point at the per-plugin settings
 * page. One small GET per candidate plugin, mirroring the per-row probes on
 * the workspaces list: there is no aggregate endpoint, installs are few, and
 * only plugins that declare a required field are probed at all.
 *
 * A failed read yields no badge. The row must never claim a plugin is
 * unconfigured on the strength of a request that did not come back.
 */
export function usePluginSetupStatus(plugins: PluginRecord[]): Set<string> {
  const probes = useMemo(() => buildProbes(plugins), [plugins]);
  // Fields are derived from the manifest, which cannot change without the
  // version changing, so id@version is a complete signature for the probe set.
  const signature = probes.map((probe) => `${probe.id}@${probe.version}`).join(",");
  const [needsSetup, setNeedsSetup] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    if (probes.length === 0) {
      setNeedsSetup((prev) => (prev.size === 0 ? prev : new Set()));
      return;
    }
    let cancelled = false;
    void Promise.all(
      probes.map(async (probe) => {
        try {
          const config = await getPluginConfig(probe.id, { cache: "no-store" });
          const values = buildInitialValues(probe.fields, config);
          return missingRequiredFields(probe.fields, values).length > 0 ? probe.id : null;
        } catch {
          return null;
        }
      }),
    ).then((results) => {
      if (cancelled) return;
      setNeedsSetup(new Set(results.filter((id): id is string => id !== null)));
    });
    return () => {
      cancelled = true;
    };
    // `signature` is the real reload trigger; `probes` is recomputed on every
    // store update and would re-probe on unrelated plugin changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [signature]);

  return needsSetup;
}
