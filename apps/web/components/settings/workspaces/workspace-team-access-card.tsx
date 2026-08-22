"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconTrash, IconCrown, IconUsers } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Badge } from "@kandev/ui/badge";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { SettingsCard } from "@/components/settings/settings-card";
import { useWorkspaceTeamAccess } from "@/hooks/domains/workspace/use-workspace-team-access";
import {
  ASSIGNABLE_WORKSPACE_ROLES,
  type AssignableWorkspaceRole,
  type DirectoryUser,
  type WorkspaceMember,
  type WorkspaceVisibility,
} from "@/lib/types/team-access";

type WorkspaceTeamAccessCardProps = {
  workspaceId: string;
  ownerId: string;
  visibility: WorkspaceVisibility;
  /** Scopes the *requesting* user holds here, from the workspace payload. */
  scopes: readonly string[] | undefined;
};

function useRoleLabel() {
  const { t } = useTranslation();
  return (role: AssignableWorkspaceRole) =>
    role === "collaborator"
      ? t("workspaces:teamAccess.roleCollaborator")
      : t("workspaces:teamAccess.roleViewer");
}

function RoleOptions() {
  const roleLabel = useRoleLabel();
  return (
    <>
      {ASSIGNABLE_WORKSPACE_ROLES.map((role) => (
        <SelectItem key={role} value={role}>
          {roleLabel(role)}
        </SelectItem>
      ))}
    </>
  );
}

type VisibilitySectionProps = {
  value: WorkspaceVisibility;
  disabled: boolean;
  onChange: (next: WorkspaceVisibility) => void;
};

function VisibilitySection({ value, disabled, onChange }: VisibilitySectionProps) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <Label htmlFor="workspace-visibility">{t("workspaces:teamAccess.visibilityLabel")}</Label>
      <Select
        value={value}
        onValueChange={(next) => onChange(next as WorkspaceVisibility)}
        disabled={disabled}
      >
        <SelectTrigger id="workspace-visibility" className="max-w-sm cursor-pointer">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="private">{t("workspaces:teamAccess.visibilityPrivate")}</SelectItem>
          <SelectItem value="org">{t("workspaces:teamAccess.visibilityOrg")}</SelectItem>
        </SelectContent>
      </Select>
      <p className="text-muted-foreground text-sm">
        {value === "org"
          ? t("workspaces:teamAccess.visibilityOrgHint")
          : t("workspaces:teamAccess.visibilityPrivateHint")}
      </p>
    </div>
  );
}

type MemberRowProps = {
  member: WorkspaceMember;
  canManageMembers: boolean;
  busy: boolean;
  onChangeRole: (userId: string, role: AssignableWorkspaceRole) => void;
  onRemove: (userId: string) => void;
  onMakeOwner: (userId: string) => void;
};

