package workflow

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"gorp/internal/domain"
	"gorp/internal/meta/view"
	"gorp/internal/model"
	"gorp/internal/record"
)

// ApplyComputedWorkflowViewIDs mirrors the OI computed approval.record.workflow_view_id field.
func ApplyComputedWorkflowViewIDs(env *record.Env, modelName string, rows []map[string]any) error {
	if env == nil || modelName == "" || len(rows) == 0 {
		return nil
	}
	if !modelHasField(env, modelName, "workflow_view_id") {
		return nil
	}
	for _, row := range rows {
		if _, requested := row["workflow_view_id"]; !requested {
			continue
		}
		recordID := int64FromAny(row["id"])
		if recordID == 0 {
			row["workflow_view_id"] = false
			continue
		}
		viewID, err := WorkflowViewIDForRecord(env, modelName, recordID)
		if err != nil {
			return err
		}
		row["workflow_view_id"] = workflowViewWebValue(env, viewID)
	}
	return nil
}

func WorkflowViewIDForRecord(env *record.Env, modelName string, recordID int64) (int64, error) {
	if env == nil || modelName == "" || recordID == 0 {
		return 0, nil
	}
	if !modelHasField(env, modelName, "workflow_view_id") {
		return 0, nil
	}
	nodeViewID, err := workflowNodeViewIDForRecord(env, modelName, recordID)
	if err != nil {
		return 0, err
	}
	if nodeViewID != 0 {
		return nodeViewID, nil
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		if strings.Contains(err.Error(), "unknown model ") {
			return 0, nil
		}
		return 0, err
	}
	sort.SliceStable(workflows, func(i, j int) bool {
		if workflows[i].Sequence == workflows[j].Sequence {
			return workflows[i].ID < workflows[j].ID
		}
		return workflows[i].Sequence < workflows[j].Sequence
	})
	base := evalContextBase(env, modelName)
	ctx, err := evalContextForRecord(env, modelName, recordID, base)
	if err != nil {
		return 0, err
	}
	for _, workflow := range workflows {
		if !workflow.Active || workflow.Model != modelName {
			continue
		}
		ok, err := workflow.Condition.Evaluate(ctx)
		if err != nil {
			return 0, err
		}
		if ok {
			return workflow.ViewID, nil
		}
	}
	return 0, nil
}

func workflowNodeViewIDForRecord(env *record.Env, modelName string, recordID int64) (int64, error) {
	if !modelHasField(env, modelName, "workflow_node_id") {
		return 0, nil
	}
	rows, err := env.Model(modelName).Browse(recordID).Read("workflow_node_id")
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	nodeID := int64FromAny(rows[0]["workflow_node_id"])
	if nodeID == 0 {
		return 0, nil
	}
	nodeRows, err := env.Model(ModelNode).Browse(nodeID).Read("workflow_id")
	if err != nil || len(nodeRows) == 0 {
		return 0, err
	}
	workflowID := int64FromAny(nodeRows[0]["workflow_id"])
	if workflowID == 0 {
		return 0, nil
	}
	workflowRows, err := env.Model(ModelWorkflow).Browse(workflowID).Read("view_id")
	if err != nil || len(workflowRows) == 0 {
		return 0, err
	}
	return int64FromAny(workflowRows[0]["view_id"]), nil
}

func modelHasField(env *record.Env, modelName string, fieldName string) bool {
	fields, err := env.Model(modelName).FieldsGet([]string{fieldName}, []string{"type"})
	if err != nil {
		return false
	}
	_, ok := fields[fieldName]
	return ok
}

func workflowViewWebValue(env *record.Env, viewID int64) any {
	if viewID == 0 {
		return false
	}
	rows, err := env.Model("ir.ui.view").Browse(viewID).NameGet()
	if err != nil || len(rows) == 0 {
		return []any{viewID, ""}
	}
	return []any{viewID, fmt.Sprint(rows[0][1])}
}

type approvalViewSettings struct {
	ID                         int64
	Model                      string
	StateField                 string
	Advance                    bool
	ShowActionApproveAll       bool
	ShowStatusDurationTracking bool
	DynamicStatusbarVisible    bool
	ApprovalAllGroups          []int64
}

