package workflow

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"gorp/internal/domain"
	"gorp/internal/field"
	"gorp/internal/model"
	"gorp/internal/registry"
	"gorp/internal/security"
)

const (
	ModelWorkflow       = "workflow"
	ModelNode           = "workflow.node"
	ModelTransition     = "workflow.transition"
	ModelNodeAction     = "workflow.node.action"
	ModelProcess        = "workflow.process"
	ModelWorkflowWizard = "workflow.process.wizard"
	ModelApprovalLog    = "approval.log"
)

type NodeType string

const (
	NodeTypeUser    NodeType = "user"
	NodeTypeAuto    NodeType = "auto"
	NodeTypeTrigger NodeType = "trigger"
	NodeTypeEnd     NodeType = "end"
)

type ButtonType string

const (
	ButtonTypeOne   ButtonType = "one"
	ButtonTypeMulti ButtonType = "multi"
)

type CommentMode string

const (
	CommentNone     CommentMode = ""
	CommentOptional CommentMode = "optional"
	CommentRequired CommentMode = "required"
)

type DelayType string

const (
	DelayMinutes DelayType = "minutes"
	DelayHours   DelayType = "hours"
	DelayDays    DelayType = "days"
	DelayMonths  DelayType = "months"
)

type Workflow struct {
	ID                 int64
	Name               string
	Code               string
	ApprovalSettingsID int64
	Model              string
	Sequence           int
	Active             bool
	State              string
	Condition          Condition
	ViewID             int64
	CreateContext      map[string]any
	OnCreate           bool
	CompanyIDs         []int64
	StartNodeID        int64
	Nodes              []Node
}

type Node struct {
	ID                        int64
	WorkflowID                int64
	Name                      string
	Code                      string
	Type                      NodeType
	Sequence                  int
	Active                    bool
	State                     string
	ResponsibleGroupIDs       []int64
	ResponsibleUserIDs        []int64
	ResponsiblePythonCode     string
	ResponsibleCondition      Condition
	ResponsibleCommittee      bool
	ResponsibleCommitteeLimit int
	ScheduleActivity          bool
	ScheduleActivityField     string
	ScheduleActivityDays      int
	Actions                   []NodeAction
	Transitions               []Transition
	ButtonType                ButtonType
	ButtonName                string
	ButtonContext             string
	ButtonIcon                string
	ButtonValidateForm        bool
	WizardViewID              int64
	AllowForward              bool
	Escalation                bool
	EscalationDelayType       DelayType
	EscalationDelay           int
	EscalationNodeID          int64
	EscalationCalendarID      int64
}

type Transition struct {
	ID                int64
	NodeID            int64
	Name              string
	Code              string
	Sequence          int
	Active            bool
	RunAsSuperuser    bool
	Condition         Condition
	NextNodeID        int64
	GroupIDs          []int64
	Comment           CommentMode
	ButtonClass       string
	WizardViewID      int64
	Context           map[string]any
	Icon              string
	Committee         bool
	CommitteeLimit    int
	ValidateForm      bool
	Trigger           string
	IsEmail           bool
	EmailTemplateID   int64
	EmailWizardFormID int64
	IsHidden          bool
}

type NodeAction struct {
	ID             int64
	NodeID         int64
	Sequence       int
	Active         bool
	Condition      Condition
	ServerActionID int64
	ActionKey      string
}

type Process struct {
	WorkflowID          int64
	Model               string
	RecordID            int64
	NodeID              int64
	State               string
	Active              bool
	ApprovalUserIDs     []int64
	ApprovalDoneUserIDs []int64
	ApprovalPartnerIDs  []int64
	UserCanApprove      bool
	EscalationDate      time.Time
	LastTransitionID    int64
	StartedAt           time.Time
	UpdatedAt           time.Time
}

type Condition struct {
	Expression string
	Predicate  Predicate
}

type Predicate func(EvaluationContext) (bool, error)

type EvaluationContext struct {
	UserID       int64
	UserGroupIDs []int64
	CompanyID    int64
	CompanyIDs   []int64
	DelegationID int64
	Model        string
	RecordID     int64
	Values       map[string]any
	Predicates   map[string]Predicate
	MailComposed bool
	Now          time.Time
}

type Hooks struct {
	Action      ActionRunner
	ApprovalLog ApprovalLogHook
	MailCompose MailComposeHook
}

type ActionRunner func(NodeAction, Process, EvaluationContext) (ActionResult, error)
type ApprovalLogHook func(ApprovalLogEvent) error
type MailComposeHook func(Transition, Process, EvaluationContext) (ActionResult, error)

type ActionResult struct {
	ActionID int64
	Key      string
	Type     string
	Payload  map[string]any
}

type ApprovalLogEvent struct {
	At           time.Time
	UserID       int64
	WorkflowID   int64
	Model        string
	RecordID     int64
	OldNodeID    int64
	NewNodeID    int64
	TransitionID int64
	DelegationID int64
	Committee    bool
	Details      map[string]string
}

type GraphMetadata struct {
	WorkflowID   int64
	WorkflowName string
	StartNodeID  int64
	Nodes        []GraphNode
	Edges        []GraphEdge
	Actions      []GraphAction
}

type GraphNode struct {
	ID       int64
	Code     string
	Label    string
	Type     NodeType
	State    string
	Sequence int
}

type GraphEdge struct {
	ID         int64
	Code       string
	Label      string
	FromNodeID int64
	ToNodeID   int64
	Condition  string
	Sequence   int
	Hidden     bool
}

type GraphAction struct {
	ID        int64
	NodeID    int64
	Key       string
	Condition string
	Sequence  int
}

func Expr(expression string) Condition {
	return Condition{Expression: expression}
}

func PredicateCondition(predicate Predicate) Condition {
	return Condition{Predicate: predicate}
}

func AdvancedModels() []model.Model {
	return []model.Model{
		workflowModel(),
		nodeModel(),
		transitionModel(),
		nodeActionModel(),
		processModel(),
		processWizardModel(),
	}
}

