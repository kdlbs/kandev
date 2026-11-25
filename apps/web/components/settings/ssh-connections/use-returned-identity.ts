"use client";
import { useEffect, useRef, useState } from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { sshConnectionSettingsPath } from "@/lib/settings/ssh-connection-settings-routes";
import type { SSHIdentity } from "@/lib/types/http-ssh-connections";

/** Consume a public identity candidate once, leaving an ordinary dirty draft. */
export function useReturnedIdentity(identities: SSHIdentity[], update: (id: string) => void) {
  const search = useSearchParams();
  const router = useRouter();
  const candidate = search.get("select_identity");
  const consumed = useRef<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  useEffect(() => {
    if (!candidate || consumed.current === candidate) return;
    router.replace(sshConnectionSettingsPath(search, { select_identity: null }), {
      scroll: false,
      onNavigated: () => {
        consumed.current = candidate;
        const exists = identities.some((identity) => identity.id === candidate);
        setUnavailable(!exists);
        if (exists) update(candidate);
        document.getElementById("ssh-managed-identity")?.focus();
      },
    });
  }, [candidate, identities, update, search, router]);
  return unavailable;
}