type workflowXMLNode struct {
	Name     string
	Attrs    []xml.Attr
	Children []*workflowXMLNode
	Text     string
}

func ApplyApprovalViewMutation(env *record.Env, modelName string, typ view.Type, arch string, groups map[int64]bool, studio bool) (string, error) {
	settings, ok, err := approvalViewSettingsForModel(env, modelName)
	if err != nil || !ok {
		return arch, err
	}
	root, err := parseWorkflowXMLDocument(arch)
	if err != nil {
		return arch, nil
	}
	if studio {
		if typ == view.Form {
			if header := firstWorkflowDescendant(root, "header"); header != nil {
				appendApprovalHiddenFields(header)
				return renderWorkflowXML(root), nil
			}
		}
		return arch, nil
	}
	meta, hasMeta := env.ModelMetadata(modelName)
	switch typ {
	case view.Form:
		applyApprovalFormViewMutation(env, root, settings, meta, hasMeta, groups)
	case view.List:
		if approvalApproveAllAllowed(settings, groups) && root.Name == "list" {
			setWorkflowAttr(root, "show_action_approve_all", "true")
		}
	default:
		return arch, nil
	}
	return renderWorkflowXML(root), nil
}

func approvalViewSettingsForModel(env *record.Env, modelName string) (approvalViewSettings, bool, error) {
	if env == nil || modelName == "" {
		return approvalViewSettings{}, false, nil
	}
	found, err := env.Model(ModelSettings).Search(domain.And(domain.Cond("model", "=", modelName)))
	if err != nil {
		if strings.Contains(err.Error(), "unknown model "+ModelSettings) {
			return approvalViewSettings{}, false, nil
		}
		return approvalViewSettings{}, false, err
	}
	rows, err := found.Read("id", "model", "active", "advance", "state_field", "show_action_approve_all", "show_status_duration_tracking", "dynamic_statusbar_visible", "approval_all_groups")
	if err != nil {
		return approvalViewSettings{}, false, err
	}
	var selected map[string]any
	for _, row := range rows {
		if row["active"] == false {
			continue
		}
		if selected == nil || int64FromAny(row["id"]) < int64FromAny(selected["id"]) {
			selected = row
		}
	}
	if selected == nil {
		return approvalViewSettings{}, false, nil
	}
	return approvalViewSettings{
		ID:                         int64FromAny(selected["id"]),
		Model:                      firstString(selected["model"], modelName),
		StateField:                 firstString(selected["state_field"], "state"),
		Advance:                    boolFromAny(selected["advance"]),
		ShowActionApproveAll:       boolDefaultTrue(selected, "show_action_approve_all"),
		ShowStatusDurationTracking: boolDefaultTrue(selected, "show_status_duration_tracking"),
		DynamicStatusbarVisible:    boolDefaultTrue(selected, "dynamic_statusbar_visible"),
		ApprovalAllGroups:          idsFromAny(selected["approval_all_groups"]),
	}, true, nil
}

func applyApprovalFormViewMutation(env *record.Env, root *workflowXMLNode, settings approvalViewSettings, meta model.Model, hasMeta bool, groups map[int64]bool) {
	if hasMeta {
		applyApprovalDefaultReadonly(root, meta)
	}
	sheet := firstWorkflowDescendant(root, "sheet")
	if sheet != nil {
		ensureApprovalButtonBox(sheet)
	}
	header := firstWorkflowDescendant(root, "header")
	if header == nil {
		return
	}
	appendApprovalHiddenFields(header)
	if settings.Advance {
		appendApprovalAdvancedHiddenFields(header)
		appendApprovalTransitionButtons(env, header, settings, groups)
	}
	appendApprovalButtons(env, header, settings)
	appendApprovalUserInfoButton(header)
	if stateField := firstWorkflowDescendantWithAttr(header, "field", "name", firstString(settings.StateField, "state")); stateField != nil {
		if settings.ShowStatusDurationTracking {
			setWorkflowAttr(stateField, "widget", "statusbar_state_duration")
		}
		if settings.DynamicStatusbarVisible {
			setWorkflowAttr(stateField, "statusbar_visible", "WORKFLOW")
		}
	}
}

