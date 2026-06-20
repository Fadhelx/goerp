package workflow

import (
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/mail"
	"gorp/internal/record"
)

const (
	approvalActivityDefaultSummary = "Waiting Approval"
	approvalActivityTypeName       = "Approval"
)

type approvalActivityOptions struct {
	Enabled   bool
	Model     string
	RecordID  int64
	UserIDs   []int64
	DateField string
	Days      int
	Summary   string
	At        time.Time
}

func syncApprovalActivities(env *record.Env, opts approvalActivityOptions) error {
	if env == nil || strings.TrimSpace(opts.Model) == "" || opts.RecordID == 0 {
		return nil
	}
	if _, ok := env.ModelMetadata("mail.activity"); !ok {
		return nil
	}
	if _, ok := env.ModelMetadata("mail.activity.type"); !ok {
		return nil
	}
	systemEnv := workflowSystemEnv(env)
	activityTypeID := approvalActivityTypeID(systemEnv)
	if activityTypeID == 0 {
		return nil
	}
	desiredUsers := approvalActivityDesiredUsers(opts)
	rows, err := approvalActivityRows(systemEnv, opts.Model, opts.RecordID, activityTypeID)
	if err != nil {
		return err
	}
	existingByUser := map[int64]bool{}
	var unlinkIDs []int64
	for _, row := range rows {
		id := int64FromAny(row["id"])
		userID := int64FromAny(row["user_id"])
		if id == 0 {
			continue
		}
		if !desiredUsers[userID] || existingByUser[userID] {
			unlinkIDs = append(unlinkIDs, id)
			continue
		}
		existingByUser[userID] = true
	}
	if len(unlinkIDs) > 0 {
		if err := systemEnv.Model("mail.activity").Browse(unlinkIDs...).Unlink(); err != nil {
			return err
		}
	}
	if !opts.Enabled || len(desiredUsers) == 0 {
		return nil
	}
	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		summary = approvalActivityDefaultSummary
	}
	deadline := approvalActivityDeadline(systemEnv, opts)
	for _, userID := range uniqueSortedIDs(opts.UserIDs) {
		if userID == 0 || existingByUser[userID] {
			continue
		}
		if _, err := mail.ScheduleActivity(systemEnv, mail.ActivityScheduleRequest{
			Model:          opts.Model,
			ResIDs:         []int64{opts.RecordID},
			ActivityTypeID: activityTypeID,
			UserID:         userID,
			DateDeadline:   deadline,
			Summary:        summary,
			Automated:      true,
		}); err != nil {
			return err
		}
	}
	return nil
}

func approvalActivityDateValue(env *record.Env, opts approvalActivityOptions) any {
	if !opts.Enabled || len(approvalActivityDesiredUsers(opts)) == 0 {
		return ""
	}
	return approvalActivityDeadline(env, opts)
}

func approvalActivityDeadline(env *record.Env, opts approvalActivityOptions) string {
	base := approvalActivityBaseDate(env, opts)
	return base.UTC().AddDate(0, 0, opts.Days).Format("2006-01-02")
}

func approvalActivityBaseDate(env *record.Env, opts approvalActivityOptions) time.Time {
	if dateField := strings.TrimSpace(opts.DateField); dateField != "" && env != nil {
		if meta, ok := env.ModelMetadata(opts.Model); ok {
			if _, ok := meta.Fields[dateField]; ok {
				rows, err := env.Model(opts.Model).Browse(opts.RecordID).Read(dateField)
				if err == nil && len(rows) > 0 {
					if parsed := approvalActivityDateFromAny(rows[0][dateField]); !parsed.IsZero() {
						return parsed
					}
				}
			}
		}
	}
	if !opts.At.IsZero() {
		return opts.At
	}
	return time.Now().UTC()
}

func approvalActivityDateFromAny(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		text := strings.TrimSpace(typed)
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if out, err := time.Parse(layout, text); err == nil {
				return out
			}
		}
	}
	return time.Time{}
}

func approvalActivityDesiredUsers(opts approvalActivityOptions) map[int64]bool {
	out := map[int64]bool{}
	if !opts.Enabled {
		return out
	}
	for _, userID := range uniqueSortedIDs(opts.UserIDs) {
		if userID != 0 {
			out[userID] = true
		}
	}
	return out
}

func approvalActivityRows(env *record.Env, modelName string, recordID int64, activityTypeID int64) ([]map[string]any, error) {
	found, err := env.Model("mail.activity").Search(domain.And(
		domain.Cond("res_model", "=", strings.TrimSpace(modelName)),
		domain.Cond("res_id", "=", recordID),
		domain.Cond("activity_type_id", "=", activityTypeID),
		domain.Cond("state", "!=", "done"),
		domain.Cond("automated", "=", true),
	))
	if err != nil {
		return nil, err
	}
	return found.Read("id", "user_id")
}

func approvalActivityTypeID(env *record.Env) int64 {
	if id := approvalActivityTypeIDFromXMLID(env); id != 0 {
		return id
	}
	found, err := env.Model("mail.activity.type").Search(domain.Cond("name", "=", approvalActivityTypeName))
	if err != nil {
		return 0
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["id"])
}

func approvalActivityTypeIDFromXMLID(env *record.Env) int64 {
	if _, ok := env.ModelMetadata("ir.model.data"); !ok {
		return 0
	}
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", "=", "oi_workflow"),
		domain.Cond("name", "=", "activity_type_approval"),
		domain.Cond("model", "=", "mail.activity.type"),
	))
	if err != nil {
		return 0
	}
	rows, err := found.Read("res_id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["res_id"])
}
