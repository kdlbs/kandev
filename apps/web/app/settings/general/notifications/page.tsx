import { NotificationsSettings } from "@/components/settings/notifications-settings";
import { StateHydrator } from "@/components/state-hydrator";
import { listNotificationProviders } from "@/lib/api";

export default async function GeneralNotificationsPage() {
  let initialState = {};
  try {
    const response = await listNotificationProviders({ cache: "no-store" });
    initialState = {
      notificationProviders: {
        items: response.providers ?? [],
        events: response.events ?? [],
        appriseAvailable: response.apprise_available ?? false,
        loaded: true,
        loading: false,
      },
    };
  } catch {
    initialState = {};
  }

  return (
    <>
      <StateHydrator initialState={initialState} />
      <NotificationsSettings />
    </>
  );
}