func applyApprovalDefaultReadonly(root *workflowXMLNode, meta model.Model) {
	expr := strings.TrimSpace(meta.DefaultFieldReadonly)
	if expr == "" {
		return
	}
	applyApprovalDefaultReadonlyNode(root, meta, expr)
}

func applyApprovalDefaultReadonlyNode(node *workflowXMLNode, meta model.Model, expr string) {
	if node == nil {
		return
	}
	if node.Name == "field" {
		name := workflowAttrValue(node, "name")
		if name != "" && name != "state" && workflowAttrValue(node, "readonly") == "" {
			invisible := workflowAttrValue(node, "invisible")
			fieldMeta, ok := meta.Fields[name]
			if invisible != "0" && invisible != "False" && ok && !fieldMeta.Readonly {
				setWorkflowAttr(node, "readonly", expr)
			}
		}
	}
	for _, child := range node.Children {
		applyApprovalDefaultReadonlyNode(child, meta, expr)
	}
}

func ensureApprovalButtonBox(sheet *workflowXMLNode) {
	box := firstWorkflowDescendantWithAttr(sheet, "div", "name", "button_box")
	if box == nil {
		box = &workflowXMLNode{Name: "div", Attrs: []xml.Attr{
			{Name: xml.Name{Local: "name"}, Value: "button_box"},
			{Name: xml.Name{Local: "class"}, Value: "oe_button_box"},
		}}
		sheet.Children = append([]*workflowXMLNode{box}, sheet.Children...)
	}
	if firstWorkflowDescendantWithAttr(box, "button", "name", "action_open_canceled_record") != nil {
		return
	}
	button := &workflowXMLNode{Name: "button", Attrs: []xml.Attr{
		{Name: xml.Name{Local: "name"}, Value: "action_open_canceled_record"},
		{Name: xml.Name{Local: "type"}, Value: "object"},
		{Name: xml.Name{Local: "invisible"}, Value: "record_cancellation_count == 0"},
		{Name: xml.Name{Local: "icon"}, Value: "fa-remove"},
		{Name: xml.Name{Local: "class"}, Value: "oe_stat_button"},
	}}
	button.Children = append(button.Children, &workflowXMLNode{Name: "field", Attrs: []xml.Attr{
		{Name: xml.Name{Local: "name"}, Value: "record_cancellation_count"},
		{Name: xml.Name{Local: "readonly"}, Value: "1"},
		{Name: xml.Name{Local: "widget"}, Value: "statinfo"},
		{Name: xml.Name{Local: "string"}, Value: "Cancellation"},
	}})
	box.Children = append(box.Children, button)
}

func appendApprovalHiddenFields(header *workflowXMLNode) {
	for _, name := range []string{"workflow_states", "user_can_approve", "document_user_id", "record_cancellation_count", "approved_button_clicked", "approval_visible_button_ids"} {
		if firstWorkflowChildWithAttr(header, "field", "name", name) != nil {
			continue
		}
		header.Children = append(header.Children, &workflowXMLNode{Name: "field", Attrs: []xml.Attr{
			{Name: xml.Name{Local: "name"}, Value: name},
			{Name: xml.Name{Local: "invisible"}, Value: "1"},
			{Name: xml.Name{Local: "readonly"}, Value: "1"},
		}})
	}
}

func appendApprovalAdvancedHiddenFields(header *workflowXMLNode) {
	for _, name := range []string{"workflow_transition_ids", "workflow_node_id"} {
		if firstWorkflowChildWithAttr(header, "field", "name", name) != nil {
			continue
		}
		header.Children = append(header.Children, &workflowXMLNode{Name: "field", Attrs: []xml.Attr{
			{Name: xml.Name{Local: "name"}, Value: name},
			{Name: xml.Name{Local: "invisible"}, Value: "1"},
			{Name: xml.Name{Local: "readonly"}, Value: "1"},
		}})
	}
}

