const FALLBACK_REASON_KEY = "settings:updateChannelUnavailable";

const REASON_KEYS: Readonly<Record<string, string>> = {
  "Nightly updates require a Kandev-managed npm or npx user service.":
    "settings:updateChannelRequiresManagedService",
  "Nightly updates require valid Kandev service metadata.":
    "settings:updateChannelRequiresServiceMetadata",
  "Nightly updates are only available for user services.":
    "settings:updateChannelRequiresUserService",
  "This service manager does not support nightly updates.":
    "settings:updateChannelUnsupportedServiceManager",
  "Nightly updates are only available for npm or npx installations.":
    "settings:updateChannelRequiresNpmInstallation",
  "Update channel settings are unavailable.": "settings:updateChannelSettingsUnavailable",
};

export function updateChannelUnsupportedReasonKey(reason: string): string {
  return REASON_KEYS[reason] ?? FALLBACK_REASON_KEY;
}
