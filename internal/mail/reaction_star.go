package mail

import (
	"fmt"
	"sort"
	"strings"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type MessageReactionRequest struct {
	MessageID   int64
	Content     string
	Action      string
	AccessToken string
	AccessHash  string
	AccessPID   int64
}

func ReactMessage(env *record.Env, req MessageReactionRequest) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail reaction requires env")
	}
	if req.MessageID == 0 {
		return nil, fmt.Errorf("mail reaction requires message id")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, fmt.Errorf("mail reaction requires content")
	}
	if req.AccessToken != "" || req.AccessHash != "" || req.AccessPID != 0 {
		env = PortalMessageContextEnv(env, req.MessageID, req.AccessToken, req.AccessHash, req.AccessPID)
	}
	systemEnv := messageSystemEnv(env)
	messageRows, err := systemEnv.Model("mail.message").Browse(req.MessageID).Read("model", "res_id")
	if err != nil {
		return nil, err
	}
	if len(messageRows) == 0 {
		return nil, fmt.Errorf("mail.message:%d not found", req.MessageID)
	}
	modelName := strings.TrimSpace(stringAny(messageRows[0]["model"]))
	resID := int64FromAny(messageRows[0]["res_id"])
	if !ThreadAccessible(env, modelName, resID) {
		return nil, fmt.Errorf("%s:%d not found", modelName, resID)
	}
	partnerID := currentPortalPartnerID(env)
	if partnerID == 0 {
		partnerID = currentUserPartnerID(env, env.Context().UserID)
	}
	guestID := int64(0)
	if partnerID == 0 {
		guestID = currentGuestID(env)
	}
	if partnerID == 0 && guestID == 0 {
		return nil, fmt.Errorf("mail reaction requires partner or guest")
	}
	action := strings.TrimSpace(req.Action)
	if action == "add" || action == "remove" {
		if err := setMessageReaction(systemEnv, req.MessageID, content, action, partnerID, guestID); err != nil {
			return nil, err
		}
	}
	return reactionStorePayload(systemEnv, req.MessageID, content), nil
}

func ToggleMessageStarred(env *record.Env, messageID int64) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail star requires env")
	}
	if messageID == 0 {
		return nil, fmt.Errorf("mail star requires message id")
	}
	partnerID := currentUserPartnerID(env, env.Context().UserID)
	if partnerID == 0 {
		return nil, fmt.Errorf("mail star requires user partner")
	}
	systemEnv := messageSystemEnv(env)
	rows, err := systemEnv.Model("mail.message").Browse(messageID).Read("model", "res_id", "starred_partner_ids")
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("mail.message:%d not found", messageID)
	}
	modelName := strings.TrimSpace(stringAny(rows[0]["model"]))
	resID := int64FromAny(rows[0]["res_id"])
	if !ThreadAccessible(env, modelName, resID) {
		return nil, fmt.Errorf("%s:%d not found", modelName, resID)
	}
	starredPartnerIDs := int64SliceFromAny(rows[0]["starred_partner_ids"])
	starred := !containsInt64(starredPartnerIDs, partnerID)
	if starred {
		starredPartnerIDs = append(starredPartnerIDs, partnerID)
	} else {
		starredPartnerIDs = removeInt64(starredPartnerIDs, partnerID)
	}
	if err := systemEnv.Model("mail.message").Browse(messageID).Write(map[string]any{
		"starred_partner_ids": uniqueIDs(starredPartnerIDs),
	}); err != nil {
		return nil, err
	}
	return map[string]any{"mail.message": []map[string]any{{"id": messageID, "starred": starred}}}, nil
}

func UnstarAllMessages(env *record.Env) ([]int64, error) {
	if env == nil {
		return nil, fmt.Errorf("mail star requires env")
	}
	partnerID := currentUserPartnerID(env, env.Context().UserID)
	if partnerID == 0 {
		return nil, fmt.Errorf("mail star requires user partner")
	}
	systemEnv := messageSystemEnv(env)
	found, err := systemEnv.Model("mail.message").Search(domain.Cond("starred_partner_ids", "in", []int64{partnerID}))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id", "starred_partner_ids")
	if err != nil {
		return nil, err
	}
	messageIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		messageID := int64FromAny(row["id"])
		messageIDs = append(messageIDs, messageID)
		starredPartnerIDs := removeInt64(int64SliceFromAny(row["starred_partner_ids"]), partnerID)
		if err := systemEnv.Model("mail.message").Browse(messageID).Write(map[string]any{
			"starred_partner_ids": uniqueIDs(starredPartnerIDs),
		}); err != nil {
			return nil, err
		}
	}
	return uniqueIDs(messageIDs), nil
}

