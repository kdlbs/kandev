C15 PostgreSQL retained-report correction on ee5b9aac81959fa7ee669007cb05f9f6c3a71ae2.

C14 bounds UTC month-edge probes. C15 restricts published-build matching to selected retained subjects and distinct work/build keys. SQLite query paths remain unchanged.

With 128 unrelated sessions and 2,050 published work items, the new lookup visits 2-3 applied rows; the old global materialization visits 2,050. Task, session, combined, dated-with-undated and lifetime cases match the fallback and expected totals, including a deleted task. Normal and race checks, vet and lint passed. The parent also ran the filtered deleted-task race case with an explicit selector.

The full PostgreSQL18.6 benchmark remains required. The C12 packet preserves its failed full-scale baseline and the passing full SQLite gate. These focused results do not close the full PostgreSQL gate. The receipt-correction file records one mistyped log digest; the original handoff is preserved.
