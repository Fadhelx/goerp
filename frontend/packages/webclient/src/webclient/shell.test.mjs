import assert from "node:assert/strict";
import { createWebClientShell } from "../../../../dist/packages/webclient/src/webclient/shell.js";

const bodyClasses = new Set();

globalThis.document = {
  body: {
    classList: {
      toggle(name, force) {
        if (force) bodyClasses.add(name);
        else bodyClasses.delete(name);
      },
      contains(name) {
        return bodyClasses.has(name);
      }
    }
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
      replaceChildren(...nodes) {
        this.children = nodes;
      },
      setAttribute(name, value) {
        this.attributes[name] = String(value);
      },
      getAttribute(name) {
        return this.attributes[name] ?? null;
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

const opened = [];
const systrayActions = [];
const shell = createWebClientShell({
  theme: {
    name: "enterprise-like",
    color: {},
    typography: {},
    radius: {},
    spacing: {},
    density: "compact"
  },
  debug: false,
  userName: "Administrator",
  companyName: "My Company",
  systray: {
    store: {
      inbox: { counter: 4 },
      activityCounter: 1,
      activityGroups: [{ name: "Tasks", model: "project.task", total_count: 1, overdue_count: 0, today_count: 1, planned_count: 0 }]
    }
  },
  menus: {
    root: { children: [1, 2] },
    1: { id: 1, name: "Settings", children: [], actionID: 9 },
    2: { id: 2, name: "Server Actions", children: [] }
  },
  onOpenApp(app, outlet) {
    opened.push({ id: app.id, actionID: app.menu.actionID, outletClass: outlet.className });
    const marker = document.createElement("section");
    marker.className = "o_action_marker";
    marker.dataset.openedApp = String(app.id);
    outlet.replaceChildren(marker);
  },
  onSystrayAction(action, outlet) {
    systrayActions.push({ action, outletClass: outlet.className });
  }
});

assert.match(shell.className, /o_web_client/);
assert.match(shell.className, /o_home_menu_background/);
assert.equal(shell.dataset.theme, "enterprise-like");
assert.equal(shell.dataset.view, "apps");
assert.equal(shell.dataset.mobileSafe, "true");
assert.equal(String(shell.children[0].className).split(/\s+/).includes("o_navbar"), true);
assert.match(String(shell.children[0].children[0].className), /o_main_navbar/);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_navbar")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_manager")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o-mobile-menu-toggle")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_app_name")).length, 2);
assert.equal(findAll(shell, (node) => String(node.className).includes("o-systray-counter") && node.hidden === false && node.textContent === "4").length, 1);
assert.equal(findAll(shell, (node) => node.dataset?.systrayItem === "Tasks").length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_user_avatar") && node.textContent === "A").length, 1);
assert.equal(findAll(shell, (node) => node.dataset?.appKey === "apps").length, 0);
assert.equal(findAll(shell, (node) => node.dataset?.menuId === "1").length, 2);
assert.equal(findAll(shell, (node) => String(node.textContent).includes("Gorp")).length, 0);

findAll(shell, (node) => node.dataset?.menuId === "1" && String(node.className).includes("o_app"))[0].listeners.click[0]();
assert.equal(shell.dataset.view, "action");
assert.equal(String(shell.className).includes("o_home_menu_background"), false);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar"))[0].dataset.activeMenuId, "1");
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 0);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_marker") && node.dataset?.openedApp === "1").length, 1);
let launcherButton = findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0];
assert.equal(String(launcherButton.className).includes("o_menu_toggle_back"), false);
launcherButton.listeners.click[0]();
assert.equal(shell.dataset.view, "apps");
assert.equal(shell.dataset.homeMenuMode, "overlay");
assert.match(shell.className, /o_home_menu_background/);
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_marker")).length, 0);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar"))[0].dataset.activeMenuId, "1");
launcherButton = findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0];
assert.equal(String(launcherButton.className).includes("o_menu_toggle_back"), true);
launcherButton.listeners.click[0]();
assert.equal(shell.dataset.view, "action");
assert.equal(shell.dataset.homeMenuMode, undefined);
assert.equal(String(shell.className).includes("o_home_menu_background"), false);
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 0);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_marker") && node.dataset?.openedApp === "1").length, 1);
launcherButton = findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0];
assert.equal(String(launcherButton.className).includes("o_menu_toggle_back"), false);
findAll(shell, (node) => node.dataset?.menuId === "1" && String(node.className).includes("o_nav_entry"))[0].listeners.click[0]();
assert.deepEqual(opened, [
  { id: 1, actionID: 9, outletClass: "o_action_manager" },
  { id: 1, actionID: 9, outletClass: "o_action_manager" }
]);
findAll(shell, (node) => node.dataset?.systrayItem === "Tasks")[0].listeners.click[0]();
assert.equal(systrayActions.at(-1).action.type, "open-activities");
assert.equal(systrayActions.at(-1).outletClass, "o_action_manager");

