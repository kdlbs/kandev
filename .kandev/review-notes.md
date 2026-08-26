## Fixed during review
- apps/web/components/kanban/adaptive-desktop-kanban.tsx:70 - disabled scroll snap synchronously before the first active pan scroll mutation, so browser snap cannot undo the initial grab-pan movement. Also restored snap on pan cancellation. (commit b0f64d9b5)
- apps/web/components/kanban-card-content.tsx:617 - added an explicit data-kanban-card marker and excluded it from background panning, so task-card descendants cannot arm the board pan gesture. (commit b0f64d9b5)
- apps/web/e2e/tests/kanban/kanban-board.spec.ts:146 - removed a fragile exact-position assertion after mouseleave, because restored CSS snap may settle the board independently of the canceled gesture. (commit b0f64d9b5)
