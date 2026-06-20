package view

import (
	"strings"
	"testing"
)

func TestForModelFiltersGroups(t *testing.T) {
	reg := NewRegistry()
	if _, err := reg.Add(View{Name: "Public", Model: "res.partner", Type: Form, Arch: "<form/>"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Add(View{Name: "Private", Model: "res.partner", Type: Form, Arch: "<form/>", Groups: []int64{2}}); err != nil {
		t.Fatal(err)
	}
	views := reg.ForModel("res.partner", map[int64]bool{1: true})
	if len(views) != 1 || views[0].Name != "Public" {
		t.Fatalf("views = %+v", views)
	}

	if _, err := reg.Add(View{Name: "Not Managers", Model: "res.partner", Type: Form, Arch: "<form/>", NotGroups: []int64{9}}); err != nil {
		t.Fatal(err)
	}
	if views := reg.ForModel("res.partner", map[int64]bool{9: true}); len(views) != 1 || views[0].Name != "Public" {
		t.Fatalf("negative group views = %+v", views)
	}
	if views := reg.ForModel("res.partner", map[int64]bool{2: true}); len(views) != 3 {
		t.Fatalf("allowed views = %+v", views)
	}
}

func TestDefaultSortsByPriorityAndID(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 20, Name: "Later", Model: "res.partner", Type: Form, Arch: "<form/>", Priority: 20}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 10, Name: "Earlier", Model: "res.partner", Type: Form, Arch: "<form/>", Priority: 10}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 30, Name: "List", Model: "res.partner", Type: List, Arch: "<list/>", Priority: 1}); err != nil {
		t.Fatal(err)
	}

	views := reg.ForModelAndType("res.partner", Form, map[int64]bool{})
	if len(views) != 2 || views[0].ID != 10 || views[1].ID != 20 {
		t.Fatalf("ordered views = %+v", views)
	}
	def, ok := reg.Default("res.partner", Form, map[int64]bool{})
	if !ok || def.ID != 10 {
		t.Fatalf("default = %+v ok=%v", def, ok)
	}
}

func TestComposeAppliesFieldXPathPositions(t *testing.T) {
	cases := []struct {
		name        string
		spec        string
		contains    []string
		notContains []string
		order       []string
	}{
		{
			name:     "inside",
			spec:     `<xpath expr="//field[@name='name']" position="inside"><span class="inside"/></xpath>`,
			contains: []string{`name="name"`, `class="inside"`},
		},
		{
			name:     "before",
			spec:     `<xpath expr="//field[@name='name']" position="before"><label string="Before"/></xpath>`,
			contains: []string{`string="Before"`, `name="name"`},
			order:    []string{`string="Before"`, `name="name"`},
		},
		{
			name:     "after",
			spec:     `<xpath expr="//field[@name='name']" position="after"><label string="After"/></xpath>`,
			contains: []string{`name="name"`, `string="After"`},
			order:    []string{`name="name"`, `string="After"`},
		},
		{
			name:        "replace",
			spec:        `<xpath expr="//field[@name='name']" position="replace"><field name="display_name"/></xpath>`,
			contains:    []string{`<field name="display_name"`},
			notContains: []string{`name="name"`},
		},
		{
			name:     "attributes",
			spec:     `<xpath expr="//field[@name='name']" position="attributes"><attribute name="string">Partner Name</attribute><attribute name="readonly">1</attribute></xpath>`,
			contains: []string{`<field name="name"`, `string="Partner Name"`, `readonly="1"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><sheet><field name="name"/></sheet></form>`}); err != nil {
				t.Fatal(err)
			}
			if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: tc.spec}); err != nil {
				t.Fatal(err)
			}
			got, err := reg.Compose(10, map[int64]bool{})
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.contains {
				if !strings.Contains(got.Arch, want) {
					t.Fatalf("arch missing %q: %s", want, got.Arch)
				}
			}
			for _, notWant := range tc.notContains {
				if strings.Contains(got.Arch, notWant) {
					t.Fatalf("arch contains %q: %s", notWant, got.Arch)
				}
			}
			if len(tc.order) > 0 {
				assertArchOrder(t, got.Arch, tc.order...)
			}
		})
	}
}

func TestComposeAttributeAddRemoveWithSpaceSeparator(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" class="oe_inline old keep"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="class" add="new extra" remove="old" separator=" "/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `class="oe_inline keep new extra"`) || strings.Contains(got.Arch, `old`) {
		t.Fatalf("class add/remove arch = %s", got.Arch)
	}
}

func TestComposeAttributeEmptyGroupsRemovesAttribute(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" groups="base.group_user" string="Name"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="groups"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Arch, `groups=`) || !strings.Contains(got.Arch, `string="Name"`) {
		t.Fatalf("empty groups attribute arch = %s", got.Arch)
	}
}

func TestComposeAttributeExpressionAddRemoveWithOrSeparator(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" invisible="is_locked or is_archived"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="invisible" add="is_hidden" remove="is_locked" separator=" or "/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `is_archived`) || !strings.Contains(got.Arch, `is_hidden`) || !strings.Contains(got.Arch, ` or `) || strings.Contains(got.Arch, `is_locked`) {
		t.Fatalf("expression add/remove arch = %s", got.Arch)
	}
	assertArchOrder(t, got.Arch, `is_archived`, `is_hidden`)
}

func TestComposeAttributeTextContentWithAddRemoveReturnsError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" class="old"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="class" add="new" remove="old" separator=" ">bad</attribute></xpath>`}); err != nil {
		t.Fatal(err)
	}

	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("attribute text content with add/remove error = nil")
	}
}

func TestComposeAttributeUnknownAttributeReturnsError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" class="old"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="class" add="new" separator=" " unknown="1"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("unknown attribute error = nil")
	}
}

