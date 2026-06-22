import assert from "node:assert/strict";

const events = {};
const fetches = [];
let sessionResponse = {
  uid: 7,
  name: "Admin",
  company_name: "My Company",
  display_switch_company_menu: true,
  user_companies: {
    current_company: 2,
    allowed_companies: {
      1: { id: 1, name: "Alpha" },
      2: { id: 2, name: "Beta" }
    }
  },
  user_context: { allowed_company_ids: [2, 1] }
};
let mailStoreResponse = {
  inbox: { counter: 2 },
  starred: { counter: 1 },
  activityCounter: 1,
  activityGroups: [
    { name: "Partners", model: "res.partner", total_count: 1, overdue_count: 0, today_count: 1, planned_count: 0, activity_ids: [41] }
  ]
};

const testLocation = { href: "/web", pathname: "/web", search: "", hash: "" };
const setTestURL = (url) => {
  testLocation.href = url;
  const hashIndex = url.indexOf("#");
  testLocation.hash = hashIndex >= 0 ? url.slice(hashIndex) : "";
};
globalThis.location = testLocation;
globalThis.window = {
  location: testLocation,
  requestAnimationFrame(callback) {
    return setTimeout(callback, 0);
  },
  cancelAnimationFrame(handle) {
    clearTimeout(handle);
  },
  history: {
    pushState(_state, _title, url) {
      setTestURL(String(url));
    },
    replaceState(_state, _title, url) {
      setTestURL(String(url));
    }
  }
};
globalThis.matchMedia = () => ({ matches: false });
globalThis.document = {
  implementation: {
    createDocument() {
      return {};
    }
  },
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
  createTextNode(text) {
    return { tag: "#text", textContent: String(text), children: [] };
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
        try {
          event.target ??= this;
          event.currentTarget = this;
        } catch {}
        for (const listener of this.listeners?.[event.type] ?? []) listener.call(this, event);
        return !event.defaultPrevented;
      },
      remove() {
        this.removed = true;
      }
    };
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
  if (route === "/web/session/switch_company") {
    return { ok: true, status: 200, async json() { return { ok: true }; } };
  }
  if (route === "/web/session/modules") {
    return { ok: true, status: 200, async json() { return {
      modules: {
        base: {
          application: true,
          category: "Technical",
          description: "Base module",
          name: "Base",
          state: "installed",
          summary: "Base module",
          technical_name: "base",
          website: ""
        }
      }
    }; } };
  }
  if (route === "/mail/data") {
    return { ok: true, status: 200, async json() { return {
      Store: mailStoreResponse
    }; } };
  }
  if (route === "/web/dataset/call_kw/mail.activity/activity_format") {
    return { ok: true, status: 200, async json() { return {
      "mail.activity": [
        { id: 41, display_name: "Call customer", res_id: 11, res_name: "Azure Interior", res_model: "res.partner", date_deadline: "2026-06-22", state: "today" }
      ]
    }; } };
  }
  if (route === "/web/dataset/call_kw/mail.activity/action_feedback") {
    mailStoreResponse = {
      inbox: { counter: 2 },
      starred: { counter: 1 },
      activityCounter: 0,
      activityGroups: []
    };
    return { ok: true, status: 200, async json() { return { done: true }; } };
  }
  if (route === "/web/webclient/load_menus") {
    return { ok: true, status: 200, async json() { return {
      all_menu_ids: [1, 2, 4],
      root: { children: [1, 2, 4] },
      1: { id: 1, name: "Settings", children: [2], actionID: 3 },
      2: { id: 2, name: "Server Actions", children: [], actionID: 7 },
      4: { id: 4, name: "Apps", children: [], actionID: 91, actionPath: "apps", xmlid: "base.menu_ir_module_module" }
    }; } };
  }
  if (route === "/web/action/load") {
    const body = JSON.parse(options.body || "{}");
    if (body.action_id === 91) {
      return { ok: true, status: 200, async json() { return {
        id: 91,
        name: "Apps",
        path: "apps",
        res_model: "ir.module.module",
        type: "ir.actions.act_window",
        view_mode: "list,form",
        views: [[false, "list"], [false, "form"]],
        context: { search_default_app: 1 }
      }; } };
    }
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
  if (route === "/web/dataset/call_kw/ir.module.module/get_views") {
    return { ok: true, status: 200, async json() { return {
      fields: {
        name: { type: "char", string: "Technical Name" },
        shortdesc: { type: "char", string: "Name" },
        state: { type: "selection", string: "Status" },
        summary: { type: "char", string: "Summary" },
        website: { type: "char", string: "Website" },
        application: { type: "boolean", string: "Application" }
      },
      related_models: {},
      views: {
        kanban: {
          arch: `<kanban create="false" can_open="0" class="o_modules_kanban"><field name="shortdesc"/><field name="name"/><field name="state"/><field name="summary"/><field name="website"/><field name="application"/></kanban>`,
          id: 91
        },
        list: {
          arch: `<list><field name="shortdesc"/><field name="name"/><field name="state"/></list>`,
          id: 92
        },
        form: {
          arch: `<form><sheet><field name="shortdesc"/><field name="name"/><field name="state"/><field name="summary"/></sheet></form>`,
          id: 93
        }
      }
    }; } };
  }
  if (route === "/web/dataset/call_kw/ir.module.module/web_search_read") {
    return { ok: true, status: 200, async json() { return {
      length: 1,
      records: [{ id: 5, name: "base", shortdesc: "Base", display_name: "Base", state: "installed", summary: "Base module", website: "", application: true }]
    }; } };
  }
  if (route === "/web/dataset/call_kw/ir.module.module/web_read") {
    return { ok: true, status: 200, async json() { return [
      { id: 5, name: "base", shortdesc: "Base", display_name: "Base", state: "installed", summary: "Base module" }
    ]; } };
  }
  if (route === "/web/dataset/call_kw/res.users/action_get") {
    return { ok: true, status: 200, async json() { return {
      name: "Change My Preferences",
      res_model: "res.users",
      target: "new",
      type: "ir.actions.act_window",
      view_mode: "form",
      views: [[false, "form"]]
    }; } };
  }
  if (route === "/web/dataset/call_kw/res.users/get_views") {
    return { ok: true, status: 200, async json() { return {
      fields: {
        name: { type: "char", string: "Name" },
        lang: { type: "char", string: "Language" },
        tz: { type: "char", string: "Timezone" }
      },
      related_models: {},
      views: {
        form: {
          arch: `<form><sheet><field name="name"/><field name="lang"/><field name="tz"/></sheet><footer><button name="preference_save" type="object" string="Update Preferences" class="btn-primary"/><button name="preference_cancel" string="Discard" special="cancel" class="btn-secondary"/></footer></form>`,
          id: 94
        }
      }
    }; } };
  }
  if (route === "/web/dataset/call_kw/res.users/web_read") {
    return { ok: true, status: 200, async json() { return [
      { id: 7, name: "Admin", display_name: "Admin", lang: "en_US", tz: "Asia/Bahrain" }
    ]; } };
  }
  if (route === "/web/dataset/call_kw") {
    const body = JSON.parse(options.body || "{}");
    if (body.model === "ir.module.module" && body.method === "search_read") {
      return { ok: true, status: 200, async json() { return [{ id: 5, name: "base", display_name: "Base" }]; } };
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
  if (route === "/web/dataset/call_kw/res.partner/get_views") {
    return { ok: true, status: 200, async json() { return {
      fields: { name: { type: "char", string: "Name" } },
      related_models: {},
      views: {
        form: {
          arch: `<form><sheet><field name="name"/></sheet></form>`,
          id: 50
        }
      }
    }; } };
  }
  if (route === "/web/dataset/call_kw/res.partner/web_read") {
    return { ok: true, status: 200, async json() { return [{ id: 11, name: "Azure Interior" }]; } };
  }
  throw new Error(`unexpected fetch ${route}`);
};

function findAll(node, predicate, out = []) {
  if (predicate(node)) out.push(node);
  for (const child of node.children ?? []) findAll(child, predicate, out);
  return out;
}

function allText(node) {
  return [node.textContent, ...(node.children ?? []).map(allText)].filter(Boolean).join(" ");
}

const ready = new Promise((resolve) => {
  globalThis.addEventListener("goerp:webclient-ready", (event) => resolve(event.detail));
});

const mod = await import("../../../dist/apps/webclient/src/main.js");
const detail = await ready;

assert.equal(globalThis.document.documentElement.dataset.tsWebclient, "ready");
assert.equal(detail.session.uid, 7);
assert.deepEqual(detail.menus.all_menu_ids, [1, 2, 4]);
assert.equal(typeof mod.bootstrapGoERPWebClient, "function");
assert.equal(typeof mod.renderAppsCatalogView, "function");
const moduleActions = [];
const catalog = mod.renderAppsCatalogView({
  modules: {
    crm: { name: "CRM", technical_name: "crm", state: "uninstalled", installable: true, category: "Sales", summary: "Pipeline and leads", depends: ["mail"] },
    calendar: { name: "Calendar", technical_name: "calendar", state: "to upgrade", installable: true, category: "Productivity", summary: "Meetings" },
    mail: { name: "Mail", technical_name: "mail", state: "installed", installable: true, category: "Productivity", description: "Discuss and messages", website: "https://example.test/mail" },
    project: { name: "Project", technical_name: "project", state: "to remove", installable: true, category: "Services" }
  }
}, {
  onModuleAction: (technicalName, method, query) => moduleActions.push({ technicalName, method, query })
});
assert.equal(findAll(catalog, (node) => String(node.className).split(/\s+/).includes("gorp-apps-catalog")).length, 1);
assert.equal(findAll(catalog, (node) => String(node.className).includes("gorp-apps-catalog-sidebar")).length, 1);
assert.deepEqual(findAll(catalog, (node) => node.dataset?.catalogFilter).map((node) => node.dataset.catalogFilter), ["all", "installed", "available", "updates"]);
assert.deepEqual(findAll(catalog, (node) => String(node.className).includes("o_search_panel_category")).map((node) => node.dataset.category), ["all", "Productivity", "Sales", "Services"]);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "crm").length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "mail").length, 1);
assert.equal(findAll(catalog, (node) => String(node.className).includes("o_app_icon") && !node.textContent).length, 4);
assert.equal(findAll(catalog, (node) => String(node.className).includes("o_app_summary") && node.textContent === "Pipeline and leads").length, 1);
findAll(catalog, (node) => node.dataset?.catalogFilter === "installed")[0].dispatchEvent(new CustomEvent("click"));
assert.equal(catalog.dataset.activeFilter, "installed");
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "mail").length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "crm").length, 0);
findAll(catalog, (node) => node.dataset?.catalogFilter === "all")[0].dispatchEvent(new CustomEvent("click"));
findAll(catalog, (node) => node.dataset?.category === "Sales")[0].dispatchEvent(new CustomEvent("click"));
assert.equal(catalog.dataset.activeCategory, "Sales");
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "crm").length, 1);
assert.equal(findAll(catalog, (node) => node.dataset?.moduleName === "mail").length, 0);
findAll(catalog, (node) => node.dataset?.moduleInfo === "crm")[0].dispatchEvent(new CustomEvent("click"));
const catalogDetail = findAll(catalog, (node) => String(node.className).includes("gorp-apps-catalog-detail"))[0];
assert.equal(catalogDetail.hidden, false);
assert.equal(catalogDetail.dataset.moduleName, "crm");
assert.match(allText(catalogDetail), /Pipeline and leads/);
assert.match(allText(catalogDetail), /mail/);
findAll(catalogDetail, (node) => String(node.className).includes("o_module_info_close"))[0].dispatchEvent(new CustomEvent("click"));
assert.equal(catalogDetail.hidden, true);
findAll(catalog, (node) => node.dataset?.category === "all")[0].dispatchEvent(new CustomEvent("click"));
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
assert.equal(findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o-mobile-menu-toggle")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_app_name")).length, 3);
assert.equal(findAll(shell, (node) => String(node.className).includes("o-systray-counter") && node.hidden === false && node.textContent === "2").length, 1);
assert.equal(findAll(shell, (node) => node.dataset?.systrayItem === "Partners").length, 1);
assert.deepEqual(fetches.map((item) => [item.route, item.options.method]), [
  ["/web/session/get_session_info", "GET"],
  ["/mail/data", "POST"],
  ["/web/webclient/load_menus", "GET"]
]);

