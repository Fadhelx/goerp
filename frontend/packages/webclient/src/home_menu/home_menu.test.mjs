import assert from "node:assert/strict";
import {
  appInitials,
  normalizeHomeMenuApps
} from "../../../../dist/packages/webclient/src/home_menu/app_metadata.js";
import { renderHomeMenu } from "../../../../dist/packages/webclient/src/home_menu/home_menu.js";

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
      }
    };
  }
};

function findAll(node, predicate, out = []) {
  if (predicate(node)) out.push(node);
  for (const child of node.children ?? []) findAll(child, predicate, out);
  return out;
}

const payload = {
  menu_roots: [1, 2, 3],
  1: { id: 1, name: "Delegation", children: [10] },
  2: { id: 2, name: "Delegation", actionID: 44, children: [] },
  3: { id: 3, name: "Sales Orders", children: [] },
  10: { id: 10, name: "Requests" }
};

const apps = normalizeHomeMenuApps(payload);
assert.deepEqual(apps.map((item) => item.name), ["Delegation", "Sales Orders"]);
assert.equal(apps[0].id, 2);
assert.equal(appInitials("Sales Orders"), "SO");

const homeMenu = renderHomeMenu(payload, { query: "sales" });
assert.match(homeMenu.className, /o_app_launcher/);
assert.equal(homeMenu.dataset.view, "apps");
assert.equal(homeMenu.dataset.mobileSafe, "true");
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("o_home_menu")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("o_apps")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("app-card")).length, 1);
assert.equal(findAll(homeMenu, (node) => node.dataset?.appName === "Sales Orders").length, 1);
assert.equal(findAll(homeMenu, (node) => node.dataset?.menuId === "3" && node.attributes?.["aria-label"] === "Sales Orders").length, 1);
assert.equal(findAll(homeMenu, (node) => node.tag === "img").length, 0);
