import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import {
  SettingsSaveProvider,
  useSettingsSaveContributor,
} from "@/components/settings/settings-save-provider";
import { createAppStore } from "@/lib/state/store";
import { buildHostApi } from "./host-api";
import type { PluginHostApi } from "./types";

function PluginContributor({ host, save }: { host: PluginHostApi; save: () => void }) {
  host.useSettingsSaveContributor({
    id: "shared-settings-id",
    revision: 1,
    isDirty: true,
    save,
    discard: () => {},
  });
  return null;
}

function NativeContributor({ save }: { save: () => void }) {
  useSettingsSaveContributor({
    id: "shared-settings-id",
    revision: 1,
    isDirty: true,
    save,
    discard: () => {},
  });
  return null;
}

describe("plugin settings save ownership", () => {
  it("keeps a plugin contributor separate from a host contributor with the same local id", async () => {
    const pluginSave = vi.fn();
    const nativeSave = vi.fn();
    const host = buildHostApi("plugin-settings", createAppStore());

    render(
      <SettingsSaveProvider>
        <PluginContributor host={host} save={pluginSave} />
        <NativeContributor save={nativeSave} />
      </SettingsSaveProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => expect(pluginSave).toHaveBeenCalledWith(1));
    await waitFor(() => expect(nativeSave).toHaveBeenCalledWith(1));
  });
});