func AdvancedExtensionModels() []model.Model {
	return []model.Model{
		advancedExtension("approval.config", "approval_config",
			field.New("workflow_advanced", field.Bool),
		),
		advancedExtension(ModelForward, "approval_forward",
			field.New("workflow_node_id", field.Many2One).WithRelation(ModelNode),
		),
		advancedExtension(ModelApprovalRecord, "",
			field.New("workflow_process_ids", field.One2Many).WithRelation(ModelProcess),
			field.New("workflow_id", field.Many2One).WithRelation(ModelWorkflow),
			field.New("workflow_node_id", field.Many2One).WithRelation(ModelNode),
			field.New("workflow_node_code", field.Char),
			field.New("workflow_transition_ids", field.Many2Many).WithRelation(ModelTransition),
			field.New("workflow_view_id", field.Many2One).WithRelation("ir.ui.view"),
			field.New("_old_workflow_node_id", field.Many2One).WithRelation(ModelNode),
			field.New("_workflow_transition_id", field.Many2One).WithRelation(ModelTransition),
		),
		advancedExtension("approval.settings", "approval_settings",
			field.New("workflow_ids", field.One2Many).WithRelation(ModelWorkflow),
			field.New("workflow_count", field.Int),
		),
		advancedExtension("ir.actions.act_window", "ir_act_window",
			field.New("multi_workflow_view", field.Char),
		),
		advancedExtension("approval.state.update", "approval_state_update",
			field.New("workflow_model", field.Bool),
			field.New("workflow_node_id", field.Many2One).WithRelation(ModelNode),
			field.New("workflow_id", field.Many2One).WithRelation(ModelWorkflow),
		),
	}
}

func RegisterAdvancedModels(reg *registry.Registry) error {
	for _, m := range AdvancedModels() {
		if err := reg.RegisterModel(m); err != nil {
			return err
		}
	}
	return nil
}

func AdvancedModelNames() []string {
	models := AdvancedModels()
	names := make([]string, 0, len(models))
	for _, m := range models {
		names = append(names, m.Name)
	}
	return names
}

func ApprovalLogFields() []field.Field {
	return []field.Field{
		field.New("old_node_id", field.Many2One).WithRelation(ModelNode),
		field.New("new_node_id", field.Many2One).WithRelation(ModelNode),
		field.New("workflow_transition_id", field.Many2One).WithRelation(ModelTransition),
	}
}

func WorkflowCompanyDomain() domain.Node {
	return domain.Or(
		domain.Cond("company_id", domain.Equal, nil),
		domain.Cond("company_id", domain.In, "user.company_ids"),
	)
}

func AdvancedSecurityRuleDefinitions() []RuleDefinition {
	return []RuleDefinition{
		{
			Name: "workflow_advance_company_" + ModelWorkflow,
			Rule: security.Rule{
				Model:      ModelWorkflow,
				Domain:     WorkflowCompanyDomain(),
				Global:     true,
				PermRead:   true,
				PermWrite:  true,
				PermCreate: true,
				PermUnlink: true,
				Active:     true,
			},
		},
	}
}

type RuleDefinition struct {
	Name string
	Rule security.Rule
}

func (c Condition) Evaluate(ctx EvaluationContext) (bool, error) {
	if c.Predicate != nil {
		return c.Predicate(ctx)
	}
	expression := strings.TrimSpace(c.Expression)
	if expression == "" {
		return true, nil
	}
	if strings.HasPrefix(expression, "predicate:") {
		name := strings.TrimSpace(strings.TrimPrefix(expression, "predicate:"))
		predicate, ok := ctx.Predicates[name]
		if !ok {
			return false, fmt.Errorf("unknown predicate %q", name)
		}
		return predicate(ctx)
	}
	parser, err := newExpressionParser(expression, ctx)
	if err != nil {
		return false, err
	}
	return parser.parse()
}

func (w Workflow) Matches(ctx EvaluationContext, onCreate bool) (bool, error) {
	if !w.Active {
		return false, nil
	}
	if w.Model != "" && ctx.Model != "" && w.Model != ctx.Model {
		return false, nil
	}
	if onCreate && !w.OnCreate {
		return false, nil
	}
	if w.State != "" {
		state, _ := resolveIdentifier("state", ctx)
		if state != w.State {
			return false, nil
		}
	}
	if !w.companyAllowed(ctx.CompanyID) {
		return false, nil
	}
	return w.Condition.Evaluate(ctx)
}

func (w Workflow) Start(ctx EvaluationContext, hooks Hooks) (Process, []ActionResult, error) {
	ok, err := w.Matches(ctx, false)
	if err != nil || !ok {
		if err != nil {
			return Process{}, nil, err
		}
		return Process{}, nil, fmt.Errorf("workflow %d does not match record", w.ID)
	}
	node, ok := w.startNode()
	if !ok {
		return Process{}, nil, fmt.Errorf("workflow %d has no start node", w.ID)
	}
	now := ctx.now()
	process := Process{
		WorkflowID: w.ID,
		Model:      firstNonEmpty(ctx.Model, w.Model),
		RecordID:   ctx.RecordID,
		NodeID:     node.ID,
		Active:     true,
		StartedAt:  now,
		UpdatedAt:  now,
	}
	return w.enterNode(process, ctx, hooks, 0)
}

func (w Workflow) AvailableTransitions(process Process, ctx EvaluationContext) ([]Transition, error) {
	node, ok := w.nodeByID(process.NodeID)
	if !ok {
		return nil, fmt.Errorf("unknown workflow node %d", process.NodeID)
	}
	canApprove, err := node.CanApprove(process, ctx)
	if err != nil {
		return nil, err
	}
	if !canApprove {
		return nil, nil
	}
	transitions := orderedTransitions(node.Transitions)
	out := make([]Transition, 0, len(transitions))
	for _, transition := range transitions {
		ok, err := transition.Available(ctx)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, transition)
		}
	}
	return out, nil
}

