import type { ActionBreadcrumb } from "../services/action_stack.js";
import type { SearchFacet } from "../search/search_model.js";

export interface ControlPanelPager {
  offset: number;
  limit: number;
  total: number;
}

export interface ControlPanelView {
  type: string;
  label?: string;
  active?: boolean;
}

export interface ControlPanelMenuItem {
  id: string;
  label: string;
  active?: boolean;
  disabled?: boolean;
}

export interface ControlPanelSearchState {
  query: string;
  facets?: readonly SearchFacet[];
  placeholder?: string;
}

export interface ControlPanelState {
  title?: string;
  breadcrumbs?: readonly ActionBreadcrumb[];
  pager?: ControlPanelPager;
  views?: readonly ControlPanelView[];
  search?: ControlPanelSearchState;
  filters?: readonly ControlPanelMenuItem[];
  groupBys?: readonly ControlPanelMenuItem[];
  favorites?: readonly ControlPanelMenuItem[];
}

export interface ControlPanelCallbacks {
  onBreadcrumb?: (breadcrumb: ActionBreadcrumb) => void;
  onSearch?: (query: string) => void;
  onViewSwitch?: (viewType: string) => void;
  onPagerPrevious?: () => void;
  onPagerNext?: () => void;
  onFilter?: (item: ControlPanelMenuItem) => void;
  onGroupBy?: (item: ControlPanelMenuItem) => void;
  onFavorite?: (item: ControlPanelMenuItem) => void;
}

export function createControlPanelState(state: ControlPanelState): ControlPanelState {
  return {
    title: state.title,
    breadcrumbs: [...(state.breadcrumbs ?? [])],
    pager: state.pager ? normalizePager(state.pager) : undefined,
    views: [...(state.views ?? [])].map(normalizeView),
    search: {
      query: state.search?.query ?? "",
      placeholder: state.search?.placeholder ?? "Search...",
      facets: [...(state.search?.facets ?? [])].map((facet) => ({ ...facet }))
    },
    filters: [...(state.filters ?? [])].map(normalizeMenuItem),
    groupBys: [...(state.groupBys ?? [])].map(normalizeMenuItem),
    favorites: [...(state.favorites ?? [])].map(normalizeMenuItem)
  };
}

export function renderControlPanel(state: ControlPanelState, callbacks: ControlPanelCallbacks = {}): HTMLElement {
  const normalized = createControlPanelState(state);
  const root = document.createElement("section");
  root.className = "o_control_panel";

  const top = document.createElement("div");
  top.className = "o_cp_top";
  top.append(renderBreadcrumbs(normalized, callbacks), renderPager(normalized.pager, callbacks));

  const bottom = document.createElement("div");
  bottom.className = "o_cp_bottom";
  bottom.append(
    renderSearch(normalized.search, callbacks),
    renderMenuLane("o_filter_menu", "Filters", normalized.filters ?? [], callbacks.onFilter),
    renderMenuLane("o_group_by_menu", "Group By", normalized.groupBys ?? [], callbacks.onGroupBy),
    renderMenuLane("o_favorites_menu", "Favorites", normalized.favorites ?? [], callbacks.onFavorite),
    renderViewSwitcher(normalized.views ?? [], callbacks)
  );

  root.append(top, bottom);
  return root;
}

function renderBreadcrumbs(state: ControlPanelState, callbacks: ControlPanelCallbacks): HTMLElement {
  const nav = document.createElement("nav");
  nav.className = "o_breadcrumb";
  const breadcrumbs = state.breadcrumbs?.length
    ? state.breadcrumbs
    : [{ id: "current", label: state.title || "" }];
  for (const [index, breadcrumb] of breadcrumbs.entries()) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = index === breadcrumbs.length - 1 ? "breadcrumb-item active" : "breadcrumb-item";
    item.textContent = breadcrumb.label;
    item.dataset.breadcrumbId = breadcrumb.id;
    item.addEventListener("click", () => callbacks.onBreadcrumb?.(breadcrumb));
    nav.append(item);
  }
  return nav;
}

