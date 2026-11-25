package sshtransport

import (
	"errors"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

var (
	ErrMigrationAliasMissing = errors.New("saved SSH alias is unavailable")
	ErrMigrationRoute        = errors.New("saved SSH route requires review")
	ErrMigrationIdentity     = errors.New("saved SSH identity selector is unavailable")
)

// ResolveMigrationTargets resolves only local configuration on the backend
// account. Alias fallbacks are disallowed when the saved selector has no match.
func ResolveMigrationTargets(cfg SSHConnConfig) (target, bastion *SSHTarget, err error) {
	var config effectiveSSHConfig
	load := func() effectiveSSHConfig {
		return config
	}
	if cfg.HostAlias != "" || cfg.ProxyJump != "" {
		config = loadEffectiveSSHConfig()
	}
	resolve := func(input SSHConnConfig) (*SSHTarget, error) {
		return resolveMigrationTarget(config, input)
	}
	target, err = resolve(cfg)
	if err != nil || target.ProxyJump == "" {
		return target, nil, err
	}
	if strings.Contains(target.ProxyJump, ",") || strings.EqualFold(target.ProxyJump, "none") {
		return target, nil, ErrMigrationRoute
	}
	if _, _, _, literal := parseLiteralProxyJump(target.ProxyJump); !literal && strings.ContainsAny(target.ProxyJump, "@:") {
		return target, nil, ErrMigrationRoute
	}
	bastion, err = resolveBastionWith(target, load, resolve)
	if err != nil {
		return target, bastion, err
	}
	if bastion.ProxyJump != "" {
		return target, bastion, ErrMigrationRoute
	}
	return target, bastion, nil
}

func resolveMigrationTarget(config effectiveSSHConfig, cfg SSHConnConfig) (*SSHTarget, error) {
	target := initialTargetFromConfig(cfg)
	target.PinnedFingerprint = cfg.PinnedFingerprint
	if alias := strings.TrimSpace(cfg.HostAlias); alias != "" {
		if err := inheritMigrationAlias(config, alias, target); err != nil {
			target.IdentitySource = ""
			return target, err
		}
	}
	if target.IdentitySource != SSHIdentitySourceFile {
		switch target.IdentityAgent {
		case sshAuthSocketEnv, "$" + sshAuthSocketEnv, "${" + sshAuthSocketEnv + "}":
			target.IdentitySource = SSHIdentitySourceAgent
			target.IdentityAgent = ""
		}
	}
	if err := applyTargetDefaults(target); err != nil {
		if target.IdentityAgent != "" {
			target.IdentitySource = ""
			return target, ErrMigrationIdentity
		}
		return target, ErrMigrationRoute
	}
	return target, nil
}

func inheritMigrationAlias(config effectiveSSHConfig, alias string, target *SSHTarget) error {
	if !migrationAliasConfigured(config, alias) {
		return ErrMigrationAliasMissing
	}
	if migrationUnsupportedPolicy(config, alias) {
		return ErrMigrationRoute
	}
	port := lookupSSHConfig(config, alias, "Port")
	if target.Port == 0 && port != "" {
		if value, err := strconv.Atoi(port); err != nil || value < 1 || value > 65535 {
			return ErrMigrationRoute
		}
	}
	inheritFromSSHConfig(config, alias, target)
	return nil
}

func migrationAliasConfigured(cfg effectiveSSHConfig, alias string) bool {
	if lookupSSHConfig(cfg, alias, "HostName") != "" {
		return true
	}
	for _, config := range []*ssh_config.Config{cfg.user, cfg.system} {
		if config == nil {
			continue
		}
		for _, block := range config.Hosts {
			if block.Matches(alias) && migrationAliasPattern(block) {
				return true
			}
		}
	}
	return false
}

func migrationAliasPattern(block *ssh_config.Host) bool {
	for _, pattern := range block.Patterns {
		value := pattern.String()
		if value != "*" && !strings.HasPrefix(value, "!") {
			return true
		}
	}
	return false
}

func migrationUnsupportedPolicy(cfg effectiveSSHConfig, alias string) bool {
	for _, key := range []string{"ProxyCommand", "HostKeyAlias", "KnownHostsCommand"} {
		value := lookupSSHConfig(cfg, alias, key)
		if value != "" && !strings.EqualFold(value, "none") {
			return true
		}
	}
	user := lookupSSHConfig(cfg, alias, "UserKnownHostsFile")
	global := lookupSSHConfig(cfg, alias, "GlobalKnownHostsFile")
	return user != "" && user != "~/.ssh/known_hosts" ||
		global != "" && global != "/etc/ssh/ssh_known_hosts"
}
