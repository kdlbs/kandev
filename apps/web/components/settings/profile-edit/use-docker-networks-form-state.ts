import { useCallback, useMemo, useState } from "react";

/** One additional network row. Both fields are form text, trimmed on save. */
export type AdditionalNetworkRow = {
  name: string;
  gwPriority: string;
};

type StoredAdditionalNetwork = {
  name?: unknown;
  gw_priority?: unknown;
};

/**
 * Reads the saved additional-network list.
 *
 * A value that is not a readable list yields an empty list rather than
 * throwing: the operator still needs the rest of the profile editor, and the
 * backend refuses the same value at launch with a message that names it.
 */
export function parseAdditionalNetworks(raw: string | undefined): AdditionalNetworkRow[] {
  if (!raw) return [];
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return [];
  }
  if (!Array.isArray(parsed)) return [];
  return parsed.flatMap((entry) => {
    if (typeof entry !== "object" || entry === null) return [];
    const { name, gw_priority: gwPriority } = entry as StoredAdditionalNetwork;
    if (typeof name !== "string") return [];
    return [
      {
        name,
        gwPriority: typeof gwPriority === "number" ? String(gwPriority) : "",
      },
    ];
  });
}

export function useDockerNetworksFormState(config?: Record<string, string>) {
  const baselinePrimary = config?.docker_network ?? "";
  const baselinePriority = config?.docker_network_gw_priority ?? "";
  const baselineAdditional = useMemo(
    () => parseAdditionalNetworks(config?.docker_additional_networks),
    [config?.docker_additional_networks],
  );

  const [primaryNetwork, setPrimaryNetwork] = useState(baselinePrimary);
  const [primaryGwPriority, setPrimaryGwPriority] = useState(baselinePriority);
  const [additionalNetworks, setAdditionalNetworks] =
    useState<AdditionalNetworkRow[]>(baselineAdditional);

  const addAdditionalNetwork = useCallback(() => {
    setAdditionalNetworks((rows) => [...rows, { name: "", gwPriority: "" }]);
  }, []);

  const updateAdditionalNetwork = useCallback(
    (index: number, patch: Partial<AdditionalNetworkRow>) => {
      setAdditionalNetworks((rows) =>
        rows.map((row, i) => (i === index ? { ...row, ...patch } : row)),
      );
    },
    [],
  );

  const removeAdditionalNetwork = useCallback((index: number) => {
    setAdditionalNetworks((rows) => rows.filter((_, i) => i !== index));
  }, []);

  const resetDockerNetworks = useCallback(() => {
    setPrimaryNetwork(baselinePrimary);
    setPrimaryGwPriority(baselinePriority);
    setAdditionalNetworks(baselineAdditional);
  }, [baselinePrimary, baselinePriority, baselineAdditional]);

  return {
    primaryNetwork,
    setPrimaryNetwork,
    primaryGwPriority,
    setPrimaryGwPriority,
    additionalNetworks,
    addAdditionalNetwork,
    updateAdditionalNetwork,
    removeAdditionalNetwork,
    resetDockerNetworks,
    baselinePrimaryNetwork: baselinePrimary,
    baselinePrimaryGwPriority: baselinePriority,
    baselineAdditionalNetworks: baselineAdditional,
  };
}
