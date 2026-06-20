package mail

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorp/internal/domain"
	internalphone "gorp/internal/phone"
	"gorp/internal/record"
)

type smsComposerRecipient struct {
	RecordID    int64
	PartnerID   int64
	Name        string
	Number      string
	RawNumber   string
	NumberField string
	Valid       bool
	FailureType string
}

func SMSComposerDefaultGet(env *record.Env, fields []string, context map[string]any) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("sms composer requires env")
	}
	values, err := env.Model("sms.composer").DefaultGet(fields, context)
	if err != nil {
		return nil, err
	}
	modelName := firstText(values["res_model"], context["default_res_model"], context["active_model"])
	putDefault(values, "res_model", modelName)
	putDefault(values, "mass_keep_log", true)
	putDefault(values, "mass_force_send", false)
	putDefault(values, "use_exclusion_list", true)
	if int64FromAny(firstNonNil(values["res_id"], context["default_res_id"])) == 0 && strings.TrimSpace(stringAny(values["res_ids"])) == "" {
		activeIDs := int64s(context["active_ids"])
		if len(activeIDs) > 1 {
			values["res_ids"] = smsComposerFormatIDs(activeIDs)
		} else if activeID := int64FromAny(context["active_id"]); activeID != 0 {
			values["res_id"] = activeID
		}
	}
	templateID := int64FromAny(firstNonNil(values["template_id"], context["default_template_id"], context["template_id"]))
	if templateID != 0 {
		values["template_id"] = templateID
		if strings.TrimSpace(stringAny(values["body"])) == "" {
			body, templateModel, err := smsTemplateBody(env, templateID)
			if err != nil {
				return nil, err
			}
			if modelName == "" {
				modelName = templateModel
				values["res_model"] = modelName
			}
			values["body"] = body
		}
	}
	resIDs := smsComposerRecordIDs(values)
	values["res_ids_count"] = len(resIDs)
	mode := firstText(values["composition_mode"], context["default_composition_mode"], context["sms_composition_mode"])
	if mode == "" || mode == "guess" {
		if len(resIDs) > 1 {
			mode = "mass"
		} else {
			mode = "comment"
		}
	}
	values["composition_mode"] = mode
	resID := int64FromAny(values["res_id"])
	if resID == 0 && len(resIDs) == 1 {
		resID = resIDs[0]
		values["res_id"] = resID
	}
	values["comment_single_recipient"] = mode == "comment" && resID != 0
	if modelName != "" {
		values["res_model_description"] = smsComposerModelDescription(env, modelName)
	}
	if mode == "comment" && resID != 0 {
		rec, err := smsComposerRecipientForRecord(env, modelName, resID, stringAny(values["number_field_name"]), "")
		if err != nil {
			return nil, err
		}
		values["recipient_single_description"] = rec.Name
		values["recipient_single_number"] = rec.Number
		values["recipient_single_number_itf"] = rec.Number
		values["recipient_single_valid"] = rec.Valid
		values["number_field_name"] = rec.NumberField
		if rec.Valid {
			values["recipient_valid_count"] = 1
			values["recipient_invalid_count"] = 0
		} else {
			values["recipient_valid_count"] = 0
			values["recipient_invalid_count"] = 1
		}
		if templateID != 0 {
			body, _, err := smsTemplateBody(env, templateID)
			if err != nil {
				return nil, err
			}
			rendered, err := smsRenderBody(env, modelName, resID, body)
			if err != nil {
				return nil, err
			}
			values["body"] = rendered
		}
	}
	if mode == "numbers" {
		sanitized := smsComposerSanitizeNumbers(stringAny(firstNonNil(values["sanitized_numbers"], values["numbers"])))
		values["sanitized_numbers"] = strings.Join(sanitized, ",")
		values["recipient_valid_count"] = len(sanitized)
	}
	if len(fields) == 0 {
		return values, nil
	}
	filtered := map[string]any{}
	for _, field := range fields {
		if value, ok := values[field]; ok {
			filtered[field] = value
		}
	}
	return filtered, nil
}

