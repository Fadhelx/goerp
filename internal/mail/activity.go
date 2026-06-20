package mail

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type ActivityScheduleRequest struct {
	Model             string
	ResIDs            []int64
	ActivityTypeXMLID string
	ActivityTypeID    int64
	RecommendedTypeID int64
	DateDeadline      any
	Summary           string
	Note              string
	UserID            int64
	Automated         bool
	PreviousTypeID    int64
}

func ScheduleActivity(env *record.Env, req ActivityScheduleRequest) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("mail activity requires env")
	}
	if skipMailActivityAutomation(env) {
		return nil, nil
	}
	modelName := strings.TrimSpace(req.Model)
	if modelName == "" || len(req.ResIDs) == 0 {
		return nil, fmt.Errorf("activity_schedule requires model and record ids")
	}
	activityTypeID := req.ActivityTypeID
	if activityTypeID == 0 && req.ActivityTypeXMLID != "" {
		activityTypeID = resolveXMLID(env, req.ActivityTypeXMLID, "mail.activity.type")
	}
	if activityTypeID == 0 && req.RecommendedTypeID != 0 {
		activityTypeID = req.RecommendedTypeID
	}
	if activityTypeID == 0 && req.PreviousTypeID != 0 {
		activityTypeID = int64FromAny(activityTypeRow(env, req.PreviousTypeID)["triggered_next_type_id"])
	}
	activityType := activityTypeRow(env, activityTypeID)
	if activityTypeID == 0 || invalidActivityTypeModel(activityType, modelName) {
		activityTypeID = defaultActivityTypeID(env)
		activityType = activityTypeRow(env, activityTypeID)
	}
	if activityTypeID == 0 {
		return nil, fmt.Errorf("mail.activity.type not found")
	}
	deadline := activityDateValue(req.DateDeadline)
	ids := make([]int64, 0, len(req.ResIDs))
	for _, resID := range uniqueIDs(req.ResIDs) {
		if err := ensureRecordExists(env, modelName, resID); err != nil {
			return nil, err
		}
		values := map[string]any{
			"activity_type_id":             activityTypeID,
			"activity_category":            firstNonEmptyString(stringFromAny(activityType["category"]), "default"),
			"recommended_activity_type_id": req.RecommendedTypeID,
			"previous_activity_type_id":    req.PreviousTypeID,
			"has_recommended_activities":   activityTypeHasRecommendations(env, req.PreviousTypeID),
			"chaining_type":                firstNonEmptyString(stringFromAny(activityType["chaining_type"]), "suggest"),
			"res_model":                    modelName,
			"res_id":                       resID,
			"user_id":                      firstNonZero(req.UserID, int64FromAny(activityType["default_user_id"]), env.Context().UserID),
			"date_deadline":                deadline,
			"summary":                      firstNonEmptyString(req.Summary, stringFromAny(activityType["summary"]), stringFromAny(activityType["name"])),
			"note":                         firstNonEmptyString(req.Note, stringFromAny(activityType["default_note"])),
			"state":                        "open",
			"automated":                    req.Automated,
			"active":                       true,
		}
		if req.RecommendedTypeID == 0 {
			delete(values, "recommended_activity_type_id")
		}
		id, err := env.Model("mail.activity").Create(values)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

type ActivitySelectRequest struct {
	Model              string
	ResIDs             []int64
	ActivityTypeXMLIDs []string
	UserID             int64
	OnlyAutomated      bool
}

func ActivityFeedback(env *record.Env, req ActivitySelectRequest, feedback string, attachmentIDs []int64) (bool, error) {
	if env == nil {
		return false, fmt.Errorf("mail activity requires env")
	}
	if skipMailActivityAutomation(env) {
		return false, nil
	}
	activities, ok, err := findActivities(env, req)
	if err != nil || !ok {
		return ok, err
	}
	_, err = completeActivities(env, activities.IDs(), feedback, attachmentIDs)
	return true, err
}

type ActivityDoneResult struct {
	MessageIDs      []int64
	NextActivityIDs []int64
}

func ActivityActionFeedback(env *record.Env, activityIDs []int64, feedback string, attachmentIDs []int64) (any, error) {
	result, err := completeActivities(env, activityIDs, feedback, attachmentIDs)
	if err != nil {
		return nil, err
	}
	if len(result.MessageIDs) == 0 {
		return false, nil
	}
	return result.MessageIDs[0], nil
}

func ActivityActionFeedbackScheduleNext(env *record.Env, activityIDs []int64, feedback string, attachmentIDs []int64) (any, error) {
	rows, err := activityRows(env, activityIDs)
	if err != nil {
		return nil, err
	}
	result, err := completeActivities(env, activityIDs, feedback, attachmentIDs)
	if err != nil {
		return nil, err
	}
	if len(result.NextActivityIDs) > 0 || len(rows) == 0 {
		return false, nil
	}
	row := rows[0]
	return map[string]any{
		"name":      "Schedule an Activity",
		"context":   activityScheduleNextContext(row),
		"view_mode": "form",
		"res_model": "mail.activity",
		"views":     []any{[]any{false, "form"}},
		"type":      "ir.actions.act_window",
		"target":    "new",
	}, nil
}

type ActivityDataOptions struct {
	Limit     int
	Offset    int
	FetchDone bool
}

func ActivityFormat(env *record.Env, activityIDs []int64) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail activity requires env")
	}
	activityIDs = uniqueIDs(activityIDs)
	if len(activityIDs) == 0 {
		return map[string]any{"mail.activity": []map[string]any{}}, nil
	}
	systemEnv := activityActiveTestEnv(env, false)
	rows, err := systemEnv.Model("mail.activity").Browse(activityIDs...).Read("id", "activity_category", "activity_type_id", "attachment_ids", "chaining_type", "date_deadline", "date_done", "note", "res_id", "res_model", "summary", "user_id", "active")
	if err != nil {
		return nil, err
	}
	typeIDs := []int64{}
	userIDs := []int64{}
	attachmentIDs := []int64{}
	for _, row := range rows {
		typeIDs = append(typeIDs, int64FromAny(row["activity_type_id"]))
		userIDs = append(userIDs, int64FromAny(row["user_id"]))
		attachmentIDs = append(attachmentIDs, int64SliceFromAny(row["attachment_ids"])...)
	}
	types, err := activityTypeRowsByID(systemEnv, typeIDs)
	if err != nil {
		return nil, err
	}
	templateIDs := []int64{}
	for _, row := range types {
		templateIDs = append(templateIDs, int64SliceFromAny(row["mail_template_ids"])...)
	}
	templates, err := activityTemplateRowsByID(systemEnv, templateIDs)
	if err != nil {
		return nil, err
	}
	attachments, err := activityAttachmentRowsByID(systemEnv, attachmentIDs)
	if err != nil {
		return nil, err
	}
	users, partners, err := activityUserPartnerRowsByID(systemEnv, userIDs)
	if err != nil {
		return nil, err
	}
	outRows := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id := int64FromAny(row["id"])
		typeID := int64FromAny(row["activity_type_id"])
		typeRow := types[typeID]
		templateIDs := int64SliceFromAny(typeRow["mail_template_ids"])
		displayName := firstNonEmptyString(stringFromAny(row["summary"]), stringFromAny(typeRow["name"]))
		outRows = append(outRows, map[string]any{
			"id":                id,
			"activity_category": firstNonEmptyString(stringFromAny(row["activity_category"]), stringFromAny(typeRow["category"])),
			"activity_type_id":  typeID,
			"attachment_ids":    uniqueIDs(int64SliceFromAny(row["attachment_ids"])),
			"can_write":         true,
			"chaining_type":     firstNonEmptyString(stringFromAny(row["chaining_type"]), stringFromAny(typeRow["chaining_type"])),
			"date_deadline":     activityDateString(row["date_deadline"]),
			"date_done":         activityDateString(row["date_done"]),
			"display_name":      displayName,
			"icon":              stringFromAny(typeRow["icon"]),
			"mail_template_ids": templateIDs,
			"note":              []any{"markup", stringFromAny(row["note"])},
			"res_id":            int64FromAny(row["res_id"]),
			"res_model":         stringFromAny(row["res_model"]),
			"state":             activityComputedState(env, row),
			"summary":           stringFromAny(row["summary"]),
			"user_id":           int64FromAny(row["user_id"]),
		})
	}
	store := map[string]any{"mail.activity": outRows}
	if len(types) > 0 {
		store["mail.activity.type"] = sortedActivityRelatedRows(types)
	}
	if len(templates) > 0 {
		store["mail.template"] = sortedActivityRelatedRows(templates)
	}
	if len(attachments) > 0 {
		store["ir.attachment"] = sortedActivityRelatedRows(attachments)
	}
	if len(users) > 0 {
		store["res.users"] = sortedActivityRelatedRows(users)
	}
	if len(partners) > 0 {
		store["res.partner"] = sortedActivityRelatedRows(partners)
	}
	return store, nil
}