function MemberRow({
  member,
  canManageMembers,
  busy,
  onChangeRole,
  onRemove,
  onMakeOwner,
}: MemberRowProps) {
  const { t } = useTranslation();
  const isOwner = member.role === "owner";
  return (
    <li className="flex flex-wrap items-center gap-3 p-3" data-testid="workspace-member-row">
      <span className="min-w-0 flex-1 truncate text-sm">
        {member.display_name || member.user_id}
      </span>
      {isOwner ? (
        <Badge variant="secondary" className="gap-1">
          <IconCrown className="size-3" aria-hidden />
          {t("workspaces:teamAccess.roleOwner")}
        </Badge>
      ) : (
        <Select
          value={member.role}
          disabled={!canManageMembers || busy}
          onValueChange={(role) => onChangeRole(member.user_id, role as AssignableWorkspaceRole)}
        >
          <SelectTrigger
            className="w-40 cursor-pointer"
            aria-label={t("workspaces:teamAccess.roleLabel")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <RoleOptions />
          </SelectContent>
        </Select>
      )}
      {canManageMembers && !isOwner ? (
        <>
          <Button
            variant="ghost"
            size="sm"
            className="cursor-pointer"
            disabled={busy}
            onClick={() => onMakeOwner(member.user_id)}
          >
            {t("workspaces:teamAccess.makeOwner")}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="cursor-pointer"
            disabled={busy}
            aria-label={t("workspaces:teamAccess.removeMember")}
            onClick={() => onRemove(member.user_id)}
          >
            <IconTrash className="size-4" aria-hidden />
          </Button>
        </>
      ) : null}
    </li>
  );
}

type AddMemberRowProps = {
  users: DirectoryUser[];
  busy: boolean;
  onAdd: (userId: string, role: AssignableWorkspaceRole, onAdded: () => void) => void;
};

function AddMemberRow({ users, busy, onAdd }: AddMemberRowProps) {
  const { t } = useTranslation();
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<AssignableWorkspaceRole>("collaborator");

  return (
    <div className="flex flex-wrap items-end gap-2">
      <div className="min-w-48 flex-1 space-y-1">
        <Label htmlFor="add-member-user">{t("workspaces:teamAccess.addMemberLabel")}</Label>
        <Select value={userId} onValueChange={setUserId} disabled={busy || users.length === 0}>
          <SelectTrigger id="add-member-user" className="cursor-pointer">
            <SelectValue placeholder={t("workspaces:teamAccess.addMemberPlaceholder")} />
          </SelectTrigger>
          <SelectContent>
            {users.map((user) => (
              <SelectItem key={user.id} value={user.id}>
                {user.display_name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="w-40 space-y-1">
        <Label htmlFor="add-member-role">{t("workspaces:teamAccess.roleLabel")}</Label>
        <Select
          value={role}
          onValueChange={(next) => setRole(next as AssignableWorkspaceRole)}
          disabled={busy}
        >
          <SelectTrigger id="add-member-role" className="cursor-pointer">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <RoleOptions />
          </SelectContent>
        </Select>
      </div>
      <Button
        className="cursor-pointer"
        disabled={busy || !userId}
        onClick={() => onAdd(userId, role, () => setUserId(""))}
      >
        {t("workspaces:teamAccess.addMember")}
      </Button>
      {users.length === 0 ? (
        <p className="text-muted-foreground w-full text-sm">
          {t("workspaces:teamAccess.noUsersToAdd")}
        </p>
      ) : null}
    </div>
  );
}

/**
 * Workspace visibility and membership.
 *
 * Visibility is the primary mechanism: a team sets it once and never invites
 * anyone. Membership below is the exception path, for private workspaces,
 * guests, and narrowing a colleague to a viewer.
 */
export function WorkspaceTeamAccessCard(props: WorkspaceTeamAccessCardProps) {
  const { t } = useTranslation();
  const access = useWorkspaceTeamAccess(props);

  return (
    <SettingsCard>
      <CardHeader>
        <CardTitle>{t("workspaces:teamAccess.title")}</CardTitle>
        <CardDescription>{t("workspaces:teamAccess.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <VisibilitySection
          value={access.currentVisibility}
          disabled={!access.canManageWorkspace || access.busy}
          onChange={access.changeVisibility}
        />

        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <IconUsers className="text-muted-foreground size-4" aria-hidden />
            <Label>{t("workspaces:teamAccess.membersLabel")}</Label>
          </div>

          <ul className="divide-border divide-y rounded-md border">
            {access.members.length === 0 ? (
              <li className="text-muted-foreground p-3 text-sm">
                {t("workspaces:teamAccess.noMembers")}
              </li>
            ) : (
              access.members.map((member) => (
                <MemberRow
                  key={member.user_id}
                  member={member}
                  canManageMembers={access.canManageMembers}
                  busy={access.busy}
                  onChangeRole={access.changeRole}
                  onRemove={access.removeMember}
                  onMakeOwner={access.makeOwner}
                />
              ))
            )}
          </ul>

          {access.canManageMembers ? (
            <AddMemberRow users={access.addableUsers} busy={access.busy} onAdd={access.addMember} />
          ) : null}
        </div>
      </CardContent>
    </SettingsCard>
  );
}