func SendSMSComposer(env *record.Env, ids []int64, massNow bool, now time.Time) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("sms composer requires env")
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("sms composer requires ids")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	fields := []string{"composition_mode", "res_model", "res_id", "res_ids", "mass_keep_log", "mass_force_send", "use_exclusion_list", "recipient_single_number", "recipient_single_number_itf", "number_field_name", "numbers", "sanitized_numbers", "template_id", "body"}
	rows, err := env.Model("sms.composer").Browse(ids...).Read(fields...)
	if err != nil {
		return nil, err
	}
	created := make([]int64, 0)
	for _, row := range rows {
		if massNow {
			if err := env.Model("sms.composer").Browse(int64FromAny(row["id"])).Write(map[string]any{"mass_force_send": true}); err != nil {
				return nil, err
			}
			row["mass_force_send"] = true
		}
		mode := firstText(row["composition_mode"], "comment")
		switch mode {
		case "numbers":
			ids, err := smsComposerSendNumbers(env, row, now)
			if err != nil {
				return nil, err
			}
			created = append(created, ids...)
		case "mass":
			ids, err := smsComposerSendMass(env, row, now)
			if err != nil {
				return nil, err
			}
			created = append(created, ids...)
		default:
			ids, err := smsComposerSendComment(env, row, now)
			if err != nil {
				return nil, err
			}
			created = append(created, ids...)
		}
	}
	return created, nil
}