func GetActivityData(env *record.Env, resModel string, node domain.Node, opts ActivityDataOptions) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail activity requires env")
	}
	resModel = strings.TrimSpace(resModel)
	if resModel == "" {
		return nil, fmt.Errorf("get_activity_data requires res_model")
	}
	if _, ok := env.ModelMetadata(resModel); !ok {
		return nil, fmt.Errorf("unknown model %s", resModel)
	}
	activityTypes, err := activityDataTypes(env, resModel)
	if err != nil {
		return nil, err
	}
	templateIDs := []int64{}
	for _, row := range activityTypes {
		templateIDs = append(templateIDs, int64SliceFromAny(row["mail_template_ids"])...)
	}
	templates, err := activityTemplateRowsByID(activityActiveTestEnv(env, false), templateIDs)
	if err != nil {
		return nil, err
	}
	activityDomain := domain.Cond("res_model", "=", resModel)
	if opts.Limit > 0 || opts.Offset > 0 || !activityEmptyDomain(node) {
		recordIDs, err := env.Model(resModel).SearchWithOptions(node, record.SearchOptions{Offset: opts.Offset, Limit: opts.Limit})
		if err != nil {
			return nil, err
		}
		activityDomain = domain.And(activityDomain, domain.Cond("res_id", "in", recordIDs.IDs()))
	}
	activityEnv := activityActiveTestEnv(env, !opts.FetchDone)
	found, err := activityEnv.Model("mail.activity").SearchWithOptions(activityDomain, record.SearchOptions{Order: "date_done desc, date_deadline asc, id asc"})
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "activity_type_id", "res_id", "user_id", "date_deadline", "date_done", "summary", "active", "attachment_ids")
	if err != nil {
		return nil, err
	}
	return activityDataPayload(env, rows, activityTypes, templates)
}

