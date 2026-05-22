"use client";

import { useRouter } from "next/navigation";
import { Button } from "@kandev/ui/button";
import { Separator } from "@kandev/ui/separator";
import { IconPlus, IconBolt } from "@tabler/icons-react";
import { useAutomations } from "@/hooks/domains/settings/use-automations";
import { AutomationsTable } from "./automations-table";

type AutomationsListPageProps = {
  workspaceId: string;
};

export function AutomationsListPage({ workspaceId }: AutomationsListPageProps) {
  const router = useRouter();
  const { items, loading, enable, disable, trigger, remove } = useAutomations(workspaceId);

  const handleToggleEnabled = async (id: string, enabled: boolean) => {
    if (enabled) {
      await enable(id);
    } else {
      await disable(id);
    }
  };

  const handleTrigger = async (id: string) => {
    await trigger(id);
  };

  const handleDelete = async (id: string) => {
    await remove(id);
  };

  return (
    <div className="space-y-6" data-testid="automations-list-page">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold flex items-center gap-2">
            <IconBolt className="h-5 w-5" />
            Automations
          </h2>
          <p className="text-sm text-muted-foreground">
            Create rules that automatically trigger agent tasks.
          </p>
        </div>
        <Button
          data-testid="new-automation-button"
          className="cursor-pointer"
          onClick={() => router.push(`/settings/workspace/${workspaceId}/automations/new`)}
        >
          <IconPlus className="h-4 w-4 mr-1" />
          New Automation
        </Button>
      </div>
      <Separator />
      {loading && items.length === 0 ? (
        <div className="py-12 text-center text-muted-foreground">Loading automations...</div>
      ) : (
        <AutomationsTable
          automations={items}
          workspaceId={workspaceId}
          onToggleEnabled={handleToggleEnabled}
          onTrigger={handleTrigger}
          onDelete={handleDelete}
        />
      )}
    </div>
  );
}
