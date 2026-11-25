"use client";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useAppStore } from "@/components/state-provider";
import { usePathname } from "@/lib/routing/client-router";
import { useFeature } from "@/hooks/domains/features/use-feature";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { SSH_CONNECTIONS_SETTINGS_HREF } from "@/lib/settings/ssh-connection-settings-routes";

export type SSHSetupDraft = {
  editor: string;
  name: string;
  connection: string;
  baseline: string;
  ownerUserId: string;
};
type Continuation = {
  draft: SSHSetupDraft | null;
  begin: (draft: SSHSetupDraft) => void;
  clear: () => void;
  select: (connection: string) => void;
};
const Context = createContext<Continuation | null>(null);
const empty: Continuation = { draft: null, begin: () => {}, clear: () => {}, select: () => {} };

export function SSHSetupContinuationProvider({ children }: { children: ReactNode }) {
  const pathname = usePathname();
  const enabled = useFeature("sshConnectionManager");
  const admin = useIsAdmin();
  const scope = useAppStore(
    (s) =>
      `${s.auth.mode}:${s.auth.authenticated}:${s.auth.user?.id ?? ""}:${s.auth.user?.role ?? ""}`,
  );
  const [saved, setSaved] = useState<{ scope: string; draft: SSHSetupDraft } | null>(null);
  const inside =
    saved &&
    (pathname === saved.draft.editor ||
      pathname === SSH_CONNECTIONS_SETTINGS_HREF ||
      pathname === "/settings/ssh-connections");
  const valid = enabled && admin && saved?.scope === scope && inside;
  const draft = valid && saved ? saved.draft : null;
  const clear = useCallback(() => setSaved(null), []);
  useEffect(() => {
    if (!valid) clear();
  }, [valid, clear]);
  const begin = useCallback(
    (value: SSHSetupDraft) => {
      if (!enabled || !admin || !isEditorPath(value.editor) || pathname !== value.editor) return;
      // Copy only public metadata, even if a caller carries extra form fields.
      const { editor, name, connection, baseline, ownerUserId } = value;
      setSaved({ scope, draft: { editor, name, connection, baseline, ownerUserId } });
    },
    [enabled, admin, scope, pathname],
  );
  const select = useCallback((connection: string) => {
    setSaved((current) =>
      current ? { ...current, draft: { ...current.draft, connection } } : null,
    );
  }, []);
  const value = useMemo(() => ({ draft, begin, clear, select }), [draft, begin, clear, select]);
  return <Context.Provider value={value}>{children}</Context.Provider>;
}
export function useSSHSetupContinuation(): Continuation {
  return useContext(Context) ?? empty;
}
function isEditorPath(path: string): boolean {
  return (
    path === "/settings/executors/new/ssh" || /^\/settings\/executors\/ssh\/[^/?#]+$/.test(path)
  );
}
