"use client";

import { useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { IconTrash, IconPlus, IconChevronRight } from "@tabler/icons-react";
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
  const router = useRouter();
  const [dialogOpen, setDialogOpen] = useState(false);
  const executors = useAppStore((state) => state.executors.items);
  const setExecutors = useAppStore((state) => state.setExecutors);

  const refreshProfiles = useCallback(async () => {
    try {
      const resp = await listExecutorProfiles(executorId, { cache: "no-store" });
      setExecutors(
        executors.map((e) => (e.id === executorId ? { ...e, profiles: resp.profiles } : e)),
      );
    } catch {
      // ignore refresh failure
    }
  }, [executorId, executors, setExecutors]);

  const handleCreate = useCallback(() => {
    setDialogOpen(true);
  }, []);

  const handleProfileCreated = useCallback(
    (profile: ExecutorProfile) => {
      refreshProfiles();
      router.push(`/settings/executor/${executorId}/profile/${profile.id}`);
    },
    [executorId, refreshProfiles, router],
  );

  const handleDelete = useCallback(
    async (e: React.MouseEvent, profileId: string) => {
      e.stopPropagation();
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
                Different configurations for this executor. Each profile can have its own prepare
                script, environment variables, and settings.
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
                  className="flex items-center justify-between rounded-md border px-3 py-2 hover:bg-muted/50 cursor-pointer transition-colors"
                  onClick={() =>
                    router.push(`/settings/executor/${executorId}/profile/${profile.id}`)
                  }
                >
                  <div className="flex items-center gap-2 min-w-0">
                    <span className="text-sm font-medium truncate">{profile.name}</span>
                    {profile.prepare_script && (
                      <Badge variant="outline" className="text-xs flex-shrink-0">
                        Prepare script
                      </Badge>
                    )}
                  </div>
                  <div className="flex items-center gap-1 flex-shrink-0">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={(e) => handleDelete(e, profile.id)}
                      className="h-7 w-7 p-0 text-destructive hover:text-destructive cursor-pointer"
                    >
                      <IconTrash className="h-3.5 w-3.5" />
                    </Button>
                    <IconChevronRight className="h-4 w-4 text-muted-foreground" />
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
        onSaved={handleProfileCreated}
      />
    </>
  );
}
