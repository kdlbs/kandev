#ifndef KANDEV_TESTUTIL_SQLITE_MEMORY_H
#define KANDEV_TESTUTIL_SQLITE_MEMORY_H
extern long long sqlite3_memory_used(void);
extern long long sqlite3_memory_highwater(int resetFlag);
extern const char *sqlite3_libversion(void);
#endif
