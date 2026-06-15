import { redirect } from "@/lib/routing/server-navigation";

export default function ChangelogPage() {
  redirect("/settings/system/updates");
}
