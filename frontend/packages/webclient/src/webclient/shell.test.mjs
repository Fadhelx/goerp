import assert from "node:assert/strict";
import { createWebClientShell } from "../../../../dist/packages/webclient/src/webclient/shell.js";

const bodyClasses = new Set();

globalThis.document = {
  body: {
    dataset: {},
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
assert.equal(document.body.dataset.view, "apps");
assert.equal(document.body.dataset.homeMenuMode, "root");
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
assert.equal(document.body.dataset.view, "action");
assert.equal(document.body.dataset.homeMenuMode, undefined);
assert.equal(String(shell.className).includes("o_home_menu_background"), false);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar"))[0].dataset.activeMenuId, "1");
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 0);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_marker") && node.dataset?.openedApp === "1").length, 1);
let launcherButton = findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0];
assert.equal(String(launcherButton.className).includes("o_menu_toggle_back"), false);
launcherButton.listeners.click[0]();
assert.equal(shell.dataset.view, "apps");
assert.equal(shell.dataset.homeMenuMode, "overlay");
assert.equal(document.body.dataset.view, "apps");
assert.equal(document.body.dataset.homeMenuMode, "overlay");
assert.match(shell.className, /o_home_menu_background/);
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_marker")).length, 0);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar"))[0].dataset.activeMenuId, "1");
launcherButton = findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0];
assert.equal(String(launcherButton.className).includes("o_menu_toggle_back"), true);
launcherButton.listeners.click[0]();
assert.equal(shell.dataset.view, "action");
assert.equal(shell.dataset.homeMenuMode, undefined);
assert.equal(document.body.dataset.view, "action");
assert.equal(document.body.dataset.homeMenuMode, undefined);
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
assert.deepEqual(findAll(catalogShell, (node) => String(node.className).split(/\s+/).includes("o_nav_entry")).map((node) => node.textContent), ["Apps"]);

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

