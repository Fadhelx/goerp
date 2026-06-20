package mail

import (
	"fmt"
	netmail "net/mail"
	"sort"
	"strings"
	"sync"

	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/record"
)

type SuggestedRecipientsRequest struct {
	Model                string
	ResID                int64
	MessageID            int64
	ReplyDiscussion      bool
	NoCreate             bool
	PrimaryEmail         string
	AdditionalPartnerIDs []int64
}

type SuggestedRecipientHook struct {
	PartnerFields           []string
	PrimaryEmailField       string
	SelfPartner             bool
	IncludeEmailCC          bool
	CreationSubtypeXMLIDs   []string
	DisablePartnerHeuristic bool
	DisableEmailHeuristic   bool
}

var (
	suggestedRecipientHooksMu sync.RWMutex
	suggestedRecipientHooks   = map[string]SuggestedRecipientHook{}
)

func RegisterSuggestedRecipientHook(modelName string, hook SuggestedRecipientHook) func() {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return func() {}
	}
	hook.PartnerFields = cleanSuggestedStrings(hook.PartnerFields)
	hook.PrimaryEmailField = strings.TrimSpace(hook.PrimaryEmailField)
	hook.CreationSubtypeXMLIDs = cleanSuggestedStrings(hook.CreationSubtypeXMLIDs)
	suggestedRecipientHooksMu.Lock()
	previous, hadPrevious := suggestedRecipientHooks[modelName]
	suggestedRecipientHooks[modelName] = hook
	suggestedRecipientHooksMu.Unlock()
	return func() {
		suggestedRecipientHooksMu.Lock()
		defer suggestedRecipientHooksMu.Unlock()
		if hadPrevious {
			suggestedRecipientHooks[modelName] = previous
			return
		}
		delete(suggestedRecipientHooks, modelName)
	}
}

func SuggestedRecipientFields(env *record.Env, modelName string) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail recipients fields require env")
	}
	modelName = strings.TrimSpace(modelName)
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil, fmt.Errorf("unknown model %s", modelName)
	}
	hook := suggestedRecipientHook(modelName)
	partnerFields := mailPartnerFieldsForModel(modelName, meta, hook)
	primaryEmail := mailPrimaryEmailFieldForModel(meta, hook)
	var primary any
	if primaryEmail != "" {
		primary = primaryEmail
	}
	return map[string]any{
		"partner_fields":      partnerFields,
		"primary_email_field": []any{primary},
	}, nil
}

func SuggestedRecipients(env *record.Env, req SuggestedRecipientsRequest) ([]map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail suggested recipients require env")
	}
	req.Model = strings.TrimSpace(req.Model)
	if req.Model == "" || req.ResID == 0 {
		return nil, fmt.Errorf("mail suggested recipients require model and record id")
	}
	if err := ensureRecordExists(env, req.Model, req.ResID); err != nil {
		return nil, err
	}
	systemEnv := messageSystemEnv(env)
	defaultPartnerIDs, err := recordSuggestedPartnerIDs(systemEnv, req.Model, req.ResID)
	if err != nil {
		return nil, err
	}
	additionalPartnerIDs := existingPartnerIDs(systemEnv, req.AdditionalPartnerIDs)
	partnerIDs := uniqueIDs(append(defaultPartnerIDs, additionalPartnerIDs...))
	emailInputs, err := recordSuggestedEmailInputs(systemEnv, req.Model, req.ResID, req.PrimaryEmail)
	if err != nil {
		return nil, err
	}
	if messageRow, ok, err := suggestedReplyMessage(systemEnv, req); err != nil {
		return nil, err
	} else if ok {
		partnerIDs = append(partnerIDs, int64s(messageRow["partner_ids"])...)
		if authorID := int64FromAny(messageRow["author_id"]); authorID != 0 {
			partnerIDs = append(partnerIDs, authorID)
		}
		emailInputs = append(emailInputs,
			stringAny(messageRow["incoming_email_to"]),
			stringAny(messageRow["incoming_email_cc"]),
			stringAny(messageRow["email_from"]),
		)
	}
	return buildSuggestedRecipients(systemEnv, req, defaultPartnerIDs, additionalPartnerIDs, partnerIDs, emailInputs)
}

