"use client";

import { useState, useCallback, useMemo, useEffect } from "react";
import { getReleaseNotes, hasReleaseNotes } from "@/lib/release-notes";
import { getChangelog, type ChangelogEntry } from "@/lib/changelog";
import { updateUserSettings } from "@/lib/api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";

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
  const showReleaseNotification = useAppStore((s) => s.userSettings.showReleaseNotification);
  const lastSeenVersion = useAppStore((s) => s.userSettings.releaseNotesLastSeenVersion);
  const settingsLoaded = useAppStore((s) => s.userSettings.loaded);
  const storeApi = useAppStoreApi();

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
      const { userSettings, setUserSettings } = storeApi.getState();
      setUserSettings({ ...userSettings, releaseNotesLastSeenVersion: parsed });
      persistLastSeenVersion(parsed);
      localStorage.removeItem(LEGACY_STORAGE_KEY);
    } catch {
      // Ignore migration errors
    }
  }, [settingsLoaded, lastSeenVersion, storeApi]);

  const markAsSeen = useCallback(() => {
    const version = latestRelease.version;
    const { userSettings, setUserSettings } = storeApi.getState();
    setUserSettings({ ...userSettings, releaseNotesLastSeenVersion: version });
    persistLastSeenVersion(version);
  }, [latestRelease.version, storeApi]);

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
