package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/db/dialect"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

const sidebarTaskColorScratchTable = "temp.kandev_sidebar_task_colors"

func sidebarColorSettings(prefs taskmodels.SidebarTaskViewPreferences) taskmodels.SidebarTaskColorSettings {
	if prefs.ColorSettings == nil {
		return taskmodels.SidebarTaskColorSettings{ManualColors: map[string]*string{}, Automation: json.RawMessage(`{"enabled":false,"rules":[]}`)}
	}
	settings := *prefs.ColorSettings
	if settings.ManualColors == nil {
		settings.ManualColors = map[string]*string{}
	}
	if len(settings.Automation) == 0 {
		settings.Automation = json.RawMessage(`{"enabled":false,"rules":[]}`)
	}
	return settings
}

func validateSidebarColorSettings(prefs taskmodels.SidebarTaskViewPreferences) error {
	if prefs.ColorSettings == nil {
		return nil
	}
	settings := sidebarColorSettings(prefs)
	if err := usermodels.ValidateSidebarTaskColors(settings.ManualColors); err != nil {
		return fmt.Errorf("invalid sidebar task colors: %w", err)
	}
	var automation usermodels.SidebarTaskColorAutomation
	if err := json.Unmarshal(settings.Automation, &automation); err != nil {
		return fmt.Errorf("invalid sidebar task color automation: %w", err)
	}
	if err := usermodels.ValidateSidebarTaskColorAutomation(automation); err != nil {
		return fmt.Errorf("invalid sidebar task color automation: %w", err)
	}
	return nil
}

func sidebarColorManualCTE(
	driver string,
	prefs taskmodels.SidebarTaskViewPreferences,
	needed bool,
) (string, []any, error) {
	if !needed {
		return "", nil, nil
	}
	if !dialect.IsPostgres(driver) {
		return "", nil, nil
	}
	encoded, err := json.Marshal(sidebarColorSettings(prefs).ManualColors)
	if err != nil {
		return "", nil, fmt.Errorf("encode sidebar manual colors: %w", err)
	}
	return "sidebar_color_manual_settings AS (SELECT ?::jsonb AS colors), ", []any{string(encoded)}, nil
}

func sidebarManualColorExpression(driver string) string {
	if dialect.IsPostgres(driver) {
		return `(SELECT settings.colors->>t.id FROM sidebar_color_manual_settings settings)`
	}
	return `(SELECT colors.color FROM ` + sidebarTaskColorScratchTable + ` colors WHERE colors.task_id = t.id)`
}

func sidebarEffectiveColorProjection(
	driver string,
	prefs taskmodels.SidebarTaskViewPreferences,
) (string, []any, error) {
	manual := sidebarManualColorExpression(driver)
	settings := sidebarColorSettings(prefs)
	var automation usermodels.SidebarTaskColorAutomation
	if err := json.Unmarshal(settings.Automation, &automation); err != nil {
		return "", nil, fmt.Errorf("decode sidebar task color automation: %w", err)
	}
	if !automation.Enabled {
		return manual, nil, nil
	}

	branches := make([]string, 0, len(automation.Rules))
	args := make([]any, 0, len(automation.Rules)*3)
	for _, rule := range automation.Rules {
		if !rule.Enabled || rule.Condition.Value == nil {
			continue
		}
		condition, conditionArgs, err := sidebarColorRuleCondition(driver, rule.Condition)
		if err != nil {
			return "", nil, err
		}
		output := ""
		if rule.Output.Kind == usermodels.SidebarTaskColorOutputWorkflowStep {
			output = sidebarWorkflowColorToken(driver, "ws.color")
			branches = append(branches, "WHEN ("+condition+") THEN "+output)
			args = append(args, conditionArgs...)
			continue
		}
		branches = append(branches, "WHEN ("+condition+") THEN ?")
		args = append(args, conditionArgs...)
		args = append(args, rule.Output.Color)
	}
	if len(branches) == 0 {
		return manual, nil, nil
	}
	return "CASE " + strings.Join(branches, " ") + " ELSE " + manual + " END", args, nil
}

