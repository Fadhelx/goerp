package oi_delegation

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorp/internal/delegation"
	"gorp/internal/mail"
	"gorp/internal/security"
)

const (
	MailTemplateDelegationAssigned int64 = 1
	MailTemplateDelegationRevoked  int64 = 2
	MailTemplateDelegationExpired  int64 = 3
)

type MailRecipient struct {
	Email string
	Name  string
}

type MailRecipientResolver func(userID int64) (MailRecipient, bool)

func NewRuntimeService(engine *security.Engine, outbox *mail.Outbox, templates map[int64]mail.Template, resolver MailRecipientResolver, now func() time.Time) *delegation.Service {
	if len(templates) == 0 {
		templates = RuntimeMailTemplates()
	}
	options := []delegation.Option{}
	if now != nil {
		options = append(options, delegation.WithNow(now))
	}
	if engine != nil {
		options = append(options, delegation.WithCacheInvalidator(engine))
	}
	if outbox != nil {
		options = append(options, delegation.WithWorkflowHooks(NewMailHooks(outbox, templates, resolver, now)))
	}
	if resolver != nil {
		options = append(options, delegation.WithMailResolver(NewTemplateMailResolver(templates, resolver)))
	}
	svc := delegation.NewService(options...)
	ConfigureService(svc)
	BindSecurity(engine, svc)
	return svc
}

func BindSecurity(engine *security.Engine, svc *delegation.Service) {
	if engine == nil || svc == nil {
		return
	}
	engine.SetDelegationProvider(svc)
}

func RuntimeMailTemplates() map[int64]mail.Template {
	return map[int64]mail.Template{
		MailTemplateDelegationAssigned: {
			ID:      MailTemplateDelegationAssigned,
			Name:    "Delegation Assigned",
			To:      "{{ email }}",
			Subject: "Delegation assigned",
			Body:    "<p>A delegation request has been assigned.</p>",
		},
		MailTemplateDelegationRevoked: {
			ID:      MailTemplateDelegationRevoked,
			Name:    "Delegation Revoked",
			To:      "{{ email }}",
			Subject: "Delegation revoked",
			Body:    "<p>A delegation request has been revoked.</p>",
		},
		MailTemplateDelegationExpired: {
			ID:      MailTemplateDelegationExpired,
			Name:    "Delegation Expired",
			To:      "{{ email }}",
			Subject: "Delegation expired",
			Body:    "<p>A delegation request has expired.</p>",
		},
	}
}

type TemplateMailResolver struct {
	templates map[int64]mail.Template
	resolver  MailRecipientResolver
}

func NewTemplateMailResolver(templates map[int64]mail.Template, resolver MailRecipientResolver) *TemplateMailResolver {
	if len(templates) == 0 {
		templates = RuntimeMailTemplates()
	}
	return &TemplateMailResolver{templates: cloneMailTemplateMap(templates), resolver: resolver}
}

func (r *TemplateMailResolver) ExpandCC(ctx delegation.MailContext) ([]string, error) {
	if r == nil || r.resolver == nil {
		return nil, nil
	}
	template, ok := r.templates[ctx.TemplateID]
	groupIDs := ctx.TemplateGroupIDs
	if ok && len(template.DelegationGroupIDs) > 0 {
		groupIDs = template.DelegationGroupIDs
	}
	if len(groupIDs) == 0 {
		return nil, nil
	}
	var cc []string
	for _, userID := range sortedUserIDs(ctx.DelegatedUserGroupID) {
		if !intersectsInt64(ctx.DelegatedUserGroupID[userID], groupIDs) {
			continue
		}
		recipient, ok := r.resolver(userID)
		if !ok || strings.TrimSpace(recipient.Email) == "" {
			continue
		}
		cc = append(cc, recipient.Email)
	}
	return uniqueStrings(cc), nil
}

func EnqueueTemplateWithDelegationCC(outbox *mail.Outbox, svc *delegation.Service, template mail.Template, values map[string]any, ctx delegation.MailContext, now time.Time) (int64, error) {
	if outbox == nil {
		return 0, fmt.Errorf("outbox unavailable")
	}
	rendered := template.Render(values)
	initialCC := splitRecipients(rendered.CC)
	cc := initialCC
	var err error
	if svc != nil {
		ctx.TemplateID = template.ID
		ctx.TemplateGroupIDs = append([]int64(nil), template.DelegationGroupIDs...)
		ctx.InitialCC = initialCC
		cc, err = svc.ExpandMailCC(ctx)
		if err != nil {
			return 0, err
		}
	}
	return outbox.Enqueue(mail.Message{
		To:      rendered.To,
		CC:      strings.Join(cc, ", "),
		Subject: rendered.Subject,
		Body:    rendered.Body,
	}, now)
}