func ActivityReschedule(env *record.Env, activityIDs []int64, deadline any) error {
	if env == nil {
		return fmt.Errorf("mail activity requires env")
	}
	ids := uniqueIDs(activityIDs)
	if len(ids) == 0 {
		return nil
	}
	return env.Model("mail.activity").Browse(ids...).Write(map[string]any{"date_deadline": activityDateValue(deadline)})
}

func ActivityRescheduleSelected(env *record.Env, req ActivitySelectRequest, deadline any, newUserID int64) ([]int64, bool, error) {
	if env == nil {
		return nil, false, fmt.Errorf("mail activity requires env")
	}
	if skipMailActivityAutomation(env) {
		return nil, false, nil
	}
	activities, ok, err := findActivities(env, req)
	if err != nil || !ok {
		return nil, ok, err
	}
	values := map[string]any{}
	if deadline != nil {
		values["date_deadline"] = activityDateValue(deadline)
	}
	if newUserID != 0 {
		values["user_id"] = newUserID
	}
	ids := activities.IDs()
	if len(values) > 0 && len(ids) > 0 {
		if err := activities.Write(values); err != nil {
			return nil, true, err
		}
	}
	return ids, true, nil
}

func ActivityCancel(env *record.Env, activityIDs []int64) error {
	if env == nil {
		return fmt.Errorf("mail activity requires env")
	}
	ids := uniqueIDs(activityIDs)
	if len(ids) == 0 {
		return nil
	}
	return env.Model("mail.activity").Browse(ids...).Unlink()
}

func activityDataPayload(env *record.Env, rows []map[string]any, activityTypes []map[string]any, templates map[int64]map[string]any) (map[string]any, error) {
	attachmentIDs := []int64{}
	for _, row := range rows {
		if !activityRowActive(row) {
			attachmentIDs = append(attachmentIDs, int64SliceFromAny(row["attachment_ids"])...)
		}
	}
	attachments, err := activityAttachmentRowsByID(activityActiveTestEnv(env, false), attachmentIDs)
	if err != nil {
		return nil, err
	}
	type groupedBucket struct {
		ongoing   []map[string]any
		completed []map[string]any
	}
	groups := map[string]*groupedBucket{}
	for _, row := range rows {
		key := fmt.Sprintf("%d:%d", int64FromAny(row["res_id"]), int64FromAny(row["activity_type_id"]))
		bucket := groups[key]
		if bucket == nil {
			bucket = &groupedBucket{}
			groups[key] = bucket
		}
		if activityRowActive(row) {
			bucket.ongoing = append(bucket.ongoing, row)
		} else {
			bucket.completed = append(bucket.completed, row)
		}
	}
	resIDToDeadline := map[int64]string{}
	resIDToDateDone := map[int64]string{}
	grouped := map[int64]map[int64]map[string]any{}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		bucket := groups[key]
		allRows := append(append([]map[string]any{}, bucket.ongoing...), bucket.completed...)
		if len(allRows) == 0 {
			continue
		}
		resID := int64FromAny(allRows[0]["res_id"])
		typeID := int64FromAny(allRows[0]["activity_type_id"])
		sortActivityRowsByDate(bucket.ongoing, "date_deadline", false)
		sortActivityRowsByDate(bucket.completed, "date_done", true)
		dateDeadline := ""
		if len(bucket.ongoing) > 0 {
			dateDeadline = activityDateString(bucket.ongoing[0]["date_deadline"])
			if dateDeadline != "" && (resIDToDeadline[resID] == "" || dateDeadline < resIDToDeadline[resID]) {
				resIDToDeadline[resID] = dateDeadline
			}
		}
		dateDone := ""
		if len(bucket.completed) > 0 {
			dateDone = activityDateString(bucket.completed[0]["date_done"])
			if dateDone != "" && (resIDToDateDone[resID] == "" || dateDone > resIDToDateDone[resID]) {
				resIDToDateDone[resID] = dateDone
			}
		}
		countByState := map[string]int{}
		for _, row := range bucket.ongoing {
			countByState[activityComputedState(env, row)]++
		}
		if len(bucket.completed) > 0 {
			countByState["done"] = len(bucket.completed)
		}
		ids := make([]int64, 0, len(allRows))
		summaries := make([]string, 0, len(allRows))
		for _, row := range allRows {
			ids = append(ids, int64FromAny(row["id"]))
			summaries = append(summaries, stringFromAny(row["summary"]))
		}
		userIDs := []int64{}
		for _, row := range bucket.ongoing {
			if id := int64FromAny(row["user_id"]); id != 0 {
				userIDs = append(userIDs, id)
			}
		}
		item := map[string]any{
			"count_by_state":    countByState,
			"ids":               uniqueIDs(ids),
			"reporting_date":    firstNonEmptyString(dateDeadline, dateDone),
			"state":             "done",
			"user_assigned_ids": uniqueIDs(userIDs),
			"summaries":         summaries,
		}
		if len(bucket.ongoing) > 0 {
			item["state"] = activityComputedState(env, bucket.ongoing[0])
		}
		if info := completedActivityAttachmentsInfo(bucket.completed, attachments); len(info) > 0 {
			item["attachments_info"] = info
		}
		if grouped[resID] == nil {
			grouped[resID] = map[int64]map[string]any{}
		}
		grouped[resID][typeID] = item
	}
	return map[string]any{
		"activity_res_ids":   activityDataOrderedResIDs(resIDToDeadline, resIDToDateDone),
		"activity_types":     activityDataTypePayload(activityTypes, templates),
		"grouped_activities": grouped,
	}, nil
}

