package canvas

import "expvar"

var canvasDataScopeTransitions = expvar.NewMap("canvas_data_scope_transition_total")

type canvasScopeTransition string
type canvasScopeTransitionResult string

const (
	canvasScopeFirstPublication canvasScopeTransition = "first_publication"
	canvasScopeReviewedUpgrade  canvasScopeTransition = "reviewed_upgrade"
	canvasScopePromotion        canvasScopeTransition = "promotion"
)

const (
	canvasScopeEnabled   canvasScopeTransitionResult = "data_scope_enabled"
	canvasScopeExpanded  canvasScopeTransitionResult = "data_scope_expanded"
	canvasScopePreserved canvasScopeTransitionResult = "data_scope_preserved"
)

func recordCanvasScopeTransition(transition canvasScopeTransition, result canvasScopeTransitionResult) {
	canvasDataScopeTransitions.Add("transition="+string(transition)+";result="+string(result), 1)
}