func appendApprovalUserInfoButton(header *workflowXMLNode) {
	if firstWorkflowChildWithAttr(header, "button", "id", "approval_user_info") != nil {
		return
	}
	header.Children = append(header.Children, &workflowXMLNode{Name: "button", Attrs: []xml.Attr{
		{Name: xml.Name{Local: "name"}, Value: ""},
		{Name: xml.Name{Local: "string"}, Value: ""},
		{Name: xml.Name{Local: "invisible"}, Value: "not waiting_approval"},
		{Name: xml.Name{Local: "type"}, Value: "action"},
		{Name: xml.Name{Local: "class"}, Value: "btn-link btn-info"},
		{Name: xml.Name{Local: "icon"}, Value: "fa-users"},
		{Name: xml.Name{Local: "id"}, Value: "approval_user_info"},
	}})
}

func appendApprovalTransitionButtons(env *record.Env, header *workflowXMLNode, settings approvalViewSettings, groups map[int64]bool) {
	if env == nil || header == nil || settings.Model == "" {
		return
	}
	workflows, err := loadAdvancedWorkflows(env)
	if err != nil {
		return
	}
	sort.SliceStable(workflows, func(i, j int) bool {
		if workflows[i].Sequence == workflows[j].Sequence {
			return workflows[i].ID < workflows[j].ID
		}
		return workflows[i].Sequence < workflows[j].Sequence
	})
	for _, workflow := range workflows {
		if !workflow.Active || workflow.Model != settings.Model {
			continue
		}
		for _, node := range orderedNodes(workflow.Nodes) {
			if !node.Active || node.Type != NodeTypeUser {
				continue
			}
			switch node.ButtonType {
			case ButtonTypeOne:
				appendApprovalTransitionWizardButton(header, node)
			case ButtonTypeMulti:
				for _, transition := range orderedTransitions(node.Transitions) {
					if transitionUserButtonAccess(env, transition, groups) {
						appendApprovalTransitionButton(header, transition)
					}
				}
			}
		}
	}
}

func appendApprovalTransitionWizardButton(header *workflowXMLNode, node Node) {
	if node.ID == 0 {
		return
	}
	buttonID := "approval_transition_wizard_" + strconv.FormatInt(node.ID, 10)
	if firstWorkflowChildWithAttr(header, "button", "id", buttonID) != nil {
		return
	}
	header.Children = append(header.Children, &workflowXMLNode{Name: "button", Attrs: []xml.Attr{
		{Name: xml.Name{Local: "name"}, Value: "approval_transition_wizard"},
		{Name: xml.Name{Local: "type"}, Value: "object"},
		{Name: xml.Name{Local: "string"}, Value: node.ButtonName},
		{Name: xml.Name{Local: "invisible"}, Value: "workflow_node_id == " + strconv.FormatInt(node.ID, 10) + " and workflow_transition_ids"},
		{Name: xml.Name{Local: "class"}, Value: "btn-primary"},
		{Name: xml.Name{Local: "args"}, Value: "[" + strconv.FormatInt(node.ID, 10) + "]"},
		{Name: xml.Name{Local: "context"}, Value: node.ButtonContext},
		{Name: xml.Name{Local: "icon"}, Value: node.ButtonIcon},
		{Name: xml.Name{Local: "id"}, Value: buttonID},
		{Name: xml.Name{Local: "validate_form"}, Value: fmt.Sprint(node.ButtonValidateForm)},
	}})
}

