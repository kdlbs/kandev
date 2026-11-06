"use client";

import { useRef, useState } from "react";
import { cloneWorkspaceAction } from "@/app/actions/workspaces";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useRouter } from "@/lib/routing/client-router";
import { mapWorkspaceItem } from "@/lib/routing/route-bootstrap";
import { workspaceSettingsHref } from "@/lib/settings/workspace-settings-tabs";
import { isOfficeWorkspace, type WorkspaceItem } from "@/lib/state/slices/workspace/selectors";

export function canCloneWorkspace(workspace: WorkspaceItem): boolean {
  // i18n-exempt: Reserved managed workspace identity, never display copy.
  const isManaged = workspace.name === "Improve Kandev";
  return (
    !isManaged &&
    !isOfficeWorkspace(workspace) &&
    Boolean(
      workspace.scopes?.includes("workspace.manage") && workspace.scopes.includes("secret.manage"),
    )
  );
}

function cloneErrorKey(error: unknown): string {
  const status = (error as { status?: number } | null)?.status;
  if (!status) return "workspaces:cloneUncertain";
  if (status === 403) return "workspaces:cloneForbidden";
  if (status === 404) return "workspaces:cloneSourceMissing";
  if (status === 409) return "workspaces:cloneConfigurationUnsupported";
  return "workspaces:cloneFailed";
}

export function useWorkspaceClone(translate: (key: string, options?: { name: string }) => string) {
  const store = useAppStoreApi();
  const router = useRouter();
  const { toast } = useToast();
  const [source, setSource] = useState<WorkspaceItem | null>(null);
  const [name, setName] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inFlight = useRef(false);
  const trigger = useRef<HTMLElement | null>(null);

  const open = (workspace: WorkspaceItem) => {
    if (inFlight.current) return;
    trigger.current = document.activeElement as HTMLElement | null;
    setSource(workspace);
    setName(translate("workspaces:cloneNameSuggestion", { name: workspace.name }));
    setError(null);
  };
  const close = () => {
    if (inFlight.current) return;
    setSource(null);
    setName("");
    setError(null);
  };
  const restoreFocus = (event: Event) => {
    event.preventDefault();
    trigger.current?.focus();
  };
  const submit = async () => {
    if (inFlight.current || !source || !name.trim()) return;
    inFlight.current = true;
    setPending(true);
    setError(null);
    try {
      const created = await cloneWorkspaceAction(source.id, { name: name.trim() });
      store.setState((state) => ({
        workspaces: {
          ...state.workspaces,
          items: [
            mapWorkspaceItem(created),
            ...state.workspaces.items.filter((item) => item.id !== created.id),
          ],
        },
      }));
      setSource(null);
      setName("");
      toast({
        title: translate("workspaces:cloneSuccess"),
        description: created.name,
        variant: "success",
      });
      router.push(workspaceSettingsHref(created.id, "overview"));
    } catch (error) {
      setError(translate(cloneErrorKey(error)));
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  };
  return { source, name, setName, pending, error, open, close, submit, restoreFocus };
}

export type WorkspaceCloneFlow = ReturnType<typeof useWorkspaceClone>;
