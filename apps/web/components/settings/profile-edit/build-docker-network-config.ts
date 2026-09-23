import type { AdditionalNetworkRow } from "@/components/settings/profile-edit/use-docker-networks-form-state";

/** The network form values a profile is saved from. */
export type DockerNetworkFormValues = {
  isDocker: boolean;
  primaryNetwork: string;
  primaryGwPriority: string;
  additionalNetworks: AdditionalNetworkRow[];
};

export type StoredAdditionalNetwork = { name: string; gw_priority?: number };

const WHOLE_NUMBER = /^-?\d+$/;

// serializeAdditionalNetworks turns the editor's rows into the stored list.
// A row without a name is dropped, and an empty priority is omitted so Docker
// keeps its own default for that attachment.
export function serializeAdditionalNetworks(
  rows: AdditionalNetworkRow[],
): StoredAdditionalNetwork[] {
  return rows
    .map((row) => ({ name: row.name.trim(), gwPriority: row.gwPriority.trim() }))
    .filter((row) => row.name !== "")
    .map((row) =>
      row.gwPriority === ""
        ? { name: row.name }
        : { name: row.name, gw_priority: Number(row.gwPriority) },
    );
}

// dockerNetworksInvalidReasonKey refuses a gateway priority the backend cannot
// read. The backend parses each priority as an integer at launch, so a value
// such as 1.5 would save cleanly and then fail every task. Only priorities
// that are actually saved are checked.
export function dockerNetworksInvalidReasonKey(input: DockerNetworkFormValues): string | null {
  if (!input.isDocker) return null;
  const priorities: string[] = [];
  if (input.primaryNetwork.trim()) priorities.push(input.primaryGwPriority.trim());
  for (const row of input.additionalNetworks) {
    if (row.name.trim()) priorities.push(row.gwPriority.trim());
  }
  const invalid = priorities.some((priority) => priority !== "" && !WHOLE_NUMBER.test(priority));
  return invalid ? "executors:gatewayPriorityMustBeAWholeNumber" : null;
}

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
  const additional = serializeAdditionalNetworks(input.additionalNetworks);
  if (additional.length > 0) {
    config.docker_additional_networks = JSON.stringify(additional);
  }
}