func recordSuggestedPartnerIDs(env *record.Env, modelName string, resID int64) ([]int64, error) {
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil, fmt.Errorf("unknown model %s", modelName)
	}
	hook := suggestedRecipientHook(modelName)
	fields := append([]string{}, mailPartnerFieldsForModel(modelName, meta, hook)...)
	if userField, ok := meta.Fields["user_id"]; ok && userField.Kind == field.Many2One && userField.Relation == "res.users" {
		fields = append(fields, "user_id")
	}
	if hook.SelfPartner && modelName == "res.partner" {
		fields = append(fields, "id")
	}
	if len(fields) == 0 {
		return nil, nil
	}
	rows, err := env.Model(modelName).Browse(resID).Read(fields...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s:%d not found", modelName, resID)
	}
	out := []int64{}
	if hook.SelfPartner && modelName == "res.partner" {
		out = append(out, resID)
	}
	for _, fieldName := range mailPartnerFieldsForModel(modelName, meta, hook) {
		out = append(out, int64s(rows[0][fieldName])...)
	}
	if userID := int64FromAny(rows[0]["user_id"]); userID != 0 {
		out = append(out, currentUserPartnerID(env, userID))
	}
	return uniqueIDs(out), nil
}

func recordSuggestedEmailInputs(env *record.Env, modelName string, resID int64, primaryEmail string) ([]string, error) {
	if strings.TrimSpace(primaryEmail) != "" {
		return []string{primaryEmail}, nil
	}
	meta, ok := env.ModelMetadata(modelName)
	if !ok {
		return nil, fmt.Errorf("unknown model %s", modelName)
	}
	toFields := []string{}
	hook := suggestedRecipientHook(modelName)
	if primaryField := mailPrimaryEmailFieldForModel(meta, hook); primaryField != "" {
		toFields = append(toFields, primaryField)
	}
	if !hook.DisableEmailHeuristic {
		for _, fieldName := range []string{"email_from", "x_email_from", "email", "x_email", "partner_email", "email_normalized", "work_email"} {
			if _, ok := meta.Fields[fieldName]; ok && !containsSuggestedString(toFields, fieldName) {
				toFields = append(toFields, fieldName)
			}
		}
	}
	ccFields := []string{}
	if hook.IncludeEmailCC {
		for _, fieldName := range []string{"email_cc", "partner_email_cc", "x_email_cc"} {
			if _, ok := meta.Fields[fieldName]; ok {
				ccFields = append(ccFields, fieldName)
			}
		}
	}
	fieldNames := append(append([]string{}, toFields...), ccFields...)
	if len(fieldNames) == 0 {
		return nil, nil
	}
	rows, err := env.Model(modelName).Browse(resID).Read(fieldNames...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s:%d not found", modelName, resID)
	}
	out := []string{}
	for _, fieldName := range toFields {
		if text := strings.TrimSpace(stringAny(rows[0][fieldName])); text != "" {
			out = append(out, text)
			break
		}
	}
	for _, fieldName := range ccFields {
		if text := strings.TrimSpace(stringAny(rows[0][fieldName])); text != "" {
			out = append(out, text)
			break
		}
	}
	return out, nil
}

func suggestedReplyMessage(env *record.Env, req SuggestedRecipientsRequest) (map[string]any, bool, error) {
	fields := []string{"id", "model", "res_id", "message_type", "author_id", "partner_ids", "incoming_email_to", "incoming_email_cc", "email_from", "date", "subtype_id"}
	if req.MessageID != 0 {
		rows, err := env.Model("mail.message").Browse(req.MessageID).Read(fields...)
		if err != nil {
			return nil, false, err
		}
		if len(rows) == 0 {
			return nil, false, fmt.Errorf("mail.message:%d not found", req.MessageID)
		}
		return rows[0], true, nil
	}
	if !req.ReplyDiscussion {
		return nil, false, nil
	}
	found, err := env.Model("mail.message").Search(domain.And(
		domain.Cond("model", "=", req.Model),
		domain.Cond("res_id", "=", req.ResID),
	))
	if err != nil {
		return nil, false, err
	}
	rows, err := found.Read(fields...)
	if err != nil {
		return nil, false, err
	}
	allowedSubtypeIDs := suggestedReplySubtypeIDs(env, req.Model)
	filtered := rows[:0]
	for _, row := range rows {
		switch strings.TrimSpace(stringAny(row["message_type"])) {
		case "comment", "email":
			subtypeID := int64FromAny(row["subtype_id"])
			if len(allowedSubtypeIDs) > 0 && !allowedSubtypeIDs[subtypeID] {
				continue
			}
			filtered = append(filtered, row)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		left := timeValue(filtered[i]["date"])
		right := timeValue(filtered[j]["date"])
		if !left.Equal(right) {
			return left.After(right)
		}
		return int64FromAny(filtered[i]["id"]) > int64FromAny(filtered[j]["id"])
	})
	if len(filtered) == 0 {
		return nil, false, nil
	}
	return filtered[0], true, nil
}

