package workflow

import (
	"fmt"
	"strings"
	"testing"

	"gorp/internal/domain"
	"gorp/internal/meta/view"
)

func TestApplyApprovalViewMutationSerializesApprovalButtonGroupsAsXMLIDs(t *testing.T) {
	env := dispatchEnv(t)
	existingGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Existing"})
	if err != nil {
		t.Fatal(err)
	}
	customGroupID, err := env.Model("res.groups").Create(map[string]any{"name": "Custom"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module":        "base",
		"name":          "group_existing",
		"complete_name": "base.group_existing",
		"model":         "res.groups",
		"res_id":        existingGroupID,
	}); err != nil {
		t.Fatal(err)
	}
	settingsID, err := env.Model(ModelSettings).Create(map[string]any{
		"name":        "PO Approval",
		"model":       "purchase.order",
		"active":      true,
		"state_field": "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id":  settingsID,
		"name":         "Approve",
		"action_type":  string(ActionApprove),
		"active":       true,
		"button_class": "btn-primary",
		"group_ids":    []int64{existingGroupID, customGroupID},
	}); err != nil {
		t.Fatal(err)
	}

	arch, err := ApplyApprovalViewMutation(env, "purchase.order", view.Form, `<form><header><field name="state"/></header><sheet><group><field name="name"/></group></sheet></form>`, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	wantGroups := fmt.Sprintf(`groups="base.group_existing,__custom__.group_%d"`, customGroupID)
	if !strings.Contains(arch, wantGroups) {
		t.Fatalf("approval button groups missing %q: %s", wantGroups, arch)
	}
	numericGroups := fmt.Sprintf(`groups="%d,%d"`, existingGroupID, customGroupID)
	if strings.Contains(arch, numericGroups) {
		t.Fatalf("approval button groups used numeric refs: %s", arch)
	}
	found, err := env.Model("ir.model.data").Search(domain.And(
		domain.Cond("module", "=", "__custom__"),
		domain.Cond("name", "=", fmt.Sprintf("group_%d", customGroupID)),
		domain.Cond("model", "=", "res.groups"),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("complete_name", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["complete_name"] != fmt.Sprintf("__custom__.group_%d", customGroupID) || int64FromAny(rows[0]["res_id"]) != customGroupID {
		t.Fatalf("custom group xml id rows = %+v", rows)
	}
}

func TestApplyApprovalViewMutationOrdersButtonsBySequenceAndFiltersModel(t *testing.T) {
	env := dispatchEnv(t)
	settingsID, err := env.Model(ModelSettings).Create(map[string]any{
		"name":        "PO Approval",
		"model":       "purchase.order",
		"active":      true,
		"state_field": "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"model":       "purchase.order",
		"name":        "Late",
		"sequence":    int64(20),
		"action_type": string(ActionApprove),
		"active":      true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"model":       "purchase.order",
		"name":        "Early",
		"sequence":    int64(10),
		"action_type": string(ActionApprove),
		"active":      true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model(ModelButton).Create(map[string]any{
		"settings_id": settingsID,
		"model":       "account.move",
		"name":        "Wrong Model",
		"sequence":    int64(5),
		"action_type": string(ActionApprove),
		"active":      true,
	}); err != nil {
		t.Fatal(err)
	}

	arch, err := ApplyApprovalViewMutation(env, "purchase.order", view.Form, `<form><header><field name="state"/></header><sheet><field name="name"/></sheet></form>`, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	early := strings.Index(arch, `string="Early"`)
	late := strings.Index(arch, `string="Late"`)
	if early < 0 || late < 0 || early > late {
		t.Fatalf("approval buttons not sequence ordered: %s", arch)
	}
	if strings.Contains(arch, "Wrong Model") {
		t.Fatalf("approval button for another model rendered: %s", arch)
	}
}

func TestApplyApprovalViewMutationUsesSourceApprovalButtonIconFallbacks(t *testing.T) {
	env := dispatchEnv(t)
	settingsID, err := env.Model(ModelSettings).Create(map[string]any{
		"name":        "PO Approval",
		"model":       "purchase.order",
		"active":      true,
		"state_field": "state",
	})
	if err != nil {
		t.Fatal(err)
	}
	serverActionID, err := env.Model("ir.actions.server").Create(map[string]any{
		"name":         "Print",
		"binding_type": "print",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name       string
		actionType string
		sequence   int64
		extra      map[string]any
	}{
		{name: "Approve", actionType: string(ActionApprove), sequence: 10},
		{name: "Reject", actionType: string(ActionReject), sequence: 20},
		{name: "Print", actionType: string(ActionServerAction), sequence: 30, extra: map[string]any{"server_action_id": serverActionID}},
	} {
		values := map[string]any{
			"settings_id": settingsID,
			"model":       "purchase.order",
			"name":        item.name,
			"sequence":    item.sequence,
			"action_type": item.actionType,
			"active":      true,
		}
		for key, value := range item.extra {
			values[key] = value
		}
		if _, err := env.Model(ModelButton).Create(values); err != nil {
			t.Fatal(err)
		}
	}

	arch, err := ApplyApprovalViewMutation(env, "purchase.order", view.Form, `<form><header><field name="state"/></header><sheet><field name="name"/></sheet></form>`, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`icon="fa-thumbs-up"`, `icon="fa-thumbs-down"`, `icon="fa-print"`} {
		if !strings.Contains(arch, want) {
			t.Fatalf("arch missing fallback icon %q: %s", want, arch)
		}
	}
}
