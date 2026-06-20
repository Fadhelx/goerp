package data

import (
	"encoding/xml"
	"sort"
	"strings"

	"gorp/internal/model"
	"gorp/internal/record"
)

func LoadModelMetadata(env *record.Env, module string, models []model.Model, externalIDs map[string]ExternalID) error {
	if _, err := env.Model("ir.model").FieldsGet([]string{"model"}, nil); err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model") {
			return nil
		}
		return err
	}
	if _, err := env.Model("ir.model.fields").FieldsGet([]string{"model"}, nil); err != nil {
		if strings.Contains(err.Error(), "unknown model ir.model.fields") {
			return nil
		}
		return err
	}
	loader := NewLoaderWithExternalIDs(env, module, externalIDs)
	var b strings.Builder
	b.WriteString("<odoo><data noupdate=\"1\">")
	for _, item := range sortedModels(models) {
		modelXMLID := modelExternalID(item.Name)
		b.WriteString(`<record id="`)
		writeXMLText(&b, modelXMLID)
		b.WriteString(`" model="ir.model"><field name="model">`)
		writeXMLText(&b, item.Name)
		b.WriteString(`</field><field name="name">`)
		writeXMLText(&b, item.Name)
		b.WriteString(`</field><field name="abstract">`)
		writeXMLText(&b, boolText(item.Abstract))
		b.WriteString(`</field><field name="transient">`)
		writeXMLText(&b, boolText(item.Transient))
		b.WriteString(`</field><field name="is_mail_thread">`)
		writeXMLText(&b, boolText(modelInherits(item, "mail.thread")))
		b.WriteString(`</field><field name="is_mail_activity">`)
		writeXMLText(&b, boolText(modelInherits(item, "mail.activity.mixin")))
		b.WriteString(`</field></record>`)
		for _, fieldName := range sortedFieldNames(item) {
			f := item.Fields[fieldName]
			b.WriteString(`<record id="`)
			writeXMLText(&b, fieldExternalID(item.Name, fieldName))
			b.WriteString(`" model="ir.model.fields"><field name="model">`)
			writeXMLText(&b, item.Name)
			b.WriteString(`</field><field name="name">`)
			writeXMLText(&b, fieldName)
			b.WriteString(`</field><field name="ttype">`)
			writeXMLText(&b, string(f.Kind))
			if f.Relation != "" {
				b.WriteString(`</field><field name="relation">`)
				writeXMLText(&b, f.Relation)
			}
			if f.RelationField != "" {
				b.WriteString(`</field><field name="relation_field">`)
				writeXMLText(&b, f.RelationField)
			}
			if len(f.Groups) > 0 {
				b.WriteString(`</field><field name="groups">`)
				writeXMLText(&b, strings.Join(f.Groups, ","))
			}
			b.WriteString(`</field></record>`)
		}
	}
	b.WriteString("</data></odoo>")
	return loader.LoadXML(strings.NewReader(b.String()))
}

func boolText(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func modelInherits(item model.Model, inherited string) bool {
	if item.Name == inherited {
		return true
	}
	for _, name := range item.Inherit {
		if name == inherited {
			return true
		}
	}
	return false
}

func sortedModels(models []model.Model) []model.Model {
	out := append([]model.Model(nil), models...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func sortedFieldNames(item model.Model) []string {
	out := make([]string, 0, len(item.Fields))
	for name := range item.Fields {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func modelExternalID(modelName string) string {
	return "model_" + strings.ReplaceAll(modelName, ".", "_")
}

func fieldExternalID(modelName string, fieldName string) string {
	return "field_" + strings.ReplaceAll(modelName, ".", "_") + "__" + strings.ReplaceAll(fieldName, ".", "_")
}

func writeXMLText(b *strings.Builder, value string) {
	_ = xml.EscapeText(b, []byte(value))
}