func buildSuggestedRecipients(env *record.Env, req SuggestedRecipientsRequest, defaultPartnerIDs []int64, additionalPartnerIDs []int64, partnerIDs []int64, emailInputs []string) ([]map[string]any, error) {
	defaultPartners := idSet(defaultPartnerIDs)
	additionalPartners := idSet(additionalPartnerIDs)
	followerIDs := followerPartnerIDs(env, req.Model, req.ResID)
	followers := idSet(followerIDs)
	bannedEmails := suggestedBannedEmails(env, emailInputs)
	partnerRows := partnerRowsByID(env, uniqueIDs(append(partnerIDs, followerIDs...)))
	existingEmails := map[string]bool{}
	for _, row := range partnerRows {
		if email := partnerRowEmail(row); email != "" {
			existingEmails[email] = true
		}
	}
	rawRecipients := []map[string]any{}
	for _, item := range suggestedEmailsFromInputs(emailInputs) {
		if item.Email == "" || bannedEmails[item.Email] || existingEmails[item.Email] {
			continue
		}
		row, found, err := partnerFromEmailAddress(env, item.Address, !req.NoCreate)
		if err != nil {
			return nil, err
		}
		if found {
			partnerID := int64FromAny(row["id"])
			partnerIDs = append(partnerIDs, partnerID)
			partnerRows[partnerID] = row
			if email := partnerRowEmail(row); email != "" {
				existingEmails[email] = true
			}
			continue
		}
		if !req.NoCreate {
			continue
		}
		existingEmails[item.Email] = true
		rawRecipients = append(rawRecipients, map[string]any{
			"email":      item.Email,
			"name":       item.Name,
			"partner_id": int64(0),
		})
	}
	out := []map[string]any{}
	for _, partnerID := range uniqueIDs(partnerIDs) {
		if followers[partnerID] && !defaultPartners[partnerID] && !additionalPartners[partnerID] {
			continue
		}
		row := partnerRows[partnerID]
		if row == nil {
			rows := partnerRowsByID(env, []int64{partnerID})
			row = rows[partnerID]
		}
		if row == nil {
			continue
		}
		if active, ok := row["active"]; ok && active != nil && !boolAny(active) {
			continue
		}
		email := partnerRowEmail(row)
		if email == "" || bannedEmails[email] {
			continue
		}
		out = append(out, map[string]any{
			"email":      email,
			"name":       stringAny(row["name"]),
			"partner_id": partnerID,
		})
	}
	out = append(out, rawRecipients...)
	return out, nil
}

type suggestedEmail struct {
	Address *netmail.Address
	Name    string
	Email   string
}

func suggestedEmailsFromInputs(inputs []string) []suggestedEmail {
	out := []suggestedEmail{}
	seen := map[string]bool{}
	replacer := strings.NewReplacer(";", ",", "\n", ",")
	for _, input := range inputs {
		for _, address := range normalizeEmails([]string{replacer.Replace(input)}) {
			email := normalizedEmailAddress(address.Address)
			if email == "" || seen[email] {
				continue
			}
			seen[email] = true
			name := strings.TrimSpace(address.Name)
			if name == "" {
				name = email
			}
			out = append(out, suggestedEmail{Address: address, Name: name, Email: email})
		}
	}
	return out
}

