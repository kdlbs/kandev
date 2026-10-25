# High-Level details

## 6. Dark mode

When `dark: true`, swap these tokens:

| Token | Light | Dark |
|---|---|---|
| Page paper | `#f5f5f5` | `#1c1f2e` |
| Ink | `#2d3142` | `#f5f5f5` |
| Muted text | `#4f5d75` | `rgba(245,245,245,0.65)` |
| Chevron dark fill | `#2d3142` | `#3d4460` |
| Chevron light fill | `#3d4460` | `#4a5270` |
| Chevron label | `#f5f5f5` | `#f5f5f5` (unchanged) |
| Dashed border | `rgba(45,49,66,0.20)` | `rgba(245,245,245,0.22)` |
| Cluster border | `rgba(45,49,66,0.18)` | `rgba(245,245,245,0.18)` |
| Node fill | white | `rgba(245,245,245,0.06)` |
| Node stroke | `rgba(45,49,66,0.25)` | `rgba(245,245,245,0.20)` |
| Focal fill | `rgba(235,108,54,0.08)` | `rgba(240,138,89,0.12)` |
| Focal stroke | `#eb6c36` | `#f08a59` |
| Accent connector | `#eb6c36` | `#f08a59` |
| Dot pattern | `rgba(45,49,66,0.10)` | `rgba(245,245,245,0.10)` |

---

## 7. Reproducibility checklist (the taste gate)

Before emitting SVG, verify **every** item. If any fails, fix it — don't ship.

1. Every cluster `node.cx` equals its chevron's `cx` (§2.2 + §2.7). This is what makes the chevron banner a real legend.
2. Every chevron `width` is a multiple of 4 and ≥ 120.
3. The reserved right strip (28 px) exists **iff** any vertical chevron is declared. If yes, `effective_w = 964`; if no, `effective_w = 1000`.
4. Exactly **one** `focal` node. If `focal` is unset in inputs, default to the first `kind: node` under chevron "Storage".
5. Every edge whose endpoint is the focal node uses `style: primary` (accent stroke + `arrow-accent` marker).
6. Every edge originating from a `kind: bar` component uses `style: trigger` (dashed + `arrow-sm`).
7. The cross-cutting bar (if any) emits **no** edges.
8. No node has > 3 outgoing edges, or if it does, it is the declared `focal` / hub.
9. All `<path>` and `<line>` connectors are emitted **before** any node `<rect>` (z-order).
10. Each vertical chevron pairs **1:1** with exactly one `bar` or `cross-cutting` component (§5 pairing rule). `len(verticals) == len(bars) + len(crosscuts)`.
11. `viewBox_h = max(540, strip_y_bot + 112)` — grow the canvas when multiple crosscuts are declared so the legend still fits.
12. Custom component colors (§3.4) apply only to container + icon + name; connectors stay topology-driven. Cap at 2 custom-colored components in addition to the focal.
13. The diagram passes SKILL.md §9 (4-px grid; ≤ 2 accent elements; mono only for technical content; hairlines; no shadows; no `rounded-2xl`).

---

## 8. Anti-patterns

- Chevron banner omitted — it's the key that maps visual columns to functional phases.
- Node x-center off-chevron (§7 #1) — breaks the "banner-as-legend" contract.
- Vertical chevron drawn on the cluster (overlay) instead of in a reserved right strip.
- More than one focal node — MinIO/S3 (or whichever storage hub) is *the* focal point.
- External zone with solid border — dashed border is the signal that these components are outside the cluster.
- Identity bar inside the cluster boundary — it applies to all components and must span the full canvas width.
- Vertical chevron without a paired bar/cross-cutting component — see §5 pairing rule.
- Bar-component edges drawn solid — orchestration triggers must be dashed.
- Source fanning out to >3 components without a hub.

---

## 9. Examples

- `assets/example-high-level.html` — horizontal-only, 5 phases, light skin.
- `assets/example-high-level-dark.html` — same, dark skin.
- `assets/example-high-level-full.html` — same, editorial-card frame.
- `assets/example-high-level-vertical.html` — adds vertical Orchestration + Security chevrons, Airflow bar, Keycloak cross-cutting. **Reference render of the full parametric pattern.**
- `assets/example-high-level-vertical-dark.html` — vertical pattern, dark skin.
- `assets/example-high-level-vertical-full.html` — vertical pattern, editorial-card frame.
- `assets/example-datalake.html` — unclustered five-phase data stack (Sources → Ingest → Data Lake → Query → Consume), zone-based flow without a container-orchestrator boundary. The MinIO lake is the focal node; vertical concerns are flattened into the horizontal chevron banner. Light skin.
- `assets/example-datalake-dark.html` — same, dark skin.
- `assets/example-datalake-full.html` — same, editorial-card frame.