func TestComposeAttributeAddRemoveAndDelete(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" class="a b c" groups="base.group_user" invisible="old or base"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Attribute Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="class" remove="b" add="d e" separator=" "/><attribute name="groups"/><attribute name="invisible" remove="old" add="new_expr" separator="or"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="a c d e"`, `invisible="(base) or (new_expr)"`} {
		if !strings.Contains(got.Arch, want) {
			t.Fatalf("arch missing %q: %s", want, got.Arch)
		}
	}
	if strings.Contains(got.Arch, `groups=`) {
		t.Fatalf("groups attribute was not removed: %s", got.Arch)
	}
}

func TestComposeAttributeExactPythonRemoveDeletesAttribute(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" readonly="foo"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Attribute Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="readonly" remove="foo" separator="or"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Arch, `readonly=`) {
		t.Fatalf("readonly attribute was not removed: %s", got.Arch)
	}
}

func TestComposeAttributeEmptyAddRemoveUseDirectSet(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" class="old" string="Old"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Attribute Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="class" add="">direct class</attribute><attribute name="string" remove="">Direct String</attribute></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="direct class"`, `string="Direct String"`} {
		if !strings.Contains(got.Arch, want) {
			t.Fatalf("arch missing %q: %s", want, got.Arch)
		}
	}
}

func TestComposeAttributeInvalidSpecsReturnErrors(t *testing.T) {
	cases := []struct {
		name string
		spec string
	}{
		{
			name: "text with add",
			spec: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="class" add="extra">bad</attribute></xpath>`,
		},
		{
			name: "unknown attribute key",
			spec: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="class" unknown="1">extra</attribute></xpath>`,
		},
		{
			name: "invalid python separator",
			spec: `<xpath expr="//field[@name='name']" position="attributes"><attribute name="invisible" add="x" separator=" "/></xpath>`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name" class="base" invisible="old"/></form>`}); err != nil {
				t.Fatal(err)
			}
			if err := reg.AddWithID(View{ID: 11, Name: "Partner Bad Attribute Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: tc.spec}); err != nil {
				t.Fatal(err)
			}
			if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
				t.Fatal("attribute error = nil")
			}
		})
	}
}

func TestComposeReplaceInnerKeepsTargetElementAndAttributes(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="target" string="Target" class="keep"><field name="old"/><label string="Old"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="replace" mode="inner"><field name="new"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<group name="target"`, `string="Target"`, `class="keep"`, `name="new"`} {
		if !strings.Contains(got.Arch, want) {
			t.Fatalf("arch missing %q: %s", want, got.Arch)
		}
	}
	for _, notWant := range []string{`name="old"`, `string="Old"`} {
		if strings.Contains(got.Arch, notWant) {
			t.Fatalf("arch contains %q: %s", notWant, got.Arch)
		}
	}
}

func TestComposeReplaceInnerMovesPositionalAndAttributeSelectedChildren(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="source"><field name="first"/><field name="second"/><field name="third"/><field name="by_attr"/></group><group name="target" string="Target"><label string="old"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="replace" mode="inner"><xpath expr="//field[2]" position="move"/><xpath expr="//field[@name='by_attr']" position="move"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<group name="target"`, `name="first"`, `name="third"`, `name="second"`, `name="by_attr"`} {
		if !strings.Contains(got.Arch, want) {
			t.Fatalf("arch missing %q: %s", want, got.Arch)
		}
	}
	for _, name := range []string{`name="second"`, `name="by_attr"`} {
		if strings.Count(got.Arch, name) != 1 {
			t.Fatalf("field count mismatch for %q: %s", name, got.Arch)
		}
	}
	if strings.Contains(got.Arch, `string="old"`) {
		t.Fatalf("arch contains replaced child: %s", got.Arch)
	}
	assertArchOrder(t, got.Arch, `name="source"`, `name="first"`, `name="third"`, `name="target"`, `name="second"`, `name="by_attr"`)
}

func TestComposeReplaceInnerInvalidModeReturnsError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="target"><field name="old"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="replace" mode="invalid"><field name="new"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("invalid replace mode error = nil")
	}
}

func TestComposeXPathAbsolutePathWorks(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group><field name="x"/><field name="y"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="/form/group/field[@name='x']" position="after"><field name="z"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="x"`, `name="z"`, `name="y"`)
}

func TestComposeOrdersExtensionsByPriorityThenID(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 100, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name"/></form>`}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []View{
		{ID: 30, Name: "Priority 20", Model: "res.partner", Type: Form, InheritID: 100, Mode: "extension", Priority: 20, Arch: `<xpath expr="//field[@name='name']" position="inside"><span data-order="priority20"/></xpath>`},
		{ID: 20, Name: "Priority 10 ID 20", Model: "res.partner", Type: Form, InheritID: 100, Mode: "extension", Priority: 10, Arch: `<xpath expr="//field[@name='name']" position="inside"><span data-order="id20"/></xpath>`},
		{ID: 10, Name: "Priority 10 ID 10", Model: "res.partner", Type: Form, InheritID: 100, Mode: "extension", Priority: 10, Arch: `<xpath expr="//field[@name='name']" position="inside"><span data-order="id10"/></xpath>`},
	} {
		if err := reg.AddWithID(item); err != nil {
			t.Fatal(err)
		}
	}
	got, err := reg.Compose(100, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-order="id10"`, `data-order="id20"`, `data-order="priority20"`)
}

func TestComposePrimaryChildUsesParentSpecsAndChildExtensions(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 100, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group><field name="name"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 110, Name: "Parent Extension", Model: "res.partner", Type: Form, InheritID: 100, Mode: "extension", Priority: 10, Arch: `<xpath expr="//field[@name='name']" position="after"><field name="parent_extension"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 200, Name: "Delegated Partner Form", Model: "res.partner", Type: Form, InheritID: 100, Mode: "primary", Priority: 5, Arch: `<xpath expr="//field[@name='name']" position="after"><field name="primary_child"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 210, Name: "Delegated Extension", Model: "res.partner", Type: Form, InheritID: 200, Mode: "extension", Priority: 1, Arch: `<xpath expr="//field[@name='name']" position="before"><field name="child_extension"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	root, err := reg.Compose(100, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(root.Arch, `name="parent_extension"`) || strings.Contains(root.Arch, `name="primary_child"`) || strings.Contains(root.Arch, `name="child_extension"`) {
		t.Fatalf("root arch = %s", root.Arch)
	}

	primary, err := reg.Compose(200, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`name="name"`, `name="primary_child"`, `name="child_extension"`} {
		if !strings.Contains(primary.Arch, want) {
			t.Fatalf("primary arch missing %q: %s", want, primary.Arch)
		}
	}
}

func TestComposeMissingLocatorReturnsError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Bad Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='missing']" position="after"><field name="email"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("missing locator error = nil")
	}
}

