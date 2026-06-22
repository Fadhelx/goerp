import type { ActionBreadcrumb } from "../services/action_stack.js";
import { searchFacetDisplay, type SearchFacet } from "../search/search_model.js";

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

export interface ControlPanelFavoriteMetadata {
  id?: number;
  userId?: number;
  actionId?: number;
  embeddedActionId?: number;
  isDefault?: boolean;
  isGlobal?: boolean;
  canDelete?: boolean;
}

export interface ControlPanelMenuItem {
  id: string;
  label: string;
  active?: boolean;
  disabled?: boolean;
  facet?: SearchFacet;
  favorite?: ControlPanelFavoriteMetadata;
  children?: readonly ControlPanelMenuItem[];
  separatorBefore?: boolean;
}

export interface ControlPanelSearchState {
  query: string;
  facets?: readonly SearchFacet[];
  placeholder?: string;
  suggestions?: readonly ControlPanelSearchSuggestion[];
}

export interface ControlPanelSearchSuggestion {
  id: string;
  label: string;
  field: string;
  value: string;
  operator?: string;
  facet?: SearchFacet;
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
  onDeleteFavorite?: (item: ControlPanelMenuItem) => void;
  onFacetRemove?: (facet: SearchFacet) => void;
  onSearchSuggestion?: (suggestion: ControlPanelSearchSuggestion) => void;
  onAddCustomFilter?: () => void;
  onAddCustomGroup?: () => void;
  onAddFavorite?: () => void;
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
      facets: [...(state.search?.facets ?? [])].map((facet) => ({ ...facet })),
      suggestions: [...(state.search?.suggestions ?? [])].map(normalizeSearchSuggestion)
    },
    filters: [...(state.filters ?? [])].map(normalizeMenuItem),
    groupBys: [...(state.groupBys ?? [])].map(normalizeMenuItem),
    favorites: [...(state.favorites ?? [])].map(normalizeMenuItem)
  };
}

export function renderControlPanel(state: ControlPanelState, callbacks: ControlPanelCallbacks = {}): HTMLElement {
  const normalized = createControlPanelState(state);
  const root = document.createElement("section");
  root.className = "o_control_panel d-flex flex-column gap-3 px-3 pt-2 pb-3";

  const main = document.createElement("div");
  main.className = "o_control_panel_main d-flex flex-wrap flex-lg-nowrap justify-content-between align-items-lg-start gap-2 gap-lg-3 flex-grow-1";

  const breadcrumbs = document.createElement("div");
  breadcrumbs.className = "o_control_panel_breadcrumbs d-flex align-items-center gap-1 order-0 h-lg-100";
  breadcrumbs.append(renderMainButtons(), renderBreadcrumbs(normalized, callbacks));

  const actions = document.createElement("div");
  actions.className = "o_control_panel_actions d-empty-none d-flex align-items-center justify-content-start justify-content-lg-around order-2 order-lg-1 w-100 mw-100 w-lg-auto";
  actions.append(renderSearch(normalized, callbacks));

  const navigation = document.createElement("div");
  navigation.className = "o_control_panel_navigation d-flex flex-wrap flex-md-nowrap justify-content-end gap-1 gap-xl-3 order-1 order-lg-2 flex-grow-1";
  navigation.append(renderPager(normalized.pager, callbacks), renderViewSwitcher(normalized.views ?? [], callbacks));

  main.append(breadcrumbs, actions, navigation);
  root.append(main);
  return root;
}

