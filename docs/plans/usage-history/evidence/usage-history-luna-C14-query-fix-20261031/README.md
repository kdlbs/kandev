C14 focused PostgreSQL query correction on ee5b9aac81959fa7ee669007cb05f9f6c3a71ae2.

Month-edge probes use bounded PostgreSQL timestamp reads. The scoped applied-build set avoids a repeated full-table scan for each fact. Focused SQLite and PostgreSQL18.6 parity, plan and race checks passed. The full PostgreSQL benchmark was not rerun.

Parent review requested C15 to restrict the applied-build set to selected work rather than all workspace work. This packet preserves C14 evidence; it does not certify the follow-up or waive the full gate. SQLite query paths and benchmark files remain unchanged.
