package coordinator

import "testing"

func TestImprovementApplyRaces_Postgres(t *testing.T) {
	e := newRaceEnv(t, newMultiConnStorePostgres(t))
	e.assertApplyTwice(t)
	e.assertApplyVersusDiscard(t)
	e.assertApplyVersusPatch(t)
}