function renderMainButtons(): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_control_panel_main_buttons d-flex gap-1 d-empty-none d-print-none";
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
  root.className = "o_cp_pager o_pager text-nowrap";
  if (!pager) return root;
  const first = pager.total === 0 ? 0 : pager.offset + 1;
  const last = Math.min(pager.total, pager.offset + pager.limit);
  const counter = document.createElement("span");
  counter.className = "o_pager_counter";
  const value = document.createElement("span");
  value.className = "o_pager_value";
  value.textContent = `${first}-${last}`;
  const limit = document.createElement("span");
  limit.className = "o_pager_limit";
  limit.textContent = String(pager.total);
  counter.append(value, document.createTextNode(" / "), limit);
  const previous = pagerButton("previous", "Previous", pager.offset <= 0, callbacks.onPagerPrevious);
  const next = pagerButton("next", "Next", last >= pager.total, callbacks.onPagerNext);
  root.append(counter, previous, next);
  return root;
}

function renderSearch(state: ControlPanelState, callbacks: ControlPanelCallbacks): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_cp_searchview d-flex input-group";
  root.setAttribute("role", "search");
  const searchView = document.createElement("div");
  searchView.className = "o_searchview form-control d-flex align-items-center py-1 border-end-0";
  searchView.setAttribute("role", "search");
  searchView.setAttribute("aria-autocomplete", "list");
  const searchButton = document.createElement("button");
  searchButton.type = "button";
  searchButton.className = "d-print-none btn border-0 p-0";
  searchButton.setAttribute("aria-label", "Search...");
  searchButton.setAttribute("title", "Search...");
  const icon = document.createElement("i");
  icon.className = "o_searchview_icon oi oi-search me-2";
  icon.setAttribute("role", "img");
  searchButton.append(icon);
  const inputContainer = document.createElement("div");
  inputContainer.className = "o_searchview_input_container d-flex flex-grow-1 flex-wrap gap-1 mw-100";
  for (const facet of state.search?.facets ?? []) {
    inputContainer.append(renderSearchFacet(facet, callbacks));
  }
  const input = document.createElement("input");
  input.className = "o_searchview_input o_input d-print-none flex-grow-1 w-auto border-0";
  input.type = "text";
  input.value = state.search?.query ?? "";
  input.placeholder = state.search?.placeholder ?? "Search...";
  input.setAttribute("role", "searchbox");
  input.addEventListener("input", () => callbacks.onSearch?.(input.value));
  inputContainer.append(input);
  searchView.append(searchButton, inputContainer);
  const autocomplete = renderSearchAutocomplete(state.search, callbacks);
  const dropdown = document.createElement("button");
  dropdown.type = "button";
  dropdown.className = "o_searchview_dropdown_toggler d-print-none btn btn-outline-secondary o-dropdown-caret rounded-start-0";
  dropdown.setAttribute("aria-label", "Search options");
  dropdown.setAttribute("aria-expanded", "false");
  const menu = document.createElement("div");
  const menuClassName = "o_search_options o_search_bar_menu o-dropdown--menu dropdown-menu";
  menu.className = menuClassName;
  menu.hidden = true;
  let menuOpen = false;
  dropdown.addEventListener("click", () => {
    menuOpen = !menuOpen;
    dropdown.setAttribute("aria-expanded", menuOpen ? "true" : "false");
    menu.hidden = !menuOpen;
    menu.className = menuOpen ? `${menuClassName} show` : menuClassName;
  });
  menu.append(
    renderMenuLane("o_filter_menu", "Filters", state.filters ?? [], callbacks.onFilter, { customFilter: callbacks.onAddCustomFilter }),
    renderMenuLane("o_group_by_menu", "Group By", state.groupBys ?? [], callbacks.onGroupBy, { customGroup: callbacks.onAddCustomGroup }),
    renderMenuLane("o_favorite_menu", "Favorites", state.favorites ?? [], callbacks.onFavorite, {
      favorite: true,
      addFavorite: callbacks.onAddFavorite,
      deleteFavorite: callbacks.onDeleteFavorite
    })
  );
  root.append(searchView, autocomplete, dropdown, menu);
  return root;
}

