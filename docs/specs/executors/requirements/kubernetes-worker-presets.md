---
status: active
system: executors
created: 2026-09-09
owners:
  - kandev
---

# Kubernetes Worker Presets Requirements

## Overview

Administrators need a tested starting configuration for Kubernetes task sessions.
The executor system owns workload preparation and its compatibility with session
bootstrap, credentials, and storage. These presets extend the
[Kubernetes foundation](../../kubernetes-executor/spec.md).

## Terminology

- **Preset:** A documented image recipe, PodTemplate, and workspace configuration.
- **Minimal:** The existing Kandev base image with no additional language tools.
  This name does not promise a smaller image than the published base.
- **Prepared image:** An image with its documented toolchain installed before task launch.

## Requirements

### REQ-EXECUTORS-K8S-PRESETS-001: Reproducible starting configurations

**Intent:** Administrators can prepare common workloads without discovering basic
toolchain or volume-permission requirements through failed sessions.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-PRESETS-001.1:** Kandev shall provide minimal, Node/pnpm,
  and Python presets with exact image-build inputs and an explicit Linux architecture.
- **AC-EXECUTORS-K8S-PRESETS-001.2:** Each preset shall identify its installed
  tools, CPU and memory requests, memory limit, runtime user, and workspace mode.
- **AC-EXECUTORS-K8S-PRESETS-001.3:** Each preset shall support a non-root
  session that writes runtime files, credentials, and workspace files without privileged containers or host mounts.
- **AC-EXECUTORS-K8S-PRESETS-001.4:** Before launch, an administrator shall be
  able to use the existing configuration test and save the preset through the existing profile flow.

### REQ-EXECUTORS-K8S-PRESETS-002: Verified workload compatibility

**Intent:** Examples describe behavior that the release's executor can perform.

#### Acceptance criteria

- **AC-EXECUTORS-K8S-PRESETS-002.1:** Each preset shall pass an actual task
  launch, terminal command, Git operation, and its documented toolchain smoke workload.
- **AC-EXECUTORS-K8S-PRESETS-002.2:** The documented managed workspace shall
  preserve a marker across Stop and Resume. Terminal cleanup shall remove only owned resources.
- **AC-EXECUTORS-K8S-PRESETS-002.3:** Preset instructions shall state the exact
  tested architecture and distinguish image-build verification from Kubernetes lifecycle verification.
- **AC-EXECUTORS-K8S-PRESETS-002.4:** Presets shall preserve credential delivery
  and runtime injection. Image layers and templates shall contain no task credentials or private repository content.

## Out of scope

- New registry publication channels, changes to Stable or Nightly releases, and a Helm chart.
- A preset picker, automatic profile creation, or changes to existing profiles.
- Warm pools, image pre-pull controllers, build caches, and startup performance guarantees.
- Provider logins or successful calls to paid agent services as a test prerequisite.
- Guaranteed compatibility with every CSI driver or restricted-cluster policy.
