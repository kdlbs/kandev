import type { StorageDockerNetworkSummary } from "@/lib/types/system";
import type { StorageResource, Translate } from "./storage-overview-resources";

const STORAGE_UNAVAILABLE_VALUE_KEY = "system:storageUnavailableValue";

export function dockerNetworksResource(
  t: Translate,
  summary: StorageDockerNetworkSummary | null | undefined,
): StorageResource {
  if (!summary || summary.available === false) {
    return {
      id: "docker-networks",
      label: t("system:storageDockerNetworks"),
      value: t(STORAGE_UNAVAILABLE_VALUE_KEY),
      detail: t("system:storageDockerNetworksUnmeasured"),
      warning: summary?.warning,
      source: "docker_networks",
    };
  }
  const candidates = summary.candidates?.length ?? 0;
  return {
    id: "docker-networks",
    label: t("system:storageDockerNetworks"),
    value: String(candidates),
    detail: t("system:storageDockerNetworksDetail", { count: candidates }),
    warning: summary.warnings?.join(" · ") || undefined,
    source: "docker_networks",
  };
}
