package runtime

import (
	"context"
	"time"

	serveractions "gorp/internal/actions"
	"gorp/internal/record"
)

const delegationClearAccessCacheAction = "delegation_clear_expired_access"

func registerDelegationRuntimeActions(reg *serveractions.Registry, env *record.Env) error {
	if reg == nil || env == nil {
		return nil
	}
	return reg.RegisterGo(delegationClearAccessCacheAction, delegationRuntimeClearAccessCache(env))
}

func delegationRuntimeClearAccessCache(env *record.Env) serveractions.GoAction {
	return func(_ context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		now := exec.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		err := persistDelegationCacheEventRuntime(env, "clear_access_cache", now)
		return serveractions.Result{
			Kind:         serveractions.KindGo,
			GoActionName: delegationClearAccessCacheAction,
			Metadata: map[string]any{
				"cache_invalidated": err == nil,
			},
		}, err
	}
}

func persistDelegationCacheEventRuntime(env *record.Env, reason string, at time.Time) error {
	if env == nil {
		return nil
	}
	if _, ok := env.ModelMetadata("delegation.cache.event"); !ok {
		return nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := env.Model("delegation.cache.event").Create(map[string]any{
		"user_ids":   "[]",
		"reason":     reason,
		"created_at": at.UTC().Format(time.RFC3339Nano),
	})
	return err
}
