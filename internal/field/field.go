package field

type Kind string

const (
	Bool                 Kind = "bool"
	Int                  Kind = "int"
	Float                Kind = "float"
	Decimal              Kind = "decimal"
	Monetary             Kind = "monetary"
	Char                 Kind = "char"
	Text                 Kind = "text"
	Date                 Kind = "date"
	DateTime             Kind = "datetime"
	Selection            Kind = "selection"
	Binary               Kind = "binary"
	Json                 Kind = "json"
	Properties           Kind = "properties"
	PropertiesDefinition Kind = "properties_definition"
	Many2One             Kind = "many2one"
	One2Many             Kind = "one2many"
	Many2Many            Kind = "many2many"
	Computed             Kind = "computed"
	Related              Kind = "related"
)

type Field struct {
	Name             string
	Label            string
	Kind             Kind
	Relation         string
	RelationField    string
	Required         bool
	Readonly         bool
	Index            bool
	Translate        bool
	Store            bool
	Aggregator       string
	CurrencyField    string
	DefaultExport    bool
	CompanyDependent bool
	DefinitionRecord string
	DefinitionField  string
	Context          map[string]any
	Groups           []string
	Selection        []SelectionOption
}

type SelectionOption struct {
	Value string
	Label string
}

func New(name string, kind Kind) Field {
	return Field{Name: name, Label: name, Kind: kind, Store: true}
}

func (f Field) WithRelation(model string) Field {
	f.Relation = model
	return f
}

func (f Field) WithRelationField(name string) Field {
	f.RelationField = name
	return f
}

func (f Field) WithContext(values map[string]any) Field {
	f.Context = make(map[string]any, len(values))
	for key, value := range values {
		f.Context[key] = value
	}
	return f
}

func (f Field) WithGroups(groups ...string) Field {
	f.Groups = append([]string(nil), groups...)
	return f
}

func (f Field) WithAggregator(aggregator string) Field {
	f.Aggregator = aggregator
	return f
}

func (f Field) WithCurrencyField(name string) Field {
	f.CurrencyField = name
	return f
}

func (f Field) WithDefaultExportCompatible() Field {
	f.DefaultExport = true
	return f
}

func (f Field) WithPropertyDefinition(recordField string, definitionField string) Field {
	f.DefinitionRecord = recordField
	f.DefinitionField = definitionField
	return f
}