func suggestedBannedEmails(env *record.Env, inputs []string) map[string]bool {
	candidates := []string{}
	for _, item := range suggestedEmailsFromInputs(inputs) {
		candidates = append(candidates, item.Email)
	}
	out := inboundAliasEmails(env, candidates)
	if rootID := resolveXMLID(env, "base.partner_root", "res.partner"); rootID != 0 {
		for _, row := range partnerRowsByID(env, []int64{rootID}) {
			if email := partnerRowEmail(row); email != "" {
				out[email] = true
			}
		}
	}
	return out
}

func partnerFromEmailAddress(env *record.Env, address *netmail.Address, createMissing bool) (map[string]any, bool, error) {
	if env == nil || address == nil {
		return nil, false, nil
	}
	email := normalizedEmailAddress(address.Address)
	if email == "" {
		return nil, false, nil
	}
	rows, err := findPartnerRowsByEmail(env, email)
	if err != nil {
		return nil, false, err
	}
	if len(rows) > 0 {
		return rows[0], true, nil
	}
	if !createMissing {
		return nil, false, nil
	}
	name := strings.TrimSpace(address.Name)
	if name == "" {
		name = email
	}
	values := map[string]any{"name": name, "email": email, "active": true}
	if meta, ok := env.ModelMetadata("res.partner"); ok {
		if _, ok := meta.Fields["email_normalized"]; ok {
			values["email_normalized"] = email
		}
	}
	id, err := env.Model("res.partner").Create(values)
	if err != nil {
		return nil, false, err
	}
	return map[string]any{"id": id, "name": name, "email": email, "email_normalized": email, "active": true}, true, nil
}

func findPartnerRowsByEmail(env *record.Env, email string) ([]map[string]any, error) {
	email = normalizedEmailAddress(email)
	if email == "" {
		return nil, nil
	}
	found, err := env.Model("res.partner").Search(domain.Or(
		domain.Cond("email_normalized", "=", email),
		domain.Cond("email", "=", email),
		domain.Cond("email", domain.ILike, email),
	))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("name", "email", "email_normalized", "active")
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, row := range rows {
		if partnerRowEmail(row) == email {
			out = append(out, row)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		leftActive := boolAny(out[i]["active"])
		rightActive := boolAny(out[j]["active"])
		if leftActive != rightActive {
			return leftActive
		}
		return int64FromAny(out[i]["id"]) < int64FromAny(out[j]["id"])
	})
	return out, nil
}

func partnerRowsByID(env *record.Env, ids []int64) map[int64]map[string]any {
	out := map[int64]map[string]any{}
	ids = uniqueIDs(ids)
	if env == nil || len(ids) == 0 {
		return out
	}
	rows, err := env.Model("res.partner").Browse(ids...).Read("name", "email", "email_normalized", "active")
	if err != nil {
		return out
	}
	for _, row := range rows {
		if id := int64FromAny(row["id"]); id != 0 {
			out[id] = row
		}
	}
	return out
}

func partnerRowEmail(row map[string]any) string {
	if row == nil {
		return ""
	}
	if email := normalizedEmailAddress(stringAny(row["email_normalized"])); email != "" {
		return email
	}
	return normalizedEmailAddress(stringAny(row["email"]))
}

func existingPartnerIDs(env *record.Env, ids []int64) []int64 {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	rows := partnerRowsByID(env, ids)
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if rows[id] != nil {
			out = append(out, id)
		}
	}
	return out
}

func mailPartnerFieldsForModel(modelName string, meta model.Model, hook SuggestedRecipientHook) []string {
	out := []string{}
	candidates := append([]string{}, hook.PartnerFields...)
	if !hook.DisablePartnerHeuristic {
		candidates = append(candidates, "partner_id", "partner_ids")
	}
	for _, fieldName := range candidates {
		if containsSuggestedString(out, fieldName) {
			continue
		}
		fieldMeta, ok := meta.Fields[fieldName]
		if !ok || fieldMeta.Relation != "res.partner" {
			continue
		}
		if fieldMeta.Kind == field.Many2One || fieldMeta.Kind == field.Many2Many {
			out = append(out, fieldName)
		}
	}
	return out
}

