# DP integration details

## 7. Reproducibility checklist (taste gate)

Before emitting SVG, verify **every** item:

1. `viewBox = "0 0 1200 {viewBox_h}"` where `viewBox_h = max(600, footer_bottom + 84)`.
2. Platform zone at `x=260 y=72 w=696 h=col_h`. Zone label paper-masked across top border at `y=zone_y+3`.
3. Left column at `x=40..200`, right column at `x=1000..1160` — both 160 wide.
4. Source / consumer rows top at `y=92`, stride 88 px.
5. `platform.rows` entries stack inside zone via the §2.1 cursor algorithm; total y-span ≤ `zone_h`.
6. Inside each `kind: row`, node x-centers are evenly spaced across zone width (§2.2).
7. **Exactly 2** focal components (`focal: true`).
8. Every edge originating from a `kind: bar` component uses `style: trigger` (dashed, unlabelled).
9. Every Trino → consumer edge uses `style: primary` (the serve-flow rule).
10. Footer nodes connect only to the zone bottom edge via `auth` style. **No** edge from a footer to a specific component.
11. Custom component colors ≤ 2 (in addition to the focal pair). Connectors never recolored by component `color`.
12. All connectors emitted before any node rect (z-order rule).

---

## 8. Sources and consumers — icon library

Define each icon as `<g id="ico-…">` in `<defs>`, drawn at translate(cx, cy) with `stroke="currentColor"` so it inherits the surrounding text color. Common icons:

- `ico-db` (cylinder) — relational sources
- `ico-sftp` (folder with down arrow) — file drops
- `ico-mail` (envelope) — email pulls
- `ico-mainframe` (server with vents) — legacy systems
- `ico-monitor` — desktop analytics tools
- `ico-chart` (bars) — BI / report tools
- `ico-globe` — public websites
- `ico-api` (brackets `{}`) — gateways and 3rd-party clients
- `ico-key` — identity / IDP
- `ico-monitoring` (chart-line) — observability stack

If you need more icons, browse `assets/icons.html` and define matching `<symbol>` blocks.

---

## 9. Identity, common services → connect to the layer, not to components

**Active Directory** (or Keycloak, IAM, OPA, any cross-cutting identity / policy / secrets store) authenticates *every* component in the platform. Wiring it to one specific tool would understate the trust scope. Connect it instead with a single arrow to the bottom edge of the platform zone, labeled `AUTH` (§3.3).

The same rule applies to any other layer-wide service: centralized logging, secrets vault, observability stack, audit sink, mTLS root. Each goes in the `footer` list, each gets its own row, each gets its own AUTH line up to the zone bottom edge (staggered by index per §3.3). The visual reading is "the platform layer delegates to all of these," which is the architectural truth.

---

## 10. Budget — this type exceeds the default

This is the one type where the default 9-node / 12-arrow budget is intentionally exceeded. A realistic platform integration shows:

- 4–6 source nodes
- 5 platform components
- 4–6 consumer nodes
- 1–3 footer nodes (identity, observability, backup, …)

That's **14–20 nodes**. The complexity is the point — the diagram is making a claim about the *number of distinct integration surfaces*. Compressing them collapses the claim.

When this gets unwieldy:
- Combine clearly-identical source rows (e.g., four MariaDB databases → one `Databases` node with sublabel `4 × MariaDB`)
- Split into two diagrams (one per integration plane: data vs. identity vs. observability)

---

## 11. Anti-patterns

- **Sources or consumers as a single collapsed node** when ≥3 distinct items exist — defeats the whole point of this type. Use Architecture or High-level if you want collapsing.
- **One bus arrow from "sources" to "the platform"** — every wire is labeled with its protocol; this is how integration teams read the diagram.
- **Per-tool color coding** (teal-NiFi, magenta-MinIO, yellow-Jupyter) inside the zone — collapses hierarchy; only the two focal accents earn coral, plus up to 2 custom colors on cross-cutting components (§4 cap).
- **More than 2 focal components** — focal exists to distinguish "platform" from "pile of tools"; >2 erases the signal (same rule as SKILL.md §1).
- **`color` override on a focal component** — ignored. Focal_accent always wins.
- **Footer wired to one specific tool** (e.g., AD → Airflow only) — wrong unless that service truly only protects one tool. The default is the layer-wide connection.
- **Footer or identity inside the zone** — identity gates the layer from outside. Drawing it inside misrepresents the trust model.
- **Phase chevrons across the top** — those belong on `high-level`.
- **Custom-colored connectors** — connectors are topology-driven. Style picks the color; `color` on a component never spreads to its edges.

---

## 12. Examples

- `assets/example-dp-integration.html` — minimal light (1 footer = AD). Gallery default.
- `assets/example-dp-integration-dark.html` — same, dark skin.
- `assets/example-dp-integration-full.html` — same, editorial-card frame.
- Extended color-override and multi-footer variants are not bundled with this installed skill; use the canonical examples above as the starting point for a project-specific variant.
