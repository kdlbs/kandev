// @covers AC-TASKS-PLAN-COMMENTS-001.8
package planws

import (
	"testing"

	"github.com/kandev/kandev/internal/task/plancomments"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestPlanCommentLimitsAreValidationErrors(t *testing.T) {
	for _, sizeErr := range []error{
		plancomments.ErrBodyTooLarge, plancomments.ErrSelectionTooLarge, plancomments.ErrCollectionTooLarge,
	} {
		response, err := PlanCommentError(request(), sizeErr, nil)
		if payload := decode(t, response, err); payload.Code != ws.ErrorCodeValidation {
			t.Errorf("%v mapped to %s", sizeErr, payload.Code)
		}
	}
}