func StarredMailboxPayload(env *record.Env, counterBusID int64) map[string]any {
	counter := int64(0)
	partnerID := int64(0)
	if env != nil {
		partnerID = currentUserPartnerID(env, env.Context().UserID)
	}
	if env != nil && partnerID != 0 {
		if count, err := env.Model("mail.message").SearchCount(domain.Cond("starred_partner_ids", "in", []int64{partnerID}), 0); err == nil {
			counter = int64(count)
		}
	}
	return map[string]any{
		"counter":        counter,
		"counter_bus_id": counterBusID,
		"id":             "starred",
		"model":          "mail.box",
	}
}

func FetchStarredMessages(env *record.Env, req ThreadMessagesRequest) (map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail starred messages require env")
	}
	partnerID := currentUserPartnerID(env, env.Context().UserID)
	if partnerID == 0 {
		return nil, fmt.Errorf("mail starred messages require user partner")
	}
	found, err := env.Model("mail.message").Search(domain.Cond("starred_partner_ids", "in", []int64{partnerID}))
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("subject", "body", "message_type", "model", "res_id", "author_id", "author_guest_id", "email_from", "date", "parent_id", "subtype_id", "partner_ids", "attachment_ids", "body_is_html", "is_internal", "reaction_ids", "starred", "starred_partner_ids", "tracking_value_ids")
	if err != nil {
		return nil, err
	}
	rows = ComputeMessageStarred(env, rows)
	rows = filterStarredRows(rows, partnerID)
	rows = filterThreadMessageRows(messageSystemEnv(env), rows, req)
	count := len(rows)
	rows = windowThreadMessageRows(rows, req)
	trackingRows, err := attachTrackingValues(env, rows)
	if err != nil {
		return nil, err
	}
	messageIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		messageIDs = append(messageIDs, int64FromAny(row["id"]))
	}
	partnerIDs, guestIDs, reactionRows := messageReactionStoreGroups(messageSystemEnv(env), messageIDs, "")
	data := map[string]any{
		"mail.message":        mailboxMessageRows(rows, partnerID, messageReactionGroupsByMessage(reactionRows)),
		"mail.tracking.value": trackingRows,
		"MessageReactions":    reactionRows,
	}
	if partnerRows := actorRows(messageSystemEnv(env), "res.partner", partnerIDs); len(partnerRows) > 0 {
		data["res.partner"] = partnerRows
	}
	if guestRows := actorRows(messageSystemEnv(env), "mail.guest", guestIDs); len(guestRows) > 0 {
		data["mail.guest"] = guestRows
	}
	result := map[string]any{
		"messages": messageIDs,
		"data":     data,
	}
	if strings.TrimSpace(req.SearchTerm) != "" {
		result["count"] = count
	}
	return result, nil
}

func ReadMessagesWithComputedStarred(env *record.Env, ids []int64, fields []string) ([]map[string]any, error) {
	if env == nil {
		return nil, fmt.Errorf("mail message read requires env")
	}
	readFields := append([]string(nil), fields...)
	starredRequested := len(readFields) == 0 || containsString(readFields, "starred")
	starredPartnersRequested := len(readFields) == 0 || containsString(readFields, "starred_partner_ids")
	if starredRequested && !starredPartnersRequested {
		readFields = append(readFields, "starred_partner_ids")
	}
	rows, err := env.Model("mail.message").Browse(ids...).Read(readFields...)
	if err != nil {
		return nil, err
	}
	if starredRequested {
		rows = ComputeMessageStarred(env, rows)
	}
	if starredRequested && !starredPartnersRequested {
		for _, row := range rows {
			delete(row, "starred_partner_ids")
		}
	}
	return rows, nil
}

func ComputeMessageStarred(env *record.Env, rows []map[string]any) []map[string]any {
	partnerID := starredPartnerID(env)
	for _, row := range rows {
		row["starred"] = partnerID != 0 && containsInt64(int64SliceFromAny(row["starred_partner_ids"]), partnerID)
	}
	return rows
}

