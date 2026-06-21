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
      type: "",
      value: "",
      children: [],
      listeners: {},
      append(...nodes) {
        this.children.push(...nodes);
      },
      replaceChildren(...nodes) {
        this.children = nodes;
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
  menu_roots: [1, 2, 3, 4],
  1: { id: 1, name: "Delegation", children: [10] },
  2: { id: 2, name: "Delegation", actionID: 44, children: [] },
  3: { id: 3, name: "Sales Orders", children: [] },
  4: { id: 4, name: "Settings", children: [40] },
  10: { id: 10, name: "Requests" },
  40: { id: 40, name: "Technical", children: [41] },
  41: { id: 41, name: "Server Actions", actionID: 55, children: [] }
};

const apps = normalizeHomeMenuApps(payload);
assert.deepEqual(apps.map((item) => item.name), ["Delegation", "Sales Orders", "Settings"]);
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

const technicalMenu = renderHomeMenu(payload, { query: "server" });
assert.equal(findAll(technicalMenu, (node) => String(node.className).includes("o_app_search_input")).length, 1);
assert.equal(findAll(technicalMenu, (node) => node.dataset?.menuId === "41" && node.dataset?.menuAction === "true").length, 1);
const serverActionCard = findAll(technicalMenu, (node) => node.dataset?.menuId === "41")[0];
assert.equal(serverActionCard.dataset.rootMenuId, "4");
assert.equal(serverActionCard.dataset.menuPath, "Settings / Technical");
assert.equal(findAll(serverActionCard, (node) => String(node.className).includes("o_app_menu_path") && node.textContent === "Settings / Technical").length, 1);

const liveMenu = renderHomeMenu(payload);
assert.equal(findAll(liveMenu, (node) => node.dataset?.menuId === "41").length, 0);
const searchInput = findAll(liveMenu, (node) => String(node.className).includes("o_app_search_input"))[0];
searchInput.value = "server";
searchInput.listeners.input[0]();
assert.equal(findAll(liveMenu, (node) => node.dataset?.menuId === "41" && node.dataset?.menuAction === "true").length, 1);
