import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DockerfileBuildCard } from "./docker-sections";
import type { AuthMode } from "@/lib/state/slices/auth/types";

const authMock: { mode: AuthMode | undefined; role: string | undefined } = {
  mode: "enabled",
  role: undefined,
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (
    selector: (state: {
      auth: { mode: AuthMode | undefined; user?: { role?: string } };
    }) => unknown,
  ) =>
    selector({
      auth: {
        mode: authMock.mode,
        user: authMock.role === undefined ? undefined : { role: authMock.role },
      },
    }),
}));

// The Dockerfile editor is a CodeMirror surface with no bearing on the
// permission wiring under test.
vi.mock("./script-editor", () => ({
  ScriptEditor: () => <div data-testid="script-editor" />,
}));

const BUILD_BUTTON = /build image/i;
const ADMIN_ONLY_COPY = "Only administrators can build images.";

function renderCard(role: string | undefined, mode: AuthMode | undefined = "enabled") {
  authMock.mode = mode;
  authMock.role = role;
  render(
    <DockerfileBuildCard
      dockerfile="FROM scratch"
      onDockerfileChange={vi.fn()}
      imageTag="kandev/multi-agent:latest"
      onImageTagChange={vi.fn()}
    />,
  );
}

afterEach(() => {
  cleanup();
  authMock.mode = "enabled";
  authMock.role = undefined;
});

// isAdminIdentity is unit-tested in lib/auth/is-admin.test.ts; these cover the
// hop from that decision into the rendered control, which a leaf-only test
// would miss.
describe("DockerfileBuildCard build permissions", () => {
  it("disables the build button and explains why for a member", () => {
    renderCard("member");

    expect(screen.getByRole("button", { name: BUILD_BUTTON }).hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(ADMIN_ONLY_COPY)).not.toBeNull();
  });

  it("leaves the build button usable for an admin", () => {
    renderCard("admin");

    expect(screen.getByRole("button", { name: BUILD_BUTTON }).hasAttribute("disabled")).toBe(false);
    expect(screen.queryByText(ADMIN_ONLY_COPY)).toBeNull();
  });

  it("leaves the build button usable when authentication is disabled", () => {
    renderCard(undefined, "disabled");

    expect(screen.getByRole("button", { name: BUILD_BUTTON }).hasAttribute("disabled")).toBe(false);
    expect(screen.queryByText(ADMIN_ONLY_COPY)).toBeNull();
  });

  it("disables the build button for a cleared enabled session", () => {
    renderCard(undefined, "enabled");

    expect(screen.getByRole("button", { name: BUILD_BUTTON }).hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(ADMIN_ONLY_COPY)).not.toBeNull();
  });
});
