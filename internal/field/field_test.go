package field

import "testing"

func TestNewField(t *testing.T) {
	f := New("partner_id", Many2One).WithRelation("res.partner").WithGroups("base.group_user")
	if f.Name != "partner_id" || f.Kind != Many2One || f.Relation != "res.partner" {
		t.Fatalf("unexpected field: %+v", f)
	}
	if len(f.Groups) != 1 {
		t.Fatalf("groups = %+v", f.Groups)
	}
}
