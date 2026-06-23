import assert from "node:assert/strict";
import {
  appInitials,
  homeMenuAppsCatalogApp,
  homeMenuEntry,
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
      hidden: false,
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
  40: { id: 40, name: "Technical", children: [41, 42] },
  41: { id: 41, name: "Server Actions", actionID: 55, children: [] },
  42: { id: 42, name: "Apps", actionID: 66, actionPath: "apps", xmlid: "base.menu_ir_module_module", children: [] }
};

const apps = normalizeHomeMenuApps(payload);
assert.deepEqual(apps.map((item) => item.name), ["Delegation", "Sales Orders", "Settings"]);
assert.equal(apps[0].id, 2);
assert.equal(appInitials("Sales Orders"), "SO");
assert.equal(homeMenuAppsCatalogApp(payload)?.id, 42);

const homeMenu = renderHomeMenu(payload, { query: "sales" });
assert.match(homeMenu.className, /o_app_launcher/);
assert.match(homeMenu.className, /o_home_menu_background/);
assert.equal(homeMenu.dataset.view, "apps");
assert.equal(homeMenu.dataset.mobileSafe, "true");
assert.equal(findAll(homeMenu, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("o_home_menu_search")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("o_home_menu_search") && node.dataset?.searchActive === "true").length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("o_home_menu_registration_banner")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("o_apps")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).split(/\s+/).includes("o_app")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).split(/\s+/).includes("o_draggable")).length, 1);
assert.equal(findAll(homeMenu, (node) => String(node.className).includes("app-card")).length, 0);
assert.equal(findAll(homeMenu, (node) => node.dataset?.appName === "Sales Orders").length, 1);
assert.equal(findAll(homeMenu, (node) => node.dataset?.menuId === "3" && node.attributes?.["aria-label"] === "Sales Orders").length, 1);
assert.equal(findAll(homeMenu, (node) => node.dataset?.menuId === "3" && node.attributes?.href === "#menu_id=3").length, 1);
assert.equal(findAll(homeMenu, (node) => node.tag === "img").length, 0);

const technicalMenu = renderHomeMenu(payload, { query: "server" });
assert.equal(findAll(technicalMenu, (node) => String(node.className).includes("o_app_search_input")).length, 1);
assert.equal(findAll(technicalMenu, (node) => node.dataset?.menuId === "41" && node.dataset?.menuAction === "true").length, 1);
const serverActionCard = findAll(technicalMenu, (node) => node.dataset?.menuId === "41")[0];
assert.equal(serverActionCard.dataset.rootMenuId, "4");
assert.equal(serverActionCard.dataset.menuPath, "Settings / Technical");
assert.equal(findAll(serverActionCard, (node) => String(node.className).includes("o_app_menu_path") && node.textContent === "Settings / Technical").length, 1);

const nestedPayload = {
  menu_roots: [1],
  root: { children: [1] },
  2: { id: 2, name: "Technical", children: [] },
  children: {
    1: { id: 1, name: "Settings", children: [2] },
    2: { id: 2, name: "Technical", children: [3] },
    3: { id: 3, name: "Server Actions", actionID: 7, children: [] }
  }
};
assert.equal(homeMenuEntry(nestedPayload, 2)?.name, "Technical");
assert.deepEqual(normalizeHomeMenuApps(nestedPayload, { includeDescendantActions: true }).map((item) => item.name), ["Settings", "Server Actions"]);

const liveMenu = renderHomeMenu(payload);
assert.equal(findAll(liveMenu, (node) => node.dataset?.menuId === "41").length, 0);
assert.deepEqual(findAll(liveMenu, (node) => String(node.className).split(/\s+/).includes("o_app_name")).map((node) => node.textContent), ["Apps", "Settings"]);
const liveShell = findAll(liveMenu, (node) => String(node.className).split(/\s+/).includes("o_home_menu"))[0];
const liveContainer = findAll(liveShell, (node) => String(node.className).split(/\s+/).includes("o_home_menu_container"))[0];
assert.deepEqual(liveContainer.children.map((node) => String(node.className).split(/\s+/)[0]), ["o_home_menu_registration_banner", "o-app-search", "o_apps"]);
assert.equal(findAll(liveMenu, (node) => String(node.className).includes("o_home_menu_registration_banner")).length, 1);
const registrationClose = findAll(liveMenu, (node) => String(node.className).includes("o_home_menu_registration_close"))[0];
const registrationBanner = findAll(liveMenu, (node) => String(node.className).includes("o_home_menu_registration_banner"))[0];
const registrationText = findAll(registrationBanner, (node) => String(node.className).includes("o_home_menu_registration_text"))[0];
assert.equal(registrationClose.textContent, "\u00d7");
assert.match(registrationText.textContent, /register your database once you have installed your first app/);
registrationClose.listeners.click[0]();
assert.equal(registrationBanner.hidden, true);
assert.equal(registrationBanner.dataset.dismissed, "true");
const liveCatalogCard = findAll(liveMenu, (node) => node.dataset?.menuId === "42")[0];
assert.equal(liveCatalogCard.dataset.appKey, "apps");
assert.equal(liveCatalogCard.dataset.menuXmlid, "base.menu_ir_module_module");
assert.equal(liveCatalogCard.dataset.menuPath, undefined);
assert.equal(findAll(liveCatalogCard, (node) => String(node.className).includes("o_app_menu_path")).length, 0);
const liveSettingsCard = findAll(liveMenu, (node) => node.dataset?.menuId === "4")[0];
assert.equal(liveSettingsCard.dataset.appKey, "settings");
const searchInput = findAll(liveMenu, (node) => String(node.className).includes("o_app_search_input"))[0];
const liveSearchWrap = findAll(liveMenu, (node) => String(node.className).includes("o_home_menu_search"))[0];
assert.equal(liveSearchWrap.dataset.searchActive, "false");
searchInput.value = "server";
searchInput.listeners.input[0]();
assert.equal(liveSearchWrap.dataset.searchActive, "true");
assert.equal(findAll(liveMenu, (node) => node.dataset?.menuId === "41" && node.dataset?.menuAction === "true").length, 1);

