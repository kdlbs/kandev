import { StateProvider } from "@/components/state-provider";
import { SystemPageShell } from "@/components/settings/system/system-page-shell";
import { LicensesList } from "@/components/settings/system/licenses-list";
import licenses from "@/generated/licenses.json";
import type { LicenseEntry } from "@/lib/types/system";

export default function SystemLicensesPage() {
  const entries = licenses as LicenseEntry[];

  return (
    <StateProvider initialState={{}}>
      <SystemPageShell
        title="Licenses"
        description="Open-source licenses for every npm and Go dependency shipped with kandev."
      >
        <LicensesList entries={entries} />
      </SystemPageShell>
    </StateProvider>
  );
}
