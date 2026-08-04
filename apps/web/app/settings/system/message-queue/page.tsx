import { redirect } from "@/lib/routing/server-navigation";

export default function MessageQueueSettingsPage() {
  redirect("/settings/general/message-queue");
}
