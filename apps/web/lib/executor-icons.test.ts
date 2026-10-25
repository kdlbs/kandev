import { describe, expect, it } from "vitest";
import {
  IconBox,
  IconCloud,
  IconFolder,
  IconFolders,
  IconPackage,
  IconPackageOff,
} from "@tabler/icons-react";

import {
  EXECUTOR_ICON_MAP,
  getExecutorIcon,
  getExecutorLabel,
  getExecutorStatusIcon,
} from "./executor-icons";

describe("executor icons", () => {
  it("maps both docker variants to the box icon", () => {
    expect(EXECUTOR_ICON_MAP.local_docker).toBe(IconBox);
    expect(EXECUTOR_ICON_MAP.remote_docker).toBe(IconBox);
    expect(getExecutorIcon("local_docker")).toBe(IconBox);
    expect(getExecutorIcon("remote_docker")).toBe(IconBox);
  });

  it("maps sprites to the cloud icon and local executors to folder icons", () => {
    expect(getExecutorIcon("sprites")).toBe(IconCloud);
    expect(getExecutorIcon("local")).toBe(IconFolder);
    expect(getExecutorIcon("worktree")).toBe(IconFolders);
  });

  it("falls back to a folder icon for unknown types", () => {
    expect(getExecutorIcon("does-not-exist")).toBe(IconFolder);
  });

  it("maps Kubernetes normal and error states to the Pod package icons", () => {
    expect(getExecutorIcon("k8s")).toBe(IconPackage);
    expect(getExecutorStatusIcon("k8s", false).Icon).toBe(IconPackage);
    expect(getExecutorStatusIcon("k8s", true).Icon).toBe(IconPackageOff);
  });

  it("returns human-readable labels for known executor types", () => {
    expect(getExecutorLabel("local")).toBe("Local");
    expect(getExecutorLabel("worktree")).toBe("Worktree");
    expect(getExecutorLabel("local_docker")).toBe("Local Docker");
    expect(getExecutorLabel("remote_docker")).toBe("Remote Docker");
    expect(getExecutorLabel("sprites")).toBe("Sprites.dev");
    expect(getExecutorLabel("ssh")).toBe("SSH");
    expect(getExecutorLabel("k8s")).toBe("Kubernetes");
    expect(getExecutorLabel("does-not-exist")).toBe("does-not-exist");
  });
});
