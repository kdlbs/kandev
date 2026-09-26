"use client";

import type { Message } from "@/lib/types/http";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { dummyMessages } from "./demo-messages-data";
import { useTranslation } from "react-i18next";

export default function MessagesDemoPage() {
  const { t } = useTranslation("common");
  const permissionsByToolCallId = new Map<string, Message>();
  const permMsg = dummyMessages.find((m) => m.id === "permission-1");
  if (permMsg) {
    permissionsByToolCallId.set("tool-4", permMsg);
  }

  return (
    <div className="min-h-screen bg-background p-8">
      <div className="max-w-4xl mx-auto">
        <div className="mb-8">
          <h1 className="text-2xl font-bold mb-2">{t("common:demoMessageTypes")}</h1>
          <p className="text-muted-foreground">{t("common:demoMessageTypesDescription")}</p>
        </div>

        <div className="space-y-2 border rounded-lg p-5 bg-card">
          {dummyMessages.map((message) => (
            <MessageRenderer
              key={message.id}
              comment={message}
              isTaskDescription={message.id === "task-description"}
              taskId="demo-task"
              permissionsByToolCallId={permissionsByToolCallId}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