const technicalOpened = [];
const technicalShell = createWebClientShell({
  theme: {
    name: "enterprise-like",
    color: {},
    typography: {},
    radius: {},
    spacing: {},
    density: "compact"
  },
  menus: {
    root: { children: [100] },
    100: { id: 100, name: "Settings", children: [101, 102] },
    101: { id: 101, name: "General Settings", actionID: 201, children: [] },
    102: { id: 102, name: "Technical", children: [110, 120, 130, 140, 150, 160, 170, 180, 190, 200, 205] },
    110: { id: 110, name: "Actions", children: [111, 112, 113, 114, 115, 116, 117, 118, 119] },
    111: { id: 111, name: "Actions", actionID: 211, children: [] },
    112: { id: 112, name: "Window Actions", actionID: 212, children: [] },
    113: { id: 113, name: "Server Actions", actionID: 213, children: [] },
    114: { id: 114, name: "Report Actions", actionID: 214, children: [] },
    115: { id: 115, name: "Client Actions", actionID: 215, children: [] },
    116: { id: 116, name: "URL Actions", actionID: 216, children: [] },
    117: { id: 117, name: "Embedded Actions", actionID: 217, children: [] },
    118: { id: 118, name: "User-defined Defaults", actionID: 218, children: [] },
    119: { id: 119, name: "Configuration Wizards", actionID: 219, children: [] },
    120: { id: 120, name: "Automation", children: [121, 122, 123] },
    121: { id: 121, name: "Scheduled Actions", actionID: 221, children: [] },
    122: { id: 122, name: "Automation Rules", actionID: 222, children: [] },
    123: { id: 123, name: "Scheduled Action Triggers", actionID: 223, children: [] },
    130: { id: 130, name: "User Interface", children: [131, 132, 133, 134] },
    131: { id: 131, name: "Views", actionID: 231, children: [] },
    132: { id: 132, name: "Menu Items", actionID: 232, children: [] },
    133: { id: 133, name: "Customized Views", actionID: 233, children: [] },
    134: { id: 134, name: "User-defined Filters", actionID: 234, children: [] },
    140: { id: 140, name: "Database Structure", children: [141, 142, 143, 144, 145, 146] },
    141: { id: 141, name: "Models", actionID: 241, children: [] },
    142: { id: 142, name: "Fields", actionID: 242, children: [] },
    143: { id: 143, name: "Selection Values", actionID: 243, children: [] },
    144: { id: 144, name: "Many-to-Many Relations", actionID: 244, children: [] },
    145: { id: 145, name: "Assets", actionID: 245, children: [] },
    146: { id: 146, name: "Decimal Accuracy", actionID: 246, children: [] },
    150: { id: 150, name: "Email", children: [151, 152] },
    151: { id: 151, name: "Scheduled Messages", actionID: 251, children: [] },
    152: { id: 152, name: "Outgoing Mail Servers", actionID: 252, children: [] },
    160: { id: 160, name: "Reporting", children: [161, 162] },
    161: { id: 161, name: "Paper Formats", actionID: 261, children: [] },
    162: { id: 162, name: "Reports", actionID: 262, children: [] },
    170: { id: 170, name: "Apps", actionID: 270, children: [] },
    180: { id: 180, name: "Parameters", children: [181] },
    181: { id: 181, name: "System Parameters", actionID: 281, children: [] },
    190: { id: 190, name: "Sequences & Identifiers", children: [191, 192] },
    191: { id: 191, name: "Sequences", actionID: 291, children: [] },
    192: { id: 192, name: "External Identifiers", actionID: 292, children: [] },
    200: { id: 200, name: "Security", children: [201, 202, 203] },
    201: { id: 201, name: "Access Rights", actionID: 301, children: [] },
    202: { id: 202, name: "Record Rules", actionID: 302, children: [] },
    203: { id: 203, name: "User Devices", actionID: 303, children: [] },
    205: { id: 205, name: "Users", actionID: 305, children: [] }
  },
  onOpenApp(app) {
    technicalOpened.push(app.id);
  }
});
assert.equal(technicalShell.setMenuContext(113), true);
const technicalButton = findAll(technicalShell, (node) => node.dataset?.menuId === "102" && String(node.className).includes("o_nav_dropdown_toggle"))[0];
technicalButton.listeners.click[0]({ stopPropagation() {} });
const technicalDropdown = findAll(technicalShell, (node) => node.dataset?.navbarDropdown === "102")[0];
const technicalLabels = [...technicalDropdown.children].map((node) => node.textContent).filter(Boolean);
assert.deepEqual(technicalLabels, [
  "Email",
  "Outgoing Mail Server",
  "Actions",
  "Actions",
  "Report",
  "Window Action",
  "Client Action",
  "Server Action",
  "Embedded Actions",
  "Configuration Wizards",
  "User-defined Defaults",
  "IAP",
  "IAP Accounts",
  "User Interface",
  "Menu Items",
  "Views",
  "Customized Views",
  "User-defined Filters",
  "Tours",
  "Database Structure",
  "Decimal Accuracy",
  "Assets",
  "Models",
  "Fields",
  "Fields Selection",
  "Model Constraints",
  "ManyToMany Relations",
  "Attachments",
  "Logging",
  "Profiling",
  "Automation",
  "Scheduled Actions",
  "Scheduled Actions Triggers",
  "Reporting",
  "Paper Format",
  "Report",
  "Sequences & Identifiers",
  "External Identifiers",
  "Sequences",
  "Parameters",
  "System Parameters",
  "Security",
  "Record Rules",
  "Access Rights",
  "User Devices"
]);
assert.equal(technicalLabels.includes("Tours"), true);
assert.equal(technicalLabels.includes("Fields Selection"), true);
assert.equal(technicalLabels.includes("ManyToMany Relations"), true);
assert.equal(technicalLabels.includes("Paper Format"), true);
assert.equal(technicalLabels.includes("Scheduled Messages"), false);
assert.equal(technicalLabels.includes("Automation Rules"), false);
assert.equal(technicalLabels.includes("Apps"), false);
assert.equal(technicalLabels.includes("Users"), false);
findAll(technicalDropdown, (node) => node.dataset?.menuId === "113")[0].listeners.click[0]();
assert.deepEqual(technicalOpened, [113]);
