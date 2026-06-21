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

const normalized = createControlPanelState({
  title: "Partners",
  breadcrumbs: [{ id: "root", label: "Contacts" }, { id: "current", label: "Partners" }],
  pager: { offset: 20, limit: 20, total: 45 },
  views: [{ type: "list", active: true }, { type: "form" }],
  search: {
    query: "azure",
    facets: [{ id: "customers", type: "filter", label: "Customers" }]
  }
});
assert.equal(normalized.pager.offset, 20);
assert.equal(normalized.search.placeholder, "Search...");

const events = [];
const root = renderControlPanel(normalized, {
  onBreadcrumb: (breadcrumb) => events.push(["breadcrumb", breadcrumb.id]),
  onSearch: (query) => events.push(["search", query]),
  onViewSwitch: (viewType) => events.push(["view", viewType]),
  onPagerNext: () => events.push(["next"])
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
assert.equal(findAll(root, (node) => String(node.className).includes("o_searchview_facet_filter"))[0].textContent, "Customers");
assert.equal(findAll(root, (node) => String(node.className).includes("o_pager_value"))[0].textContent, "21-40");
assert.equal(findAll(root, (node) => node.className === "o_pager_limit")[0].textContent, "45");

const input = findAll(root, (node) => String(node.className).includes("o_searchview_input") && node.attributes?.role === "searchbox")[0];
input.value = "beta";
input.dispatchEvent(new TestEvent("input"));

findAll(root, (node) => node.dataset?.viewType === "form")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => node.dataset?.breadcrumbId === "root")[0].dispatchEvent(new TestEvent("click"));
findAll(root, (node) => String(node.className).includes("o_pager_next"))[0].dispatchEvent(new TestEvent("click"));

assert.deepEqual(events, [
  ["search", "beta"],
  ["view", "form"],
  ["breadcrumb", "root"],
  ["next"]
]);