func sidebarColorRuleCondition(driver string, condition usermodels.SidebarTaskColorCondition) (string, []any, error) {
	value, ok := condition.Value.(string)
	switch condition.Dimension {
	case usermodels.SidebarTaskColorDimensionWorkflowStep,
		usermodels.SidebarTaskColorDimensionWorkflow:
		target, ok := condition.Value.(map[string]any)
		if !ok {
			return "", nil, errors.New("sidebar color scope target is malformed")
		}
		workspaceID, workspaceOK := target["workspace_id"].(string)
		field, key := "workflow_id", "workflow_id"
		if condition.Dimension == usermodels.SidebarTaskColorDimensionWorkflowStep {
			field, key = "workflow_step_id", "step_id"
		}
		entityID, entityOK := target[key].(string)
		if !workspaceOK || !entityOK {
			return "", nil, errors.New("sidebar color scope target is malformed")
		}
		return "t.workspace_id = ? AND t." + field + " = ?", []any{workspaceID, entityID}, nil
	case usermodels.SidebarTaskColorDimensionExecutorProfile:
		primaryProfile := `(SELECT NULLIF(primary_session.executor_profile_id, '')
			FROM task_sessions primary_session
			WHERE primary_session.task_id = t.id AND primary_session.is_primary = 1
			LIMIT 1)`
		profile := "COALESCE(" + primaryProfile + ", " + dialect.JSONExtract(driver, "t.metadata", "executor_profile_id") + ")"
		return sidebarStringColorCondition(value, ok, "sidebar executor color target is malformed", profile+" = ?")
	case usermodels.SidebarTaskColorDimensionTaskState:
		return sidebarStringColorCondition(value, ok, "sidebar task-state color target is malformed", "t.state = ?")
	case usermodels.SidebarTaskColorDimensionPriority:
		return sidebarStringColorCondition(value, ok, "sidebar priority color target is malformed", "COALESCE(t.priority, '') = ?")
	case usermodels.SidebarTaskColorDimensionOrigin:
		return sidebarStringColorCondition(value, ok, "sidebar origin color target is malformed", "COALESCE(NULLIF(t.origin, ''), 'kanban') = ?")
	case usermodels.SidebarTaskColorDimensionRepository:
		return sidebarRepositoryColorCondition(driver, condition.Value)
	default:
		return "", nil, fmt.Errorf("unsupported sidebar color dimension %q", condition.Dimension)
	}
}

func sidebarStringColorCondition(value string, valid bool, message, expression string) (string, []any, error) {
	if !valid {
		return "", nil, errors.New(message)
	}
	return expression, []any{value}, nil
}

func sidebarRepositoryColorCondition(driver string, value any) (string, []any, error) {
	target, ok := value.(map[string]any)
	if !ok {
		return "", nil, errors.New("sidebar repository color target is malformed")
	}
	kind, _ := target["kind"].(string)
	base := `EXISTS (SELECT 1 FROM task_repositories tr JOIN repositories r ON r.id = tr.repository_id
		WHERE tr.task_id = t.id AND `
	switch kind {
	case "workspace":
		workspaceID, workspaceOK := target["workspace_id"].(string)
		repositoryID, repositoryOK := target["repository_id"].(string)
		if !workspaceOK || !repositoryOK {
			return "", nil, errors.New("sidebar workspace repository target is malformed")
		}
		providerIdentity := sidebarRepositoryHasProviderIdentity()
		localIdentity := sidebarRepositoryHasLocalIdentity()
		return base + "r.workspace_id = ? AND t.workspace_id = ? AND r.id = ? AND NOT " + providerIdentity + " AND NOT " + localIdentity + ")",
			[]any{workspaceID, workspaceID, repositoryID}, nil
	case "provider":
		providerID, providerOK := target["provider_id"].(string)
		host, hostOK := target["host"].(string)
		scope, scopeOK := target["scope"].(string)
		repositoryID, repositoryOK := target["provider_repository_id"].(string)
		if !providerOK || !hostOK || !scopeOK || !repositoryOK {
			return "", nil, errors.New("sidebar provider repository target is malformed")
		}
		hostExpr := sidebarProviderHostExpression(driver)
		scopeExpr := `COALESCE(NULLIF(r.provider_scope, ''), r.provider_owner, '')`
		return base + sidebarRepositoryHasProviderIdentity() + " AND r.provider = ? AND " + hostExpr + " = ? AND " + scopeExpr + " = ? AND r.provider_repo_id = ?)",
			[]any{providerID, host, scope, repositoryID}, nil
	case "local":
		path, pathOK := target["path"].(string)
		if !pathOK {
			return "", nil, errors.New("sidebar local repository target is malformed")
		}
		pathExpr := sidebarLocalRepositoryPathExpression(driver)
		return base + "NOT " + sidebarRepositoryHasProviderIdentity() + " AND " + sidebarRepositoryHasLocalIdentity() + " AND " + pathExpr + " = ?)",
			[]any{normalizeSidebarLocalRepositoryPath(path)}, nil
	default:
		return "", nil, fmt.Errorf("unsupported sidebar repository target kind %q", kind)
	}
}

