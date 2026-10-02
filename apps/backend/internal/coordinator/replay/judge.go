package replay

// judge gives the verdict and, for an unmeasured one, its reason. It is
// called with a compared-case set that is not empty. A blocked guard is never
// an improvement whatever the gain or the case count.
func judge(guard string, heldOutCompared int, candidate, baseline int64) (verdict, reason string) {
	switch {
	case guard == GuardBlocked:
		return VerdictNotAnImprovement, ""
	case heldOutCompared < MinHeldOut:
		return VerdictUnmeasured, ReasonTooFew
	case candidate-baseline >= MinGainThousandths:
		return VerdictImprovement, ""
	}
	return VerdictNotAnImprovement, ""
}
