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
assert.equal(findAll(navbar, (node) => String(node.className).split(/\s+/).includes("o_switch_company_menu")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("oe_topbar_name")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_debug_manager")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("o_user_menu")).length, 1);
assert.equal(findAll(navbar, (node) => String(node.className).includes("dropdown-menu")).length, 5);
assert.equal(findAll(navbar, (node) => String(node.className).includes("dropdown-menu") && node.hidden === true).length, 5);
assert.equal(findAll(navbar, (node) => String(node.textContent).includes("Gorp")).length, 0);
const systray = findAll(navbar, (node) => String(node.className).includes("o_menu_systray"))[0];
assert.match(String(systray.children[0].className), /o_debug_manager/);
assert.match(String(systray.children[2].className), /o_mail_systray_item/);
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

const systrayActions = [];
const liveNavbar = renderNavbar({
  userName: "Admin",
  companyName: "Beta",
  systray: {
    store: {
      inbox: { counter: 2 },
      starred: { counter: 5 },
      activityCounter: 3,
      activityGroups: [
        { name: "Partners", model: "res.partner", overdue_count: 1, today_count: 2, planned_count: 0, total_count: 3, view_type: "list" }
      ]
    },
    companies: [
      { id: 1, name: "Alpha" },
      { id: 2, name: "Beta", current: true, active: true }
    ],
    currentCompanyId: 2,
    displaySwitchCompanyMenu: true
  },
  onSystrayAction: (action) => systrayActions.push(action)
});
assert.deepEqual(defaultSystrayItems({
  inbox: { counter: 2 },
  starred: { counter: 5 },
  activityCounter: 3,
  activityGroups: [{ name: "Partners", total_count: 3 }]
}).map((item) => item.count), [2, 3]);
assert.equal(findAll(liveNavbar, (node) => String(node.className).includes("o-systray-counter") && node.hidden === false && node.textContent === "2").length, 1);
assert.equal(findAll(liveNavbar, (node) => String(node.className).includes("o-systray-counter") && node.hidden === false && node.textContent === "3").length, 1);
assert.equal(findAll(liveNavbar, (node) => node.dataset?.systrayItem === "Partners").length, 1);
assert.equal(findAll(liveNavbar, (node) => node.dataset?.systrayItem === "Beta" && String(node.className).includes("active")).length, 1);
const companyMenu = findAll(liveNavbar, (node) => String(node.className).includes("o_switch_company_menu_dropdown"))[0];
assert.equal(companyMenu.dataset.systrayDropdown, "company");
const companyItems = findAll(liveNavbar, (node) => String(node.className).split(/\s+/).includes("o_switch_company_item") && node.dataset?.companyId);
assert.deepEqual(companyItems.map((node) => node.dataset.companyId), ["1", "2"]);
const alphaCompany = companyItems[0];
const betaCompany = companyItems[1];
assert.equal(betaCompany.attributes["aria-checked"], "true");
assert.equal(betaCompany.attributes["aria-pressed"], "true");
assert.equal(alphaCompany.attributes["aria-checked"], "false");
assert.equal(alphaCompany.attributes["aria-pressed"], "false");
assert.equal(findAll(liveNavbar, (node) => String(node.className).includes("o_switch_company_menu_buttons")).length, 1);
alphaCompany.listeners.click[0]();
assert.equal(alphaCompany.attributes["aria-checked"], "true");
assert.equal(betaCompany.attributes["aria-checked"], "true");
assert.equal(systrayActions.length, 0);
const resetCompany = findAll(liveNavbar, (node) => String(node.className).includes("o_switch_company_reset"))[0];
resetCompany.listeners.click[0]();
assert.equal(alphaCompany.attributes["aria-checked"], "false");
assert.equal(betaCompany.attributes["aria-checked"], "true");
alphaCompany.listeners.click[0]();
const confirmCompany = findAll(liveNavbar, (node) => String(node.className).includes("o_switch_company_confirm"))[0];
confirmCompany.listeners.click[0]();
assert.deepEqual(systrayActions.at(-1), { type: "switch-company", companyId: 2, companyIds: [2, 1] });
const logIntoBeta = findAll(betaCompany, (node) => String(node.className).includes("log_into"))[0];
logIntoBeta.listeners.click[0]({ stopPropagation() {} });
assert.deepEqual(systrayActions.at(-1), { type: "switch-company", companyId: 2, companyIds: [2, 1] });
const logIntoAlpha = findAll(alphaCompany, (node) => String(node.className).includes("log_into"))[0];
logIntoAlpha.listeners.click[0]({ stopPropagation() {} });
assert.deepEqual(systrayActions.at(-1), { type: "switch-company", companyId: 1, companyIds: [1, 2] });
const starredItem = findAll(liveNavbar, (node) => node.dataset?.systrayItem === "Starred")[0];
starredItem.listeners.click[0]();
assert.deepEqual(systrayActions.at(-1), { type: "open-mailbox", mailbox: "starred" });
const activityItem = findAll(liveNavbar, (node) => node.dataset?.systrayItem === "Partners")[0];
activityItem.listeners.click[0]();
assert.equal(systrayActions.at(-1).type, "open-activities");
assert.equal(systrayActions.at(-1).model, "res.partner");

const searchableNavbar = renderNavbar({
  systray: {
    companies: Array.from({ length: 10 }, (_, index) => ({
      id: index + 1,
      name: `Company ${index + 1}`,
      current: index === 0
    })),
    currentCompanyId: 1,
    displaySwitchCompanyMenu: true
  }
});
const companySearch = findAll(searchableNavbar, (node) => node.tag === "input" && node.attributes?.role === "searchbox")[0];
assert.ok(companySearch);
companySearch.value = "company10";
companySearch.listeners.input[0]();
const searchableCompanies = findAll(searchableNavbar, (node) => String(node.className).split(/\s+/).includes("o_switch_company_item") && node.dataset?.companyId);
assert.deepEqual(searchableCompanies.filter((node) => node.hidden === false).map((node) => node.dataset.companyId), ["10"]);
