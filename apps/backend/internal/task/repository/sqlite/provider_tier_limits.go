package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/jmoiron/sqlx"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	"time"
)

func (r *Repository) LoadAccountTierConfig(ctx context.Context, key string) (agentusage.AccountTierConfig, agentusage.AccountTierConfig, error) {
	var owner, learned agentusage.AccountTierConfig
	var rawOwner, rawLearned string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT config_json, learned_json FROM provider_account_limits WHERE account_key=?`), key).Scan(&rawOwner, &rawLearned)
	if err == sql.ErrNoRows {
		return owner, learned, nil
	}
	if err != nil {
		return owner, learned, err
	}
	if err = json.Unmarshal([]byte(rawOwner), &owner); err != nil {
		return owner, learned, err
	}
	err = json.Unmarshal([]byte(rawLearned), &learned)
	return owner, learned, err
}

func (r *Repository) SaveAccountTierOverrides(ctx context.Context, key string, config agentusage.AccountTierConfig) error {
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO provider_account_limits(account_key,config_json,learned_json,updated_at) VALUES (?,?,'{}',?) ON CONFLICT(account_key) DO UPDATE SET config_json=excluded.config_json,updated_at=excluded.updated_at`), key, string(encoded), time.Now().UTC())
	return err
}

func (r *Repository) SaveAccountTierLearned(ctx context.Context, key string, config agentusage.AccountTierConfig) error {
	encoded, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO provider_account_limits(account_key,config_json,learned_json,updated_at) VALUES (?,'{}',?,?) ON CONFLICT(account_key) DO UPDATE SET learned_json=excluded.learned_json,updated_at=excluded.updated_at`), key, string(encoded), time.Now().UTC())
	return err
}

// CountLimitActiveTurns includes claimed starts and executing sessions once.
// A retained idle session does not hold a turn slot. The current session is
// excluded when it starts its next turn or changes its candidate.
func (r *Repository) CountLimitActiveTurns(ctx context.Context, ids []string, exclude string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	query, args, err := sqlx.In(`SELECT COUNT(DISTINCT s.id) FROM task_sessions s LEFT JOIN dynamic_route_states d ON d.session_id=s.id
		WHERE s.id<>? AND ((s.state IN ('STARTING','RUNNING') AND COALESCE(NULLIF(s.execution_profile_id,''),s.agent_profile_id) IN (?))
		OR (d.state='starting' AND d.execution_profile_id IN (?)))`, exclude, ids, ids)
	if err != nil {
		return 0, err
	}
	var count int64
	err = r.ro.QueryRowContext(ctx, r.ro.Rebind(query), args...).Scan(&count)
	return count, err
}
