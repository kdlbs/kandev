import { redirect } from "@/lib/routing/server-navigation";

export default function GeneralChangesPanelPage() {
  redirect("/settings/general/appearance");
}