func TestComposeMoveInsideTarget(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><sheet><group name="source"><field name="email"/><field name="phone"/></group><group name="target"><field name="name"/></group></sheet></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="inside"><xpath expr="//field[@name='email']" position="move"/></xpath>`}); err != nil {
		t.Fatal(err)
	}

	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="source"`, `name="phone"`, `name="target"`, `name="name"`, `name="email"`)
	if strings.Count(got.Arch, `name="email"`) != 1 {
		t.Fatalf("email field count mismatch: %s", got.Arch)
	}
}

func TestComposeMoveBeforeAndAfter(t *testing.T) {
	cases := []struct {
		name  string
		spec  string
		order []string
	}{
		{
			name:  "before",
			spec:  `<xpath expr="//field[@name='first']" position="before"><xpath expr="//field[@name='middle']" position="move"/></xpath>`,
			order: []string{`name="middle"`, `name="first"`, `name="last"`},
		},
		{
			name:  "after",
			spec:  `<xpath expr="//field[@name='last']" position="after"><xpath expr="//field[@name='middle']" position="move"/></xpath>`,
			order: []string{`name="first"`, `name="last"`, `name="middle"`},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group><field name="first"/><field name="middle"/><field name="last"/></group></form>`}); err != nil {
				t.Fatal(err)
			}
			if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: tc.spec}); err != nil {
				t.Fatal(err)
			}

			got, err := reg.Compose(10, map[int64]bool{})
			if err != nil {
				t.Fatal(err)
			}
			assertArchOrder(t, got.Arch, tc.order...)
			if strings.Count(got.Arch, `name="middle"`) != 1 {
				t.Fatalf("middle field count mismatch: %s", got.Arch)
			}
		})
	}
}

func TestComposeMoveMissingLocatorReturnsError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="after"><xpath expr="//field[@name='missing']" position="move"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("missing move locator error = nil")
	}
}

func TestComposeMoveWithChildContentReturnsError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name"/><field name="email"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Form Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="after"><xpath expr="//field[@name='email']" position="move"><field name="bad"/></xpath></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("move with child content error = nil")
	}
}

func TestComposeReplaceInnerKeepsTargetAndReplacesChildren(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="target" string="Target"><field name="old"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Replace Inner Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="replace" mode="inner"><field name="name"/><field name="email"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<group name="target" string="Target">`, `name="name"`, `name="email"`} {
		if !strings.Contains(got.Arch, want) {
			t.Fatalf("arch missing %q: %s", want, got.Arch)
		}
	}
	if strings.Contains(got.Arch, `name="old"`) {
		t.Fatalf("replace inner kept old child: %s", got.Arch)
	}
}

func TestComposeReplaceInnerMovesIndexedChildren(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="target"><field name="name"/><field name="email"><field name="email_child"/></field><field name="phone"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Replace Inner Move Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="replace" mode="inner"><field name="first"/><xpath expr="//field[2]" position="move"/><xpath expr="//field[@name='phone']" position="move"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="first"`, `name="email"`, `name="phone"`)
	if strings.Contains(got.Arch, `<field name="name"`) || strings.Count(got.Arch, `<field name="email"`) != 1 || !strings.Contains(got.Arch, `name="email_child"`) || strings.Count(got.Arch, `<field name="phone"`) != 1 {
		t.Fatalf("replace inner move arch = %s", got.Arch)
	}
}

func TestComposeXPathNumericPredicateUsesPerParentPosition(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="a"><field name="a1"/><field name="a2"/></group><group name="b"><field name="b1"/><field name="b2"/></group><group name="target"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Positional XPath Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="inside"><xpath expr="//field[2]" position="move"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="a1"`, `name="b1"`, `name="b2"`, `<group name="target"><field name="a2"/></group>`)
	if strings.Contains(got.Arch, `name="b2"/></group><group name="target"><field name="b2"`) || strings.Count(got.Arch, `name="a2"`) != 1 {
		t.Fatalf("numeric predicate arch = %s", got.Arch)
	}
}

func TestComposeXPathPositionPredicateUsesMatchedCandidateIndex(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><div class="active" data-id="first"/><div data-id="plain"/><div class="active" data-id="second"/><div class="active" data-id="third"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Active Position Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[hasclass('active')][2]" position="after"><span data-hit="active-second"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="first"`, `data-id="plain"`, `data-id="second"`, `data-hit="active-second"`, `data-id="third"`)
	if strings.Count(got.Arch, `data-hit="active-second"`) != 1 {
		t.Fatalf("position predicate arch = %s", got.Arch)
	}
}

func TestComposeXPathLastPredicateUsesMatchedCandidateIndexPerParent(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><div class="o-section" data-id="first"/><div data-id="plain"/><div class="o-section" data-id="second"/><group><div class="o-section" data-id="nested"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Last Section Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[hasclass('o-section')][last()]" position="after"><span data-hit="last-section"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="first"`, `data-id="plain"`, `data-id="second"`, `data-hit="last-section"`, `data-id="nested"`)
	if strings.Count(got.Arch, `data-hit="last-section"`) != 1 {
		t.Fatalf("last predicate arch = %s", got.Arch)
	}
}

