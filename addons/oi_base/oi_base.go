package oi_base

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/module"
	"gorp/internal/record"
	"gorp/internal/registry"
	"gorp/internal/security"
)

const (
	ModuleName = "oi_base"
	GroupUser  = 8101
	GroupAdmin = 8102

	ModelXMLIDMixin               = "xml_id.mixin"
	ModelMany2manyAttachmentResID = "many2many.attachment.res_id.mixin"
	ModelResGroups                = "res.groups"
	ModelResConfigSettings        = "res.config.settings"
	FieldXMLID                    = "xml_id"
	FieldInheritedByIDs           = "inherited_by_ids"
	FieldEnterprise               = "is_enterprise"
)

var xmlNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.-]*$`)

func Manifest() module.Manifest {
	return module.Manifest{
		Name:          "OI Base",
		TechnicalName: ModuleName,
		Version:       "19.0.1.0.0",
		Category:      "Technical",
		Depends:       []string{"base"},
		Data: []string{
			"security/oi_base_groups.xml",
			"security/ir.model.access.csv",
			"data/oi_sequence_data.xml",
			"view/res_groups.xml",
		},
		Assets: map[string][]string{
			"web.assets_backend": {
				"frontend/packages/oi-workflow/src/index.ts",
				"frontend/packages/oi-login-as/src/index.ts",
			},
		},
		Installable:   true,
		SourceVersion: "18.0.1.0.cleanroom",
		SourceLicense: "clean-room feature parity; source manifests were inspected, code/assets not copied",
	}
}

func DependencyManifests() []module.Manifest {
	return nil
}

func RegisterModels(reg *registry.Registry) error {
	for _, m := range Models() {
		if err := reg.RegisterModel(m); err != nil {
			return err
		}
	}
	return nil
}

func RegisterRecordModels(reg *record.Registry) error {
	for _, m := range Models() {
		if err := reg.Register(m); err != nil {
			return err
		}
	}
	return nil
}

func Models() []model.Model {
	return []model.Model{
		xmlIDMixinModel(),
		many2manyAttachmentResIDMixinModel(),
	}
}

func ExtensionModels() []model.Model {
	settings := extension(ModelResConfigSettings, "res_config_settings", computed(field.New(FieldEnterprise, field.Bool)))
	settings.Transient = true
	return []model.Model{
		extension(ModelResGroups, "res_groups", field.New(FieldInheritedByIDs, field.Many2Many).WithRelation(ModelResGroups)),
		settings,
	}
}

func ModelNames() []string {
	models := Models()
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	return names
}

func ApplySecurity(engine *security.Engine) {
	for _, group := range SecurityGroups() {
		engine.Groups[group.ID] = group
	}
}

func SecurityGroups() []security.Group {
	return []security.Group{
		{ID: GroupUser, Name: "OI Base / User"},
		{ID: GroupAdmin, Name: "OI Base / Administrator", ImpliedIDs: []int64{GroupUser}},
	}
}

type Sequence struct {
	mu      sync.Mutex
	Code    string
	Prefix  string
	Padding int
	next    int64
}

func NewSequence(code string, prefix string, padding int, start int64) *Sequence {
	if padding <= 0 {
		padding = 4
	}
	if start <= 0 {
		start = 1
	}
	return &Sequence{Code: code, Prefix: prefix, Padding: padding, next: start}
}

func (s *Sequence) Next() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := s.next
	s.next++
	return fmt.Sprintf("%s%0*d", s.Prefix, s.Padding, value)
}

type XMLID struct {
	Module string
	Name   string
}

func ParseXMLID(value string) (XMLID, error) {
	moduleName, name, ok := strings.Cut(strings.TrimSpace(value), ".")
	if !ok || moduleName == "" || name == "" {
		return XMLID{}, fmt.Errorf("xml id must be module.name")
	}
	if !xmlNamePattern.MatchString(moduleName) || !xmlNamePattern.MatchString(name) {
		return XMLID{}, fmt.Errorf("invalid xml id %q", value)
	}
	return XMLID{Module: moduleName, Name: name}, nil
}

func MustXMLID(moduleName string, name string) string {
	id := moduleName + "." + name
	if _, err := ParseXMLID(id); err != nil {
		panic(err)
	}
	return id
}

func Ping() map[string]string {
	return map[string]string{"status": "ok", "module": ModuleName}
}

func EffectiveGroupClosure(groups map[int64]security.Group, groupIDs []int64) []int64 {
	seen := map[int64]bool{}
	var visit func(int64)
	visit = func(id int64) {
		if seen[id] {
			return
		}
		seen[id] = true
		for _, implied := range groups[id].ImpliedIDs {
			visit(implied)
		}
	}
	for _, id := range groupIDs {
		visit(id)
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func InheritedByGroupIDs(impliedGroups map[int64][]int64, groupID int64) []int64 {
	seen := map[int64]bool{}
	for parentID, impliedIDs := range impliedGroups {
		for _, impliedID := range impliedIDs {
			if impliedID == groupID {
				seen[parentID] = true
				break
			}
		}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func XMLIDMixinValue(externalIDs map[int64][]string, recordID int64) string {
	return strings.Join(externalIDs[recordID], ",")
}

type AttachmentReference struct {
	ID       int64
	ResID    int64
	ResField string
}

func SetMany2manyAttachmentResID(recordID int64, fieldName string, attachments []AttachmentReference) []AttachmentReference {
	out := append([]AttachmentReference(nil), attachments...)
	for i := range out {
		out[i].ResID = recordID
		out[i].ResField = fieldName
	}
	return out
}

func IsEnterpriseVersionInfo(serverVersionInfo []any) bool {
	if len(serverVersionInfo) == 0 {
		return false
	}
	edition, ok := serverVersionInfo[len(serverVersionInfo)-1].(string)
	return ok && edition == "e"
}

func EscapeText(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&#34;",
		"'", "&#39;",
	)
	return replacer.Replace(value)
}

func NationalIDChecksum(value string) (int, error) {
	digits := make([]int, 0, len(value))
	for _, r := range value {
		if r < '0' || r > '9' {
			continue
		}
		digits = append(digits, int(r-'0'))
	}
	if len(digits) == 0 {
		return 0, fmt.Errorf("national id requires digits")
	}
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		n := digits[i]
		if double {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		double = !double
	}
	return sum % 10, nil
}

func ValidNationalID(value string) bool {
	checksum, err := NationalIDChecksum(value)
	return err == nil && checksum == 0
}

func xmlIDMixinModel() model.Model {
	m := model.New(ModelXMLIDMixin, "")
	m.Abstract = true
	m.AddField(computed(field.New(FieldXMLID, field.Char)))
	return m
}

func many2manyAttachmentResIDMixinModel() model.Model {
	m := model.New(ModelMany2manyAttachmentResID, "")
	m.Abstract = true
	return m
}

func extension(name string, table string, fields ...field.Field) model.Model {
	m := model.New(name, table)
	m.Inherit = []string{name}
	for _, f := range fields {
		m.AddField(f)
	}
	return m
}

func computed(f field.Field) field.Field {
	f.Store = false
	f.Readonly = true
	return f
}
