package view

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Type string

const (
	Form   Type = "form"
	List   Type = "list"
	Search Type = "search"
	Kanban Type = "kanban"
)

type View struct {
	ID        int64
	Name      string
	Model     string
	Type      Type
	Arch      string
	InheritID int64
	Mode      string
	Primary   bool
	Priority  int
	Groups    []int64
	NotGroups []int64
}

type Registry struct {
	nextID int64
	views  map[int64]View
}

func NewRegistry() *Registry {
	return &Registry{nextID: 1, views: map[int64]View{}}
}

func (r *Registry) Add(view View) (int64, error) {
	if view.Name == "" || view.Model == "" || view.Type == "" {
		return 0, fmt.Errorf("view requires name, model, and type")
	}
	id := r.nextID
	r.nextID++
	view.ID = id
	r.views[id] = view
	return id, nil
}

func (r *Registry) AddWithID(view View) error {
	if view.ID <= 0 {
		return fmt.Errorf("view requires id")
	}
	if view.Name == "" || view.Model == "" || view.Type == "" {
		return fmt.Errorf("view requires name, model, and type")
	}
	r.views[view.ID] = view
	if view.ID >= r.nextID {
		r.nextID = view.ID + 1
	}
	return nil
}

func (r *Registry) Get(id int64) (View, bool) {
	view, ok := r.views[id]
	return view, ok
}

func (r *Registry) ForModel(model string, groups map[int64]bool) []View {
	var out []View
	for _, view := range r.views {
		if view.Model == model && view.Allowed(groups) {
			out = append(out, view)
		}
	}
	sortViews(out)
	return out
}

func (r *Registry) ForModelAndType(model string, typ Type, groups map[int64]bool) []View {
	var out []View
	for _, view := range r.views {
		if view.Model == model && sameType(view.Type, typ) && view.Allowed(groups) {
			out = append(out, view)
		}
	}
	sortViews(out)
	return out
}

func (r *Registry) Default(model string, typ Type, groups map[int64]bool) (View, bool) {
	views := r.ForModelAndType(model, typ, groups)
	for _, view := range views {
		if view.Selectable() {
			return view, true
		}
	}
	return View{}, false
}

func (r *Registry) CombinedView(id int64, groups map[int64]bool) (View, error) {
	view, ok := r.Get(id)
	if !ok {
		return View{}, fmt.Errorf("view %d not found", id)
	}
	arch, err := r.CombinedArch(id, groups)
	if err != nil {
		return View{}, err
	}
	view.Arch = arch
	return view, nil
}

func (r *Registry) Compose(id int64, groups map[int64]bool) (View, error) {
	return r.CombinedView(id, groups)
}

func (r *Registry) CombinedArch(id int64, groups map[int64]bool) (string, error) {
	view, ok := r.Get(id)
	if !ok {
		return "", fmt.Errorf("view %d not found", id)
	}
	if !view.Allowed(groups) {
		return "", fmt.Errorf("view %d is not available for current user groups", id)
	}
	return r.combinedArch(view, groups, map[int64]bool{}, 0)
}

func (r *Registry) ForModelComposed(model string, groups map[int64]bool) ([]View, error) {
	views := r.ForModel(model, groups)
	out := make([]View, 0, len(views))
	for _, item := range views {
		if !item.Selectable() {
			continue
		}
		combined, err := r.CombinedView(item.ID, groups)
		if err != nil {
			return nil, err
		}
		out = append(out, combined)
	}
	return out, nil
}

func (r *Registry) combinedArch(item View, groups map[int64]bool, stack map[int64]bool, skipChildID int64) (string, error) {
	if stack[item.ID] {
		return "", fmt.Errorf("cyclic view inheritance at view %d", item.ID)
	}
	stack[item.ID] = true
	defer delete(stack, item.ID)

	var arch string
	if item.InheritID != 0 {
		parent, ok := r.Get(item.InheritID)
		if !ok {
			return "", fmt.Errorf("view %d inherits missing view %d", item.ID, item.InheritID)
		}
		if !parent.Allowed(groups) {
			return "", fmt.Errorf("view %d inherits unavailable view %d", item.ID, item.InheritID)
		}
		parentSkip := int64(0)
		if item.Extension() {
			parentSkip = item.ID
		}
		parentArch, err := r.combinedArch(parent, groups, stack, parentSkip)
		if err != nil {
			return "", err
		}
		applied, err := applyInheritanceSpecs(parentArch, item.Arch)
		if err != nil {
			return "", fmt.Errorf("view %d: %w", item.ID, err)
		}
		arch = applied
	} else {
		arch = item.Arch
	}

	for _, child := range r.extensionChildren(item.ID, item.Model, item.Type, groups, skipChildID) {
		applied, err := r.applyExtensionTree(arch, child, groups, stack)
		if err != nil {
			return "", err
		}
		arch = applied
	}
	return arch, nil
}

func (r *Registry) applyExtensionTree(baseArch string, item View, groups map[int64]bool, stack map[int64]bool) (string, error) {
	if stack[item.ID] {
		return "", fmt.Errorf("cyclic view inheritance at view %d", item.ID)
	}
	stack[item.ID] = true
	defer delete(stack, item.ID)

	arch, err := applyInheritanceSpecs(baseArch, item.Arch)
	if err != nil {
		return "", fmt.Errorf("view %d: %w", item.ID, err)
	}
	for _, child := range r.extensionChildren(item.ID, item.Model, item.Type, groups, 0) {
		arch, err = r.applyExtensionTree(arch, child, groups, stack)
		if err != nil {
			return "", err
		}
	}
	return arch, nil
}

func (r *Registry) extensionChildren(parentID int64, model string, typ Type, groups map[int64]bool, skipID int64) []View {
	var out []View
	for _, item := range r.views {
		if item.ID == skipID || item.InheritID != parentID || !item.Extension() || !item.Allowed(groups) {
			continue
		}
		if item.Model != model || !sameType(item.Type, typ) {
			continue
		}
		out = append(out, item)
	}
	sortViews(out)
	return out
}

func (view View) Allowed(groups map[int64]bool) bool {
	return allowed(view.Groups, view.NotGroups, groups)
}

func (view View) Selectable() bool {
	return !view.Extension()
}

func (view View) Extension() bool {
	return view.EffectiveMode() == "extension"
}

func (view View) EffectiveMode() string {
	mode := strings.ToLower(strings.TrimSpace(view.Mode))
	if mode != "" {
		return mode
	}
	if view.Primary {
		return "primary"
	}
	if view.InheritID != 0 {
		return "extension"
	}
	return "primary"
}