func activityDataTypes(env *record.Env, resModel string) ([]map[string]any, error) {
	found, err := activityActiveTestEnv(env, false).Model("mail.activity.type").SearchWithOptions(domain.Or(
		domain.Cond("res_model", "=", resModel),
		domain.Cond("res_model", "=", ""),
		domain.Cond("res_model", "=", false),
	), record.SearchOptions{Order: "id asc"})
	if err != nil {
		return nil, err
	}
	return found.Read("id", "name", "mail_template_ids")
}

func activityDataTypePayload(rows []map[string]any, templates map[int64]map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"id":           int64FromAny(row["id"]),
			"name":         stringFromAny(row["name"]),
			"template_ids": activityTemplateTupleRows(row["mail_template_ids"], templates),
		})
	}
	return out
}

func activityTemplateTupleRows(value any, templates map[int64]map[string]any) []map[string]any {
	ids := uniqueIDs(int64SliceFromAny(value))
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]any{"id": id, "name": stringFromAny(templates[id]["name"])})
	}
	return out
}

func completedActivityAttachmentsInfo(rows []map[string]any, attachments map[int64]map[string]any) map[string]any {
	ids := []int64{}
	for _, row := range rows {
		ids = append(ids, int64SliceFromAny(row["attachment_ids"])...)
	}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool {
		left := attachments[ids[i]]
		right := attachments[ids[j]]
		leftDate := activityDateTimeSortKey(left["create_date"])
		rightDate := activityDateTimeSortKey(right["create_date"])
		if leftDate.Equal(rightDate) {
			return ids[i] > ids[j]
		}
		return leftDate.After(rightDate)
	})
	mostRecent := attachments[ids[0]]
	return map[string]any{
		"count":            len(ids),
		"most_recent_id":   ids[0],
		"most_recent_name": stringFromAny(mostRecent["name"]),
	}
}

func activityDataOrderedResIDs(deadlines map[int64]string, doneDates map[int64]string) []int64 {
	ongoing := make([]int64, 0, len(deadlines))
	for id := range deadlines {
		ongoing = append(ongoing, id)
	}
	sort.Slice(ongoing, func(i, j int) bool {
		left := deadlines[ongoing[i]]
		right := deadlines[ongoing[j]]
		if left == right {
			return ongoing[i] < ongoing[j]
		}
		return left < right
	})
	completed := []int64{}
	for id := range doneDates {
		if deadlines[id] == "" {
			completed = append(completed, id)
		}
	}
	sort.Slice(completed, func(i, j int) bool {
		left := doneDates[completed[i]]
		right := doneDates[completed[j]]
		if left == right {
			return completed[i] < completed[j]
		}
		return left > right
	})
	return append(ongoing, completed...)
}

