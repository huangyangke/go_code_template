package async_queue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ================================
// ResolveFeatureMode
// ================================

func TestResolveFeatureMode_Lite(t *testing.T) {
	cfg := ResolveFeatureMode(FeatureModeLite, nil)
	assert.False(t, cfg.EnableStatusStore)
	assert.False(t, cfg.EnablePelRecovery)
	assert.False(t, cfg.EnableHeartbeat)
	assert.False(t, cfg.EnableCancel)
}

func TestResolveFeatureMode_Standard(t *testing.T) {
	cfg := ResolveFeatureMode(FeatureModeStandard, nil)
	assert.True(t, cfg.EnableStatusStore)
	assert.False(t, cfg.EnablePelRecovery)
	assert.False(t, cfg.EnableHeartbeat)
	assert.True(t, cfg.EnableCancel)
}

func TestResolveFeatureMode_Full(t *testing.T) {
	cfg := ResolveFeatureMode(FeatureModeFull, nil)
	assert.True(t, cfg.EnableStatusStore)
	assert.True(t, cfg.EnablePelRecovery)
	assert.True(t, cfg.EnableHeartbeat)
	assert.True(t, cfg.EnableCancel)
}

func TestResolveFeatureMode_Overrides_AddFeatureToLite(t *testing.T) {
	overrides := &FeatureConfig{EnableStatusStore: true}
	cfg := ResolveFeatureMode(FeatureModeLite, overrides)
	assert.True(t, cfg.EnableStatusStore)
	// others remain false
	assert.False(t, cfg.EnablePelRecovery)
	assert.False(t, cfg.EnableHeartbeat)
	assert.False(t, cfg.EnableCancel)
}

func TestResolveFeatureMode_Overrides_AddPelRecovery(t *testing.T) {
	// Adding PelRecovery to Lite via override must also force Heartbeat
	overrides := &FeatureConfig{EnablePelRecovery: true}
	cfg := ResolveFeatureMode(FeatureModeLite, overrides)
	assert.True(t, cfg.EnablePelRecovery)
	assert.True(t, cfg.EnableHeartbeat, "EnablePelRecovery must force EnableHeartbeat")
}

func TestResolveFeatureMode_Overrides_AddCancel(t *testing.T) {
	// Adding Cancel to Lite via override must also force StatusStore
	overrides := &FeatureConfig{EnableCancel: true}
	cfg := ResolveFeatureMode(FeatureModeLite, overrides)
	assert.True(t, cfg.EnableCancel)
	assert.True(t, cfg.EnableStatusStore, "EnableCancel must force EnableStatusStore")
}

func TestResolveFeatureMode_DependencyConstraint_PelForcesHeartbeat(t *testing.T) {
	// Full already has both; verify constraint holds even when set independently
	cfg := ResolveFeatureMode(FeatureModeStandard, &FeatureConfig{EnablePelRecovery: true})
	assert.True(t, cfg.EnableHeartbeat)
}

func TestResolveFeatureMode_DependencyConstraint_CancelForcesStatusStore(t *testing.T) {
	// Standard already has both; force via override on Lite
	cfg := ResolveFeatureMode(FeatureModeLite, &FeatureConfig{EnableCancel: true})
	assert.True(t, cfg.EnableStatusStore)
}

func TestResolveFeatureMode_NilOverrides(t *testing.T) {
	// nil overrides must not panic and must return clean preset
	assert.NotPanics(t, func() {
		_ = ResolveFeatureMode(FeatureModeFull, nil)
	})
}

// ================================
// ValidateSchedulerConfig
// ================================