function renderSearchAutocomplete(search: ControlPanelSearchState | undefined, callbacks: ControlPanelCallbacks): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_searchview_autocomplete o-dropdown--menu dropdown-menu";
  root.setAttribute("role", "listbox");
  root.hidden = true;
  const query = String(search?.query ?? "").trim();
  const suggestions = [...(search?.suggestions ?? [])].filter((item) => item.field && item.value);
  if (!query || !suggestions.length) return root;
  root.hidden = false;
  root.className = `${root.className} show`;
  for (const suggestion of suggestions) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_searchview_autocomplete_item o-dropdown-item dropdown-item";
    button.setAttribute("role", "option");
    button.dataset.searchSuggestionId = suggestion.id;
    button.dataset.searchField = suggestion.field;
    button.textContent = suggestion.label;
    button.addEventListener("click", () => callbacks.onSearchSuggestion?.(suggestion));
    root.append(button);
  }
  if (callbacks.onAddCustomFilter) {
    const custom = document.createElement("button");
    custom.type = "button";
    custom.className = "o_searchview_autocomplete_item o_searchview_autocomplete_custom_filter o-dropdown-item dropdown-item";
    custom.setAttribute("role", "option");
    custom.dataset.searchSuggestionId = "custom-filter";
    custom.textContent = "Custom Filter...";
    custom.addEventListener("click", () => callbacks.onAddCustomFilter?.());
    root.append(custom);
  }
  return root;
}

function renderSearchFacet(facet: SearchFacet, callbacks: ControlPanelCallbacks): HTMLElement {
  const display = searchFacetDisplay(facet);
  const tag = document.createElement("span");
  tag.className = `o_searchview_facet o_searchview_facet_${facet.type} position-relative d-inline-flex align-items-stretch rounded-2 bg-200 text-nowrap`;
  tag.dataset.facetId = facet.id;
  const label = document.createElement("span");
  label.className = "o_searchview_facet_label";
  label.textContent = display.categoryLabel;
  const value = document.createElement("span");
  value.className = "o_facet_values";
  for (const [index, valueLabel] of display.valueLabels.entries()) {
    if (index > 0) {
      const separator = document.createElement("span");
      separator.className = "o_facet_values_sep o_facet_value_separator";
      separator.textContent = "or";
      value.append(separator);
    }
    const valueText = document.createElement("span");
    valueText.className = "o_facet_value";
    valueText.textContent = valueLabel;
    value.append(valueText);
  }
  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "o_facet_remove";
  remove.setAttribute("aria-label", `Remove ${display.valueLabels.join(" or ") || display.categoryLabel}`);
  remove.textContent = "x";
  remove.addEventListener("click", () => callbacks.onFacetRemove?.(facet));
  tag.append(label, value, remove);
  return tag;
}