func sortActivityRowsByDate(rows []map[string]any, field string, desc bool) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := activityDateTimeSortKey(rows[i][field])
		right := activityDateTimeSortKey(rows[j][field])
		if left.Equal(right) {
			return int64FromAny(rows[i]["id"]) < int64FromAny(rows[j]["id"])
		}
		if desc {
			return left.After(right)
		}
		return left.Before(right)
	})
}

func activityComputedState(env *record.Env, row map[string]any) string {
	if !activityRowActive(row) {
		return "done"
	}
	deadline, ok := parseActivityDate(activityDateString(row["date_deadline"]))
	if !ok {
		return firstNonEmptyString(stringFromAny(row["state"]), "planned")
	}
	now := time.Now().UTC()
	if env != nil {
		if fixed, ok := env.Context().Values["mail_activity_today"]; ok {
			if parsed, parsedOK := parseActivityDate(activityDateString(fixed)); parsedOK {
				now = parsed
			}
		}
	}
	today := now.UTC().Format("2006-01-02")
	due := deadline.UTC().Format("2006-01-02")
	switch {
	case due == today:
		return "today"
	case due < today:
		return "overdue"
	default:
		return "planned"
	}
}

func activityRowActive(row map[string]any) bool {
	if value, ok := row["active"]; ok {
		return boolFromAny(value)
	}
	return stringFromAny(row["state"]) != "done"
}

func activityDateString(value any) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC().Format("2006-01-02")
	case string:
		text := strings.TrimSpace(typed)
		if len(text) >= len("2006-01-02") {
			return text[:len("2006-01-02")]
		}
	}
	return ""
}

func activityDateTimeSortKey(value any) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC()
	case string:
		text := strings.TrimSpace(typed)
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
			if parsed, err := time.Parse(layout, text); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func activityEmptyDomain(node domain.Node) bool {
	return node.Kind == "" || (node.Kind == domain.All && len(node.Children) == 0)
}

func activityActiveTestEnv(env *record.Env, activeTest bool) *record.Env {
	if env == nil {
		return env
	}
	ctx := env.Context()
	values := map[string]any{}
	for key, value := range ctx.Values {
		values[key] = value
	}
	values["active_test"] = activeTest
	ctx.Values = values
	return env.WithContext(ctx)
}

func activityTypeRowsByID(env *record.Env, ids []int64) (map[int64]map[string]any, error) {
	out := map[int64]map[string]any{}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := env.Model("mail.activity.type").Browse(ids...).Read("id", "name", "category", "chaining_type", "mail_template_ids", "icon")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[int64FromAny(row["id"])] = row
	}
	return out, nil
}

func activityTemplateRowsByID(env *record.Env, ids []int64) (map[int64]map[string]any, error) {
	out := map[int64]map[string]any{}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := env.Model("mail.template").Browse(ids...).Read("id", "name")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[int64FromAny(row["id"])] = row
	}
	return out, nil
}

func activityAttachmentRowsByID(env *record.Env, ids []int64) (map[int64]map[string]any, error) {
	out := map[int64]map[string]any{}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := env.Model("ir.attachment").Browse(ids...).Read("id", "name", "create_date")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[int64FromAny(row["id"])] = row
	}
	return out, nil
}

func activityUserPartnerRowsByID(env *record.Env, ids []int64) (map[int64]map[string]any, map[int64]map[string]any, error) {
	users := map[int64]map[string]any{}
	partners := map[int64]map[string]any{}
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return users, partners, nil
	}
	rows, err := env.Model("res.users").Browse(ids...).Read("id", "name", "partner_id")
	if err != nil {
		return nil, nil, err
	}
	partnerIDs := []int64{}
	for _, row := range rows {
		id := int64FromAny(row["id"])
		partnerID := int64FromAny(row["partner_id"])
		users[id] = map[string]any{"id": id, "name": stringFromAny(row["name"]), "partner_id": partnerID}
		partnerIDs = append(partnerIDs, partnerID)
	}
	partnerIDs = uniqueIDs(partnerIDs)
	if len(partnerIDs) == 0 {
		return users, partners, nil
	}
	partnerRows, err := env.Model("res.partner").Browse(partnerIDs...).Read("id", "name")
	if err != nil {
		return nil, nil, err
	}
	for _, row := range partnerRows {
		partners[int64FromAny(row["id"])] = row
	}
	return users, partners, nil
}

func sortedActivityRelatedRows(rows map[int64]map[string]any) []map[string]any {
	ids := make([]int64, 0, len(rows))
	for id := range rows {
		if id != 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, rows[id])
	}
	return out
}