func TestComposeXPathParenthesizedGlobalOrdinalPreservesQuotedOr(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Invoice Report", Model: "account.move", Type: Form, Arch: `<form><section name="first"><span t-out="o.l10n_mx_edi_cfdi_customer_rfc or o.partner_id.vat" data-id="first"/></section><section name="second"><span t-out="o.l10n_mx_edi_cfdi_customer_rfc or o.partner_id.vat" data-id="second"/></section><span t-out="o.partner_id.vat" data-id="vat-only"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Invoice Global Ordinal Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="(//span[@t-out='o.l10n_mx_edi_cfdi_customer_rfc or o.partner_id.vat'])[2]" position="after"><span data-hit="global-second"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="first"`, `data-id="second"`, `data-hit="global-second"`, `data-id="vat-only"`)
	if strings.Count(got.Arch, `data-hit="global-second"`) != 1 {
		t.Fatalf("global ordinal quoted or arch = %s", got.Arch)
	}
}

func TestComposeXPathParenthesizedGlobalLastDiffersFromPerParentLast(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="first"><div class="active" data-id="first"/><div class="active" data-id="second"/></group><group name="second"><div class="active" data-id="third"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Per Parent Last Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[hasclass('active')][last()]" position="after"><span data-hit="per-parent-last"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 12, Name: "Partner Global Last Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="(//div[hasclass('active')])[last()]" position="after"><span data-hit="global-last"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="first"`, `data-id="second"`, `data-hit="per-parent-last"`, `data-id="third"`, `data-hit="global-last"`)
	if strings.Count(got.Arch, `data-hit="per-parent-last"`) != 1 || strings.Count(got.Arch, `data-hit="global-last"`) != 1 {
		t.Fatalf("global last arch = %s", got.Arch)
	}
}

func TestComposeXPathUnionUsesDocumentOrderFirstMatch(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="y"/><field name="x"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Union Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='x'] | //field[@name='y']" position="after"><field name="union_marker"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="y"`, `name="union_marker"`, `name="x"`)
	if strings.Count(got.Arch, `name="union_marker"`) != 1 {
		t.Fatalf("union arch = %s", got.Arch)
	}
}

func TestComposeXPathGroupedOrdinalSupportsSuffixPath(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Cards", Model: "res.partner", Type: Form, Arch: `<form><div class="card"><p data-id="first"/></div><div class="card-wrapper"><p data-id="wrong-wrapper"/></div><div class="card"><p data-id="second"/></div><div class="card-wrapper"><p data-id="second-wrapper"/></div></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Card Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="(//div[hasclass('card')])[2]//p|(//div[hasclass('card-wrapper')])[2]//p" position="after"><p data-hit="grouped-suffix"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="second"`, `data-hit="grouped-suffix"`, `data-id="second-wrapper"`)
	if strings.Count(got.Arch, `data-hit="grouped-suffix"`) != 1 {
		t.Fatalf("grouped suffix arch = %s", got.Arch)
	}
}

func TestComposeXPathPositionPredicateMatchesInteriorDescendantPath(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Invoice Form", Model: "account.move", Type: Form, Arch: `<form><table name="invoice_line_table"><tbody><tr data-row="first"><td data-cell="first-a"/><td data-cell="first-b"><table><tbody><tr data-row="nested"><td data-cell="nested-a"/></tr></tbody></table></td></tr><tr data-row="second"><td data-cell="second-a"/></tr></tbody></table></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Invoice Cell Position Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//table[@name='invoice_line_table']/tbody//tr[1]//td[1]" position="after"><span data-hit="first-cell"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-cell="first-a"`, `data-hit="first-cell"`, `data-cell="first-b"`, `data-cell="nested-a"`, `data-row="second"`)
	if strings.Count(got.Arch, `data-hit="first-cell"`) != 1 {
		t.Fatalf("interior position predicate arch = %s", got.Arch)
	}
}

func TestComposeXPathContainsAttributePredicateBoundsDescendantLast(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Cart Form", Model: "sale.order", Type: Form, Arch: `<form><div t-attf-class="card o_cart_total shadow"><table><tr data-row="total-first"/><tr data-row="total-last"/></table></div><div t-attf-class="card o_cart_lines"><table><tr data-row="lines-last"/></table></div><div><table><tr data-row="plain-last"/></table></div></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Cart Total Extension", Model: "sale.order", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[contains(@t-attf-class, 'o_cart_total')]//table/tr[last()]" position="after"><tr data-hit="cart-total"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-row="total-last"`, `data-hit="cart-total"`, `data-row="lines-last"`, `data-row="plain-last"`)
	if strings.Count(got.Arch, `data-hit="cart-total"`) != 1 {
		t.Fatalf("contains attribute descendant arch = %s", got.Arch)
	}
}

func TestComposeXPathContainsTextPredicateSupportsParentClimb(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Summary Form", Model: "sale.order", Type: Form, Arch: `<form><section name="summary"><div><h3>Summary: Order</h3></div></section><section name="details"><div><h3>Details</h3></div></section></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Summary Extension", Model: "sale.order", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div/h3[contains(text(), 'Summary:')]/../.." position="inside"><span data-hit="summary-parent"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<section name="summary"><div><h3>Summary: Order</h3></div><span data-hit="summary-parent"/></section>`) || strings.Count(got.Arch, `data-hit="summary-parent"`) != 1 {
		t.Fatalf("contains text parent climb arch = %s", got.Arch)
	}
	assertArchOrder(t, got.Arch, `name="summary"`, `data-hit="summary-parent"`, `name="details"`)
}

func TestComposeXPathPredicateOrMatchesExistingParentField(t *testing.T) {
	cases := []struct {
		name       string
		parentName string
	}{
		{name: "journal line ids", parentName: "journal_line_ids"},
		{name: "line ids", parentName: "line_ids"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			if err := reg.AddWithID(View{ID: 10, Name: "Move Form", Model: "account.move", Type: Form, Arch: `<form><field name="other_line_ids"><tree><field name="account_id"/></tree></field><field name="` + tc.parentName + `"><tree><field name="account_id"/></tree></field></form>`}); err != nil {
				t.Fatal(err)
			}
			if err := reg.AddWithID(View{ID: 11, Name: "Move Line Account Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='journal_line_ids' or @name='line_ids']//field[@name='account_id']" position="after"><field name="account_marker"/></xpath>`}); err != nil {
				t.Fatal(err)
			}
			got, err := reg.Compose(10, map[int64]bool{})
			if err != nil {
				t.Fatal(err)
			}
			want := `<field name="` + tc.parentName + `"><tree><field name="account_id"/><field name="account_marker"/></tree></field>`
			if !strings.Contains(got.Arch, want) || strings.Contains(got.Arch, `<field name="other_line_ids"><tree><field name="account_id"/><field name="account_marker"/>`) || strings.Count(got.Arch, `name="account_marker"`) != 1 {
				t.Fatalf("or parent field arch = %s", got.Arch)
			}
		})
	}
}

