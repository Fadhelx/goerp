import assert from "node:assert/strict";
import {
  defaultSystrayItems,
  renderNavbar
} from "../../../../../dist/packages/webclient/src/webclient/navbar/navbar.js";

const documentEvents = {};
globalThis.document = {
  addEventListener(type, listener) {
    documentEvents[type] = [...(documentEvents[type] ?? []), listener];
  },
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
      hidden: false,
      children: [],
      listeners: {},
      contains() {
        return false;
      },
      append(...nodes) {
        this.children.push(...nodes);
      },
      setAttribute(name, value) {
        this.attributes[name] = String(value);
      },
      getAttribute(name) {
        return this.attributes[name] ?? null;
      },
      removeAttribute(name) {
        delete this.attributes[name];
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
assert.equal(findAll(navbar, (node) => String(node.className).includes("o-mobile-menu-toggle")).length, 1);
assert.equal(findAll(navbar, (node) => node.dataset?.menuId === "7" && node.attributes?.["aria-current"] === "page").length, 1);
assert.equal(navbar.dataset.activeMenuId, "7");
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_menu_brand") && node.textContent === "Sales").length, 1);
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
assert.equal(findAll(navbar, (node) => String(node.className).includes("dropdown-menu")).length, 5);
assert.equal(findAll(navbar, (node) => String(node.className).includes("dropdown-menu") && node.hidden === true).length, 5);
assert.equal(findAll(navbar, (node) => String(node.textContent).includes("Gorp")).length, 0);
const messageSystray = findAll(navbar, (node) => String(node.className).includes("o_mail_systray_item"))[0];
const messageMenu = findAll(navbar, (node) => node.dataset?.systrayDropdown === "messages")[0];
messageSystray.listeners.click[0]({ stopPropagation() {} });
assert.equal(messageSystray.attributes["aria-expanded"], "true");
assert.equal(messageMenu.hidden, false);
assert.match(messageMenu.className, /show/);
const activitySystray = findAll(navbar, (node) => String(node.className).includes("o_activity_menu"))[0];
activitySystray.listeners.click[0]({ stopPropagation() {} });
assert.equal(messageSystray.attributes["aria-expanded"], "false");
assert.equal(messageMenu.hidden, true);
assert.equal(activitySystray.attributes["aria-expanded"], "true");
documentEvents.keydown[0]({ key: "Escape" });
assert.equal(activitySystray.attributes["aria-expanded"], "false");
assert.equal(findAll(navbar, (node) => String(node.className).includes("dropdown-menu show")).length, 0);

const toggled = [];
const interactiveNavbar = renderNavbar({
  apps: [{ id: 7, name: "Sales" }],
  onToggleMobileMenu: (expanded) => toggled.push(expanded)
});
const mobileMenu = findAll(interactiveNavbar, (node) => String(node.className).includes("o-mobile-menu-toggle"))[0];
mobileMenu.listeners.click[0]();
assert.equal(mobileMenu.attributes["aria-expanded"], "true");
mobileMenu.listeners.click[0]();
assert.equal(mobileMenu.attributes["aria-expanded"], "false");
assert.deepEqual(toggled, [true, false]);

const activeNavbar = renderNavbar({
  apps: [{ id: 7, name: "Sales" }, { id: 8, name: "Settings" }]
});
findAll(activeNavbar, (node) => node.dataset?.menuId === "8")[0].listeners.click[0]();
assert.equal(activeNavbar.dataset.activeMenuId, "8");
assert.equal(findAll(activeNavbar, (node) => String(node.className).includes("o_menu_brand") && node.textContent === "Settings").length, 1);
findAll(activeNavbar, (node) => String(node.className).startsWith("o_menu_toggle "))[0].listeners.click[0]();
assert.equal(activeNavbar.dataset.activeMenuId, undefined);
assert.equal(findAll(activeNavbar, (node) => String(node.className).includes("o_menu_brand") && node.textContent === "Odoo").length, 1);
