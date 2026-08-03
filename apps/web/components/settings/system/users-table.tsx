"use client";

import { useCallback, useEffect, useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@kandev/ui/alert-dialog";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconMailForward, IconUserPlus, IconUsers } from "@tabler/icons-react";
import { useToast } from "@/components/toast-provider";
import { ApiError } from "@/lib/api/client";
import { listUsers, updateUser, type AuthUser } from "@/lib/api/domains/auth-api";
import { CreateUserDialog } from "./create-user-dialog";
import { InviteDialog } from "./invite-dialog";

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
    <Badge variant={status === "active" ? "default" : "secondary"} className="text-[10px]">
      {STATUS_LABEL_KEYS[status] ? t(STATUS_LABEL_KEYS[status]) : status}
    </Badge>
  );
}

function UserRow({
  user,
  onToggleRole,
  onToggleStatus,
}: {
  user: AuthUser;
  onToggleRole: (user: AuthUser) => void;
  onToggleStatus: (user: AuthUser) => void;
}) {
  const { t } = useTranslation();
  const isDisabled = user.status === "disabled";
  return (
    <TableRow data-testid="users-table-row" data-user-id={user.id}>
      {/* Email and display name are user data. */}
      <TableCell className="text-xs" data-testid="users-table-email">
        {user.email}
      </TableCell>
      <TableCell className="text-xs">{user.display_name}</TableCell>
      <TableCell>
        <Badge variant={user.role === "admin" ? "default" : "secondary"} className="text-[10px]">
          {roleLabel(user.role, t)}
        </Badge>
      </TableCell>
      <TableCell>
        <StatusBadge status={user.status} />
      </TableCell>
      <TableCell>
        <div className="flex items-center justify-end gap-1">
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer"
            onClick={() => onToggleRole(user)}
            data-testid="users-table-toggle-role"
          >
            {/* Two whole-word variants get their own keys rather than
                interpolating a role name into "Make {x}", which does not
                survive languages that inflect the object. */}
            {user.role === "admin" ? t("system:usersMakeMember") : t("system:usersMakeAdmin")}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer text-destructive"
            onClick={() => onToggleStatus(user)}
            data-testid="users-table-toggle-status"
          >
            {isDisabled ? t("system:usersEnable") : t("system:usersDisable")}
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

function UsersConfirmDialog({
  pending,
  onCancel,
  onConfirm,
}: {
  pending: PendingAction | null;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <AlertDialog open={pending !== null} onOpenChange={(open) => !open && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{pending?.label}</AlertDialogTitle>
          <AlertDialogDescription>
            {t("system:usersTakesEffectImmediately", { email: pending?.user.email ?? "" })}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} className="cursor-pointer">
            {t("system:usersConfirm")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
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
}: {
  users: AuthUser[];
  onToggleRole: (u: AuthUser) => void;
  onToggleStatus: (u: AuthUser) => void;
}) {
  const { t } = useTranslation();
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
          <UserRow
            key={user.id}
            user={user}
            onToggleRole={onToggleRole}
            onToggleStatus={onToggleStatus}
          />
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
  const { toast } = useToast();

  const onConfirm = async () => {
    if (!pending) return;
    try {
      await updateUser(pending.user.id, pending.next);
      await reload();
    } catch (err) {
      toast({
        variant: "error",
        title: t("system:usersUpdateFailed"),
        description: err instanceof ApiError ? err.message : t("system:usersLastAdminGuard"),
      });
    } finally {
      setPending(null);
    }
  };

  return (
    <Card data-testid="users-table-card">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-base flex items-center gap-2">
          <IconUsers className="h-4 w-4" /> {t("system:usersTitle")}
        </CardTitle>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer"
            onClick={() => setInviteOpen(true)}
            data-testid="users-table-invite"
          >
            <IconMailForward className="h-3.5 w-3.5" /> {t("system:usersInviteLink")}
          </Button>
          <Button
            size="sm"
            className="cursor-pointer"
            onClick={() => setCreateOpen(true)}
            data-testid="users-table-create"
          >
            <IconUserPlus className="h-3.5 w-3.5" /> {t("system:usersAddUser")}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <p className="text-xs text-destructive" data-testid="users-table-error">
            {error}
          </p>
        )}
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
      <UsersConfirmDialog
        pending={pending}
        onCancel={() => setPending(null)}
        onConfirm={() => void onConfirm()}
      />
    </Card>
  );
}