func TestComposeXPathPredicateOrSelectsAddressAlternativeParent(t *testing.T) {
	cases := []struct {
		name        string
		addressName string
	}{
		{name: "not same as shipping", addressName: "address_not_same_as_shipping"},
		{name: "same as shipping", addressName: "address_same_as_shipping"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := NewRegistry()
			if err := reg.AddWithID(View{ID: 10, Name: "Invoice Report", Model: "account.move", Type: Form, Arch: `<form><div name="other_address"><p data-id="wrong"><span t-field="o.partner_id.vat"/></p></div><div name="` + tc.addressName + `"><p data-id="target"><span t-field="o.partner_id.vat"/></p></div></form>`}); err != nil {
				t.Fatal(err)
			}
			if err := reg.AddWithID(View{ID: 11, Name: "Invoice VAT Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[@name='address_not_same_as_shipping' or @name='address_same_as_shipping']//span[@t-field='o.partner_id.vat']/.." position="inside"><span data-hit="vat-parent"/></xpath>`}); err != nil {
				t.Fatal(err)
			}
			got, err := reg.Compose(10, map[int64]bool{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got.Arch, `<p data-id="target"><span t-field="o.partner_id.vat"/><span data-hit="vat-parent"/></p>`) || strings.Contains(got.Arch, `<p data-id="wrong"><span t-field="o.partner_id.vat"/><span data-hit="vat-parent"/></p>`) || strings.Count(got.Arch, `data-hit="vat-parent"`) != 1 {
				t.Fatalf("or address parent arch = %s", got.Arch)
			}
		})
	}
}

func TestComposeXPathUnsupportedComplexOrDoesNotTagOnlyMatch(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><div name="first"/><div name="second"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Complex Or Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[@name='missing' or contains(@class, 'missing')]" position="after"><span data-hit="complex-or"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("unsupported complex or predicate matched by tag only")
	}
}

func TestComposeXPathPredicateOrInsideQuotedValueIsLiteral(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Timesheet Report", Model: "account.analytic.line", Type: Form, Arch: `<form><td t-if="show_task"/><td t-if="show_task or show_project"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Timesheet Literal Or Extension", Model: "account.analytic.line", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//td[@t-if='show_task or show_project']" position="inside"><span data-hit="literal-or"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<td t-if="show_task or show_project"><span data-hit="literal-or"/></td>`) || strings.Count(got.Arch, `data-hit="literal-or"`) != 1 {
		t.Fatalf("literal or arch = %s", got.Arch)
	}
}

func TestComposeXPathContainsAttributePredicateDoesNotMatchMissingSubstring(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><span class="bar baz"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Missing Contains Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//span[contains(@class, 'foo')]" position="after"><span data-hit="foo"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("contains attribute predicate matched missing substring")
	}
}

func TestComposeXPathContainsClassTokenIdiom(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Dialog Form", Model: "res.partner", Type: Form, Arch: `<form><h4 class="modal-title-lg" data-id="partial"/><h4 class="modal-title text-truncate" data-id="target"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Dialog Title Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//h4[contains(concat(' ',normalize-space(@class),' '),' modal-title ')]" position="before"><span data-hit="title-token"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="partial"`, `data-hit="title-token"`, `data-id="target"`)
	if strings.Count(got.Arch, `data-hit="title-token"`) != 1 {
		t.Fatalf("contains class-token idiom arch = %s", got.Arch)
	}
}

func TestComposeXPathHasclassPredicateMatchesExactTokenAndDoubleQuotes(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><button name="primary" class="btn btn-primary"/><button name="secondary" class="btn btn-secondary"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Hasclass Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//button[hasclass('btn-primary')]" position="after"><span data-hit="single"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 12, Name: "Partner Hasclass Double Quote Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr='//button[hasclass("btn-secondary")]' position="after"><span data-hit="double"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="primary"`, `data-hit="single"`, `name="secondary"`, `data-hit="double"`)
}

func TestComposeXPathHasclassPredicateDoesNotMatchPartialToken(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><button name="partial" class="btn-primary-outline"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Partial Hasclass Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//button[hasclass('btn-primary')]" position="after"><span data-hit="partial"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("partial class token matched")
	}
}

func TestComposeXPathHasclassPredicateCombinesWithAttributePredicate(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><a name="x" class="btn btn-secondary" data-id="wrong-class"/><a name="y" class="btn btn-primary" data-id="wrong-name"/><a name="x" class="btn btn-primary" data-id="target"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Combined Hasclass Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//a[@name='x'][hasclass('btn-primary')]" position="after"><span data-hit="combined"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="target"`, `data-hit="combined"`)
	if strings.Count(got.Arch, `data-hit="combined"`) != 1 {
		t.Fatalf("combined hasclass arch = %s", got.Arch)
	}
}

func TestComposeXPathHasclassPredicateSupportsMultiArgAndAndNotForms(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><div class="page o_content_pdf" data-id="multi"/><div class="w-100 p-2 px-3" data-id="and"/><button name="action_send_mail" class="btn-secondary" data-id="not-target"/><button name="action_send_mail" class="o_mail_send" data-id="not-skip"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Multi Hasclass Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[hasclass('page', 'o_content_pdf')]" position="after"><span data-hit="multi"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 12, Name: "And Hasclass Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[hasclass('w-100') and hasclass('p-2') and hasclass('px-3')]" position="after"><span data-hit="and"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 13, Name: "Not Hasclass Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//button[@name='action_send_mail'][not(hasclass('o_mail_send'))]" position="after"><span data-hit="not"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="multi"`, `data-hit="multi"`, `data-id="and"`, `data-hit="and"`, `data-id="not-target"`, `data-hit="not"`, `data-id="not-skip"`)
	if strings.Count(got.Arch, `data-hit=`) != 3 {
		t.Fatalf("hasclass variants arch = %s", got.Arch)
	}
}

func TestComposeXPathDescendantAxisMatchesNestedHasclassPath(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><sheet><footer><div class="d-flex ms-auto"><group><field name="state"/></group></div><div class="d-flex"><field name="other"/></div></footer></sheet></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Descendant Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//footer/div[hasclass('d-flex', 'ms-auto')]//field[@name='state']" position="after"><field name="state_marker"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `class="d-flex ms-auto"`, `name="state"`, `name="state_marker"`, `name="other"`)
	if strings.Count(got.Arch, `name="state_marker"`) != 1 {
		t.Fatalf("descendant hasclass arch = %s", got.Arch)
	}
}

func TestComposeXPathDescendantAxisMatchesAbsoluteInteriorPath(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><sheet><group><div><field name="target"/></div></group><field name="sibling"/></sheet></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Absolute Descendant Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="/form/sheet//field[@name='target']" position="after"><field name="target_marker"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="target"`, `name="target_marker"`, `name="sibling"`)
	if strings.Count(got.Arch, `name="target_marker"`) != 1 {
		t.Fatalf("absolute descendant arch = %s", got.Arch)
	}
}

func TestComposeXPathDescendantAxisPreservesDirectChildSlash(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><sheet><footer><div name="target"><group><field name="state"/></group><field name="state"/></div></footer></sheet></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Direct Child Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//footer/div[@name='target']/field[@name='state']" position="attributes"><attribute name="string">Direct State</attribute></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<group><field name="state"/></group><field name="state" string="Direct State"/>`) || strings.Count(got.Arch, `string="Direct State"`) != 1 {
		t.Fatalf("direct child slash arch = %s", got.Arch)
	}
}

func TestComposeXPathFollowingSiblingAxisSelectsFirstMatchingSibling(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><div class="oe_structure" data-id="source"/><span data-id="skip"/><div data-id="target"/><div data-id="next"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Following Sibling Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[hasclass('oe_structure')]/following-sibling::div[1]" position="after"><span data-hit="following-sibling"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-id="source"`, `data-id="skip"`, `data-id="target"`, `data-hit="following-sibling"`, `data-id="next"`)
	if strings.Count(got.Arch, `data-hit="following-sibling"`) != 1 {
		t.Fatalf("following sibling arch = %s", got.Arch)
	}
}

func TestComposeXPathAncestorAxisBoundsChildSelectionToMatchedAncestor(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Invoice Form", Model: "account.move", Type: Form, Arch: `<form><table><tr><td data-cell="target"><span data-id="first-child"/><div><Field name="line"><span data-id="inside-field"/></Field></div><span data-id="last-child"/></td><td data-cell="other"><span data-id="other-first"/></td></tr></table></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Invoice Ancestor Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//Field//ancestor::td/*[1]" position="after"><span data-hit="ancestor-first-child"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `data-cell="target"`, `data-id="first-child"`, `data-hit="ancestor-first-child"`, `data-id="inside-field"`, `data-id="last-child"`, `data-cell="other"`)
	if strings.Count(got.Arch, `data-hit="ancestor-first-child"`) != 1 || strings.Contains(got.Arch, `<td data-cell="other"><span data-id="other-first"/><span data-hit="ancestor-first-child"/>`) {
		t.Fatalf("ancestor first child arch = %s", got.Arch)
	}
}

func TestComposeXPathParentAxisSelectsImmediateMatchingParent(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="target"><field name="email"/></group><group name="other"><field name="phone"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Parent Axis Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='email']/parent::group" position="inside"><field name="parent_axis_marker"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<group name="target"><field name="email"/><field name="parent_axis_marker"/></group>`) || strings.Count(got.Arch, `name="parent_axis_marker"`) != 1 {
		t.Fatalf("parent axis arch = %s", got.Arch)
	}
	assertArchOrder(t, got.Arch, `name="target"`, `name="parent_axis_marker"`, `name="other"`)
}

