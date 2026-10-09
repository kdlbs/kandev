"use client";

import { useCallback, useEffect, useState } from "react";
import { fetchLocalGitIdentity } from "@/lib/api/domains/settings-api";
import type { ExecutorProfile } from "@/lib/types/http";
import { getGitIdentityBaseline } from "./executor-profile-baselines";
import type { GitIdentityMode, GitIdentityState } from "./remote-credentials-card";

export function useGitIdentityState(isRemote: boolean, profile: ExecutorProfile) {
  const [localGitIdentity, setLocalGitIdentity] = useState<GitIdentityState>({
    userName: "",
    userEmail: "",
    detected: false,
  });
  const [gitIdentityMode, setGitIdentityMode] = useState<GitIdentityMode>("override");
  const [gitUserName, setGitUserName] = useState(profile.config?.git_user_name ?? "");
  const [gitUserEmail, setGitUserEmail] = useState(profile.config?.git_user_email ?? "");
  const [loaded, setLoaded] = useState(!isRemote);

  useEffect(() => {
    if (!isRemote) {
      setLoaded(true);
      return;
    }
    setLoaded(false);
    fetchLocalGitIdentity()
      .then((identity) => {
        const local: GitIdentityState = {
          userName: identity.user_name ?? "",
          userEmail: identity.user_email ?? "",
          detected: Boolean(identity.detected),
        };
        setLocalGitIdentity(local);

        const hasStoredOverride = Boolean(
          profile.config?.git_user_name?.trim() || profile.config?.git_user_email?.trim(),
        );
        if (hasStoredOverride) {
          setGitIdentityMode("override");
          return;
        }
        if (local.detected) {
          setGitIdentityMode("local");
          setGitUserName(local.userName);
          setGitUserEmail(local.userEmail);
          return;
        }
        setGitIdentityMode("override");
      })
      .catch(() => {})
      .finally(() => setLoaded(true));
  }, [isRemote, profile.config?.git_user_email, profile.config?.git_user_name]);

  const reset = useCallback(() => {
    const baseline = getGitIdentityBaseline(profile, localGitIdentity);
    setGitIdentityMode(baseline.mode);
    setGitUserName(baseline.userName);
    setGitUserEmail(baseline.userEmail);
  }, [localGitIdentity, profile]);

  return {
    localGitIdentity,
    gitIdentityMode,
    setGitIdentityMode,
    gitUserName,
    setGitUserName,
    gitUserEmail,
    setGitUserEmail,
    loaded,
    reset,
  };
}