func completeActivities(env *record.Env, activityIDs []int64, feedback string, attachmentIDs []int64) (ActivityDoneResult, error) {
	if env == nil {
		return ActivityDoneResult{}, fmt.Errorf("mail activity requires env")
	}
	activityIDs = uniqueIDs(activityIDs)
	if len(activityIDs) == 0 {
		return ActivityDoneResult{}, nil
	}
	rows, err := activityRows(env, activityIDs)
	if err != nil {
		return ActivityDoneResult{}, err
	}
	activityAttachments, err := activityAttachmentsByActivity(env, activityIDs)
	if err != nil {
		return ActivityDoneResult{}, err
	}
	result := ActivityDoneResult{}
	now := time.Now().UTC()
	completedIDs := make([]int64, 0, len(rows))
	removedIDs := []int64{}
	for _, row := range rows {
		activityID := int64FromAny(row["id"])
		targetExists, err := activityTargetExists(env, stringFromAny(row["res_model"]), int64FromAny(row["res_id"]))
		if err != nil {
			return ActivityDoneResult{}, err
		}
		if !targetExists {
			removedIDs = append(removedIDs, activityID)
			if err := unlinkActivityAttachments(env, activityAttachments[activityID]); err != nil {
				return ActivityDoneResult{}, err
			}
			continue
		}
		messageID, err := PostMessage(env, PostRequest{
			Model:              stringFromAny(row["res_model"]),
			ResID:              int64FromAny(row["res_id"]),
			Body:               activityDoneMessageBody(env, row, feedback),
			MessageType:        "notification",
			SubtypeXMLID:       "mail.mt_activities",
			MailActivityTypeID: int64FromAny(row["activity_type_id"]),
			AttachmentIDs:      attachmentIDs,
			BodyIsHTML:         true,
		})
		if err != nil {
			return ActivityDoneResult{}, err
		}
		if messageID != 0 {
			result.MessageIDs = append(result.MessageIDs, messageID)
			if err := moveActivityAttachmentsToMessage(env, messageID, activityAttachments[activityID], attachmentIDs); err != nil {
				return ActivityDoneResult{}, err
			}
		}
		nextID, err := createTriggeredNextActivity(env, row)
		if err != nil {
			return ActivityDoneResult{}, err
		}
		if nextID != 0 {
			result.NextActivityIDs = append(result.NextActivityIDs, nextID)
		}
		completedIDs = append(completedIDs, activityID)
	}
	if len(removedIDs) > 0 {
		if err := env.Model("mail.activity").Browse(removedIDs...).Unlink(); err != nil {
			return ActivityDoneResult{}, err
		}
	}
	if len(completedIDs) == 0 {
		return result, nil
	}
	values := map[string]any{"state": "done", "active": false, "date_done": now}
	if strings.TrimSpace(feedback) != "" {
		values["feedback"] = feedback
	}
	if ids := uniqueIDs(attachmentIDs); len(ids) > 0 {
		values["attachment_ids"] = ids
	}
	if err := env.Model("mail.activity").Browse(completedIDs...).Write(values); err != nil {
		return ActivityDoneResult{}, err
	}
	return result, nil
}

func activityRows(env *record.Env, activityIDs []int64) ([]map[string]any, error) {
	activityIDs = uniqueIDs(activityIDs)
	if len(activityIDs) == 0 {
		return nil, nil
	}
	return env.Model("mail.activity").Browse(activityIDs...).Read("id", "res_model", "res_id", "activity_type_id", "user_id", "date_deadline", "summary", "note")
}