function renderPager(pager: ControlPanelPager | undefined, callbacks: ControlPanelCallbacks): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_cp_pager";
  if (!pager) return root;
  const first = pager.total === 0 ? 0 : pager.offset + 1;
  const last = Math.min(pager.total, pager.offset + pager.limit);
  const value = document.createElement("span");
  value.className = "o_pager_value";
  value.textContent = `${first}-${last}`;
  const limit = document.createElement("span");
  limit.className = "o_pager_limit";
  limit.textContent = String(pager.total);
  const previous = pagerButton("previous", "Previous", pager.offset <= 0, callbacks.onPagerPrevious);
  const next = pagerButton("next", "Next", last >= pager.total, callbacks.onPagerNext);
  root.append(value, document.createTextNode(" / "), limit, previous, next);
  return root;
}

function renderSearch(search: ControlPanelSearchState | undefined, callbacks: ControlPanelCallbacks): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_cp_searchview o_searchview";
  for (const facet of search?.facets ?? []) {
    const tag = document.createElement("span");
    tag.className = `o_searchview_facet o_searchview_facet_${facet.type}`;
    tag.textContent = facet.label;
    tag.dataset.facetId = facet.id;
    root.append(tag);
  }
  const input = document.createElement("input");
  input.className = "o_searchview_input";
  input.type = "search";
  input.value = search?.query ?? "";
  input.placeholder = search?.placeholder ?? "Search...";
  input.addEventListener("input", () => callbacks.onSearch?.(input.value));
  root.append(input);
  return root;
}

function renderMenuLane(
  className: string,
  label: string,
  items: readonly ControlPanelMenuItem[],
  callback: ((item: ControlPanelMenuItem) => void) | undefined
): HTMLElement {
  const root = document.createElement("div");
  root.className = `o_cp_menu ${className}`;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o_cp_menu_button";
  button.textContent = label;
  button.disabled = items.length === 0;
  root.append(button);
  for (const item of items) {
    const menuItem = document.createElement("button");
    menuItem.type = "button";
    menuItem.className = item.active ? "o_menu_item active" : "o_menu_item";
    menuItem.textContent = item.label;
    menuItem.dataset.menuItemId = item.id;
    menuItem.disabled = item.disabled === true;
    menuItem.addEventListener("click", () => callback?.(item));
    root.append(menuItem);
  }
  return root;
}

function renderViewSwitcher(views: readonly ControlPanelView[], callbacks: ControlPanelCallbacks): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_cp_switch_buttons";
  for (const view of views) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = view.active ? "o_switch_view active" : "o_switch_view";
    button.textContent = view.label || view.type;
    button.dataset.viewType = view.type;
    button.addEventListener("click", () => callbacks.onViewSwitch?.(view.type));
    root.append(button);
  }
  return root;
}

function pagerButton(
  direction: string,
  label: string,
  disabled: boolean,
  callback: (() => void) | undefined
): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `o_pager_${direction}`;
  button.textContent = label;
  button.disabled = disabled;
  button.addEventListener("click", () => callback?.());
  return button;
}

function normalizePager(pager: ControlPanelPager): ControlPanelPager {
  const total = Math.max(0, Math.trunc(pager.total || 0));
  const limit = Math.max(1, Math.trunc(pager.limit || 1));
  const maxOffset = Math.max(0, total - 1);
  return {
    total,
    limit,
    offset: Math.min(Math.max(0, Math.trunc(pager.offset || 0)), maxOffset)
  };
}

function normalizeView(view: ControlPanelView): ControlPanelView {
  return {
    type: view.type,
    label: view.label || view.type,
    active: view.active === true
  };
}

function normalizeMenuItem(item: ControlPanelMenuItem): ControlPanelMenuItem {
  return {
    id: item.id,
    label: item.label,
    active: item.active === true,
    disabled: item.disabled === true
  };
}