func (t Transition) Available(ctx EvaluationContext) (bool, error) {
	if !t.Active || t.IsHidden {
		return false, nil
	}
	if len(t.GroupIDs) > 0 && !intersectsInt64(t.GroupIDs, ctx.UserGroupIDs) {
		return false, nil
	}
	return t.Condition.Evaluate(ctx)
}

func (n Node) CanApprove(process Process, ctx EvaluationContext) (bool, error) {
	if n.Type != NodeTypeUser {
		return true, nil
	}
	if ctx.UserID != 0 && containsInt64(process.ApprovalDoneUserIDs, ctx.UserID) {
		return false, nil
	}
	if ctx.UserID == 1 {
		return true, nil
	}
	if len(process.ApprovalUserIDs) > 0 {
		return ctx.UserID != 0 && containsInt64(process.ApprovalUserIDs, ctx.UserID), nil
	}
	hasResponsible := len(process.ApprovalUserIDs) > 0 || len(n.ResponsibleUserIDs) > 0 || len(n.ResponsibleGroupIDs) > 0 || n.ResponsibleCondition.Expression != "" || n.ResponsibleCondition.Predicate != nil
	if !hasResponsible {
		return true, nil
	}
	if ctx.UserID != 0 && (containsInt64(process.ApprovalUserIDs, ctx.UserID) || containsInt64(n.ResponsibleUserIDs, ctx.UserID)) {
		return true, nil
	}
	if len(n.ResponsibleGroupIDs) > 0 && intersectsInt64(n.ResponsibleGroupIDs, ctx.UserGroupIDs) {
		return true, nil
	}
	if n.ResponsibleCondition.Expression == "" && n.ResponsibleCondition.Predicate == nil {
		return false, nil
	}
	return n.ResponsibleCondition.Evaluate(ctx)
}

func (w Workflow) ApplyTransition(process Process, transitionID int64, ctx EvaluationContext, hooks Hooks) (Process, []ActionResult, error) {
	if !process.Active {
		return process, nil, fmt.Errorf("workflow process is inactive")
	}
	transition, err := w.findAvailableTransition(process, transitionID, ctx)
	if err != nil {
		return process, nil, err
	}
	if transition.IsEmail && !ctx.MailComposed && hooks.MailCompose != nil {
		process.LastTransitionID = transition.ID
		process.UpdatedAt = ctx.now()
		result, err := hooks.MailCompose(transition, process, ctx)
		if err != nil {
			return process, nil, err
		}
		return process, []ActionResult{result}, nil
	}
	if transition.Committee && !containsInt64(process.ApprovalDoneUserIDs, ctx.UserID) {
		process.ApprovalDoneUserIDs = append(process.ApprovalDoneUserIDs, ctx.UserID)
	}
	if transition.Committee {
		process.ApprovalUserIDs = withoutIDs(process.ApprovalUserIDs, process.ApprovalDoneUserIDs)
	}
	if transition.Committee && transition.CommitteeLimit > 0 && len(process.ApprovalDoneUserIDs) < transition.CommitteeLimit {
		if err := emitApprovalLog(process, ctx, hooks, ApprovalLogEvent{
			WorkflowID:   w.ID,
			Model:        process.Model,
			RecordID:     process.RecordID,
			OldNodeID:    process.NodeID,
			NewNodeID:    process.NodeID,
			TransitionID: transition.ID,
			Committee:    true,
		}); err != nil {
			return process, nil, err
		}
		process.UpdatedAt = ctx.now()
		return process, nil, nil
	}
	oldNodeID := process.NodeID
	process.NodeID = transition.NextNodeID
	process.LastTransitionID = transition.ID
	process.UpdatedAt = ctx.now()
	if err := emitApprovalLog(process, ctx, hooks, ApprovalLogEvent{
		WorkflowID:   w.ID,
		Model:        process.Model,
		RecordID:     process.RecordID,
		OldNodeID:    oldNodeID,
		NewNodeID:    process.NodeID,
		TransitionID: transition.ID,
		Committee:    transition.Committee,
	}); err != nil {
		return process, nil, err
	}
	return w.enterNode(process, ctx, hooks, 0)
}

func (w Workflow) Graph() GraphMetadata {
	nodes := orderedNodes(w.Nodes)
	graph := GraphMetadata{
		WorkflowID:   w.ID,
		WorkflowName: w.Name,
		StartNodeID:  w.startNodeID(),
		Nodes:        make([]GraphNode, 0, len(nodes)),
	}
	for _, node := range nodes {
		if !node.Active {
			continue
		}
		graph.Nodes = append(graph.Nodes, GraphNode{
			ID:       node.ID,
			Code:     node.Code,
			Label:    node.Name,
			Type:     node.Type,
			State:    node.State,
			Sequence: node.Sequence,
		})
		for _, action := range orderedActions(node.Actions) {
			if !action.Active {
				continue
			}
			graph.Actions = append(graph.Actions, GraphAction{
				ID:        action.ID,
				NodeID:    node.ID,
				Key:       action.ActionKey,
				Condition: action.Condition.Expression,
				Sequence:  action.Sequence,
			})
		}
		for _, transition := range orderedTransitions(node.Transitions) {
			if !transition.Active {
				continue
			}
			graph.Edges = append(graph.Edges, GraphEdge{
				ID:         transition.ID,
				Code:       transition.Code,
				Label:      transition.Name,
				FromNodeID: node.ID,
				ToNodeID:   transition.NextNodeID,
				Condition:  transition.Condition.Expression,
				Sequence:   transition.Sequence,
				Hidden:     transition.IsHidden,
			})
		}
	}
	return graph
}

func (w Workflow) Flowchart() string {
	return w.Graph().FlowchartDSL()
}