func TestComposeXPathUnsupportedExplicitAxisDoesNotTagOnlyMatch(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group name="target"><field name="nested"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Unsupported Axis Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']/unsupported::field" position="after"><field name="axis_marker"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("unsupported explicit axis matched by tag only")
	}
}

func TestComposeXPathNodePredicateRequiresChildNode(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "MPS Search", Model: "mrp.production.schedule", Type: Search, Arch: `<Search><Dropdown><t t-set-slot="content"><t/><t data-id="target"><span data-id="target-child"/></t><t data-id="next"><span data-id="next-child"/></t></t></Dropdown></Search>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "MPS Content Extension", Model: "mrp.production.schedule", Type: Search, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//Dropdown/t[@t-set-slot='content']/t[node()]" position="before"><t data-hit="node-predicate"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `<t/>`, `data-hit="node-predicate"`, `data-id="target"`, `data-id="next"`)
	if strings.Count(got.Arch, `data-hit="node-predicate"`) != 1 {
		t.Fatalf("node predicate arch = %s", got.Arch)
	}
}

func TestComposeXPathLocalNamePredicateBoundsWildcardSteps(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "JPK Export", Model: "account.move", Type: Form, Arch: `<root><Other><Naglowek><KodFormularza id="wrong"/></Naglowek></Other><JPK><Naglowek><KodFormularza id="target"/></Naglowek></JPK></root>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "JPK Export Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//*[local-name()='JPK']/*[local-name()='Naglowek']/*[local-name()='KodFormularza']" position="replace"><KodFormularza id="replacement"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<Other><Naglowek><KodFormularza id="wrong"/></Naglowek></Other>`) || !strings.Contains(got.Arch, `<JPK><Naglowek><KodFormularza id="replacement"/></Naglowek></JPK>`) {
		t.Fatalf("local-name arch = %s", got.Arch)
	}
	if strings.Contains(got.Arch, `id="target"`) || strings.Count(got.Arch, `id="replacement"`) != 1 {
		t.Fatalf("local-name replacement arch = %s", got.Arch)
	}
}

func TestComposeXPathCurrentNodeSupportsInsideAndReplace(t *testing.T) {
	t.Run("inside", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Payment Template", Model: "payment.provider", Type: Form, Arch: `<t t-name="payment.base"><div data-id="body"/></t>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Payment Template Extension", Model: "payment.provider", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="." position="inside"><span data-hit="current-inside"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Arch, `<t t-name="payment.base"><div data-id="body"/><span data-hit="current-inside"/></t>`) {
			t.Fatalf("current-node inside arch = %s", got.Arch)
		}
	})
	t.Run("replace", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Payment Template", Model: "payment.provider", Type: Form, Arch: `<t t-name="payment.base"><div data-id="body"/></t>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Payment Template Replacement", Model: "payment.provider", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="." position="replace"><t t-name="payment.base"><section data-hit="current-replace"/></t></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Arch != `<t t-name="payment.base"><section data-hit="current-replace"/></t>` {
			t.Fatalf("current-node replace arch = %s", got.Arch)
		}
	})
}