func activityDoneMessageBody(env *record.Env, activity map[string]any, feedback string) string {
	activityType := activityTypeRow(env, int64FromAny(activity["activity_type_id"]))
	var b strings.Builder
	b.WriteString(`<div><p><span class="fa`)
	if icon := strings.TrimSpace(stringFromAny(activityType["icon"])); icon != "" {
		b.WriteString(" ")
		b.WriteString(html.EscapeString(icon))
	}
	b.WriteString(` fa-fw"></span><span>`)
	b.WriteString(html.EscapeString(firstNonEmptyString(stringFromAny(activityType["name"]), "Activity")))
	b.WriteString(`</span> done`)
	if assigneeID := int64FromAny(activity["user_id"]); assigneeID != 0 && assigneeID != env.Context().UserID {
		b.WriteString(` (originally assigned to `)
		b.WriteString(html.EscapeString(activityUserName(env, assigneeID)))
		b.WriteString(`)`)
	}
	if summary := strings.TrimSpace(stringFromAny(activity["summary"])); summary != "" {
		b.WriteString(`: `)
		b.WriteString(html.EscapeString(summary))
	}
	b.WriteString(`</p>`)
	note := strings.TrimSpace(stringFromAny(activity["note"]))
	if note != "" && note != `<p><br></p>` && note != `&lt;p&gt;&lt;br&gt;&lt;/p&gt;` {
		b.WriteString(`<div class="o_mail_note_title fw-bold">Original note:</div><div>`)
		b.WriteString(note)
		b.WriteString(`</div>`)
	}
	if feedback != "" {
		b.WriteString(`<div><div class="fw-bold">Feedback:</div>`)
		lines := strings.Split(feedback, "\n")
		for idx, line := range lines {
			b.WriteString(html.EscapeString(line))
			if idx < len(lines)-1 {
				b.WriteString(`<br/>`)
			}
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func activityUserName(env *record.Env, userID int64) string {
	if userID == 0 {
		return ""
	}
	rows, err := messageSystemEnv(env).Model("res.users").Browse(userID).Read("name")
	if err != nil || len(rows) == 0 {
		return ""
	}
	return stringFromAny(rows[0]["name"])
}

func activityTargetExists(env *record.Env, modelName string, resID int64) (bool, error) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || resID == 0 {
		return false, nil
	}
	if _, ok := env.ModelMetadata(modelName); !ok {
		return false, fmt.Errorf("unknown model %s", modelName)
	}
	rows, err := messageSystemEnv(env).Model(modelName).Browse(resID).Read("id")
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func activityAttachmentsByActivity(env *record.Env, activityIDs []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	activityIDs = uniqueIDs(activityIDs)
	if len(activityIDs) == 0 {
		return out, nil
	}
	found, err := messageSystemEnv(env).Model("ir.attachment").Search(domain.And(
		domain.Cond("res_model", "=", "mail.activity"),
		domain.Cond("res_id", "in", activityIDs),
	))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "res_id")
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		activityID := int64FromAny(row["res_id"])
		attachmentID := int64FromAny(row["id"])
		if activityID != 0 && attachmentID != 0 {
			out[activityID] = uniqueIDs(append(out[activityID], attachmentID))
		}
	}
	return out, nil
}

func moveActivityAttachmentsToMessage(env *record.Env, messageID int64, activityAttachmentIDs []int64, directAttachmentIDs []int64) error {
	activityAttachmentIDs = uniqueIDs(activityAttachmentIDs)
	if messageID == 0 || len(activityAttachmentIDs) == 0 {
		return nil
	}
	systemEnv := messageSystemEnv(env)
	for _, attachmentID := range activityAttachmentIDs {
		if err := systemEnv.Model("ir.attachment").Browse(attachmentID).Write(map[string]any{
			"res_model": "mail.message",
			"res_id":    messageID,
		}); err != nil {
			return err
		}
	}
	rows, err := systemEnv.Model("mail.message").Browse(messageID).Read("attachment_ids")
	if err != nil {
		return err
	}
	attachmentIDs := uniqueIDs(append(append([]int64{}, directAttachmentIDs...), activityAttachmentIDs...))
	if len(rows) > 0 {
		attachmentIDs = uniqueIDs(append(int64SliceFromAny(rows[0]["attachment_ids"]), attachmentIDs...))
	}
	return systemEnv.Model("mail.message").Browse(messageID).Write(map[string]any{"attachment_ids": attachmentIDs})
}

func unlinkActivityAttachments(env *record.Env, attachmentIDs []int64) error {
	attachmentIDs = uniqueIDs(attachmentIDs)
	if len(attachmentIDs) == 0 {
		return nil
	}
	return messageSystemEnv(env).Model("ir.attachment").Browse(attachmentIDs...).Unlink()
}

func createTriggeredNextActivity(env *record.Env, activity map[string]any) (int64, error) {
	activityType := activityTypeRow(env, int64FromAny(activity["activity_type_id"]))
	if strings.TrimSpace(stringFromAny(activityType["chaining_type"])) != "trigger" {
		return 0, nil
	}
	nextTypeID := int64FromAny(activityType["triggered_next_type_id"])
	if nextTypeID == 0 {
		return 0, nil
	}
	nextType := activityTypeRow(env, nextTypeID)
	return env.Model("mail.activity").Create(map[string]any{
		"activity_type_id":           nextTypeID,
		"activity_category":          firstNonEmptyString(stringFromAny(nextType["category"]), "default"),
		"previous_activity_type_id":  int64FromAny(activity["activity_type_id"]),
		"has_recommended_activities": activityTypeHasRecommendations(env, int64FromAny(activity["activity_type_id"])),
		"chaining_type":              firstNonEmptyString(stringFromAny(nextType["chaining_type"]), "suggest"),
		"res_model":                  stringFromAny(activity["res_model"]),
		"res_id":                     int64FromAny(activity["res_id"]),
		"user_id":                    firstNonZero(int64FromAny(nextType["default_user_id"]), int64FromAny(activity["user_id"]), env.Context().UserID),
		"date_deadline":              nextActivityDeadline(activity, nextType),
		"summary":                    firstNonEmptyString(stringFromAny(nextType["summary"]), stringFromAny(nextType["name"])),
		"note":                       firstNonEmptyString(stringFromAny(nextType["default_note"])),
		"state":                      "open",
		"automated":                  false,
		"active":                     true,
	})
}

func activityScheduleNextContext(activity map[string]any) map[string]any {
	return map[string]any{
		"default_previous_activity_type_id": int64FromAny(activity["activity_type_id"]),
		"activity_previous_deadline":        stringFromAny(activity["date_deadline"]),
		"default_res_id":                    int64FromAny(activity["res_id"]),
		"default_res_model":                 stringFromAny(activity["res_model"]),
	}
}

func nextActivityDeadline(activity map[string]any, nextType map[string]any) string {
	base := time.Now().UTC()
	if strings.TrimSpace(stringFromAny(nextType["delay_from"])) == "previous_activity" {
		if parsed, ok := parseActivityDate(stringFromAny(activity["date_deadline"])); ok {
			base = parsed
		}
	}
	count := int(int64FromAny(nextType["delay_count"]))
	switch strings.TrimSpace(stringFromAny(nextType["delay_unit"])) {
	case "weeks":
		return base.AddDate(0, 0, count*7).Format("2006-01-02")
	case "months":
		return base.AddDate(0, count, 0).Format("2006-01-02")
	default:
		return base.AddDate(0, 0, count).Format("2006-01-02")
	}
}

func parseActivityDate(value string) (time.Time, bool) {
	if len(value) < len("2006-01-02") {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value[:len("2006-01-02")])
	return parsed, err == nil
}

func ActivityUnlink(env *record.Env, req ActivitySelectRequest) (bool, error) {
	if env == nil {
		return false, fmt.Errorf("mail activity requires env")
	}
	if skipMailActivityAutomation(env) {
		return false, nil
	}
	activities, ok, err := findActivities(env, req)
	if err != nil || !ok {
		return ok, err
	}
	return true, activities.Unlink()
}

func findActivities(env *record.Env, req ActivitySelectRequest) (record.RecordSet, bool, error) {
	typeIDs := activityTypeIDs(env, req.ActivityTypeXMLIDs)
	if len(typeIDs) == 0 {
		return record.RecordSet{}, false, nil
	}
	node := domain.And(
		domain.Cond("res_model", "=", strings.TrimSpace(req.Model)),
		domain.Cond("res_id", "in", uniqueIDs(req.ResIDs)),
		domain.Cond("activity_type_id", "in", typeIDs),
		domain.Cond("state", "!=", "done"),
	)
	if req.UserID != 0 {
		node = domain.And(node, domain.Cond("user_id", "=", req.UserID))
	}
	if req.OnlyAutomated {
		node = domain.And(node, domain.Cond("automated", "=", true))
	}
	found, err := env.Model("mail.activity").Search(node)
	return found, true, err
}

func activityTypeIDs(env *record.Env, xmlIDs []string) []int64 {
	out := make([]int64, 0, len(xmlIDs))
	for _, xmlID := range xmlIDs {
		if id := resolveXMLID(env, xmlID, "mail.activity.type"); id != 0 {
			out = append(out, id)
		}
	}
	return uniqueIDs(out)
}

func activityTypeRow(env *record.Env, id int64) map[string]any {
	if id == 0 {
		return nil
	}
	rows, err := env.Model("mail.activity.type").Browse(id).Read("name", "summary", "default_note", "default_user_id", "res_model", "category", "delay_count", "delay_unit", "delay_from", "chaining_type", "triggered_next_type_id", "suggested_next_type_ids", "previous_type_ids", "mail_template_ids", "icon")
	if err != nil || len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func activityTypeHasRecommendations(env *record.Env, activityTypeID int64) bool {
	if activityTypeID == 0 {
		return false
	}
	return len(int64SliceFromAny(activityTypeRow(env, activityTypeID)["suggested_next_type_ids"])) > 0
}

func defaultActivityTypeID(env *record.Env) int64 {
	found, err := env.Model("mail.activity.type").Search(domain.And())
	if err != nil {
		return 0
	}
	rows, err := found.Read("id")
	if err != nil || len(rows) == 0 {
		return 0
	}
	return int64FromAny(rows[0]["id"])
}

func invalidActivityTypeModel(row map[string]any, modelName string) bool {
	if row == nil {
		return true
	}
	resModel := strings.TrimSpace(stringFromAny(row["res_model"]))
	return resModel != "" && resModel != modelName
}

func activityDateValue(value any) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC().Format("2006-01-02")
	case string:
		text := strings.TrimSpace(typed)
		if len(text) >= len("2006-01-02") {
			return text[:len("2006-01-02")]
		}
	}
	return time.Now().UTC().Format("2006-01-02")
}

func skipMailActivityAutomation(env *record.Env) bool {
	return boolFromAny(env.Context().Values["mail_activity_automation_skip"])
}

func firstNonZero(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func boolFromAny(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes":
			return true
		}
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	}
	return false
}
