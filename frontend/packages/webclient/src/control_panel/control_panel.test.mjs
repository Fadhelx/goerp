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
    facets: [{ id: "customers", type: "filter", label: "Customers" }]
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
  favorites: [{ id: "mine", label: "My Search" }]
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
  onAddCustomFilter: () => events.push(["customFilter"]),
  onAddCustomGroup: () => events.push(["customGroup"])
});

assert.ok(String(root.className).includes("o_control_panel"));
assert.equal(findAll(root, (node) => String(node.className).startsWith("o_control_panel_main ")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_control_panel_breadcrumbs")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_control_panel_actions")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_control_panel_navigation")).length, 1);
assert.equal(findAll(root, (node) => node.className === "breadcrumb-item").length, 1);
assert.equal(findAll(root, (node) => node.className === "breadcrumb-item active").length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_searchview_input_container")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_searchview_dropdown_toggler")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_search_bar_menu")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_filter_menu")).length, 1);
assert.equal(findAll(root, (node) => hasClass(node, "o_group_by_menu")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_menu")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorites_menu")).length, 0);
assert.equal(findAll(root, (node) => String(node.className).includes("selected"))[0].attributes["aria-checked"], "true");
assert.equal(findAll(root, (node) => String(node.className).includes("o_add_custom_filter")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_add_custom_group_menu")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_group_by_menu_item")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_favorite_item")).length, 1);
assert.ok(findAll(root, (node) => String(node.className).includes("dropdown-divider")).length >= 2);
assert.equal(findAll(root, (node) => String(node.className).includes("o_item_option")).length, 2);
assert.equal(findAll(root, (node) => node.dataset?.parentMenuItemId === "create_date").length, 2);
assert.equal(findAll(root, (node) => node.dataset?.menuItemId === "create_date:month")[0].attributes["aria-checked"], "true");
const facet = findAll(root, (node) => String(node.className).includes("o_searchview_facet_filter"))[0];
assert.ok(facet);
assert.equal(findAll(facet, (node) => String(node.className).includes("o_searchview_facet_label"))[0].textContent, "Filter");
assert.equal(findAll(facet, (node) => String(node.className) === "o_facet_value")[0].textContent, "Customers");
assert.equal(findAll(facet, (node) => String(node.className).includes("o_facet_remove")).length, 1);
assert.equal(findAll(root, (node) => String(node.className).includes("o_pager_value"))[0].textContent, "21-40");
assert.equal(findAll(root, (node) => node.className === "o_pager_limit")[0].textContent, "45");

const input = findAll(root, (node) => String(node.className).includes("o_searchview_input") && node.attributes?.role === "searchbox")[0];
input.value = "beta";
input.dispatchEvent(new TestEvent("input"));

findAll(root, (node) => node.dataset?.viewType === "form")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => node.dataset?.breadcrumbId === "root")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_pager_next"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => node.dataset?.menuItemId === "create_date:week")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_facet_remove"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_add_custom_filter"))[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_add_custom_group_menu"))[0].dispatchEvent(new TestEvent("click"));

assert.deepEqual(events, [
  ["search", "beta"],
  ["view", "form"],
  ["breadcrumb", "root"],
  ["next"],
  ["groupBy", "create_date:week"],
  ["remove", "customers"],
  ["customFilter"],
  ["customGroup"]
]);