func StarredSearchDomain(env *record.Env, node domain.Node) domain.Node {
	switch node.Kind {
	case domain.Condition:
		if node.Field != "starred" {
			return node
		}
		return starredConditionDomain(env, node.Operator, node.Value)
	case domain.All, domain.Any:
		children := make([]domain.Node, 0, len(node.Children))
		for _, child := range node.Children {
			children = append(children, StarredSearchDomain(env, child))
		}
		node.Children = children
		return node
	case domain.None:
		if len(node.Children) == 1 {
			node.Children[0] = StarredSearchDomain(env, node.Children[0])
		}
		return node
	default:
		return node
	}
}

func starredConditionDomain(env *record.Env, op domain.Operator, value any) domain.Node {
	matchesTrue, matchesFalse, known := starredBooleanSet(op, value)
	if !known {
		return domain.Cond("starred", op, value)
	}
	switch {
	case matchesTrue && matchesFalse:
		return domain.Bool(true)
	case !matchesTrue && !matchesFalse:
		return domain.Bool(false)
	case matchesTrue:
		return starredPartnerDomain(env, domain.In)
	default:
		return starredPartnerDomain(env, domain.NotIn)
	}
}

func starredBooleanSet(op domain.Operator, value any) (bool, bool, bool) {
	switch op {
	case domain.Equal, domain.OptionalEqual:
		if op == domain.OptionalEqual && !boolAny(value) {
			return true, true, true
		}
		if boolAny(value) {
			return true, false, true
		}
		return false, true, true
	case domain.NotEqual:
		if boolAny(value) {
			return false, true, true
		}
		return true, false, true
	case domain.In, domain.NotIn:
		values := boolSetValues(value)
		hasTrue := values[true]
		hasFalse := values[false]
		if op == domain.NotIn {
			return !hasTrue, !hasFalse, true
		}
		return hasTrue, hasFalse, true
	default:
		return false, false, false
	}
}

func boolSetValues(value any) map[bool]bool {
	out := map[bool]bool{}
	switch typed := value.(type) {
	case []bool:
		for _, item := range typed {
			out[item] = true
		}
	case []any:
		for _, item := range typed {
			out[boolAny(item)] = true
		}
	default:
		out[boolAny(typed)] = true
	}
	return out
}

func starredPartnerDomain(env *record.Env, op domain.Operator) domain.Node {
	if env == nil {
		return domain.Bool(op == domain.NotIn)
	}
	partnerID := currentUserPartnerID(env, env.Context().UserID)
	if partnerID == 0 {
		return domain.Bool(op == domain.NotIn)
	}
	return domain.Cond("starred_partner_ids", op, []int64{partnerID})
}

func setMessageReaction(env *record.Env, messageID int64, content string, action string, partnerID int64, guestID int64) error {
	found, err := env.Model("mail.message.reaction").Search(domain.And(
		domain.Cond("message_id", "=", messageID),
		domain.Cond("content", "=", content),
	))
	if err != nil {
		return err
	}
	rows, err := found.Read("id", "partner_id", "guest_id")
	if err != nil {
		return err
	}
	var existingIDs []int64
	for _, row := range rows {
		if int64FromAny(row["partner_id"]) == partnerID && int64FromAny(row["guest_id"]) == guestID {
			existingIDs = append(existingIDs, int64FromAny(row["id"]))
		}
	}
	switch action {
	case "add":
		if len(existingIDs) > 0 {
			return nil
		}
		values := map[string]any{"message_id": messageID, "content": content}
		if partnerID != 0 {
			values["partner_id"] = partnerID
		}
		if guestID != 0 {
			values["guest_id"] = guestID
		}
		_, err := env.Model("mail.message.reaction").Create(values)
		return err
	case "remove":
		if len(existingIDs) == 0 {
			return nil
		}
		return env.Model("mail.message.reaction").Browse(existingIDs...).Unlink()
	default:
		return nil
	}
}

func reactionStorePayload(env *record.Env, messageID int64, content string) map[string]any {
	partnerIDs, guestIDs, groups := messageReactionStoreGroups(env, []int64{messageID}, content)
	reactionCommand := []any{"DELETE", map[string]any{"message": messageID, "content": content}}
	if len(groups) > 0 {
		reactionCommand = []any{"ADD", []map[string]any{{"message": messageID, "content": content}}}
	}
	payload := map[string]any{
		"mail.message": []map[string]any{{
			"id":        messageID,
			"reactions": []any{reactionCommand},
		}},
	}
	if len(groups) > 0 {
		payload["MessageReactions"] = groups
	}
	if partnerRows := actorRows(env, "res.partner", partnerIDs); len(partnerRows) > 0 {
		payload["res.partner"] = partnerRows
	}
	if guestRows := actorRows(env, "mail.guest", guestIDs); len(guestRows) > 0 {
		payload["mail.guest"] = guestRows
	}
	return payload
}

