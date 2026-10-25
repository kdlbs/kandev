# ADR-2026-09-05-trusted-browser-html-preview: Treat HTML Preview as Trusted Workspace Code

**Status:** accepted
**Date:** 2026-09-05
**Area:** frontend, backend, agentctl, security
**Amended:** 2026-09-06 to make in-editor preview the primary desktop surface

## Context

Kandev needs a one-click preview for `.html` and `.htm` files. The desired
product behavior is normal browser fidelity: relative assets, native DOM and
browser APIs, links, forms, media, modules, and network behavior. The preview
must also use the current unsaved editor buffer.

The first design treated every workspace document as hostile. It used QuickJS
in a Web Worker, a virtual DOM, a controlled renderer, resource filtering, and
execution budgets. That design reduced ambient authority but did not provide a
normal browser. It also duplicated a large and permanently incomplete browser
compatibility surface.

Kandev already routes executor-local HTTP ports through a session-authorized
proxy and renders trusted workspace pages in iframes that permit scripts and
same-origin behavior. Reusing this path gives static HTML the same fidelity and
executor coverage as an explicitly started development server.

[ADR-2026-07-24-operator-owned-agent-launcher-settings](2026-07-24-operator-owned-agent-launcher-settings.md)
requires workspace-controlled web content to use a separate origin when it is
treated as untrusted. The product owner explicitly selected the existing
trusted native-browser model for HTML preview instead of a dedicated preview
origin.

## Decision

Treat native HTML preview as execution of trusted workspace code. Use the
existing Browser-panel iframe sandbox, origin behavior, and session port proxy.
Do not claim that preview isolates users from hostile HTML or JavaScript.

Agentctl will lazily start an ephemeral loopback static server for each selected
workspace or repository root. The current editor buffer is stored as a bounded
in-memory overlay for its workspace path. The server reads other relative assets
from that root. The backend authorizes the task session and forwards publish
requests to agentctl. Desktop and mobile replace the active file viewer body
with the returned session proxy URL. Desktop can open the same URL in the
Browser panel only through an explicit secondary action.

This decision narrows the untrusted-content rule in the 2026-07-24 ADR: native
HTML preview is an explicit trusted-content exception. That ADR continues to
govern operator-owned launcher settings and any future feature that promises to
render workspace-controlled content as untrusted. If Kandev later offers an
untrusted HTML preview mode, that mode must use a dedicated origin or a stronger
independently reviewed boundary.

The static server must contain filesystem reads to the selected root and listen
only on loopback. It must use the existing authenticated proxy and stop with
agentctl. Logs must not contain source content. These requirements control the
host and routing. They are not a browser-script sandbox.

## Consequences

- HTML, CSS, JavaScript, relative assets, and browser APIs behave like they do
  in the existing Browser-panel iframe.
- Unsaved HTML can be previewed without writing it to the workspace.
- Local, Docker, SSH, and Kubernetes task sessions reuse one routing mechanism.
- Users must trust previewed documents. The UI and public documentation must
  state that previewing executes workspace code with Browser-panel authority.
- Same-origin deployments can expose Kandev/browser authority to previewed code.
  This risk is accepted for this feature and must not be described as isolated.
- Kandev owns a small static-server and in-memory-overlay lifecycle instead of
  an ECMAScript engine, virtual DOM, browser emulation layer, and safe renderer.
- Applications that need builds, backend routes, HMR, or framework-specific
  behavior continue to use their configured development server.

## Alternatives considered

### QuickJS with a virtual DOM and controlled renderer

Rejected because it is a browser reimplementation with limited compatibility.
Its complexity is not justified when browser fidelity is the product goal.

### Dedicated preview origin

Rejected for the current feature because it adds deployment, DNS, cookie,
desktop, and remote-executor routing work. It remains the preferred direction
if Kandev needs to claim hostile-content isolation later.

### Scriptless `srcDoc` iframe

Rejected because it does not support the required scripts, relative assets, or
normal browser APIs.

### Require users to configure a development-server command

Rejected as the only workflow because a static HTML document needs one action,
not project-specific command configuration. The explicit development server
remains available for non-static applications.

### Open a Browser panel as the primary desktop result

Rejected because a file-preview action should preserve the user's active file
context and match the existing source/preview mental model. The Browser panel
remains available as an explicit secondary action for broader browsing and
inspection workflows.