func allowed(required []int64, excluded []int64, groups map[int64]bool) bool {
	for _, group := range excluded {
		if groups[group] {
			return false
		}
	}
	if len(required) == 0 {
		return true
	}
	for _, group := range required {
		if groups[group] {
			return true
		}
	}
	return false
}

func sortViews(views []View) {
	sort.SliceStable(views, func(i, j int) bool {
		left := views[i]
		right := views[j]
		if left.Priority != right.Priority {
			return left.Priority < right.Priority
		}
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		return left.Name < right.Name
	})
}

func sameType(left Type, right Type) bool {
	return strings.TrimSpace(string(left)) == strings.TrimSpace(string(right))
}

type xmlNode struct {
	Name     string
	Attrs    []xml.Attr
	Children []*xmlNode
	Text     string
}

type xmlNodeRef struct {
	Node   *xmlNode
	Parent *xmlNode
	Index  int
}

type xpathAttrPredicate struct {
	Name  string
	Value string
}

type xpathAttrOrPredicate struct {
	Options []xpathAttrPredicate
}

type xpathContainsPredicate struct {
	Name  string
	Value string
}

type xpathPathSegment struct {
	Raw        string
	Descendant bool
}

type xpathStep struct {
	Tag              string
	Attrs            []xpathAttrPredicate
	AttrOrs          []xpathAttrOrPredicate
	AttrExists       []string
	AttrNotExists    []string
	AttrNotEquals    []xpathAttrPredicate
	AttrContains     []xpathContainsPredicate
	TextContains     []string
	ClassNames       []string
	NotClassNameSets [][]string
	LocalName        string
	HasNode          bool
	RelativePaths    []string
	Position         int
	Last             bool
	Unsupported      bool
}

func applyInheritanceSpecs(baseArch string, specsArch string) (string, error) {
	if strings.TrimSpace(specsArch) == "" {
		return baseArch, nil
	}
	root, err := parseXMLDocument(baseArch)
	if err != nil {
		return "", fmt.Errorf("parse base arch: %w", err)
	}
	specRoot, err := parseXMLFragment(specsArch)
	if err != nil {
		return "", fmt.Errorf("parse inherited arch: %w", err)
	}
	for _, spec := range inheritanceSpecNodes(specRoot) {
		if err := applyInheritanceSpec(root, spec); err != nil {
			return "", err
		}
	}
	return renderXML(root), nil
}

func parseXMLDocument(raw string) (*xmlNode, error) {
	fragment, err := parseXMLFragment(raw)
	if err != nil {
		return nil, err
	}
	var roots []*xmlNode
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

func parseXMLFragment(raw string) (*xmlNode, error) {
	decoder := xml.NewDecoder(strings.NewReader("<__root__>" + raw + "</__root__>"))
	var stack []*xmlNode
	var root *xmlNode
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
			node := &xmlNode{Name: typed.Name.Local, Attrs: cloneAttrs(typed.Attr)}
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
			parent.Children = append(parent.Children, &xmlNode{Text: text})
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("unclosed XML element %s", stack[len(stack)-1].Name)
	}
	if root == nil {
		return nil, fmt.Errorf("empty XML fragment")
	}
	return root, nil
}

func inheritanceSpecNodes(root *xmlNode) []*xmlNode {
	var specs []*xmlNode
	for _, child := range root.Children {
		if child.Name == "" {
			continue
		}
		if child.Name == "data" || child.Name == "__root__" {
			specs = append(specs, inheritanceSpecNodes(child)...)
			continue
		}
		specs = append(specs, child)
	}
	return specs
}

