import { redirect } from "next/navigation";

export default function ChangelogPage() {
  redirect("/settings/system/updates");
}
