"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";
import {
  listDirectoryUsers,
  listWorkspaceMembers,
  removeWorkspaceMember,
  setWorkspaceVisibility,
  transferWorkspaceOwnership,
  upsertWorkspaceMember,
} from "@/lib/api/domains/team-access-api";
import {
  hasScope,
  SCOPE,
  type AssignableWorkspaceRole,
  type DirectoryUser,
  type WorkspaceMember,
  type WorkspaceVisibility,
} from "@/lib/types/team-access";

type UseWorkspaceTeamAccessArgs = {
  workspaceId: string;
  ownerId: string;
  visibility: WorkspaceVisibility;
  scopes: readonly string[] | undefined;
};

/**
 * Membership and visibility state for one workspace.
 *
 * Capability flags come from the server-issued scopes rather than from
 * comparing the current user to the owner: the backend is authoritative, and a
 * client that derives permission independently will eventually derive it
 * differently.
 */
export function useWorkspaceTeamAccess({
  workspaceId,
  ownerId,
  visibility,
  scopes,
}: UseWorkspaceTeamAccessArgs) {
  const { t } = useTranslation();
  const { toast } = useToast();

  const [currentVisibility, setCurrentVisibility] = useState<WorkspaceVisibility>(visibility);
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [directory, setDirectory] = useState<DirectoryUser[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => setCurrentVisibility(visibility), [visibility]);

  const refresh = useCallback(async () => {
    try {
      const [memberList, users] = await Promise.all([
        listWorkspaceMembers(workspaceId),
        listDirectoryUsers(),
      ]);
      setMembers(memberList.members ?? []);
      setDirectory(users.users ?? []);
    } catch {
      // A read failure leaves the previous list on screen. Mutations surface
      // their own errors, and blanking the card would lose more than it tells.
    }
  }, [workspaceId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const run = useCallback(
    async (action: () => Promise<unknown>, successMessage: string) => {
      setBusy(true);
      try {
        await action();
        await refresh();
        toast({ title: successMessage });
      } catch (error) {
        toast({
          title: error instanceof Error ? error.message : t("workspaces:teamAccess.genericError"),
          variant: "error",
        });
      } finally {
        setBusy(false);
      }
    },
    [refresh, toast, t],
  );

  const actions = useTeamAccessActions({
    workspaceId,
    run,
    members,
    directory,
    currentVisibility,
    setCurrentVisibility,
  });

  const memberIds = useMemo(() => new Set(members.map((member) => member.user_id)), [members]);
  const addableUsers = useMemo(
    () => directory.filter((user) => !memberIds.has(user.id) && user.id !== ownerId),
    [directory, memberIds, ownerId],
  );

  return {
    busy,
    members,
    addableUsers,
    currentVisibility,
    canManageWorkspace: hasScope(scopes, SCOPE.workspaceManage),
    canManageMembers: hasScope(scopes, SCOPE.memberManage),
    ...actions,
  };
}

type TeamAccessActionsArgs = {
  workspaceId: string;
  run: (action: () => Promise<unknown>, successMessage: string) => Promise<void>;
  members: WorkspaceMember[];
  directory: DirectoryUser[];
  currentVisibility: WorkspaceVisibility;
  setCurrentVisibility: (next: WorkspaceVisibility) => void;
};

/** The mutation half of team access, split out to keep each hook readable. */
function useTeamAccessActions({
  workspaceId,
  run,
  members,
  directory,
  currentVisibility,
  setCurrentVisibility,
}: TeamAccessActionsArgs) {
  const { t } = useTranslation();

  const nameFor = useCallback(
    (userId: string) =>
      members.find((member) => member.user_id === userId)?.display_name ??
      directory.find((user) => user.id === userId)?.display_name ??
      userId,
    [members, directory],
  );

  const changeVisibility = useCallback(
    (next: WorkspaceVisibility) => {
      const previous = currentVisibility;
      setCurrentVisibility(next);
      void run(
        () =>
          setWorkspaceVisibility(workspaceId, next).catch((error) => {
            setCurrentVisibility(previous);
            throw error;
          }),
        next === "org"
          ? t("workspaces:teamAccess.nowOrgVisible")
          : t("workspaces:teamAccess.nowPrivate"),
      );
    },
    [currentVisibility, run, setCurrentVisibility, t, workspaceId],
  );

  const changeRole = useCallback(
    (userId: string, role: AssignableWorkspaceRole) =>
      void run(
        () => upsertWorkspaceMember(workspaceId, userId, role),
        t("workspaces:teamAccess.roleUpdated"),
      ),
    [run, t, workspaceId],
  );

  const addMember = useCallback(
    (userId: string, role: AssignableWorkspaceRole, onAdded: () => void) =>
      void run(async () => {
        await upsertWorkspaceMember(workspaceId, userId, role);
        onAdded();
      }, t("workspaces:teamAccess.memberAdded")),
    [run, t, workspaceId],
  );

  const removeMember = useCallback(
    (userId: string) =>
      void run(
        () => removeWorkspaceMember(workspaceId, userId),
        t("workspaces:teamAccess.memberRemoved"),
      ),
    [run, t, workspaceId],
  );

  const makeOwner = useCallback(
    (userId: string) =>
      void run(
        () => transferWorkspaceOwnership(workspaceId, userId),
        t("workspaces:teamAccess.ownershipTransferred", { name: nameFor(userId) }),
      ),
    [nameFor, run, t, workspaceId],
  );

  return { changeVisibility, changeRole, addMember, removeMember, makeOwner };
}