const logIntoAlpha = findAll(shell, (node) => String(node.className).includes("log_into") && node.dataset?.companyId === "1")[0];
logIntoAlpha.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const switchFetch = fetches.find((item) => item.route === "/web/session/switch_company");
assert.equal(switchFetch.options.method, "POST");
assert.deepEqual(JSON.parse(switchFetch.options.body), { company_id: 1, company_ids: [1, 2] });
assert.equal(globalThis.location.href, "/web");

findAll(shell, (node) => node.dataset?.menuId === "1" && String(node.className).includes("o_app"))[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const actionManager = findAll(shell, (node) => String(node.className).includes("o_action_manager"))[0];
assert.equal(actionManager.dataset.tsActionStatus, "ready");
assert.equal(findAll(actionManager, (node) => String(node.className).includes("gorp-window-action") && String(node.className).includes("o_settings_view")).length, 1);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("o_settings_container")).length, 1);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("app_settings_block")).length, 4);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("o_setting_box")).length, 15);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "server_actions" && node.dataset?.settingsTargetModel === "ir.actions.server").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "users" && node.dataset?.settingsTargetModel === "res.users").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "groups" && node.dataset?.settingsTargetModel === "res.groups").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "access_rights" && node.dataset?.settingsTargetModel === "ir.model.access").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "record_rules" && node.dataset?.settingsTargetModel === "ir.rule").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "views" && node.dataset?.settingsTargetModel === "ir.ui.view").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "scheduled_actions" && node.dataset?.settingsTargetModel === "ir.cron").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "email_templates" && node.dataset?.settingsTargetModel === "mail.template").length, 1);
assert.equal(findAll(actionManager, (node) => node.dataset?.settingsTarget === "apps" && node.dataset?.settingsTargetModel === "ir.module.module").length, 1);
assert.match(allText(actionManager), /General Settings/);
assert.match(allText(actionManager), /Users & Companies/);
assert.match(allText(actionManager), /Technical/);
assert.match(allText(actionManager), /Apps & AI/);
assert.equal(globalThis.location.hash.includes("model=res.config.settings"), true);
assert.equal(globalThis.location.hash.includes("view_type=form"), true);