const keyboardMenu = renderHomeMenu(payload);
const keyboardSection = keyboardMenu;
const keyboardSearch = findAll(keyboardMenu, (node) => String(node.className).includes("o_app_search_input"))[0];
let prevented = false;
keyboardSection.listeners.keydown[0]({ key: "s", preventDefault() { prevented = true; } });
assert.equal(prevented, true);
assert.equal(keyboardSearch.value, "s");
assert.equal(findAll(keyboardMenu, (node) => String(node.className).includes("o_home_menu_search") && node.dataset?.searchActive === "true").length, 1);

const normalUserMenu = renderHomeMenu({
  menu_roots: [1],
  1: { id: 1, name: "Approvals", children: [] }
});
assert.equal(findAll(normalUserMenu, (node) => node.dataset?.appName === "Approvals").length, 1);
assert.equal(findAll(normalUserMenu, (node) => node.dataset?.appKey === "apps").length, 0);

const catalogEvents = [];
const installMenu = renderHomeMenu(payload, {
  query: "install",
  onOpenApp: (app) => catalogEvents.push(["app", app.id])
});
const catalogCard = findAll(installMenu, (node) => node.dataset?.menuId === "42" && node.dataset?.menuAction === "true")[0];
assert.equal(catalogCard.dataset.menuPath, "Settings / Technical");
catalogCard.listeners.click[0]();
assert.deepEqual(catalogEvents, [["app", 42]]);

const technicalSearch = renderHomeMenu(payload, { query: "developer" });
assert.equal(findAll(technicalSearch, (node) => node.dataset?.menuId === "41").length, 1);

const imageMenu = renderHomeMenu({
  root: { children: [1] },
  1: { id: 1, name: "Inventory", children: [], webIconData: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII=", webIconDataMimetype: "image/png" }
});
const imageIcon = findAll(imageMenu, (node) => node.tag === "img")[0];
assert.match(imageIcon.src, /^data:image\/png;base64,/);
const glyphMenu = renderHomeMenu({
  root: { children: [1] },
  1: { id: 1, name: "Technical", children: [], webIcon: "fa-cog,#ffffff,#714b67" }
});
const glyphIcon = findAll(glyphMenu, (node) => String(node.className).includes("o_app_icon_with_glyph"))[0];
assert.equal(glyphIcon.attributes.style, "background-color: #714b67; --app-icon-bg: #714b67; color: #ffffff;");
assert.equal(glyphIcon.dataset.webIcon, "fa fa-cog");
assert.equal(findAll(glyphIcon, (node) => String(node.className).includes("fa-cog")).length, 1);
const coreIconMenu = renderHomeMenu({
  root: { children: [1] },
  1: { id: 1, name: "Settings", children: [], webIcon: "fa-cog,#ffffff,#714b67" }
});
assert.equal(findAll(coreIconMenu, (node) => String(node.className).includes("o_app_icon_with_glyph")).length, 0);
assert.equal(findAll(coreIconMenu, (node) => String(node.className).includes("o_app_icon_fallback")).length, 1);
const defaultGlyphMenu = renderHomeMenu({
  root: { children: [1] },
  1: { id: 1, name: "Approvals", children: [] }
});
assert.equal(findAll(defaultGlyphMenu, (node) => String(node.className).includes("fa-check-square-o")).length, 1);
const fallbackMenu = renderHomeMenu({
  root: { children: [1] },
  1: { id: 1, name: "Broken Icon", children: [], webIconData: "abc123", webIconDataMimetype: "image/png" }
});
assert.equal(findAll(fallbackMenu, (node) => node.tag === "img").length, 0);
assert.equal(findAll(fallbackMenu, (node) => String(node.className).includes("o_app_icon"))[0].textContent, "");
