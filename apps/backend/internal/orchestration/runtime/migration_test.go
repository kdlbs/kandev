package runtime

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMigrationPreservesRegisteredPersonaHistoryOnly(t *testing.T) {
	s, db, task := newRuntime(t)
	ctx := context.Background()
	// Historical tables match the old layout; only registered identities migrate.
	_, err := db.Exec(`CREATE TABLE office_agent_instructions AS SELECT * FROM orchestration_instructions;
 CREATE TABLE office_agent_memory AS SELECT id,agent_profile_id,layer,key,content,metadata,created_at,updated_at FROM orchestration_memory;
 CREATE TABLE office_channels AS SELECT * FROM orchestration_conversations;
 DELETE FROM orchestration_conversations;
 INSERT INTO office_agent_instructions(id,agent_profile_id,filename,content,is_entry,created_at,updated_at) VALUES
 ('instruction','chief','ROLE.md','Keep my role',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP),
 ('unrelated','office-only','ROLE.md','Office role',0,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP);
 INSERT INTO office_agent_memory VALUES('memory','chief','user','preference','Keep my preference','{}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	require.NoError(t, s.Repo.Migrate())
	owner, ws, err := s.Repo.ConversationOwner(ctx, task)
	require.NoError(t, err)
	require.Equal(t, "chief", owner)
	require.Equal(t, "ws", ws)
	file, err := s.Repo.GetInstruction(ctx, "chief", "ROLE.md")
	require.NoError(t, err)
	require.Equal(t, "Keep my role", file.Content)
	rows, err := s.Repo.ListInstructions(ctx, "office-only")
	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, s.Repo.UpsertInstruction(ctx, "chief", "ROLE.md", "Edited independently", false))
	require.NoError(t, s.Repo.Migrate())
	file, err = s.Repo.GetInstruction(ctx, "chief", "ROLE.md")
	require.NoError(t, err)
	require.Equal(t, "Edited independently", file.Content)
	memory, err := s.Repo.ListAgentMemory(ctx, "chief")
	require.NoError(t, err)
	require.Len(t, memory, 1)
	require.Equal(t, "Keep my preference", memory[0].Content)
	require.False(t, memory[0].Confirmed)
	require.Equal(t, "workspace", memory[0].Scope)
	require.Empty(t, memory[0].OwnerUserID)
	_, err = db.Exec(`DELETE FROM orchestration_memory;
 INSERT INTO office_agent_instructions(id,agent_profile_id,filename,content) VALUES ('late','chief','LATE.md','Must stay in Office')`)
	require.NoError(t, err)
	require.NoError(t, s.Repo.Migrate())
	_, err = s.Repo.GetInstruction(ctx, "chief", "LATE.md")
	require.Error(t, err)
	var old string
	require.NoError(t, db.Get(&old, `SELECT content FROM office_agent_instructions WHERE id='instruction'`))
	require.Equal(t, "Keep my role", old)
	memory, err = s.Repo.ListAgentMemory(ctx, "chief")
	require.NoError(t, err)
	require.Empty(t, memory, "deleted memory must not reappear from legacy storage")
}
