package runtime

import (
	"sort"

	internaldelegation "gorp/internal/delegation"
	"gorp/internal/meta/action"
	"gorp/internal/meta/menu"
	"gorp/internal/record"
	"gorp/internal/security"
)

type delegationMenuResolver struct {
	engine       *security.Engine
	menus        *menu.Registry
	actions      *action.Registry
	noOneGroupID int64
}

func newDelegationMenuResolver(engine *security.Engine, menus *menu.Registry, actions *action.Registry, noOneGroupID int64) *delegationMenuResolver {
	return &delegationMenuResolver{engine: engine, menus: menus, actions: actions, noOneGroupID: noOneGroupID}
}

func (r *delegationMenuResolver) VisibleMenuIDs(ctx internaldelegation.AccessContext, debug bool) ([]int64, error) {
	if r == nil || r.menus == nil {
		return nil, nil
	}
	groups := r.effectiveGroups(ctx)
	if !debug && r.noOneGroupID != 0 {
		delete(groups, r.noOneGroupID)
	}
	visible := map[int64]bool{}
	for _, item := range r.menus.All() {
		if item.ActionID == 0 || !menuGroupsAllowed(item.Groups, groups) || !r.menuActionAllowed(ctx.UserID, item) {
			continue
		}
		r.markMenuAndVisibleParents(item, groups, visible)
	}
	ids := make([]int64, 0, len(visible))
	for id := range visible {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (r *delegationMenuResolver) effectiveGroups(ctx internaldelegation.AccessContext) map[int64]bool {
	if r.engine != nil && ctx.UserID != 0 {
		return r.engine.EffectiveGroupIDs(ctx.UserID)
	}
	groups := map[int64]bool{}
	for _, id := range ctx.EffectiveGroupIDs {
		groups[id] = true
	}
	return groups
}

func (r *delegationMenuResolver) menuActionAllowed(userID int64, item menu.Menu) bool {
	if r.actions == nil {
		return true
	}
	act, ok := r.actions.Get(item.ActionID)
	if !ok {
		return false
	}
	if act.Kind != action.ActWindow || act.ResModel == "" || r.engine == nil {
		return true
	}
	return r.engine.Check(record.Context{UserID: userID}, act.ResModel, record.OpRead, nil) == nil
}

func (r *delegationMenuResolver) markMenuAndVisibleParents(item menu.Menu, groups map[int64]bool, visible map[int64]bool) {
	for {
		if !menuGroupsAllowed(item.Groups, groups) {
			return
		}
		visible[item.ID] = true
		if item.ParentID == 0 {
			return
		}
		parent, ok := r.menus.Get(item.ParentID)
		if !ok {
			return
		}
		item = parent
	}
}

func menuGroupsAllowed(required []int64, groups map[int64]bool) bool {
	if len(required) == 0 {
		return true
	}
	for _, id := range required {
		if groups[id] {
			return true
		}
	}
	return false
}