func applyInheritanceSpec(root *xmlNode, spec *xmlNode) error {
	position := attrValue(spec, "position")
	if position == "" {
		position = "inside"
	}
	var refs []xmlNodeRef
	if spec.Name == "xpath" {
		expr := attrValue(spec, "expr")
		if expr == "" {
			return fmt.Errorf("xpath spec is missing expr")
		}
		refs = findXPathRefs(root, expr)
	} else {
		refs = findDirectLocatorRefs(root, spec)
	}
	if len(refs) > 1 {
		refs = refs[:1]
	}
	if len(refs) == 0 {
		return fmt.Errorf("locator %q did not match", locatorName(spec))
	}
	switch position {
	case "inside":
		content, err := inheritanceContent(root, spec)
		if err != nil {
			return err
		}
		for _, ref := range refs {
			ref.Node.Children = append(ref.Node.Children, cloneNodes(content)...)
		}
	case "before":
		content, err := inheritanceContent(root, spec)
		if err != nil {
			return err
		}
		for i := len(refs) - 1; i >= 0; i-- {
			ref := refs[i]
			if ref.Parent == nil {
				return fmt.Errorf("cannot insert before XML root")
			}
			index := childIndex(ref.Parent, ref.Node)
			if index < 0 {
				return fmt.Errorf("target %q was removed before insertion", locatorName(spec))
			}
			ref.Parent.Children = insertNodes(ref.Parent.Children, index, cloneNodes(content))
		}
	case "after":
		content, err := inheritanceContent(root, spec)
		if err != nil {
			return err
		}
		for i := len(refs) - 1; i >= 0; i-- {
			ref := refs[i]
			if ref.Parent == nil {
				return fmt.Errorf("cannot insert after XML root")
			}
			index := childIndex(ref.Parent, ref.Node)
			if index < 0 {
				return fmt.Errorf("target %q was removed before insertion", locatorName(spec))
			}
			ref.Parent.Children = insertNodes(ref.Parent.Children, index+1, cloneNodes(content))
		}
	case "replace":
		mode := attrValue(spec, "mode")
		if mode == "" {
			mode = "outer"
		}
		switch mode {
		case "inner":
			content, err := inheritanceContent(root, spec)
			if err != nil {
				return err
			}
			for _, ref := range refs {
				ref.Node.Children = cloneNodes(content)
			}
		case "outer":
			content, err := inheritanceContent(root, spec)
			if err != nil {
				return err
			}
			for i := len(refs) - 1; i >= 0; i-- {
				ref := refs[i]
				if ref.Parent == nil {
					if len(content) != 1 {
						return fmt.Errorf("root replacement requires one element")
					}
					*root = *cloneNode(content[0])
					continue
				}
				index := childIndex(ref.Parent, ref.Node)
				if index < 0 {
					return fmt.Errorf("target %q was removed before replacement", locatorName(spec))
				}
				children := append([]*xmlNode{}, ref.Parent.Children[:index]...)
				children = append(children, cloneNodes(content)...)
				children = append(children, ref.Parent.Children[index+1:]...)
				ref.Parent.Children = children
			}
		default:
			return fmt.Errorf("unsupported replace mode %q", mode)
		}
	case "attributes":
		for _, ref := range refs {
			for _, attrNode := range elementChildren(spec) {
				if attrNode.Name != "attribute" {
					continue
				}
				if err := applyAttributeSpec(ref.Node, attrNode); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("unsupported inheritance position %q", position)
	}
	return nil
}

func findXPathRefs(root *xmlNode, expr string) []xmlNodeRef {
	expr = strings.TrimSpace(expr)
	if expr == "." {
		return []xmlNodeRef{{Node: root, Parent: nil, Index: 0}}
	}
	if parts := splitXPathUnion(expr); len(parts) > 1 {
		var refs []xmlNodeRef
		for _, part := range parts {
			refs = append(refs, findXPathRefs(root, part)...)
		}
		return dedupeRefsDocumentOrder(root, refs)
	}
	if refs, ok := findGroupedXPathRefs(root, expr); ok {
		return refs
	}
	switch {
	case strings.HasPrefix(expr, ".//"):
		return findDescendantPathRefs(root, strings.TrimPrefix(expr, ".//"))
	case strings.HasPrefix(expr, "//"):
		return findDescendantPathRefs(root, strings.TrimPrefix(expr, "//"))
	case strings.HasPrefix(expr, "/"):
		return findAbsolutePathRefs(root, strings.TrimPrefix(expr, "/"))
	default:
		return findDescendantPathRefs(root, expr)
	}
}

func findGroupedXPathRefs(root *xmlNode, expr string) ([]xmlNodeRef, bool) {
	if !strings.HasPrefix(expr, "(") {
		return nil, false
	}
	closeIndex := findMatchingCloseParen(expr, 0)
	if closeIndex < 0 {
		return nil, false
	}
	tail := strings.TrimSpace(expr[closeIndex+1:])
	if !strings.HasPrefix(tail, "[") {
		return nil, false
	}
	end := findMatchingCloseBracket(tail, 0)
	if end < 0 {
		return nil, false
	}
	predicate := strings.TrimSpace(tail[1:end])
	rest := strings.TrimSpace(tail[end+1:])
	refs := findXPathRefs(root, strings.TrimSpace(expr[1:closeIndex]))
	selected := selectGroupedXPathRefs(refs, predicate)
	if len(selected) == 0 || rest == "" {
		return selected, true
	}
	segments := splitXPathPath(rest)
	if strings.HasPrefix(rest, "//") && len(segments) > 0 {
		segments[0].Descendant = true
	}
	var expanded []xmlNodeRef
	for _, ref := range selected {
		expanded = append(expanded, matchPathRefs(root, ref, segments)...)
	}
	return dedupeRefsDocumentOrder(root, expanded), true
}

func selectGroupedXPathRefs(refs []xmlNodeRef, predicate string) []xmlNodeRef {
	if len(refs) == 0 {
		return nil
	}
	if predicate == "last()" {
		return []xmlNodeRef{refs[len(refs)-1]}
	}
	position, err := strconv.Atoi(predicate)
	if err != nil || position <= 0 || position > len(refs) {
		return nil
	}
	return []xmlNodeRef{refs[position-1]}
}

func splitXPathUnion(expr string) []string {
	var parts []string
	start := 0
	quote := byte(0)
	bracketDepth := 0
	parenDepth := 0
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '|':
			if bracketDepth == 0 && parenDepth == 0 {
				if part := strings.TrimSpace(expr[start:i]); part != "" {
					parts = append(parts, part)
				}
				start = i + 1
			}
		}
	}
	if part := strings.TrimSpace(expr[start:]); part != "" {
		parts = append(parts, part)
	}
	return parts
}

func findMatchingCloseParen(value string, openIndex int) int {
	return findMatchingClose(value, openIndex, '(', ')')
}

func findMatchingCloseBracket(value string, openIndex int) int {
	return findMatchingClose(value, openIndex, '[', ']')
}

