import assert from "node:assert/strict";

const events = {};
const fetches = [];
let sessionResponse = { uid: 7, name: "Admin", company_name: "My Company" };

globalThis.location = { search: "", hash: "" };
globalThis.matchMedia = () => ({ matches: false });
globalThis.document = {
  documentElement: { dataset: {} },
  body: {
    classList: {
      values: new Set(),
      add(value) {
        this.values.add(value);
      },
      remove(value) {
        this.values.delete(value);
      },
      toggle(value, force) {
        if (force) this.values.add(value);
        else this.values.delete(value);
      },
      contains(value) {
        return this.values.has(value);
      }
    },
    replaceChildren(...nodes) {
      this.children = nodes;
    }
  },
  querySelector() {
    return null;
  },
  createElement(tag) {
    return {
      tag,
      id: "",
      className: "",
      dataset: {},
      attributes: {},
      children: [],
      disabled: false,
      hidden: false,
      type: "",
      value: "",
      replaceChildren(...nodes) {
        this.children = nodes;
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
        this.listeners = this.listeners || {};
        this.listeners[type] = [...(this.listeners[type] ?? []), listener];
      },
      dispatchEvent(event) {
        event.target ??= this;
        event.currentTarget = this;
        for (const listener of this.listeners?.[event.type] ?? []) listener.call(this, event);
        return !event.defaultPrevented;
      },
      remove() {
        this.removed = true;
      }
    };
  }
};
globalThis.CustomEvent = class TestCustomEvent {
  constructor(type, options = {}) {
    this.type = type;
    this.detail = options.detail;
  }
};
globalThis.addEventListener = (type, listener) => {
  events[type] = [...(events[type] ?? []), listener];
};
globalThis.dispatchEvent = (event) => {
  for (const listener of events[event.type] ?? []) listener(event);
  return true;
};
globalThis.fetch = async (route, options = {}) => {
  fetches.push({ route, options });
  if (route === "/web/session/get_session_info") {
    return { ok: true, status: 200, async json() { return sessionResponse; } };
  }
  if (route === "/web/session/authenticate") {
    sessionResponse = { uid: 7, name: "Admin", company_name: "My Company" };
    return { ok: true, status: 200, async json() { return sessionResponse; } };
  }
  if (route === "/mail/data") {
    return { ok: true, status: 200, async json() { return {
      Store: {
        inbox: { counter: 2 },
        starred: { counter: 1 },
        activityCounter: 1,
        activityGroups: [
          { name: "Partners", model: "res.partner", total_count: 1, overdue_count: 0, today_count: 1, planned_count: 0 }
        ]
      }
    }; } };
  }
  if (route === "/web/webclient/load_menus") {
    return { ok: true, status: 200, async json() { return {
      all_menu_ids: [1, 2],
      root: { children: [1, 2] },
      1: { id: 1, name: "Settings", children: [], actionID: 3 },
      2: { id: 2, name: "Server Actions", children: [] }
    }; } };
  }
  if (route === "/web/action/load") {
    const body = JSON.parse(options.body || "{}");
    if (body.action_id === 3) {
      return { ok: true, status: 200, async json() { return {
        id: 3,
        name: "Parent",
        res_model: "x.parent",
        type: "ir.actions.act_window",
        view_mode: "form",
        views: [[false, "form"]]
      }; } };
    }
    if (body.action_id === "base.open_wizard") {
      return { ok: true, status: 200, async json() { return {
        name: "Partner Wizard",
        res_model: "partner.wizard",
        target: "new",
        type: "ir.actions.act_window",
        view_mode: "form",
        views: [[false, "form"]]
      }; } };
    }
  }
  if (route === "/web/dataset/call_kw/x.parent/get_views") {
    return { ok: true, status: 200, async json() { return {
      fields: { name: { type: "char", string: "Name" } },
      related_models: {},
      views: {
        form: {
          arch: `<form><header><button name="base.open_wizard" type="action" string="Wizard"/></header><sheet><field name="name"/></sheet></form>`,
          id: 30
        }
      }
    }; } };
  }
  if (route === "/web/dataset/call_kw/x.parent/default_get") {
    return { ok: true, status: 200, async json() { return { id: 11, name: "Parent" }; } };
  }
  if (route === "/web/dataset/call_kw/x.parent/web_read") {
    return { ok: true, status: 200, async json() { return [{ id: 11, name: "Restored Parent" }]; } };
  }
  if (route === "/web/dataset/call_kw/partner.wizard/get_views") {
    return { ok: true, status: 200, async json() { return {
      fields: { name: { type: "char", string: "Name" } },
      related_models: {},
      views: {
        form: {
          arch: `<form><sheet><field name="name"/></sheet></form>`,
          id: 40
        }
      }
    }; } };
  }
  if (route === "/web/dataset/call_kw/partner.wizard/default_get") {
    return { ok: true, status: 200, async json() { return { name: "Wizard" }; } };
  }
  throw new Error(`unexpected fetch ${route}`);
};

function findAll(node, predicate, out = []) {
  if (predicate(node)) out.push(node);
  for (const child of node.children ?? []) findAll(child, predicate, out);
  return out;
}

const ready = new Promise((resolve) => {
  globalThis.addEventListener("goerp:webclient-ready", (event) => resolve(event.detail));
});

const mod = await import("../../../dist/apps/webclient/src/main.js");
const detail = await ready;

