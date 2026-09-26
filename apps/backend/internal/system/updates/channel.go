package updates

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

type Channel string

const (
	ChannelStable  Channel = "stable"
	ChannelNightly Channel = "nightly"
)

const updatesChannelSettingKey = "updates_channel"

const channelSettingsUnavailableReason = "Update channel settings are unavailable."

var ErrInvalidChannel = errors.New("invalid updates channel")
var ErrChannelUnsupported = errors.New("updates channel unsupported")
var ErrUpdateResolve = errors.New("update target resolution failed")
var ErrUpdateTargetChanged = errors.New("update target changed; refresh updates before applying")

var nightlyVersionPattern = regexp.MustCompile(
	`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)-nightly\.sha[0-9a-f]{12}$`,
)

type settingsStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Save(ctx context.Context, key string, value []byte) error
}

// WithSettingsStore wires the install-wide settings store used for channel selection.
func WithSettingsStore(store settingsStore) Option {
	return func(s *Service) {
		s.settings = store
	}
}

func parseChannel(value string) (Channel, bool) {
	channel := Channel(value)
	switch channel {
	case ChannelStable, ChannelNightly:
		return channel, true
	default:
		return ChannelStable, false
	}
}

func (s *Service) selectedChannel(ctx context.Context) (Channel, error) {
	if s.settings == nil {
		return ChannelStable, nil
	}
	raw, ok, err := s.settings.Get(ctx, updatesChannelSettingKey)
	if err != nil {
		return ChannelStable, fmt.Errorf("read updates channel: %w", err)
	}
	if !ok {
		return ChannelStable, nil
	}
	channel, valid := parseChannel(string(raw))
	if !valid {
		return ChannelStable, nil
	}
	return channel, nil
}

func (s *Service) persistChannel(ctx context.Context, channel Channel) error {
	if _, valid := parseChannel(string(channel)); !valid {
		return fmt.Errorf("%w: %q", ErrInvalidChannel, channel)
	}
	if s.settings == nil {
		return errors.New("updates settings store is unavailable")
	}
	if err := s.settings.Save(ctx, updatesChannelSettingKey, []byte(channel)); err != nil {
		return fmt.Errorf("save updates channel: %w", err)
	}
	return nil
}

func (s *Service) effectiveChannel(
	ctx context.Context,
	install InstallStateResponse,
) (Channel, error) {
	editable, _ := s.channelSupport(install)
	if !editable {
		return ChannelStable, nil
	}
	return s.selectedChannel(ctx)
}

func (s *Service) channelSupport(install InstallStateResponse) (bool, string) {
	editable, reason := install.nightlySupport()
	if !editable {
		return false, reason
	}
	if s.settings == nil {
		return false, channelSettingsUnavailableReason
	}
	return true, ""
}

func isNightlyVersion(version string) bool {
	return nightlyVersionPattern.MatchString(version)
}

// SelectChannel validates and persists a new update channel, returning its
// resolved update state.
func (s *Service) SelectChannel(ctx context.Context, value string) (UpdatesResponse, error) {
	channel, valid := parseChannel(value)
	if !valid {
		return UpdatesResponse{}, fmt.Errorf("%w: %q", ErrInvalidChannel, value)
	}
	install, _ := s.detectInstallState()
	editable, reason := s.channelSupport(install)
	if !editable {
		return UpdatesResponse{}, fmt.Errorf("%w: %s", ErrChannelUnsupported, reason)
	}
	if ok, _ := s.limiter.Allow(); !ok {
		return UpdatesResponse{}, ErrRateLimited
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	version, targetURL, err := s.resolveLatest(ctx, channel)
	if err != nil {
		return UpdatesResponse{}, fmt.Errorf("%w: %v", ErrUpdateResolve, err)
	}
	checkedAt := s.now().UTC()
	if err := s.writeLatestVersion(channel, version, targetURL, checkedAt); err != nil {
		return UpdatesResponse{}, err
	}
	if err := s.persistChannel(ctx, channel); err != nil {
		return UpdatesResponse{}, err
	}
	return s.buildResponseFromChannel(channel, install, version, targetURL, checkedAt), nil
}