func appendApprovalTransitionButton(header *workflowXMLNode, transition Transition) {
	if transition.ID == 0 {
		return
	}
	buttonID := "approval_transition_button_" + strconv.FormatInt(transition.ID, 10)
	if firstWorkflowChildWithAttr(header, "button", "id", buttonID) != nil {
		return
	}
	header.Children = append(header.Children, &workflowXMLNode{Name: "button", Attrs: []xml.Attr{
		{Name: xml.Name{Local: "name"}, Value: "approval_transition_button"},
		{Name: xml.Name{Local: "type"}, Value: "object"},
		{Name: xml.Name{Local: "string"}, Value: transition.Name},
		{Name: xml.Name{Local: "invisible"}, Value: strconv.FormatInt(transition.ID, 10) + " not in workflow_transition_ids"},
		{Name: xml.Name{Local: "class"}, Value: transition.ButtonClass},
		{Name: xml.Name{Local: "args"}, Value: "[" + strconv.FormatInt(transition.ID, 10) + "]"},
		{Name: xml.Name{Local: "context"}, Value: transitionContextRaw(transition)},
		{Name: xml.Name{Local: "icon"}, Value: transition.Icon},
		{Name: xml.Name{Local: "id"}, Value: buttonID},
		{Name: xml.Name{Local: "validate_form"}, Value: fmt.Sprint(transition.ValidateForm)},
	}})
}

func transitionContextRaw(transition Transition) string {
	if transition.Context == nil {
		return ""
	}
	return stringFromAny(transition.Context["raw"])
}

func transitionUserButtonAccess(env *record.Env, transition Transition, groups map[int64]bool) bool {
	if !transition.Active {
		return false
	}
	if len(transition.GroupIDs) == 0 || env.Context().UserID == 1 {
		return true
	}
	for _, groupID := range transition.GroupIDs {
		if groups[groupID] {
			return true
		}
	}
	return false
}

func appendApprovalButtons(env *record.Env, header *workflowXMLNode, settings approvalViewSettings) {
	if env == nil || settings.ID == 0 {
		return
	}
	found, err := env.Model(ModelButton).Search(domain.And())
	if err != nil {
		return
	}
	rows, err := found.Read("id", "settings_id", "model", "sequence", "active", "name", "action_type", "server_action_id", "invisible", "button_class", "confirm_message", "comment", "context", "icon", "hotkey", "validate_form", "group_ids")
	if err != nil {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		iSeq := int64FromAny(rows[i]["sequence"])
		jSeq := int64FromAny(rows[j]["sequence"])
		if iSeq == jSeq {
			return int64FromAny(rows[i]["id"]) < int64FromAny(rows[j]["id"])
		}
		return iSeq < jSeq
	})
	for _, row := range rows {
		id := int64FromAny(row["id"])
		if id == 0 || row["active"] == false || firstInt64(row["settings_id"]) != settings.ID {
			continue
		}
		if rowModel := strings.TrimSpace(stringFromAny(row["model"])); rowModel != "" && rowModel != settings.Model {
			continue
		}
		buttonID := "approval_button_" + strconv.FormatInt(id, 10)
		if firstWorkflowChildWithAttr(header, "button", "id", buttonID) != nil {
			continue
		}
		attrs := []xml.Attr{
			{Name: xml.Name{Local: "name"}, Value: "approval_action_button"},
			{Name: xml.Name{Local: "string"}, Value: stringFromAny(row["name"])},
			{Name: xml.Name{Local: "invisible"}, Value: firstString(row["invisible"], "0")},
			{Name: xml.Name{Local: "type"}, Value: "object"},
			{Name: xml.Name{Local: "class"}, Value: stringFromAny(row["button_class"])},
			{Name: xml.Name{Local: "confirm"}, Value: approvalButtonConfirm(row)},
			{Name: xml.Name{Local: "context"}, Value: stringFromAny(row["context"])},
			{Name: xml.Name{Local: "args"}, Value: "[" + strconv.FormatInt(id, 10) + "]"},
			{Name: xml.Name{Local: "icon"}, Value: approvalButtonIcon(env, row)},
			{Name: xml.Name{Local: "id"}, Value: buttonID},
			{Name: xml.Name{Local: "validate_form"}, Value: fmt.Sprint(boolFromAny(row["validate_form"]))},
		}
		if groups := workflowGroupXMLIDRefs(env, idsFromAny(row["group_ids"])); len(groups) > 0 {
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "groups"}, Value: strings.Join(groups, ",")})
		}
		if hotkey := strings.TrimSpace(stringFromAny(row["hotkey"])); hotkey != "" {
			attrs = append(attrs, xml.Attr{Name: xml.Name{Local: "data-hotkey"}, Value: hotkey})
		}
		header.Children = append(header.Children, &workflowXMLNode{Name: "button", Attrs: attrs})
	}
}

