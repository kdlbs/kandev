import { redirect } from "@/lib/routing/server-navigation";

export default function GeneralShellPage() {
  redirect("/settings/general/terminal");
}