func (g GraphMetadata) Mermaid() string {
	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, node := range g.Nodes {
		fmt.Fprintf(&b, "  n%d[%q]\n", node.ID, node.Label)
	}
	for _, edge := range g.Edges {
		if edge.Hidden {
			continue
		}
		if edge.Label != "" {
			fmt.Fprintf(&b, "  n%d -->|%s| n%d\n", edge.FromNodeID, mermaidLabel(edge.Label), edge.ToNodeID)
		} else {
			fmt.Fprintf(&b, "  n%d --> n%d\n", edge.FromNodeID, edge.ToNodeID)
		}
	}
	for _, action := range g.Actions {
		if action.Key == "" {
			continue
		}
		fmt.Fprintf(&b, "  n%d -. %s .-> a%d[%q]\n", action.NodeID, mermaidLabel(action.Key), action.ID, action.Key)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g GraphMetadata) FlowchartDSL() string {
	if len(g.Nodes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("st=>start: Start\n")
	b.WriteString("e=>end: End\n")
	for _, node := range g.Nodes {
		nodeType := "inputoutput"
		if node.Type == NodeTypeUser {
			nodeType = "subroutine"
		}
		fmt.Fprintf(&b, "node%d=>%s: %s|approved\n", node.ID, nodeType, flowchartLabel(node.Label))
		for _, edge := range g.edgesFrom(node.ID) {
			if edge.Hidden {
				continue
			}
			fmt.Fprintf(&b, "trans%d=>condition: %s\n", edge.ID, flowchartLabel(edge.Label))
		}
		for _, action := range g.actionsForNode(node.ID) {
			label := action.Key
			if label == "" {
				label = fmt.Sprintf("Action %d", action.ID)
			}
			fmt.Fprintf(&b, "action%d=>operation: %s|past\n", action.ID, flowchartLabel(label))
		}
	}
	if g.StartNodeID != 0 {
		fmt.Fprintf(&b, "st->%s\n", g.nodeDirection(g.StartNodeID))
	}
	for _, node := range g.Nodes {
		edges := g.edgesFrom(node.ID)
		var last *GraphEdge
		for index := range edges {
			edge := edges[index]
			if edge.Hidden {
				continue
			}
			if last != nil {
				fmt.Fprintf(&b, "trans%d(no)->trans%d\n", last.ID, edge.ID)
			}
			fmt.Fprintf(&b, "trans%d(yes)->%s\n", edge.ID, g.nodeDirection(edge.ToNodeID))
			last = &edge
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (g GraphMetadata) nodeDirection(nodeID int64) string {
	parts := make([]string, 0, 4)
	for _, action := range g.actionsForNode(nodeID) {
		parts = append(parts, fmt.Sprintf("action%d", action.ID))
	}
	parts = append(parts, fmt.Sprintf("node%d", nodeID))
	edges := g.edgesFrom(nodeID)
	for _, edge := range edges {
		if edge.Hidden {
			continue
		}
		parts = append(parts, fmt.Sprintf("trans%d", edge.ID))
		return strings.Join(parts, "->")
	}
	parts = append(parts, "e")
	return strings.Join(parts, "->")
}

func (g GraphMetadata) edgesFrom(nodeID int64) []GraphEdge {
	var out []GraphEdge
	for _, edge := range g.Edges {
		if edge.FromNodeID == nodeID {
			out = append(out, edge)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func (g GraphMetadata) actionsForNode(nodeID int64) []GraphAction {
	var out []GraphAction
	for _, action := range g.Actions {
		if action.NodeID == nodeID {
			out = append(out, action)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func flowchartLabel(label string) string {
	label = strings.TrimSpace(label)
	label = strings.ReplaceAll(label, "\n", " ")
	label = strings.ReplaceAll(label, "\r", " ")
	if label == "" {
		return "-"
	}
	return label
}

func (w Workflow) enterNode(process Process, ctx EvaluationContext, hooks Hooks, depth int) (Process, []ActionResult, error) {
	if depth > len(w.Nodes)+len(w.allTransitions())+1 {
		return process, nil, fmt.Errorf("workflow %d auto transition cycle detected", w.ID)
	}
	node, ok := w.nodeByID(process.NodeID)
	if !ok {
		return process, nil, fmt.Errorf("unknown workflow node %d", process.NodeID)
	}
	if node.State != "" {
		process.State = node.State
	}
	process.ApprovalDoneUserIDs = nil
	process.ApprovalUserIDs = node.approvalUserIDs()
	process.UpdatedAt = ctx.now()
	process.EscalationDate = nodeEscalationDate(node, process.UpdatedAt)
	results, err := runNodeActions(node, process, ctx, hooks)
	if err != nil {
		return process, nil, err
	}
	if node.Type == NodeTypeEnd {
		process.Active = false
		return process, results, nil
	}
	if node.Type != NodeTypeAuto && node.Type != NodeTypeTrigger {
		return process, results, nil
	}
	transitions, err := w.AvailableTransitions(process, ctx)
	if err != nil {
		return process, results, err
	}
	if len(transitions) == 0 {
		return process, results, nil
	}
	next, more, err := w.applyAutoTransition(process, transitions[0], ctx, hooks, depth+1)
	results = append(results, more...)
	return next, results, err
}

func (w Workflow) applyAutoTransition(process Process, transition Transition, ctx EvaluationContext, hooks Hooks, depth int) (Process, []ActionResult, error) {
	if transition.IsEmail && !ctx.MailComposed && hooks.MailCompose != nil {
		process.LastTransitionID = transition.ID
		process.UpdatedAt = ctx.now()
		result, err := hooks.MailCompose(transition, process, ctx)
		if err != nil {
			return process, nil, err
		}
		return process, []ActionResult{result}, nil
	}
	oldNodeID := process.NodeID
	process.NodeID = transition.NextNodeID
	process.LastTransitionID = transition.ID
	process.UpdatedAt = ctx.now()
	if err := emitApprovalLog(process, ctx, hooks, ApprovalLogEvent{
		WorkflowID:   w.ID,
		Model:        process.Model,
		RecordID:     process.RecordID,
		OldNodeID:    oldNodeID,
		NewNodeID:    process.NodeID,
		TransitionID: transition.ID,
	}); err != nil {
		return process, nil, err
	}
	return w.enterNode(process, ctx, hooks, depth)
}

func (w Workflow) ApplyEscalation(process Process, ctx EvaluationContext, hooks Hooks) (Process, []ActionResult, error) {
	if !process.Active {
		return process, nil, fmt.Errorf("workflow process is inactive")
	}
	node, ok := w.nodeByID(process.NodeID)
	if !ok {
		return process, nil, fmt.Errorf("unknown workflow node %d", process.NodeID)
	}
	if !node.Escalation || node.EscalationNodeID == 0 {
		return process, nil, fmt.Errorf("workflow node %d has no escalation target", node.ID)
	}
	oldNodeID := process.NodeID
	process.NodeID = node.EscalationNodeID
	process.LastTransitionID = 0
	process.UpdatedAt = ctx.now()
	if err := emitApprovalLog(process, ctx, hooks, ApprovalLogEvent{
		WorkflowID: w.ID,
		Model:      process.Model,
		RecordID:   process.RecordID,
		OldNodeID:  oldNodeID,
		NewNodeID:  process.NodeID,
		Details: map[string]string{
			"description": "Workflow escalation",
		},
	}); err != nil {
		return process, nil, err
	}
	return w.enterNode(process, ctx, hooks, 0)
}

func runNodeActions(node Node, process Process, ctx EvaluationContext, hooks Hooks) ([]ActionResult, error) {
	actions := orderedActions(node.Actions)
	results := make([]ActionResult, 0, len(actions))
	for _, action := range actions {
		if !action.Active {
			continue
		}
		ok, err := action.Condition.Evaluate(ctx)
		if err != nil {
			return results, err
		}
		if !ok {
			continue
		}
		if hooks.Action == nil {
			continue
		}
		result, err := hooks.Action(action, process, ctx)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (w Workflow) findAvailableTransition(process Process, transitionID int64, ctx EvaluationContext) (Transition, error) {
	transitions, err := w.AvailableTransitions(process, ctx)
	if err != nil {
		return Transition{}, err
	}
	for _, transition := range transitions {
		if transition.ID == transitionID {
			return transition, nil
		}
	}
	return Transition{}, fmt.Errorf("transition %d is not available from node %d", transitionID, process.NodeID)
}

func (w Workflow) nodeByID(id int64) (Node, bool) {
	for _, node := range w.Nodes {
		if node.ID == id {
			return node, true
		}
	}
	return Node{}, false
}

func (w Workflow) startNode() (Node, bool) {
	if w.StartNodeID != 0 {
		return w.nodeByID(w.StartNodeID)
	}
	nodes := orderedNodes(w.Nodes)
	for _, node := range nodes {
		if node.Active {
			return node, true
		}
	}
	return Node{}, false
}

func (w Workflow) startNodeID() int64 {
	node, ok := w.startNode()
	if !ok {
		return 0
	}
	return node.ID
}

func (w Workflow) companyAllowed(companyID int64) bool {
	if len(w.CompanyIDs) == 0 || companyID == 0 {
		return true
	}
	return containsInt64(w.CompanyIDs, companyID)
}

func (w Workflow) allTransitions() []Transition {
	var transitions []Transition
	for _, node := range w.Nodes {
		transitions = append(transitions, node.Transitions...)
	}
	return transitions
}

func (n Node) approvalUserIDs() []int64 {
	return append([]int64(nil), n.ResponsibleUserIDs...)
}

func nodeEscalationDate(node Node, at time.Time) time.Time {
	if !node.Escalation {
		return time.Time{}
	}
	delay := node.EscalationDelay
	switch node.EscalationDelayType {
	case DelayMinutes:
		return at.Add(time.Duration(delay) * time.Minute)
	case DelayHours:
		return at.Add(time.Duration(delay) * time.Hour)
	case DelayMonths:
		return at.AddDate(0, delay, 0)
	case DelayDays, "":
		return at.AddDate(0, 0, delay)
	default:
		return at.AddDate(0, 0, delay)
	}
}

func emitApprovalLog(process Process, ctx EvaluationContext, hooks Hooks, event ApprovalLogEvent) error {
	if hooks.ApprovalLog == nil {
		return nil
	}
	event.At = ctx.now()
	event.UserID = ctx.UserID
	if event.WorkflowID == 0 {
		event.WorkflowID = process.WorkflowID
	}
	if event.Model == "" {
		event.Model = process.Model
	}
	if event.RecordID == 0 {
		event.RecordID = process.RecordID
	}
	if event.DelegationID == 0 {
		event.DelegationID = ctx.DelegationID
	}
	return hooks.ApprovalLog(event)
}

func orderedNodes(nodes []Node) []Node {
	out := append([]Node(nil), nodes...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func orderedTransitions(transitions []Transition) []Transition {
	out := append([]Transition(nil), transitions...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func orderedActions(actions []NodeAction) []NodeAction {
	out := append([]NodeAction(nil), actions...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Sequence == out[j].Sequence {
			return out[i].ID < out[j].ID
		}
		return out[i].Sequence < out[j].Sequence
	})
	return out
}

func (ctx EvaluationContext) now() time.Time {
	if !ctx.Now.IsZero() {
		return ctx.Now
	}
	return time.Now()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func containsInt64(values []int64, value int64) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func intersectsInt64(a, b []int64) bool {
	for _, value := range a {
		if containsInt64(b, value) {
			return true
		}
	}
	return false
}

func mermaidLabel(value string) string {
	return strings.NewReplacer("|", "/", "\n", " ", "\r", " ").Replace(value)
}

type tokenKind int

const (
	tokenEOF tokenKind = iota
	tokenIdentifier
	tokenString
	tokenNumber
	tokenOperator
	tokenLParen
	tokenRParen
)

type exprToken struct {
	kind  tokenKind
	value string
}

type expressionParser struct {
	tokens []exprToken
	pos    int
	ctx    EvaluationContext
}

func newExpressionParser(input string, ctx EvaluationContext) (*expressionParser, error) {
	tokens, err := lexExpression(input)
	if err != nil {
		return nil, err
	}
	return &expressionParser{tokens: tokens, ctx: ctx}, nil
}

func (p *expressionParser) parse() (bool, error) {
	value, err := p.parseOr()
	if err != nil {
		return false, err
	}
	if p.current().kind != tokenEOF {
		return false, fmt.Errorf("unexpected token %q", p.current().value)
	}
	return truthy(value), nil
}

func (p *expressionParser) parseOr() (any, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.matchKeyword("or") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = truthy(left) || truthy(right)
	}
	return left, nil
}

func (p *expressionParser) parseAnd() (any, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.matchKeyword("and") {
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = truthy(left) && truthy(right)
	}
	return left, nil
}

func (p *expressionParser) parseNot() (any, error) {
	if p.matchKeyword("not") {
		value, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return !truthy(value), nil
	}
	return p.parseComparison()
}

func (p *expressionParser) parseComparison() (any, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if p.current().kind == tokenOperator {
		op := p.current().value
		p.pos++
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return compareValues(left, op, right)
	}
	if p.matchKeyword("in") {
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return containsValue(right, left)
	}
	if p.currentIsKeyword("not") && p.peekIsKeyword("in") {
		p.pos += 2
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		ok, err := containsValue(right, left)
		if err != nil {
			return nil, err
		}
		return !ok, nil
	}
	return left, nil
}

func (p *expressionParser) parsePrimary() (any, error) {
	token := p.current()
	switch token.kind {
	case tokenIdentifier:
		p.pos++
		switch strings.ToLower(token.value) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil", "null", "none":
			return nil, nil
		default:
			return resolveIdentifier(token.value, p.ctx)
		}
	case tokenString:
		p.pos++
		return token.value, nil
	case tokenNumber:
		p.pos++
		if strings.Contains(token.value, ".") {
			return strconv.ParseFloat(token.value, 64)
		}
		return strconv.ParseInt(token.value, 10, 64)
	case tokenLParen:
		p.pos++
		value, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.current().kind != tokenRParen {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return value, nil
	default:
		return nil, fmt.Errorf("unexpected token %q", token.value)
	}
}

func (p *expressionParser) current() exprToken {
	if p.pos >= len(p.tokens) {
		return exprToken{kind: tokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *expressionParser) matchKeyword(keyword string) bool {
	if p.currentIsKeyword(keyword) {
		p.pos++
		return true
	}
	return false
}

func (p *expressionParser) currentIsKeyword(keyword string) bool {
	token := p.current()
	return token.kind == tokenIdentifier && strings.EqualFold(token.value, keyword)
}

func (p *expressionParser) peekIsKeyword(keyword string) bool {
	if p.pos+1 >= len(p.tokens) {
		return false
	}
	token := p.tokens[p.pos+1]
	return token.kind == tokenIdentifier && strings.EqualFold(token.value, keyword)
}

func lexExpression(input string) ([]exprToken, error) {
	var tokens []exprToken
	for i := 0; i < len(input); {
		r := rune(input[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if r == '(' {
			tokens = append(tokens, exprToken{kind: tokenLParen, value: "("})
			i++
			continue
		}
		if r == ')' {
			tokens = append(tokens, exprToken{kind: tokenRParen, value: ")"})
			i++
			continue
		}
		if r == '"' || r == '\'' {
			value, next, err := readString(input, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, exprToken{kind: tokenString, value: value})
			i = next
			continue
		}
		if isOperatorStart(byte(r)) {
			value, next, err := readOperator(input, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, exprToken{kind: tokenOperator, value: value})
			i = next
			continue
		}
		if unicode.IsDigit(r) || (r == '-' && i+1 < len(input) && unicode.IsDigit(rune(input[i+1]))) {
			value, next := readNumber(input, i)
			tokens = append(tokens, exprToken{kind: tokenNumber, value: value})
			i = next
			continue
		}
		if isIdentifierStart(r) {
			value, next := readIdentifier(input, i)
			tokens = append(tokens, exprToken{kind: tokenIdentifier, value: value})
			i = next
			continue
		}
		return nil, fmt.Errorf("unsupported character %q", r)
	}
	tokens = append(tokens, exprToken{kind: tokenEOF})
	return tokens, nil
}

func readString(input string, start int) (string, int, error) {
	quote := input[start]
	var b strings.Builder
	for i := start + 1; i < len(input); i++ {
		ch := input[i]
		if ch == quote {
			return b.String(), i + 1, nil
		}
		if ch == '\\' && i+1 < len(input) {
			i++
			switch input[i] {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(input[i])
			}
			continue
		}
		b.WriteByte(ch)
	}
	return "", 0, fmt.Errorf("unterminated string")
}

func readOperator(input string, start int) (string, int, error) {
	if start+1 < len(input) {
		two := input[start : start+2]
		switch two {
		case "==", "!=", ">=", "<=":
			return two, start + 2, nil
		}
	}
	switch input[start] {
	case '=', '>', '<':
		return string(input[start]), start + 1, nil
	default:
		return "", 0, fmt.Errorf("unsupported operator %q", input[start])
	}
}

func readNumber(input string, start int) (string, int) {
	i := start
	if input[i] == '-' {
		i++
	}
	for i < len(input) && (unicode.IsDigit(rune(input[i])) || input[i] == '.') {
		i++
	}
	return input[start:i], i
}

func readIdentifier(input string, start int) (string, int) {
	i := start
	for i < len(input) && isIdentifierPart(rune(input[i])) {
		i++
	}
	return input[start:i], i
}

func isOperatorStart(ch byte) bool {
	return ch == '=' || ch == '!' || ch == '>' || ch == '<'
}

func isIdentifierStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentifierPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.'
}

func resolveIdentifier(name string, ctx EvaluationContext) (any, error) {
	switch name {
	case "user.id":
		return ctx.UserID, nil
	case "user.company_id":
		return ctx.CompanyID, nil
	case "user.company_ids":
		return append([]int64(nil), ctx.CompanyIDs...), nil
	case "user.group_ids":
		return append([]int64(nil), ctx.UserGroupIDs...), nil
	case "record.id":
		return ctx.RecordID, nil
	case "record.model":
		return ctx.Model, nil
	}
	if strings.HasPrefix(name, "record.") {
		name = strings.TrimPrefix(name, "record.")
	}
	if ctx.Values != nil {
		if value, ok := ctx.Values[name]; ok {
			return value, nil
		}
	}
	return nil, fmt.Errorf("unknown identifier %q", name)
}

func compareValues(left any, op string, right any) (bool, error) {
	switch op {
	case "=", "==":
		return equalValues(left, right), nil
	case "!=":
		return !equalValues(left, right), nil
	case ">", ">=", "<", "<=":
		leftFloat, leftOK := toFloat(left)
		rightFloat, rightOK := toFloat(right)
		if leftOK && rightOK {
			switch op {
			case ">":
				return leftFloat > rightFloat, nil
			case ">=":
				return leftFloat >= rightFloat, nil
			case "<":
				return leftFloat < rightFloat, nil
			case "<=":
				return leftFloat <= rightFloat, nil
			}
		}
		leftString, leftOK := left.(string)
		rightString, rightOK := right.(string)
		if leftOK && rightOK {
			switch op {
			case ">":
				return leftString > rightString, nil
			case ">=":
				return leftString >= rightString, nil
			case "<":
				return leftString < rightString, nil
			case "<=":
				return leftString <= rightString, nil
			}
		}
		return false, fmt.Errorf("operator %s requires comparable values", op)
	default:
		return false, fmt.Errorf("unsupported operator %q", op)
	}
}

func equalValues(left, right any) bool {
	if leftFloat, ok := toFloat(left); ok {
		if rightFloat, ok := toFloat(right); ok {
			return leftFloat == rightFloat
		}
	}
	return reflect.DeepEqual(left, right)
}

func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func containsValue(container any, value any) (bool, error) {
	if container == nil {
		return false, nil
	}
	if text, ok := container.(string); ok {
		valueText, ok := value.(string)
		if !ok {
			return false, nil
		}
		return strings.Contains(text, valueText), nil
	}
	rv := reflect.ValueOf(container)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false, fmt.Errorf("in operator requires slice, array, or string")
	}
	for i := 0; i < rv.Len(); i++ {
		if equalValues(rv.Index(i).Interface(), value) {
			return true, nil
		}
	}
	return false, nil
}

func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return rv.Len() > 0
		default:
			return true
		}
	}
}

func workflowModel() model.Model {
	m := model.New(ModelWorkflow, "workflow")
	m.Inherit = []string{"mail.thread", "mail.activity.mixin"}
	for _, f := range []field.Field{
		required(field.New("name", field.Char)),
		field.New("code", field.Char),
		required(field.New("approval_settings_id", field.Many2One).WithRelation("approval.settings")),
		field.New("model_id", field.Many2One).WithRelation("ir.model"),
		field.New("model", field.Char),
		field.New("sequence", field.Int),
		field.New("active", field.Bool),
		field.New("condition", field.Char),
		field.New("node_ids", field.One2Many).WithRelation(ModelNode),
		field.New("node_count", field.Int),
		field.New("flowchart", field.Text),
		field.New("state", field.Char),
		field.New("view_id", field.Many2One).WithRelation("ir.ui.view"),
		field.New("create_context", field.Text),
		field.New("on_create", field.Bool),
		field.New("company_id", field.Many2One).WithRelation("res.company"),
		field.New("company_ids", field.Many2Many).WithRelation("res.company"),
		field.New("start_node_id", field.Many2One).WithRelation(ModelNode),
	} {
		m.AddField(f)
	}
	return m
}

func nodeModel() model.Model {
	m := model.New(ModelNode, "workflow_node")
	for _, f := range []field.Field{
		required(field.New("name", field.Char)),
		field.New("code", field.Char),
		required(field.New("workflow_id", field.Many2One).WithRelation(ModelWorkflow)),
		field.New("model_id", field.Many2One).WithRelation("ir.model"),
		field.New("model", field.Char),
		field.New("sequence", field.Int),
		field.New("active", field.Bool),
		selectionField("type", []field.SelectionOption{
			{Value: string(NodeTypeUser), Label: "User Choice"},
			{Value: string(NodeTypeAuto), Label: "Automatic"},
			{Value: string(NodeTypeTrigger), Label: "Trigger"},
			{Value: string(NodeTypeEnd), Label: "End"},
		}),
		field.New("responsible_group_ids", field.Many2Many).WithRelation("res.groups"),
		field.New("responsible_user_ids", field.Many2Many).WithRelation("res.users"),
		field.New("responsible_python_code", field.Char),
		field.New("responsible_condition", field.Char),
		field.New("responsible_committee", field.Bool),
		field.New("responsible_committee_limit", field.Int),
		field.New("schedule_activity", field.Bool),
		field.New("schedule_activity_field_id", field.Many2One).WithRelation("ir.model.fields"),
		field.New("schedule_activity_days", field.Int),
		field.New("schedule_activity_enabled", field.Bool),
		field.New("action_ids", field.One2Many).WithRelation(ModelNodeAction),
		field.New("state", field.Char),
		field.New("transition_ids", field.One2Many).WithRelation(ModelTransition),
		selectionField("button_type", []field.SelectionOption{
			{Value: string(ButtonTypeOne), Label: "One"},
			{Value: string(ButtonTypeMulti), Label: "Multiple"},
		}),
		field.New("button_name", field.Char),
		field.New("button_context", field.Char),
		field.New("button_icon", field.Char),
		field.New("button_validate_form", field.Bool),
		field.New("wizard_view_id", field.Many2One).WithRelation("ir.ui.view"),
		field.New("allow_forward", field.Bool),
		field.New("escalation", field.Bool),
		selectionField("escalation_delay_type", []field.SelectionOption{
			{Value: string(DelayMinutes), Label: "Minutes"},
			{Value: string(DelayHours), Label: "Hours"},
			{Value: string(DelayDays), Label: "Days"},
			{Value: string(DelayMonths), Label: "Months"},
		}),
		field.New("escalation_delay", field.Int),
		field.New("escalation_node_id", field.Many2One).WithRelation(ModelNode),
		field.New("trg_date_calendar_id", field.Many2One).WithRelation("resource.calendar"),
	} {
		m.AddField(f)
	}
	return m
}

func transitionModel() model.Model {
	m := model.New(ModelTransition, "workflow_transition")
	for _, f := range []field.Field{
		required(field.New("name", field.Char)),
		required(field.New("node_id", field.Many2One).WithRelation(ModelNode)),
		selectionField("button_type", []field.SelectionOption{
			{Value: string(ButtonTypeOne), Label: "One"},
			{Value: string(ButtonTypeMulti), Label: "Multiple"},
		}),
		selectionField("type", []field.SelectionOption{
			{Value: string(NodeTypeUser), Label: "User Choice"},
			{Value: string(NodeTypeAuto), Label: "Automatic"},
			{Value: string(NodeTypeTrigger), Label: "Trigger"},
			{Value: string(NodeTypeEnd), Label: "End"},
		}),
		field.New("model_id", field.Many2One).WithRelation("ir.model"),
		field.New("workflow_id", field.Many2One).WithRelation(ModelWorkflow),
		field.New("sequence", field.Int),
		field.New("active", field.Bool),
		field.New("run_as_superuser", field.Bool),
		field.New("condition", field.Char),
		field.New("code", field.Char),
		required(field.New("next_node_id", field.Many2One).WithRelation(ModelNode)),
		field.New("groups_ids", field.Many2Many).WithRelation("res.groups"),
		selectionField("comment", []field.SelectionOption{
			{Value: string(CommentOptional), Label: "Optional"},
			{Value: string(CommentRequired), Label: "Required"},
		}),
		field.New("button_class", field.Char),
		field.New("wizard_view_id", field.Many2One).WithRelation("ir.ui.view"),
		field.New("context", field.Text),
		field.New("icon", field.Char),
		field.New("committee", field.Bool),
		field.New("committee_limit", field.Int),
		field.New("validate_form", field.Bool),
		field.New("trigger", field.Selection),
		field.New("is_email", field.Bool),
		field.New("email_template_id", field.Many2One).WithRelation("mail.template"),
		field.New("email_wizard_form_id", field.Many2One).WithRelation("ir.ui.view"),
		field.New("is_hidden", field.Bool),
	} {
		m.AddField(f)
	}
	return m
}

func nodeActionModel() model.Model {
	m := model.New(ModelNodeAction, "workflow_node_action")
	for _, f := range []field.Field{
		required(field.New("node_id", field.Many2One).WithRelation(ModelNode)),
		field.New("sequence", field.Int),
		field.New("active", field.Bool),
		field.New("condition", field.Char),
		required(field.New("server_action_id", field.Many2One).WithRelation("ir.actions.server")),
		field.New("action_key", field.Char),
	} {
		m.AddField(f)
	}
	return m
}

func processModel() model.Model {
	m := model.New(ModelProcess, "workflow_process")
	for _, f := range []field.Field{
		field.New("workflow_id", field.Many2One).WithRelation(ModelWorkflow),
		required(field.New("model", field.Char)),
		required(field.New("record_id", field.Int)),
		field.New("node_id", field.Many2One).WithRelation(ModelNode),
		field.New("active", field.Bool),
		field.New("state", field.Char),
		field.New("last_transition_id", field.Many2One).WithRelation(ModelTransition),
		field.New("started_at", field.DateTime),
		field.New("updated_at", field.DateTime),
		field.New("approval_user_ids", field.Many2Many).WithRelation("res.users"),
		field.New("approval_done_user_ids", field.Many2Many).WithRelation("res.users"),
		field.New("approval_partner_ids", field.Many2Many).WithRelation("res.partner"),
		field.New("user_can_approve", field.Bool),
		field.New("escalation_date", field.DateTime),
	} {
		m.AddField(f)
	}
	return m
}

func processWizardModel() model.Model {
	m := model.New(ModelWorkflowWizard, "workflow_process_wizard")
	m.Transient = true
	for _, f := range []field.Field{
		required(field.New("model", field.Char)),
		required(field.New("record_id", field.Int)),
		field.New("record_name", field.Char),
		field.New("workflow_process_id", field.Many2One).WithRelation(ModelProcess),
		field.New("workflow_node_id", field.Many2One).WithRelation(ModelNode),
		field.New("workflow_id", field.Many2One).WithRelation(ModelWorkflow),
		field.New("workflow_transition_ids", field.Many2Many).WithRelation(ModelTransition),
		required(field.New("workflow_transition_id", field.Many2One).WithRelation(ModelTransition)),
		field.New("comment", field.Text),
		field.New("comment_required", field.Bool),
	} {
		m.AddField(f)
	}
	return m
}

func selectionField(name string, options []field.SelectionOption) field.Field {
	f := field.New(name, field.Selection)
	f.Selection = append([]field.SelectionOption(nil), options...)
	return f
}

func required(f field.Field) field.Field {
	f.Required = true
	return f
}

func advancedExtension(name string, table string, fields ...field.Field) model.Model {
	m := model.New(name, table)
	m.Inherit = []string{name}
	if table == "" {
		m.Abstract = true
	}
	for _, f := range fields {
		m.AddField(f)
	}
	return m
}
