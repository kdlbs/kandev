import type { DesktopV1Adapter, DesktopUnlisten } from "./adapter";
import { triggerUrlDownload } from "@/lib/utils/file-download";

export function listenForDesktopDownloadReady(adapter: DesktopV1Adapter): Promise<DesktopUnlisten> {
  if (!adapter.isAvailable()) return Promise.resolve(() => undefined);
  return adapter.listen("download-ready", ({ url, fileName }) => {
    triggerUrlDownload(url, fileName);
  });
}