findAll(actionManager, (node) => node.dataset?.settingsTarget === "apps")[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionManager.dataset.tsActionStatus, "ready", allText(actionManager));
assert.equal(findAll(actionManager, (node) => String(node.className).split(/\s+/).includes("gorp-apps-catalog")).length, 1);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("o_apps_view")).length, 1);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("o_app_name")).length, 1);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("o_app_icon")).length, 1);
assert.equal(findAll(actionManager, (node) => String(node.className).includes("gorp-window-action") && node.dataset?.model === "ir.module.module" && node.dataset?.view === "kanban").length, 0);
assert.equal(globalThis.location.hash.includes("action=91"), true);
assert.equal(globalThis.location.hash.includes("model=ir.module.module"), true);
assert.equal(globalThis.location.hash.includes("view_type=kanban"), true);
assert.equal(globalThis.location.hash.includes("menu_id=4"), true);
assert.equal(globalThis.location.hash.includes("model=res.config.settings"), false);
const moduleInfo = findAll(actionManager, (node) => node.dataset?.moduleInfo === "base" && String(node.className).includes("o_module_info_button"))[0];
moduleInfo.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionManager.dataset.tsDialogStatus, "ready");
assert.equal(globalThis.document.body.classList.contains("modal-open"), true);
const moduleInfoDialog = findAll(actionManager, (node) => String(node.className).split(/\s+/).includes("gorp-action-dialog") && node.dataset?.model === "ir.module.module").at(-1);
assert.ok(moduleInfoDialog);
assert.equal(findAll(moduleInfoDialog, (node) => String(node.className).includes("modal o_dialog_container")).length, 1);
assert.equal(findAll(moduleInfoDialog, (node) => String(node.className).includes("modal-title"))[0].textContent, "Module Info");
findAll(moduleInfoDialog, (node) => String(node.className).includes("btn-close"))[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionManager.dataset.tsDialogStatus, "closed");
assert.equal(globalThis.document.body.classList.contains("modal-open"), false);