func SecurityMailRecipientResolver(engine *security.Engine) MailRecipientResolver {
	return func(userID int64) (MailRecipient, bool) {
		if engine == nil {
			return MailRecipient{}, false
		}
		user, ok := engine.Users[userID]
		if !ok || strings.TrimSpace(user.Email) == "" {
			return MailRecipient{}, false
		}
		return MailRecipient{Email: user.Email, Name: user.Name}, true
	}
}

func NewMailHooks(outbox *mail.Outbox, templates map[int64]mail.Template, resolver MailRecipientResolver, now func() time.Time) delegation.WorkflowHooks {
	if len(templates) == 0 {
		templates = RuntimeMailTemplates()
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &mailHooks{
		outbox:    outbox,
		templates: cloneMailTemplateMap(templates),
		resolver:  resolver,
		now:       now,
	}
}

type mailHooks struct {
	outbox    *mail.Outbox
	templates map[int64]mail.Template
	resolver  MailRecipientResolver
	now       func() time.Time
}

func (h *mailHooks) OnSubmitted(delegation.Request) error {
	return nil
}

func (h *mailHooks) OnConfirmed(req delegation.Request) error {
	return h.dispatch(MailTemplateDelegationAssigned, req)
}

func (h *mailHooks) OnRevoked(req delegation.Request) error {
	return h.dispatch(MailTemplateDelegationRevoked, req)
}

func (h *mailHooks) OnExpired(req delegation.Request) error {
	return h.dispatch(MailTemplateDelegationExpired, req)
}

func (h *mailHooks) dispatch(templateID int64, req delegation.Request) error {
	if h.outbox == nil {
		return fmt.Errorf("delegation mail outbox unavailable")
	}
	if h.resolver == nil {
		return fmt.Errorf("delegation mail recipient resolver unavailable")
	}
	template, ok := h.templates[templateID]
	if !ok {
		return fmt.Errorf("delegation mail template %d unavailable", templateID)
	}
	groupsByUser := delegatedGroupsByUser(req)
	userIDs := make([]int64, 0, len(groupsByUser))
	for delegateUserID := range groupsByUser {
		userIDs = append(userIDs, delegateUserID)
	}
	sort.Slice(userIDs, func(i, j int) bool { return userIDs[i] < userIDs[j] })
	for _, delegateUserID := range userIDs {
		groupIDs := groupsByUser[delegateUserID]
		recipient, ok := h.resolver(delegateUserID)
		if !ok {
			return fmt.Errorf("delegation mail recipient %d unavailable", delegateUserID)
		}
		if _, err := mail.EnqueueTemplate(h.outbox, template, delegationMailValues(req, delegateUserID, groupIDs, recipient), h.now().UTC()); err != nil {
			return err
		}
	}
	return nil
}

func delegatedGroupsByUser(req delegation.Request) map[int64][]int64 {
	out := map[int64][]int64{}
	seen := map[int64]map[int64]bool{}
	for _, line := range req.Lines {
		if line.DelegateUserID == 0 || line.GroupID == 0 {
			continue
		}
		if seen[line.DelegateUserID] == nil {
			seen[line.DelegateUserID] = map[int64]bool{}
		}
		if seen[line.DelegateUserID][line.GroupID] {
			continue
		}
		seen[line.DelegateUserID][line.GroupID] = true
		out[line.DelegateUserID] = append(out[line.DelegateUserID], line.GroupID)
	}
	for userID := range out {
		sort.Slice(out[userID], func(i, j int) bool { return out[userID][i] < out[userID][j] })
	}
	return out
}

func delegationMailValues(req delegation.Request, delegateUserID int64, groupIDs []int64, recipient MailRecipient) map[string]any {
	return map[string]any{
		"email":             recipient.Email,
		"name":              recipient.Name,
		"request_id":        req.ID,
		"request_name":      req.Name,
		"state":             string(req.State),
		"delegator_user_id": req.DelegatorUserID,
		"delegate_user_id":  delegateUserID,
		"group_ids":         formatInt64s(groupIDs),
		"date_from":         req.DateFrom.Format("2006-01-02"),
		"date_to":           req.DateTo.Format("2006-01-02"),
		"source_model":      req.SourceModel,
		"source_record_id":  req.SourceRecordID,
	}
}

func formatInt64s(values []int64) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatInt(value, 10))
	}
	return strings.Join(parts, ",")
}

func sortedUserIDs(values map[int64][]int64) []int64 {
	ids := make([]int64, 0, len(values))
	for id := range values {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func intersectsInt64(left []int64, right []int64) bool {
	seen := map[int64]bool{}
	for _, id := range left {
		seen[id] = true
	}
	for _, id := range right {
		if seen[id] {
			return true
		}
	}
	return false
}

func splitRecipients(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if trimmed := strings.TrimSpace(field); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return uniqueStrings(out)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cloneMailTemplateMap(in map[int64]mail.Template) map[int64]mail.Template {
	out := make(map[int64]mail.Template, len(in))
	for id, template := range in {
		template.DelegationGroupIDs = append([]int64(nil), template.DelegationGroupIDs...)
		out[id] = template
	}
	return out
}
