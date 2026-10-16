package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/kandev/kandev/internal/githubhost"
)

const githubCLIHTTPS = "https"

// Explicit CLI targets cannot widen the repository scope of a managed token.
func validateGitHubCLITargets(args []string, getenv func(string) string, scope githubBrokerResolveRequest, host githubhost.Host) error {
	raw, found, err := githubCLIRepositoryArgument(args)
	if err != nil {
		return err
	}
	if !found {
		raw = strings.TrimSpace(getenv("GH_REPO"))
	}
	if raw != "" {
		repo, err := parseGitHubCLIRepository(raw, host.String())
		if err != nil {
			return err
		}
		if !strings.EqualFold(repo.host, host.String()) || !strings.EqualFold(repo.owner, scope.Owner) || !strings.EqualFold(repo.repo, scope.Repo) {
			return fmt.Errorf("GitHub CLI repository is outside the managed credential scope")
		}
	}
	return validateGitHubCLIHostArguments(args, scope, host)
}

func validateGitHubCLIHostArguments(args []string, scope githubBrokerResolveRequest, host githubhost.Host) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--hostname" || strings.HasPrefix(arg, "--hostname=") {
			raw := strings.TrimPrefix(arg, "--hostname=")
			if arg == "--hostname" {
				i++
				if i >= len(args) {
					return fmt.Errorf("GitHub CLI hostname argument is empty")
				}
				raw = args[i]
			}
			target, err := githubhost.Parse(raw)
			if err != nil || target.String() != host.String() {
				return fmt.Errorf("GitHub CLI hostname is outside the managed credential scope")
			}
			continue
		}
		if githubCLIContentFlag(arg) {
			i++
			continue
		}
		if strings.Contains(arg, "://") && !strings.HasPrefix(arg, "-") {
			if err := validateGitHubCLITargetURL(arg, scope, host); err != nil {
				return err
			}
		}
	}
	return nil
}

func githubCLIContentFlag(arg string) bool {
	switch arg {
	case "--body", "--body-file", "--title", "--jq", "--template", "--header", "-H", "--field", "-F", "--raw-field", "-f":
		return true
	default:
		return false
	}
}

func validateGitHubCLITargetURL(raw string, scope githubBrokerResolveRequest, host githubhost.Host) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != githubCLIHTTPS || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("GitHub CLI target URL is invalid")
	}
	graphql, _ := url.Parse(host.GraphQLURL())
	if strings.EqualFold(u.Host, graphql.Host) && u.Path == graphql.Path {
		return nil
	}
	api, _ := url.Parse(host.RESTURL())
	if !strings.EqualFold(u.Host, host.String()) && !strings.EqualFold(u.Host, api.Host) {
		return fmt.Errorf("GitHub CLI target URL is outside the managed credential host")
	}
	return validateGitHubCLITargetPath(u, api, scope)
}

func validateGitHubCLITargetPath(u, api *url.URL, scope githubBrokerResolveRequest) error {
	path := strings.Trim(u.Path, "/")
	apiPath := strings.Trim(api.Path, "/")
	isAPI := u.Host == api.Host && (apiPath == "" || path == apiPath || strings.HasPrefix(path, apiPath+"/"))
	if isAPI {
		path = strings.TrimPrefix(strings.TrimPrefix(path, apiPath), "/")
		if !strings.HasPrefix(path, "repos/") {
			return nil
		}
		path = strings.TrimPrefix(path, "repos/")
	}
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && (!strings.EqualFold(parts[0], scope.Owner) || !strings.EqualFold(parts[1], scope.Repo)) {
		return fmt.Errorf("GitHub CLI target URL is outside the managed credential repository")
	}
	return nil
}