const mobileMenu = findAll(shell, (node) => String(node.className).includes("o-mobile-menu-toggle"))[0];
mobileMenu.listeners.click[0]();
assert.equal(bodyClasses.has("o-mobile-menu-open"), true);
findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0].listeners.click[0]();
assert.equal(shell.dataset.view, "apps");
assert.equal(shell.dataset.homeMenuMode, "overlay");
assert.match(shell.className, /o_home_menu_background/);
assert.equal(bodyClasses.has("o-mobile-menu-open"), false);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar"))[0].dataset.activeMenuId, "1");
assert.equal(String(findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0].className).includes("o_menu_toggle_back"), true);
findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0].listeners.click[0]();
assert.equal(shell.dataset.view, "action");
assert.equal(shell.dataset.homeMenuMode, undefined);

const catalogOpened = [];
const catalogShell = createWebClientShell({
  theme: {
    name: "enterprise-like",
    color: {},
    typography: {},
    radius: {},
    spacing: {},
    density: "compact"
  },
  menus: {
    root: { children: [10] },
    10: { id: 10, name: "Settings", children: [11], actionID: 90 },
    11: { id: 11, name: "Technical", children: [12] },
    12: { id: 12, name: "Apps", actionID: 91, actionPath: "apps", xmlid: "base.menu_ir_module_module", children: [] }
  },
  onOpenApp(app, outlet) {
    catalogOpened.push({ id: app.id, rootId: app.rootId, actionID: app.menu.actionID, outletClass: outlet.className });
  }
});
findAll(catalogShell, (node) => node.dataset?.menuId === "12" && String(node.className).includes("o_app"))[0].listeners.click[0]();
assert.deepEqual(catalogOpened, [
  { id: 12, rootId: 10, actionID: 91, outletClass: "o_action_manager" }
]);
assert.equal(findAll(catalogShell, (node) => String(node.className).includes("o_menu_brand"))[0].textContent, "Apps");
assert.equal(findAll(catalogShell, (node) => String(node.className).includes("o_main_navbar"))[0].dataset.activeMenuId, "12");
assert.equal(findAll(catalogShell, (node) => node.dataset?.menuId === "12" && String(node.className).includes("o_nav_entry active")).length, 1);

const directContextOpened = [];
const directContextShell = createWebClientShell({
  theme: {
    name: "enterprise-like",
    color: {},
    typography: {},
    radius: {},
    spacing: {},
    density: "compact"
  },
  menus: {
    root: { children: [20] },
    20: { id: 20, name: "Settings", children: [21, 22, 23, 24] },
    21: { id: 21, name: "General Settings", actionID: 121, children: [] },
    22: { id: 22, name: "Users & Companies", children: [25] },
    23: { id: 23, name: "Translations", actionID: 123, children: [] },
    24: { id: 24, name: "Technical", children: [26] },
    25: { id: 25, name: "Users", actionID: 125, children: [] },
    26: { id: 26, name: "Server Actions", actionID: 126, children: [] }
  },
  onOpenApp(app) {
    directContextOpened.push(app.id);
  }
});
assert.equal(directContextShell.setMenuContext(26), true);
const directContextNavbar = findAll(directContextShell, (node) => String(node.className).includes("o_main_navbar"))[0];
assert.equal(directContextNavbar.dataset.activeMenuId, "24");
assert.equal(findAll(directContextShell, (node) => String(node.className).includes("o_menu_brand"))[0].textContent, "Settings");
assert.deepEqual(findAll(directContextShell, (node) => String(node.className).split(/\s+/).includes("o_nav_entry")).map((node) => node.textContent), [
  "General Settings",
  "Users & Companies",
  "Translations",
  "Technical"
]);
assert.equal(findAll(directContextShell, (node) => node.dataset?.menuId === "24" && String(node.className).includes("active")).length, 1);
assert.deepEqual(directContextOpened, []);
