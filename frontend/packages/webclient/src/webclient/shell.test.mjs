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
  menus: {
    root: { children: [1, 2] },
    1: { id: 1, name: "Settings", children: [], actionID: 9 },
    2: { id: 2, name: "Server Actions", children: [] }
  },
  onOpenApp(app, outlet) {
    opened.push({ id: app.id, actionID: app.menu.actionID, outletClass: outlet.className });
  }
});

assert.match(shell.className, /o_web_client/);
assert.equal(shell.dataset.theme, "enterprise-like");
assert.equal(shell.dataset.mobileSafe, "true");
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_manager")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o-mobile-menu-toggle")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_app_name")).length, 3);
assert.equal(findAll(shell, (node) => node.dataset?.menuId === "1").length, 2);
assert.equal(findAll(shell, (node) => String(node.textContent).includes("Gorp")).length, 0);

findAll(shell, (node) => node.dataset?.menuId === "1" && String(node.className).includes("o_app"))[0].listeners.click[0]();
findAll(shell, (node) => node.dataset?.menuId === "1" && String(node.className).includes("o_nav_entry"))[0].listeners.click[0]();
assert.deepEqual(opened, [
  { id: 1, actionID: 9, outletClass: "o_action_manager" },
  { id: 1, actionID: 9, outletClass: "o_action_manager" }
]);

const mobileMenu = findAll(shell, (node) => String(node.className).includes("o-mobile-menu-toggle"))[0];
mobileMenu.listeners.click[0]();
assert.equal(bodyClasses.has("o-mobile-menu-open"), true);
findAll(shell, (node) => String(node.className).startsWith("o_menu_toggle "))[0].listeners.click[0]();
assert.equal(bodyClasses.has("o-mobile-menu-open"), false);
