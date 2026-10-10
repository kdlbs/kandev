package backendapp

import (
	"context"
	taskdto "github.com/kandev/kandev/internal/task/dto"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (b bootStateBuilder) enrichSessionRecoveryBlocks(ctx context.Context, dto *taskdto.TaskSessionDTO, session *taskmodels.TaskSession) {
	if b.p.taskRepo == nil {
		return
	}
	if err := taskdto.EnrichSessionRecoveryBlocks(ctx, dto, session, b.p.taskRepo); err != nil {
		b.logBootError("get session recovery blocks", err)
	}
}