func findMatchingClose(value string, openIndex int, open byte, close byte) int {
	if openIndex < 0 || openIndex >= len(value) || value[openIndex] != open {
		return -1
	}
	quote := byte(0)
	depth := 0
	for i := openIndex; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == open {
			depth++
			continue
		}
		if ch == close {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func dedupeRefsDocumentOrder(root *xmlNode, refs []xmlNodeRef) []xmlNodeRef {
	if len(refs) == 0 {
		return nil
	}
	byNode := make(map[*xmlNode]xmlNodeRef, len(refs))
	for _, ref := range refs {
		if _, exists := byNode[ref.Node]; !exists {
			byNode[ref.Node] = ref
		}
	}
	ordered := make([]xmlNodeRef, 0, len(byNode))
	walkRefs(root, nil, 0, func(ref xmlNodeRef) {
		matched, ok := byNode[ref.Node]
		if !ok {
			return
		}
		ordered = append(ordered, matched)
		delete(byNode, ref.Node)
	})
	return ordered
}

func findDescendantPathRefs(root *xmlNode, path string) []xmlNodeRef {
	segments := splitXPathPath(path)
	if len(segments) == 0 {
		return nil
	}
	first, ok := parseXPathStep(segments[0].Raw)
	if !ok {
		return nil
	}
	var refs []xmlNodeRef
	seen := map[*xmlNode]bool{}
	walkRefs(root, nil, 0, func(ref xmlNodeRef) {
		if matchesStepRef(ref, first) {
			for _, matched := range matchPathRefs(root, ref, segments[1:]) {
				if !seen[matched.Node] {
					seen[matched.Node] = true
					refs = append(refs, matched)
				}
			}
		}
	})
	return refs
}

func findAbsolutePathRefs(root *xmlNode, path string) []xmlNodeRef {
	segments := splitXPathPath(path)
	if len(segments) == 0 {
		return nil
	}
	first, ok := parseXPathStep(segments[0].Raw)
	if !ok || !matchesStepRef(xmlNodeRef{Node: root, Parent: nil, Index: 0}, first) {
		return nil
	}
	return matchPathRefs(root, xmlNodeRef{Node: root, Parent: nil, Index: 0}, segments[1:])
}

func matchPathRefs(root *xmlNode, ref xmlNodeRef, segments []xpathPathSegment) []xmlNodeRef {
	if len(segments) == 0 {
		return []xmlNodeRef{ref}
	}
	if strings.TrimSpace(segments[0].Raw) == ".." {
		if ref.Parent == nil {
			return nil
		}
		parentRef, ok := findNodeRef(root, ref.Parent)
		if !ok {
			return nil
		}
		return matchPathRefs(root, parentRef, segments[1:])
	}
	if axis, rawStep, ok := parseAxisStep(segments[0].Raw); ok {
		var refs []xmlNodeRef
		for _, axisRef := range matchAxisRefs(root, ref, axis, rawStep) {
			refs = append(refs, matchPathRefs(root, axisRef, segments[1:])...)
		}
		return dedupeRefsDocumentOrder(root, refs)
	}
	step, ok := parseXPathStep(segments[0].Raw)
	if !ok {
		return nil
	}
	var refs []xmlNodeRef
	if segments[0].Descendant {
		seen := map[*xmlNode]bool{}
		for idx, child := range ref.Node.Children {
			walkRefs(child, ref.Node, idx, func(descRef xmlNodeRef) {
				if matchesStepRef(descRef, step) {
					for _, matched := range matchPathRefs(root, descRef, segments[1:]) {
						if !seen[matched.Node] {
							seen[matched.Node] = true
							refs = append(refs, matched)
						}
					}
				}
			})
		}
		return refs
	}
	seen := map[*xmlNode]bool{}
	for idx, child := range ref.Node.Children {
		if child.Name == "" {
			continue
		}
		childRef := xmlNodeRef{Node: child, Parent: ref.Node, Index: idx}
		if matchesStepRef(childRef, step) {
			for _, matched := range matchPathRefs(root, childRef, segments[1:]) {
				if !seen[matched.Node] {
					seen[matched.Node] = true
					refs = append(refs, matched)
				}
			}
		}
	}
	return refs
}

func parseAxisStep(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if before, after, ok := strings.Cut(raw, "::"); ok {
		axis := strings.TrimSpace(before)
		switch axis {
		case "following-sibling", "ancestor", "parent":
			return axis, strings.TrimSpace(after), true
		default:
			return axis, strings.TrimSpace(after), true
		}
	}
	return "", "", false
}

func matchAxisRefs(root *xmlNode, ref xmlNodeRef, axis string, rawStep string) []xmlNodeRef {
	step, ok := parseXPathStep(rawStep)
	if !ok {
		return nil
	}
	var candidates []xmlNodeRef
	switch axis {
	case "following-sibling":
		if ref.Parent == nil {
			return nil
		}
		index := childIndex(ref.Parent, ref.Node)
		if index < 0 {
			return nil
		}
		for idx := index + 1; idx < len(ref.Parent.Children); idx++ {
			child := ref.Parent.Children[idx]
			if child.Name == "" || !matchesStep(child, step) {
				continue
			}
			candidates = append(candidates, xmlNodeRef{Node: child, Parent: ref.Parent, Index: idx})
		}
	case "ancestor":
		for current := ref.Parent; current != nil; {
			ancestorRef, ok := findNodeRef(root, current)
			if !ok {
				break
			}
			if matchesStep(ancestorRef.Node, step) {
				candidates = append(candidates, ancestorRef)
			}
			current = ancestorRef.Parent
		}
	case "parent":
		if ref.Parent == nil {
			return nil
		}
		parentRef, ok := findNodeRef(root, ref.Parent)
		if ok && matchesStep(parentRef.Node, step) {
			candidates = append(candidates, parentRef)
		}
	default:
		return nil
	}
	return selectAxisRefs(candidates, step)
}

func selectAxisRefs(refs []xmlNodeRef, step xpathStep) []xmlNodeRef {
	if len(refs) == 0 {
		return nil
	}
	if step.Position > 0 {
		if step.Position > len(refs) {
			return nil
		}
		return []xmlNodeRef{refs[step.Position-1]}
	}
	if step.Last {
		return []xmlNodeRef{refs[len(refs)-1]}
	}
	return refs
}

func findNodeRef(root *xmlNode, target *xmlNode) (xmlNodeRef, bool) {
	if target == nil {
		return xmlNodeRef{}, false
	}
	var found xmlNodeRef
	ok := false
	walkRefs(root, nil, 0, func(ref xmlNodeRef) {
		if ok || ref.Node != target {
			return
		}
		found = ref
		ok = true
	})
	return found, ok
}

func findDirectLocatorRefs(root *xmlNode, spec *xmlNode) []xmlNodeRef {
	name := attrValue(spec, "name")
	var refs []xmlNodeRef
	walkRefs(root, nil, 0, func(ref xmlNodeRef) {
		if ref.Node.Name != spec.Name {
			return
		}
		if name != "" {
			if attrValue(ref.Node, "name") == name {
				refs = append(refs, ref)
			}
			return
		}
		if directLocatorAttrsMatch(ref.Node, spec) {
			refs = append(refs, ref)
		}
	})
	return refs
}

func walkRefs(node *xmlNode, parent *xmlNode, index int, visit func(xmlNodeRef)) {
	if node == nil || node.Name == "" {
		return
	}
	visit(xmlNodeRef{Node: node, Parent: parent, Index: index})
	for idx, child := range node.Children {
		walkRefs(child, node, idx, visit)
	}
}

func parseXPathStep(raw string) (xpathStep, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return xpathStep{}, false
	}
	step := xpathStep{Tag: raw}
	if bracket := strings.Index(raw, "["); bracket >= 0 {
		step.Tag = strings.TrimSpace(raw[:bracket])
		for _, predicate := range splitXPathPredicates(raw[bracket:]) {
			for _, part := range splitXPathPredicateAnd(predicate) {
				applyXPathPredicate(&step, part)
			}
		}
	}
	if step.Tag == "" {
		step.Tag = "*"
	}
	return step, true
}

func splitXPathPredicates(raw string) []string {
	var predicates []string
	for i := 0; i < len(raw); i++ {
		if raw[i] != '[' {
			continue
		}
		start := i + 1
		quote := byte(0)
		depth := 1
		for i++; i < len(raw); i++ {
			ch := raw[i]
			if quote != 0 {
				if ch == quote {
					quote = 0
				}
				continue
			}
			if ch == '\'' || ch == '"' {
				quote = ch
				continue
			}
			if ch == '[' {
				depth++
				continue
			}
			if ch == ']' {
				depth--
				if depth != 0 {
					continue
				}
				predicate := strings.TrimSpace(raw[start:i])
				if predicate != "" {
					predicates = append(predicates, predicate)
				}
				break
			}
		}
	}
	return predicates
}

func splitXPathPredicateAnd(predicate string) []string {
	return splitXPathPredicateBool(predicate, "and")
}

func splitXPathPredicateOr(predicate string) []string {
	return splitXPathPredicateBool(predicate, "or")
}

func splitXPathPredicateBool(predicate string, operator string) []string {
	var parts []string
	start := 0
	quote := byte(0)
	depth := 0
	for i := 0; i < len(predicate); i++ {
		ch := predicate[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && isXPathBoolAt(predicate, operator, i) {
				if part := strings.TrimSpace(predicate[start:i]); part != "" {
					parts = append(parts, part)
				}
				i += len(operator) - 1
				start = i + 1
			}
		}
	}
	if part := strings.TrimSpace(predicate[start:]); part != "" {
		parts = append(parts, part)
	}
	return parts
}

func isXPathAndAt(value string, index int) bool {
	return isXPathBoolAt(value, "and", index)
}

func isXPathBoolAt(value string, operator string, index int) bool {
	if index+len(operator) > len(value) || value[index:index+len(operator)] != operator {
		return false
	}
	beforeOK := index == 0 || isXPathPredicateBoundary(value[index-1])
	after := index + len(operator)
	afterOK := after == len(value) || isXPathPredicateBoundary(value[after])
	return beforeOK && afterOK
}

func isXPathPredicateBoundary(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func applyXPathPredicate(step *xpathStep, predicate string) {
	predicate = strings.TrimSpace(predicate)
	if predicate == "" {
		return
	}
	if parts := splitXPathPredicateOr(predicate); len(parts) > 1 {
		if !applyOrPredicateParts(step, parts) {
			step.Unsupported = true
		}
		return
	}
	if position, err := strconv.Atoi(predicate); err == nil {
		step.Position = position
		return
	}
	if predicate == "last()" {
		step.Last = true
		return
	}
	if predicate == "node()" {
		step.HasNode = true
		return
	}
	if localName, ok := parseLocalNamePredicate(predicate); ok {
		step.LocalName = localName
		return
	}
	if attr, mode, ok := parseNotAttrPredicate(predicate); ok {
		switch mode {
		case "missing":
			step.AttrNotExists = append(step.AttrNotExists, attr.Name)
		case "not-equal":
			step.AttrNotEquals = append(step.AttrNotEquals, attr)
		}
		return
	}
	if strings.HasPrefix(predicate, "@") {
		if attr, ok := parseAttrEqualityPredicate(predicate); ok {
			step.Attrs = append(step.Attrs, attr)
			return
		}
		if name, ok := parseAttrPresencePredicate(predicate); ok {
			step.AttrExists = append(step.AttrExists, name)
		}
		return
	}
	if strings.HasPrefix(predicate, "contains(") {
		if !applyContainsPredicate(step, predicate) {
			step.Unsupported = true
		}
		return
	}
	for _, className := range parseHasClassPredicate(predicate) {
		step.ClassNames = append(step.ClassNames, className)
	}
	if classNames := parseNotHasClassPredicate(predicate); len(classNames) > 0 {
		step.NotClassNameSets = append(step.NotClassNameSets, classNames)
		return
	}
	if isRelativeXPathPredicate(predicate) {
		step.RelativePaths = append(step.RelativePaths, predicate)
	}
}

func applyOrPredicateParts(step *xpathStep, parts []string) bool {
	if len(parts) < 2 {
		return false
	}
	orPredicate := xpathAttrOrPredicate{Options: make([]xpathAttrPredicate, 0, len(parts))}
	for _, part := range parts {
		attr, ok := parseAttrEqualityPredicate(part)
		if !ok {
			return false
		}
		orPredicate.Options = append(orPredicate.Options, attr)
	}
	step.AttrOrs = append(step.AttrOrs, orPredicate)
	return true
}

func applyContainsPredicate(step *xpathStep, predicate string) bool {
	args, ok := parseXPathFunctionArgs(predicate, "contains")
	if !ok || len(args) != 2 {
		return false
	}
	left := strings.TrimSpace(args[0])
	value := cleanXPathStringArg(args[1])
	if strings.HasPrefix(left, "@") {
		step.AttrContains = append(step.AttrContains, xpathContainsPredicate{
			Name:  strings.TrimSpace(strings.TrimPrefix(left, "@")),
			Value: value,
		})
		return true
	}
	if left == "text()" {
		step.TextContains = append(step.TextContains, value)
		return true
	}
	if strings.HasPrefix(left, "concat(") && strings.Contains(left, "normalize-space(@class)") {
		if className := strings.TrimSpace(value); className != "" {
			step.ClassNames = append(step.ClassNames, className)
			return true
		}
	}
	return false
}

func parseLocalNamePredicate(predicate string) (string, bool) {
	predicate = strings.TrimSpace(predicate)
	if !strings.HasPrefix(predicate, "local-name()") {
		return "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(predicate, "local-name()"), "=", 2)
	if len(parts) != 2 {
		return "", false
	}
	value := cleanXPathStringArg(parts[1])
	return value, value != ""
}

func parseNotAttrPredicate(predicate string) (xpathAttrPredicate, string, bool) {
	predicate = strings.TrimSpace(predicate)
	if !strings.HasPrefix(predicate, "not(") || !strings.HasSuffix(predicate, ")") {
		return xpathAttrPredicate{}, "", false
	}
	inner := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(predicate, "not("), ")"))
	if attr, ok := parseAttrEqualityPredicate(inner); ok {
		return attr, "not-equal", true
	}
	if name, ok := parseAttrPresencePredicate(inner); ok {
		return xpathAttrPredicate{Name: name}, "missing", true
	}
	return xpathAttrPredicate{}, "", false
}

func parseAttrPresencePredicate(predicate string) (string, bool) {
	predicate = strings.TrimSpace(predicate)
	if !strings.HasPrefix(predicate, "@") || strings.Contains(predicate, "=") {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimPrefix(predicate, "@"))
	return name, name != ""
}

func parseAttrEqualityPredicate(predicate string) (xpathAttrPredicate, bool) {
	predicate = strings.TrimSpace(predicate)
	if !strings.HasPrefix(predicate, "@") {
		return xpathAttrPredicate{}, false
	}
	parts := strings.SplitN(strings.TrimPrefix(predicate, "@"), "=", 2)
	if len(parts) != 2 {
		return xpathAttrPredicate{}, false
	}
	name := strings.TrimSpace(parts[0])
	if name == "" {
		return xpathAttrPredicate{}, false
	}
	return xpathAttrPredicate{
		Name:  name,
		Value: cleanXPathStringArg(parts[1]),
	}, true
}

func parseXPathFunctionArgs(predicate string, name string) ([]string, bool) {
	predicate = strings.TrimSpace(predicate)
	prefix := name + "("
	if !strings.HasPrefix(predicate, prefix) || !strings.HasSuffix(predicate, ")") {
		return nil, false
	}
	args := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(predicate, prefix), ")"))
	return splitXPathArgs(args), true
}

