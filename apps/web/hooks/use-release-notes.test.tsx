import type { ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useReleaseNotes } from "./use-release-notes";

const { latest, previous, persist } = vi.hoisted(() => ({
  latest: { version: "2.0.0", date: "2026-09-29", notes: "Improved navigation" },
  previous: { version: "1.0.0", date: "2026-09-01", notes: "Earlier improvements" },
  persist: vi.fn().mockResolvedValue({}),
}));

vi.mock("@/lib/release-notes", () => ({
  getReleaseNotes: () => latest,
  hasReleaseNotes: () => true,
}));
vi.mock("@/lib/changelog", () => ({ getChangelog: () => [latest, previous] }));
vi.mock("@/lib/api", () => ({ updateUserSettings: persist }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => null }));

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderNotes() {
  return renderHook(() => ({ notes: useReleaseNotes(), store: useAppStoreApi() }), {
    wrapper: ({ children }: { children: ReactNode }) => <StateProvider>{children}</StateProvider>,
  });
}

it("keeps the current release readable after opening and marking the notes seen", () => {
  const { result } = renderNotes();
  act(() => result.current.notes.openDialog());
  expect(result.current.notes.unseenEntries).toEqual([latest]);
  expect(result.current.notes.hasUnseen).toBe(false);
  expect(persist).toHaveBeenCalledWith(
    { release_notes_last_seen_version: latest.version },
    { cache: "no-store" },
  );
  act(() => result.current.notes.closeDialog());
  act(() => result.current.notes.openDialog());
  expect(result.current.notes.unseenEntries).toEqual([latest]);
});

it("opens every unseen release even when notification badges are disabled", () => {
  const { result } = renderNotes();
  act(() => {
    const state = result.current.store.getState();
    state.setUserSettings({
      ...state.userSettings,
      loaded: true,
      showReleaseNotification: false,
      releaseNotesLastSeenVersion: "0.0.0",
    });
  });
  expect(result.current.notes.showTopbarButton).toBe(false);
  expect(result.current.notes.hasNotes).toBe(true);
  act(() => result.current.notes.openDialog());
  expect(result.current.notes.unseenEntries).toEqual([latest, previous]);
});