func TestComposeXPathAttributePresenceAndNegationPredicates(t *testing.T) {
	t.Run("attribute presence", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Hierarchy Card", Model: "hr.employee", Type: Form, Arch: `<form><button name="hierarchy_search_subsidiaries"><t/><t t-if="employee"><span data-id="target"/></t></button></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Hierarchy Card Extension", Model: "hr.employee", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//button[@name='hierarchy_search_subsidiaries']/t[@t-if]" position="after"><t data-hit="attr-present"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		assertArchOrder(t, got.Arch, `<t/>`, `t-if="employee"`, `data-hit="attr-present"`)
		if strings.Count(got.Arch, `data-hit="attr-present"`) != 1 {
			t.Fatalf("attribute presence arch = %s", got.Arch)
		}
	})
	t.Run("not attribute presence", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "CRM Lead", Model: "crm.lead", Type: Form, Arch: `<form><group name="named"><field name="tag_ids"/></group><group><field name="tag_ids"/></group></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "CRM Lead Extension", Model: "crm.lead", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[not(@name)]/field[@name='tag_ids']" position="after"><field name="unnamed_group_marker"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Arch, `<group><field name="tag_ids"/><field name="unnamed_group_marker"/></group>`) || strings.Count(got.Arch, `name="unnamed_group_marker"`) != 1 {
			t.Fatalf("not attr presence arch = %s", got.Arch)
		}
	})
	t.Run("not attribute equality with position", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Purchase Order", Model: "purchase.order", Type: Form, Arch: `<form><field name="currency_id" invisible="1"/><field name="currency_id" invisible="0"/><field name="currency_id"/></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Purchase Order Extension", Model: "purchase.order", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='currency_id' and not(@invisible='1')][1]" position="after"><field name="currency_marker"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		assertArchOrder(t, got.Arch, `invisible="1"`, `invisible="0"`, `name="currency_marker"`)
		if strings.Count(got.Arch, `name="currency_marker"`) != 1 {
			t.Fatalf("not attr equality arch = %s", got.Arch)
		}
	})
}

func TestComposeXPathRelativeChildPredicates(t *testing.T) {
	t.Run("immediate child path", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Disbursement Report", Model: "account.move", Type: Form, Arch: `<form><div data-id="wrong"><span/></div><div data-id="target"><table name="invoices"/></div></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Disbursement Report Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[table[@name='invoices']]" position="inside"><span data-hit="child-path"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Arch, `<div data-id="target"><table name="invoices"/><span data-hit="child-path"/></div>`) || strings.Count(got.Arch, `data-hit="child-path"`) != 1 {
			t.Fatalf("relative child path arch = %s", got.Arch)
		}
	})
	t.Run("nested child predicate", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Voucher Report", Model: "account.move", Type: Form, Arch: `<form><div class="row" data-id="wrong"><div class="col-6"/></div><div class="row" data-id="target"><div class="col-6" t-if="o.memo"/></div></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Voucher Report Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[hasclass('row')][div[hasclass('col-6')][@t-if='o.memo']]" position="inside"><span data-hit="nested-child"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Arch, `<div class="row" data-id="target"><div class="col-6" t-if="o.memo"/><span data-hit="nested-child"/></div>`) || strings.Count(got.Arch, `data-hit="nested-child"`) != 1 {
			t.Fatalf("nested child predicate arch = %s", got.Arch)
		}
	})
	t.Run("explicit current child path", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Appointment Event", Model: "calendar.event", Type: Form, Arch: `<form><div data-id="wrong"><field name="name"/></div><div data-id="target"><field name="partner_ids"/></div></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Appointment Event Extension", Model: "calendar.event", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[./field[@name='partner_ids']]" position="inside"><span data-hit="dot-child"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Arch, `<div data-id="target"><field name="partner_ids"/><span data-hit="dot-child"/></div>`) || strings.Count(got.Arch, `data-hit="dot-child"`) != 1 {
			t.Fatalf("dot child predicate arch = %s", got.Arch)
		}
	})
	t.Run("child attribute equality", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Check Printing Settings", Model: "res.config.settings", Type: Form, Arch: `<form><div data-id="wrong"><field name="other"/></div><div data-id="target"><field name="account_check_printing_margin_top"/></div></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Check Printing Settings Extension", Model: "res.config.settings", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//div[field/@name = 'account_check_printing_margin_top']" position="inside"><span data-hit="child-attr"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Arch, `<div data-id="target"><field name="account_check_printing_margin_top"/><span data-hit="child-attr"/></div>`) || strings.Count(got.Arch, `data-hit="child-attr"`) != 1 {
			t.Fatalf("child attr predicate arch = %s", got.Arch)
		}
	})
	t.Run("wildcard child quoted attribute", func(t *testing.T) {
		reg := NewRegistry()
		if err := reg.AddWithID(View{ID: 10, Name: "Dynamic Snippet", Model: "website.snippet", Type: Form, Arch: `<form><BuilderRow data-id="wrong"><span id="template_opt"/></BuilderRow><BuilderRow data-id="target"><span id="'template_opt'"/></BuilderRow></form>`}); err != nil {
			t.Fatal(err)
		}
		if err := reg.AddWithID(View{ID: 11, Name: "Dynamic Snippet Extension", Model: "website.snippet", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//BuilderRow[*[@id=&quot;'template_opt'&quot;]]" position="inside"><span data-hit="quoted-child-attr"/></xpath>`}); err != nil {
			t.Fatal(err)
		}
		got, err := reg.Compose(10, map[int64]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Arch, `<BuilderRow data-id="target"><span id="&#39;template_opt&#39;"/><span data-hit="quoted-child-attr"/></BuilderRow>`) || strings.Count(got.Arch, `data-hit="quoted-child-attr"`) != 1 {
			t.Fatalf("quoted child attr predicate arch = %s", got.Arch)
		}
	})
}

