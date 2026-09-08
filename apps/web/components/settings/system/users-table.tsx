"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconMailForward, IconUserPlus, IconUsers } from "@tabler/icons-react";
import { ActionConfirmPopover } from "@/components/confirmation/action-confirm-popover";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { useToast } from "@/components/toast-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { ApiError } from "@/lib/api/client";
import { listUsers, updateUser, type AuthUser } from "@/lib/api/domains/auth-api";
import { CreateUserDialog } from "./create-user-dialog";
import { InviteDialog } from "./invite-dialog";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import { SettingsErrorText } from "@/components/settings/settings-typography";
import { settingsActionClassName } from "@/components/settings/settings-control";

type PendingAction = { user: AuthUser; next: { role?: string; status?: string }; label: string };

/**
 * `role` and `status` are wire values sent straight back to the API, so the
 * `next` payload above always carries the raw token. Only these badge/button
 * labels are copy, and an unrecognized value from a newer backend echoes the
 * token rather than rendering blank.
 */
const ROLE_LABEL_KEYS: Record<string, string> = {
  admin: "system:usersRoleAdmin",
  member: "system:usersRoleMember",
};

const STATUS_LABEL_KEYS: Record<string, string> = {
  active: "system:usersStatusActive",
  disabled: "system:usersStatusDisabled",
};

function roleLabel(role: string, t: TFunction): string {
  return ROLE_LABEL_KEYS[role] ? t(ROLE_LABEL_KEYS[role]) : role;
}