function renderMenuLane(
  className: string,
  label: string,
  items: readonly ControlPanelMenuItem[],
  callback: ((item: ControlPanelMenuItem) => void) | undefined,
  options: {
    customFilter?: () => void;
    customGroup?: () => void;
    favorite?: boolean;
    addFavorite?: () => void;
    deleteFavorite?: (item: ControlPanelMenuItem) => void;
  } = {}
): HTMLElement {
  const root = document.createElement("div");
  root.className = `o_dropdown_container ${className}`;
  const title = document.createElement("h3");
  title.className = "o_dropdown_title";
  title.textContent = label;
  root.append(title);
  for (const item of items) {
    if (item.separatorBefore) root.append(dropdownDivider());
    if (item.children?.length) {
      const group = document.createElement("div");
      group.className = item.active ? "o_menu_item o_group_by_menu_item selected" : "o_menu_item o_group_by_menu_item";
      group.dataset.menuItemId = item.id;
      const groupLabel = document.createElement("span");
      groupLabel.className = "text-truncate";
      groupLabel.textContent = item.label;
      group.append(groupLabel);
      for (const child of item.children) {
        const option = document.createElement("button");
        option.type = "button";
        option.className = child.active ? "o_item_option o-dropdown-item dropdown-item selected" : "o_item_option o-dropdown-item dropdown-item";
        option.textContent = child.label;
        option.dataset.menuItemId = child.id;
        option.dataset.parentMenuItemId = item.id;
        option.setAttribute("role", "menuitemcheckbox");
        option.setAttribute("aria-checked", child.active ? "true" : "false");
        option.disabled = child.disabled === true;
        option.addEventListener("click", () => callback?.(child));
        group.append(option);
      }
      root.append(group);
      continue;
    }
    root.append(renderMenuItem(item, callback, options));
  }
  if (options.customFilter) {
    if (items.length) root.append(dropdownDivider());
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_menu_item o_add_custom_filter o-dropdown-item dropdown-item";
    button.textContent = "Custom Filter...";
    button.addEventListener("click", () => options.customFilter?.());
    root.append(button);
  }
  if (options.customGroup) {
    if (items.length) root.append(dropdownDivider());
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_menu_item o_add_custom_group_menu o-dropdown-item dropdown-item";
    button.textContent = "Add Custom Group";
    button.addEventListener("click", () => options.customGroup?.());
    root.append(button);
  }
  if (options.favorite) {
    if (items.length) root.append(dropdownDivider());
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_menu_item o_add_favorite o-dropdown-item dropdown-item";
    button.textContent = "Save current search";
    button.addEventListener("click", () => options.addFavorite?.());
    root.append(button);
  }
  return root;
}

function renderMenuItem(
  item: ControlPanelMenuItem,
  callback: ((item: ControlPanelMenuItem) => void) | undefined,
  options: { favorite?: boolean; deleteFavorite?: (item: ControlPanelMenuItem) => void } = {}
): HTMLElement {
  const menuItem = document.createElement("button");
  menuItem.type = "button";
  const favoriteClass = options.favorite ? " o_favorite_item" : "";
  menuItem.className = item.active ? `o_menu_item o-dropdown-item dropdown-item${favoriteClass} selected` : `o_menu_item o-dropdown-item dropdown-item${favoriteClass}`;
  menuItem.textContent = item.label;
  menuItem.dataset.menuItemId = item.id;
  menuItem.setAttribute("role", "menuitemcheckbox");
  menuItem.setAttribute("aria-checked", item.active ? "true" : "false");
  menuItem.disabled = item.disabled === true;
  menuItem.addEventListener("click", () => callback?.(item));
  applyFavoriteDataset(menuItem, item);
  if (!options.favorite || !item.favorite) return menuItem;

  const row = document.createElement("div");
  row.className = item.active ? "o_favorite_row selected" : "o_favorite_row";
  applyFavoriteDataset(row, item);
  const meta = document.createElement("span");
  meta.className = "o_favorite_meta text-muted";
  meta.textContent = favoriteMetadataLabel(item.favorite);
  menuItem.append(meta);
  row.append(menuItem);
  if (item.favorite.canDelete && options.deleteFavorite) {
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "o_favorite_delete o-dropdown-item dropdown-item";
    remove.dataset.favoriteId = String(item.favorite.id ?? "");
    remove.dataset.favoriteAction = "delete";
    remove.setAttribute("aria-label", `Delete favorite ${item.label}`);
    remove.textContent = "Delete";
    remove.addEventListener("click", () => options.deleteFavorite?.(item));
    row.append(remove);
  }
  return row;
}

function applyFavoriteDataset(element: HTMLElement, item: ControlPanelMenuItem): void {
  if (!item.favorite) return;
  if (item.favorite.id !== undefined) element.dataset.favoriteId = String(item.favorite.id);
  if (item.favorite.userId !== undefined) element.dataset.favoriteUserId = String(item.favorite.userId);
  if (item.favorite.actionId !== undefined) element.dataset.favoriteActionId = String(item.favorite.actionId);
  if (item.favorite.embeddedActionId !== undefined) element.dataset.favoriteEmbeddedActionId = String(item.favorite.embeddedActionId);
  element.dataset.favoriteDefault = item.favorite.isDefault ? "true" : "false";
  element.dataset.favoriteScope = item.favorite.isGlobal ? "global" : "user";
}