func workflowGroupXMLIDRefs(env *record.Env, groupIDs []int64) []string {
	if len(groupIDs) == 0 {
		return nil
	}
	refs := workflowExistingGroupXMLIDRefs(env, groupIDs)
	out := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		if groupID == 0 {
			continue
		}
		ref := refs[groupID]
		if ref == "" {
			ref = workflowEnsureCustomGroupXMLID(env, groupID)
		}
		if ref == "" {
			ref = strconv.FormatInt(groupID, 10)
		}
		out = append(out, ref)
	}
	return out
}

func workflowExistingGroupXMLIDRefs(env *record.Env, groupIDs []int64) map[int64]string {
	refs := map[int64]string{}
	if env == nil || len(groupIDs) == 0 {
		return refs
	}
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("model", "=", "res.groups"),
		domain.Cond("res_id", "in", groupIDs),
	))
	if err != nil {
		return refs
	}
	rows, err := found.Read("module", "name", "complete_name", "res_id")
	if err != nil {
		return refs
	}
	for _, row := range rows {
		groupID := int64FromAny(row["res_id"])
		if groupID == 0 || refs[groupID] != "" {
			continue
		}
		ref := stringFromAny(row["complete_name"])
		if ref == "" {
			moduleName := stringFromAny(row["module"])
			name := stringFromAny(row["name"])
			if moduleName != "" && name != "" {
				ref = moduleName + "." + name
			}
		}
		if ref != "" {
			refs[groupID] = ref
		}
	}
	return refs
}

func workflowEnsureCustomGroupXMLID(env *record.Env, groupID int64) string {
	if env == nil || groupID == 0 {
		return ""
	}
	name := "group_" + strconv.FormatInt(groupID, 10)
	ref := "__custom__." + name
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module":        "__custom__",
		"name":          name,
		"complete_name": ref,
		"model":         "res.groups",
		"res_id":        groupID,
		"noupdate":      false,
	}); err != nil {
		return ""
	}
	return ref
}

func approvalButtonConfirm(row map[string]any) string {
	if stringFromAny(row["comment"]) != "" {
		return ""
	}
	return stringFromAny(row["confirm_message"])
}

func approvalButtonIcon(env *record.Env, row map[string]any) string {
	if icon := strings.TrimSpace(stringFromAny(row["icon"])); icon != "" {
		return icon
	}
	if serverActionID := int64FromAny(row["server_action_id"]); serverActionID != 0 {
		return approvalButtonServerActionIcon(env, serverActionID)
	}
	switch stringFromAny(row["action_type"]) {
	case string(ActionApprove):
		return "fa-thumbs-up"
	case string(ActionReject):
		return "fa-thumbs-down"
	case string(ActionReturn):
		return "fa-reply"
	case string(ActionCancel):
		return "fa-times"
	case string(ActionCancelWorkflow):
		return "fa-times-circle"
	case string(ActionDraft):
		return "fa-edit"
	case string(ActionForward):
		return "fa-mail-forward"
	case string(ActionTransfer):
		return "fa-exchange"
	case string(ActionServerAction):
		return "fa-cog"
	case string(ActionEmail):
		return "fa-envelope"
	default:
		return ""
	}
}

func approvalButtonServerActionIcon(env *record.Env, serverActionID int64) string {
	if env == nil || serverActionID == 0 {
		return "fa-cog"
	}
	rows, err := env.Model("ir.actions.server").Browse(serverActionID).Read("code", "binding_type")
	if err != nil || len(rows) == 0 {
		return "fa-cog"
	}
	if strings.Contains(stringFromAny(rows[0]["code"]), "Excel") {
		return "fa-file-excel-o"
	}
	if stringFromAny(rows[0]["binding_type"]) == "print" {
		return "fa-print"
	}
	return "fa-cog"
}