func splitXPathArgs(args string) []string {
	var parts []string
	start := 0
	quote := byte(0)
	depth := 0
	for i := 0; i < len(args); i++ {
		ch := args[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				if part := strings.TrimSpace(args[start:i]); part != "" {
					parts = append(parts, part)
				}
				start = i + 1
			}
		}
	}
	if part := strings.TrimSpace(args[start:]); part != "" {
		parts = append(parts, part)
	}
	return parts
}

func parseHasClassPredicate(predicate string) []string {
	predicate = strings.TrimSpace(predicate)
	if !strings.HasPrefix(predicate, "hasclass(") || !strings.HasSuffix(predicate, ")") {
		return nil
	}
	args := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(predicate, "hasclass("), ")"))
	var classes []string
	quote := byte(0)
	start := 0
	for i := 0; i < len(args); i++ {
		ch := args[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			quote = ch
			continue
		}
		if ch == ',' {
			if className := cleanXPathStringArg(args[start:i]); className != "" {
				classes = append(classes, className)
			}
			start = i + 1
		}
	}
	if className := cleanXPathStringArg(args[start:]); className != "" {
		classes = append(classes, className)
	}
	return classes
}

func parseNotHasClassPredicate(predicate string) []string {
	predicate = strings.TrimSpace(predicate)
	if !strings.HasPrefix(predicate, "not(") || !strings.HasSuffix(predicate, ")") {
		return nil
	}
	return parseHasClassPredicate(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(predicate, "not("), ")")))
}

