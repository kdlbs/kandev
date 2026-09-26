export type NewSessionProfileSelectionSource =
  | "handoff"
  | "recent"
  | "current"
  | "source"
  | "manual";

export type NewSessionProfileSelection = {
  profileId: string;
  source: NewSessionProfileSelectionSource;
  profileExplicit: boolean;
};

export type NewSessionProfileSelectionInput = {
  compatibleProfiles: readonly { id: string }[];
  recentProfileIds?: readonly string[];
  currentProfileId?: string;
  handoffProfileId?: string;
};

/** Resolves the initial profile for an ordinary new-session dialog. */
export function resolveNewSessionProfileSelection({
  compatibleProfiles,
  recentProfileIds,
  currentProfileId,
  handoffProfileId,
}: NewSessionProfileSelectionInput): NewSessionProfileSelection {
  const compatibleProfileIds = new Set(compatibleProfiles.map((profile) => profile.id));

  if (handoffProfileId && compatibleProfileIds.has(handoffProfileId)) {
    return {
      profileId: handoffProfileId,
      source: "handoff",
      profileExplicit: true,
    };
  }

  const recentProfileId = recentProfileIds?.find((profileId) =>
    compatibleProfileIds.has(profileId),
  );
  if (recentProfileId) {
    return {
      profileId: recentProfileId,
      source: "recent",
      profileExplicit: true,
    };
  }

  if (currentProfileId && compatibleProfileIds.has(currentProfileId)) {
    return {
      profileId: currentProfileId,
      source: "current",
      profileExplicit: false,
    };
  }

  return {
    profileId: compatibleProfiles[0]?.id ?? "",
    source: "source",
    profileExplicit: false,
  };
}
