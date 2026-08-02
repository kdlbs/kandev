import { afterEach, describe, expect, it } from "vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { SpritesInstance, SpritesStatus } from "@/lib/types/http-sprites";
import { SpritesConnectionCard, SpritesInstancesCard } from "./sprites-settings";

type StoreApi = ReturnType<typeof useAppStoreApi>;

function instance(overrides: Partial<SpritesInstance> = {}): SpritesInstance {
  return {
    name: "sprite-1",
    health_status: "running" as SpritesInstance["health_status"],
    created_at: "2026-01-01T00:00:00Z",
    uptime_seconds: 30,
    ...overrides,
  };
}

/**
 * Seeds the sprites slice through the store's own actions rather than
 * `StateProvider initialState`: `buildStateOverrides` does not re-assert
 * `sprites`, so `createSettingsSlice`'s default overwrites a caller-supplied
 * value.
 */
function renderSprites(sprites: { status?: SpritesStatus; instances?: SpritesInstance[] }) {
  let storeApi: StoreApi | null = null;
  function CaptureStore() {
    storeApi = useAppStoreApi();
    return null;
  }

  render(
    <StateProvider>
      <CaptureStore />
      <SpritesConnectionCard />
      <SpritesInstancesCard />
    </StateProvider>,
  );

  act(() => {
    const state = storeApi!.getState();
    if (sprites.status) state.setSpritesStatus(sprites.status);
    state.setSpritesInstances(sprites.instances ?? []);
  });
}

afterEach(cleanup);

describe("SpritesInstancesCard health labels", () => {
  it("renders a localized label for each known health status", () => {
    renderSprites({
      instances: [
        instance({ name: "a", health_status: "running" as SpritesInstance["health_status"] }),
        instance({ name: "b", health_status: "cold" as SpritesInstance["health_status"] }),
        instance({ name: "c", health_status: "unknown" }),
      ],
    });

    expect(screen.getByText("Running")).toBeTruthy();
    expect(screen.getByText("Cold")).toBeTruthy();
    expect(screen.getByText("Unknown")).toBeTruthy();
  });

  it("capitalizes a status that is not in the label map instead of showing it raw", () => {
    // The backend lower-cases whatever Sprites.dev reports and does not constrain
    // the value (`NormalizeSpriteStatus`), so an unmapped status must still
    // render capitalized, as it did before the labels were externalized.
    renderSprites({
      instances: [instance({ health_status: "provisioning" as SpritesInstance["health_status"] })],
    });

    expect(screen.getByText("Provisioning")).toBeTruthy();
    expect(screen.queryByText("provisioning")).toBeNull();
  });
});

describe("SpritesConnectionCard instance count", () => {
  it("uses the singular form for exactly one active sprite", () => {
    renderSprites({ status: { connected: true, token_configured: true, instance_count: 1 } });

    expect(screen.getByText(/1 active sprite\./)).toBeTruthy();
  });

  it("uses the plural form for other counts", () => {
    renderSprites({ status: { connected: true, token_configured: true, instance_count: 3 } });

    expect(screen.getByText(/3 active sprites\./)).toBeTruthy();
  });

  it("reports the disconnected case instead of a count", () => {
    renderSprites({ status: { connected: false, token_configured: true, instance_count: 0 } });

    expect(screen.getByText(/Unable to connect\./)).toBeTruthy();
  });
});