func cleanXPathStringArg(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '\'' || quote == '"') && value[len(value)-1] == quote {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func isRelativeXPathPredicate(predicate string) bool {
	predicate = strings.TrimSpace(predicate)
	if predicate == "" || strings.HasPrefix(predicate, "@") {
		return false
	}
	if strings.HasPrefix(predicate, "./") || strings.HasPrefix(predicate, ".//") {
		return true
	}
	first := firstXPathPredicateToken(predicate)
	if first == "*" {
		return true
	}
	return isXPathNameToken(first)
}

func firstXPathPredicateToken(predicate string) string {
	end := len(predicate)
	for idx, ch := range predicate {
		switch ch {
		case '[', '/', ' ', '\t', '\n', '\r', '=':
			end = idx
			goto done
		}
	}
done:
	return strings.TrimSpace(predicate[:end])
}

func isXPathNameToken(token string) bool {
	if token == "" || strings.ContainsAny(token, "()") {
		return false
	}
	for idx, ch := range token {
		if idx == 0 {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || ch == '_' {
				continue
			}
			return false
		}
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-' || ch == '.' || ch == ':' {
			continue
		}
		return false
	}
	return true
}

func splitXPathPath(path string) []xpathPathSegment {
	path = strings.TrimSpace(path)
	var segments []xpathPathSegment
	descendant := false
	for i := 0; i < len(path); {
		if path[i] == '/' {
			if i+1 < len(path) && path[i+1] == '/' {
				descendant = true
				i += 2
			} else {
				descendant = false
				i++
			}
			continue
		}
		start := i
		quote := byte(0)
		depth := 0
		for i < len(path) {
			ch := path[i]
			if quote != 0 {
				if ch == quote {
					quote = 0
				}
				i++
				continue
			}
			switch ch {
			case '\'', '"':
				quote = ch
			case '[':
				depth++
			case ']':
				if depth > 0 {
					depth--
				}
			case '/':
				if depth == 0 {
					goto segmentEnd
				}
			}
			i++
		}
	segmentEnd:
		if raw := strings.TrimSpace(path[start:i]); raw != "" {
			segments = append(segments, xpathPathSegment{Raw: raw, Descendant: descendant})
			descendant = false
		}
	}
	return segments
}

