import { redirect } from "@/lib/routing/server-navigation";

export default function GeneralChatInputPage() {
  redirect("/settings/general/keyboard-shortcuts");
}
