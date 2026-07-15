import {
  DESKTOP_NATIVE_EVENTS,
  type DesktopEventName,
  type DesktopEventPayloads,
} from "./protocol";

export type DesktopUnlisten = () => void;

export type DesktopEventTransport = {
  isAvailable: () => boolean;
  listen: (
    eventName: (typeof DESKTOP_NATIVE_EVENTS)[DesktopEventName],
    listener: (payload: unknown) => void,
  ) => Promise<DesktopUnlisten>;
};

export type DesktopV1Adapter = {
  isAvailable: () => boolean;
  listen: <TName extends DesktopEventName>(
    eventName: TName,
    listener: (payload: DesktopEventPayloads[TName]) => void,
  ) => Promise<DesktopUnlisten>;
};

const noOpUnlisten: DesktopUnlisten = () => undefined;

export function createDesktopV1Adapter(transport: DesktopEventTransport): DesktopV1Adapter {
  return {
    isAvailable: transport.isAvailable,
    listen: async (eventName, listener) => {
      if (!transport.isAvailable()) return noOpUnlisten;
      return transport.listen(DESKTOP_NATIVE_EVENTS[eventName], (payload) => {
        listener(payload as DesktopEventPayloads[typeof eventName]);
      });
    },
  };
}