findAll(shell, (node) => String(node.className).split(/\s+/).includes("o_user_menu"))[0].dispatchEvent(new CustomEvent("click"));
findAll(shell, (node) => node.dataset?.systrayItem === "My Preferences")[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionManager.dataset.tsDialogStatus, "ready");
assert.equal(globalThis.document.body.classList.contains("modal-open"), true);
const preferencesDialog = findAll(actionManager, (node) => String(node.className).split(/\s+/).includes("gorp-action-dialog") && node.dataset?.model === "res.users").at(-1);
assert.ok(preferencesDialog);
assert.equal(findAll(preferencesDialog, (node) => String(node.className).includes("modal o_dialog_container")).length, 1);
assert.equal(findAll(preferencesDialog, (node) => String(node.className).includes("modal-title"))[0].textContent, "Change My Preferences");
assert.equal(findAll(preferencesDialog, (node) => String(node.className).includes("modal-footer")).length, 1);
findAll(preferencesDialog, (node) => String(node.className).includes("btn-close"))[0].dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionManager.dataset.tsDialogStatus, "closed");
assert.equal(globalThis.document.body.classList.contains("modal-open"), false);

const activityMenuItem = findAll(shell, (node) => node.dataset?.systrayItem === "Partners")[0];
activityMenuItem.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const activityManager = findAll(shell, (node) => String(node.className).includes("o_action_manager"))[0];
const activityCard = findAll(activityManager, (node) => String(node.className).includes("o_activity_card") && node.dataset?.activityId === "41")[0];
assert.equal(activityCard.dataset.resModel, "res.partner");
assert.equal(activityCard.dataset.resId, "11");
activityCard.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(findAll(activityManager, (node) => String(node.className).includes("gorp-window-action") && node.dataset?.model === "res.partner" && node.dataset?.view === "form").length, 1);
assert.equal(globalThis.location.hash, "#model=res.partner&view_type=form&id=11");
assert.equal(fetches.some((item) => item.route === "/web/dataset/call_kw/res.partner/web_read"), true);
activityMenuItem.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const doneButton = findAll(activityManager, (node) => node.dataset?.activityAction === "action_feedback")[0];
const feedback = findAll(activityManager, (node) => node.attributes?.placeholder === "Write Feedback")[0];
feedback.value = "Resolved";
fetches.length = 0;
doneButton.dispatchEvent(new CustomEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(fetches.map((item) => item.route), [
  "/web/dataset/call_kw/mail.activity/action_feedback",
  "/mail/data"
]);
const doneFetch = fetches.find((item) => item.route === "/web/dataset/call_kw/mail.activity/action_feedback");
assert.deepEqual(JSON.parse(doneFetch.options.body), { args: [[41]], kwargs: { attachment_ids: [], feedback: "Resolved" } });
const activitySystray = findAll(shell, (node) => String(node.className).includes("o_activity_menu"))[0];
assert.equal(findAll(activitySystray, (node) => String(node.className).includes("o-systray-counter") && node.hidden === true && node.textContent === "0").length, 1);
assert.equal(findAll(shell, (node) => node.dataset?.systrayItem === "Partners").length, 0);
assert.equal(findAll(shell, (node) => node.dataset?.systrayItem === "No activities").length, 1);
assert.equal(findAll(activityManager, (node) => String(node.className).includes("o_systray_action_row") && node.textContent === "No activities").length, 1);

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
