"use client";

import Link from "next/link";
import { IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { EnvironmentCard } from "@/components/settings/environment-card";
import { useAppStore } from "@/components/state-provider";
import type { Environment } from "@/lib/types/http";

export default function EnvironmentsSettingsPage() {
  const environments = useAppStore((state) => state.environments.items);

  return (
    <div className="space-y-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold">Environments</h2>
          <p className="text-sm text-muted-foreground mt-1">
            Configure runtime environments for agent sessions.
          </p>
        </div>
        <Button asChild size="sm">
          <Link href="/settings/environment/new">
            <IconPlus className="h-4 w-4 mr-2" />
            Create Custom Environment
          </Link>
        </Button>
      </div>

      <Separator />

      <div className="grid gap-3">
        {environments.map((env: Environment) => (
          <EnvironmentCard key={env.id} environment={env} />
        ))}
      </div>
    </div>
  );
}
