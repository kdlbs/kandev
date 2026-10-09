"use client";

import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import type {
  PluginHostV2ManagedAgentInputReceipt,
  PluginManagedConversationSnapshot,
} from "@kandev/plugin-sdk";
import { WorkspaceAgentChatInteractions } from "./workspace-agent-chat-interactions";
import type { ClarificationAnswer } from "./workspace-agent-chat-interactions";

export function ConversationControls({
  status,
  inputs,
  touchTargets,
  busy,
  cancelInput,
  permission,
  clarification,
}: {
  status: PluginManagedConversationSnapshot;
  inputs: readonly PluginHostV2ManagedAgentInputReceipt[];
  touchTargets: boolean;
  busy: boolean;
  cancelInput(receipt: PluginHostV2ManagedAgentInputReceipt): void;
  permission(id: string, version: string, optionId?: string): void;
  clarification(id: string, version: string, answers: ClarificationAnswer[]): void;
}) {
  const { t } = useTranslation("plugins");
  const active = inputs.filter((input) => input.state === "accepted" || input.state === "running");
  const interactions = status.pendingInteractions;
  if (active.length === 0 && interactions.length === 0) return null;
  return (
    <div
      className="max-h-52 shrink-0 space-y-2 overflow-y-auto border-b p-3"
      data-testid="managed-chat-controls"
    >
      {active.map((receipt) => (
        <div
          key={receipt.hostInputId}
          className={`flex ${touchTargets ? "min-h-11" : "min-h-7"} min-w-0 items-center gap-2 text-sm`}
          data-testid={`managed-chat-input-${receipt.hostInputId}`}
        >
          <span className="min-w-0 flex-1">
            <span className="block truncate">
              {t("managedChatInputState", { state: receipt.state, sequence: receipt.sequence })}
            </span>
            {receipt.payload ? (
              <span className="block truncate text-muted-foreground">{receipt.payload}</span>
            ) : null}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className={touchTargets ? "min-h-11" : "min-h-7"}
            disabled={busy || status.readOnly || receipt.state !== "accepted"}
            onClick={() => cancelInput(receipt)}
          >
            {t("managedChatCancelInput")}
          </Button>
        </div>
      ))}
      {interactions.map((interaction) => (
        <WorkspaceAgentChatInteractions
          key={interaction.id}
          interaction={interaction}
          touchTargets={touchTargets}
          busy={
            busy ||
            Boolean(status.readOnly) ||
            status.state === "revoked" ||
            status.state === "unsupported"
          }
          onPermission={(optionId) =>
            permission(interaction.id, interaction.expectedResourceVersion, optionId)
          }
          onClarification={(answers) =>
            clarification(interaction.id, interaction.expectedResourceVersion, answers)
          }
        />
      ))}
    </div>
  );
}

export function MobileChatPanels({
  tab,
  onTab,
  tasks,
  outcomes,
  transcript,
  controls,
  composer,
}: {
  tab: string;
  onTab(value: string): void;
  tasks: React.ReactNode;
  outcomes: React.ReactNode;
  transcript: React.ReactNode;
  controls: React.ReactNode;
  composer: React.ReactNode;
}) {
  const { t } = useTranslation("plugins");
  return (
    <Tabs
      value={tab}
      onValueChange={onTab}
      className="flex min-h-0 flex-1 flex-col gap-0"
      data-testid="managed-chat-mobile-tabs"
    >
      <TabsList className="grid h-auto min-h-11 w-full shrink-0 grid-cols-3 rounded-none border-b bg-background">
        <TabsTrigger className="min-h-11" value="chat">
          {t("managedChatChatTab")}
        </TabsTrigger>
        <TabsTrigger className="min-h-11" value="tasks">
          {t("managedChatTasksTab")}
        </TabsTrigger>
        <TabsTrigger className="min-h-11" value="outcomes">
          {t("managedChatOutcomesTab")}
        </TabsTrigger>
      </TabsList>
      <TabsContent value="chat" className="m-0 flex min-h-0 flex-1 flex-col overflow-hidden">
        {controls}
        {transcript}
        {composer}
      </TabsContent>
      <TabsContent value="tasks" className="m-0 min-h-0 flex-1 overflow-y-auto p-3">
        {tasks}
      </TabsContent>
      <TabsContent value="outcomes" className="m-0 min-h-0 flex-1 overflow-y-auto p-3">
        {outcomes}
      </TabsContent>
    </Tabs>
  );
}

export function ManagedChatQueueCount({ count }: { count: number }) {
  const { t } = useTranslation("plugins");
  if (!count) return null;
  return (
    <Badge variant="secondary" data-testid="managed-chat-queued-count">
      {t("managedChatQueuedCount", { count })}
    </Badge>
  );
}

export function ManagedChatEmptyPanel({ kind }: { kind: "tasks" | "outcomes" }) {
  const { t } = useTranslation("plugins");
  return (
    <p className="text-sm text-muted-foreground">
      {t(kind === "tasks" ? "managedChatNoTasks" : "managedChatNoOutcomes")}
    </p>
  );
}
