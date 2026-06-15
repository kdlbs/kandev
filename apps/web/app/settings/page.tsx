import { redirect } from "@/lib/routing/server-navigation";

export default function SettingsPage() {
  redirect("/settings/general");
}
