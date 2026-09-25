# Legacy conversation journal fixture

`conversation-journal-88c6c0fe.sql` contains the legacy SQLite journal tables,
indexes, and seven payload-copy triggers from commit
`88c6c0fe0a6ae5d25332070d603b99f7d9241645`.

The SQL was exported from `conversationJournalTables` and
`initSQLiteConversationJournalTriggers` in the historical
`conversation_journal.go`. The historical Go helpers expanded the system-content,
metadata, and timestamp expressions. The export excludes the old post-install
sanitization UPDATE statements. The load fixture contains synthetic text only.

This is test data, not an installation or migration script. Production startup
must not execute it. `TestConversationLargeLegacyUpgrade` loads it only when
`KANDEV_LARGE_UPGRADE_DIR` names an empty disposable directory. The test retains
that directory for an isolated backend startup check; remove it after collecting
results. Allow at least 10 GiB free for the database, WAL, backup, and headroom.
