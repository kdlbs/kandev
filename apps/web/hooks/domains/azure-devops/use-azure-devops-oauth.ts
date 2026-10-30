"use client";

import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { useToast } from "@/components/toast-provider";
import {
  getAzureDevOpsOAuthAvailability,
  signOutAzureDevOpsOAuth,
} from "@/lib/api/domains/azure-devops-oauth-api";
import type { AzureDevOpsConfig } from "@/lib/types/azure-devops";

/** The installation's default Entra application (client) ID, or "" when none. */
export function useDefaultOAuthClientId(workspaceId: string): string {
  const [clientId, setClientId] = useState("");
  useEffect(() => {
    let active = true;
    getAzureDevOpsOAuthAvailability(workspaceId)
      .then((next) => active && setClientId(next.defaultClientId))
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [workspaceId]);
  return clientId;
}

/** Signs the workspace out of its Microsoft account and applies the result. */
export function useOAuthSignOut(
  workspaceId: string,
  applyConfig: (config: AzureDevOpsConfig) => void,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [signingOut, setSigningOut] = useState(false);
  const signOut = useCallback(async () => {
    setSigningOut(true);
    try {
      applyConfig(await signOutAzureDevOpsOAuth(workspaceId));
      toast({ description: t("azuredevops:oauthSignedOut"), variant: "success" });
    } catch (err) {
      toast({
        description: t("azuredevops:oauthSignOutFailed", { error: String(err) }),
        variant: "error",
      });
    } finally {
      setSigningOut(false);
    }
  }, [applyConfig, t, toast, workspaceId]);
  return { signingOut, signOut: () => void signOut() };
}