func messageReactionStoreGroups(env *record.Env, messageIDs []int64, content string) ([]int64, []int64, []map[string]any) {
	messageIDs = uniqueIDs(messageIDs)
	if len(messageIDs) == 0 {
		return nil, nil, []map[string]any{}
	}
	filters := []domain.Node{domain.Cond("message_id", "in", messageIDs)}
	if strings.TrimSpace(content) != "" {
		filters = append(filters, domain.Cond("content", "=", content))
	}
	found, err := env.Model("mail.message.reaction").Search(domain.And(filters...))
	if err != nil {
		return nil, nil, []map[string]any{}
	}
	rows, err := found.Read("id", "message_id", "content", "partner_id", "guest_id")
	if err != nil {
		return nil, nil, []map[string]any{}
	}
	type key struct {
		message int64
		content string
	}
	grouped := map[key]map[string]any{}
	partnerSet := map[int64]bool{}
	guestSet := map[int64]bool{}
	for _, row := range rows {
		messageID := int64FromAny(row["message_id"])
		reactionContent := strings.TrimSpace(stringAny(row["content"]))
		if messageID == 0 || reactionContent == "" {
			continue
		}
		k := key{message: messageID, content: reactionContent}
		group := grouped[k]
		if group == nil {
			group = map[string]any{
				"message":  messageID,
				"content":  reactionContent,
				"count":    0,
				"guests":   []int64{},
				"partners": []int64{},
				"sequence": int64FromAny(row["id"]),
			}
			grouped[k] = group
		}
		group["count"] = int64FromAny(group["count"]) + 1
		reactionID := int64FromAny(row["id"])
		if sequence := int64FromAny(group["sequence"]); sequence == 0 || reactionID < sequence {
			group["sequence"] = reactionID
		}
		if partnerID := int64FromAny(row["partner_id"]); partnerID != 0 {
			group["partners"] = append(group["partners"].([]int64), partnerID)
			partnerSet[partnerID] = true
		}
		if guestID := int64FromAny(row["guest_id"]); guestID != 0 {
			group["guests"] = append(group["guests"].([]int64), guestID)
			guestSet[guestID] = true
		}
	}
	keys := make([]key, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].message == keys[j].message {
			return keys[i].content < keys[j].content
		}
		return keys[i].message > keys[j].message
	})
	groups := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		group := grouped[k]
		group["partners"] = uniqueIDs(group["partners"].([]int64))
		group["guests"] = uniqueIDs(group["guests"].([]int64))
		groups = append(groups, group)
	}
	return setIDs(partnerSet), setIDs(guestSet), groups
}

func mailboxMessageRows(rows []map[string]any, partnerID int64, reactionGroups map[int64][]map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := make(map[string]any, len(row)+2)
		for key, value := range row {
			item[key] = value
		}
		item["body"] = []any{"markup", stringAny(row["body"])}
		item["starred"] = containsInt64(int64SliceFromAny(row["starred_partner_ids"]), partnerID)
		item["thread"] = map[string]any{
			"has_mail_thread": true,
			"id":              int64FromAny(row["res_id"]),
			"model":           stringAny(row["model"]),
		}
		item["default_subject"] = stringAny(row["subject"])
		if groups := reactionGroups[int64FromAny(row["id"])]; len(groups) > 0 {
			item["reactions"] = groups
		} else if item["reactions"] == nil {
			item["reactions"] = []any{}
		}
		out = append(out, item)
	}
	return out
}

func filterStarredRows(rows []map[string]any, partnerID int64) []map[string]any {
	out := rows[:0]
	for _, row := range rows {
		if containsInt64(int64SliceFromAny(row["starred_partner_ids"]), partnerID) {
			out = append(out, row)
		}
	}
	return out
}

func starredPartnerID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	if partnerID := currentPortalPartnerID(env); partnerID != 0 {
		return partnerID
	}
	return currentUserPartnerID(env, env.Context().UserID)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func actorRows(env *record.Env, modelName string, ids []int64) []map[string]any {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	rows, err := env.Model(modelName).Browse(ids...).Read("name")
	if err != nil {
		return nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{"id": int64FromAny(row["id"]), "name": stringAny(row["name"])})
	}
	return out
}

func setIDs(values map[int64]bool) []int64 {
	ids := make([]int64, 0, len(values))
	for id := range values {
		if id != 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func removeInt64(values []int64, target int64) []int64 {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}