assert.equal(globalThis.document.documentElement.dataset.tsWebclient, "ready");
assert.equal(detail.session.uid, 7);
assert.deepEqual(detail.menus.all_menu_ids, [1, 2]);
assert.equal(typeof mod.bootstrapGoERPWebClient, "function");
assert.equal(typeof mod.renderAppsCatalogView, "function");
const moduleActions = [];
const catalog = mod.renderAppsCatalogView({
  modules: {
    crm: { name: "CRM", technical_name: "crm", state: "uninstalled", installable: true },
    calendar: { name: "Calendar", technical_name: "calendar", state: "to upgrade", installable: true },
    mail: { name: "Mail", technical_name: "mail", state: "installed", installable: true },
    project: { name: "Project", technical_name: "project", state: "to remove", installable: true }
  }
}, {
  onModuleAction: (technicalName, method, query) => moduleActions.push({ technicalName, method, query })
});
assert.equal(findAll(catalog, (node) => String(node.className).split(/\s+/).includes("gorp-apps-catalog")).length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "crm").length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "mail").length, 1);
const crmInstall = findAll(catalog, (node) => node.dataset?.moduleAction === "button_immediate_install" && node.disabled === false)[0];
crmInstall.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(moduleActions, [{ technicalName: "crm", method: "button_immediate_install", query: "" }]);
assert.equal(crmInstall.textContent, "Installing");
assert.equal(findAll(catalog, (node) => node.dataset?.moduleAction === "button_immediate_upgrade" && node.disabled === false).length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleAction === "button_immediate_uninstall" && node.disabled === false).length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleAction === "button_cancel_upgrade" && node.disabled === false).length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleAction === "button_cancel_uninstall" && node.disabled === false).length, 1);
const catalogSearch = findAll(catalog, (node) => String(node.className).includes("o_searchview_input"))[0];
catalogSearch.value = "crm";
catalogSearch.dispatchEvent(new CustomEvent("input"));
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "crm").length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "mail").length, 0);
const filteredCrmInstall = findAll(catalog, (node) => node.dataset?.moduleAction === "button_immediate_install" && node.disabled === false)[0];
filteredCrmInstall.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(moduleActions.at(-1), { technicalName: "crm", method: "button_immediate_install", query: "crm" });
let shell = globalThis.document.body.children[0].children[0];
assert.match(shell.className, /o_web_client/);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_manager")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o-mobile-menu-toggle")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_app_name")).length, 2);
assert.equal(findAll(shell, (node) => String(node.className).includes("o-systray-counter") && node.hidden === false && node.textContent === "2").length, 1);
assert.equal(findAll(shell, (node) => node.dataset?.systrayItem === "Partners").length, 1);
assert.deepEqual(fetches.map((item) => [item.route, item.options.method]), [
  ["/web/session/get_session_info", "GET"],
  ["/mail/data", "POST"],
  ["/web/webclient/load_menus", "GET"]
]);

findAll(shell, (node) => node.dataset?.menuId === "1" && String(node.className).includes("o_app"))[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const actionManager = findAll(shell, (node) => String(node.className).includes("o_action_manager"))[0];
assert.equal(actionManager.dataset.tsActionStatus, "ready");
assert.equal(findAll(actionManager, (node) => String(node.className).includes("gorp-window-action")).length, 1);
findAll(actionManager, (node) => node.dataset?.workflowAction === "base.open_wizard")[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionManager.dataset.tsDialogStatus, "ready");
assert.equal(findAll(actionManager, (node) => String(node.className).split(/\s+/).includes("gorp-action-dialog")).length, 1);
assert.equal(findAll(actionManager, (node) => String(node.className).split(/\s+/).includes("gorp-action-dialog-backdrop")).length, 1);
assert.equal(globalThis.document.body.classList.contains("modal-open"), true);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("modal-title"))[0].textContent, "Partner Wizard");
assert.equal(findAll(actionManager, (node) => String(node.className).split(/\s+/).includes("gorp-action-dialog") && node.dataset?.model === "partner.wizard").length, 1);
findAll(actionManager, (node) => String(node.className).includes("btn-close"))[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionManager.dataset.tsDialogStatus, "closed");
assert.equal(globalThis.document.body.classList.contains("modal-open"), false);

fetches.length = 0;
sessionResponse = { uid: 7, name: "Admin", company_name: "My Company" };
globalThis.location.search = "";
globalThis.location.hash = "#action=3&model=x.parent&view_type=form&id=11&menu_id=1";
globalThis.document.body.children = [];
await mod.bootstrapGoERPWebClient();
shell = globalThis.document.body.children[0].children[0];
const restoredActionManager = findAll(shell, (node) => String(node.className).includes("o_action_manager"))[0];
assert.equal(restoredActionManager.dataset.tsActionStatus, "ready");
assert.equal(findAll(restoredActionManager, (node) => String(node.className).includes("gorp-window-action") && node.dataset?.model === "x.parent" && node.dataset?.view === "form").length, 1);
const restoredActionLoad = fetches.find((item) => item.route === "/web/action/load");
assert.deepEqual(JSON.parse(restoredActionLoad.options.body).context, { menu_id: 1, active_id: 11 });
assert.equal(fetches.some((item) => item.route === "/web/dataset/call_kw/x.parent/web_read"), true);

fetches.length = 0;
sessionResponse = { uid: 0, name: "User 0", company_name: "My Company", quick_login: true };
globalThis.location.search = "?legacy_webclient=1";
globalThis.location.hash = "";
globalThis.document.body.children = [];
await mod.bootstrapGoERPWebClient();
assert.equal(globalThis.document.body.children.length, 0);
assert.deepEqual(fetches.map((item) => [item.route, item.options.method]), [
  ["/web/session/get_session_info", "GET"],
  ["/web/session/authenticate", "POST"],
  ["/mail/data", "POST"],
  ["/web/webclient/load_menus", "GET"]
]);
