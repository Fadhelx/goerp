import assert from "node:assert/strict";

const events = {};
const fetches = [];

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
    return { ok: true, status: 200, async json() { return { uid: 7, name: "Admin" }; } };
  }
  if (route === "/web/webclient/load_menus") {
    return { ok: true, status: 200, async json() { return { all_menu_ids: [1], children: {} }; } };
  }
  throw new Error(`unexpected fetch ${route}`);
};

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
assert.deepEqual(detail.menus.all_menu_ids, [1]);
assert.equal(typeof mod.bootstrapGoERPWebClient, "function");
assert.deepEqual(fetches.map((item) => [item.route, item.options.method]), [
  ["/web/session/get_session_info", "GET"],
  ["/web/webclient/load_menus", "GET"]
]);
