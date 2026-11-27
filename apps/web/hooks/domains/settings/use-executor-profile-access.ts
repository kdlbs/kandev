"use client";

import { useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import type { Executor, ExecutorProfile } from "@/lib/types/http";

type ProfileAccess = { executor: Executor; profile: ExecutorProfile };

export function useExecutorProfileAccess(profileId: string) {
  const executor = useAppStore(
    (state) =>
      state.executors.items.find((item) =>
        item.profiles?.some((profile) => profile.id === profileId),
      ) ?? null,
  );
  const profile = executor?.profiles?.find((item) => item.id === profileId) ?? null;
  const loaded = useAppStore((state) => state.settingsData.executorsLoaded);
  const identity = useAppStore((state) => {
    const { user, mode } = state.auth;
    return `${mode}:${user?.id}:${user?.role}:${user?.status}`;
  });
  const [retained, setRetained] = useState<{
    identity: string;
    profileId: string;
    result: ProfileAccess;
  } | null>(null);
  useEffect(() => {
    if (executor && profile) setRetained({ identity, profileId, result: { executor, profile } });
    else
      setRetained((previous) =>
        loaded || previous?.identity !== identity || previous.profileId !== profileId
          ? null
          : previous,
      );
  }, [executor, profile, identity, profileId, loaded]);

  const current = executor && profile ? { executor, profile } : null;
  const pending =
    !loaded && retained?.identity === identity && retained.profileId === profileId
      ? retained.result
      : null;
  // Keep the editor mounted, but hidden and inert, while its catalogue entry is
  // revalidated. A confirmed removal or identity change discards the editor.
  return { result: current ?? pending, checking: !current && Boolean(pending) };
}