func smsComposerSendNumbers(env *record.Env, row map[string]any, _ time.Time) ([]int64, error) {
	body := strings.TrimSpace(stringAny(row["body"]))
	numbers := smsComposerSanitizeNumbers(firstText(row["sanitized_numbers"], row["numbers"], row["recipient_single_number_itf"], row["recipient_single_number"]))
	if len(numbers) == 0 {
		return nil, fmt.Errorf("sms composer requires a valid phone number")
	}
	ids := make([]int64, 0, len(numbers))
	for _, number := range numbers {
		id, err := env.Model("sms.sms").Create(map[string]any{
			"number": number,
			"body":   body,
			"state":  "outgoing",
		})
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func smsComposerSendComment(env *record.Env, row map[string]any, now time.Time) ([]int64, error) {
	modelName := strings.TrimSpace(stringAny(row["res_model"]))
	resIDs := smsComposerRecordIDs(row)
	if len(resIDs) == 0 {
		return smsComposerSendNumbers(env, row, now)
	}
	body := strings.TrimSpace(stringAny(row["body"]))
	created := make([]int64, 0, len(resIDs))
	for _, resID := range uniqueIDs(resIDs) {
		requestedNumber := firstText(row["recipient_single_number_itf"], row["recipient_single_number"])
		rec, err := smsComposerRecipientForRecord(env, modelName, resID, stringAny(row["number_field_name"]), requestedNumber)
		if err != nil {
			return nil, err
		}
		if !rec.Valid {
			return nil, fmt.Errorf("sms composer requires a valid phone number")
		}
		if rec.NumberField != "" && strings.TrimSpace(requestedNumber) != "" {
			currentRows, err := env.Model(modelName).Browse(resID).Read(rec.NumberField)
			if err != nil {
				return nil, err
			}
			if len(currentRows) > 0 && strings.TrimSpace(stringAny(currentRows[0][rec.NumberField])) != strings.TrimSpace(requestedNumber) {
				if err := env.Model(modelName).Browse(resID).Write(map[string]any{rec.NumberField: requestedNumber}); err != nil {
					return nil, err
				}
			}
		}
		recordBody, err := smsComposerRenderRecordBody(env, modelName, resID, body, int64FromAny(row["template_id"]))
		if err != nil {
			return nil, err
		}
		smsID, err := env.Model("sms.sms").Create(map[string]any{
			"number":     rec.Number,
			"body":       recordBody,
			"partner_id": rec.PartnerID,
			"state":      "outgoing",
		})
		if err != nil {
			return nil, err
		}
		messageID, err := PostMessage(env, PostRequest{
			Model:        modelName,
			ResID:        resID,
			Body:         recordBody,
			MessageType:  "sms",
			SubtypeXMLID: "mail.mt_note",
			PartnerIDs:   smsComposerNotificationPartners(rec),
			BodyIsHTML:   false,
			Now:          now,
		})
		if err != nil {
			return nil, err
		}
		if err := env.Model("sms.sms").Browse(smsID).Write(map[string]any{"mail_message_id": messageID}); err != nil {
			return nil, err
		}
		if err := smsComposerLinkNotifications(env, messageID, smsID, rec.Number); err != nil {
			return nil, err
		}
		created = append(created, smsID)
	}
	return created, nil
}

func smsComposerSendMass(env *record.Env, row map[string]any, now time.Time) ([]int64, error) {
	modelName := strings.TrimSpace(stringAny(row["res_model"]))
	resIDs := smsComposerRecordIDs(row)
	body := strings.TrimSpace(stringAny(row["body"]))
	seen := map[string]bool{}
	created := make([]int64, 0, len(resIDs))
	for _, resID := range uniqueIDs(resIDs) {
		rec, err := smsComposerRecipientForRecord(env, modelName, resID, "", "")
		if err != nil {
			return nil, err
		}
		failure := rec.FailureType
		if rec.Valid && smsComposerPhoneBlacklisted(env, rec.Number) {
			failure = "sms_blacklist"
		}
		if rec.Valid && seen[rec.Number] {
			failure = "sms_duplicate"
		}
		state := "outgoing"
		if failure != "" {
			state = "canceled"
		} else {
			seen[rec.Number] = true
		}
		recordBody, err := smsComposerRenderRecordBody(env, modelName, resID, body, int64FromAny(row["template_id"]))
		if err != nil {
			return nil, err
		}
		values := map[string]any{
			"number":       firstText(rec.Number, rec.RawNumber),
			"body":         recordBody,
			"partner_id":   rec.PartnerID,
			"state":        state,
			"failure_type": failure,
		}
		smsID, err := env.Model("sms.sms").Create(values)
		if err != nil {
			return nil, err
		}
		if boolAny(row["mass_keep_log"]) && failure == "" {
			messageID, err := PostMessage(env, PostRequest{
				Model:        modelName,
				ResID:        resID,
				Body:         recordBody,
				MessageType:  "sms",
				SubtypeXMLID: "mail.mt_note",
				PartnerIDs:   smsComposerNotificationPartners(rec),
				BodyIsHTML:   false,
				Now:          now,
			})
			if err != nil {
				return nil, err
			}
			if err := env.Model("sms.sms").Browse(smsID).Write(map[string]any{"mail_message_id": messageID}); err != nil {
				return nil, err
			}
			if err := smsComposerLinkNotifications(env, messageID, smsID, rec.Number); err != nil {
				return nil, err
			}
		}
		created = append(created, smsID)
	}
	return created, nil
}

func smsComposerRecordIDs(row map[string]any) []int64 {
	ids := int64s(row["res_ids"])
	if len(ids) == 0 {
		ids = smsComposerParseIDs(stringAny(row["res_ids"]))
	}
	if len(ids) == 0 {
		if id := int64FromAny(row["res_id"]); id != 0 {
			ids = []int64{id}
		}
	}
	return uniqueIDs(ids)
}

func smsComposerParseIDs(text string) []int64 {
	text = strings.NewReplacer("[", " ", "]", " ", "(", " ", ")", " ").Replace(text)
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ';' || r == ' ' })
	out := make([]int64, 0, len(parts))
	for _, part := range parts {
		id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
		if err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func smsComposerFormatIDs(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range uniqueIDs(ids) {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func smsTemplateBody(env *record.Env, templateID int64) (string, string, error) {
	rows, err := env.Model("sms.template").Browse(templateID).Read("body", "model")
	if err != nil || len(rows) == 0 {
		return "", "", err
	}
	return stringAny(rows[0]["body"]), stringAny(rows[0]["model"]), nil
}

func smsComposerRenderRecordBody(env *record.Env, modelName string, resID int64, body string, templateID int64) (string, error) {
	if templateID != 0 {
		templateBody, _, err := smsTemplateBody(env, templateID)
		if err != nil {
			return "", err
		}
		body = templateBody
	}
	return smsRenderBody(env, modelName, resID, body)
}

func smsRenderBody(env *record.Env, modelName string, resID int64, body string) (string, error) {
	values, err := renderValues(env, modelName, resID, body)
	if err != nil {
		return "", err
	}
	return renderText(body, values), nil
}

func smsComposerRecipientForRecord(env *record.Env, modelName string, resID int64, preferredField string, overrideNumber string) (smsComposerRecipient, error) {
	rec := smsComposerRecipient{RecordID: resID, NumberField: preferredField}
	if env == nil || strings.TrimSpace(modelName) == "" || resID == 0 {
		rec.FailureType = "sms_number_missing"
		return rec, nil
	}
	fields := smsComposerRecipientFields(env, modelName, preferredField)
	if len(fields) == 0 {
		rec.FailureType = "sms_number_missing"
		return rec, nil
	}
	rows, err := env.Model(modelName).Browse(resID).Read(fields...)
	if err != nil || len(rows) == 0 {
		return rec, err
	}
	row := rows[0]
	rec.Name = firstText(row["display_name"], row["name"], fmt.Sprintf("%s,%d", modelName, resID))
	rec.PartnerID = int64FromAny(row["partner_id"])
	for _, fieldName := range smsComposerNumberFields(preferredField) {
		if _, ok := row[fieldName]; !ok {
			continue
		}
		raw := strings.TrimSpace(stringAny(row[fieldName]))
		if raw == "" {
			continue
		}
		rec.RawNumber = raw
		rec.NumberField = fieldName
		break
	}
	if strings.TrimSpace(overrideNumber) != "" {
		rec.RawNumber = strings.TrimSpace(overrideNumber)
	}
	rec.Number = smsNormalizeNumber(rec.RawNumber)
	rec.Valid = strings.HasPrefix(rec.Number, "+") && len(rec.Number) > 4
	if !rec.Valid {
		if strings.TrimSpace(rec.RawNumber) == "" {
			rec.FailureType = "sms_number_missing"
		} else {
			rec.FailureType = "sms_number_format"
		}
	}
	if rec.PartnerID == 0 && modelName == "res.partner" {
		rec.PartnerID = resID
	}
	return rec, nil
}

func smsComposerRecipientFields(env *record.Env, modelName string, preferredField string) []string {
	fields := []string{}
	for _, fieldName := range []string{"name", "display_name", "partner_id"} {
		if modelHasField(env, modelName, fieldName) {
			fields = append(fields, fieldName)
		}
	}
	for _, fieldName := range smsComposerNumberFields(preferredField) {
		if modelHasField(env, modelName, fieldName) {
			fields = append(fields, fieldName)
		}
	}
	return uniqueStrings(fields)
}

func smsComposerNumberFields(preferredField string) []string {
	fields := []string{}
	if strings.TrimSpace(preferredField) != "" {
		fields = append(fields, strings.TrimSpace(preferredField))
	}
	fields = append(fields, "mobile", "phone", "phone_sanitized")
	return uniqueStrings(fields)
}

func smsComposerSanitizeNumbers(text string) []string {
	parts := strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\t' })
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		number := smsNormalizeNumber(part)
		if number == "" || seen[number] {
			continue
		}
		seen[number] = true
		out = append(out, number)
	}
	return out
}

func smsNormalizeNumber(value string) string {
	return internalphone.NormalizeE164(value, internalphone.Country{})
}

func smsComposerPhoneBlacklisted(env *record.Env, number string) bool {
	if strings.TrimSpace(number) == "" || !modelHasField(env, "phone.blacklist", "number") {
		return false
	}
	found, err := env.Model("phone.blacklist").SearchWithOptions(domain.And(
		domain.Cond("number", domain.Equal, number),
		domain.Cond("active", domain.Equal, true),
	), record.SearchOptions{Limit: 1})
	return err == nil && found.Len() > 0
}

func smsComposerNotificationPartners(rec smsComposerRecipient) []int64 {
	if rec.PartnerID == 0 {
		return nil
	}
	return []int64{rec.PartnerID}
}

func smsComposerLinkNotifications(env *record.Env, messageID int64, smsID int64, number string) error {
	if env == nil || messageID == 0 || smsID == 0 {
		return nil
	}
	found, err := env.Model("mail.notification").Search(domain.Cond("mail_message_id", domain.Equal, messageID))
	if err != nil || found.Len() == 0 {
		return err
	}
	values := map[string]any{}
	if modelHasField(env, "mail.notification", "sms_id") {
		values["sms_id"] = smsID
	}
	if modelHasField(env, "mail.notification", "sms_id_int") {
		values["sms_id_int"] = smsID
	}
	if modelHasField(env, "mail.notification", "sms_number") {
		values["sms_number"] = number
	}
	if len(values) == 0 {
		return nil
	}
	return found.Write(values)
}

func smsComposerModelDescription(env *record.Env, modelName string) string {
	if env == nil || strings.TrimSpace(modelName) == "" || !modelHasField(env, "ir.model", "model") {
		return modelName
	}
	found, err := env.Model("ir.model").SearchWithOptions(domain.Cond("model", domain.Equal, modelName), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return modelName
	}
	rows, err := found.Read("name")
	if err != nil || len(rows) == 0 {
		return modelName
	}
	return firstText(rows[0]["name"], modelName)
}
