"use client";

import { useEffect, useMemo, useState } from "react";
import { getPluginConfig } from "@/lib/api/domains/plugins-api";
import { parseConfigSchema } from "@/lib/plugins/config-schema";
import type { PluginRecord } from "@/lib/types/plugins";

type SetupProbe = {
  id: string;
  version: string;
  /** Names from the manifest's `required` list, in schema order. */
  required: string[];
};

/**
 * Whether a stored config value satisfies a required field.
 *
 * The host's checkRequiredKeys (internal/plugins/config.go) accepts any
 * present key, whatever its value. This is deliberately narrower on one point:
 * a key present but empty (`null`, `""`, or blank) counts as unset here. The
 * badge answers "is there something the operator still has to fill in", not
 * "would the host reject this config", and a plugin handed an empty required
 * token is broken in a way worth pointing at. The host draws the same
 * distinction in isZeroConfigValue, which treats nil and "" as unset.
 *
 * Everything else follows the host: a stored `false` is configured, and a
 * required boolean that was never saved is not.
 *
 * The blank cases cannot come from the settings form, which omits empty inputs
 * rather than storing them, but a hand-edited config file can carry one.
 */
function isConfigured(value: unknown): boolean {
  if (value === undefined || value === null) return false;
  return typeof value !== "string" || value.trim() !== "";
}

/**
 * The installed plugins worth probing: only a manifest that declares a
 * required field can leave the operator with something to fill in.
 */
function buildProbes(plugins: PluginRecord[]): SetupProbe[] {
  const probes: SetupProbe[] = [];
  for (const plugin of plugins) {
    if (plugin.status === "uninstalled") continue;
    const required = parseConfigSchema(plugin.config_schema)
      .filter((field) => field.required)
      .map((field) => field.name);
    if (required.length === 0) continue;
    probes.push({ id: plugin.id, version: plugin.version, required });
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
  // The required list comes from the manifest, which cannot change without
  // the version changing, so id@version is a complete signature.
  const signature = probes.map((probe) => `${probe.id}@${probe.version}`).join(",");
  const [needsSetup, setNeedsSetup] = useState<Set<string>>(() => new Set());

  useEffect(() => {
    if (probes.length === 0) {
      // No request is in flight, so this branch needs no cleanup, unlike the
      // `cancelled` path below.
      setNeedsSetup((prev) => (prev.size === 0 ? prev : new Set()));
      return;
    }
    let cancelled = false;
    void Promise.all(
      probes.map(async (probe) => {
        try {
          const config = await getPluginConfig(probe.id, { cache: "no-store" });
          const unset = probe.required.some((name) => !isConfigured(config[name]));
          return unset ? probe.id : null;
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