func matchesStep(node *xmlNode, step xpathStep) bool {
	if node == nil || node.Name == "" {
		return false
	}
	if step.Unsupported {
		return false
	}
	if step.Tag != "*" && node.Name != step.Tag {
		return false
	}
	if step.LocalName != "" && node.Name != step.LocalName {
		return false
	}
	if step.HasNode && !xmlNodeHasNode(node) {
		return false
	}
	for _, attr := range step.Attrs {
		if attr.Name != "" && attrValue(node, attr.Name) != attr.Value {
			return false
		}
	}
	for _, name := range step.AttrExists {
		if _, ok := attrLookup(node, name); !ok {
			return false
		}
	}
	for _, name := range step.AttrNotExists {
		if _, ok := attrLookup(node, name); ok {
			return false
		}
	}
	for _, attr := range step.AttrNotEquals {
		if value, ok := attrLookup(node, attr.Name); ok && value == attr.Value {
			return false
		}
	}
	for _, attrOr := range step.AttrOrs {
		matched := false
		for _, option := range attrOr.Options {
			if option.Name != "" && attrValue(node, option.Name) == option.Value {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, predicate := range step.AttrContains {
		if predicate.Name != "" && !strings.Contains(attrValue(node, predicate.Name), predicate.Value) {
			return false
		}
	}
	for _, value := range step.TextContains {
		if !strings.Contains(nodeDirectText(node), value) {
			return false
		}
	}
	for _, className := range step.ClassNames {
		if !hasClassToken(attrValue(node, "class"), className) {
			return false
		}
	}
	for _, classNames := range step.NotClassNameSets {
		if hasClassTokens(attrValue(node, "class"), classNames) {
			return false
		}
	}
	for _, relativePath := range step.RelativePaths {
		if !matchesRelativeXPathPredicate(node, relativePath) {
			return false
		}
	}
	return true
}

func xmlNodeHasNode(node *xmlNode) bool {
	for _, child := range node.Children {
		if child.Name != "" || child.Text != "" {
			return true
		}
	}
	return false
}

func matchesRelativeXPathPredicate(node *xmlNode, predicate string) bool {
	path, attr, value, ok := parseRelativeAttrEqualityPredicate(predicate)
	if ok {
		for _, ref := range findRelativeXPathRefs(node, path) {
			if attrValue(ref.Node, attr) == value {
				return true
			}
		}
		return false
	}
	return len(findRelativeXPathRefs(node, predicate)) > 0
}

func parseRelativeAttrEqualityPredicate(predicate string) (string, string, string, bool) {
	eq := findTopLevelXPathOperator(predicate, "=")
	if eq < 0 {
		return "", "", "", false
	}
	left := strings.TrimSpace(predicate[:eq])
	right := cleanXPathStringArg(predicate[eq+1:])
	path, attr, ok := strings.Cut(left, "/@")
	if !ok || strings.TrimSpace(attr) == "" || strings.TrimSpace(path) == "" {
		return "", "", "", false
	}
	return strings.TrimSpace(path), strings.TrimSpace(attr), right, true
}

func findTopLevelXPathOperator(value string, operator string) int {
	quote := byte(0)
	bracketDepth := 0
	parenDepth := 0
	for i := 0; i <= len(value)-len(operator); i++ {
		ch := value[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		default:
			if bracketDepth == 0 && parenDepth == 0 && strings.HasPrefix(value[i:], operator) {
				return i
			}
		}
	}
	return -1
}

func findRelativeXPathRefs(node *xmlNode, predicate string) []xmlNodeRef {
	predicate = strings.TrimSpace(predicate)
	if strings.HasPrefix(predicate, "./") {
		predicate = strings.TrimPrefix(predicate, "./")
	}
	if predicate == "" {
		return nil
	}
	if predicate == "." {
		return []xmlNodeRef{{Node: node, Parent: nil, Index: 0}}
	}
	segments := splitXPathPath(predicate)
	if strings.HasPrefix(predicate, "//") && len(segments) > 0 {
		segments[0].Descendant = true
	}
	return matchPathRefs(node, xmlNodeRef{Node: node, Parent: nil, Index: 0}, segments)
}

func hasClassTokens(value string, classNames []string) bool {
	for _, className := range classNames {
		if !hasClassToken(value, className) {
			return false
		}
	}
	return true
}

func hasClassToken(value string, className string) bool {
	for _, token := range strings.Fields(value) {
		if token == className {
			return true
		}
	}
	return false
}

func matchesStepRef(ref xmlNodeRef, step xpathStep) bool {
	if !matchesStep(ref.Node, step) {
		return false
	}
	if step.Position <= 0 && !step.Last {
		return true
	}
	position := siblingMatchingElementPosition(ref, step)
	if step.Position > 0 && position != step.Position {
		return false
	}
	if step.Last && position != siblingMatchingElementCount(ref, step) {
		return false
	}
	return position > 0
}

func siblingMatchingElementPosition(ref xmlNodeRef, step xpathStep) int {
	if ref.Parent == nil {
		return 1
	}
	position := 0
	for _, child := range ref.Parent.Children {
		if child.Name == "" || !matchesStep(child, step) {
			continue
		}
		position++
		if child == ref.Node {
			return position
		}
	}
	return 0
}

func siblingMatchingElementCount(ref xmlNodeRef, step xpathStep) int {
	if ref.Parent == nil {
		return 1
	}
	count := 0
	for _, child := range ref.Parent.Children {
		if child.Name != "" && matchesStep(child, step) {
			count++
		}
	}
	return count
}

func applyAttributeSpec(node *xmlNode, spec *xmlNode) error {
	for _, attr := range spec.Attrs {
		key := attr.Name.Local
		if key != "name" && key != "add" && key != "remove" && key != "separator" && !strings.HasPrefix(key, "data-oe-") {
			return fmt.Errorf("invalid attribute spec key %q", key)
		}
	}
	name := attrValue(spec, "name")
	if name == "" {
		return fmt.Errorf("attribute spec is missing name")
	}
	add, hasAdd := attrLookup(spec, "add")
	remove, hasRemove := attrLookup(spec, "remove")
	if (hasAdd && add != "") || (hasRemove && remove != "") {
		if text := nodeDirectText(spec); text != "" {
			return fmt.Errorf("attribute %q with add/remove cannot contain text", name)
		}
		value, _ := attrLookup(node, name)
		if pythonAttribute(name) {
			separator := strings.TrimSpace(attrValue(spec, "separator"))
			if separator != "and" && separator != "or" {
				return fmt.Errorf("invalid separator %q for python attribute %q", separator, name)
			}
			value = removePythonExpression(value, remove, separator)
			if add != "" {
				if value != "" {
					value = "(" + value + ") " + separator + " (" + add + ")"
				} else {
					value = add
				}
			}
		} else {
			separator, hasSeparator := attrLookup(spec, "separator")
			if !hasSeparator {
				separator = ","
			}
			value = mutateSeparatedAttribute(value, add, remove, separator)
		}
		setOrRemoveAttr(node, name, value)
		return nil
	}
	setOrRemoveAttr(node, name, nodeDirectText(spec))
	return nil
}

func pythonAttribute(name string) bool {
	switch name {
	case "readonly", "required", "invisible", "column_invisible", "t-if", "t-elif":
		return true
	default:
		return strings.HasPrefix(name, "decoration-")
	}
}

func removePythonExpression(value string, remove string, separator string) string {
	if value == "" || remove == "" {
		return value
	}
	exact := regexp.MustCompile(`^\(*` + regexp.QuoteMeta(remove) + `\)*$`)
	if exact.MatchString(value) {
		return ""
	}
	for _, pattern := range []string{
		"(" + remove + ") " + separator + " ",
		" " + separator + " (" + remove + ")",
		remove + " " + separator + " ",
		" " + separator + " " + remove,
	} {
		if index := strings.Index(value, pattern); index >= 0 {
			return value[:index] + value[index+len(pattern):]
		}
	}
	return value
}

func mutateSeparatedAttribute(value string, add string, remove string, separator string) string {
	if separator == "" {
		separator = ","
	}
	joiner := separator
	if joiner == " " {
		joiner = " "
	}
	removeSet := map[string]bool{}
	for _, item := range splitAttributeValues(remove, separator) {
		if item != "" {
			removeSet[item] = true
		}
	}
	var values []string
	for _, item := range splitAttributeValues(value, separator) {
		if item != "" && !removeSet[item] {
			values = append(values, item)
		}
	}
	for _, item := range splitAttributeValues(add, separator) {
		if item != "" {
			values = append(values, item)
		}
	}
	return strings.Join(values, joiner)
}

func splitAttributeValues(value string, separator string) []string {
	if value == "" {
		return nil
	}
	if separator == "" {
		separator = ","
	}
	var raw []string
	if separator == " " {
		raw = strings.Fields(value)
	} else {
		raw = strings.Split(value, separator)
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		out = append(out, strings.TrimSpace(item))
	}
	return out
}

func directLocatorAttrsMatch(node *xmlNode, spec *xmlNode) bool {
	for _, attr := range spec.Attrs {
		if attr.Name.Local == "position" || attr.Name.Local == "version" {
			continue
		}
		if attrValue(node, attr.Name.Local) != attr.Value {
			return false
		}
	}
	return true
}

func inheritanceContent(root *xmlNode, spec *xmlNode) ([]*xmlNode, error) {
	var out []*xmlNode
	for _, child := range spec.Children {
		if child.Name == "" {
			out = append(out, cloneNode(child))
			continue
		}
		if attrValue(child, "position") == "move" {
			moved, err := extractMovedNode(root, child)
			if err != nil {
				return nil, err
			}
			out = append(out, moved)
			continue
		}
		out = append(out, cloneNode(child))
	}
	return out, nil
}

func extractMovedNode(root *xmlNode, spec *xmlNode) (*xmlNode, error) {
	if len(elementChildren(spec)) > 0 {
		return nil, fmt.Errorf("move locator %q cannot contain child nodes", locatorName(spec))
	}
	var refs []xmlNodeRef
	if spec.Name == "xpath" {
		expr := attrValue(spec, "expr")
		if expr == "" {
			return nil, fmt.Errorf("xpath spec is missing expr")
		}
		refs = findXPathRefs(root, expr)
	} else {
		refs = findDirectLocatorRefs(root, spec)
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("move locator %q did not match", locatorName(spec))
	}
	ref := refs[0]
	if ref.Parent == nil {
		return nil, fmt.Errorf("cannot move XML root")
	}
	index := childIndex(ref.Parent, ref.Node)
	if index < 0 {
		return nil, fmt.Errorf("move locator %q was already removed", locatorName(spec))
	}
	moved := ref.Parent.Children[index]
	ref.Parent.Children = append(append([]*xmlNode{}, ref.Parent.Children[:index]...), ref.Parent.Children[index+1:]...)
	return moved, nil
}

func elementChildren(node *xmlNode) []*xmlNode {
	var out []*xmlNode
	for _, child := range node.Children {
		if child.Name != "" {
			out = append(out, child)
		}
	}
	return out
}

func cloneNodes(nodes []*xmlNode) []*xmlNode {
	out := make([]*xmlNode, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, cloneNode(node))
	}
	return out
}

func cloneNode(node *xmlNode) *xmlNode {
	if node == nil {
		return nil
	}
	cloned := &xmlNode{Name: node.Name, Text: node.Text, Attrs: cloneAttrs(node.Attrs)}
	cloned.Children = cloneNodes(node.Children)
	return cloned
}

func cloneAttrs(attrs []xml.Attr) []xml.Attr {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]xml.Attr, len(attrs))
	copy(out, attrs)
	return out
}