func TestValidateSchedulerConfig_NegativeWorkerCapacity(t *testing.T) {
	err := ValidateSchedulerConfig(SchedulerConfig{WorkerCapacity: -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WorkerCapacity")
}

func TestValidateSchedulerConfig_NegativeDefaultTimeout(t *testing.T) {
	err := ValidateSchedulerConfig(SchedulerConfig{
		WorkerCapacity: 10,
		DefaultTimeout: -1 * time.Second,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DefaultTimeout")
}

func TestValidateSchedulerConfig_Valid(t *testing.T) {
	err := ValidateSchedulerConfig(SchedulerConfig{
		WorkerCapacity: 0,
		DefaultTimeout: 0,
	})
	assert.NoError(t, err)

	err = ValidateSchedulerConfig(SchedulerConfig{
		WorkerCapacity: 100,
		DefaultTimeout: 30 * time.Second,
	})
	assert.NoError(t, err)
}

// ================================
// ValidatePelConfig
// ================================

func TestValidatePelConfig_ZeroMinIdle(t *testing.T) {
	err := ValidatePelConfig(PelConfig{MinIdle: 0, MaxRetries: 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MinIdle")
}

func TestValidatePelConfig_NegativeMinIdle(t *testing.T) {
	err := ValidatePelConfig(PelConfig{MinIdle: -1 * time.Second, MaxRetries: 3})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MinIdle")
}

func TestValidatePelConfig_NegativeMaxRetries(t *testing.T) {
	err := ValidatePelConfig(PelConfig{MinIdle: 1 * time.Second, MaxRetries: -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MaxRetries")
}

func TestValidatePelConfig_Valid(t *testing.T) {
	err := ValidatePelConfig(PelConfig{MinIdle: 5 * time.Minute, MaxRetries: 3})
	assert.NoError(t, err)

	// MaxRetries=0 is allowed (no retries)
	err = ValidatePelConfig(PelConfig{MinIdle: 1 * time.Second, MaxRetries: 0})
	assert.NoError(t, err)
}

// ================================
// ValidateEndpointConfig
// ================================

func TestValidateEndpointConfig_NilHandler(t *testing.T) {
	endpoints := map[string]EndpointConfig{
		"/submit": {Handler: nil},
	}
	err := ValidateEndpointConfig(endpoints)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing Handler")
}

func TestValidateEndpointConfig_ReservedPathConflict(t *testing.T) {
	for _, reserved := range []string{"/status", "/cancel", "/events"} {
		endpoints := map[string]EndpointConfig{
			reserved: {Handler: func(ctx Context) (any, error) { return nil, nil }},
		}
		err := ValidateEndpointConfig(endpoints)
		require.Error(t, err, "expected error for reserved path %q", reserved)
		assert.Contains(t, err.Error(), "reserved path")
	}
}

func TestValidateEndpointConfig_ReservedPathSubPath(t *testing.T) {
	// /status/foo should also conflict
	endpoints := map[string]EndpointConfig{
		"/status/foo": {Handler: func(ctx Context) (any, error) { return nil, nil }},
	}
	err := ValidateEndpointConfig(endpoints)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved path")
}

func TestValidateEndpointConfig_Valid(t *testing.T) {
	endpoints := map[string]EndpointConfig{
		"/submit": {
			Handler:        func(ctx Context) (any, error) { return nil, nil },
			MaxConcurrency: 10,
			Timeout:        30 * time.Second,
		},
	}
	err := ValidateEndpointConfig(endpoints)
	assert.NoError(t, err)
}

func TestValidateEndpointConfig_NegativeMaxConcurrency(t *testing.T) {
	endpoints := map[string]EndpointConfig{
		"/submit": {
			Handler:        func(ctx Context) (any, error) { return nil, nil },
			MaxConcurrency: -1,
		},
	}
	err := ValidateEndpointConfig(endpoints)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MaxConcurrency")
}

func TestValidateEndpointConfig_NegativeTimeout(t *testing.T) {
	endpoints := map[string]EndpointConfig{
		"/submit": {
			Handler: func(ctx Context) (any, error) { return nil, nil },
			Timeout: -1 * time.Second,
		},
	}
	err := ValidateEndpointConfig(endpoints)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Timeout")
}

func TestValidateEndpointConfig_EmptyMap(t *testing.T) {
	err := ValidateEndpointConfig(map[string]EndpointConfig{})
	assert.NoError(t, err)
}

// ================================
// Key builder helpers
// ================================

func TestBuildCancelChannel(t *testing.T) {
	got := buildCancelChannel("myns")
	assert.Equal(t, "aikit:async:myns:channel:cancel", got)
}

func TestBuildHeartbeatKey(t *testing.T) {
	got := buildHeartbeatKey("myns", "group1", "consumer1")
	assert.Equal(t, "aikit:async:myns:heartbeat:group1:consumer1", got)
}

func TestBuildEndpointLimitKeyPrefix(t *testing.T) {
	got := buildEndpointLimitKeyPrefix("myns")
	assert.Equal(t, "aikit:async:myns:limit", got)
}

func TestBuildStatusScanPattern(t *testing.T) {
	got := buildStatusScanPattern("myns")
	assert.Equal(t, "aikit:async:myns:task:status:*", got)
}

func TestResolveFeatureMode_Overrides_PelAndCancel(t *testing.T) {
	cfg := ResolveFeatureMode(FeatureModeLite, &FeatureConfig{
		EnablePelRecovery: true,
		EnableCancel:      true,
	})
	assert.True(t, cfg.EnablePelRecovery)
	assert.True(t, cfg.EnableHeartbeat) // forced by PelRecovery
	assert.True(t, cfg.EnableCancel)
	assert.True(t, cfg.EnableStatusStore) // forced by Cancel
}

func TestResolveFeatureMode_Overrides_Heartbeat(t *testing.T) {
	cfg := ResolveFeatureMode(FeatureModeLite, &FeatureConfig{
		EnableHeartbeat: true,
	})
	assert.True(t, cfg.EnableHeartbeat)
	assert.False(t, cfg.EnableStatusStore)
}

func TestBuildStreamKey(t *testing.T) {
	got := buildStreamKey("myns")
	assert.Equal(t, "aikit:async:myns:stream", got)
}
