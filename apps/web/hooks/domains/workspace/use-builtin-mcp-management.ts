import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getBackendConfig } from "@/lib/config";
import { ApiError } from "@/lib/api/client";
import * as api from "@/lib/api/domains/builtin-mcp-api";
import type {
  BuiltinMCPChoices,
  BuiltinMCPManagementView,
  BuiltinMCPPackage,
  BuiltinMCPPolicyChoice,
  BuiltinMCPPreference,
  BuiltinMCPSnapshot,
} from "@/lib/types/builtin-mcp";

type Scope = "workspaces" | "tasks";
type Choices = BuiltinMCPChoices<BuiltinMCPPolicyChoice | BuiltinMCPPreference>;
type Editor = {
  key: string;
  view: BuiltinMCPManagementView | null;
  draft: Choices;
  error: unknown;
  pending: boolean;
};
const choicesOf = (scope: Scope, value: BuiltinMCPSnapshot): Choices =>
  scope === "workspaces" ? value.policy.packages : (value.preferences?.packages ?? {});
const sameChoices = (a: Choices, b: Choices) =>
  [...new Set([...Object.keys(a), ...Object.keys(b)])].every(
    (id) => a[id as BuiltinMCPPackage] === b[id as BuiltinMCPPackage],
  );
function assertOwner(value: BuiltinMCPSnapshot, scope: Scope, id: string, workspace: string) {
  if (
    value.policy.workspace_id !== workspace ||
    (scope === "tasks" && value.preferences?.task_id !== id)
  ) {
    // i18n-exempt: machine-only response validation; UI displays a localized error category.
    throw new ApiError("builtin_mcp_owner_mismatch", 409, {
      error_code: "builtin_mcp_owner_mismatch",
    });
  }
}
function emptyEditor(key: string): Editor {
  return { key, view: null, draft: {}, error: null, pending: true };
}

async function persistChoices(
  scope: Scope,
  ownerId: string,
  view: BuiltinMCPManagementView,
  draft: Choices,
): Promise<BuiltinMCPSnapshot> {
  if (scope === "workspaces")
    return {
      policy: await api.saveBuiltinMCPPolicy(
        ownerId,
        view.policy.revision,
        draft as BuiltinMCPChoices<BuiltinMCPPolicyChoice>,
      ),
    };
  return api.saveBuiltinMCPPreferences(
    ownerId,
    view.preferences?.revision ?? 0,
    view.policy.revision,
    draft as BuiltinMCPChoices<BuiltinMCPPreference>,
  );
}

export function useBuiltinMCPManagement(scope: Scope, ownerId: string | null, workspaceId: string) {
  const auth = useAppStore((state) =>
    JSON.stringify([state.auth.mode, state.auth.authenticated, state.auth.user?.id]),
  );
  const key = JSON.stringify([scope, ownerId, workspaceId, auth, getBackendConfig().apiBaseUrl]);
  const owner = useRef({ key, live: true, sequence: 0, saving: false });
  if (owner.current.key !== key) owner.current = { key, live: true, sequence: 0, saving: false };
  const [state, setState] = useState<Editor>(() => emptyEditor(key));
  const editor = state.key === key ? state : emptyEditor(key);
  const latest = useRef(editor);
  latest.current = editor;
  const reload = useCallback(async () => {
    const captured = owner.current;
    if (!ownerId || captured.saving) return;
    const sequence = ++captured.sequence;
    const current = () =>
      owner.current === captured && captured.live && captured.sequence === sequence;
    setState((old) => ({ ...(old.key === key ? old : emptyEditor(key)), pending: true }));
    try {
      const view = await api.getBuiltinMCPManagement(scope, ownerId);
      assertOwner(view, scope, ownerId, workspaceId);
      if (current())
        setState({ key, view, draft: { ...choicesOf(scope, view) }, error: null, pending: false });
    } catch (error) {
      if (current()) setState((old) => ({ ...old, error, pending: false }));
    }
  }, [key, scope, ownerId, workspaceId]);
  useEffect(() => {
    const captured = owner.current;
    captured.live = true;
    void reload();
    return () => {
      captured.live = false;
      captured.sequence += 1;
    };
  }, [reload]);
  const save = useCallback(async () => {
    const captured = owner.current;
    const submitted = latest.current;
    if (!ownerId || !submitted.view?.can_edit || captured.saving || submitted.pending) return;
    captured.saving = true;
    ++captured.sequence;
    const current = () => owner.current === captured && captured.live;
    setState((old) => ({ ...old, pending: true, error: null }));
    try {
      const snapshot = await persistChoices(scope, ownerId, submitted.view, submitted.draft);
      assertOwner(snapshot, scope, ownerId, workspaceId);
      if (current())
        setState({
          key,
          view: { ...submitted.view, ...snapshot, desired: undefined },
          draft: { ...choicesOf(scope, snapshot) },
          error: null,
          pending: false,
        });
    } catch (error) {
      if (current()) setState((old) => ({ ...old, error, pending: false }));
      throw error;
    } finally {
      captured.saving = false;
    }
  }, [key, ownerId, scope, workspaceId]);
  const setChoice = (id: BuiltinMCPPackage, choice: Choices[BuiltinMCPPackage]) => {
    if (!editor.view?.can_edit || editor.pending || id === "core" || !choice) return;
    setState((old) => ({ ...old, draft: { ...old.draft, [id]: choice } }));
  };
  const discard = () =>
    setState((old) =>
      old.view ? { ...old, draft: { ...choicesOf(scope, old.view) }, error: null } : old,
    );
  const reset = () => {
    if (!editor.view?.can_edit || editor.pending) return;
    const draft: Choices = {};
    if (scope === "workspaces")
      for (const item of editor.view.catalog) {
        if (item.default_policy !== "required") draft[item.id] = item.default_policy;
      }
    setState((old) => ({ ...old, draft }));
  };
  return {
    ...editor,
    setChoice,
    save,
    reload,
    discard,
    reset,
    dirty: Boolean(editor.view && !sameChoices(editor.draft, choicesOf(scope, editor.view))),
  };
}
