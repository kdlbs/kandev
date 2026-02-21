"use client";

import { useState, useCallback } from "react";
import { IconTrash, IconPlus, IconPencil, IconStar } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@kandev/ui/card";
import { deleteExecutorProfile, listExecutorProfiles } from "@/lib/api/domains/settings-api";
import { ExecutorProfileDialog } from "@/components/settings/executor-profile-dialog";
import { useAppStore } from "@/components/state-provider";
import type { ExecutorProfile } from "@/lib/types/http";

type ExecutorProfilesCardProps = {
  executorId: string;
  profiles: ExecutorProfile[];
};

export function ExecutorProfilesCard({ executorId, profiles }: ExecutorProfilesCardProps) {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingProfile, setEditingProfile] = useState<ExecutorProfile | null>(null);
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);

  const refreshProfiles = useCallback(async () => {
    try {
      const resp = await listExecutorProfiles(executorId, { cache: "no-store" });
      setExecutors(
        executors.map((e) =>
          e.id === executorId ? { ...e, profiles: resp.profiles } : e,
        ),
      );
    } catch {
      // ignore refresh failure
    }
  }, [executorId, executors, setExecutors]);

  const handleCreate = useCallback(() => {
    setEditingProfile(null);
    setDialogOpen(true);
  }, []);

  const handleEdit = useCallback((profile: ExecutorProfile) => {
    setEditingProfile(profile);
    setDialogOpen(true);
  }, []);

  const handleDelete = useCallback(
    async (profileId: string) => {
      try {
        await deleteExecutorProfile(executorId, profileId);
        await refreshProfiles();
      } catch {
        // ignore delete failure
      }
    },
    [executorId, refreshProfiles],
  );

  return (
    <>
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Profiles</CardTitle>
              <CardDescription>
                Different configurations for this executor. Each profile can have its own setup
                script.
              </CardDescription>
            </div>
            <Button variant="outline" size="sm" onClick={handleCreate} className="cursor-pointer">
              <IconPlus className="h-4 w-4 mr-1" />
              Add
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          {profiles.length === 0 ? (
            <p className="text-sm text-muted-foreground">No profiles configured.</p>
          ) : (
            <div className="space-y-2">
              {profiles.map((profile) => (
                <div
                  key={profile.id}
                  className="flex items-center justify-between rounded-md border px-3 py-2"
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-sm font-medium truncate">{profile.name}</span>
                    {profile.is_default && (
                      <Badge variant="secondary" className="text-xs flex-shrink-0">
                        <IconStar className="h-3 w-3 mr-1" />
                        Default
                      </Badge>
                    )}
                    {profile.setup_script && (
                      <Badge variant="outline" className="text-xs flex-shrink-0">
                        Setup script
                      </Badge>
                    )}
                  </div>
                  <div className="flex items-center gap-1 flex-shrink-0">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleEdit(profile)}
                      className="h-7 w-7 p-0 cursor-pointer"
                    >
                      <IconPencil className="h-3.5 w-3.5" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => handleDelete(profile.id)}
                      className="h-7 w-7 p-0 text-destructive hover:text-destructive cursor-pointer"
                    >
                      <IconTrash className="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
      <ExecutorProfileDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        executorId={executorId}
        profile={editingProfile}
        onSaved={refreshProfiles}
      />
    </>
  );
}
