import { redirect } from "@/lib/routing/server-navigation";
import { SetupWizard } from "./setup-wizard";
import { loadSetupRouteData } from "./setup-route-data";

export default async function SetupPage({
  searchParams,
}: {
  searchParams: Promise<{ mode?: string }> | { mode?: string };
}) {
  const params = await searchParams;
  const data = await loadSetupRouteData(params.mode);

  if (data.kind === "redirect") {
    redirect(data.href);
  }

  return <SetupWizard {...data.props} />;
}
