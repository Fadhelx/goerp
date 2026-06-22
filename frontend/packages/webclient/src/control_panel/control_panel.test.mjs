import assert from "node:assert/strict";
import {
  createControlPanelState,
  renderControlPanel
} from "../../../../dist/packages/webclient/src/control_panel/control_panel.js";

class TestEvent {
  constructor(type) {
    this.type = type;
    this.defaultPrevented = false;
    this.target = null;
    this.currentTarget = null;
  }

  preventDefault() {
    this.defaultPrevented = true;
  }
}

globalThis.document = {
  createTextNode(text) {
    return { tag: "#text", textContent: text, children: [] };
  },
  createElement(tag) {
    return {
      tag,
      tagName: tag.toUpperCase(),
      className: "",
      dataset: {},
      attributes: {},
      textContent: "",
      value: "",
      placeholder: "",
      type: "",
      disabled: false,
      children: [],
      listeners: {},
      append(...nodes) {
        this.children.push(...nodes);
      },
      setAttribute(name, value) {
        this.attributes[name] = String(value);
      },
      addEventListener(type, listener) {
        this.listeners[type] = [...(this.listeners[type] ?? []), listener];
      },
      dispatchEvent(event) {
        event.target ??= this;
        event.currentTarget = this;
        for (const listener of this.listeners[event.type] ?? []) listener.call(this, event);
        return !event.defaultPrevented;
      }
    };
  }
};

function findAll(node, predicate, out = []) {
  if (predicate(node)) out.push(node);
  for (const child of node.children ?? []) findAll(child, predicate, out);
  return out;
}

function hasClass(node, name) {
  return String(node.className).split(/\s+/).includes(name);
}

const normalized = createControlPanelState({
  title: "Partners",
  breadcrumbs: [{ id: "root", label: "Contacts" }, { id: "current", label: "Partners" }],
  pager: { offset: 20, limit: 20, total: 45 },
  views: [{ type: "list", active: true }, { type: "form" }],
  search: {
    query: "azure",
    facets: [
      { id: "customers", type: "filter", label: "Customers" },
      {
        id: "stage",
        type: "filter",
        label: "Stage",
        categoryLabel: "Pipeline Stage",
        valueLabels: ["New", "Won"]
      }
    ],
    suggestions: [
      { id: "text-name-azure", label: "Search Name for: azure", field: "name", value: "azure" },
      { id: "text-email-azure", label: "Search Email for: azure", field: "email", value: "azure" }
    ]
  },
  filters: [{ id: "customers", label: "Customers", active: true }],
  groupBys: [
    {
      id: "create_date",
      label: "Creation Date",
      children: [
        { id: "create_date:month", label: "Month", active: true },
        { id: "create_date:week", label: "Week" }
      ]
    },
    { id: "salesperson", label: "Salesperson" }
  ],
  favorites: [{
    id: "mine",
    label: "My Search",
    favorite: { id: 7, userId: 7, isDefault: true, isGlobal: false, canDelete: true }
  }]
});
assert.equal(normalized.pager.offset, 20);
assert.equal(normalized.search.placeholder, "Search...");

const events = [];
const root = renderControlPanel(normalized, {
  onBreadcrumb: (breadcrumb) => events.push(["breadcrumb", breadcrumb.id]),
  onSearch: (query) => events.push(["search", query]),
  onViewSwitch: (viewType) => events.push(["view", viewType]),
  onPagerNext: () => events.push(["next"]),
  onGroupBy: (item) => events.push(["groupBy", item.id]),
  onFacetRemove: (facet) => events.push(["remove", facet.id]),
  onSearchSuggestion: (suggestion) => events.push(["suggestion", suggestion.id, suggestion.field, suggestion.value]),
  onDeleteFavorite: (item) => events.push(["deleteFavorite", item.favorite?.id]),
  onAddCustomFilter: () => events.push(["customFilter"]),
  onAddCustomGroup: () => events.push(["customGroup"]),
  onAddFavorite: () => events.push(["addFavorite"])
});