func sidebarRepositoryHasProviderIdentity() string {
	return `(COALESCE(r.provider, '') <> '' AND COALESCE(r.provider_repo_id, '') <> '')`
}

func sidebarRepositoryHasLocalIdentity() string {
	return `(COALESCE(r.local_path, '') <> '')`
}

func sidebarLocalRepositoryPathExpression(driver string) string {
	backslash := "CHAR(92)"
	if dialect.IsPostgres(driver) {
		backslash = "CHR(92)"
	}
	raw := "REPLACE(COALESCE(r.local_path, ''), " + backslash + ", '/')"
	trimmed := "RTRIM(" + raw + ", '/')"
	return "CASE WHEN " + raw + " = '' THEN '' WHEN " + trimmed + " = '' AND SUBSTR(" + raw + ", 1, 1) = '/' THEN '/' " +
		"WHEN SUBSTR(" + trimmed + ", 1, 1) = '/' THEN '/' || LTRIM(" + trimmed + ", '/') ELSE " + trimmed + " END"
}

func normalizeSidebarLocalRepositoryPath(path string) string {
	normalized := strings.ReplaceAll(path, `\`, "/")
	if normalized == "" {
		return ""
	}
	withoutTrailing := strings.TrimRight(normalized, "/")
	if withoutTrailing == "" && strings.HasPrefix(normalized, "/") {
		return "/"
	}
	if strings.HasPrefix(withoutTrailing, "/") {
		return "/" + strings.TrimLeft(withoutTrailing, "/")
	}
	return withoutTrailing
}

func sidebarProviderHostExpression(driver string) string {
	explicit := "TRIM(COALESCE(r.provider_host, ''))"
	explicitHost := sidebarNormalizeProviderHost(driver, explicit, false, true)
	remoteHost := sidebarNormalizeProviderHost(driver, "COALESCE(r.remote_url, '')", true, false)
	defaultHost := `CASE r.provider WHEN 'github' THEN 'github.com' WHEN 'gitlab' THEN 'gitlab.com' WHEN 'azure_devops' THEN 'dev.azure.com' ELSE '' END`
	return "CASE WHEN " + explicitHost + " <> '' THEN " + explicitHost + " WHEN " + remoteHost + " <> '' THEN " + remoteHost + " ELSE " + defaultHost + " END"
}

func sidebarNormalizeProviderHost(driver, value string, requireScheme, includePath bool) string {
	trimmed := "RTRIM(TRIM(" + value + "), '/')"
	schemePosition := sidebarSQLPosition(driver, trimmed, "://")
	var tail string
	if requireScheme {
		tail = "CASE WHEN " + schemePosition + " > 0 THEN SUBSTR(" + trimmed + ", " + schemePosition + " + 3) ELSE '' END"
	} else {
		tail = "CASE WHEN " + schemePosition + " > 0 THEN SUBSTR(" + trimmed + ", " + schemePosition + " + 3) ELSE " + trimmed + " END"
	}
	authorityEnd := sidebarFirstURLDelimiterEnd(driver, tail, []string{"/", "?", "#"})
	authority := "SUBSTR(" + tail + ", 1, " + authorityEnd + " - 1)"
	userinfoPosition := sidebarSQLPosition(driver, authority, "@")
	host := "LOWER(CASE WHEN " + userinfoPosition + " > 0 THEN SUBSTR(" + authority + ", " + userinfoPosition + " + 1) ELSE " + authority + " END)"
	scheme := "CASE WHEN " + schemePosition + " > 0 THEN LOWER(SUBSTR(" + trimmed + ", 1, " + schemePosition + " - 1)) ELSE '' END"
	defaultPort := "CASE WHEN " + scheme + " = 'http' THEN '80' WHEN " + scheme + " IN ('', 'https') THEN '443' ELSE '' END"
	host = sidebarStripDefaultProviderPort(host, defaultPort)
	if !includePath {
		return "CASE WHEN " + host + " = '' THEN '' ELSE " + host + " END"
	}
	pathPosition := sidebarSQLPosition(driver, tail, "/")
	pathEnd := sidebarFirstURLDelimiterEnd(driver, tail, []string{"?", "#"})
	path := "CASE WHEN " + pathPosition + " > 0 THEN RTRIM(SUBSTR(" + tail + ", " + pathPosition + ", " + pathEnd + " - " + pathPosition + "), '/') ELSE '' END"
	return "CASE WHEN " + host + " = '' THEN '' ELSE " + host + " || " + path + " END"
}

func sidebarStripDefaultProviderPort(host, port string) string {
	suffix := "':' || (" + port + ")"
	portSuffix := "SUBSTR(" + host + ", LENGTH(" + host + ") - LENGTH(" + port + "))"
	hostWithoutPort := "SUBSTR(" + host + ", 1, LENGTH(" + host + ") - LENGTH(" + port + ") - 1)"
	return "CASE WHEN (" + port + ") <> '' AND " + portSuffix + " = " + suffix +
		" THEN " + hostWithoutPort + " ELSE " + host + " END"
}

func sidebarFirstURLDelimiterEnd(driver, value string, delimiters []string) string {
	end := "LENGTH(" + value + ") + 1"
	for _, delimiter := range delimiters {
		position := sidebarSQLPosition(driver, value, delimiter)
		end = "CASE WHEN " + position + " > 0 AND " + position + " < " + end + " THEN " + position + " ELSE " + end + " END"
	}
	return end
}

func sidebarSQLPosition(driver, value, needle string) string {
	if dialect.IsPostgres(driver) {
		if needle == "?" {
			return "POSITION(CHR(63) IN " + value + ")"
		}
		return "POSITION('" + needle + "' IN " + value + ")"
	}
	if needle == "?" {
		return "INSTR(" + value + ", CHAR(63))"
	}
	return "INSTR(" + value + ", '" + needle + "')"
}

func sidebarWorkflowColorToken(driver, value string) string {
	lower := "LOWER(COALESCE(" + value + ", ''))"
	class := `CASE ` + lower + `
		WHEN 'gray' THEN 'gray' WHEN 'grey' THEN 'gray' WHEN 'slate' THEN 'gray'
		WHEN 'zinc' THEN 'gray' WHEN 'neutral' THEN 'gray' WHEN 'stone' THEN 'gray'
		WHEN 'bg-slate-500' THEN 'gray' WHEN 'bg-gray-500' THEN 'gray' WHEN 'bg-zinc-500' THEN 'gray'
		WHEN 'bg-neutral-400' THEN 'gray' WHEN 'bg-neutral-500' THEN 'gray' WHEN 'bg-stone-500' THEN 'gray'
		WHEN 'red' THEN 'red' WHEN 'bg-red-500' THEN 'red'
		WHEN 'orange' THEN 'orange' WHEN 'bg-orange-500' THEN 'orange'
		WHEN 'amber' THEN 'yellow' WHEN 'yellow' THEN 'yellow' WHEN 'bg-amber-500' THEN 'yellow' WHEN 'bg-yellow-500' THEN 'yellow'
		WHEN 'lime' THEN 'green' WHEN 'green' THEN 'green' WHEN 'emerald' THEN 'green'
		WHEN 'bg-lime-500' THEN 'green' WHEN 'bg-green-500' THEN 'green' WHEN 'bg-emerald-500' THEN 'green'
		WHEN 'teal' THEN 'cyan' WHEN 'cyan' THEN 'cyan' WHEN 'bg-teal-500' THEN 'cyan' WHEN 'bg-cyan-500' THEN 'cyan'
		WHEN 'sky' THEN 'blue' WHEN 'blue' THEN 'blue' WHEN 'bg-sky-500' THEN 'blue' WHEN 'bg-blue-500' THEN 'blue'
		WHEN 'indigo' THEN 'indigo' WHEN 'bg-indigo-500' THEN 'indigo'
		WHEN 'violet' THEN 'purple' WHEN 'purple' THEN 'purple' WHEN 'bg-violet-500' THEN 'purple' WHEN 'bg-purple-500' THEN 'purple'
		WHEN 'fuchsia' THEN 'pink' WHEN 'pink' THEN 'pink' WHEN 'bg-fuchsia-500' THEN 'pink' WHEN 'bg-pink-500' THEN 'pink'
		ELSE `
	var custom string
	if dialect.IsPostgres(driver) {
		custom = "CASE WHEN COALESCE(" + value + ", '') ~ '^#([0-9A-Fa-f]{3}|[0-9A-Fa-f]{4}|[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$' THEN 'custom' ELSE 'gray' END"
	} else {
		custom = "CASE WHEN LENGTH(" + value + ") IN (4, 5, 7, 9) AND SUBSTR(" + value + ", 1, 1) = '#' AND SUBSTR(" + value + ", 2) NOT GLOB '*[^0-9a-fA-F]*' THEN 'custom' ELSE 'gray' END"
	}
	return class + custom + " END"
}
