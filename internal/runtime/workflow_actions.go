package runtime

import (
	"context"
	"time"

	serveractions "gorp/internal/actions"
	"gorp/internal/record"
	internalworkflow "gorp/internal/workflow"
)

const workflowProcessEscalationAction = "workflow.process.escalation"

func registerWorkflowRuntimeActions(reg *serveractions.Registry, env *record.Env, delegations internalworkflow.ApprovalDelegationProvider, mailer envActionHooks) error {
	if reg == nil || env == nil {
		return nil
	}
	return reg.RegisterGo(workflowProcessEscalationAction, func(ctx context.Context, _ serveractions.ServerAction, exec serveractions.ExecutionContext) (serveractions.Result, error) {
		now := exec.Now
		if now.IsZero() {
			now = time.Now().UTC()
		}
		runEnv := workflowActionEnv(env, exec)
		if mailer.env == env {
			mailer.env = runEnv
		}
		result, err := (internalworkflow.Dispatcher{
			Actions:     reg,
			Mailer:      mailer,
			Delegations: delegations,
			Now:         func() time.Time { return now },
		}).ProcessEscalations(ctx, runEnv)
		return serveractions.Result{Metadata: map[string]any{
			"due":     result.Due,
			"applied": result.Applied,
			"skipped": result.Skipped,
		}}, err
	})
}

func workflowActionEnv(env *record.Env, exec serveractions.ExecutionContext) *record.Env {
	if env == nil || exec.UserID == 0 {
		return env
	}
	base := env.Context()
	values := make(map[string]any, len(base.Values)+1)
	for key, value := range base.Values {
		values[key] = value
	}
	if len(exec.UserGroupIDs) > 0 {
		values["group_ids"] = append([]int64(nil), exec.UserGroupIDs...)
	}
	return env.WithContext(record.Context{
		UserID:     exec.UserID,
		CompanyID:  base.CompanyID,
		CompanyIDs: append([]int64(nil), base.CompanyIDs...),
		Values:     values,
	})
}