func insertNodes(nodes []*xmlNode, index int, inserted []*xmlNode) []*xmlNode {
	out := append([]*xmlNode{}, nodes[:index]...)
	out = append(out, inserted...)
	out = append(out, nodes[index:]...)
	return out
}

func childIndex(parent *xmlNode, child *xmlNode) int {
	for index, item := range parent.Children {
		if item == child {
			return index
		}
	}
	return -1
}

func attrLookup(node *xmlNode, name string) (string, bool) {
	for _, attr := range node.Attrs {
		if attr.Name.Local == name {
			return attr.Value, true
		}
	}
	return "", false
}

func attrValue(node *xmlNode, name string) string {
	value, _ := attrLookup(node, name)
	return value
}

func setOrRemoveAttr(node *xmlNode, name string, value string) {
	if value == "" {
		removeAttr(node, name)
		return
	}
	setAttrValue(node, name, value)
}

func removeAttr(node *xmlNode, name string) {
	for idx, attr := range node.Attrs {
		if attr.Name.Local == name {
			node.Attrs = append(node.Attrs[:idx], node.Attrs[idx+1:]...)
			return
		}
	}
}

func setAttrValue(node *xmlNode, name string, value string) {
	for idx := range node.Attrs {
		if node.Attrs[idx].Name.Local == name {
			node.Attrs[idx].Value = value
			return
		}
	}
	node.Attrs = append(node.Attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}

func nodeText(node *xmlNode) string {
	var builder strings.Builder
	var visit func(*xmlNode)
	visit = func(item *xmlNode) {
		if item.Name == "" {
			builder.WriteString(item.Text)
			return
		}
		for _, child := range item.Children {
			visit(child)
		}
	}
	visit(node)
	return strings.TrimSpace(builder.String())
}

func nodeDirectText(node *xmlNode) string {
	var builder strings.Builder
	for _, child := range node.Children {
		if child.Name == "" {
			builder.WriteString(child.Text)
		}
	}
	return builder.String()
}

func locatorName(spec *xmlNode) string {
	if spec.Name == "xpath" {
		return attrValue(spec, "expr")
	}
	if name := attrValue(spec, "name"); name != "" {
		return spec.Name + "[@name='" + name + "']"
	}
	return spec.Name
}

func renderXML(node *xmlNode) string {
	var buffer bytes.Buffer
	renderNode(&buffer, node)
	return buffer.String()
}

func renderNode(buffer *bytes.Buffer, node *xmlNode) {
	if node.Name == "" {
		_ = xml.EscapeText(buffer, []byte(node.Text))
		return
	}
	buffer.WriteByte('<')
	buffer.WriteString(node.Name)
	for _, attr := range node.Attrs {
		buffer.WriteByte(' ')
		buffer.WriteString(attr.Name.Local)
		buffer.WriteString(`="`)
		_ = xml.EscapeText(buffer, []byte(attr.Value))
		buffer.WriteByte('"')
	}
	if len(node.Children) == 0 {
		buffer.WriteString("/>")
		return
	}
	buffer.WriteByte('>')
	for _, child := range node.Children {
		renderNode(buffer, child)
	}
	buffer.WriteString("</")
	buffer.WriteString(node.Name)
	buffer.WriteByte('>')
}
