"use client";

import { useCallback, useEffect, useState } from "react";
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

function useUsersList() {
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
      setError(err instanceof ApiError ? err.message : "Failed to load users.");
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return { users, loaded, isLoading, error, reload };
}

function StatusBadge({ status }: { status: string }) {
  return (
    <Badge variant={status === "active" ? "default" : "secondary"} className="text-[10px]">
      {status}
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
  const isDisabled = user.status === "disabled";
  return (
    <TableRow data-testid="users-table-row" data-user-id={user.id}>
      <TableCell className="text-xs" data-testid="users-table-email">
        {user.email}
      </TableCell>
      <TableCell className="text-xs">{user.display_name}</TableCell>
      <TableCell>
        <Badge variant={user.role === "admin" ? "default" : "secondary"} className="text-[10px]">
          {user.role}
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
            Make {user.role === "admin" ? "member" : "admin"}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer text-destructive"
            onClick={() => onToggleStatus(user)}
            data-testid="users-table-toggle-status"
          >
            {isDisabled ? "Enable" : "Disable"}
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
  return (
    <AlertDialog open={pending !== null} onOpenChange={(open) => !open && onCancel()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{pending?.label}</AlertDialogTitle>
          <AlertDialogDescription>
            {pending?.user.email} — this takes effect immediately.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} className="cursor-pointer">
            Confirm
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function roleTogglePending(u: AuthUser): PendingAction {
  const next = u.role === "admin" ? "member" : "admin";
  return { user: u, next: { role: next }, label: `Change ${u.email} to ${next}?` };
}

function statusTogglePending(u: AuthUser): PendingAction {
  const disabling = u.status !== "disabled";
  return {
    user: u,
    next: { status: disabling ? "disabled" : "active" },
    label: disabling
      ? `Disable ${u.email}? They will be signed out everywhere.`
      : `Re-enable ${u.email}?`,
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
  return (
    <Table data-testid="users-table">
      <TableHeader>
        <TableRow>
          <TableHead>Email</TableHead>
          <TableHead>Name</TableHead>
          <TableHead>Role</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Actions</TableHead>
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
  const { users, loaded, isLoading, error, reload } = useUsersList();
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
        title: "Could not update user",
        description:
          err instanceof ApiError
            ? err.message
            : "This deployment must keep at least one active admin.",
      });
    } finally {
      setPending(null);
    }
  };

  return (
    <Card data-testid="users-table-card">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-base flex items-center gap-2">
          <IconUsers className="h-4 w-4" /> Users
        </CardTitle>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            className="cursor-pointer"
            onClick={() => setInviteOpen(true)}
            data-testid="users-table-invite"
          >
            <IconMailForward className="h-3.5 w-3.5" /> Invite link
          </Button>
          <Button
            size="sm"
            className="cursor-pointer"
            onClick={() => setCreateOpen(true)}
            data-testid="users-table-create"
          >
            <IconUserPlus className="h-3.5 w-3.5" /> Add user
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
            <Spinner className="size-4" /> Loading users...
          </div>
        )}
        {loaded && users.length > 0 && (
          <UsersTableList
            users={users}
            onToggleRole={(u) => setPending(roleTogglePending(u))}
            onToggleStatus={(u) => setPending(statusTogglePending(u))}
          />
        )}
        {loaded && users.length === 0 && !error && (
          <p className="text-sm text-muted-foreground" data-testid="users-table-empty">
            No users yet.
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
