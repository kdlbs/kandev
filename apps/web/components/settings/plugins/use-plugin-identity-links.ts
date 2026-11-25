"use client";

import { useCallback, useEffect, useState } from "react";
import {
  createPluginIdentityLinkCode,
  listPluginIdentityLinks,
  revokePluginIdentityLink,
} from "@/lib/api/domains/plugins-api";
import type { PluginIdentityLink, PluginIdentityLinkCode } from "@/lib/types/plugins";

/**
 * Linked external accounts for one plugin. Members see and revoke their own
 * links; admins see and revoke every link of the installation.
 */
export function usePluginIdentityLinks(pluginId: string, isAdmin: boolean) {
  const [links, setLinks] = useState<PluginIdentityLink[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [code, setCode] = useState<PluginIdentityLinkCode | null>(null);
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const scope = isAdmin ? "all" : "own";

  const fetchLinks = useCallback(async () => {
    try {
      setLinks(await listPluginIdentityLinks(pluginId, scope));
      setErrorKey(null);
    } catch {
      setErrorKey("plugins:identityLinksLoadFailed");
    }
  }, [pluginId, scope]);

  const reload = useCallback(async () => {
    setLoading(true);
    try {
      await fetchLinks();
    } finally {
      setLoading(false);
    }
  }, [fetchLinks]);

  useEffect(() => {
    void reload();
  }, [reload]);

  // A code is redeemed in the external chat app, so the account links change
  // while this page is in the background. Refresh quietly when it regains focus.
  useEffect(() => {
    const onFocus = () => void fetchLinks();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [fetchLinks]);

  const createCode = useCallback(async () => {
    setBusy(true);
    try {
      setCode(await createPluginIdentityLinkCode(pluginId));
      setErrorKey(null);
    } catch {
      setErrorKey("plugins:identityLinkCodeFailed");
    } finally {
      setBusy(false);
    }
  }, [pluginId]);

  const revoke = useCallback(
    async (linkId: string) => {
      setBusy(true);
      try {
        await revokePluginIdentityLink(pluginId, linkId);
        setLinks((current) => current.filter((link) => link.id !== linkId));
        setErrorKey(null);
      } catch {
        setErrorKey("plugins:identityLinkRevokeFailed");
      } finally {
        setBusy(false);
      }
    },
    [pluginId],
  );

  return { links, loading, busy, code, errorKey, reload, createCode, revoke };
}
