import type { AdditionalNetworkRow } from "@/components/settings/profile-edit/use-docker-networks-form-state";

/** The network form values a new profile is created from. */
export type DockerNetworkFormValues = {
  isDocker: boolean;
  primaryNetwork: string;
  primaryGwPriority: string;
  additionalNetworks: AdditionalNetworkRow[];
};

// applyDockerNetworks writes the network keys a new profile configures. An
// untouched card writes none, so a profile created without it is
// indistinguishable from one created before the card existed.
export function applyDockerNetworks(
  config: Record<string, string>,
  input: DockerNetworkFormValues,
) {
  if (!input.isDocker) return;
  const primary = input.primaryNetwork.trim();
  if (primary) {
    config.docker_network = primary;
    const priority = input.primaryGwPriority.trim();
    if (priority) config.docker_network_gw_priority = priority;
  }
  const additional = input.additionalNetworks
    .map((row) => ({ name: row.name.trim(), gwPriority: row.gwPriority.trim() }))
    .filter((row) => row.name !== "")
    .map((row) =>
      row.gwPriority === ""
        ? { name: row.name }
        : { name: row.name, gw_priority: Number(row.gwPriority) },
    );
  if (additional.length > 0) {
    config.docker_additional_networks = JSON.stringify(additional);
  }
}