assert.ok(String(root.className).includes("o_control_panel"));
assert.equal(findAll(root, (node) => String(node.className).startsWith("o_control_panel_main ")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_control_panel_breadcrumbs")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_control_panel_actions")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_control_panel_navigation")).length, 1);
assert.equal(findAll(root, (node) => node.className === "breadcrumb-item").length, 1);
assert.equal(findAll(root, (node) => node.className === "breadcrumb-item active").length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_searchview_input_container")).length, 1);
const autocomplete = findAll(root, (node) => hasClass(node, "o_searchview_autocomplete"))[0];
assert.ok(autocomplete);
assert.equal(autocomplete.hidden, false);
assert.equal(hasClass(autocomplete, "show"), true);
assert.equal(autocomplete.attributes.role, "listbox");
assert.deepEqual(
  findAll(autocomplete, (node) => hasClass(node, "o_searchview_autocomplete_item")).map((node) => [node.textContent, node.dataset?.searchField]),
  [["Search Name for: azure", "name"], ["Search Email for: azure", "email"], ["Custom Filter...", undefined]]
);
assert.equal(findAll(autocomplete, (node) => String(node.className).includes("o_searchview_autocomplete_custom_filter")).length, 1);
const searchOptionsToggler = findAll(root, (node) => String(node.className).includes("o_searchview_dropdown_toggler"))[0];
const searchOptionsMenu = findAll(root, (node) => String(node.className).includes("o_search_bar_menu"))[0];
assert.ok(searchOptionsToggler);
assert.ok(searchOptionsMenu);
assert.equal(searchOptionsToggler.attributes["aria-expanded"], "false");
assert.equal(searchOptionsMenu.hidden, true);
assert.equal(hasClass(searchOptionsMenu, "o_search_options"), true);
assert.equal(hasClass(searchOptionsMenu, "o_search_bar_menu"), true);
assert.equal(hasClass(searchOptionsMenu, "show"), false);
searchOptionsToggler.dispatchEvent(new TestEvent("click"));
assert.equal(searchOptionsToggler.attributes["aria-expanded"], "true");
assert.equal(searchOptionsMenu.hidden, false);
assert.equal(hasClass(searchOptionsMenu, "show"), true);
searchOptionsToggler.dispatchEvent(new TestEvent("click"));
assert.equal(searchOptionsToggler.attributes["aria-expanded"], "false");
assert.equal(searchOptionsMenu.hidden, true);
assert.equal(hasClass(searchOptionsMenu, "show"), false);
assert.equal(findAll(root, (node) => String(node.className).includes("o_filter_menu")).length, 1);
assert.equal(findAll(root, (node) => hasClass(node, "o_group_by_menu")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_menu")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorites_menu")).length, 0);
assert.equal(findAll(root, (node) => String(node.className).includes("selected"))[0].attributes["aria-checked"], "true");
assert.equal(findAll(root, (node) => String(node.className).includes("o_add_custom_filter")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_add_custom_group_menu")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_add_favorite")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_group_by_menu_item")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_item")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_row")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_meta"))[0].textContent, "Personal / Default");
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_delete")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_item"))[0].dataset.favoriteId, "7");
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_item"))[0].dataset.favoriteScope, "user");
assert.ok(findAll(root, (node) => String(node.className).includes("dropdown-divider")).length >= 2);
assert.equal(findAll(root, (node) => String(node.className).includes("o_item_option")).length, 2);
assert.equal(findAll(root, (node) => node.dataset?.parentMenuItemId === "create_date").length, 2);
assert.equal(findAll(root, (node) => node.dataset?.menuItemId === "create_date:month")[0].attributes["aria-checked"], "true");
const facet = findAll(root, (node) => String(node.className).includes("o_searchview_facet_filter"))[0];
assert.ok(facet);
assert.equal(findAll(facet, (node) => String(node.className).includes("o_searchview_facet_label"))[0].textContent, "Filter");
assert.equal(findAll(facet, (node) => String(node.className) === "o_facet_value")[0].textContent, "Customers");
assert.equal(findAll(facet, (node) => String(node.className).includes("o_facet_remove")).length, 1);
const stageFacet = findAll(root, (node) => node.dataset?.facetId === "stage")[0];
assert.ok(stageFacet);
assert.equal(findAll(stageFacet, (node) => String(node.className).includes("o_searchview_facet_label"))[0].textContent, "Pipeline Stage");
assert.deepEqual(
  findAll(stageFacet, (node) => String(node.className) === "o_facet_value").map((node) => node.textContent),
  ["New", "Won"]
);
assert.equal(findAll(stageFacet, (node) => String(node.className).includes("o_facet_values_sep"))[0].textContent, "or");
assert.equal(findAll(root, (node) => String(node.className).includes("o_pager_value"))[0].textContent, "21-40");
assert.equal(findAll(root, (node) => node.className === "o_pager_limit")[0].textContent, "45");
assert.equal(findAll(root, (node) => String(node.className).includes("o_pager_previous"))[0].attributes["aria-label"], "Previous");
assert.equal(findAll(root, (node) => String(node.className).includes("o_pager_next"))[0].children[0].textContent, ">");
assert.equal(findAll(root, (node) => node.dataset?.viewType === "list")[0].attributes["aria-label"], "list");
assert.equal(findAll(root, (node) => node.dataset?.viewType === "form")[0].children[0].textContent, "F");

const input = findAll(root, (node) => String(node.className).includes("o_searchview_input") && node.attributes?.role === "searchbox")[0];
input.value = "beta";
input.dispatchEvent(new TestEvent("input"));

findAll(root, (node) => node.dataset?.viewType === "form")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => node.dataset?.breadcrumbId === "root")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_pager_next"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => node.dataset?.menuItemId === "create_date:week")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => node.dataset?.searchSuggestionId === "text-name-azure")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_facet_remove"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_add_custom_filter"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_add_custom_group_menu"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_favorite_delete"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_add_favorite"))[0].dispatchEvent(new TestEvent("click"));

assert.deepEqual(events, [
  ["search", "beta"],
  ["view", "form"],
  ["breadcrumb", "root"],
  ["next"],
  ["groupBy", "create_date:week"],
  ["suggestion", "text-name-azure", "name", "azure"],
  ["remove", "customers"],
  ["customFilter"],
  ["customGroup"],
  ["deleteFavorite", 7],
  ["addFavorite"]
]);

const emptyAutocompleteRoot = renderControlPanel({
  title: "Empty",
  search: {
    query: "",
    suggestions: [{ id: "text-name-empty", label: "Search Name for: Empty", field: "name", value: "Empty" }]
  }
});
const emptyAutocomplete = findAll(emptyAutocompleteRoot, (node) => String(node.className).includes("o_searchview_autocomplete"))[0];
assert.equal(emptyAutocomplete.hidden, true);
assert.equal(hasClass(emptyAutocomplete, "show"), false);