function useUsersList(t: TFunction) {
  const [users, setUsers] = useState<AuthUser[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await listUsers({ cache: "no-store" });
      setUsers(res.users);
      setLoaded(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : t("system:usersLoadFailed"));
    } finally {
      setIsLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { users, loaded, isLoading, error, reload };
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  return (
    <Badge
      variant={status === "active" ? "default" : "secondary"}
      className="text-[10px]"
      data-testid="users-table-status"
    >
      {STATUS_LABEL_KEYS[status] ? t(STATUS_LABEL_KEYS[status]) : status}
    </Badge>
  );
}

type UserRowProps = {
  user: AuthUser;
  isLastActiveAdmin: boolean;
  isFinePointer: boolean;
  isMutating: boolean;
  pending: PendingAction | null;
  onToggleRole: (user: AuthUser) => void;
  onToggleStatus: (user: AuthUser) => void;
  onCancel: () => void;
  onConfirm: (action: PendingAction) => void;
};

function UserRow({ user, ...props }: UserRowProps) {
  const { t } = useTranslation();
  return (
    <TableRow data-testid="users-table-row" data-user-id={user.id}>
      {/* Email and display name are user data. */}
      <TableCell className="text-xs" data-testid="users-table-email">
        {user.email}
      </TableCell>
      <TableCell className="text-xs">{user.display_name}</TableCell>
      <TableCell>
        <Badge
          variant={user.role === "admin" ? "default" : "secondary"}
          className="text-[10px]"
          data-testid="users-table-role"
        >
          {roleLabel(user.role, t)}
        </Badge>
      </TableCell>
      <TableCell>
        <StatusBadge status={user.status} />
      </TableCell>
      <TableCell>
        <UserActionRegion user={user} {...props} />
      </TableCell>
    </TableRow>
  );
}

function MobileUserRow({ user, ...props }: UserRowProps) {
  const { t } = useTranslation();
  return (
    <article
      className="space-y-3 rounded-md border p-3"
      data-testid="users-table-row"
      data-user-id={user.id}
    >
      {/* Identity and current state stay above the row-owned action region on phones. */}
      <div className="min-w-0 space-y-1">
        <p className="break-words text-xs font-medium" data-testid="users-table-email">
          {user.email}
        </p>
        <p className="break-words text-xs text-muted-foreground">{user.display_name}</p>
      </div>
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <span className="text-muted-foreground">{t("system:usersColumnRole")}:</span>
        <Badge
          variant={user.role === "admin" ? "default" : "secondary"}
          className="text-[10px]"
          data-testid="users-table-role"
        >
          {roleLabel(user.role, t)}
        </Badge>
        <span className="ml-1 text-muted-foreground">{t("system:usersColumnStatus")}:</span>
        <StatusBadge status={user.status} />
      </div>
      <UserActionRegion user={user} {...props} />
    </article>
  );
}

function UserActionRegion({
  user,
  isLastActiveAdmin,
  isFinePointer,
  isMutating,
  pending,
  onToggleRole,
  onToggleStatus,
  onCancel,
  onConfirm,
}: UserRowProps) {
  const { t } = useTranslation();
  const roleAnchorRef = useRef<HTMLButtonElement>(null);
  const statusAnchorRef = useRef<HTMLButtonElement>(null);
  const action = pending?.user.id === user.id ? pending : null;
  const anchorRef = action?.next.role !== undefined ? roleAnchorRef : statusAnchorRef;

  if (!isFinePointer && action) {
    return (
      <InlineConfirmActions
        density="touch"
        testId="users-table-inline-confirmation"
        ariaLabel={action.label}
        description={action.label}
        cancelLabel={t("common:cancel")}
        confirmLabel={t("system:usersConfirm")}
        confirmAriaLabel={action.label}
        confirmTestId="users-table-confirm"
        onCancel={onCancel}
        onClose={onCancel}
        onConfirm={() => onConfirm(action)}
      />
    );
  }

  return (
    <>
      <UserActionButtons
        user={user}
        isLastActiveAdmin={isLastActiveAdmin}
        isFinePointer={isFinePointer}
        roleAnchorRef={roleAnchorRef}
        statusAnchorRef={statusAnchorRef}
        onToggleRole={onToggleRole}
        onToggleStatus={onToggleStatus}
        isMutating={isMutating}
      />
      {isFinePointer && action ? (
        <UserConfirmPopover
          action={action}
          anchorRef={anchorRef}
          onCancel={onCancel}
          onConfirm={onConfirm}
        />
      ) : null}
    </>
  );
}

function UserActionButtons({
  user,
  isLastActiveAdmin,
  isFinePointer,
  isMutating,
  roleAnchorRef,
  statusAnchorRef,
  onToggleRole,
  onToggleStatus,
}: {
  user: AuthUser;
  isLastActiveAdmin: boolean;
  isFinePointer: boolean;
  isMutating: boolean;
  roleAnchorRef: RefObject<HTMLButtonElement | null>;
  statusAnchorRef: RefObject<HTMLButtonElement | null>;
  onToggleRole: (user: AuthUser) => void;
  onToggleStatus: (user: AuthUser) => void;
}) {
  const { t } = useTranslation();
  const roleButtonClassName = isFinePointer ? "cursor-pointer" : "min-h-11 cursor-pointer";
  const statusButtonClassName = isFinePointer
    ? "cursor-pointer text-destructive"
    : "min-h-11 cursor-pointer text-destructive";

  return (
    <div
      className={isFinePointer ? "flex items-center justify-end gap-1" : "grid grid-cols-2 gap-2"}
      data-testid="users-table-action-region"
    >
      <Button
        ref={roleAnchorRef}
        size={isFinePointer ? "sm" : "default"}
        variant="ghost"
        className={roleButtonClassName}
        onClick={() => onToggleRole(user)}
        disabled={isLastActiveAdmin || isMutating}
        data-testid="users-table-toggle-role"
      >
        {/* Two whole-word variants get their own keys rather than
            interpolating a role name into "Make {x}", which does not
            survive languages that inflect the object. */}
        {user.role === "admin" ? t("system:usersMakeMember") : t("system:usersMakeAdmin")}
      </Button>
      <Button
        ref={statusAnchorRef}
        size={isFinePointer ? "sm" : "default"}
        variant="ghost"
        className={statusButtonClassName}
        onClick={() => onToggleStatus(user)}
        disabled={isLastActiveAdmin || isMutating}
        data-testid="users-table-toggle-status"
      >
        {user.status === "disabled" ? t("system:usersEnable") : t("system:usersDisable")}
      </Button>
    </div>
  );
}

function UserConfirmPopover({
  action,
  anchorRef,
  onCancel,
  onConfirm,
}: {
  action: PendingAction;
  anchorRef: RefObject<HTMLElement | null>;
  onCancel: () => void;
  onConfirm: (action: PendingAction) => void;
}) {
  const { t } = useTranslation();
  return (
    <ActionConfirmPopover
      open
      anchorRef={anchorRef}
      title={action.label}
      description={t("system:usersTakesEffectImmediately", { email: action.user.email })}
      cancelLabel={t("common:cancel")}
      confirmLabel={t("system:usersConfirm")}
      confirmAriaLabel={action.label}
      confirmTestId="users-table-confirm"
      testId="users-table-confirm-popover"
      onOpenChange={(open) => !open && onCancel()}
      onConfirm={() => onConfirm(action)}
    />
  );
}

/**
 * These two build the confirmation sentence outside JSX, which is why
 * `mode: "jsx-only"` never reported them. The email is interpolated as a value
 * and the target role travels as its own translated label.
 */
function roleTogglePending(u: AuthUser, t: TFunction): PendingAction {
  const next = u.role === "admin" ? "member" : "admin";
  return {
    user: u,
    next: { role: next },
    label: t("system:usersConfirmRoleChange", { email: u.email, role: roleLabel(next, t) }),
  };
}

function statusTogglePending(u: AuthUser, t: TFunction): PendingAction {
  const disabling = u.status !== "disabled";
  return {
    user: u,
    next: { status: disabling ? "disabled" : "active" },
    label: disabling
      ? t("system:usersConfirmDisable", { email: u.email })
      : t("system:usersConfirmEnable", { email: u.email }),
  };
}

function UsersTableList({
  users,
  onToggleRole,
  onToggleStatus,
  pending,
  isMutating,
  onCancel,
  onConfirm,
}: {
  users: AuthUser[];
  onToggleRole: (u: AuthUser) => void;
  onToggleStatus: (u: AuthUser) => void;
  pending: PendingAction | null;
  isMutating: boolean;
  onCancel: () => void;
  onConfirm: (action: PendingAction) => void;
}) {
  const { t } = useTranslation();
  const { isFinePointer, isMobile } = useResponsiveBreakpoint();
  // Mirrors the backend last-admin guard (ensureAnotherAdmin): an active
  // admin cannot be demoted or disabled when no other active admin exists,
  // so those toggles are greyed out on that row.
  const activeAdminCount = users.filter((u) => u.role === "admin" && u.status === "active").length;

  const rowProps = (user: AuthUser): UserRowProps => ({
    user,
    isLastActiveAdmin: user.role === "admin" && user.status === "active" && activeAdminCount === 1,
    isFinePointer,
    isMutating,
    pending,
    onToggleRole,
    onToggleStatus,
    onCancel,
    onConfirm,
  });

  if (isMobile) {
    return (
      <div className="space-y-3" data-testid="users-table-mobile-list">
        {users.map((user) => (
          <MobileUserRow key={user.id} {...rowProps(user)} />
        ))}
      </div>
    );
  }

  return (
    <Table data-testid="users-table">
      <TableHeader>
        <TableRow>
          <TableHead>{t("system:usersColumnEmail")}</TableHead>
          <TableHead>{t("system:usersColumnName")}</TableHead>
          <TableHead>{t("system:usersColumnRole")}</TableHead>
          <TableHead>{t("system:usersColumnStatus")}</TableHead>
          <TableHead className="text-right">{t("system:usersColumnActions")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {users.map((user) => (
          <UserRow key={user.id} {...rowProps(user)} />
        ))}
      </TableBody>
    </Table>
  );
}

export function UsersTable() {
  const { t } = useTranslation();
  const { users, loaded, isLoading, error, reload } = useUsersList(t);
  const [createOpen, setCreateOpen] = useState(false);
  const [inviteOpen, setInviteOpen] = useState(false);
  const [pending, setPending] = useState<PendingAction | null>(null);
  const [activeMutation, setActiveMutation] = useState<PendingAction | null>(null);
  const { toast } = useToast();

  useEffect(() => {
    if (pending && !users.some((user) => user.id === pending.user.id)) setPending(null);
  }, [pending, users]);

  const onConfirm = async (action: PendingAction) => {
    if (activeMutation) return;
    setActiveMutation(action);
    try {
      await updateUser(action.user.id, action.next);
      await reload();
    } catch (err) {
      toast({
        variant: "error",
        title: t("system:usersUpdateFailed"),
        description: err instanceof ApiError ? err.message : t("system:usersLastAdminGuard"),
      });
    } finally {
      setPending((current) => (current === action ? null : current));
      setActiveMutation((current) => (current === action ? null : current));
    }
  };

  return (
    <Card data-testid="users-table-card">
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconUsers className="h-4 w-4" /> {t("system:usersTitle")}
          </span>
        }
        actions={
          <div className="flex w-full flex-col gap-2 md:w-auto md:flex-row">
            <Button
              size="sm"
              variant="outline"
              className={settingsActionClassName("cursor-pointer")}
              onClick={() => setInviteOpen(true)}
              data-testid="users-table-invite"
            >
              <IconMailForward className="h-3.5 w-3.5" /> {t("system:usersInviteLink")}
            </Button>
            <Button
              size="sm"
              className={settingsActionClassName("cursor-pointer")}
              onClick={() => setCreateOpen(true)}
              data-testid="users-table-create"
            >
              <IconUserPlus className="h-3.5 w-3.5" /> {t("system:usersAddUser")}
            </Button>
          </div>
        }
      />
      <CardContent className="space-y-4">
        {error && <SettingsErrorText data-testid="users-table-error">{error}</SettingsErrorText>}
        {!loaded && isLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> {t("system:usersLoading")}
          </div>
        )}
        {loaded && users.length > 0 && (
          <UsersTableList
            users={users}
            onToggleRole={(u) => setPending(roleTogglePending(u, t))}
            onToggleStatus={(u) => setPending(statusTogglePending(u, t))}
            pending={pending}
            isMutating={activeMutation !== null}
            onCancel={() => setPending(null)}
            onConfirm={(action) => void onConfirm(action)}
          />
        )}
        {loaded && users.length === 0 && !error && (
          <p className="text-sm text-muted-foreground" data-testid="users-table-empty">
            {t("system:usersEmpty")}
          </p>
        )}
      </CardContent>
      <CreateUserDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={() => void reload()}
      />
      <InviteDialog
        open={inviteOpen}
        onOpenChange={setInviteOpen}
        onCreated={() => void reload()}
      />
    </Card>
  );
}
