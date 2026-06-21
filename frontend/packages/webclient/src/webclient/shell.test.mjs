import assert from "node:assert/strict";
import { createWebClientShell } from "../../../../dist/packages/webclient/src/webclient/shell.js";

globalThis.document = {
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
    1: { id: 1, name: "Settings", children: [] },
    2: { id: 2, name: "Server Actions", children: [] }
  }
});

assert.match(shell.className, /o_web_client/);
assert.equal(shell.dataset.theme, "enterprise-like");
assert.equal(shell.dataset.mobileSafe, "true");
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_manager")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_app_name")).length, 3);
assert.equal(findAll(shell, (node) => node.dataset?.menuId === "1").length, 2);
assert.equal(findAll(shell, (node) => String(node.textContent).includes("Gorp")).length, 0);