func mailPrimaryEmailFieldForModel(meta model.Model, hook SuggestedRecipientHook) string {
	if hook.PrimaryEmailField != "" {
		if fieldMeta, ok := meta.Fields[hook.PrimaryEmailField]; ok && (fieldMeta.Kind == field.Char || fieldMeta.Kind == field.Text) {
			return hook.PrimaryEmailField
		}
		return ""
	}
	for _, fieldName := range []string{"email", "email_from", "x_email_from", "x_email", "partner_email", "contact_email", "customer_email", "email_normalized"} {
		fieldMeta, ok := meta.Fields[fieldName]
		if ok && (fieldMeta.Kind == field.Char || fieldMeta.Kind == field.Text) {
			return fieldName
		}
	}
	return ""
}

func suggestedRecipientHook(modelName string) SuggestedRecipientHook {
	modelName = strings.TrimSpace(modelName)
	hook := defaultSuggestedRecipientHook(modelName)
	suggestedRecipientHooksMu.RLock()
	registered, ok := suggestedRecipientHooks[modelName]
	suggestedRecipientHooksMu.RUnlock()
	if ok {
		hook = mergeSuggestedRecipientHook(hook, registered)
	}
	return hook
}

func defaultSuggestedRecipientHook(modelName string) SuggestedRecipientHook {
	switch modelName {
	case "res.partner":
		return SuggestedRecipientHook{SelfPartner: true}
	case "hr.employee":
		return SuggestedRecipientHook{
			PartnerFields:     []string{"work_contact_id", "user_partner_id"},
			PrimaryEmailField: "work_email",
		}
	case "crm.lead":
		return SuggestedRecipientHook{PrimaryEmailField: "email_from", IncludeEmailCC: true}
	case "helpdesk.ticket":
		return SuggestedRecipientHook{PrimaryEmailField: "partner_email", IncludeEmailCC: true}
	case "event.track":
		return SuggestedRecipientHook{PrimaryEmailField: "contact_email"}
	case "gamification.goal":
		return SuggestedRecipientHook{PartnerFields: []string{"user_partner_id"}}
	case "loyalty.card":
		return SuggestedRecipientHook{PartnerFields: []string{"source_pos_order_partner_id", "order_id_partner_id"}}
	case "slide.channel":
		return SuggestedRecipientHook{DisablePartnerHeuristic: true}
	case "project.task", "project.update", "hr.applicant", "maintenance.request", "quality.check", "hr.payslip", "mrp.eco", "documents.document":
		return SuggestedRecipientHook{IncludeEmailCC: true}
	default:
		return SuggestedRecipientHook{}
	}
}

func mergeSuggestedRecipientHook(base, override SuggestedRecipientHook) SuggestedRecipientHook {
	if len(override.PartnerFields) > 0 {
		base.PartnerFields = append([]string(nil), override.PartnerFields...)
	}
	if override.PrimaryEmailField != "" {
		base.PrimaryEmailField = override.PrimaryEmailField
	}
	if override.SelfPartner {
		base.SelfPartner = true
	}
	if override.IncludeEmailCC {
		base.IncludeEmailCC = true
	}
	if len(override.CreationSubtypeXMLIDs) > 0 {
		base.CreationSubtypeXMLIDs = append([]string(nil), override.CreationSubtypeXMLIDs...)
	}
	if override.DisablePartnerHeuristic {
		base.DisablePartnerHeuristic = true
	}
	if override.DisableEmailHeuristic {
		base.DisableEmailHeuristic = true
	}
	return base
}

func suggestedReplySubtypeIDs(env *record.Env, modelName string) map[int64]bool {
	xmlIDs := []string{"mail.mt_comment"}
	xmlIDs = append(xmlIDs, suggestedRecipientHook(modelName).CreationSubtypeXMLIDs...)
	out := map[int64]bool{}
	for _, xmlID := range uniqueSuggestedStrings(xmlIDs) {
		if id := resolveXMLID(env, xmlID, "mail.message.subtype"); id != 0 {
			out[id] = true
		}
	}
	return out
}

func cleanSuggestedStrings(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsSuggestedString(out, value) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func uniqueSuggestedStrings(values []string) []string {
	return cleanSuggestedStrings(values)
}

func idSet(ids []int64) map[int64]bool {
	out := map[int64]bool{}
	for _, id := range ids {
		if id != 0 {
			out[id] = true
		}
	}
	return out
}

func containsSuggestedString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
