package startup

// StepID identifies a registered, named unit of startup work. The registry is
// the only source of valid identifiers (AC-PLATFORM-STARTUP-PROGRESS-005.1).
type StepID string

const (
	StepDatabaseBackup            StepID = "database.backup"
	StepStoresRepositories        StepID = "stores.repositories"
	StepStoresServices            StepID = "stores.services"
	StepPromptSeqBackfill         StepID = "task.prompt_seq.backfill"
	StepMessageTimestampsBackfill StepID = "task.message_timestamps.backfill"
	StepSubagentContextBackfill   StepID = "task.subagent_context.backfill"
	StepSessionsRecovery          StepID = "sessions.recovery"
)

// Measure is the ladder a step's reported progress can occupy: counted (done
// and total both known) > counting (done known, no total) > opaque (neither).
type Measure string

const (
	MeasureOpaque   Measure = "opaque"
	MeasureCounting Measure = "counting"
	MeasureCounted  Measure = "counted"
)

// Unit names the noun a step's count is measured in. It is an enum, not a
// free string, so the localized noun and its plural belong to the rendering
// surface rather than the wire payload.
type Unit string

const (
	UnitRows     Unit = "rows"
	UnitMessages Unit = "messages"
	UnitTurns    Unit = "turns"
	UnitSessions Unit = "sessions"
	UnitBytes    Unit = "bytes"
	UnitStores   Unit = "stores"
)

// Dialect is a database backend a step can apply under.
type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

var bothDialects = []Dialect{DialectSQLite, DialectPostgres}

// StepSpec is a registry entry declaring everything about one step that does
// not vary between activations.
type StepSpec struct {
	ID       StepID
	Phase    Phase
	Measure  Measure
	Unit     Unit
	LabelKey string
	Dialects []Dialect

	// Corpus names the record set a reported count covers. Empty when Measure
	// is opaque.
	Corpus string
	// Outstanding names the predicate selecting work still outstanding.
	// Non-empty only for a step that seeds its done count from durable state.
	Outstanding string
	// Order lists the loop query's existing ordering and tiebreak columns,
	// outermost first. Empty when the loop walks no query-ordered row set.
	Order []string
}

// registry is the complete, exhaustive set of registered steps
// (AC-PLATFORM-STARTUP-PROGRESS-005.3).
var registry = []StepSpec{
	{
		ID:       StepDatabaseBackup,
		Phase:    BackingUpDatabase,
		Measure:  MeasureCounting,
		Unit:     UnitBytes,
		LabelKey: "startup.step.database_backup",
		Dialects: []Dialect{DialectSQLite},
		Corpus:   "staging file size in bytes",
	},
	{
		ID:       StepStoresRepositories,
		Phase:    ApplyingMigrations,
		Measure:  MeasureCounted,
		Unit:     UnitStores,
		LabelKey: "startup.step.stores_repositories",
		Dialects: bothDialects,
		Corpus:   "catalog entries whose descriptor names this sweep",
	},
	{
		ID:       StepStoresServices,
		Phase:    InitializingServices,
		Measure:  MeasureCounted,
		Unit:     UnitStores,
		LabelKey: "startup.step.stores_services",
		Dialects: bothDialects,
		Corpus:   "catalog entries whose descriptor names this sweep",
	},
	{
		ID:       StepPromptSeqBackfill,
		Phase:    ApplyingMigrations,
		Measure:  MeasureOpaque,
		Unit:     UnitRows,
		LabelKey: "startup.step.prompt_seq",
		Dialects: bothDialects,
	},
	{
		ID:       StepMessageTimestampsBackfill,
		Phase:    ApplyingMigrations,
		Measure:  MeasureOpaque,
		Unit:     UnitRows,
		LabelKey: "startup.step.message_timestamps",
		Dialects: bothDialects,
	},
	{
		ID:       StepSubagentContextBackfill,
		Phase:    ApplyingMigrations,
		Measure:  MeasureOpaque,
		Unit:     UnitRows,
		LabelKey: "startup.step.subagent_context",
		Dialects: bothDialects,
	},
	{
		ID:       StepSessionsRecovery,
		Phase:    RecoveringSessions,
		Measure:  MeasureCounted,
		Unit:     UnitSessions,
		LabelKey: "startup.step.session_recovery",
		Dialects: bothDialects,
		Corpus:   "the executors_running record set read before the sweep",
	},
}

// retiredStepIDs is append-only: a removed identifier is listed here forever
// and can never be re-registered (AC-PLATFORM-STARTUP-PROGRESS-005.6).
var retiredStepIDs = map[StepID]struct{}{
	"task.journal.turns.backfill":    {},
	"task.journal.messages.backfill": {},
	"plugins.session_events.mirror":  {},
}

func init() {
	assertNoRetiredRegistrations(registry)
}

// assertNoRetiredRegistrations panics if any spec reuses a retired identifier
// (AC-PLATFORM-STARTUP-PROGRESS-005.6).
func assertNoRetiredRegistrations(specs []StepSpec) {
	for _, spec := range specs {
		if _, retired := retiredStepIDs[spec.ID]; retired {
			panic("startup: registered step reuses a retired identifier: " + string(spec.ID))
		}
	}
}

// Registry returns a copy of the complete step registry.
func Registry() []StepSpec {
	out := make([]StepSpec, len(registry))
	copy(out, registry)
	return out
}

// IsRetired reports whether id was once registered and has since been
// retired, and so must never be reused.
func IsRetired(id StepID) bool {
	_, retired := retiredStepIDs[id]
	return retired
}

func lookupStep(id StepID) (StepSpec, bool) {
	for _, spec := range registry {
		if spec.ID == id {
			return spec, true
		}
	}
	return StepSpec{}, false
}