function favoriteMetadataLabel(favorite: ControlPanelFavoriteMetadata): string {
  const parts: string[] = [];
  parts.push(favorite.isGlobal ? "Global" : "Personal");
  if (favorite.isDefault) parts.push("Default");
  return parts.join(" / ");
}

function dropdownDivider(): HTMLElement {
  const divider = document.createElement("div");
  divider.className = "dropdown-divider";
  divider.setAttribute("role", "separator");
  return divider;
}

function renderViewSwitcher(views: readonly ControlPanelView[], callbacks: ControlPanelCallbacks): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_cp_switch_buttons d-print-none d-inline-flex btn-group";
  for (const view of views) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = view.active ? `btn btn-secondary o_switch_view o_${view.type} active` : `btn btn-secondary o_switch_view o_${view.type}`;
    button.setAttribute("aria-label", view.label || view.type);
    button.setAttribute("title", view.label || view.type);
    button.dataset.viewType = view.type;
    const icon = document.createElement("i");
    icon.className = viewIconClass(view.type);
    icon.setAttribute("aria-hidden", "true");
    icon.textContent = viewIconText(view.type);
    button.append(icon);
    button.addEventListener("click", () => callbacks.onViewSwitch?.(view.type));
    root.append(button);
  }
  return root;
}

function viewIconClass(viewType: string): string {
  if (viewType === "list") return "oi oi-view-list";
  if (viewType === "kanban") return "oi oi-view-kanban";
  if (viewType === "form") return "oi oi-view-form";
  if (viewType === "calendar") return "oi oi-calendar";
  if (viewType === "pivot") return "oi oi-view-pivot";
  if (viewType === "graph") return "oi oi-view-graph";
  return "oi oi-view";
}

function viewIconText(viewType: string): string {
  if (viewType === "list") return "L";
  if (viewType === "kanban") return "K";
  if (viewType === "form") return "F";
  if (viewType === "calendar") return "C";
  if (viewType === "pivot") return "P";
  if (viewType === "graph") return "G";
  return "";
}

function pagerButton(
  direction: string,
  label: string,
  disabled: boolean,
  callback: (() => void) | undefined
): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `btn btn-secondary o_pager_${direction}`;
  button.setAttribute("aria-label", label);
  button.setAttribute("title", label);
  const icon = document.createElement("i");
  icon.className = direction === "previous" ? "oi oi-chevron-left" : "oi oi-chevron-right";
  icon.setAttribute("aria-hidden", "true");
  icon.textContent = direction === "previous" ? "<" : ">";
  button.append(icon);
  button.disabled = disabled;
  button.addEventListener("click", () => {
    if (button.disabled) return;
    callback?.();
  });
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
    disabled: item.disabled === true,
    facet: item.facet ? { ...item.facet } : undefined,
    favorite: item.favorite ? { ...item.favorite } : undefined,
    children: item.children?.map(normalizeMenuItem),
    separatorBefore: item.separatorBefore === true
  };
}

function normalizeSearchSuggestion(suggestion: ControlPanelSearchSuggestion): ControlPanelSearchSuggestion {
  const field = String(suggestion.field ?? "").trim();
  const value = String(suggestion.value ?? "").trim();
  const operator = String(suggestion.operator ?? "ilike").trim() || "ilike";
  const label = String(suggestion.label ?? "").trim() || (field ? `${field}: ${value}` : value);
  return {
    id: String(suggestion.id || `${field}-${operator}-${value}`).trim(),
    label,
    field,
    value,
    operator,
    facet: suggestion.facet ? { ...suggestion.facet } : undefined
  };
}