func TestComposeXPathParentStepSelectsParentCell(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Invoice Form", Model: "account.move", Type: Form, Arch: `<form><table><tbody><tr><td name="price"><span t-field="line.price_unit"/></td><td name="qty"><span t-field="line.quantity"/></td></tr></tbody></table></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Invoice Price Parent Extension", Model: "account.move", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//span[@t-field='line.price_unit']/.." position="inside"><span data-hit="price-parent"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<td name="price"><span t-field="line.price_unit"/><span data-hit="price-parent"/></td>`) || strings.Count(got.Arch, `data-hit="price-parent"`) != 1 {
		t.Fatalf("parent cell arch = %s", got.Arch)
	}
	assertArchOrder(t, got.Arch, `name="price"`, `data-hit="price-parent"`, `name="qty"`)
}

func TestComposeXPathParentStepThenDirectChildPredicate(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><sheet><group name="first"><field name="parent_id"/><group name="nested"/></group><group name="target"><field name="target"/></group><group name="third"><field name="third"/></group></sheet></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Parent Climb Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='parent_id']/../../group[2]" position="inside"><field name="target_marker"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<group name="target"><field name="target"/><field name="target_marker"/></group>`) || strings.Count(got.Arch, `name="target_marker"`) != 1 {
		t.Fatalf("parent climb direct child arch = %s", got.Arch)
	}
	assertArchOrder(t, got.Arch, `name="nested"`, `name="target"`, `name="target_marker"`, `name="third"`)
}

func TestComposeXPathParentStepDeduplicatesAndPreservesDocumentOrder(t *testing.T) {
	root, err := parseXMLDocument(`<form><group name="first"><field name="first_a"/><field name="first_b"/></group><group name="second"><field name="second_a"/><field name="second_b"/></group></form>`)
	if err != nil {
		t.Fatal(err)
	}
	refs := findXPathRefs(root, "//field/..")
	if len(refs) != 2 || attrValue(refs[0].Node, "name") != "first" || attrValue(refs[1].Node, "name") != "second" {
		t.Fatalf("parent refs = %+v", refs)
	}
}

func TestComposeReplaceInvalidModeReturnsError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><field name="name"/></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Bad Replace Mode Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//field[@name='name']" position="replace" mode="bad"><field name="email"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.Compose(10, map[int64]bool{}); err == nil {
		t.Fatal("invalid replace mode error = nil")
	}
}

func TestComposeAbsoluteXPathWithAttributePredicate(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><group><field name="name"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Absolute XPath Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="/form/group/field[@name='name']" position="after"><field name="email"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	assertArchOrder(t, got.Arch, `name="name"`, `name="email"`)
}

func TestComposeAbsoluteXPathMoveInsideReplaceFindsSourceRootPath(t *testing.T) {
	reg := NewRegistry()
	if err := reg.AddWithID(View{ID: 10, Name: "Partner Form", Model: "res.partner", Type: Form, Arch: `<form><div><div><span name="moving"/></div></div><group name="target"><field name="old"/></group></form>`}); err != nil {
		t.Fatal(err)
	}
	if err := reg.AddWithID(View{ID: 11, Name: "Partner Absolute Move Extension", Model: "res.partner", Type: Form, InheritID: 10, Mode: "extension", Arch: `<xpath expr="//group[@name='target']" position="replace" mode="inner"><xpath expr="/form/div/div/span[@name='moving']" position="move"/></xpath>`}); err != nil {
		t.Fatal(err)
	}
	got, err := reg.Compose(10, map[int64]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Arch, `<group name="target"><span name="moving"/></group>`) || strings.Contains(got.Arch, `name="old"`) || strings.Count(got.Arch, `name="moving"`) != 1 {
		t.Fatalf("absolute move arch = %s", got.Arch)
	}
}

func assertArchOrder(t *testing.T, arch string, parts ...string) {
	t.Helper()
	last := -1
	for _, part := range parts {
		index := strings.Index(arch, part)
		if index < 0 {
			t.Fatalf("arch missing %q: %s", part, arch)
		}
		if index <= last {
			t.Fatalf("arch order mismatch for %q: %s", part, arch)
		}
		last = index
	}
}
