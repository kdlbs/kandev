import { NotificationsSettings } from '@/components/settings/notifications-settings';
import { StateProvider } from '@/components/state-provider';
import { listNotificationProviders } from '@/lib/api';

export default async function GeneralNotificationsPage() {
  let initialState = {};
  try {
    const response = await listNotificationProviders({ cache: 'no-store' });
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
    <StateProvider initialState={initialState}>
      <NotificationsSettings />
    </StateProvider>
  );
}
