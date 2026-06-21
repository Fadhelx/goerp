import assert from "node:assert/strict";

const events = {};
const fetches = [];
let sessionResponse = { uid: 7, name: "Admin", company_name: "My Company" };

globalThis.location = { search: "" };
globalThis.matchMedia = () => ({ matches: false });
globalThis.document = {
  documentElement: { dataset: {} },
  body: {
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
      children: [],
      replaceChildren(...nodes) {
        this.children = nodes;
      },
      append(...nodes) {
        this.children.push(...nodes);
      },
      setAttribute(name, value) {
        this[name] = String(value);
      },
      addEventListener() {}
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
  if (route === "/web/webclient/load_menus") {
    return { ok: true, status: 200, async json() { return {
      all_menu_ids: [1, 2],
      root: { children: [1, 2] },
      1: { id: 1, name: "Settings", children: [] },
      2: { id: 2, name: "Server Actions", children: [] }
    }; } };
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
assert.equal(globalThis.document.documentElement.dataset.tsWebclient, "available");
assert.equal(fetches.length, 0);
await mod.bootstrapGoERPWebClient();
const detail = await ready;

assert.equal(globalThis.document.documentElement.dataset.tsWebclient, "ready");
assert.equal(detail.session.uid, 7);
assert.deepEqual(detail.menus.all_menu_ids, [1, 2]);
assert.equal(typeof mod.bootstrapGoERPWebClient, "function");
assert.deepEqual(fetches.map((item) => [item.route, item.options.method]), [
  ["/web/session/get_session_info", "GET"],
  ["/web/webclient/load_menus", "GET"]
]);

fetches.length = 0;
sessionResponse = { uid: 0, name: "User 0", company_name: "My Company", quick_login: true };
globalThis.location.search = "?ts_webclient=1";
await mod.bootstrapGoERPWebClient();
const shell = globalThis.document.body.children[0].children[0];
assert.match(shell.className, /o_web_client/);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_main_navbar")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_action_manager")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_home_menu")).length, 1);
assert.equal(findAll(shell, (node) => String(node.className).includes("o_app_name")).length, 3);
assert.equal(findAll(shell, (node) => String(node.textContent).includes("Admin")).length, 1);
assert.equal(findAll(shell, (node) => String(node.textContent).includes("My Company")).length, 1);
assert.deepEqual(fetches.map((item) => [item.route, item.options.method]), [
  ["/web/session/get_session_info", "GET"],
  ["/web/session/authenticate", "POST"],
  ["/web/webclient/load_menus", "GET"]
]);
