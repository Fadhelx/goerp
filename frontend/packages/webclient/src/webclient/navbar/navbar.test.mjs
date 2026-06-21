import assert from "node:assert/strict";
import {
  defaultSystrayItems,
  renderNavbar
} from "../../../../../dist/packages/webclient/src/webclient/navbar/navbar.js";

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

const navbar = renderNavbar({
  apps: [{ id: 7, name: "Sales" }],
  activeAppId: 7,
  userName: "Administrator",
  companyName: "My Company",
  debug: true
});

assert.deepEqual(defaultSystrayItems().map((item) => item.key), ["messages", "activities"]);
assert.match(navbar.className, /o_main_navbar/);
assert.equal(findAll(navbar, (node) => String(node.className).startsWith("o_menu_toggle ")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_menu_toggle_icon")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_navbar_apps_menu")).length, 1);
assert.equal(findAll(navbar, (node) => node.dataset?.menuId === "7" && node.attributes?.["aria-current"] === "page").length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_menu_systray")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_mail_systray_item")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_activity_menu")).length, 1);
assert.equal(findAll(navbar, (node) => node.tag === "i" && node.attributes?.["aria-label"] === "Messages").length, 1);
assert.equal(findAll(navbar, (node) => node.tag === "i" && node.attributes?.["aria-label"] === "Activities").length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o-systray-counter") && node.hidden === true).length, 2);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_switch_company_menu")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("oe_topbar_name")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_debug_manager")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_user_menu")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.textContent).includes("Gorp")).length, 0);
