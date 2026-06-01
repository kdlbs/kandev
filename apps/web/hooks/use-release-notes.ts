"use client";

import { useState, useCallback, useMemo, useEffect } from "react";
import { getReleaseNotes, hasReleaseNotes } from "@/lib/release-notes";
import { getChangelog, type ChangelogEntry } from "@/lib/changelog";
import { updateUserSettings } from "@/lib/api";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  useUserSettings,
  useUserSettingsController,
} from "@/hooks/domains/settings/use-user-settings";
import { DEFAULT_USER_SETTINGS } from "@/lib/types/settings";

const LEGACY_STORAGE_KEY = "kandev.releaseNotes.lastSeenVersion";

function isVersionNewer(version: string, baseline: string): boolean {
  const a = version.split(".").map(Number);
  const b = baseline.split(".").map(Number);
  for (let i = 0; i < Math.max(a.length, b.length); i++) {
    if ((a[i] ?? 0) > (b[i] ?? 0)) return true;
    if ((a[i] ?? 0) < (b[i] ?? 0)) return false;
  }
  return false;
}

function getUnseenEntries(changelog: ChangelogEntry[], lastSeen: string | null): ChangelogEntry[] {
  if (!hasReleaseNotes()) return [];
  if (!lastSeen) return changelog.slice(0, 1);
  return changelog.filter((entry) => isVersionNewer(entry.version, lastSeen));
}

function persistLastSeenVersion(version: string) {
  const payload = { release_notes_last_seen_version: version };
  const client = getWebSocketClient();
  if (client) {
    client.request("user.settings.update", payload).catch(() => {
      updateUserSettings(payload, { cache: "no-store" }).catch(() => {});
    });
  } else {
    updateUserSettings(payload, { cache: "no-store" }).catch(() => {});
  }
}

export function useReleaseNotes() {
  const latestRelease = getReleaseNotes();
  const userSettings = useUserSettings().data ?? DEFAULT_USER_SETTINGS;
  const showReleaseNotification = userSettings.showReleaseNotification;
  const lastSeenVersion = userSettings.releaseNotesLastSeenVersion;
  const settingsLoaded = userSettings.loaded;
  const { setUserSettings, getLatest } = useUserSettingsController();

  const changelog = useMemo(() => getChangelog(), []);
  const unseenEntries = useMemo(
    () => getUnseenEntries(changelog, lastSeenVersion),
    [changelog, lastSeenVersion],
  );

  const hasUnseen = unseenEntries.length > 0;
  const [dialogOpen, setDialogOpen] = useState(false);
  // Snapshot entries when dialog opens so markAsSeen doesn't clear them mid-view
  const [dialogEntries, setDialogEntries] = useState<ChangelogEntry[]>([]);

  // One-time migration from localStorage to backend
  useEffect(() => {
    if (!settingsLoaded || lastSeenVersion) return;
    try {
      const raw = localStorage.getItem(LEGACY_STORAGE_KEY);
      if (!raw) return;
      const parsed = JSON.parse(raw) as string;
      if (!parsed) return;
      setUserSettings({ ...getLatest(), releaseNotesLastSeenVersion: parsed });
      persistLastSeenVersion(parsed);
      localStorage.removeItem(LEGACY_STORAGE_KEY);
    } catch {
      // Ignore migration errors
    }
  }, [settingsLoaded, lastSeenVersion, setUserSettings, getLatest]);

  const markAsSeen = useCallback(() => {
    // Mark against the newest version in the full changelog rather than the
    // current build's release-notes version. In dev/CI those can diverge —
    // git-cliff backfills the changelog from unreleased commits while
    // release-notes.json is pinned to `git describe --tags --abbrev=0`. If we
    // marked against the build version, getUnseenEntries would still see the
    // backfilled entries as newer and the topbar dot would never clear.
    const version = changelog[0]?.version ?? latestRelease.version;
    setUserSettings({ ...getLatest(), releaseNotesLastSeenVersion: version });
    persistLastSeenVersion(version);
  }, [changelog, latestRelease.version, setUserSettings, getLatest]);

  const openDialog = useCallback(() => {
    setDialogEntries(unseenEntries);
    setDialogOpen(true);
    markAsSeen();
  }, [markAsSeen, unseenEntries]);

  const closeDialog = useCallback(() => {
    setDialogOpen(false);
  }, []);

  return {
    unseenEntries: dialogOpen ? dialogEntries : unseenEntries,
    latestVersion: latestRelease.version,
    hasUnseen,
    dialogOpen,
    openDialog,
    closeDialog,
    hasNotes: hasReleaseNotes(),
    showTopbarButton: hasReleaseNotes() && hasUnseen && showReleaseNotification,
  };
}