func approvalApproveAllAllowed(settings approvalViewSettings, groups map[int64]bool) bool {
	if !settings.ShowActionApproveAll {
		return false
	}
	if len(settings.ApprovalAllGroups) == 0 {
		return true
	}
	for _, groupID := range settings.ApprovalAllGroups {
		if groups[groupID] {
			return true
		}
	}
	return false
}

func parseWorkflowXMLDocument(raw string) (*workflowXMLNode, error) {
	fragment, err := parseWorkflowXMLFragment(raw)
	if err != nil {
		return nil, err
	}
	var roots []*workflowXMLNode
	for _, child := range fragment.Children {
		if child.Name != "" {
			roots = append(roots, child)
		}
	}
	if len(roots) != 1 {
		return nil, fmt.Errorf("expected one XML root, got %d", len(roots))
	}
	return roots[0], nil
}

func parseWorkflowXMLFragment(raw string) (*workflowXMLNode, error) {
	decoder := xml.NewDecoder(strings.NewReader("<__root__>" + raw + "</__root__>"))
	var stack []*workflowXMLNode
	var root *workflowXMLNode
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			node := &workflowXMLNode{Name: typed.Name.Local, Attrs: append([]xml.Attr(nil), typed.Attr...)}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			if node.Name == "__root__" {
				root = node
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("unexpected end element %s", typed.Name.Local)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			text := string([]byte(typed))
			if strings.TrimSpace(text) == "" {
				continue
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, &workflowXMLNode{Text: text})
		}
	}
	if root == nil {
		return nil, fmt.Errorf("empty XML fragment")
	}
	return root, nil
}

func renderWorkflowXML(root *workflowXMLNode) string {
	var buf bytes.Buffer
	renderWorkflowXMLNode(&buf, root)
	return buf.String()
}

func renderWorkflowXMLNode(buf *bytes.Buffer, node *workflowXMLNode) {
	if node.Name == "" {
		buf.WriteString(xmlEscape(node.Text))
		return
	}
	buf.WriteByte('<')
	buf.WriteString(node.Name)
	for _, attr := range node.Attrs {
		buf.WriteByte(' ')
		buf.WriteString(attr.Name.Local)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscape(attr.Value))
		buf.WriteByte('"')
	}
	if len(node.Children) == 0 {
		buf.WriteString("/>")
		return
	}
	buf.WriteByte('>')
	for _, child := range node.Children {
		renderWorkflowXMLNode(buf, child)
	}
	buf.WriteString("</")
	buf.WriteString(node.Name)
	buf.WriteByte('>')
}

func xmlEscape(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func firstWorkflowDescendant(node *workflowXMLNode, name string) *workflowXMLNode {
	if node == nil {
		return nil
	}
	if node.Name == name {
		return node
	}
	for _, child := range node.Children {
		if found := firstWorkflowDescendant(child, name); found != nil {
			return found
		}
	}
	return nil
}

func firstWorkflowDescendantWithAttr(node *workflowXMLNode, name string, attr string, value string) *workflowXMLNode {
	if node == nil {
		return nil
	}
	if node.Name == name && workflowAttrValue(node, attr) == value {
		return node
	}
	for _, child := range node.Children {
		if found := firstWorkflowDescendantWithAttr(child, name, attr, value); found != nil {
			return found
		}
	}
	return nil
}

func firstWorkflowChildWithAttr(node *workflowXMLNode, name string, attr string, value string) *workflowXMLNode {
	if node == nil {
		return nil
	}
	for _, child := range node.Children {
		if child.Name == name && workflowAttrValue(child, attr) == value {
			return child
		}
	}
	return nil
}

func workflowAttrValue(node *workflowXMLNode, name string) string {
	for _, attr := range node.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func setWorkflowAttr(node *workflowXMLNode, name string, value string) {
	for idx, attr := range node.Attrs {
		if attr.Name.Local == name {
			node.Attrs[idx].Value = value
			return
		}
	}
	node.Attrs = append(node.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func boolDefaultTrue(row map[string]any, name string) bool {
	value, ok := row[name]
	if !ok || value == nil {
		return true
	}
	return boolFromAny(value)
}
