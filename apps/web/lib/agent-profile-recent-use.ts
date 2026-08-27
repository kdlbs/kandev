import type {
  AgentProfileRecentUseApiRecord,
  AgentProfileRecentUseContext,
} from "@/lib/types/http-agent-profile-recent-use";
import { recordAgentProfileRecentUse } from "@/lib/api/domains/agent-profile-recent-use-api";

export const MAX_AGENT_PROFILE_RECENT_USE_IDS = 10;

export type AgentProfileRecentUseRecord = {
  profileIds: string[];
  revision: number;
  updatedAt: string;
};

export type AgentProfileRecentUseRecords = Partial<
  Record<AgentProfileRecentUseContext, AgentProfileRecentUseRecord>
>;

export type AgentProfileRecentUseState = {
  records: AgentProfileRecentUseRecords;
  loaded: boolean;
};

/** Orders a source list by its bounded, already-authorized recent IDs. */
export function orderAgentProfilesByRecentUse<T extends { id: string }>(
  profiles: readonly T[],
  recentProfileIds: readonly string[] | undefined,
): T[] {
  const profilesById = new Map(profiles.map((profile) => [profile.id, profile]));
  const ordered: T[] = [];
  const seen = new Set<string>();
  for (const profileId of recentProfileIds?.slice(0, MAX_AGENT_PROFILE_RECENT_USE_IDS) ?? []) {
    const profile = profilesById.get(profileId);
    if (!profile || seen.has(profileId)) continue;
    ordered.push(profile);
    seen.add(profileId);
  }
  for (const profile of profiles) {
    if (seen.has(profile.id)) continue;
    ordered.push(profile);
    seen.add(profile.id);
  }
  return ordered;
}

/** Maps one API record into the camel-cased store shape. */
export function mapAgentProfileRecentUseApiRecord(
  record: AgentProfileRecentUseApiRecord,
): AgentProfileRecentUseRecord {
  return {
    profileIds: [...record.profile_ids],
    revision: record.revision,
    updatedAt: record.updated_at,
  };
}

/** Maps API records into the context-keyed store shape. */
export function mapAgentProfileRecentUseApiRecords(
  records: readonly AgentProfileRecentUseApiRecord[],
): AgentProfileRecentUseState {
  const byContext: AgentProfileRecentUseRecords = {};
  for (const record of records) {
    byContext[record.context] = mapAgentProfileRecentUseApiRecord(record);
  }
  return { records: byContext, loaded: true };
}

/** Starts a recency write without coupling its result to the launch lifecycle. */
export function recordAgentProfileRecentUseBestEffort(
  context: AgentProfileRecentUseContext,
  profileId: string,
  onSuccess: (record: AgentProfileRecentUseRecord) => void,
): void {
  if (!profileId) return;
  void recordAgentProfileRecentUse(context, profileId)
    .then((record) => onSuccess(mapAgentProfileRecentUseApiRecord(record)))
    .catch(() => undefined);
}

/** Merges boot or WS state without allowing an older context revision to win. */
export function mergeAgentProfileRecentUseState(
  current: AgentProfileRecentUseState,
  incoming: Partial<AgentProfileRecentUseState>,
): AgentProfileRecentUseState {
  const records: AgentProfileRecentUseRecords = { ...current.records };
  for (const [context, incomingRecord] of Object.entries(incoming.records ?? {})) {
    if (!incomingRecord) continue;
    const typedContext = context as AgentProfileRecentUseContext;
    const currentRecord = records[typedContext];
    if (!currentRecord || incomingRecord.revision >= currentRecord.revision) {
      records[typedContext] = {
        profileIds: [...incomingRecord.profileIds],
        revision: incomingRecord.revision,
        updatedAt: incomingRecord.updatedAt,
      };
    }
  }
  return { records, loaded: current.loaded || incoming.loaded === true };
}
