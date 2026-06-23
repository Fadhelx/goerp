import assert from "node:assert/strict";
let createActionService;
let createClientActionExecutor;
let createDatasetService;
let createORMService;
let createRPCService;
let createWebClient;
let createWebClientServices;
let BUILTINS;
let createDialogService;
let createNotificationService;
let createPortalMailService;
let Domain;
let DuplicatedKeyError;
let EvaluationError;
let evaluateBooleanExpr;
let evaluateExpr;
let evalPartialContext;
let execOnIterable;
let formatAST;
let InvalidDomainError;
let KeyNotFoundError;
let makeContext;
let makeEnv;
let parseExpr;
let portalAccessFormFields;
let portalAccessPayload;
let portalMailAvatarUrl;
let PyDate;
let PyDateTime;
let PyTime;
let PyTimeDelta;
let registry;
let Registry;
let renderWindowAction;
let renderWindowActionDialog;
let registries;
let session;
let serviceMetadata;
let startServices;
let toPyDict;
let toPyValue;
let UPDATE_METHODS;
let uniqueId;
let user;
let x2ManyCommands;

class TestEvent {
  constructor(type, options = {}) {
    this.type = type;
    Object.assign(this, options);
    this.detail = options.detail;
    this.bubbles = options.bubbles === true;
    this.defaultPrevented = false;
    this.target = null;
    this.currentTarget = null;
  }

  preventDefault() {
    this.defaultPrevented = true;
  }
}

function testDataTransfer() {
  const data = new Map();
  return {
    dropEffect: "",
    effectAllowed: "",
    setData(type, value) {
      data.set(type, String(value));
    },
    getData(type) {
      return data.get(type) ?? "";
    }
  };
}

const documentListeners = {};
globalThis.document = {
  activeElement: null,
  implementation: {
    createDocument() {
      return {};
    }
  },
  createDocumentFragment() {
    return {
      tag: "#fragment",
      tagName: "#FRAGMENT",
      className: "",
      dataset: {},
      attributes: {},
      textContent: "",
      children: [],
      get childNodes() {
        return this.children;
      },
      append(...nodes) {
        this.children.push(...nodes.flatMap((node) => node?.tag === "#fragment" ? node.children : [node]));
      }
    };
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
      value: "",
      children: [],
      listeners: {},
      append(...nodes) {
        this.children.push(...nodes.flatMap((node) => node?.tag === "#fragment" ? node.children : [node]));
      },
      replaceChildren(...nodes) {
        this.children = nodes.flatMap((node) => node?.tag === "#fragment" ? node.children : [node]);
      },
      setAttribute(name, value) {
        this.attributes[name] = String(value);
        if (name.startsWith("data-")) this.dataset[dataAttributeKey(name)] = String(value);
      },
      getAttribute(name) {
        return this.attributes[name] ?? null;
      },
      removeAttribute(name) {
        delete this.attributes[name];
        if (name.startsWith("data-")) delete this.dataset[dataAttributeKey(name)];
      },
      focus() {
        this.focused = true;
        globalThis.document.activeElement = this;
      },
      select() {
        this.selectionStart = 0;
        this.selectionEnd = String(this.value ?? "").length;
        this.textSelected = true;
      },
      click() {
        this.dispatchEvent(new TestEvent("click"));
      },
      addEventListener(type, listener) {
        this.listeners[type] = [...(this.listeners[type] ?? []), listener];
      },
      dispatchEvent(event) {
        try {
          event.target ??= this;
          event.currentTarget = this;
        } catch {}
        for (const listener of this.listeners[event.type] ?? []) {
          listener.call(this, event);
        }
        return !event.defaultPrevented;
      }
    };
  },
  addEventListener(type, listener) {
    documentListeners[type] = [...(documentListeners[type] ?? []), listener];
  },
  removeEventListener(type, listener) {
    documentListeners[type] = (documentListeners[type] ?? []).filter((item) => item !== listener);
  },
  dispatchEvent(event) {
    try {
      event.target ??= this;
      event.currentTarget = this;
    } catch {}
    for (const listener of documentListeners[event.type] ?? []) listener.call(this, event);
    return !event.defaultPrevented;
  }
};

function dataAttributeKey(name) {
  return name.slice(5).replace(/-([a-z])/g, (_match, letter) => letter.toUpperCase());
}
const windowListeners = {};
globalThis.window = {
  innerWidth: 1024,
  requestAnimationFrame(callback) {
    return setTimeout(callback, 0);
  },
  cancelAnimationFrame(handle) {
    clearTimeout(handle);
  },
  matchMedia(query) {
    return { media: query, matches: this.innerWidth < 768 };
  },
  addEventListener(type, listener) {
    windowListeners[type] = [...(windowListeners[type] ?? []), listener];
  },
  removeEventListener(type, listener) {
    windowListeners[type] = (windowListeners[type] ?? []).filter((item) => item !== listener);
  },
  dispatchEvent(event) {
    try {
      event.target ??= this;
      event.currentTarget = this;
    } catch {}
    for (const listener of windowListeners[event.type] ?? []) listener.call(this, event);
    return !event.defaultPrevented;
  }
};
if (!globalThis.CustomEvent) globalThis.CustomEvent = TestEvent;

({
  createActionService,
  createClientActionExecutor,
  createDatasetService,
  createORMService,
  createRPCService,
  createWebClient,
  createWebClientServices,
  BUILTINS,
  createDialogService,
  createNotificationService,
  createPortalMailService,
  Domain,
  DuplicatedKeyError,
  EvaluationError,
  evaluateBooleanExpr,
  evaluateExpr,
  evalPartialContext,
  execOnIterable,
  formatAST,
  InvalidDomainError,
  KeyNotFoundError,
  makeContext,
  makeEnv,
  parseExpr,
  portalAccessFormFields,
  portalAccessPayload,
  portalMailAvatarUrl,
  PyDate,
  PyDateTime,
  PyTime,
  PyTimeDelta,
  registry,
  Registry,
  renderWindowAction,
  renderWindowActionDialog,
  registries,
  session,
  serviceMetadata,
  startServices,
  toPyDict,
  toPyValue,
  UPDATE_METHODS,
  uniqueId,
  user,
  x2ManyCommands
} = await import("../../../dist/packages/webclient/src/index.js"));

function findAll(node, predicate, out = []) {
  if (predicate(node)) out.push(node);
  for (const child of node.children ?? []) findAll(child, predicate, out);
  return out;
}

const sessionAlias = await import("../../../dist/packages/webclient/src/aliases/session.js");
const functionsAlias = await import("../../../dist/packages/webclient/src/aliases/core/utils/functions.js");
assert.equal(sessionAlias.session, session);
assert.equal(functionsAlias.uniqueId, uniqueId);

registries.actions.add("demo", { type: "client" });
assert.deepEqual(registries.actions.get("demo"), { type: "client" });

const updates = [];
const ordered = registry.category("ordered_demo");
ordered.addEventListener("UPDATE", (event) => updates.push(event.detail));
ordered.add("late", "late", { sequence: 80 }).add("early", "early", { sequence: 10 });
assert.deepEqual(ordered.getAll(), ["early", "late"]);
assert.deepEqual(ordered.getEntries(), [["early", "early"], ["late", "late"]]);
assert.equal(ordered.contains("early"), true);
assert.equal(ordered.get("missing", "fallback"), "fallback");
assert.throws(() => ordered.get("missing"), KeyNotFoundError);
assert.throws(() => ordered.add("early", "again"), DuplicatedKeyError);
ordered.add("early", "forced", { force: true });
assert.deepEqual(ordered.getEntries(), [["early", "forced"], ["late", "late"]]);
ordered.remove("late");
assert.deepEqual(ordered.getAll(), ["forced"]);
assert.deepEqual(updates.map((update) => update.operation), ["add", "add", "add", "delete"]);
assert.deepEqual(updates.at(-1).value, [80, "late"]);

const debugDefault = registry.category("debug").category("default");
debugDefault.add("becomeSuperuser", { description: "Become Superuser" });
assert.equal(debugDefault.contains("becomeSuperuser"), true);
delete debugDefault.content.becomeSuperuser;
assert.equal(debugDefault.contains("becomeSuperuser"), false);
debugDefault.add("becomeSuperuser", { description: "Login As Superuser" });
assert.deepEqual(debugDefault.get("becomeSuperuser"), { description: "Login As Superuser" });

const serviceEvents = [];
const serviceSource = new Registry("test_services");
serviceSource.add("base", {
  start(env) {
    serviceEvents.push(["base", env.debug]);
    return { value: 1 };
  }
});
serviceSource.add("dependent", {
  dependencies: ["base"],
  async: true,
  start(_env, deps) {
    serviceEvents.push(["dependent", deps.base.value]);
    return { value: deps.base.value + 1 };
  }
});
const env = makeEnv({ debug: "assets" });
await startServices(env, serviceSource);
assert.deepEqual(serviceEvents, [["base", "assets"], ["dependent", 1]]);
assert.equal(env.services.dependent.value, 2);
assert.equal(serviceMetadata.dependent, true);
await env.isReady;

serviceSource.add("late", {
  dependencies: ["dependent"],
  start(_env, deps) {
    return { value: deps.dependent.value + 1 };
  }
});
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(env.services.late.value, 3);

const missingSource = new Registry("missing_services");
missingSource.add("broken", {
  dependencies: ["missing"],
  start() {
    return true;
  }
});
await assert.rejects(() => startServices(makeEnv(), missingSource), /Missing dependencies: missing/);

const defaultEnv = makeEnv({
  services: {},
  debug: true
});
defaultEnv.rpcTransport = (request) => Promise.resolve({ route: request.route, params: request.params });
await startServices(defaultEnv);
assert.equal(typeof defaultEnv.services.rpc.call, "function");
assert.equal(typeof defaultEnv.services.dataset.callKw, "function");
assert.equal(typeof defaultEnv.services.orm.webRead, "function");
assert.equal(typeof defaultEnv.services.view.loadViews, "function");
assert.equal(typeof defaultEnv.services.action.doAction, "function");
assert.equal(typeof defaultEnv.services.session.load, "function");
assert.equal(typeof defaultEnv.services.mail.chatterFetch, "function");
assert.equal(typeof defaultEnv.services.dialog.add, "function");
assert.equal(typeof defaultEnv.services.notification.add, "function");
assert.equal(serviceMetadata.orm.includes("webSearchRead"), true);
assert.equal(serviceMetadata.view.includes("loadViews"), true);
const defaultSession = await defaultEnv.services.session.load();
assert.equal(defaultSession.route, "/web/session/get_session_info");
assert.equal(session.route, "/web/session/get_session_info");

const defaultActionRequests = [];
const defaultActionEnv = makeEnv({
  services: {}
});
defaultActionEnv.rpcTransport = (request) => {
  defaultActionRequests.push(request);
  if (request.route === "/web/action/run") return Promise.resolve(false);
  return Promise.resolve({});
};
await startServices(defaultActionEnv);
const defaultServerActionResult = await defaultActionEnv.services.action.doAction(
  { id: 77, type: "ir.actions.server", context: { from_action: true } },
  { additionalContext: { active_model: "res.partner", active_id: 12, active_ids: [12] } }
);
assert.deepEqual(defaultServerActionResult, { type: "ir.actions.act_window_close" });
assert.equal(defaultActionRequests[0].route, "/web/action/run");
assert.deepEqual(defaultActionRequests[0].params.context, {
  from_action: true,
  active_model: "res.partner",
  active_id: 12,
  active_ids: [12]
});

const previousFetch = globalThis.fetch;
globalThis.fetch = () => Promise.resolve({
  json: () => Promise.resolve({
    jsonrpc: "2.0",
    id: 1,
    error: {
      code: 0,
      message: "Odoo Server Error",
      data: {
        name: "odoo.addons.base.models.ir_actions.ServerActionWithWarningsError",
        message: "Server action Warned Action has one or more warnings, address them first."
      }
    }
  })
});
await assert.rejects(
  () => createRPCService().call("/web/action/run", {}),
  (error) => {
    assert.equal(error.message, "Server action Warned Action has one or more warnings, address them first.");
    assert.equal(error.rpcError.data.name, "odoo.addons.base.models.ir_actions.ServerActionWithWarningsError");
    return true;
  }
);
globalThis.fetch = previousFetch;

const sessionService = createWebClientServices({
  transport(request) {
    return Promise.resolve({
      uid: 7,
      is_system: true,
      is_admin: true,
      user_context: { lang: "en_US" },
      groups: { "10": { id: 10 }, "base.group_allow_export": false, "base.group_user": true },
      route: request.route
    });
  }
}).session;
await sessionService.load();
assert.equal(session.uid, 7);
assert.equal(user.userId, 7);
assert.equal(user.isSystem, true);
assert.equal(user.isAdmin, true);
assert.equal(await user.hasGroup("10"), true);
assert.equal(await user.hasGroup("base.group_user"), true);
assert.equal(await user.hasGroup("base.group_allow_export"), false);
assert.equal(uniqueId("workflow-name").startsWith("workflow-name"), true);
const dialog = createDialogService();
let confirmed = false;
await dialog.add("Dialog", { confirm: () => { confirmed = true; } });
assert.equal(confirmed, true);
assert.equal(dialog.calls.length, 1);
const notification = createNotificationService();
notification.add("Done", { type: "success" });
assert.deepEqual(notification.calls[0], { message: "Done", type: "success" });
assert.equal(registries.systray, registry.category("systray"));
assert.equal(registries.user_menuitems, registry.category("user_menuitems"));

const app = createWebClient({
  env: { debug: true },
  theme: {
    name: "standard",
    color: {},
    typography: {},
    radius: {},
    spacing: {},
    density: "comfortable"
  }
});
const root = app.render();
assert.equal(root.dataset.theme, "standard");
assert.equal(root.children.length, 2);

const requests = [];
const rpc = createRPCService({
  transport(request) {
    requests.push(request);
    return Promise.resolve({ ok: true, route: request.route, params: request.params });
  }
});
const dataset = createDatasetService(rpc);
await dataset.callKw("res.partner", "search_read", [[["active", "=", true]]], { limit: 5 });
await dataset.callButton("purchase.order", "approval_action_button", [[42], 7], { context: { lang: "en_US" } });
assert.equal(requests[0].route, "/web/dataset/call_kw/res.partner/search_read");
assert.deepEqual(requests[0].params.args, [[["active", "=", true]]]);
assert.equal(requests[1].route, "/web/dataset/call_button/purchase.order/approval_action_button");
assert.deepEqual(requests[1].params.args, [[42], 7]);

const mailRequests = [];
const uploadRequests = [];
const mail = createPortalMailService(createRPCService({
  transport(request) {
    mailRequests.push(request);
    return Promise.resolve({ ok: true, route: request.route, params: request.params });
  }
}), {
  uploadTransport(request) {
    uploadRequests.push(request);
    return Promise.resolve({ ok: true });
  }
});
assert.deepEqual(portalAccessPayload({ access_token: "thread-token", _hash: "hash-token", pid: "12" }), {
  token: "thread-token",
  hash: "hash-token",
  pid: "12"
});
assert.deepEqual(portalAccessFormFields({ accessToken: "thread-token", hash: "hash-token", pid: 12 }), {
  token: "thread-token",
  hash: "hash-token",
  pid: "12"
});
assert.equal(
  portalMailAvatarUrl(55, { access_token: "thread token", hash: "hash/token", pid: 12 }),
  "/mail/avatar/mail.message/55/author_avatar/50x50?access_token=thread+token&_hash=hash%2Ftoken&pid=12"
);
await mail.chatterInit({ threadModel: "portal.thread", threadId: 4 }, { access_token: "thread-token" });
await mail.chatterFetch({ thread_model: "portal.thread", thread_id: 4 }, { limit: 10, before: 9 }, { hash: "hash-token", pid: 12 });
await mail.postMessage(
  { threadModel: "portal.thread", threadId: 4 },
  { body: "<p>Portal</p>", attachment_ids: [8], attachment_tokens: ["own"] },
  { access: { token: "thread-token" }, context: { mail_post_autofollow: true } }
);
await mail.updateMessageContent(17, { body: "<p>Edit</p>" }, { hash: "hash-token", pid: 12 });
await mail.reactMessage(17, "ok", "add", { token: "thread-token" });
await mail.starredMessages({ limit: 5 });
await mail.storeData(["init_messaging", ["systray_get_activities", {}]], { lang: "en_US" });
await mail.toggleMessageStarred(17);
await mail.unstarAllMessages();
await mail.deleteAttachment(8, "own");
await mail.uploadAttachment(
  { threadModel: "portal.thread", threadId: 4 },
  "hello",
  {
    access: { access_token: "thread-token" },
    isPending: true,
    temporaryId: "tmp-1",
    tmpUrl: "blob:tmp",
    activityId: 33,
    extra: { custom: "1", token: "bad" }
  }
);
assert.equal(mailRequests[0].route, "/portal/chatter_init");
assert.deepEqual(mailRequests[0].params, { thread_model: "portal.thread", thread_id: 4, token: "thread-token" });
assert.equal(mailRequests[1].route, "/mail/chatter_fetch");
assert.deepEqual(mailRequests[1].params, {
  thread_model: "portal.thread",
  thread_id: 4,
  fetch_params: { limit: 10, before: 9 },
  hash: "hash-token",
  pid: 12
});
assert.equal(mailRequests[2].route, "/mail/message/post");
assert.deepEqual(mailRequests[2].params.post_data.attachment_ids, [8]);
assert.deepEqual(mailRequests[2].params.post_data.attachment_tokens, ["own"]);
assert.equal(mailRequests[2].params.token, "thread-token");
assert.deepEqual(mailRequests[2].params.context, { mail_post_autofollow: true });
assert.equal(mailRequests[3].route, "/mail/message/update_content");
assert.deepEqual(mailRequests[3].params, { message_id: 17, update_data: { body: "<p>Edit</p>" }, hash: "hash-token", pid: 12 });
assert.equal(mailRequests[4].route, "/mail/message/reaction");
assert.deepEqual(mailRequests[4].params, { message_id: 17, content: "ok", action: "add", token: "thread-token" });
assert.equal(mailRequests[5].route, "/mail/starred/messages");
assert.deepEqual(mailRequests[5].params, { fetch_params: { limit: 5 } });
assert.equal(mailRequests[6].route, "/mail/data");
assert.deepEqual(mailRequests[6].params, {
  fetch_params: ["init_messaging", ["systray_get_activities", {}]],
  context: { lang: "en_US" }
});
assert.equal(mailRequests[7].route, "/web/dataset/call_kw/mail.message/toggle_message_starred");
assert.deepEqual(mailRequests[7].params, {
  model: "mail.message",
  method: "toggle_message_starred",
  args: [[17]],
  kwargs: {}
});
assert.equal(mailRequests[8].route, "/web/dataset/call_kw/mail.message/unstar_all");
assert.deepEqual(mailRequests[8].params, {
  model: "mail.message",
  method: "unstar_all",
  args: [],
  kwargs: {}
});
assert.equal(mailRequests[9].route, "/mail/attachment/delete");
assert.deepEqual(mailRequests[9].params, { attachment_id: 8, access_token: "own" });
assert.equal(uploadRequests[0].route, "/mail/attachment/upload");
assert.equal(uploadRequests[0].formData.get("thread_model"), "portal.thread");
assert.equal(uploadRequests[0].formData.get("thread_id"), "4");
assert.equal(uploadRequests[0].formData.get("token"), "thread-token");
assert.equal(uploadRequests[0].formData.get("is_pending"), "true");
assert.equal(uploadRequests[0].formData.get("temporary_id"), "tmp-1");
assert.equal(uploadRequests[0].formData.get("tmp_url"), "blob:tmp");
assert.equal(uploadRequests[0].formData.get("activity_id"), "33");
assert.equal(uploadRequests[0].formData.get("custom"), "1");
assert.equal(uploadRequests[0].formData.get("ufile"), "hello");

const orm = createORMService(rpc, { userContext: { lang: "en_US" } });
await orm.searchRead("res.partner", [["active", "=", true]], ["name"], { limit: 3, order: "name desc", context: { tz: "UTC" } });
await orm.webRead("res.partner", [42], {
  specification: { child_ids: { fields: { display_name: {} } } }
});
await orm.webReadGroup("res.partner", [["active", "=", true]], ["company_id"], ["amount:sum"], { orderby: "amount:sum desc" });
await orm.webSearchRead("res.partner", [["active", "=", true]], { order: "name asc", limit: 1 });
await orm.write("res.partner", [42], { name: "Updated" });
assert.equal(requests[2].route, "/web/dataset/call_kw/res.partner/search_read");
assert.deepEqual(requests[2].params.kwargs.domain, [["active", "=", true]]);
assert.deepEqual(requests[2].params.kwargs.fields, ["name"]);
assert.equal(requests[2].params.kwargs.order, "name desc");
assert.deepEqual(requests[2].params.kwargs.context, { lang: "en_US", tz: "UTC" });
assert.equal(requests[3].params.method, "web_read");
assert.deepEqual(requests[3].params.args, [[42]]);
assert.equal(requests[4].params.method, "web_read_group");
assert.deepEqual(requests[4].params.kwargs.domain, [["active", "=", true]]);
assert.deepEqual(requests[4].params.kwargs.groupby, ["company_id"]);
assert.deepEqual(requests[4].params.kwargs.aggregates, ["amount:sum"]);
assert.equal(requests[4].params.kwargs.orderby, "amount:sum desc");
assert.equal(requests[5].params.method, "web_search_read");
assert.deepEqual(requests[5].params.kwargs.domain, [["active", "=", true]]);
assert.equal(requests[5].params.kwargs.order, "name asc");
assert.equal(requests[5].params.kwargs.limit, 1);
assert.equal(requests[6].params.method, "write");
assert.deepEqual(requests[6].params.args, [[42], { name: "Updated" }]);
assert.deepEqual(await orm.read("res.partner", [], ["name"]), []);
assert.deepEqual(await orm.unlink("res.partner", []), true);
assert.deepEqual(x2ManyCommands.create(false, { id: 99, name: "Line" }), [0, false, { name: "Line" }]);
assert.deepEqual(x2ManyCommands.set([1, 2]), [6, false, [1, 2]]);
assert.equal(UPDATE_METHODS.includes("web_save_multi"), true);
assert.deepEqual(evaluateExpr("{'default_partner_id': active_id, 'flag': True, 'empty': None}", { active_id: 42 }), {
  default_partner_id: 42,
  flag: true,
  empty: null
});
assert.deepEqual(evaluateExpr("[('id', 'in', active_ids), ('company_id', '=', context.get('company_id', False))]", {
  active_ids: [1, 2],
  context: { company_id: 3 }
}), [["id", "in", [1, 2]], ["company_id", "=", 3]]);
assert.equal(evaluateBooleanExpr("payment_state == 'invoicing_legacy' or move_type == 'entry'", {
  payment_state: "not_paid",
  move_type: "entry"
}), true);
assert.equal(evaluateBooleanExpr("question_type in ['char', 'text'] and question_type not in ['date']", {
  question_type: "text"
}), true);
assert.equal(evaluateBooleanExpr("parent.move_type == 'entry' or parent.state != 'posted'", {
  parent: { move_type: "out_invoice", state: "draft" }
}), true);
assert.equal(evaluateExpr("({'amounts': [1, 2, 3]}['amounts'][1] + 4) == 6"), true);
assert.equal(evaluateExpr("(amount_total - amount_residual) == 90", { amount_total: 120, amount_residual: 30 }), true);
assert.equal(evaluateExpr("'INV'.lower() == 'inv' and len(set([1, 1, 2])) == 2"), true);
assert.match(String(evaluateExpr("context_today().strftime('%Y-%m-%d')")), /^\d{4}-\d{2}-\d{2}$/);
assert.match(String(evaluateExpr("(datetime.date(2026, 6, 17) + relativedelta(weeks=1)).strftime('%Y-%m-%d')")), /^2026-06-24$/);
assert.throws(() => evaluateExpr("missing + 1"), EvalError);
assert.equal(formatAST(parseExpr("amount_total == 10")), "amount_total == 10");
assert.deepEqual(toPyValue({ a: 1 }), { type: 11, value: { a: { type: 0, value: 1 } } });
assert.equal(formatAST(toPyValue(null)), "None");
assert.equal(formatAST(toPyValue(new PyDate(2026, 6, 17))), "\"2026-06-17\"");
assert.equal(Object.getPrototypeOf(toPyDict({ a: 1 })) !== Object.prototype, true);
assert.deepEqual(makeContext([{ a: 1 }, "{'b': a + 1}", "{'c': b + 1}"]), { a: 1, b: 2, c: 3 });
assert.deepEqual(evalPartialContext("{'a': missing, 'b': 1, 'c': b + 1}", { b: 2 }), { b: 1, c: 3 });
const domain = new Domain("[('active', '=', True), ('company_id', '=', company_id)]");
assert.deepEqual(domain.toList({ company_id: 3 }), ["&", ["active", "=", true], ["company_id", "=", 3]]);
assert.equal(domain.contains({ active: true, company_id: 3 }), true);
assert.equal(domain.contains({ active: false, company_id: 3 }), false);
assert.deepEqual(Domain.or([[["state", "=", "draft"]], [["state", "=", "posted"]]]).toList(), ["|", ["state", "=", "draft"], ["state", "=", "posted"]]);
assert.deepEqual(Domain.removeDomainLeaves([["name", "ilike", "a"], ["company_id", "=", 3]], ["company_id"]).toList(), ["&", ["name", "ilike", "a"], [1, "=", 1]]);
assert.throws(() => new Domain(["|", ["name", "=", "x"]]), InvalidDomainError);
assert.equal(PyDate.create(2026, 6, 17).strftime("%Y-%m-%d"), "2026-06-17");
assert.equal(PyDateTime.create(2026, 6, 17, 13, 4, 5).strftime("%Y-%m-%d %H:%M:%S"), "2026-06-17 13:04:05");
assert.equal(PyTime.create(13, 4, 5).strftime("%H:%M:%S"), "13:04:05");
assert.equal(PyDate.create(2026, 6, 17).plus(PyTimeDelta.create({ weeks: 1 })).strftime("%Y-%m-%d"), "2026-06-24");
assert.equal(evaluateExpr("td.create(days=2).totalSeconds()", { td: PyTimeDelta }), 172800);
assert.equal(BUILTINS.bool([]), false);
assert.equal(BUILTINS.bool(PyTimeDelta.create()), false);
assert.deepEqual([...BUILTINS.set({ a: 1, b: 2 })], ["a", "b"]);
assert.equal(BUILTINS.max(1, 2, {}), 2);
assert.equal(BUILTINS.min(1, 2, {}), 1);
assert.equal(BUILTINS.datetime.date.create(2026, 6, 17).strftime("%Y-%m-%d"), "2026-06-17");
assert.equal(BUILTINS.datetime.time.create(13, 4, 5).strftime("%H:%M:%S"), "13:04:05");
assert.match(BUILTINS.today, /^\d{4}-\d{2}-\d{2}$/);
assert.match(BUILTINS.now, /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/);
assert.deepEqual(execOnIterable({ a: 1, b: 2 }, (iterable) => [...iterable]), ["a", "b"]);
assert.throws(() => execOnIterable(null, () => null), EvaluationError);

const actionCalls = [];
const action = createActionService((invocation) => {
  actionCalls.push(invocation);
  return { done: true };
});
const actionResult = await action.doAction({ type: "ir.actions.client", tag: "soft_reload" });
assert.deepEqual(actionResult, { done: true });
assert.equal(action.history.length, 1);
assert.equal(action.current.action.tag, "soft_reload");
assert.equal(actionCalls[0].action.type, "ir.actions.client");

const clientActions = new Registry("test_client_actions");
const dispatched = [];
const clientEnv = makeEnv();
clientActions.add("agent_chat_action", (handlerEnv, handlerAction, handlerOptions) => {
  dispatched.push({ env: handlerEnv, action: handlerAction, options: handlerOptions });
  return { channelId: handlerAction.params.channelId };
});
const clientAction = createActionService(createClientActionExecutor(clientActions, undefined, clientEnv));
const clientActionResult = await clientAction.doAction({
  type: "ir.actions.client",
  tag: "agent_chat_action",
  params: { channelId: 77 }
}, { additional_context: { active_id: 77 } });
assert.deepEqual(clientActionResult, { channelId: 77 });
assert.equal(dispatched.length, 1);
assert.equal(dispatched[0].action.tag, "agent_chat_action");
assert.equal(dispatched[0].env, clientEnv);
assert.deepEqual(dispatched[0].options.additional_context, { active_id: 77 });

clientActions.add("execute_action", {
  execute(handlerAction, handlerEnv, handlerOptions) {
    dispatched.push({ env: handlerEnv, action: handlerAction, options: handlerOptions });
    return handlerAction.params.name;
  }
});
assert.equal(
  await clientAction.doAction({ type: "ir.actions.client", tag: "execute_action", params: { name: "ok" } }),
  "ok"
);

const unknownClientAction = await createClientActionExecutor(clientActions)({
  action: { type: "ir.actions.client", tag: "missing" },
  options: {}
});
assert.deepEqual(unknownClientAction, { type: "ir.actions.client", tag: "missing" });
clientActions.add("not_executable", { component: "x" });
assert.throws(
  () => createClientActionExecutor(clientActions)({
    action: { type: "ir.actions.client", tag: "not_executable" },
    options: {}
  }),
  /not executable/
);

const serverActionRequests = [];
let serverActionCloseCount = 0;
const serverActionServices = createWebClientServices({
  transport(request) {
    serverActionRequests.push(request);
    if (request.route === "/web/action/load") {
      return Promise.resolve({
        id: 55,
        type: "ir.actions.server",
        name: "Open Partner Wizard",
        path: "/server/partners",
        context: { from_action: true }
      });
    }
    if (request.route === "/web/action/run") {
      return Promise.resolve({
        type: "ir.actions.act_window",
        name: "Returned Partner",
        res_model: "res.partner",
        views: [[false, "form"]],
        target: "new"
      });
    }
    if (request.route === "/web/dataset/call_kw/res.partner/get_views") {
      return Promise.resolve({
        views: {
          form: { arch: "<form><field name=\"name\"/></form>", id: 91 }
        },
        models: {
          "res.partner": {
            fields: { name: { type: "char", string: "Name" } }
          }
        }
      });
    }
    return Promise.resolve({});
  }
});
const serverActionResult = await serverActionServices.action.doAction(55, {
  additionalContext: { active_model: "res.partner", active_id: 9, active_ids: [9], from_options: true },
  onClose: () => {
    serverActionCloseCount += 1;
  }
});
assert.equal(serverActionResult.type, "ir.actions.act_window");
assert.equal(serverActionResult.resModel, "res.partner");
assert.equal(serverActionResult.action.path, "/server/partners");
assert.equal(serverActionCloseCount, 0);
assert.equal(serverActionServices.action.stack.length, 1);
assert.equal(serverActionServices.action.stack[0].action.type, "ir.actions.act_window");
assert.equal(serverActionServices.action.stack[0].dialog, true);
assert.equal(serverActionServices.action.stack[0].action.res_model, "res.partner");
assert.equal(serverActionRequests[0].route, "/web/action/load");
assert.deepEqual(serverActionRequests[0].params.context, { active_model: "res.partner", active_id: 9, active_ids: [9], from_options: true });
assert.equal(serverActionRequests[1].route, "/web/action/run");
assert.equal(serverActionRequests[1].params.action_id, 55);
assert.deepEqual(serverActionRequests[1].params.context, {
  from_action: true,
  active_model: "res.partner",
  active_id: 9,
  active_ids: [9],
  from_options: true
});
assert.equal(serverActionRequests[2].route, "/web/dataset/call_kw/res.partner/get_views");
await serverActionServices.action.doAction({ type: "ir.actions.act_window_close" });
assert.equal(serverActionCloseCount, 1);
assert.equal(serverActionServices.action.stack.length, 0);

const falseServerActionRequests = [];
let falseServerActionCloseCount = 0;
const falseServerActionServices = createWebClientServices({
  transport(request) {
    falseServerActionRequests.push(request);
    if (request.route === "/web/action/run") {
      return Promise.resolve(false);
    }
    return Promise.resolve({});
  }
});
const falseServerActionResult = await falseServerActionServices.action.doAction(
  { id: 56, type: "ir.actions.server", context: { from_action: "false" } },
  {
    additional_context: { active_id: 10, active_model: "res.partner" },
    onClose: () => {
      falseServerActionCloseCount += 1;
    }
  }
);
assert.deepEqual(falseServerActionResult, { type: "ir.actions.act_window_close" });
assert.equal(falseServerActionCloseCount, 1);
assert.equal(falseServerActionServices.action.stack.length, 0);
assert.equal(falseServerActionRequests[0].route, "/web/action/run");
assert.deepEqual(falseServerActionRequests[0].params.context, {
  from_action: "false",
  active_id: 10,
  active_model: "res.partner"
});

const services = createWebClientServices({
  transport(request) {
    return Promise.resolve({ uid: 5, requestId: request.id });
  }
});
const loadedSession = await services.session.load();
assert.equal(loadedSession.uid, 5);
assert.equal(services.session.info.uid, 5);

const windowActionRequests = [];
const actionServices = createWebClientServices({
  transport(request) {
    windowActionRequests.push(request);
    if (request.route === "/web/action/load") {
      return Promise.resolve({
        id: 7,
        type: "ir.actions.act_window",
        name: "Partners",
        res_model: "res.partner",
        views: [[8, "list"], [false, "form"], [9, "search"]],
        view_mode: "list,form",
        domain: "[('active', '=', True), ('id', 'in', active_ids)]",
        limit: 25,
        target: "current",
        context: { search_default_customer: true }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.partner/get_views") {
      return Promise.resolve({
        views: {
          list: {
            arch: "<list><field name=\"name\"/><field name=\"company_id\" context=\"{'default_active_id': active_id, 'from_context': context.get('from_context', False), 'none_value': None}\"/><field name=\"legacy_note\" invisible=\"payment_state == 'invoicing_legacy' or move_type == 'entry'\"/><field name=\"column_note\" column_invisible=\"parent.move_type == 'entry'\"/><field name=\"line_ids\" limit=\"2\" context=\"{'default_partner_id': active_id, 'flag': True}\"><list default_order=\"sequence desc\"><field name=\"description\"/><field name=\"user_id\"/></list></field></list>",
            id: 8,
            toolbar: {}
          },
          form: { arch: "<form/>", id: 10 },
          search: {
            arch: `
              <search>
                <field name="name"/>
                <field name="company_id"/>
                <field name="missing_field"/>
                <filter name="customer" string="Customers" domain="[('customer_rank', '>', 0)]" context="{'group_by': 'company_id', 'from_search': True}"/>
                <filter name="created_on" string="Created On" date="create_date" default_period="year,month-1"/>
                <filter name="group_company" string="Company" context="{'group_by': 'company_id'}"/>
                <filter name="group_created" string="Created" context="{'group_by': 'create_date'}"/>
              </search>
            `,
            id: 9,
            filters: [
              {
                id: 14,
                name: "Partner Favorite",
                domain: "[('supplier_rank', '>', 0)]",
                context: "{'search_default_supplier': 1}",
                group_by: ["user_id"],
                user_id: 7,
                action_id: 7,
                is_default: false
              }
            ]
          }
        },
        models: {
          "res.partner": {
            fields: {
              name: { type: "char", string: "Name" },
              company_id: { type: "many2one", relation: "res.company", string: "Company" },
              create_date: { type: "datetime", string: "Created on" },
              legacy_note: { type: "char", string: "Legacy" },
              column_note: { type: "char", string: "Column" },
              line_ids: { type: "one2many", relation: "res.partner.line", string: "Lines" }
            }
          },
          "res.partner.line": {
            fields: {
              description: { type: "char", string: "Description" },
              user_id: { type: "many2one", relation: "res.users", string: "User" }
            }
          }
        }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.partner/web_search_read") {
      return Promise.resolve({
        length: 1,
        records: [{ id: 1, name: "Azure Interior", company_id: [3, "My Company"], create_date: "2026-06-22 09:00:00", legacy_note: "hidden", column_note: "hidden", move_type: "entry", payment_state: "not_paid", line_ids: [] }]
      });
    }
    return Promise.resolve({});
  }
});
const windowResult = await actionServices.action.doAction("partners", {
  additionalContext: { active_id: 42, active_ids: [1, 2], lang: "en_US", from_context: 7 }
});
assert.equal(windowResult.type, "ir.actions.act_window");
assert.equal(windowResult.activeView, "list");
assert.equal(windowResult.resModel, "res.partner");
assert.equal(windowResult.viewDescriptions.views.list.id, 8);
assert.equal(windowResult.length, 1);
assert.equal(windowResult.offset, 0);
const windowSearchRead = windowActionRequests.find((request) => request.route === "/web/dataset/call_kw/res.partner/web_search_read");
assert.deepEqual(windowSearchRead.params.kwargs.domain, [
  ["active", "=", true],
  ["id", "in", [1, 2]],
  ["customer_rank", ">", 0]
]);
assert.equal(windowSearchRead.params.kwargs.offset, 0);
assert.equal(windowSearchRead.params.kwargs.count_limit, 10001);
assert.equal(windowSearchRead.params.kwargs.context.from_search, true);
assert.deepEqual(windowSearchRead.params.kwargs.groupby, ["company_id"]);
assert.deepEqual(windowResult.search.state.facets.map((facet) => [facet.id, facet.label]), [["filter-customer", "Customers"]]);
assert.deepEqual(windowResult.search.parsed.searchFields, ["name", "company_id", "missing_field"]);
assert.deepEqual(windowResult.search.filters.map((item) => [item.id, item.active, item.children?.length ?? 0]), [["filter-customer", true, 0], ["filter-created_on", false, 10]]);
assert.equal(windowResult.search.filters[1].children[0].id, "filter-created_on-month");
assert.equal(windowResult.search.filters[1].children[0].label, new Date().toLocaleString("en-US", { month: "long" }));
assert.deepEqual(windowResult.search.groupBys.map((item) => [item.id, item.children?.length ?? 0]), [["group-by-group_company", 0], ["group-by-group_created", 5]]);
assert.deepEqual(windowResult.search.groupBys[1].children.map((item) => [item.id, item.label]), [
  ["group-by-group_created-year", "Year"],
  ["group-by-group_created-quarter", "Quarter"],
  ["group-by-group_created-month", "Month"],
  ["group-by-group_created-week", "Week"],
  ["group-by-group_created-day", "Day"]
]);
assert.deepEqual(windowResult.search.favorites.map((item) => item.id), ["favorite-14"]);
assert.deepEqual(windowResult.search.favorites.map((item) => [item.favorite.id, item.favorite.userId, item.favorite.actionId, item.favorite.isDefault, item.favorite.canDelete]), [
  [14, 7, 7, false, true]
]);
assert.deepEqual(windowResult.records, [{ id: 1, name: "Azure Interior", company_id: [3, "My Company"], create_date: "2026-06-22 09:00:00", legacy_note: "hidden", column_note: "hidden", move_type: "entry", payment_state: "not_paid", line_ids: [] }]);
const renderedWindow = renderWindowAction(windowResult);
assert.equal(renderedWindow.className, "gorp-window-action");
assert.equal(renderedWindow.dataset.model, "res.partner");
assert.equal(renderedWindow.dataset.view, "list");
assert.ok(String(renderedWindow.children[0].className).includes("o_control_panel"));
const renderedMenuIDs = findAll(renderedWindow, (node) => node.dataset?.menuItemId).map((node) => node.dataset.menuItemId);
for (const id of ["filter-customer", "filter-created_on", "filter-created_on-year", "group-by-group_company", "group-by-group_created", "group-by-group_created-month", "favorite-14"]) {
  assert.ok(renderedMenuIDs.includes(id), `missing menu id ${id}`);
}
assert.equal(findAll(renderedWindow, (node) => node.className === "o_facet_value")[0].textContent, "Customers");
assert.ok(String(renderedWindow.children[1].className).includes("gorp-list-shell"));
assert.ok(String(renderedWindow.children[1].className).includes("o-list-view"));
const renderedListTable = findAll(renderedWindow, (node) => String(node.className ?? "").includes("gorp-list-view"))[0];
assert.ok(String(renderedListTable.className).includes("o_list_table"));
assert.equal(findAll(renderedListTable, (node) => String(node.className).includes("o_column_sortable") && node.dataset?.name === "name").length, 1);
assert.equal(renderedListTable.children[0].children[0].children[0].children[0].textContent, "Name");
assert.equal(renderedListTable.children[1].children[0].children[0].children[0].textContent, "Azure Interior");
const renderedNameHeader = findAll(renderedListTable, (node) => node.dataset?.name === "name")[0];
findAll(renderedNameHeader, (node) => String(node.className).includes("o_list_header_button"))[0].dispatchEvent(new TestEvent("click"));
assert.equal(renderedNameHeader.attributes["aria-sort"], "ascending");
const renderedMobileCard = findAll(renderedWindow, (node) => String(node.className ?? "").includes("o_mobile_record_card"))[0];
assert.equal(renderedMobileCard.attributes.role, "link");
assert.equal(findAll(renderedMobileCard, (node) => String(node.className ?? "").includes("o_mobile_record_title"))[0].textContent, "Azure Interior");
assert.equal(findAll(renderedMobileCard, (node) => String(node.className ?? "").includes("o_mobile_record_value"))[0].children[0].textContent, "My Company");
const listOpenCalls = [];
const interactiveListWindow = renderWindowAction(windowResult, {
  services: {
    action: {
      doAction(action, options) {
        listOpenCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const interactiveListRow = findAll(interactiveListWindow, (node) => node.tag === "tr" && node.dataset?.id === "1")[0];
assert.equal(interactiveListRow.attributes.role, "link");
interactiveListRow.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(listOpenCalls.length, 1);
assert.equal(listOpenCalls[0].action.res_id, 1);
assert.equal(listOpenCalls[0].action.res_model, "res.partner");
assert.equal(listOpenCalls[0].action.view_mode, "form");
assert.deepEqual(listOpenCalls[0].options, { additionalContext: {}, replaceLastAction: true });
const interactiveMobileCard = findAll(interactiveListWindow, (node) => String(node.className ?? "").includes("o_mobile_record_card") && node.dataset?.id === "1")[0];
interactiveMobileCard.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(listOpenCalls.length, 2);
assert.equal(listOpenCalls[1].action.res_id, 1);

const controlActionCalls = [];
const controlActionWindow = renderWindowAction({
  ...windowResult,
  action: {
    ...windowResult.action,
    __pager_offset: 25
  },
  offset: 25
}, {
  services: {
    action: {
      doAction(action, options) {
        controlActionCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
findAll(controlActionWindow, (node) => node.dataset?.menuItemId === "filter-customer")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(controlActionCalls.length, 1);
assert.deepEqual(controlActionCalls[0].action.__search_facets, []);
assert.equal("__pager_offset" in controlActionCalls[0].action, false);
assert.deepEqual(controlActionCalls[0].options, { additionalContext: {}, replaceLastAction: true });
findAll(controlActionWindow, (node) => node.dataset?.menuItemId === "group-by-group_company")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(controlActionCalls.length, 2);
assert.deepEqual(controlActionCalls[1].action.__search_facets.map((facet) => [facet.id, facet.type, facet.field]), [
  ["filter-customer", "filter", undefined],
  ["group-by-group_company", "groupBy", "company_id"]
]);
assert.equal("__pager_offset" in controlActionCalls[1].action, false);
assert.deepEqual(controlActionCalls[1].options, { additionalContext: {}, replaceLastAction: true });
findAll(controlActionWindow, (node) => node.dataset?.menuItemId === "filter-created_on-year")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
const currentYear = new Date().getFullYear();
assert.equal(controlActionCalls.length, 3);
assert.deepEqual(controlActionCalls[2].action.__search_facets.map((facet) => [facet.id, facet.type, facet.field, facet.categoryLabel, facet.valueLabels, facet.dateFilterID, facet.datePeriodID]), [
  ["filter-customer", "filter", undefined, undefined, undefined, undefined, undefined],
  ["filter-created_on-year", "dateFilter", "create_date", "Created On", [String(currentYear)], "filter-created_on", "year"]
]);
assert.equal(controlActionCalls[2].action.__search_facets[1].dateFieldType, "datetime");
findAll(controlActionWindow, (node) => node.dataset?.menuItemId === "filter-created_on-month-1")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(controlActionCalls.length, 4);
assert.deepEqual(controlActionCalls[3].action.__search_facets.map((facet) => [facet.id, facet.datePeriodID]), [
  ["filter-customer", undefined],
  ["filter-created_on-month-1", "month-1"],
  ["filter-created_on-year", "year"]
]);
findAll(controlActionWindow, (node) => node.dataset?.menuItemId === "group-by-group_created-year")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(controlActionCalls.length, 5);
assert.deepEqual(controlActionCalls[4].action.__search_facets.map((facet) => [facet.id, facet.type, facet.field, facet.interval, facet.categoryLabel, facet.valueLabels]), [
  ["filter-customer", "filter", undefined, undefined, undefined, undefined],
  ["group-by-group_created-year", "groupBy", "create_date", "year", "Created", ["Year"]]
]);
findAll(controlActionWindow, (node) => node.dataset?.viewType === "form")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(controlActionCalls.length, 6);
assert.deepEqual(controlActionCalls[5].action.views.slice(0, 2), [[false, "form"], [8, "list"]]);
assert.equal(controlActionCalls[5].action.view_mode, "form,list");
assert.equal(controlActionCalls[5].action.view_type, "form");
assert.equal("__pager_offset" in controlActionCalls[5].action, false);
assert.deepEqual(controlActionCalls[5].options, { additionalContext: {}, replaceLastAction: true });

const liveSearchCalls = [];
const liveSearchWindow = renderWindowAction({
  ...windowResult,
  action: {
    ...windowResult.action,
    __pager_offset: 25
  },
  offset: 25
}, {
  services: {
    action: {
      doAction(action, options) {
        liveSearchCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const liveSearchInput = findAll(liveSearchWindow, (node) => node.tag === "input" && String(node.className).includes("o_searchview_input"))[0];
liveSearchInput.value = "Azure";
liveSearchInput.dispatchEvent(new TestEvent("input"));
await Promise.resolve();
assert.equal(liveSearchCalls.length, 1);
assert.equal(liveSearchCalls[0].action.__search_query, "Azure");
assert.equal("__pager_offset" in liveSearchCalls[0].action, false);
assert.deepEqual(liveSearchCalls[0].action.__search_facets.map((facet) => facet.id), ["filter-customer"]);
assert.deepEqual(liveSearchCalls[0].options, { additionalContext: {}, replaceLastAction: true });

const pagedRequestStart = windowActionRequests.length;
const pagedWindowResult = await actionServices.action.doAction({
  ...windowResult.action,
  __pager_offset: 25
});
const pagedSearchRead = windowActionRequests.slice(pagedRequestStart).find((request) => request.route === "/web/dataset/call_kw/res.partner/web_search_read");
assert.equal(pagedSearchRead.params.kwargs.offset, 25);
assert.equal(pagedSearchRead.params.kwargs.limit, 25);
assert.equal(pagedSearchRead.params.kwargs.count_limit, 10001);
assert.equal(pagedWindowResult.offset, 25);

const extendedCountLimitStart = windowActionRequests.length;
await actionServices.action.doAction({
  ...windowResult.action,
  __pager_offset: 10000
});
const extendedCountLimitRead = windowActionRequests.slice(extendedCountLimitStart).find((request) => request.route === "/web/dataset/call_kw/res.partner/web_search_read");
assert.equal(extendedCountLimitRead.params.kwargs.offset, 10000);
assert.equal(extendedCountLimitRead.params.kwargs.count_limit, 10026);

const exactTotalRequestStart = windowActionRequests.length;
await actionServices.action.doAction({
  ...windowResult.action,
  __pager_offset: 25,
  __pager_total: 45
});
const exactTotalSearchRead = windowActionRequests.slice(exactTotalRequestStart).find((request) => request.route === "/web/dataset/call_kw/res.partner/web_search_read");
assert.equal(exactTotalSearchRead.params.kwargs.offset, 25);
assert.equal("count_limit" in exactTotalSearchRead.params.kwargs, false);

const viewCountLimitRequests = [];
const viewCountLimitServices = createWebClientServices({
  transport(request) {
    viewCountLimitRequests.push(request);
    if (request.route === "/web/action/load") {
      return Promise.resolve({
        id: 27,
        type: "ir.actions.act_window",
        name: "Tiny Partners",
        res_model: "res.partner",
        views: [[31, "list"], [false, "search"]],
        view_mode: "list",
        limit: 2,
        target: "current"
      });
    }
    if (request.route === "/web/dataset/call_kw/res.partner/get_views") {
      return Promise.resolve({
        views: {
          list: { arch: "<list count_limit=\"3\"><field name=\"name\"/></list>", id: 31 },
          search: { arch: "<search/>", id: false, filters: [] }
        },
        models: {
          "res.partner": {
            fields: { name: { type: "char", string: "Name" } }
          }
        }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.partner/web_search_read") {
      return Promise.resolve({ length: 4, records: [{ id: 1, name: "A" }, { id: 2, name: "B" }] });
    }
    return Promise.resolve({});
  }
});
const viewCountLimitResult = await viewCountLimitServices.action.doAction(27);
const viewCountLimitSearchRead = viewCountLimitRequests.find((request) => request.route === "/web/dataset/call_kw/res.partner/web_search_read");
assert.equal(viewCountLimitSearchRead.params.kwargs.limit, 2);
assert.equal(viewCountLimitSearchRead.params.kwargs.count_limit, 4);
assert.equal(viewCountLimitResult.length, 3);
assert.equal(viewCountLimitResult.countLimited, true);

const pagerActionCalls = [];
const pagerWindow = renderWindowAction({
  ...windowResult,
  action: {
    ...windowResult.action,
    __pager_offset: 25
  },
  length: 65,
  offset: 25
}, {
  services: {
    action: {
      doAction(action, options) {
        pagerActionCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const pagerPrevious = findAll(pagerWindow, (node) => String(node.className).includes("o_pager_previous"))[0];
const pagerNext = findAll(pagerWindow, (node) => String(node.className).includes("o_pager_next"))[0];
assert.equal(findAll(pagerWindow, (node) => String(node.className).includes("o_pager_value"))[0].textContent, "26-50");
pagerNext.dispatchEvent(new TestEvent("click"));
pagerPrevious.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(pagerActionCalls.length, 2);
assert.equal(pagerActionCalls[0].action.__pager_offset, 50);
assert.equal(pagerActionCalls[1].action.__pager_offset, 0);
assert.deepEqual(pagerActionCalls[0].action.__search_facets.map((facet) => facet.id), ["filter-customer"]);
assert.deepEqual(pagerActionCalls[0].options, { additionalContext: {}, replaceLastAction: true });

const limitedCountCalls = [];
const limitedCountActionCalls = [];
const limitedPagerWindow = renderWindowAction({
  ...windowResult,
  length: 10000,
  countLimited: true
}, {
  services: {
    orm: {
      searchCount(model, domain, kwargs) {
        limitedCountCalls.push({ model, domain, kwargs });
        return Promise.resolve(45);
      }
    },
    action: {
      doAction(action, options) {
        limitedCountActionCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const limitedPagerTotal = findAll(limitedPagerWindow, (node) => String(node.className).includes("o_pager_limit_fetch"))[0];
assert.equal(limitedPagerTotal.textContent, "10000+");
limitedPagerTotal.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
await Promise.resolve();
assert.equal(limitedCountCalls.length, 1);
assert.equal(limitedCountCalls[0].model, "res.partner");
assert.deepEqual(limitedCountCalls[0].domain, windowResult.search.state.domain);
assert.equal(limitedCountCalls[0].kwargs.context.from_search, true);
assert.equal(limitedCountActionCalls.length, 1);
assert.equal(limitedCountActionCalls[0].action.__pager_total, 45);
assert.equal(limitedCountActionCalls[0].action.__pager_offset, 0);

const autocompleteActionCalls = [];
const autocompleteWindow = renderWindowAction({
  ...windowResult,
  action: {
    ...windowResult.action,
    __search_query: "Azure",
    __pager_offset: 25
  },
  offset: 25,
  search: {
    ...windowResult.search,
    state: {
      ...windowResult.search.state,
      query: "Azure"
    },
    suggestions: [
      {
        id: "text-name-Azure",
        label: "Search Name for: Azure",
        field: "name",
        operator: "ilike",
        value: "Azure",
        facet: {
          id: "text-name-Azure",
          type: "text",
          label: "Azure",
          categoryLabel: "Name",
          valueLabels: ["Azure"],
          field: "name",
          operator: "ilike",
          value: "Azure"
        }
      }
    ]
  }
}, {
  services: {
    action: {
      doAction(action, options) {
        autocompleteActionCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const autocompleteItem = findAll(autocompleteWindow, (node) => String(node.className).includes("o_searchview_autocomplete_item"))[0];
assert.equal(autocompleteItem.dataset.searchField, "name");
assert.deepEqual(
  findAll(autocompleteWindow, (node) => String(node.className).includes("o_searchview_autocomplete_item")).map((node) => node.textContent),
  ["Search Name for: Azure", "Custom Filter..."]
);
autocompleteItem.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(autocompleteActionCalls.length, 1);
assert.equal("__search_query" in autocompleteActionCalls[0].action, false);
assert.equal("__pager_offset" in autocompleteActionCalls[0].action, false);
assert.deepEqual(autocompleteActionCalls[0].action.__search_facets.map((facet) => [facet.id, facet.type, facet.field, facet.value]), [
  ["filter-customer", "filter", undefined, undefined],
  ["text-name-Azure", "text", "name", "Azure"]
]);
assert.deepEqual(autocompleteActionCalls[0].options, { additionalContext: {}, replaceLastAction: true });

const favoriteCreates = [];
const favoriteActionCalls = [];
const favoriteNotifications = [];
const favoriteWindow = renderWindowAction({
  ...windowResult,
  action: {
    ...windowResult.action,
    __search_query: "Azure"
  },
  search: {
    ...windowResult.search,
    state: {
      ...windowResult.search.state,
      query: "Azure"
    }
  }
}, {
  services: {
    action: {
      doAction(action, options) {
        favoriteActionCalls.push({ action, options });
        return Promise.resolve({});
      }
    },
    orm: {
      create(model, records) {
        favoriteCreates.push({ model, records });
        return Promise.resolve([99]);
      }
    },
    notification: {
      add(message, options) {
        favoriteNotifications.push({ message, options });
      }
    }
  }
});
findAll(favoriteWindow, (node) => String(node.className).includes("o_add_favorite"))[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
await Promise.resolve();
assert.equal(favoriteCreates.length, 1);
assert.equal(favoriteCreates[0].model, "ir.filters");
assert.equal(favoriteCreates[0].records[0].name, "Azure");
assert.equal(favoriteCreates[0].records[0].model_id, "res.partner");
assert.equal(favoriteCreates[0].records[0].action_id, 7);
assert.deepEqual(JSON.parse(favoriteCreates[0].records[0].domain), windowResult.search.state.domain);
assert.equal(favoriteActionCalls.length, 1);
assert.equal(favoriteActionCalls[0].action.__search_query, "Azure");
assert.deepEqual(favoriteNotifications, [{ message: "Favorite saved", options: { type: "success" } }]);

const favoriteDeletes = [];
const favoriteDeleteActionCalls = [];
const favoriteDeleteNotifications = [];
const favoriteDeleteWindow = renderWindowAction({
  ...windowResult,
  action: {
    ...windowResult.action,
    __search_facets: [{ id: "favorite-14", type: "favorite", label: "Partner Favorite", domain: [["supplier_rank", ">", 0]], groupBy: ["user_id"] }]
  },
  search: {
    ...windowResult.search,
    state: {
      ...windowResult.search.state,
      facets: [{ id: "favorite-14", type: "favorite", label: "Partner Favorite", domain: [["supplier_rank", ">", 0]], groupBy: ["user_id"] }]
    },
    favorites: windowResult.search.favorites.map((item) => ({ ...item, active: item.id === "favorite-14" }))
  }
}, {
  services: {
    action: {
      doAction(action, options) {
        favoriteDeleteActionCalls.push({ action, options });
        return Promise.resolve({});
      }
    },
    orm: {
      unlink(model, ids) {
        favoriteDeletes.push({ model, ids });
        return Promise.resolve(true);
      }
    },
    notification: {
      add(message, options) {
        favoriteDeleteNotifications.push({ message, options });
      }
    }
  }
});
const favoriteDeleteButton = findAll(favoriteDeleteWindow, (node) => String(node.className).includes("o_favorite_delete"))[0];
assert.equal(favoriteDeleteButton.dataset.favoriteId, "14");
favoriteDeleteButton.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
await Promise.resolve();
assert.deepEqual(favoriteDeletes, [{ model: "ir.filters", ids: [14] }]);
assert.deepEqual(favoriteDeleteActionCalls[0].action.__search_facets, []);
assert.deepEqual(favoriteDeleteNotifications, [{ message: "Favorite deleted", options: { type: "success" } }]);

const nonFormSwitchCalls = [];
const formSwitchWindow = renderWindowAction({
  ...windowResult,
  activeView: "form",
  action: {
    ...windowResult.action,
    res_id: 1,
    views: [[false, "form"], [8, "list"], [false, "kanban"], [9, "search"]],
    view_mode: "form,list,kanban"
  }
}, {
  services: {
    action: {
      doAction(action, options) {
        nonFormSwitchCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
assert.equal(findAll(formSwitchWindow, (node) => String(node.className).includes("o_searchview_input")).length, 0);
assert.equal(findAll(formSwitchWindow, (node) => String(node.className).includes("o_searchview_dropdown_toggler")).length, 0);
findAll(formSwitchWindow, (node) => node.dataset?.viewType === "list")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(nonFormSwitchCalls.length, 1);
assert.equal(nonFormSwitchCalls[0].action.views[0][1], "list");
assert.equal(nonFormSwitchCalls[0].action.view_mode, "list,form,kanban");
assert.equal(nonFormSwitchCalls[0].action.view_type, "list");
assert.equal("res_id" in nonFormSwitchCalls[0].action, false);
findAll(formSwitchWindow, (node) => node.dataset?.viewType === "kanban")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(nonFormSwitchCalls.length, 2);
assert.equal(nonFormSwitchCalls[1].action.views[0][1], "kanban");
assert.equal(nonFormSwitchCalls[1].action.view_mode, "kanban,form,list");
assert.equal(nonFormSwitchCalls[1].action.view_type, "kanban");
assert.equal("res_id" in nonFormSwitchCalls[1].action, false);

const notebookFormWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Partner" },
  activeView: "form",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      email: { type: "char", string: "Email" },
      phone: { type: "char", string: "Phone" },
      internal_note: { type: "text", string: "Internal Note" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `
          <form>
            <sheet>
              <group><field name="name"/><field name="email"/></group>
              <notebook>
                <page string="Contacts" name="contacts_page"><field name="phone"/><field name="name"/></page>
                <page string="Internal" name="internal_page"><field name="internal_note"/></page>
              </notebook>
            </sheet>
          </form>
        `,
        id: 70
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 1,
    display_name: "Azure Interior",
    name: "Azure Interior",
    email: "info@example.test",
    phone: "+973 1700 0000",
    internal_note: "VIP"
  }
});
const notebook = findAll(notebookFormWindow, (node) => String(node.className ?? "").includes("o_notebook"))[0];
assert.equal(notebook.dataset.notebook, "notebook-0");
const notebookTabs = findAll(notebook, (node) => node.tag === "button" && String(node.className ?? "").includes("gorp-form-notebook-tab"));
assert.deepEqual(notebookTabs.map((node) => node.textContent), ["Contacts", "Internal"]);
assert.deepEqual(notebookTabs.map((node) => node.attributes["aria-selected"]), ["true", "false"]);
const notebookPages = findAll(notebook, (node) => String(node.className ?? "").includes("gorp-form-notebook-page"));
assert.deepEqual(notebookPages.map((node) => node.dataset.notebookPage), ["page-0-contacts_page", "page-0-internal_page"]);
assert.equal(notebookPages[0].hidden, undefined);
assert.equal(notebookPages[1].hidden, true);
assert.equal(findAll(notebookFormWindow, (node) => node.dataset?.field === "phone").length, 1);
assert.equal(findAll(notebookFormWindow, (node) => node.dataset?.field === "internal_note").length, 1);
assert.equal(findAll(notebookFormWindow, (node) => node.dataset?.field === "name").length, 2);
notebookTabs[1].dispatchEvent(new TestEvent("click"));
assert.deepEqual(notebookTabs.map((node) => node.attributes["aria-selected"]), ["false", "true"]);
assert.equal(notebookPages[0].hidden, true);
assert.equal(notebookPages[1].hidden, false);

const relationOpenCalls = [];
const many2OneFormWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Server Action" },
  activeView: "form",
  resModel: "ir.actions.server",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      model_id: { type: "many2one", relation: "ir.model", string: "Model" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="model_id"/></sheet></form>`,
        id: 71
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 23,
    display_name: "Update Records",
    name: "Update Records",
    model_id: [5, "Contact"]
  },
  context: { lang: "en_US" },
  services: {
    action: {
      doAction(action, options) {
        relationOpenCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const modelLink = findAll(many2OneFormWindow, (node) => String(node.className ?? "").includes("gorp-many2one-link"))[0];
assert.equal(modelLink.tag, "a");
assert.equal(modelLink.dataset.field, "model_id");
assert.equal(modelLink.dataset.relation, "ir.model");
assert.equal(modelLink.dataset.resId, "5");
assert.equal(modelLink.textContent, "Contact");
assert.equal(modelLink.href, "#model=ir.model&view_type=form&id=5");
modelLink.dispatchEvent(new TestEvent("click"));
assert.equal(relationOpenCalls.length, 1);
assert.deepEqual(relationOpenCalls[0].action, {
  type: "ir.actions.act_window",
  name: "Contact",
  res_model: "ir.model",
  res_id: 5,
  views: [[false, "form"]],
  view_mode: "form",
  target: "current"
});
assert.deepEqual(relationOpenCalls[0].options, { additionalContext: { lang: "en_US" }, replaceLastAction: true });

const dottedRelationSearchCalls = [];
const dottedRelationWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Delegation" },
  activeView: "form",
  resModel: "hr.delegation",
  viewDescriptions: {
    fields: {
      employee_id: { type: "many2one", relation: "hr.employee", string: "Employee" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="employee_id" domain="[('user_id.active','=', True)]" context="{'active_test': False}" limit="4"/></sheet></form>`,
        id: 72
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 25, employee_id: false },
  context: { lang: "en_US" },
  services: {
    orm: {
      call(model, method, args, kwargs) {
        dottedRelationSearchCalls.push({ model, method, args, kwargs });
        return Promise.resolve([[44, "Employee"]]);
      }
    }
  }
});
findAll(dottedRelationWindow, (node) => node.dataset?.formAction === "edit")[0].dispatchEvent(new TestEvent("click"));
const dottedRelationEditor = findAll(dottedRelationWindow, (node) => String(node.className ?? "").includes("gorp-many2one-editor") && node.dataset?.field === "employee_id")[0];
const dottedRelationToggle = findAll(dottedRelationEditor, (node) => String(node.className ?? "").includes("gorp-many2one-dropdown-toggle"))[0];
assert.equal(dottedRelationEditor.dataset.skippedDomain, "true");
assert.deepEqual(JSON.parse(dottedRelationEditor.dataset.searchDomain), []);
dottedRelationToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(dottedRelationSearchCalls[0], {
  model: "hr.employee",
  method: "name_search",
  args: [],
  kwargs: { name: "", domain: [], operator: "ilike", limit: 4, context: { lang: "en_US", active_test: false } }
});

const relationAffordanceCalls = [];
const relationAffordanceActions = [];
const relationAffordanceWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Contact Relation" },
  activeView: "form",
  resModel: "x.relation.test",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      partner_id: { type: "many2one", relation: "res.partner", string: "Partner" },
      tag_ids: { type: "many2many", relation: "res.partner", string: "Tags" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="partner_id" limit="1"/><field name="tag_ids" limit="1" options="{'search_more_limit': 3}"/></sheet></form>`,
        id: 74
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 26, name: "Relation Test", partner_id: false, tag_ids: [] },
  context: { lang: "en_US" },
  services: {
    orm: {
      call(model, method, args, kwargs) {
        relationAffordanceCalls.push({ kind: "call", model, method, args, kwargs });
        if (method === "name_create") return Promise.resolve([99, "Created Record"]);
        if (method === "name_search" && kwargs.limit === 1) return Promise.resolve([[1, "Alpha"]]);
        return Promise.resolve([[1, "Alpha"], [2, "Beta"], [3, "Gamma"]]);
      }
    },
    action: {
      doAction(action, options) {
        relationAffordanceActions.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
findAll(relationAffordanceWindow, (node) => node.dataset?.formAction === "edit")[0].dispatchEvent(new TestEvent("click"));
const relationAffordanceForm = relationAffordanceWindow.children[1];
const relationAffordanceM2O = findAll(relationAffordanceForm, (node) => String(node.className ?? "").includes("gorp-many2one-editor") && node.dataset?.field === "partner_id")[0];
const relationAffordanceM2OInput = findAll(relationAffordanceM2O, (node) => node.tag === "input" && node.dataset?.field === "partner_id")[0];
relationAffordanceM2OInput.value = "acme";
relationAffordanceM2OInput.dispatchEvent(new TestEvent("input"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(relationAffordanceCalls[0], {
  kind: "call",
  model: "res.partner",
  method: "name_search",
  args: [],
  kwargs: { name: "acme", domain: [], operator: "ilike", limit: 1, context: { lang: "en_US" } }
});
assert.deepEqual(findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-option")).map((node) => node.textContent), ["Alpha"]);
assert.equal(findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-create")).map((node) => node.textContent)[0], `Create "acme"`);
assert.equal(findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-create-edit")).map((node) => node.textContent)[0], "Create and edit...");
assert.equal(findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-search-more")).length, 1);
findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-search-more"))[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(relationAffordanceCalls[1], {
  kind: "call",
  model: "res.partner",
  method: "name_search",
  args: [],
  kwargs: { name: "acme", domain: [], operator: "ilike", limit: 80, context: { lang: "en_US" } }
});
assert.equal(relationAffordanceM2O.dataset.searchMoreOpened, "true");
assert.deepEqual(findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-option")).map((node) => node.textContent), ["Alpha", "Beta", "Gamma"]);
assert.equal(findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-search-more")).length, 0);
relationAffordanceM2OInput.value = "omega";
relationAffordanceM2OInput.dispatchEvent(new TestEvent("input"));
await new Promise((resolve) => setTimeout(resolve, 0));
findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-create-edit"))[0].dispatchEvent(new TestEvent("click"));
assert.deepEqual(relationAffordanceActions[0].action, {
  type: "ir.actions.act_window",
  name: "Create omega",
  res_model: "res.partner",
  views: [[false, "form"]],
  view_mode: "form",
  target: "new",
  context: { lang: "en_US", default_name: "omega" }
});
relationAffordanceM2OInput.value = "new acme";
relationAffordanceM2OInput.dispatchEvent(new TestEvent("input"));
await new Promise((resolve) => setTimeout(resolve, 0));
findAll(relationAffordanceM2O, (node) => String(node.className ?? "").includes("gorp-many2one-create"))[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(relationAffordanceCalls.filter((call) => call.method === "name_create").at(-1), {
  kind: "call",
  model: "res.partner",
  method: "name_create",
  args: ["new acme"],
  kwargs: { context: { lang: "en_US", default_name: "new acme" }, create_name_field: "name" }
});
assert.equal(relationAffordanceM2O.dataset.resId, "99");
assert.equal(relationAffordanceM2OInput.value, "Created Record");
const relationAffordanceM2M = findAll(relationAffordanceForm, (node) => String(node.className ?? "").includes("gorp-x2many-editor") && node.dataset?.field === "tag_ids")[0];
const relationAffordanceM2MInput = findAll(relationAffordanceM2M, (node) => node.tag === "input" && node.dataset?.field === "tag_ids")[0];
relationAffordanceM2MInput.value = "tag";
relationAffordanceM2MInput.dispatchEvent(new TestEvent("input"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(relationAffordanceCalls.filter((call) => call.kind === "call").at(-1), {
  kind: "call",
  model: "res.partner",
  method: "name_search",
  args: [],
  kwargs: { name: "tag", domain: [], operator: "ilike", limit: 1, context: { lang: "en_US" } }
});
assert.equal(findAll(relationAffordanceM2M, (node) => String(node.className ?? "").includes("gorp-x2many-create")).map((node) => node.textContent)[0], `Create "tag"`);
assert.equal(findAll(relationAffordanceM2M, (node) => String(node.className ?? "").includes("gorp-x2many-search-more")).length, 1);
findAll(relationAffordanceM2M, (node) => String(node.className ?? "").includes("gorp-x2many-search-more"))[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(relationAffordanceCalls.filter((call) => call.kind === "call").at(-1), {
  kind: "call",
  model: "res.partner",
  method: "name_search",
  args: [],
  kwargs: { name: "tag", domain: [], operator: "ilike", limit: 3, context: { lang: "en_US" } }
});
assert.equal(relationAffordanceM2M.dataset.searchMoreOpened, "true");
assert.deepEqual(findAll(relationAffordanceM2M, (node) => String(node.className ?? "").includes("gorp-x2many-option")).map((node) => node.textContent), ["Alpha", "Beta", "Gamma"]);

const genericFormSearchCalls = [];
const genericFormSaveCalls = [];
const genericFormSaveEvents = [];
const genericFormDiscardEvents = [];
const genericFormWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Server Action" },
  activeView: "form",
  resModel: "ir.actions.server",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name", required: true },
      email: { type: "char", string: "Email" },
      model_id: { type: "many2one", relation: "ir.model", string: "Model" },
      state: { type: "selection", string: "Type" },
      code: { type: "text", string: "Code" },
      group_ids: { type: "many2many", relation: "res.groups", string: "Groups" },
      line_ids: { type: "one2many", relation: "ir.actions.server.line", string: "Lines" },
      active: { type: "boolean", string: "Active" }
    },
    relatedModels: {
      "ir.actions.server.line": {
        fields: {
          description: { type: "char", string: "Description" },
          owner_id: { type: "many2one", relation: "res.users", string: "Owner" },
          quantity: { type: "integer", string: "Quantity" }
        }
      }
    },
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="model_id" domain="[('transient','=',False)]" context="{'active_test': False}" options="{'no_create': True, 'no_open': True}" limit="12"/><field name="state"/><field name="code"/><field name="group_ids" domain="[('share','=',False)]" context="{'active_test': False}" options="{'no_create': True}" limit="15"/><field name="active"/><notebook><page string="Details"><field name="email"/><field name="line_ids"><list><field name="description"/><field name="owner_id" domain="[('active','=',True)]" context="{'active_test': False}" options="{'no_create': True}" limit="5"/><field name="quantity"/></list></field></page></notebook></sheet></form>`,
        id: 73
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 24,
    display_name: "Update Records",
    name: "Update Records",
    email: "old@example.com",
    model_id: [5, "Contact"],
    state: "code",
    code: "result = True\nlog('ok')",
    group_ids: [[11, "Base / User"]],
    line_ids: [
      { id: 201, description: "Old line", owner_id: [7, "Administrator"], quantity: 1 },
      { id: 202, description: "Drop line", owner_id: false, quantity: 2 }
    ],
    active: true
  },
  context: { lang: "en_US" },
  services: {
    orm: {
      call(model, method, args, kwargs) {
        genericFormSearchCalls.push({ model, method, args, kwargs });
        if (model === "res.groups") return Promise.resolve([[11, "Base / User"], [30, "Sales / Manager"]]);
        if (model === "res.users") return Promise.resolve([[7, "Administrator"], [8, "Demo User"]]);
        return Promise.resolve([[81, "mail.mail"], [82, "mail.message"]]);
      },
      webSave(model, ids, changes, kwargs) {
        genericFormSaveCalls.push({ model, ids, changes, kwargs });
        return Promise.resolve([{ id: ids[0], ...changes }]);
      }
    }
  }
});
genericFormWindow.addEventListener("form:save", (event) => genericFormSaveEvents.push(event.detail));
genericFormWindow.addEventListener("form:discard", (event) => genericFormDiscardEvents.push(event.detail));
const genericEditButton = findAll(genericFormWindow, (node) => node.dataset?.formAction === "edit")[0];
const genericSaveButton = findAll(genericFormWindow, (node) => node.dataset?.formAction === "save")[0];
const genericDiscardButton = findAll(genericFormWindow, (node) => node.dataset?.formAction === "discard")[0];
const genericControlPanel = genericFormWindow.children[0];
const genericControlPanelButtons = findAll(genericControlPanel, (node) => String(node.className ?? "").includes("o_control_panel_main_buttons"))[0];
const genericControlPanelActions = findAll(genericControlPanel, (node) => String(node.className ?? "").includes("o_control_panel_actions"))[0];
assert.equal(findAll(genericControlPanelButtons, (node) => String(node.className ?? "").includes("gorp-form-action-menu")).length, 0);
assert.equal(findAll(genericControlPanelActions, (node) => String(node.className ?? "").includes("gorp-form-action-menu") && node.dataset?.controlPanelPlacement === "actions").length, 1);
const genericActionMenuSection = findAll(genericControlPanelActions, (node) => String(node.className ?? "").includes("gorp-action-menu-section") && node.dataset?.menu === "action")[0];
const genericActionMenuToggle = findAll(genericActionMenuSection, (node) => node.dataset?.actionMenuToggle === "action")[0];
assert.equal(genericActionMenuToggle.attributes["aria-expanded"], "false");
genericActionMenuToggle.dispatchEvent(new TestEvent("click"));
assert.equal(genericActionMenuToggle.attributes["aria-expanded"], "true");
assert.equal(genericActionMenuSection.dataset.open, "true");
assert.equal(String(genericActionMenuSection.className).split(/\s+/).includes("open"), true);
globalThis.document.dispatchEvent(new TestEvent("click"));
assert.equal(genericActionMenuToggle.attributes["aria-expanded"], "false");
assert.equal(genericActionMenuSection.dataset.open, "false");
genericActionMenuToggle.dispatchEvent(new TestEvent("click"));
assert.equal(genericActionMenuToggle.attributes["aria-expanded"], "true");
globalThis.document.dispatchEvent(new TestEvent("keydown", { key: "Escape" }));
assert.equal(genericActionMenuToggle.attributes["aria-expanded"], "false");
assert.equal(genericActionMenuSection.dataset.open, "false");
genericActionMenuToggle.dispatchEvent(new TestEvent("click"));
assert.equal(genericActionMenuToggle.attributes["aria-expanded"], "true");
genericActionMenuToggle.dispatchEvent(new TestEvent("keydown", { key: "Escape" }));
assert.equal(genericActionMenuToggle.attributes["aria-expanded"], "false");
assert.equal(genericActionMenuSection.dataset.open, "false");
assert.equal(genericEditButton.hidden, false);
assert.equal(genericSaveButton.hidden, true);
assert.equal(genericDiscardButton.hidden, true);
let genericForm = genericFormWindow.children[1];
const genericServerBand = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-server-action-band"))[0];
assert.equal(genericServerBand.dataset.state, "code");
assert.equal(findAll(genericServerBand, (node) => String(node.className ?? "").includes("gorp-server-action-state"))[0].textContent, "Execute Code");
const genericFormLabels = findAll(genericForm, (node) => String(node.className ?? "").split(/\s+/).includes("o_form_label")).map((node) => node.textContent);
assert.deepEqual(genericFormLabels.slice(0, 5), ["Name", "Model", "Type", "Allowed Groups", "Active"]);
const genericServerNotebook = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-server-action-notebook"))[0];
assert.deepEqual(findAll(genericServerNotebook, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-form-notebook-tab")).map((node) => node.textContent), ["Code", "Help"]);
const genericCodeViewer = findAll(genericServerNotebook, (node) => String(node.className ?? "").includes("gorp-code-viewer") && node.dataset?.field === "code")[0];
assert.equal(findAll(genericCodeViewer, (node) => node.tag === "code")[0].textContent.includes("log('ok')"), true);
const genericReadonlyState = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-selection-pills") && node.dataset?.field === "state")[0];
assert.equal(genericReadonlyState.dataset.value, "code");
const genericStatePills = findAll(genericReadonlyState, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-selection-pill"));
assert.deepEqual(genericStatePills.filter((node) => ["Execute Code", "Update Record", "Multi Actions"].includes(node.textContent)).map((node) => [node.textContent, node.dataset.selected]), [
  ["Execute Code", "true"],
  ["Update Record", "false"],
  ["Multi Actions", "false"]
]);
assert.equal(genericStatePills.some((node) => node.textContent === "Send WhatsApp"), true);
genericEditButton.dispatchEvent(new TestEvent("click"));
genericForm = genericFormWindow.children[1];
const genericNameInput = findAll(genericForm, (node) => node.tag === "input" && node.dataset?.field === "name")[0];
const genericEmailInput = findAll(genericForm, (node) => node.tag === "input" && node.dataset?.field === "email")[0];
const genericRelation = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-many2one-editor"))[0];
const genericRelationInput = findAll(genericRelation, (node) => node.tag === "input" && node.dataset?.field === "model_id")[0];
const genericStateRadio = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-selection-radio-group") && node.dataset?.field === "state")[0];
const genericCodeEditor = findAll(genericForm, (node) => node.tag === "textarea" && String(node.className ?? "").includes("gorp-code-editor") && node.dataset?.field === "code")[0];
const genericGroups = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-x2many-editor"))[0];
const genericGroupsInput = findAll(genericGroups, (node) => node.tag === "input" && node.dataset?.field === "group_ids")[0];
const genericLines = findAll(genericForm, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-one2many-editor"))[0];
const genericRelationToggle = findAll(genericRelation, (node) => String(node.className ?? "").includes("gorp-many2one-dropdown-toggle"))[0];
assert.equal(genericEditButton.hidden, true);
assert.equal(genericSaveButton.hidden, false);
assert.equal(genericSaveButton.disabled, true);
assert.equal(genericDiscardButton.hidden, false);
assert.equal(genericNameInput.required, true);
assert.equal(genericEmailInput.value, "old@example.com");
assert.equal(genericEmailInput.required, false);
assert.equal(genericStateRadio.dataset.value, "code");
assert.equal(findAll(genericStateRadio, (node) => node.tag === "input" && node.value === "code")[0].checked, true);
assert.equal(findAll(genericStateRadio, (node) => node.textContent === "Send WhatsApp").length, 1);
assert.equal(genericCodeEditor.value.includes("log('ok')"), true);
assert.equal(genericCodeEditor.rows, 14);
assert.equal(genericRelation.dataset.relation, "ir.model");
assert.equal(genericRelation.dataset.resId, "5");
assert.deepEqual(JSON.parse(genericRelation.dataset.searchDomain), [["transient", "=", false]]);
assert.deepEqual(JSON.parse(genericRelation.dataset.searchContext), { lang: "en_US", active_test: false });
assert.equal(genericRelation.dataset.searchLimit, "12");
assert.equal(genericRelation.dataset.noCreate, "true");
assert.equal(genericRelation.dataset.noQuickCreate, "true");
assert.equal(genericRelation.dataset.noCreateEdit, "true");
assert.equal(genericRelation.dataset.noOpen, "true");
assert.equal(genericRelationInput.attributes["aria-haspopup"], "listbox");
assert.equal(genericRelationToggle.attributes["aria-haspopup"], "listbox");
assert.equal(genericRelationToggle.attributes["aria-expanded"], "false");
assert.equal(genericGroups.dataset.relation, "res.groups");
assert.equal(genericGroups.dataset.count, "1");
assert.equal(genericGroups.dataset.mobileWidget, "many2many_tags");
assert.deepEqual(JSON.parse(genericGroups.dataset.searchDomain), [["share", "=", false]]);
assert.deepEqual(JSON.parse(genericGroups.dataset.searchContext), { lang: "en_US", active_test: false });
assert.equal(genericGroups.dataset.searchLimit, "15");
assert.equal(genericGroups.dataset.noCreate, "true");
assert.equal(genericGroups.attributes["aria-label"], "Allowed Groups");
assert.equal(genericGroupsInput.attributes["aria-label"], "Add Allowed Groups");
assert.equal(genericLines.dataset.relation, "ir.actions.server.line");
assert.equal(genericLines.dataset.count, "2");
assert.equal(genericLines.dataset.mobileWidget, "one2many_list");
assert.equal(genericLines.dataset.mobileLayout, "cards");
const genericLineCells = findAll(genericLines, (node) => node.tag === "td" && node.dataset?.field === "description");
assert.deepEqual(genericLineCells.map((node) => node.dataset.label), ["Description", "Description"]);
const genericLineOwner = findAll(genericLines, (node) => String(node.className ?? "").includes("gorp-one2many-many2one-editor") && node.dataset?.field === "owner_id")[0];
const genericLineOwnerInput = findAll(genericLineOwner, (node) => node.tag === "input" && node.dataset?.field === "owner_id")[0];
const genericLineOwnerToggle = findAll(genericLineOwner, (node) => String(node.className ?? "").includes("gorp-many2one-dropdown-toggle"))[0];
assert.equal(genericLineOwner.dataset.relation, "res.users");
assert.equal(genericLineOwner.dataset.resId, "7");
assert.deepEqual(JSON.parse(genericLineOwner.dataset.searchDomain), [["active", "=", true]]);
assert.deepEqual(JSON.parse(genericLineOwner.dataset.searchContext), { lang: "en_US", active_test: false });
assert.equal(genericLineOwner.dataset.searchLimit, "5");
assert.equal(genericLineOwner.dataset.noCreate, "true");
assert.equal(genericLineOwnerInput.value, "Administrator");
assert.equal(genericLineOwnerInput.attributes["role"], "combobox");
assert.equal(genericLineOwnerToggle.attributes["aria-haspopup"], "listbox");
genericNameInput.value = "Send Follow-up";
genericNameInput.dispatchEvent(new TestEvent("input"));
genericEmailInput.value = "follow@example.com";
genericEmailInput.dispatchEvent(new TestEvent("input"));
genericRelationToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(genericFormSearchCalls[0], {
  model: "ir.model",
  method: "name_search",
  args: [],
  kwargs: { name: "", domain: [["transient", "=", false]], operator: "ilike", limit: 12, context: { lang: "en_US", active_test: false } }
});
assert.equal(genericRelationToggle.attributes["aria-expanded"], "true");
assert.equal(genericRelationInput.attributes["aria-expanded"], "true");
assert.equal(genericRelation.dataset.resId, "5");
let genericRelationOptions = findAll(genericRelation, (node) => String(node.className ?? "").includes("gorp-many2one-option"));
assert.deepEqual(genericRelationOptions.map((node) => node.textContent), ["Contact", "Mail", "Message"]);
assert.equal(genericRelationOptions[0].dataset.selected, "true");
assert.equal(genericRelationOptions[0].dataset.active, "true");
assert.equal(genericRelationInput.attributes["aria-activedescendant"], genericRelationOptions[0].id);
genericRelationInput.dispatchEvent(new TestEvent("keydown", { key: "ArrowDown" }));
assert.equal(genericRelationOptions[1].dataset.active, "true");
genericRelationInput.dispatchEvent(new TestEvent("keydown", { key: "Enter" }));
assert.equal(genericRelation.dataset.resId, "81");
assert.equal(genericRelationInput.value, "Mail");
genericRelationInput.value = "mail";
genericRelationInput.dispatchEvent(new TestEvent("input"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(genericFormSearchCalls[1], {
  model: "ir.model",
  method: "name_search",
  args: [],
  kwargs: { name: "mail", domain: [["transient", "=", false]], operator: "ilike", limit: 12, context: { lang: "en_US", active_test: false } }
});
genericRelationOptions = findAll(genericRelation, (node) => String(node.className ?? "").includes("gorp-many2one-option"));
assert.deepEqual(genericRelationOptions.map((node) => node.textContent), ["Mail", "Message"]);
assert.equal(findAll(genericRelation, (node) => String(node.className ?? "").includes("gorp-many2one-create")).length, 0);
assert.equal(findAll(genericRelation, (node) => String(node.className ?? "").includes("gorp-many2one-create-edit")).length, 0);
genericRelationOptions[0].dispatchEvent(new TestEvent("click"));
assert.equal(genericRelation.dataset.resId, "81");
assert.equal(genericRelationInput.value, "Mail");
const genericInitialGroupTags = findAll(genericGroups, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-editor-tag"));
assert.deepEqual(findAll(genericInitialGroupTags[0], (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-editor-label")).map((node) => node.textContent), ["Base / User"]);
genericGroupsInput.value = "sales";
genericGroupsInput.dispatchEvent(new TestEvent("input"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(genericFormSearchCalls[2], {
  model: "res.groups",
  method: "name_search",
  args: [],
  kwargs: { name: "sales", domain: [["share", "=", false]], operator: "ilike", limit: 15, context: { lang: "en_US", active_test: false } }
});
const genericGroupOptions = findAll(genericGroups, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-option"));
assert.deepEqual(genericGroupOptions.map((node) => node.textContent), ["Sales / Manager"]);
assert.equal(findAll(genericGroups, (node) => String(node.className ?? "").includes("gorp-x2many-create")).length, 0);
genericGroupOptions[0].dispatchEvent(new TestEvent("click"));
assert.equal(genericGroups.dataset.count, "2");
const genericGroupRemove = findAll(genericGroups, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-editor-remove") && node.dataset?.resId === "11")[0];
genericGroupRemove.dispatchEvent(new TestEvent("click"));
assert.equal(genericGroups.dataset.count, "1");
genericLineOwnerToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(genericFormSearchCalls[3], {
  model: "res.users",
  method: "name_search",
  args: [],
  kwargs: { name: "", domain: [["active", "=", true]], operator: "ilike", limit: 5, context: { lang: "en_US", active_test: false } }
});
assert.equal(genericLineOwnerToggle.attributes["aria-expanded"], "true");
assert.equal(genericLineOwnerInput.attributes["aria-expanded"], "true");
const genericLineOwnerOptions = findAll(genericLineOwner, (node) => String(node.className ?? "").includes("gorp-many2one-option"));
assert.deepEqual(genericLineOwnerOptions.map((node) => node.textContent), ["Administrator", "Demo User"]);
genericLineOwnerOptions[1].dispatchEvent(new TestEvent("click"));
assert.equal(genericLineOwner.dataset.resId, "8");
assert.equal(genericLineOwnerInput.value, "Demo User");
let genericLineDescriptionInputs = findAll(genericLines, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-one2many-input") && node.dataset?.field === "description");
let genericLineQuantityInputs = findAll(genericLines, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-one2many-input") && node.dataset?.field === "quantity");
assert.deepEqual(genericLineDescriptionInputs.map((node) => node.value), ["Old line", "Drop line"]);
genericLineDescriptionInputs[0].value = "Updated line";
genericLineDescriptionInputs[0].dispatchEvent(new TestEvent("input"));
genericLineQuantityInputs[0].value = "5";
genericLineQuantityInputs[0].dispatchEvent(new TestEvent("input"));
const genericLineRemove = findAll(genericLines, (node) => node.dataset?.one2manyAction === "remove")[1];
genericLineRemove.dispatchEvent(new TestEvent("click"));
assert.equal(genericLines.dataset.count, "1");
const genericLineAdd = findAll(genericLines, (node) => node.dataset?.one2manyAction === "add")[0];
genericLineAdd.dispatchEvent(new TestEvent("click"));
assert.equal(genericLines.dataset.count, "2");
genericLineDescriptionInputs = findAll(genericLines, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-one2many-input") && node.dataset?.field === "description");
genericLineQuantityInputs = findAll(genericLines, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-one2many-input") && node.dataset?.field === "quantity");
genericLineDescriptionInputs.at(-1).value = "New line";
genericLineDescriptionInputs.at(-1).dispatchEvent(new TestEvent("input"));
genericLineQuantityInputs.at(-1).value = "3";
genericLineQuantityInputs.at(-1).dispatchEvent(new TestEvent("input"));
genericSaveButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(genericFormSaveCalls, [{
  model: "ir.actions.server",
  ids: [24],
  changes: {
    name: "Send Follow-up",
    email: "follow@example.com",
    model_id: 81,
    group_ids: [[6, false, [30]]],
    line_ids: [
      [1, 201, { description: "Updated line", owner_id: 8, quantity: "5" }],
      [3, 202, false],
      [0, false, { description: "New line", owner_id: false, quantity: "3" }]
    ]
  },
  kwargs: { context: { lang: "en_US" } }
}]);
assert.equal(genericFormSaveEvents.length, 1);
assert.deepEqual(genericFormSaveEvents[0].changes, genericFormSaveCalls[0].changes);
assert.equal(genericEditButton.hidden, false);
assert.equal(genericSaveButton.hidden, true);
genericForm = genericFormWindow.children[1];
const genericSavedRelationValue = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-many2one-value") && node.dataset?.field === "model_id")[0];
assert.equal(genericSavedRelationValue.tag, "output");
assert.equal(genericSavedRelationValue.textContent, "Mail");
assert.equal(genericSavedRelationValue.dataset.resId, "81");
assert.equal(genericSavedRelationValue.dataset.noOpen, "true");
const genericSavedGroups = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-x2many-tags") && node.dataset?.field === "group_ids")[0];
assert.equal(genericSavedGroups.dataset.count, "1");
assert.deepEqual(findAll(genericSavedGroups, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-tag")).map((node) => node.textContent), ["Sales / Manager"]);
const genericSavedLines = findAll(genericForm, (node) => String(node.className ?? "").includes("gorp-x2many-tags") && node.dataset?.field === "line_ids")[0];
assert.equal(genericSavedLines.dataset.count, "2");
assert.deepEqual(findAll(genericSavedLines, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-tag")).map((node) => node.textContent), ["Updated line", "New line"]);
genericEditButton.dispatchEvent(new TestEvent("click"));
genericForm = genericFormWindow.children[1];
const genericDiscardedName = findAll(genericForm, (node) => node.tag === "input" && node.dataset?.field === "name")[0];
genericDiscardedName.value = "Discarded";
genericDiscardedName.dispatchEvent(new TestEvent("input"));
assert.equal(genericDiscardButton.disabled, false);
genericDiscardButton.dispatchEvent(new TestEvent("click"));
genericForm = genericFormWindow.children[1];
const genericRestoredName = findAll(genericForm, (node) => node.tag === "input" && node.dataset?.field === "name")[0];
assert.equal(genericRestoredName.value, "Send Follow-up");
assert.equal(genericFormSaveCalls.length, 1);
assert.equal(genericFormDiscardEvents.length, 1);

const lockedOne2ManyWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Locked Lines" },
  activeView: "form",
  resModel: "x.locked.parent",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      line_ids: { type: "one2many", relation: "x.locked.line", string: "Lines" }
    },
    relatedModels: {
      "x.locked.line": {
        fields: {
          description: { type: "char", string: "Description" },
          owner_id: { type: "many2one", relation: "res.users", string: "Owner" }
        }
      }
    },
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="line_ids" create="false"><list delete="false" editable="false" open_form_view="1"><field name="description"/><field name="owner_id"/></list></field></sheet></form>`,
        id: 810
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 31,
    name: "Parent",
    line_ids: [
      { id: 401, description: "Locked", owner_id: [7, "Administrator"] }
    ]
  }
});
findAll(lockedOne2ManyWindow, (node) => node.dataset?.formAction === "edit")[0].dispatchEvent(new TestEvent("click"));
const lockedOne2Many = findAll(lockedOne2ManyWindow, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-one2many-editor"))[0];
assert.equal(lockedOne2Many.dataset.canCreate, "false");
assert.equal(lockedOne2Many.dataset.canDelete, "false");
assert.equal(lockedOne2Many.dataset.inlineEditable, "false");
assert.equal(lockedOne2Many.dataset.openFormView, "true");
assert.equal(findAll(lockedOne2Many, (node) => node.dataset?.one2manyAction === "add").length, 0);
assert.equal(findAll(lockedOne2Many, (node) => node.dataset?.one2manyAction === "remove").length, 0);
assert.equal(findAll(lockedOne2Many, (node) => String(node.className ?? "").includes("gorp-one2many-actions-head")).length, 0);
assert.deepEqual(findAll(lockedOne2Many, (node) => String(node.className ?? "").includes("gorp-one2many-readonly")).map((node) => node.textContent), ["Locked", "Administrator"]);
assert.equal(findAll(lockedOne2Many, (node) => String(node.className ?? "").includes("gorp-one2many-input")).length, 0);

const openOne2ManyActions = [];
const openOne2ManyEvents = [];
const openOne2ManyWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Open Lines" },
  activeView: "form",
  resModel: "x.open.parent",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      line_ids: { type: "one2many", relation: "x.open.line", string: "Lines" }
    },
    relatedModels: {
      "x.open.line": {
        fields: {
          description: { type: "char", string: "Description" },
          owner_id: { type: "many2one", relation: "res.users", string: "Owner" }
        }
      }
    },
    views: {
      form: {
        arch: `<form><sheet><field name="line_ids"><list delete="false" editable="bottom" open_form_view="1"><field name="description"/><field name="owner_id"/></list></field></sheet></form>`,
        id: 811
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 32,
    name: "Parent",
    line_ids: [
      { id: 402, description: "Open me", owner_id: [8, "Demo User"] }
    ]
  },
  context: { lang: "en_US" },
  services: {
    action: {
      doAction(action, options) {
        openOne2ManyActions.push({ action, options });
        return Promise.resolve(action);
      }
    }
  }
});
findAll(openOne2ManyWindow, (node) => node.dataset?.formAction === "edit")[0].dispatchEvent(new TestEvent("click"));
const openOne2Many = findAll(openOne2ManyWindow, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-one2many-editor"))[0];
openOne2Many.addEventListener("one2many:open-form", (event) => openOne2ManyEvents.push(event.detail));
assert.equal(openOne2Many.dataset.canCreate, "true");
assert.equal(openOne2Many.dataset.canDelete, "false");
assert.equal(openOne2Many.dataset.inlineEditable, "true");
assert.equal(openOne2Many.dataset.openFormView, "true");
const openOne2ManyButton = findAll(openOne2Many, (node) => node.dataset?.one2manyAction === "open")[0];
assert.equal(openOne2ManyButton.dataset.resId, "402");
assert.equal(findAll(openOne2Many, (node) => node.dataset?.one2manyAction === "remove").length, 0);
openOne2ManyButton.dispatchEvent(new TestEvent("click"));
assert.equal(openOne2ManyActions.length, 1);
assert.deepEqual(openOne2ManyActions[0].action, {
  type: "ir.actions.act_window",
  name: "x.open.line",
  res_model: "x.open.line",
  views: [[false, "form"]],
  view_mode: "form",
  target: "new",
  context: { lang: "en_US" },
  res_id: 402
});
assert.equal(openOne2ManyEvents.length, 1);
assert.equal(openOne2ManyEvents[0].relation, "x.open.line");
assert.equal(openOne2ManyEvents[0].id, 402);

const cronWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Scheduled Actions" },
  activeView: "form",
  resModel: "ir.cron",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "name" },
      active: { type: "boolean", string: "active" },
      interval_number: { type: "integer", string: "interval_number" },
      interval_type: { type: "selection", string: "interval_type" },
      nextcall: { type: "datetime", string: "nextcall" },
      user_id: { type: "many2one", relation: "res.users", string: "user_id" },
      state: { type: "selection", string: "state" },
      code: { type: "text", string: "code" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="active"/><field name="interval_number"/><field name="interval_type"/><field name="nextcall"/><field name="user_id"/><field name="state"/><field name="code"/></sheet></form>`,
        id: 75
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 41,
    display_name: "Mail: Email Queue Manager",
    name: "Mail: Email Queue Manager",
    active: true,
    interval_number: 4,
    interval_type: "hours",
    nextcall: "2026-06-22 12:00:00",
    user_id: [1, "Administrator"],
    state: "code",
    code: "model._process_queue()"
  }
});
let cronForm = cronWindow.children[1];
const cronBand = findAll(cronForm, (node) => String(node.className ?? "").includes("gorp-scheduled-action-band"))[0];
assert.equal(cronBand.dataset.model, "ir.cron");
assert.equal(cronBand.dataset.state, "code");
assert.equal(findAll(cronBand, (node) => String(node.className ?? "").includes("gorp-server-action-badge"))[0].textContent, "Scheduled Action");
assert.equal(findAll(cronBand, (node) => String(node.className ?? "").includes("gorp-server-action-state"))[0].textContent, "Execute Code");
assert.equal(findAll(cronBand, (node) => String(node.className ?? "").includes("gorp-server-action-meta-value"))[0].textContent, "4 Hours");
assert.deepEqual(findAll(cronForm, (node) => String(node.className ?? "").split(/\s+/).includes("o_form_label")).map((node) => node.textContent).slice(0, 7), [
  "Name",
  "Active",
  "Repeat Every",
  "Interval Unit",
  "Next Execution Date",
  "Run As",
  "Action Type"
]);
const cronNotebook = findAll(cronForm, (node) => String(node.className ?? "").includes("gorp-scheduled-action-notebook"))[0];
assert.deepEqual(findAll(cronNotebook, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-form-notebook-tab")).map((node) => node.textContent), ["Code", "Help"]);
assert.equal(findAll(cronNotebook, (node) => String(node.className ?? "").includes("gorp-code-viewer") && node.dataset?.field === "code").length, 1);
const cronEditButton = findAll(cronWindow, (node) => node.dataset?.formAction === "edit")[0];
cronEditButton.dispatchEvent(new TestEvent("click"));
cronForm = cronWindow.children[1];
const cronStateRadio = findAll(cronForm, (node) => String(node.className ?? "").includes("gorp-selection-radio-group") && node.dataset?.field === "state")[0];
const cronIntervalRadio = findAll(cronForm, (node) => String(node.className ?? "").includes("gorp-selection-radio-group") && node.dataset?.field === "interval_type")[0];
const cronCodeEditor = findAll(cronForm, (node) => node.tag === "textarea" && String(node.className ?? "").includes("gorp-code-editor") && node.dataset?.field === "code")[0];
assert.equal(cronStateRadio.dataset.value, "code");
assert.deepEqual(findAll(cronIntervalRadio, (node) => node.tag === "input").map((node) => [node.value, node.checked]), [
  ["minutes", false],
  ["hours", true],
  ["days", false],
  ["weeks", false],
  ["months", false]
]);
assert.equal(cronCodeEditor.value, "model._process_queue()");

const automationWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Automation Rules" },
  activeView: "form",
  resModel: "base.automation",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "name" },
      active: { type: "boolean", string: "active" },
      model_id: { type: "many2one", relation: "ir.model", string: "model_id" },
      model_name: { type: "char", string: "model_name" },
      trigger: { type: "selection", string: "trigger" },
      action_server_id: { type: "many2one", relation: "ir.actions.server", string: "action_server_id" },
      description: { type: "text", string: "description" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="active"/><field name="model_id"/><field name="model_name"/><field name="trigger"/><field name="action_server_id"/><field name="description"/></sheet></form>`,
        id: 76
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 42,
    display_name: "Post Message",
    name: "Post Message",
    active: true,
    model_id: [9, "Mail"],
    model_name: "mail.mail",
    trigger: "create_or_write",
    action_server_id: [15, "Send Follow-up"],
    description: "Run after mail changes"
  }
});
let automationForm = automationWindow.children[1];
const automationBand = findAll(automationForm, (node) => String(node.className ?? "").includes("gorp-automation-action-band"))[0];
assert.equal(automationBand.dataset.model, "base.automation");
assert.equal(automationBand.dataset.trigger, "create_or_write");
assert.equal(findAll(automationBand, (node) => String(node.className ?? "").includes("gorp-server-action-badge"))[0].textContent, "Automation Rule");
assert.equal(findAll(automationBand, (node) => String(node.className ?? "").includes("gorp-server-action-state"))[0].textContent, "On Creation & Update");
assert.deepEqual(findAll(automationForm, (node) => String(node.className ?? "").split(/\s+/).includes("o_form_label")).map((node) => node.textContent).slice(0, 7), [
  "Name",
  "Active",
  "Model",
  "Model",
  "Trigger",
  "Server Action",
  "Description"
]);
const automationTriggerPills = findAll(automationForm, (node) => String(node.className ?? "").includes("gorp-selection-pills") && node.dataset?.field === "trigger")[0];
assert.equal(automationTriggerPills.dataset.value, "create_or_write");
assert.equal(findAll(automationTriggerPills, (node) => node.textContent === "On Creation & Update")[0].dataset.selected, "true");
const automationEditButton = findAll(automationWindow, (node) => node.dataset?.formAction === "edit")[0];
automationEditButton.dispatchEvent(new TestEvent("click"));
automationForm = automationWindow.children[1];
const automationTriggerRadio = findAll(automationForm, (node) => String(node.className ?? "").includes("gorp-selection-radio-group") && node.dataset?.field === "trigger")[0];
assert.equal(automationTriggerRadio.dataset.value, "create_or_write");
assert.equal(findAll(automationTriggerRadio, (node) => node.tag === "input" && node.value === "manual")[0].checked, false);
assert.equal(findAll(automationTriggerRadio, (node) => node.textContent === "Based on Timed Condition").length, 1);

const serverActionListWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Server Actions" },
  activeView: "list",
  resModel: "ir.actions.server",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "name" },
      model_id: { type: "many2one", relation: "ir.model", string: "model_id" },
      state: { type: "selection", string: "state" },
      model_name: { type: "char", string: "model_name" },
      usage: { type: "selection", string: "usage" },
      active: { type: "boolean", string: "active" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="state"/><field name="model_name"/><field name="active"/></list>`,
        id: 74
      }
    }
  },
  records: [
    { id: 31, name: "Mail: Email Queue Manager", model_id: [12, "Mail"], state: "code", model_name: "mail.mail", usage: "ir_cron", active: true },
    { id: 32, name: "Delegation Approval", model_id: [13, "Delegation"], state: "code", model_name: "delegation", usage: "ir_actions_server", active: true }
  ],
  length: 2
});
const serverActionListTable = findAll(serverActionListWindow, (node) => String(node.className ?? "").includes("gorp-list-view"))[0];
assert.deepEqual(findAll(serverActionListTable, (node) => String(node.className ?? "").includes("o_list_header_button")).map((node) => node.textContent), ["Name", "Model", "Type", "Usage"]);
const serverActionModelCellTexts = findAll(serverActionListTable, (node) => node.tag === "td" && (node.dataset?.field === "model_name" || node.dataset?.field === "model_id"))
  .flatMap((node) => findAll(node, (child) => child.tag === "output" || String(child.className ?? "").includes("gorp-many2one-link")).map((child) => child.textContent))
  .filter(Boolean);
assert.ok(serverActionModelCellTexts.includes("Mail"));
assert.ok(serverActionModelCellTexts.includes("Delegation"));
assert.ok(!serverActionModelCellTexts.includes("mail.mail"));
assert.ok(!serverActionModelCellTexts.includes("delegation"));
const serverActionStateCell = findAll(serverActionListTable, (node) => node.dataset?.field === "state")[0];
assert.equal(findAll(serverActionStateCell, (node) => node.tag === "output")[0].textContent, "Execute Code");
const serverActionUsageCell = findAll(serverActionListTable, (node) => node.dataset?.field === "usage")[0];
assert.equal(findAll(serverActionUsageCell, (node) => node.tag === "output")[0].textContent, "Scheduled Action");
const serverActionCustomFilterCalls = [];
const serverActionCustomFilterWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { id: 74, name: "Server Actions" },
  activeView: "list",
  resModel: "ir.actions.server",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "name" },
      state: { type: "selection", string: "state" },
      model_name: { type: "char", string: "model_name" },
      model_id: { type: "many2one", relation: "ir.model", string: "model_id" },
      active: { type: "boolean", string: "active" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="state"/><field name="model_name"/><field name="active"/></list>`,
        id: 74
      }
    }
  },
  records: [{ id: 31, name: "Mail: Email Queue Manager", state: "code", model_name: "mail.mail", active: true }],
  length: 1,
  search: {
    state: { query: "", facets: [], domain: [], context: {}, groupBy: [] },
    suggestions: [],
    filters: [],
    groupBys: [],
    favorites: []
  }
}, {
  services: {
    action: {
      doAction(action, options) {
        serverActionCustomFilterCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
findAll(serverActionCustomFilterWindow, (node) => String(node.className ?? "").includes("o_searchview_dropdown_toggler"))[0].dispatchEvent(new TestEvent("click"));
findAll(serverActionCustomFilterWindow, (node) => String(node.className ?? "").includes("o_add_custom_filter"))[0].dispatchEvent(new TestEvent("click"));
const customFilterDialog = findAll(serverActionCustomFilterWindow, (node) => String(node.className ?? "").includes("gorp-custom-filter-dialog"))[0];
assert.equal(customFilterDialog.attributes.role, "dialog");
const customFilterField = findAll(customFilterDialog, (node) => node.dataset?.customFilterField === "true")[0];
const customFilterOperator = findAll(customFilterDialog, (node) => node.dataset?.customFilterOperator === "true")[0];
const customFilterValue = findAll(customFilterDialog, (node) => node.dataset?.customFilterValue === "true")[0];
assert.equal(customFilterField.value, "model_name");
assert.equal(findAll(customFilterField, (node) => node.tag === "option").map((node) => node.textContent).includes("Model"), true);
customFilterOperator.value = "ilike";
customFilterValue.value = "mail";
findAll(customFilterDialog, (node) => node.dataset?.customFilterApply === "true")[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(findAll(serverActionCustomFilterWindow, (node) => String(node.className ?? "").includes("gorp-custom-filter-dialog")).length, 0);
assert.deepEqual(serverActionCustomFilterCalls[0].action.__search_facets.map((facet) => [
  facet.id,
  facet.type,
  facet.field,
  facet.operator,
  facet.categoryLabel,
  facet.valueLabels,
  facet.value
]), [
  ["custom-model_name-ilike-mail", "text", "model_name", "ilike", "Model", ["mail"], "mail"]
]);
assert.equal("__search_query" in serverActionCustomFilterCalls[0].action, false);
assert.deepEqual(serverActionCustomFilterCalls[0].options, { additionalContext: {}, replaceLastAction: true });

const x2ManyOpenCalls = [];
const x2ManyFormWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Group" },
  activeView: "form",
  resModel: "res.groups",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      inherited_by_ids: { type: "many2many", relation: "res.groups", string: "Inherited By" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="inherited_by_ids"/></sheet></form>`,
        id: 72
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 3,
    display_name: "Sales / User",
    name: "Sales / User",
    inherited_by_ids: [
      [11, "Sales / Manager"],
      { id: 30, display_name: "Export Reports" },
      [11, "Sales / Manager"],
      [x2ManyCommands.LINK, 40],
      [x2ManyCommands.UPDATE, 60, { display_name: "Updated Role" }],
      [x2ManyCommands.CREATE, false, { display_name: "Transient Role" }]
    ]
  },
  context: { lang: "en_US" },
  services: {
    action: {
      doAction(action, options) {
        x2ManyOpenCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const x2ManyTags = findAll(x2ManyFormWindow, (node) => String(node.className ?? "").includes("gorp-x2many-tags"))[0];
assert.equal(x2ManyTags.dataset.field, "inherited_by_ids");
assert.equal(x2ManyTags.dataset.fieldType, "many2many");
assert.equal(x2ManyTags.dataset.relation, "res.groups");
assert.equal(x2ManyTags.dataset.count, "5");
assert.ok(String(x2ManyTags.className).includes("o_field_many2many_tags"));
const x2ManyItems = findAll(x2ManyTags, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-tag"));
assert.deepEqual(x2ManyItems.map((node) => node.textContent), ["Sales / Manager", "Export Reports", "40", "Updated Role", "Transient Role"]);
assert.deepEqual(x2ManyItems.map((node) => node.dataset.resId), ["11", "30", "40", "60", undefined]);
assert.equal(x2ManyItems[0].tag, "a");
assert.equal(x2ManyItems[4].tag, "span");
assert.equal(x2ManyItems[4].href, undefined);
x2ManyItems[4].dispatchEvent(new TestEvent("click"));
assert.equal(x2ManyOpenCalls.length, 0);
x2ManyItems[0].dispatchEvent(new TestEvent("click"));
assert.equal(x2ManyOpenCalls.length, 1);
assert.deepEqual(x2ManyOpenCalls[0].action, {
  type: "ir.actions.act_window",
  name: "Sales / Manager",
  res_model: "res.groups",
  res_id: 11,
  views: [[false, "form"]],
  view_mode: "form",
  target: "current"
});
assert.deepEqual(x2ManyOpenCalls[0].options, { additionalContext: { lang: "en_US" }, replaceLastAction: true });

const groupsInheritedByFallbackWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Groups" },
  activeView: "form",
  resModel: "res.groups",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      implied_ids: { type: "many2many", relation: "res.groups", string: "Inherited" },
      inherited_by_ids: { type: "many2many", relation: "res.groups", string: "Inherited By" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><group><field name="name"/></group><notebook><page string="Inherited"><field name="implied_ids"/></page></notebook></sheet></form>`,
        id: 73
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 3,
    name: "Settings",
    implied_ids: [[7, "User"]],
    inherited_by_ids: [[11, "Administrator"]]
  }
});
const groupsNotebookTabs = findAll(groupsInheritedByFallbackWindow, (node) => String(node.className ?? "").includes("gorp-form-notebook-tab") && node.attributes?.role === "tab");
assert.deepEqual(groupsNotebookTabs.map((node) => node.textContent), ["Inherited", "Inherited By"]);
const groupsInheritedByTags = findAll(groupsInheritedByFallbackWindow, (node) => String(node.className ?? "").includes("gorp-x2many-tags") && node.dataset?.field === "inherited_by_ids")[0];
assert.equal(groupsInheritedByTags.dataset.relation, "res.groups");
assert.equal(groupsInheritedByTags.dataset.count, "1");
assert.deepEqual(findAll(groupsInheritedByTags, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-tag")).map((node) => node.textContent), ["Administrator"]);

function renderX2ManyOnlyWindow(fieldType, value) {
  return renderWindowAction({
    type: "ir.actions.act_window",
    action: { name: "Relation" },
    activeView: "form",
    resModel: "x.parent",
    viewDescriptions: {
      fields: {
        rel_ids: { type: fieldType, relation: "res.groups", string: "Relations" }
      },
      relatedModels: {},
      views: {
        form: {
          arch: `<form><sheet><field name="rel_ids"/></sheet></form>`,
          id: 73
        }
      }
    },
    records: [],
    length: 0
  }, {
    values: { id: 1, display_name: "Parent", rel_ids: value }
  });
}

function x2ManyOnlyTags(fieldType, value) {
  const window = renderX2ManyOnlyWindow(fieldType, value);
  const tags = findAll(window, (node) => String(node.className ?? "").includes("gorp-x2many-tags"))[0];
  const items = findAll(tags, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-tag"));
  return { tags, items };
}

let commandTags = x2ManyOnlyTags("many2many", [[x2ManyCommands.LINK, 11], [x2ManyCommands.UNLINK, 11]]);
assert.equal(commandTags.tags.dataset.count, "0");
assert.deepEqual(commandTags.items.map((node) => node.textContent), []);

commandTags = x2ManyOnlyTags("many2many", [[x2ManyCommands.SET, false, [11, 12]], [x2ManyCommands.CLEAR]]);
assert.equal(commandTags.tags.dataset.count, "0");
assert.deepEqual(commandTags.items.map((node) => node.textContent), []);

commandTags = x2ManyOnlyTags("many2many", [[x2ManyCommands.LINK, 30], [x2ManyCommands.UPDATE, 30, { display_name: "Updated" }]]);
assert.equal(commandTags.tags.dataset.count, "1");
assert.deepEqual(commandTags.items.map((node) => node.textContent), ["Updated"]);
assert.deepEqual(commandTags.items.map((node) => node.dataset.resId), ["30"]);

const one2ManyTags = x2ManyOnlyTags("one2many", [[1, "Line A"], { id: 2, display_name: "Line B" }, 3, [1, "Line A"], false]);
assert.equal(one2ManyTags.tags.dataset.fieldType, "one2many");
assert.equal(one2ManyTags.tags.dataset.count, "3");
assert.ok(String(one2ManyTags.tags.className).includes("o_field_one2many"));
assert.deepEqual(one2ManyTags.items.map((node) => node.textContent), ["Line A", "Line B", "3"]);

const groupedRequestStart = windowActionRequests.length;
const groupedWindowResult = await actionServices.action.doAction({
  ...windowResult.action,
  __search_facets: [
    { id: "group-by-group_company", type: "groupBy", label: "Company", field: "company_id" }
  ]
});
const groupedSearchRead = windowActionRequests.slice(groupedRequestStart).find((request) => request.route === "/web/dataset/call_kw/res.partner/web_search_read");
assert.deepEqual(groupedSearchRead.params.kwargs.groupby, ["company_id"]);
assert.deepEqual(groupedWindowResult.search.state.facets.map((facet) => facet.id), ["group-by-group_company"]);

const searchFieldRequestStart = windowActionRequests.length;
await actionServices.action.doAction({
  ...windowResult.action,
  context: {},
  domain: [],
  __search_query: "Azure",
  __search_facets: []
});
const searchFieldRead = windowActionRequests.slice(searchFieldRequestStart).find((request) => request.route === "/web/dataset/call_kw/res.partner/web_search_read");
assert.deepEqual(searchFieldRead.params.kwargs.domain, [
  ["|", ["name", "ilike", "Azure"], ["company_id", "ilike", "Azure"]]
]);
const dialogCloseEvents = [];
const renderedDialog = renderWindowActionDialog({
  ...windowResult,
  action: { ...windowResult.action, name: "Partner Wizard", target: "new" }
});
renderedDialog.addEventListener("dialog:close", (event) => dialogCloseEvents.push(event.detail));
assert.equal(renderedDialog.dataset.target, "new");
assert.equal(renderedDialog.dataset.dialogOpen, "true");
assert.equal(renderedDialog.dataset.model, "res.partner");
assert.equal(String(renderedDialog.className).split(/\s+/).includes("o_dialog"), true);
assert.equal(String(renderedDialog.className).split(/\s+/).includes("modal-open"), true);
assert.equal(findAll(renderedDialog, (node) => String(node.className).includes("gorp-action-dialog-backdrop") && node.attributes["aria-hidden"] === "true").length, 1);
assert.equal(findAll(renderedDialog, (node) => String(node.className).includes("modal o_dialog_container")).length, 1);
assert.equal(findAll(renderedDialog, (node) => String(node.className).includes("modal-dialog")).length, 1);
assert.equal(findAll(renderedDialog, (node) => String(node.className).includes("modal-body") && String(node.className).includes("o_act_window")).length, 1);
assert.equal(findAll(renderedDialog, (node) => String(node.className).split(/\s+/).includes("gorp-window-action")).length, 2);
assert.equal(findAll(renderedDialog, (node) => String(node.className).includes("gorp-dialog-window-action")).length, 1);
assert.equal(findAll(renderedDialog, (node) => String(node.className).split(/\s+/).includes("o_control_panel")).length, 1);
assert.equal(findAll(renderedDialog, (node) => String(node.className).includes("modal-title"))[0].textContent, "Partner Wizard");
findAll(renderedDialog, (node) => String(node.className).includes("btn-close"))[0].dispatchEvent(new TestEvent("click"));
assert.equal(renderedDialog.dataset.dialogOpen, "false");
assert.deepEqual(dialogCloseEvents, [{ model: "res.partner" }]);

const formDialog = renderWindowActionDialog({
  ...windowResult,
  activeView: "form",
  action: { ...windowResult.action, name: "Partner Wizard Form", target: "new" },
  viewDescriptions: {
    ...windowResult.viewDescriptions,
    views: {
      ...windowResult.viewDescriptions.views,
      form: {
        arch: `<form><sheet><field name="name"/></sheet><footer><button name="action_confirm" string="Confirm" type="object"/></footer></form>`,
        id: 91
      }
    }
  },
  records: [{ id: 42, name: "Azure Interior", display_name: "Azure Interior" }]
});
const formDialogFooter = findAll(formDialog, (node) => String(node.className).includes("gorp-action-dialog-footer"))[0];
assert.ok(formDialogFooter);
assert.equal(findAll(formDialog, (node) => String(node.className).split(/\s+/).includes("o_control_panel")).length, 0);
assert.equal(findAll(formDialogFooter, (node) => node.dataset?.formAction === "edit").length, 0);
assert.equal(findAll(formDialogFooter, (node) => node.dataset?.formAction === "save").length, 1);
assert.equal(findAll(formDialogFooter, (node) => node.dataset?.formAction === "discard").length, 1);
assert.equal(findAll(formDialogFooter, (node) => node.dataset?.workflowAction === "action_confirm").length, 1);
assert.equal(findAll(formDialog, (node) => String(node.className).includes("gorp-form-header") && node.textContent.includes("Confirm")).length, 0);
const requiredDialogCalls = [];
const requiredDialogValues = { id: 43, name: "" };
const requiredDialog = renderWindowActionDialog({
  ...windowResult,
  activeView: "form",
  action: { ...windowResult.action, name: "Required Footer Dialog", target: "new" },
  viewDescriptions: {
    ...windowResult.viewDescriptions,
    fields: {
      ...windowResult.viewDescriptions.fields,
      name: { type: "char", string: "Name", required: true }
    },
    views: {
      ...windowResult.viewDescriptions.views,
      form: {
        arch: `<form><sheet><field name="name"/></sheet><footer><button name="action_confirm" string="Confirm" type="object"/></footer></form>`,
        id: 92
      }
    }
  },
  records: [requiredDialogValues]
}, {
  services: {
    dataset: {
      callButton(model, method, args, kwargs) {
        requiredDialogCalls.push({ model, method, args, kwargs });
        return Promise.resolve(true);
      }
    }
  }
});
const requiredDialogFooter = findAll(requiredDialog, (node) => String(node.className).includes("gorp-action-dialog-footer"))[0];
const requiredDialogName = findAll(requiredDialog, (node) => node.dataset?.requiredField === "name")[0];
const requiredDialogConfirm = findAll(requiredDialogFooter, (node) => node.dataset?.workflowAction === "action_confirm")[0];
assert.equal(requiredDialogName.required, true);
assert.equal(requiredDialogName.getAttribute("aria-invalid"), "false");
requiredDialogConfirm.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(requiredDialogCalls, []);
assert.equal(requiredDialogName.getAttribute("aria-invalid"), "true");
assert.equal(requiredDialogName.focused, true);
requiredDialogName.value = "Ready";
requiredDialogName.dispatchEvent(new TestEvent("input"));
requiredDialogConfirm.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(requiredDialogCalls, [{ model: "res.partner", method: "action_confirm", args: [[43]], kwargs: {} }]);
const createActionCalls = [];
const createActionWindow = renderWindowAction(windowResult, {
  context: { active_id: 42 },
  services: {
    action: {
      doAction(action, options) {
        createActionCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const newRecordButton = findAll(createActionWindow, (node) => node.dataset?.createAction === "true")[0];
assert.ok(newRecordButton.className.includes("o_list_button_add"));
assert.equal(newRecordButton.textContent, "New");
newRecordButton.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(createActionCalls.length, 1);
assert.deepEqual(createActionCalls[0].action.views, [[false, "form"]]);
assert.equal(createActionCalls[0].action.view_mode, "form");
assert.equal("res_id" in createActionCalls[0].action, false);
assert.deepEqual(createActionCalls[0].options, { additionalContext: { active_id: 42 }, replaceLastAction: true });

const implicitSearchRequests = [];
const implicitSearchServices = createWebClientServices({
  transport(request) {
    implicitSearchRequests.push(request);
    if (request.route === "/web/action/load") {
      return Promise.resolve({
        id: 11,
        type: "ir.actions.act_window",
        name: "Server Actions",
        res_model: "ir.actions.server",
        views: [[21, "list"], [22, "form"]],
        search_view_id: [23, "Server Actions Search"],
        view_mode: "list,form",
        target: "current"
      });
    }
    if (request.route === "/web/dataset/call_kw/ir.actions.server/get_views") {
      return Promise.resolve({
        views: {
          list: { arch: "<list><field name=\"name\"/></list>", id: 21 },
          form: { arch: "<form/>", id: 22 },
          search: {
            arch: "<search/>",
            id: 23,
            filters: []
          }
        },
        models: {
          "ir.actions.server": {
            fields: {
              name: { type: "char", string: "Name" },
              active: { type: "boolean", string: "Active" },
              model_id: { type: "many2one", relation: "ir.model", string: "Model" }
            }
          }
        }
      });
    }
    if (request.route === "/web/dataset/call_kw/ir.actions.server/web_search_read") {
      return Promise.resolve({ length: 1, records: [{ id: 3, name: "Update Records" }] });
    }
    return Promise.resolve({});
  }
});
const implicitSearchResult = await implicitSearchServices.action.doAction(11);
const implicitGetViews = implicitSearchRequests.find((request) => request.route === "/web/dataset/call_kw/ir.actions.server/get_views");
assert.deepEqual(implicitGetViews.params.kwargs.views, [[21, "list"], [22, "form"], [23, "search"]]);
assert.equal(implicitGetViews.params.kwargs.options.load_filters, true);
assert.equal(implicitSearchResult.activeView, "list");
assert.deepEqual(implicitSearchResult.search.filters.map((item) => item.id), ["filter-active", "filter-archived"]);
assert.deepEqual(implicitSearchResult.search.groupBys.map((item) => item.id), ["group-by-group_model"]);

const settingsSaveCalls = [];
const settingsEvents = [];
const settingsWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Settings",
    res_model: "res.config.settings",
    view_mode: "form",
    context: { active_app: "general_settings" }
  },
  activeView: "form",
  resModel: "res.config.settings",
  viewDescriptions: {
    fields: {
      group_multi_currency: { type: "boolean", string: "Multi Currency" },
      default_provider: {
        type: "selection",
        string: "Default Provider",
        selection: [["local", "Local"], ["openai", "OpenAI"]]
      }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form>
          <app name="general_settings" string="General Settings">
            <block title="Companies">
              <setting string="Multi Currency"><field name="group_multi_currency"/></setting>
              <setting string="Provider"><field name="default_provider"/></setting>
            </block>
          </app>
        </form>`,
        id: 30
      }
    }
  },
  records: [{ id: 5, group_multi_currency: false, default_provider: "local" }],
  length: 1
}, {
  services: {
    orm: {
      webSave(model, ids, data, kwargs) {
        settingsSaveCalls.push({ model, ids, data, kwargs });
        return Promise.resolve(true);
      }
    }
  }
});
settingsWindow.addEventListener("settings:field-change", (event) => settingsEvents.push(["field", event.detail]));
settingsWindow.addEventListener("settings:save", (event) => settingsEvents.push(["save", event.detail]));
settingsWindow.addEventListener("settings:discard", (event) => settingsEvents.push(["discard", event.detail]));
assert.equal(findAll(settingsWindow, (node) => String(node.className).includes("o_settings_container")).length, 1);
assert.equal(findAll(settingsWindow, (node) => String(node.className).includes("o_cp_searchview")).length, 1);
assert.equal(findAll(settingsWindow, (node) => String(node.className).includes("o_settings_search_panel")).length, 0);
const settingsSave = findAll(settingsWindow, (node) => node.dataset?.settingsAction === "save")[0];
const settingsDiscard = findAll(settingsWindow, (node) => node.dataset?.settingsAction === "discard")[0];
assert.equal(settingsSave.disabled, true);
assert.equal(settingsDiscard.disabled, true);
const currencyToggle = findAll(settingsWindow, (node) => node.dataset?.field === "group_multi_currency" && node.type === "checkbox")[0];
currencyToggle.checked = true;
currencyToggle.dispatchEvent(new TestEvent("change"));
assert.equal(settingsSave.disabled, false);
assert.equal(settingsDiscard.disabled, false);
settingsSave.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(settingsSaveCalls, [{
  model: "res.config.settings",
  ids: [5],
  data: { group_multi_currency: true },
  kwargs: { context: { active_app: "general_settings" } }
}]);
assert.equal(settingsSave.disabled, true);
assert.equal(findAll(settingsWindow, (node) => String(node.className).includes("o_settings_dirty_status"))[0].textContent, "Saved");
const providerSelect = findAll(settingsWindow, (node) => node.dataset?.field === "default_provider" && node.tag === "select")[0];
providerSelect.value = "openai";
providerSelect.dispatchEvent(new TestEvent("change"));
assert.equal(settingsDiscard.disabled, false);
settingsDiscard.dispatchEvent(new TestEvent("click"));
assert.equal(settingsDiscard.disabled, true);
assert.equal(findAll(settingsWindow, (node) => node.dataset?.field === "default_provider" && node.tag === "select")[0].value, "local");
assert.equal(settingsEvents.some((entry) => entry[0] === "save"), true);
assert.equal(settingsEvents.some((entry) => entry[0] === "discard"), true);

const kanbanOpenEvents = [];
const kanbanWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Partners",
    res_model: "res.partner",
    view_mode: "kanban,form",
    views: [[false, "kanban"], [false, "form"]]
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      email: { type: "char", string: "Email" },
      company_id: { type: "many2one", relation: "res.company", string: "Company" }
    },
    relatedModels: {},
    views: {
      kanban: {
        arch: `<kanban><field name="display_name"/><field name="email"/><field name="company_id"/></kanban>`,
        id: 18
      },
      form: {
        arch: `<form><field name="display_name"/></form>`,
        id: 19
      }
    }
  },
  records: [
    { id: 11, display_name: "Azure Interior", email: "azure@example.test", company_id: [3, "My Company"] }
  ],
  length: 1
});
assert.equal(kanbanWindow.dataset.view, "kanban");
const kanbanCreateButton = findAll(kanbanWindow, (node) => node.dataset?.createAction === "true")[0];
assert.ok(String(kanbanCreateButton.className).includes("o-kanban-button-new"));
assert.equal(kanbanCreateButton.attributes.accesskey, "c");
const kanbanRenderer = findAll(kanbanWindow, (node) => String(node.className ?? "").includes("o_kanban_renderer"))[0];
const kanbanQuickCreateEvents = [];
kanbanRenderer.addEventListener("action:open-record", (event) => kanbanOpenEvents.push(event.detail));
kanbanRenderer.addEventListener("action:create", (event) => kanbanQuickCreateEvents.push(event.detail));
assert.ok(String(kanbanRenderer.className).includes("o_kanban_ungrouped"));
assert.equal(kanbanRenderer.dataset.model, "res.partner");
const kanbanCard = findAll(kanbanRenderer, (node) => String(node.className ?? "").includes("o_kanban_record"))[0];
assert.ok(String(kanbanCard.className).includes("o_kanban_global_click"));
assert.ok(String(kanbanCard.className).includes("d-flex"));
assert.equal(kanbanCard.attributes.role, "link");
assert.equal(kanbanCard.dataset.id, "11");
assert.equal(findAll(kanbanCard, (node) => String(node.className ?? "").includes("o_kanban_record_title"))[0].textContent, "Azure Interior");
assert.deepEqual(findAll(kanbanCard, (node) => String(node.className ?? "").includes("o_kanban_record_field")).map((node) => node.dataset.field), ["email", "company_id"]);
const kanbanRecordMenuToggle = findAll(kanbanCard, (node) => node.dataset?.kanbanRecordMenu === "true")[0];
assert.equal(kanbanRecordMenuToggle.attributes["aria-expanded"], "false");
const kanbanRecordMenuDropdown = findAll(kanbanCard, (node) => String(node.className ?? "").includes("o_kanban_record_menu_dropdown"))[0];
assert.equal(kanbanRecordMenuDropdown.hidden, true);
kanbanRecordMenuToggle.dispatchEvent(new TestEvent("click"));
assert.equal(kanbanRecordMenuToggle.attributes["aria-expanded"], "true");
assert.equal(kanbanRecordMenuDropdown.hidden, false);
assert.ok(String(kanbanRecordMenuDropdown.className).includes("show"));
const kanbanRecordMenuOpen = findAll(kanbanRecordMenuDropdown, (node) => node.dataset?.kanbanRecordMenuAction === "open")[0];
assert.deepEqual(findAll(kanbanRecordMenuDropdown, (node) => node.dataset?.kanbanRecordMenuAction).map((node) => [node.dataset.kanbanRecordMenuAction, node.textContent]), [["open", "Open"], ["duplicate", "Duplicate"], ["delete", "Delete"]]);
assert.equal(findAll(kanbanRecordMenuDropdown, (node) => node.dataset?.kanbanRecordMenuAction === "duplicate")[0].disabled, true);
assert.equal(findAll(kanbanRecordMenuDropdown, (node) => node.dataset?.kanbanRecordMenuAction === "delete")[0].disabled, true);
kanbanRecordMenuOpen.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(kanbanOpenEvents.length, 1);
assert.equal(kanbanOpenEvents[0].action.res_id, 11);
assert.equal(kanbanRecordMenuToggle.attributes["aria-expanded"], "false");
kanbanCard.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(kanbanOpenEvents.length, 2);
assert.equal(kanbanOpenEvents[1].action.res_id, 11);
assert.deepEqual(kanbanOpenEvents[1].action.views, [[false, "form"]]);
kanbanCard.dispatchEvent(new TestEvent("keydown", { key: "Enter" }));
await Promise.resolve();
assert.equal(kanbanOpenEvents.length, 3);
assert.equal(kanbanOpenEvents[2].action.res_id, 11);
const kanbanQuickCreate = findAll(kanbanRenderer, (node) => node.dataset?.kanbanQuickCreate === "true")[0];
assert.equal(kanbanQuickCreate.textContent, "+ Add");
kanbanQuickCreate.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(kanbanQuickCreateEvents.length, 1);
assert.deepEqual(kanbanQuickCreateEvents[0].action.views, [[false, "form"]]);
assert.deepEqual(kanbanQuickCreateEvents[0].context, {});

const kanbanActionCalls = [];
const kanbanActionEvents = [];
const kanbanConfirmMessages = [];
let kanbanRefreshCount = 0;
const kanbanActionsWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Partners",
    res_model: "res.partner",
    view_mode: "kanban,form",
    views: [[false, "kanban"], [false, "form"]]
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      email: { type: "char", string: "Email" }
    },
    relatedModels: {},
    views: {
      kanban: { arch: `<kanban><field name="display_name"/><field name="email"/></kanban>`, id: 22 },
      form: { arch: `<form><field name="display_name"/></form>`, id: 23 }
    }
  },
  records: [{ id: 15, display_name: "Ready Mat", email: "ready@example.test" }],
  length: 1
}, {
  confirm(message) {
    kanbanConfirmMessages.push(message);
    return true;
  },
  onRefresh() {
    kanbanRefreshCount += 1;
  },
  services: {
    orm: {
      call(model, method, args) {
        kanbanActionCalls.push({ model, method, args });
        return Promise.resolve(115);
      },
      unlink(model, ids) {
        kanbanActionCalls.push({ model, method: "unlink", ids });
        return Promise.resolve(true);
      }
    }
  }
});
const kanbanActionsRenderer = findAll(kanbanActionsWindow, (node) => String(node.className ?? "").includes("o_kanban_renderer"))[0];
kanbanActionsRenderer.addEventListener("action-menu:duplicate", (event) => kanbanActionEvents.push(["duplicate", event.detail]));
kanbanActionsRenderer.addEventListener("action-menu:delete", (event) => kanbanActionEvents.push(["delete", event.detail]));
const kanbanActionsCard = findAll(kanbanActionsRenderer, (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_record"))[0];
const kanbanActionsMenuToggle = findAll(kanbanActionsCard, (node) => node.dataset?.kanbanRecordMenu === "true")[0];
const kanbanActionsDuplicate = findAll(kanbanActionsCard, (node) => node.dataset?.kanbanRecordMenuAction === "duplicate")[0];
const kanbanActionsDelete = findAll(kanbanActionsCard, (node) => node.dataset?.kanbanRecordMenuAction === "delete")[0];
assert.equal(kanbanActionsDuplicate.disabled, false);
assert.equal(kanbanActionsDelete.disabled, false);
kanbanActionsMenuToggle.dispatchEvent(new TestEvent("click"));
kanbanActionsDuplicate.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(kanbanActionCalls[0], { model: "res.partner", method: "copy", args: [15, {}] });
assert.deepEqual(kanbanActionEvents[0], ["duplicate", { model: "res.partner", ids: [15], newId: 115 }]);
kanbanActionsMenuToggle.dispatchEvent(new TestEvent("click"));
kanbanActionsDelete.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(kanbanConfirmMessages, ["Are you sure you want to delete this record?"]);
assert.deepEqual(kanbanActionCalls[1], { model: "res.partner", method: "unlink", ids: [15] });
assert.deepEqual(kanbanActionEvents[1], ["delete", { model: "res.partner", ids: [15] }]);
assert.equal(kanbanRefreshCount, 2);

const kanbanServerActionCalls = [];
const kanbanServerActionEvents = [];
const kanbanServerMenuWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Partners",
    res_model: "res.partner",
    view_mode: "kanban,form",
    views: [[false, "kanban"], [false, "form"]]
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      email: { type: "char", string: "Email" }
    },
    relatedModels: {},
    views: {
      kanban: {
        arch: `<kanban><field name="display_name"/><field name="email"/></kanban>`,
        id: 26,
        actionMenus: {
          print: [{ id: 720, name: "Print Card", description: "Print Card", sequence: 2, groupNumber: 1 }],
          action: [{ id: 710, name: "Run Card Action", sequence: 5, groupNumber: 2 }]
        }
      },
      form: { arch: `<form><field name="display_name"/></form>`, id: 27 }
    }
  },
  records: [{ id: 21, display_name: "Action Partner", email: "action@example.test" }],
  length: 1
}, {
  services: {
    action: {
      doAction(action, options) {
        kanbanServerActionCalls.push({ action, additionalContext: options.additionalContext });
        return Promise.resolve(true);
      }
    }
  }
});
const kanbanServerMenuRenderer = findAll(kanbanServerMenuWindow, (node) => String(node.className ?? "").includes("o_kanban_renderer"))[0];
kanbanServerMenuRenderer.addEventListener("action-menu:execute", (event) => kanbanServerActionEvents.push(event.detail));
const kanbanServerMenuCard = findAll(kanbanServerMenuRenderer, (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_record"))[0];
const kanbanServerMenuToggle = findAll(kanbanServerMenuCard, (node) => node.dataset?.kanbanRecordMenu === "true")[0];
const kanbanServerMenuDropdown = findAll(kanbanServerMenuCard, (node) => String(node.className ?? "").includes("o_kanban_record_menu_dropdown"))[0];
kanbanServerMenuToggle.dispatchEvent(new TestEvent("click"));
assert.deepEqual(findAll(kanbanServerMenuDropdown, (node) => node.dataset?.kanbanRecordMenuSection).map((node) => node.textContent), ["Print", "Actions"]);
const kanbanServerMenuItems = findAll(kanbanServerMenuDropdown, (node) => node.dataset?.kanbanRecordServerAction === "true");
assert.deepEqual(kanbanServerMenuItems.map((node) => [node.dataset.kanbanRecordMenuAction, node.dataset.actionId, node.dataset.recordId, node.textContent]), [
  ["print", "720", "21", "Print Card"],
  ["action", "710", "21", "Run Card Action"]
]);
kanbanServerMenuItems[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(kanbanServerMenuToggle.attributes["aria-expanded"], "false");
assert.deepEqual(kanbanServerActionCalls[0], {
  action: 720,
  additionalContext: { active_id: 21, active_ids: [21], active_model: "res.partner", active_domain: [] }
});
assert.deepEqual(kanbanServerActionEvents[0], {
  model: "res.partner",
  ids: [21],
  action: { id: 720, name: "Print Card", description: "Print Card", sequence: 2, groupNumber: 1 },
  type: "print"
});
kanbanServerMenuToggle.dispatchEvent(new TestEvent("click"));
kanbanServerMenuItems[1].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(kanbanServerActionCalls[1], {
  action: 710,
  additionalContext: { active_id: 21, active_ids: [21], active_model: "res.partner", active_domain: [] }
});
assert.deepEqual(kanbanServerActionEvents[1], {
  model: "res.partner",
  ids: [21],
  action: { id: 710, name: "Run Card Action", sequence: 5, groupNumber: 2 },
  type: "action"
});

const kanbanLoadMoreActions = [];
const kanbanLoadMoreWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Partners",
    res_model: "res.partner",
    view_mode: "kanban,form",
    views: [[false, "kanban"], [false, "form"]],
    limit: 2
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      email: { type: "char", string: "Email" }
    },
    relatedModels: {},
    views: {
      kanban: { arch: `<kanban><field name="display_name"/><field name="email"/></kanban>`, id: 24 },
      form: { arch: `<form><field name="display_name"/></form>`, id: 25 }
    }
  },
  records: [
    { id: 16, display_name: "First", email: "first@example.test" },
    { id: 17, display_name: "Second", email: "second@example.test" }
  ],
  length: 5,
  offset: 0,
  countLimited: false
}, {
  services: {
    action: {
      doAction(action, options) {
        kanbanLoadMoreActions.push({ action, options });
        return Promise.resolve(true);
      }
    }
  }
});
const kanbanLoadMoreButton = findAll(kanbanLoadMoreWindow, (node) => node.dataset?.kanbanLoadMore === "true")[0];
assert.equal(kanbanLoadMoreButton.textContent, "Load more");
assert.equal(kanbanLoadMoreButton.dataset.loaded, "2");
assert.equal(kanbanLoadMoreButton.dataset.total, "5");
assert.equal(kanbanLoadMoreButton.dataset.nextLimit, "4");
kanbanLoadMoreButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(kanbanLoadMoreActions.length, 1);
assert.equal(kanbanLoadMoreActions[0].action.limit, 4);
assert.equal(kanbanLoadMoreActions[0].action.__pager_offset, 0);
assert.equal(kanbanLoadMoreActions[0].action.res_model, "res.partner");
assert.deepEqual(kanbanLoadMoreActions[0].options, { additionalContext: {}, replaceLastAction: true });
assert.equal(kanbanLoadMoreButton.disabled, true);

const kanbanProgressWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Partners",
    res_model: "res.partner",
    view_mode: "kanban",
    views: [[false, "kanban"]]
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      state: { type: "selection", string: "State", selection: [["code", "Execute Code"], ["multi", "Multi Actions"]] },
      amount: { type: "float", string: "Amount" },
      color: { type: "integer", string: "Color" }
    },
    relatedModels: {},
    views: {
      kanban: {
        arch: `<kanban><progressbar field="state" colors="{'code':'success','multi':'warning'}" sum_field="amount"/><field name="display_name"/><field name="state"/><field name="amount"/><field name="color"/></kanban>`,
        id: 26
      }
    }
  },
  records: [
    { id: 18, display_name: "First", state: "code", amount: 10, color: 2 },
    { id: 19, display_name: "Second", state: "multi", amount: 5, color: 5 },
    { id: 20, display_name: "Third", state: "code", amount: 5, color: 2 }
  ],
  length: 3,
  offset: 0,
  countLimited: false
});
const kanbanProgress = findAll(kanbanProgressWindow, (node) => String(node.className ?? "").includes("o_kanban_progressbar"))[0];
assert.equal(kanbanProgress.dataset.field, "state");
assert.equal(kanbanProgress.dataset.sumField, "amount");
assert.equal(kanbanProgress.attributes.role, "group");
const kanbanProgressSegments = findAll(kanbanProgress, (node) => String(node.className ?? "").includes("o_kanban_progressbar_segment"));
assert.deepEqual(kanbanProgressSegments.map((node) => [node.dataset.value, node.dataset.label, node.dataset.count, node.dataset.sum]), [
  ["code", "Execute Code", "2", "15"],
  ["multi", "Multi Actions", "1", "5"]
]);
assert.ok(String(kanbanProgressSegments[0].className).includes("o_kanban_progress_color_success"));
assert.ok(String(kanbanProgressSegments[1].className).includes("o_kanban_progress_color_warning"));
assert.equal(kanbanProgressSegments[0].attributes.style, "width: 75.00%;");
assert.equal(kanbanProgressSegments[1].attributes.style, "width: 25.00%;");
const kanbanProgressLegend = findAll(kanbanProgress, (node) => String(node.className ?? "").includes("o_kanban_progressbar_legend_item"));
assert.deepEqual(
  kanbanProgressLegend.map((node) => findAll(node, (child) => String(child.className ?? "").includes("o_kanban_progressbar_legend_text"))[0].textContent),
  ["Execute Code 15", "Multi Actions 5"]
);
const kanbanProgressCards = findAll(kanbanProgressWindow, (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_record"));
assert.equal(kanbanProgressCards[0].dataset.kanbanColor, "2");
assert.ok(String(kanbanProgressCards[0].className).includes("o_kanban_color_2"));
assert.ok(String(kanbanProgressCards[1].className).includes("o_kanban_color_5"));

const kanbanTemplateWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Template Partners",
    res_model: "res.partner",
    view_mode: "kanban,form",
    views: [[false, "kanban"], [false, "form"]]
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      email: { type: "char", string: "Email" },
      state: { type: "selection", string: "State", selection: [["new", "New"], ["done", "Done"]] },
      tags: { type: "many2many", string: "Tags", relation: "res.partner.category" },
      employee_id: { type: "many2one", string: "Employee", relation: "hr.employee" },
      url: { type: "char", string: "URL" }
    },
    relatedModels: {},
    views: {
      kanban: {
        arch: `<kanban><field name="display_name"/><field name="email"/><field name="state"/><field name="tags"/><field name="employee_id"/><field name="url"/><templates><t t-name="kanban-box"><div class="tmpl-card" t-att="{'data-state': record.state.raw_value}" t-att-data-id="record.id.raw_value" t-att-title="record.display_name.value" t-attf-aria-label="Partner #{record.display_name.value}" t-attf-class="state-#{record.state.raw_value}"><t t-set="badge" t-value="record.state.value"/><t t-set="body_note"><span class="tmpl-captured">Captured:<t t-esc="record.display_name.value"/></span></t><strong class="tmpl-title"><field name="display_name"/></strong><field name="employee_id" widget="many2one_avatar_employee" class="tmpl-assignee"/><field name="state" widget="badge" decoration-success="state == 'new'" class="tmpl-state-badge"/><field name="tags" widget="many2many_tags" class="tmpl-tag-widget"/><span class="tmpl-badge" t-att-data-badge="badge" t-esc="badge"/><t t-out="body_note"/><a class="tmpl-link" t-att-href="record.url.raw_value" t-att-rel="'noopener'">Open</a><t t-if="record.email.raw_value"><span class="tmpl-email"><field name="email"/></span></t><span class="tmpl-state" t-esc="record.state.value"/><t t-call="kanban-tag-list"><span class="tmpl-slot">Slot:<t t-esc="record.state.value"/></span></t></div></t><t t-name="kanban-tag-list"><section class="tmpl-subtemplate" data-called="tag-list"><t t-out="0"/><ul class="tmpl-tags"><t t-foreach="record.tags.raw_value" t-as="tag"><li class="tmpl-tag" t-att-data-index="tag_index" t-attf-class="tag-#{tag_index}" t-esc="tag[1]"/></t></ul></section></t><t t-inherit="kanban-box" t-inherit-mode="extension"><xpath expr="//div[hasclass('tmpl-card')]" position="inside"><span class="tmpl-inherited-inside" t-esc="record.email.value"/></xpath><xpath expr="//strong[hasclass('tmpl-title')]" position="after"><span class="tmpl-inherited-after">After Title</span></xpath><xpath expr="//field[@name='employee_id']" position="attributes"><attribute name="class" add="tmpl-inherited-avatar" separator=" "/></xpath></t></templates></kanban>`,
        id: 29
      },
      form: { arch: `<form><field name="display_name"/></form>`, id: 30 }
    }
  },
  records: [{ id: 41, display_name: "Template Partner", email: "template@example.test", state: "new", tags: [[12, "VIP"], [13, "Supplier"]], employee_id: [17, "Mina Reyes"], url: "#record-41" }],
  length: 1
});
const kanbanTemplateCard = findAll(kanbanTemplateWindow, (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_record"))[0];
const kanbanTemplateDetails = findAll(kanbanTemplateCard, (node) => node.dataset?.kanbanTemplate === "kanban-box")[0];
const kanbanTemplateBody = findAll(kanbanTemplateCard, (node) => node.dataset?.kanbanTemplateBody === "true")[0];
const kanbanTemplateRoot = findAll(kanbanTemplateCard, (node) => String(node.className ?? "").includes("tmpl-card"))[0];
const kanbanTemplateTitle = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("tmpl-title"))[0];
const kanbanTemplateEmail = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("tmpl-email"))[0];
const kanbanTemplateState = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("tmpl-state"))[0];
const kanbanTemplateLink = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("tmpl-link"))[0];
const kanbanTemplateBadge = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("tmpl-badge"))[0];
const kanbanTemplateWidgetBadge = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("gorp-badge"))[0];
const kanbanTemplateX2ManyWidget = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("gorp-x2many-tags"))[0];
const kanbanTemplateAvatarWidget = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("gorp-many2one-avatar"))[0];
const kanbanTemplateInheritedAvatar = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").split(/\s+/).includes("tmpl-inherited-avatar"))[0];
const kanbanTemplateCaptured = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("tmpl-captured"))[0];
const kanbanTemplateSlot = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").includes("tmpl-slot"))[0];
const kanbanTemplateSubtemplate = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").split(/\s+/).includes("tmpl-subtemplate"))[0];
const kanbanTemplateTagList = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").split(/\s+/).includes("tmpl-tags"))[0];
const kanbanTemplateTags = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").split(/\s+/).includes("tmpl-tag"));
const kanbanTemplateInheritedInside = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").split(/\s+/).includes("tmpl-inherited-inside"))[0];
const kanbanTemplateInheritedAfter = findAll(kanbanTemplateRoot, (node) => String(node.className ?? "").split(/\s+/).includes("tmpl-inherited-after"))[0];
assert.equal(kanbanTemplateDetails.dataset.kanbanTemplate, "kanban-box");
assert.ok(String(kanbanTemplateDetails.className).includes("o_kanban_template_details"));
assert.equal(kanbanTemplateBody.dataset.kanbanTemplateBody, "true");
assert.ok(String(kanbanTemplateRoot.className).includes("state-new"));
assert.equal(kanbanTemplateRoot.dataset.id, "41");
assert.equal(kanbanTemplateRoot.dataset.state, "new");
assert.equal(kanbanTemplateRoot.attributes.title, "Template Partner");
assert.equal(kanbanTemplateRoot.attributes["aria-label"], "Partner Template Partner");
assert.equal(findAll(kanbanTemplateTitle, (node) => node.dataset?.field === "display_name")[0].children[0].textContent, "Template Partner");
assert.equal(kanbanTemplateBadge.dataset.badge, "New");
assert.equal(kanbanTemplateBadge.children[0].textContent, "New");
assert.equal(kanbanTemplateWidgetBadge.dataset.field, "state");
assert.equal(kanbanTemplateWidgetBadge.dataset.widget, "badge");
assert.equal(kanbanTemplateWidgetBadge.dataset.decoration, "success");
assert.equal(kanbanTemplateWidgetBadge.textContent, "New");
assert.equal(kanbanTemplateX2ManyWidget.dataset.field, "tags");
assert.equal(kanbanTemplateX2ManyWidget.dataset.count, "2");
assert.equal(findAll(kanbanTemplateX2ManyWidget, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-x2many-tag"))[0].textContent, "VIP");
assert.equal(kanbanTemplateAvatarWidget.dataset.field, "employee_id");
assert.equal(kanbanTemplateAvatarWidget.dataset.relation, "hr.employee");
assert.equal(kanbanTemplateAvatarWidget.dataset.resId, "17");
assert.equal(kanbanTemplateInheritedAvatar.dataset.field, "employee_id");
assert.equal(kanbanTemplateInheritedInside.children[0].textContent, "template@example.test");
assert.equal(kanbanTemplateInheritedAfter.children[0].textContent, "After Title");
assert.equal(kanbanTemplateCaptured.children[0].textContent, "Captured:");
assert.equal(kanbanTemplateCaptured.children[1].textContent, "Template Partner");
assert.equal(kanbanTemplateSlot.children[0].textContent, "Slot:");
assert.equal(kanbanTemplateSlot.children[1].textContent, "New");
assert.equal(kanbanTemplateSubtemplate.dataset.called, "tag-list");
assert.equal(kanbanTemplateLink.attributes.href, "#record-41");
assert.equal(kanbanTemplateLink.attributes.rel, "noopener");
assert.equal(findAll(kanbanTemplateEmail, (node) => node.dataset?.field === "email")[0].children[0].textContent, "template@example.test");
assert.equal(kanbanTemplateState.children[0].textContent, "New");
assert.equal(kanbanTemplateTags.length, 2);
assert.equal(kanbanTemplateTags[0].dataset.index, "0");
assert.ok(String(kanbanTemplateTags[0].className).includes("tag-0"));
assert.equal(kanbanTemplateTags[0].children[0].textContent, "VIP");
assert.equal(kanbanTemplateTags[1].dataset.index, "1");
assert.ok(String(kanbanTemplateTags[1].className).includes("tag-1"));
assert.equal(kanbanTemplateTags[1].children[0].textContent, "Supplier");
assert.equal(findAll(kanbanTemplateCard, (node) => String(node.className ?? "").includes("o_kanban_record_field")).length, 0);

const groupedKanbanCreateCalls = [];
const groupedKanbanWriteCalls = [];
const groupedKanbanDropEvents = [];
let groupedKanbanRefreshCount = 0;
const groupedKanbanWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Partners",
    res_model: "res.partner",
    view_mode: "kanban,form",
    views: [[false, "kanban"], [false, "form"]]
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      email: { type: "char", string: "Email" },
      company_id: { type: "many2one", relation: "res.company", string: "Company" }
    },
    relatedModels: {},
    views: {
      kanban: {
        arch: `<kanban><field name="display_name"/><field name="email"/><field name="company_id"/></kanban>`,
        id: 20
      },
      form: {
        arch: `<form><field name="display_name"/></form>`,
        id: 21
      }
    }
  },
  search: {
    state: { query: "", facets: [], groupBy: ["company_id"], domain: [] },
    suggestions: [],
    filters: [],
    groupBys: [],
    favorites: []
  },
  records: [
    { id: 12, display_name: "Azure Interior", email: "azure@example.test", company_id: [3, "My Company"] },
    { id: 13, display_name: "Deco Addict", email: "deco@example.test", company_id: [4, "Second Company"] },
    { id: 14, display_name: "Open Space", email: "space@example.test", company_id: [3, "My Company"] }
  ],
  length: 3
}, {
  context: { active_id: 77 },
  onRefresh() {
    groupedKanbanRefreshCount += 1;
  },
  services: {
    action: {
      doAction(action, options) {
        groupedKanbanCreateCalls.push({ action, options });
        return Promise.resolve({});
      }
    },
    orm: {
      write(model, ids, data, kwargs) {
        groupedKanbanWriteCalls.push({ model, ids, data, kwargs });
        return Promise.resolve(true);
      }
    }
  }
});
const groupedKanbanRenderer = findAll(groupedKanbanWindow, (node) => String(node.className ?? "").includes("o_kanban_renderer"))[0];
groupedKanbanRenderer.addEventListener("action:kanban-record-drop", (event) => groupedKanbanDropEvents.push(event.detail));
assert.ok(String(groupedKanbanRenderer.className).includes("o_kanban_grouped"));
assert.equal(groupedKanbanRenderer.dataset.groupby, "company_id");
const groupedKanbanGroups = findAll(groupedKanbanRenderer, (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_group"));
assert.deepEqual(groupedKanbanGroups.map((node) => findAll(node, (child) => String(child.className ?? "").includes("o_kanban_header_title"))[0].textContent), ["My Company", "Second Company"]);
assert.deepEqual(groupedKanbanGroups.map((node) => findAll(node, (child) => String(child.className ?? "").includes("o_kanban_counter"))[0].textContent), ["2", "1"]);
assert.deepEqual(groupedKanbanGroups.map((node) => findAll(node, (child) => String(child.className ?? "").split(/\s+/).includes("o_kanban_record")).length), [2, 1]);
const firstGroupFoldToggle = findAll(groupedKanbanGroups[0], (node) => node.dataset?.kanbanGroupFold === "true")[0];
const firstGroupRecords = findAll(groupedKanbanGroups[0], (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_records"))[0];
assert.equal(groupedKanbanGroups[0].dataset.folded, "false");
assert.equal(firstGroupFoldToggle.attributes["aria-expanded"], "true");
assert.equal(firstGroupRecords.hidden, false);
firstGroupFoldToggle.dispatchEvent(new TestEvent("click"));
assert.equal(groupedKanbanGroups[0].dataset.folded, "true");
assert.ok(String(groupedKanbanGroups[0].className).includes("o_column_folded"));
assert.equal(firstGroupFoldToggle.attributes["aria-expanded"], "false");
assert.equal(firstGroupFoldToggle.attributes["aria-label"], "Unfold column");
assert.equal(firstGroupRecords.hidden, true);
firstGroupFoldToggle.dispatchEvent(new TestEvent("click"));
assert.equal(groupedKanbanGroups[0].dataset.folded, "false");
assert.equal(firstGroupFoldToggle.attributes["aria-expanded"], "true");
assert.equal(firstGroupRecords.hidden, false);
const groupedQuickCreateButtons = groupedKanbanGroups.map((node) => findAll(node, (child) => child.dataset?.kanbanQuickCreate === "true")[0]);
assert.equal(groupedQuickCreateButtons.length, 2);
assert.deepEqual(groupedQuickCreateButtons.map((node) => node.dataset.groupField), ["company_id", "company_id"]);
assert.deepEqual(groupedQuickCreateButtons.map((node) => node.dataset.groupDefault), ["3", "4"]);
groupedQuickCreateButtons[0].dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(groupedKanbanCreateCalls.length, 1);
assert.deepEqual(groupedKanbanCreateCalls[0].action.views, [[false, "form"]]);
assert.deepEqual(groupedKanbanCreateCalls[0].options, { additionalContext: { active_id: 77, default_company_id: 3 }, replaceLastAction: true });
const groupedFirstCard = findAll(groupedKanbanGroups[0], (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_record"))[0];
const groupedSecondRecords = findAll(groupedKanbanGroups[1], (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_records"))[0];
const groupedDragData = testDataTransfer();
assert.equal(groupedFirstCard.getAttribute("draggable"), "true");
assert.equal(groupedFirstCard.dataset.kanbanDraggable, "true");
assert.equal(groupedFirstCard.dataset.groupField, "company_id");
assert.equal(groupedFirstCard.dataset.groupValue, "3");
groupedFirstCard.dispatchEvent(new TestEvent("dragstart", { dataTransfer: groupedDragData }));
assert.equal(groupedKanbanRenderer.dataset.kanbanDraggingId, "12");
assert.equal(groupedDragData.getData("text/plain"), "12");
const groupedDragOver = new TestEvent("dragover", { dataTransfer: groupedDragData });
groupedKanbanGroups[1].dispatchEvent(groupedDragOver);
assert.equal(groupedDragOver.defaultPrevented, true);
assert.equal(groupedKanbanGroups[1].dataset.dropTargetActive, "true");
assert.ok(String(groupedKanbanGroups[1].className).includes("o_kanban_group_drop_target"));
assert.ok(String(groupedSecondRecords.className).includes("o_kanban_records_drop_target"));
groupedKanbanGroups[1].dispatchEvent(new TestEvent("drop", { dataTransfer: groupedDragData }));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(groupedKanbanWriteCalls, [{
  model: "res.partner",
  ids: [12],
  data: { company_id: 4 },
  kwargs: { context: { active_id: 77 } }
}]);
assert.equal(groupedKanbanRefreshCount, 1);
assert.deepEqual(groupedKanbanDropEvents, [{
  model: "res.partner",
  id: 12,
  field: "company_id",
  value: 4,
  groupKey: "4",
  previousGroupKey: "3"
}]);
assert.equal(groupedKanbanGroups[1].dataset.dropTargetActive, "false");
assert.equal(groupedKanbanRenderer.dataset.kanbanDropField, "company_id");
assert.equal(groupedKanbanRenderer.dataset.kanbanDropValue, "4");
assert.equal(groupedKanbanRenderer.dataset.kanbanDroppingId, undefined);
groupedFirstCard.dispatchEvent(new TestEvent("dragend", { dataTransfer: groupedDragData }));
assert.equal(groupedKanbanRenderer.dataset.kanbanDraggingId, undefined);

const groupedKanbanLoadEvents = [];
const groupedKanbanLoadWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    name: "Partners",
    res_model: "res.partner",
    view_mode: "kanban,form",
    views: [[false, "kanban"], [false, "form"]],
    __kanban_group_limit: 2
  },
  activeView: "kanban",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      display_name: { type: "char", string: "Name" },
      stage_id: { type: "many2one", relation: "crm.stage", string: "Stage" }
    },
    relatedModels: {},
    views: {
      kanban: {
        arch: `<kanban><field name="display_name"/><field name="stage_id"/></kanban>`,
        id: 28
      }
    }
  },
  search: {
    state: { query: "", facets: [], groupBy: ["stage_id"], domain: [] },
    suggestions: [],
    filters: [],
    groupBys: [],
    favorites: []
  },
  records: [
    { id: 31, display_name: "Stage One", stage_id: [8, "New"] },
    { id: 32, display_name: "Stage Two", stage_id: [8, "New"] },
    { id: 33, display_name: "Stage Three", stage_id: [8, "New"] },
    { id: 34, display_name: "Stage Four", stage_id: [8, "New"] },
    { id: 35, display_name: "Stage Five", stage_id: [8, "New"] },
    { id: 36, display_name: "Done One", stage_id: [9, "Done"] }
  ],
  length: 6
});
const groupedKanbanLoadRenderer = findAll(groupedKanbanLoadWindow, (node) => String(node.className ?? "").includes("o_kanban_renderer"))[0];
groupedKanbanLoadRenderer.addEventListener("action:kanban-group-load-more", (event) => groupedKanbanLoadEvents.push(event.detail));
const groupedKanbanLoadGroups = findAll(groupedKanbanLoadRenderer, (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_group"));
const groupedKanbanLoadFirstBody = findAll(groupedKanbanLoadGroups[0], (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_records"))[0];
const groupedKanbanLoadCards = () => findAll(groupedKanbanLoadFirstBody, (node) => String(node.className ?? "").split(/\s+/).includes("o_kanban_record"));
const groupedKanbanLoadButton = findAll(groupedKanbanLoadFirstBody, (node) => node.dataset?.kanbanGroupLoadMore === "true")[0];
assert.equal(groupedKanbanLoadButton.textContent, "Load more");
assert.equal(groupedKanbanLoadButton.dataset.loaded, "2");
assert.equal(groupedKanbanLoadButton.dataset.total, "5");
assert.equal(groupedKanbanLoadButton.dataset.remaining, "3");
assert.equal(groupedKanbanLoadButton.dataset.limit, "2");
assert.deepEqual(groupedKanbanLoadCards().map((node) => [node.dataset.id, node.hidden === true, node.dataset.kanbanGroupHidden ?? ""]), [
  ["31", false, ""],
  ["32", false, ""],
  ["33", true, "true"],
  ["34", true, "true"],
  ["35", true, "true"]
]);
groupedKanbanLoadButton.dispatchEvent(new TestEvent("click"));
assert.deepEqual(groupedKanbanLoadCards().map((node) => [node.dataset.id, node.hidden === true, node.dataset.kanbanGroupHidden ?? ""]), [
  ["31", false, ""],
  ["32", false, ""],
  ["33", false, ""],
  ["34", false, ""],
  ["35", true, "true"]
]);
assert.equal(groupedKanbanLoadButton.dataset.loaded, "4");
assert.equal(groupedKanbanLoadButton.dataset.remaining, "1");
assert.equal(groupedKanbanLoadButton.hidden, false);
assert.deepEqual(groupedKanbanLoadEvents[0], { groupKey: "8", loaded: 4, total: 5, revealed: 2, remaining: 1 });
groupedKanbanLoadButton.dispatchEvent(new TestEvent("click"));
assert.equal(groupedKanbanLoadButton.dataset.loaded, "5");
assert.equal(groupedKanbanLoadButton.dataset.remaining, "0");
assert.equal(groupedKanbanLoadButton.hidden, true);
assert.equal(groupedKanbanLoadButton.attributes.hidden, "hidden");
assert.equal(groupedKanbanLoadCards().filter((node) => node.hidden === true).length, 0);
assert.deepEqual(groupedKanbanLoadEvents[1], { groupKey: "8", loaded: 5, total: 5, revealed: 1, remaining: 0 });

const moduleInfoCalls = [];
const moduleKanbanWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: {
    id: 91,
    name: "Apps",
    path: "apps",
    res_model: "ir.module.module",
    view_mode: "kanban,list,form",
    views: [[false, "kanban"], [false, "list"], [false, "form"]]
  },
  activeView: "kanban",
  resModel: "ir.module.module",
  viewDescriptions: {
    fields: {
      shortdesc: { type: "char", string: "Name" },
      name: { type: "char", string: "Technical Name" },
      state: { type: "selection", string: "Status" }
    },
    relatedModels: {},
    views: {
      kanban: {
        arch: `<kanban create="false" can_open="0" class="o_modules_kanban"><field name="shortdesc"/><field name="name"/><field name="state"/></kanban>`,
        id: 91
      },
      form: {
        arch: `<form><sheet><field name="shortdesc"/><field name="name"/><field name="state"/></sheet></form>`,
        id: 92
      }
    }
  },
  records: [{ id: 5, shortdesc: "Base", name: "base", display_name: "Base", state: "installed" }],
  length: 1
}, {
  context: { search_default_app: 1 },
  services: {
    action: {
      doAction(action, options) {
        moduleInfoCalls.push({ action, options });
        return Promise.resolve({});
      }
    }
  }
});
const moduleInfoButton = findAll(moduleKanbanWindow, (node) => node.dataset?.moduleInfo === "base" && String(node.className ?? "").includes("o_module_info_button"))[0];
assert.equal(moduleInfoButton.textContent, "Module Info");
moduleInfoButton.dispatchEvent(new TestEvent("click"));
await Promise.resolve();
assert.equal(moduleInfoCalls.length, 1);
assert.deepEqual(moduleInfoCalls[0].action, {
  id: 91,
  name: "Module Info",
  path: "apps",
  res_model: "ir.module.module",
  view_mode: "form",
  views: [[false, "form"]],
  res_id: 5,
  target: "new"
});
assert.deepEqual(moduleInfoCalls[0].options, {
  additionalContext: {
    search_default_app: 1,
    active_model: "ir.module.module",
    active_id: 5,
    active_ids: [5]
  }
});

const delegationWidgetWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Delegation" },
  activeView: "list",
  resModel: "delegation",
  viewDescriptions: {
    fields: {
      employee_id: { type: "many2one", relation: "hr.employee", string: "Employee" },
      state: { type: "selection", string: "Status", selection: [["draft", "Draft"], ["approved", "Approved"], ["cancel", "Cancelled"]] },
      waiting_approval: { type: "boolean", string: "Waiting" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list decoration-danger="state == 'cancel'"><field name="employee_id" widget="many2one_avatar_employee"/><field name="state" widget="badge" decoration-success="state == 'approved'" decoration-warning="waiting_approval"/><field name="waiting_approval" column_invisible="1"/></list>`,
        id: 91
      }
    }
  },
  records: [
    { id: 5, employee_id: [7, "Mitchell Admin"], state: "approved", waiting_approval: false },
    { id: 6, employee_id: [8, "Marc Demo"], state: "draft", waiting_approval: true },
    { id: 7, employee_id: [9, "Cancelled User"], state: "cancel", waiting_approval: false }
  ],
  length: 3
});
assert.deepEqual(findAll(delegationWidgetWindow, (node) => node.tag === "th").map((node) => node.children[0]?.textContent ?? node.textContent), ["Employee", "Status"]);
const delegationAvatar = findAll(delegationWidgetWindow, (node) => String(node.className ?? "").includes("gorp-many2one-avatar"))[0];
assert.equal(delegationAvatar.dataset.relation, "hr.employee");
assert.equal(delegationAvatar.dataset.resId, "7");
assert.equal(findAll(delegationAvatar, (node) => node.tag === "img")[0].src, "/web/image/hr.employee/7/avatar_128");
const delegationBadges = findAll(delegationWidgetWindow, (node) => String(node.className ?? "").includes("gorp-badge"));
const delegationBadge = delegationBadges[0];
assert.equal(delegationBadge.textContent, "Approved");
assert.equal(delegationBadge.dataset.decoration, "success");
assert.ok(String(delegationBadge.className).includes("text-bg-success"));
assert.equal(delegationBadges[1].textContent, "Draft");
assert.equal(delegationBadges[1].dataset.decoration, "warning");
assert.ok(String(delegationBadges[1].className).includes("text-bg-warning"));
const decoratedRows = findAll(delegationWidgetWindow, (node) => String(node.className ?? "").includes("o_list_record_danger"));
assert.equal(decoratedRows.length, 1);
assert.equal(decoratedRows[0].dataset.field, undefined);

const delegationChatterFetches = [];
const delegationFormWidgetWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Delegation" },
  activeView: "form",
  resModel: "delegation",
  viewDescriptions: {
    fields: {
      employee_id: { type: "many2one", relation: "hr.employee", string: "Employee" },
      name: { type: "char", string: "Name" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="employee_id" widget="many2one_avatar_employee"/><field name="name"/></sheet><chatter/></form>`,
        id: 92
      }
    }
  },
  records: [{ id: 6, employee_id: [8, "Marc Demo"], name: "DG0001" }],
  length: 1
}, {
  context: { access_token: "thread-token", hash: "thread-hash", pid: 12 },
  services: {
    mail: {
      chatterFetch(thread, fetchParams, access) {
        delegationChatterFetches.push({ thread, fetchParams, access });
        return Promise.resolve({
          messages: [44],
          data: {
            "mail.message": [{
              id: 44,
              author_id: { id: 8, name: "Marc Demo" },
              author_avatar_url: "/mail/avatar/mail.message/44/author_avatar/50x50?access_token=thread-token",
              body: ["markup", "<p>Approved<br/>Now</p>"],
              published_date_str: "2026-06-21 09:30:00",
              attachment_ids: [{ id: 7, filename: "approval.pdf" }],
              reactions: [{ content: "ok", count: 2 }]
            }]
          }
        });
      }
    }
  }
});
const delegationForm = delegationFormWidgetWindow.children[1];
assert.equal(findAll(delegationForm, (node) => String(node.className ?? "").includes("gorp-many2one-avatar"))[0].dataset.resId, "8");
const delegationChatter = findAll(delegationForm, (node) => String(node.className ?? "").includes("gorp-chatter"))[0];
assert.equal(delegationChatter.dataset.threadModel, "delegation");
assert.equal(delegationChatter.dataset.threadId, "6");
assert.ok(String(delegationChatter.className).includes("o-mail-ChatterContainer"));
assert.ok(String(delegationChatter.className).includes("o-mail-Form-chatter"));
assert.ok(String(delegationChatter.className).includes("o-mail-Chatter"));
await Promise.resolve();
await Promise.resolve();
assert.deepEqual(delegationChatterFetches[0], {
  thread: { thread_model: "delegation", thread_id: 6 },
  fetchParams: { limit: 30 },
  access: { token: "thread-token", hash: "thread-hash", pid: 12 }
});
const delegationMessages = findAll(delegationChatter, (node) => String(node.className ?? "").includes("o-mail-Message"));
assert.equal(delegationMessages[0].dataset.messageId, "44");
assert.ok(findAll(delegationMessages[0], (node) => String(node.className ?? "").includes("o-mail-Message-author"))[0].textContent.includes("Marc Demo"));
assert.ok(findAll(delegationMessages[0], (node) => String(node.className ?? "").includes("o-mail-Message-body"))[0].textContent.includes("Approved"));
assert.equal(findAll(delegationMessages[0], (node) => String(node.className ?? "") === "gorp-chatter-attachment o-mail-Attachment")[0].textContent, "approval.pdf");
assert.equal(findAll(delegationMessages[0], (node) => String(node.className ?? "") === "gorp-chatter-reaction o-mail-Reaction")[0].textContent, "ok 2");

const invalidDirectWindowRequests = [];
const invalidDirectWindowServices = createWebClientServices({
  transport(request) {
    invalidDirectWindowRequests.push(request);
    return Promise.resolve({});
  }
});
await assert.rejects(
  () => invalidDirectWindowServices.action.doAction({
    type: "ir.actions.act_window",
    name: "Invalid Direct Window",
    res_model: "res.partner",
    view_mode: "list,form",
    view_id: 77
  }),
  /either multiple view modes or a single view mode and an optional view id/
);
assert.equal(invalidDirectWindowRequests.length, 0);

const approveAllCalls = [];
let approveAllEvent;
let approveAllRefreshes = 0;
const approveAllWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Purchase Requests" },
  activeView: "list",
  resModel: "purchase.request",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" } },
    relatedModels: {},
    views: { list: { arch: `<list show_action_approve_all="true"><field name="name"/></list>`, id: 21 } }
  },
  records: [{ id: 11, name: "PR001" }, { id: 12, name: "PR002" }],
  length: 2
}, {
  services: {
    dataset: {
      callButton(model, method, args, kwargs) {
        approveAllCalls.push({ model, method, args, kwargs });
        return Promise.resolve({ type: "ir.actions.client", tag: "soft_reload" });
      }
    },
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(action);
      },
      doAction(action) {
        approveAllCalls.push({ action });
        return Promise.resolve(action);
      }
    }
  },
  confirm(message) {
    approveAllCalls.push({ confirm: message });
    return true;
  },
  onRefresh() {
    approveAllRefreshes += 1;
  }
});
const approveAllShell = approveAllWindow.children[1];
approveAllShell.addEventListener("workflow:approve-all", (event) => { approveAllEvent = event.detail; });
assert.ok(String(approveAllShell.className).includes("gorp-list-shell"));
assert.ok(String(approveAllShell.className).includes("o-list-view"));
const approveAllButton = findAll(approveAllWindow, (node) => node.dataset?.workflowAction === "approve")[0];
assert.equal(approveAllButton.textContent, "Approve");
assert.equal(approveAllButton.dataset.sequence, "110");
assert.equal(approveAllButton.dataset.icon, "fa fa-thumbs-up");
assert.equal(approveAllButton.disabled, true);
const approveAllCheckboxes = findAll(approveAllShell, (node) => node.tag === "input" && node.type === "checkbox" && node.dataset?.recordId);
approveAllCheckboxes[0].checked = true;
approveAllCheckboxes[0].dispatchEvent(new TestEvent("change"));
assert.equal(approveAllButton.disabled, false);
approveAllButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(approveAllCalls[0], { confirm: "Are you sure you want to approve selected documents?" });
assert.deepEqual(approveAllCalls[1], { model: "purchase.request", method: "action_approve_all", args: [[11]], kwargs: {} });
assert.equal(approveAllCalls[2].action.tag, "soft_reload");
assert.equal(approveAllRefreshes, 1);
assert.deepEqual(approveAllEvent.ids, [11]);
const approvalButtonCalls = [];
const approvalUpdates = [];
const approvalValidation = [];
let approvalButtonEvent;
let approvalRefreshes = 0;
const approvalFormWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Purchase Order" },
  activeView: "form",
  resModel: "purchase.order",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      state: { type: "selection", string: "Status" },
      approved_button_clicked: { type: "integer", string: "Clicked" },
      workflow_transition_ids: { type: "many2many", string: "Transitions" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><header><field name="approved_button_clicked" invisible="1"/><field name="workflow_transition_ids" invisible="1"/><button name="approval_action_button" type="object" string="Approve" args="[7]" validate_form="True" context="{&#39;lang&#39;: &#39;en_US&#39;}" confirm="Confirm approval?"/><button name="approval_transition_button" type="object" string="Route" args="[9]" validate_form="False" invisible="9 not in workflow_transition_ids"/><button name="approval_transition_wizard" type="object" string="Wizard" args="[12]" validate_form="False"/><button name="approval_transition_button" type="object" string="Hidden" args="[99]" invisible="99 not in workflow_transition_ids"/></header><sheet><field name="name"/><field name="state"/></sheet></form>`,
        id: 31
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 42, name: "PO001", state: "pending", approved_button_clicked: false, workflow_transition_ids: [9] },
  services: {
    dataset: {
      callButton(model, method, args, kwargs) {
        approvalButtonCalls.push({ model, method, args, kwargs });
        if (method === "approval_action_button") return Promise.resolve({ type: "ir.actions.client", tag: "soft_reload" });
        if (method === "approval_transition_wizard") return Promise.resolve({ type: "ir.actions.act_window", res_model: "workflow.process.wizard", target: "new" });
        return Promise.resolve(false);
      }
    },
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(action);
      },
      doAction(action) {
        approvalButtonCalls.push({ action });
        return Promise.resolve(action);
      }
    }
  },
  confirm(message) {
    approvalButtonCalls.push({ confirm: message });
    return true;
  },
  validateForm(context) {
    approvalValidation.push({ clicked: context.values.approved_button_clicked, button: context.button.attrs.name });
    return true;
  },
  onUpdate(name, value) {
    approvalUpdates.push({ name, value });
  },
  onRefresh() {
    approvalRefreshes += 1;
  }
});
const approvalForm = approvalFormWindow.children[1];
approvalForm.addEventListener("workflow:button", (event) => { approvalButtonEvent = event.detail; });
const formButtons = findAll(approvalForm, (node) => node.dataset?.workflowAction);
assert.deepEqual(formButtons.map((button) => button.textContent), ["Approve", "Route", "Wizard"]);
formButtons[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(approvalButtonCalls[0], { confirm: "Confirm approval?" });
assert.deepEqual(approvalUpdates, [{ name: "approved_button_clicked", value: 7 }]);
assert.deepEqual(approvalValidation, [{ clicked: 7, button: "approval_action_button" }]);
assert.deepEqual(approvalButtonCalls[1], { model: "purchase.order", method: "approval_action_button", args: [[42], 7], kwargs: { context: { lang: "en_US" } } });
assert.equal(approvalButtonCalls[2].action.tag, "soft_reload");
assert.equal(approvalRefreshes, 1);
assert.equal(approvalButtonEvent.method, "approval_action_button");
formButtons[1].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(approvalButtonCalls[3], { model: "purchase.order", method: "approval_transition_button", args: [[42], 9], kwargs: {} });
formButtons[2].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(approvalButtonCalls[4], { model: "purchase.order", method: "approval_transition_wizard", args: [[42], 12], kwargs: {} });
assert.equal(approvalButtonCalls[5].action.res_model, "workflow.process.wizard");
assert.deepEqual(approvalUpdates, [{ name: "approved_button_clicked", value: 7 }]);

const formActionButtonCalls = [];
const formActionButtonEvents = [];
let formActionButtonRefreshes = 0;
const formActionButtonWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Invoice" },
  activeView: "form",
  resModel: "account.move",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Number" } },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><header><button name="%(action_account_payment_register)d" type="action" string="Register" context="{&#39;default_move_type&#39;: &#39;out_invoice&#39;}"/><button name="base.action_direct" type="action" string="Direct"/><button name="55" type="action" string="Numeric"/></header><sheet><field name="name"/></sheet></form>`,
        id: 33
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 91, name: "INV/001" },
  services: {
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(typeof action === "object" ? action : { id: action });
      },
      doAction(action, options) {
        formActionButtonCalls.push({ action, options });
        return Promise.resolve(action);
      }
    }
  },
  onRefresh() {
    formActionButtonRefreshes += 1;
  }
});
const formActionButtonForm = formActionButtonWindow.children[1];
formActionButtonForm.addEventListener("workflow:action-button", (event) => formActionButtonEvents.push(event.detail));
const actionButtons = findAll(formActionButtonForm, (node) => node.dataset?.workflowAction);
assert.deepEqual(actionButtons.map((button) => button.textContent), ["Register", "Direct", "Numeric"]);
actionButtons[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(formActionButtonCalls[0].action, "action_account_payment_register");
assert.deepEqual(formActionButtonCalls[0].options.additionalContext, {
  default_move_type: "out_invoice",
  active_id: 91,
  active_ids: [91],
  active_model: "account.move",
  active_domain: []
});
actionButtons[1].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(formActionButtonCalls[1].action, "base.action_direct");
actionButtons[2].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(formActionButtonCalls[2].action, 55);
assert.equal(formActionButtonRefreshes, 3);
assert.deepEqual(formActionButtonEvents.map((event) => event.action), ["%(action_account_payment_register)d", "base.action_direct", "55"]);

const wizardButtonCalls = [];
const wizardUpdates = [];
const wizardValues = { id: 51, comment: "" };
const wizardWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Workflow Process" },
  activeView: "form",
  resModel: "workflow.process.wizard",
  viewDescriptions: {
    fields: {
      comment: { type: "text", string: "Comment", required: true }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="comment"/></sheet><footer><button name="action_process" type="object" string="Process"/></footer></form>`,
        id: 33
      }
    }
  },
  records: [],
  length: 0
}, {
  values: wizardValues,
  services: {
    dataset: {
      callButton(model, method, args, kwargs) {
        wizardButtonCalls.push({ model, method, args, kwargs });
        return Promise.resolve(true);
      }
    }
  },
  onUpdate(name, value) {
    wizardUpdates.push({ name, value });
  }
});
const wizardForm = wizardWindow.children[1];
const wizardComment = findAll(wizardForm, (node) => node.dataset?.requiredField === "comment")[0];
const wizardProcess = findAll(wizardForm, (node) => node.dataset?.workflowAction === "action_process")[0];
assert.equal(wizardComment.tag, "textarea");
assert.equal(wizardComment.required, true);
assert.equal(wizardComment.getAttribute("aria-required"), "true");
assert.equal(wizardComment.getAttribute("aria-invalid"), "false");
wizardProcess.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(wizardButtonCalls, []);
assert.equal(wizardComment.getAttribute("aria-invalid"), "true");
assert.equal(wizardComment.className.includes("is-invalid"), true);
assert.equal(wizardComment.focused, true);
wizardComment.value = "Approved with comment";
wizardComment.dispatchEvent(new TestEvent("input"));
assert.equal(wizardValues.comment, "Approved with comment");
assert.deepEqual(wizardUpdates, [{ name: "comment", value: "Approved with comment" }]);
assert.equal(wizardComment.getAttribute("aria-invalid"), "false");
wizardProcess.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(wizardButtonCalls, [{ model: "workflow.process.wizard", method: "action_process", args: [[51]], kwargs: {} }]);

const workflowActionCalls = [];
user.isSystem = true;
const workflowActionWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Purchase Order" },
  activeView: "form",
  resModel: "purchase.order",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      state: { type: "selection", string: "Status" },
      user_can_approve: { type: "boolean", string: "Can Approve" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><header><field name="state"/><field name="user_can_approve" invisible="1"/></header><sheet><field name="name"/></sheet></form>`,
        id: 32
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 45, name: "PO003", state: "pending", user_can_approve: true },
  services: {
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(action);
      },
      doAction(action, options) {
        workflowActionCalls.push({ action, options });
        return Promise.resolve(action);
      }
    }
  },
  onRefresh() {
    workflowActionCalls.push({ refresh: true });
  }
});
const workflowActionForm = workflowActionWindow.children[1];
const workflowMenuButtons = findAll(workflowActionForm, (node) => node.dataset?.workflowAction === "update_status" || node.dataset?.workflowAction === "approval_log");
assert.deepEqual(workflowMenuButtons.map((button) => button.dataset.workflowAction), ["approval_log", "update_status"]);
assert.deepEqual(workflowMenuButtons.map((button) => [button.textContent, button.dataset.sequence, button.dataset.icon]), [["Approval Log", "100", "fa fa-arrows-h"], ["Update Status", "100", "fa fa-code"]]);
assert.deepEqual(workflowMenuButtons.map((button) => button.children[0].className), ["fa fa-arrows-h", "fa fa-code"]);
workflowMenuButtons[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(workflowActionCalls[0].action.res_model, "approval.log");
assert.deepEqual(workflowActionCalls[0].action.domain, [["model", "=", "purchase.order"], ["record_id", "=", 45]]);
assert.deepEqual(workflowActionCalls[0].action.context, { hide_record: true, hide_model: true });
workflowMenuButtons[1].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(workflowActionCalls[1].action.res_model, "approval.state.update");
assert.deepEqual(workflowActionCalls[1].action.context, { default_res_model: "purchase.order", default_res_ids: [45] });
workflowActionCalls[1].options.onClose();
assert.deepEqual(workflowActionCalls[2], { refresh: true });
const workflowListCalls = [];
const workflowListWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Purchase Orders" },
  activeView: "list",
  resModel: "purchase.order",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      state: { type: "selection", string: "Status" },
      user_can_approve: { type: "boolean", string: "Can Approve" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="state"/><field name="user_can_approve"/></list>`,
        id: 34
      }
    }
  },
  records: [{ id: 61, name: "PO061", state: "pending", user_can_approve: true }, { id: 62, name: "PO062", state: "draft", user_can_approve: true }],
  length: 2
}, {
  services: {
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(action);
      },
      doAction(action, options) {
        workflowListCalls.push({ action, options });
        return Promise.resolve(action);
      }
    }
  }
});
const workflowListShell = workflowListWindow.children[1];
const workflowListButtons = findAll(workflowListWindow, (node) => node.dataset?.workflowAction === "update_status" || node.dataset?.workflowAction === "approve_log");
assert.deepEqual(workflowListButtons.map((button) => button.dataset.workflowAction), ["update_status", "approve_log"]);
assert.deepEqual(workflowListButtons.map((button) => [button.textContent, button.disabled, button.dataset.sequence, button.dataset.icon]), [["Update Status", true, "100", "fa fa-code"], ["Approval Log", true, "120", "fa fa-arrows-h"]]);
assert.deepEqual(workflowListButtons.map((button) => button.children[0].className), ["fa fa-code", "fa fa-arrows-h"]);
const workflowListCheckboxes = findAll(workflowListShell, (node) => node.tag === "input" && node.type === "checkbox" && node.dataset?.recordId);
workflowListCheckboxes[0].checked = true;
workflowListCheckboxes[0].dispatchEvent(new TestEvent("change"));
assert.deepEqual(workflowListButtons.map((button) => button.disabled), [false, false]);
workflowListButtons[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(workflowListCalls[0].action.context, { default_res_model: "purchase.order", default_res_ids: [61] });
workflowListButtons[1].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(workflowListCalls[1].action.domain, [["model", "=", "purchase.order"], ["record_id", "in", [61]]]);
assert.deepEqual(workflowListCalls[1].action.context, { hide_record: false, hide_model: true });
const workflowFullToolbarWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Purchase Orders" },
  activeView: "list",
  resModel: "purchase.order",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      state: { type: "selection", string: "Status" },
      user_can_approve: { type: "boolean", string: "Can Approve" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list show_action_approve_all="true"><field name="name"/><field name="state"/><field name="user_can_approve"/></list>`,
        id: 35
      }
    }
  },
  records: [{ id: 63, name: "PO063", state: "pending", user_can_approve: true }],
  length: 1
});
const workflowFullToolbar = findAll(workflowFullToolbarWindow, (node) => String(node.className ?? "").includes("gorp-list-toolbar"))[0];
const workflowFullButtons = findAll(workflowFullToolbar, (node) => node.dataset?.workflowAction);
assert.deepEqual(workflowFullButtons.map((button) => [button.dataset.workflowAction, button.dataset.sequence, button.dataset.icon, button.textContent]), [
  ["update_status", "100", "fa fa-code", "Update Status"],
  ["approve", "110", "fa fa-thumbs-up", "Approve"],
  ["approve_log", "120", "fa fa-arrows-h", "Approval Log"]
]);
assert.ok(String(workflowFullToolbar.className).includes("o_cp_action_menus"));
assert.deepEqual(findAll(workflowFullToolbar, (node) => node.dataset?.menu).map((node) => node.dataset.menu), ["action"]);
const serverActionMenuCalls = [];
let serverActionMenuRefreshes = 0;
const serverActionMenuWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Partners" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" } },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/></list>`,
        id: 36,
        actionMenus: {
          action: [{ id: 310, name: "Server Export", type: "ir.actions.server" }],
          print: [{ id: 320, name: "Partner Label", type: "ir.actions.report", icon: "fa fa-file-pdf-o" }]
        }
      }
    }
  },
  records: [{ id: 81, name: "Alpha" }, { id: 82, name: "Beta" }],
  length: 2
}, {
  services: {
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(typeof action === "object" ? action : { id: action });
      },
      doAction(action, options) {
        serverActionMenuCalls.push({ action, options });
        return Promise.resolve(action);
      }
    }
  },
  onRefresh() {
    serverActionMenuRefreshes += 1;
  }
});
const serverActionMenuShell = serverActionMenuWindow.children[1];
const serverActionMenu = findAll(serverActionMenuWindow, (node) => String(node.className ?? "").includes("gorp-action-menus"))[0];
assert.deepEqual(findAll(serverActionMenu, (node) => node.dataset?.menu).map((node) => node.dataset.menu), ["print", "action"]);
const serverPrintSection = findAll(serverActionMenu, (node) => String(node.className ?? "").includes("gorp-action-menu-section") && node.dataset?.menu === "print")[0];
const serverActionSection = findAll(serverActionMenu, (node) => String(node.className ?? "").includes("gorp-action-menu-section") && node.dataset?.menu === "action")[0];
const serverExportButton = findAll(serverActionMenu, (node) => node.dataset?.actionId === "310")[0];
const serverPrintToggle = findAll(serverActionMenu, (node) => node.dataset?.actionMenuToggle === "print")[0];
const serverActionToggle = findAll(serverActionMenu, (node) => node.dataset?.actionMenuToggle === "action")[0];
const serverActionItems = findAll(serverActionSection, (node) => node.dataset?.actionMenuItems === "action")[0];
const serverPrintItems = findAll(serverPrintSection, (node) => node.dataset?.actionMenuItems === "print")[0];
assert.equal(findAll(serverActionMenu, (node) => node.dataset?.actionId === "320").length, 0);
assert.deepEqual([serverExportButton.disabled], [true]);
serverPrintToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(serverPrintSection.dataset.open, "true");
serverActionToggle.dispatchEvent(new TestEvent("click"));
assert.equal(serverActionSection.dataset.open, "true");
assert.equal(serverPrintSection.dataset.open, "false");
assert.equal(findAll(serverActionMenu, (node) => String(node.className ?? "").includes("gorp-action-menu-section") && node.dataset?.open === "true").length, 1);
globalThis.document.dispatchEvent(new TestEvent("click"));
assert.equal(serverActionSection.dataset.open, "false");
assert.equal(serverPrintSection.dataset.open, "false");
const serverActionMenuCheckbox = findAll(serverActionMenuShell, (node) => node.tag === "input" && node.type === "checkbox" && node.dataset?.recordId === "82")[0];
serverActionMenuCheckbox.checked = true;
serverActionMenuCheckbox.dispatchEvent(new TestEvent("change"));
assert.deepEqual([serverExportButton.disabled], [false]);
serverPrintToggle.focus();
serverActionMenu.dispatchEvent(new TestEvent("keydown", { key: "U", shiftKey: true }));
await new Promise((resolve) => setTimeout(resolve, 0));
const serverHotkeyPrintButton = findAll(serverActionMenu, (node) => node.dataset?.actionId === "320")[0];
assert.equal(serverPrintSection.dataset.open, "true");
assert.equal(serverPrintToggle.attributes["aria-expanded"], "true");
assert.equal(globalThis.document.activeElement, serverHotkeyPrintButton);
serverPrintItems.dispatchEvent(new TestEvent("keydown", { key: "Escape" }));
assert.equal(serverPrintSection.dataset.open, "false");
assert.equal(globalThis.document.activeElement, serverPrintToggle);
serverActionToggle.dispatchEvent(new TestEvent("keydown", { key: "ArrowDown" }));
assert.equal(serverActionSection.dataset.open, "true");
assert.equal(globalThis.document.activeElement, serverExportButton);
serverActionItems.dispatchEvent(new TestEvent("keydown", { key: "End" }));
assert.equal(globalThis.document.activeElement, serverExportButton);
serverActionItems.dispatchEvent(new TestEvent("keydown", { key: "Escape" }));
assert.equal(serverActionSection.dataset.open, "false");
assert.equal(globalThis.document.activeElement, serverActionToggle);
serverActionMenu.dispatchEvent(new TestEvent("keydown", { key: "u" }));
assert.equal(serverActionSection.dataset.open, "true");
assert.equal(serverPrintSection.dataset.open, "false");
assert.equal(globalThis.document.activeElement, serverExportButton);
serverActionItems.dispatchEvent(new TestEvent("keydown", { key: "Enter" }));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(serverActionSection.dataset.open, "false");
assert.equal(serverActionMenuCalls[0].action, 310);
assert.deepEqual(serverActionMenuCalls[0].options.additionalContext, {
  active_id: 82,
  active_ids: [82],
  active_model: "res.partner",
  active_domain: []
});
serverActionMenuCalls[0].options.onClose();
assert.equal(serverActionMenuRefreshes, 1);
serverPrintToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const serverPrintButton = findAll(serverActionMenu, (node) => node.dataset?.actionId === "320")[0];
assert.equal(serverPrintButton.dataset.icon, "fa fa-file-pdf-o");
assert.equal(serverPrintButton.disabled, false);
serverPrintButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(serverActionMenuCalls[1].action, 320);
assert.deepEqual(serverActionMenuCalls[1].options.additionalContext.active_ids, [82]);
serverActionMenuCheckbox.checked = false;
serverActionMenuCheckbox.dispatchEvent(new TestEvent("change"));
assert.equal(findAll(serverActionMenu, (node) => node.dataset?.actionId === "320").length, 0);
const actionMenuRunRequests = [];
let actionMenuRunRefreshes = 0;
const actionMenuRunServices = createWebClientServices({
  transport(request) {
    actionMenuRunRequests.push(request);
    if (request.route === "/web/action/load") {
      return Promise.resolve({ id: 710, type: "ir.actions.server", name: "Run Server Action" });
    }
    if (request.route === "/web/action/run") {
      return Promise.resolve(false);
    }
    return Promise.resolve({});
  }
});
const actionMenuRunWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Server Actions" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" } },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/></list>`,
        id: 710,
        actionMenus: { action: [{ id: 710, name: "Run Server Action", type: "ir.actions.server" }] }
      }
    }
  },
  records: [{ id: 91, name: "Action Partner" }],
  length: 1
}, {
  services: actionMenuRunServices,
  onRefresh() {
    actionMenuRunRefreshes += 1;
  }
});
const actionMenuRunButton = findAll(actionMenuRunWindow, (node) => node.dataset?.actionId === "710")[0];
const actionMenuRunCheckbox = findAll(actionMenuRunWindow, (node) => node.tag === "input" && node.type === "checkbox")[0];
actionMenuRunCheckbox.checked = true;
actionMenuRunCheckbox.dispatchEvent(new TestEvent("change"));
actionMenuRunButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(actionMenuRunRequests[0].route, "/web/action/load");
assert.equal(actionMenuRunRequests[1].route, "/web/action/run");
assert.deepEqual(actionMenuRunRequests[1].params.context, {
  active_id: 91,
  active_ids: [91],
  active_model: "res.partner",
  active_domain: []
});
assert.equal(actionMenuRunRefreshes, 1);
const domainSelectedSearches = [];
const domainSelectedActions = [];
const domainSelectedReportCalls = [];
const domainSelectedReportID = "ir.actions.report,316";
const domainSelectedWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Partners" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" } },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/></list>`,
        id: 37,
        actionMenus: {
          action: [{ id: 315, name: "Domain Action" }],
          print: [{ id: domainSelectedReportID, name: "Domain Partner Label", domain: `[("active","=",True)]` }]
        }
      }
    }
  },
  records: [{ id: 81, name: "Alpha" }, { id: 82, name: "Beta" }],
  length: 2
}, {
  activeDomain: [["active", "=", true]],
  isDomainSelected: true,
  activeIdsLimit: 500,
  context: { lang: "en_US" },
  services: {
    orm: {
      search(model, domain, kwargs) {
        domainSelectedSearches.push({ model, domain, kwargs });
        return Promise.resolve([81, 82]);
      },
      call(model, method, args) {
        domainSelectedReportCalls.push({ model, method, args });
        return Promise.resolve([domainSelectedReportID]);
      }
    },
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(typeof action === "object" ? action : { id: action });
      },
      doAction(action, options) {
        domainSelectedActions.push({ action, options });
        return Promise.resolve(action);
      }
    }
  }
});
const domainSelectedButton = findAll(domainSelectedWindow, (node) => node.dataset?.actionId === "315")[0];
assert.equal(domainSelectedButton.disabled, false);
domainSelectedButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(domainSelectedSearches[0], { model: "res.partner", domain: [["active", "=", true]], kwargs: { limit: 500, context: { lang: "en_US" } } });
assert.equal(domainSelectedActions[0].action, 315);
assert.deepEqual(domainSelectedActions[0].options.additionalContext, {
  active_id: 81,
  active_ids: [81, 82],
  active_model: "res.partner",
  active_domain: [["active", "=", true]]
});
const domainSelectedPrintToggle = findAll(domainSelectedWindow, (node) => node.dataset?.actionMenuToggle === "print")[0];
domainSelectedPrintToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(domainSelectedSearches[1], { model: "res.partner", domain: [["active", "=", true]], kwargs: { limit: 500, context: { lang: "en_US" } } });
assert.deepEqual(domainSelectedReportCalls[0], {
  model: "ir.actions.report",
  method: "get_valid_action_reports",
  args: [[domainSelectedReportID], "res.partner", [81, 82]]
});
const domainSelectedPrintButton = findAll(domainSelectedWindow, (node) => node.dataset?.actionId === domainSelectedReportID)[0];
assert.equal(domainSelectedPrintButton.disabled, false);
const reportDomainCalls = [];
const reportDomainActionCalls = [];
const domainReportID = "ir.actions.report,330";
let validDomainReports = [];
const reportDomainWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Partners" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" } },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/></list>`,
        id: 38,
        actionMenus: {
          print: [{ id: domainReportID, name: "Active Partner Label", domain: `[("active","=",True)]` }]
        }
      }
    }
  },
  records: [{ id: 84, name: "Delta" }],
  length: 1
}, {
  services: {
    orm: {
      call(model, method, args) {
        reportDomainCalls.push({ model, method, args });
        return Promise.resolve(validDomainReports);
      }
    },
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(typeof action === "object" ? action : { id: action });
      },
      doAction(action, options) {
        reportDomainActionCalls.push({ action, options });
        return Promise.resolve(action);
      }
    }
  }
});
let printLoadedEvent;
const reportDomainShell = reportDomainWindow.children[1];
reportDomainShell.addEventListener("action-menu:print-loaded", (event) => { printLoadedEvent = event.detail; });
const reportDomainCheckbox = findAll(reportDomainShell, (node) => node.tag === "input" && node.type === "checkbox")[0];
reportDomainCheckbox.checked = true;
reportDomainCheckbox.dispatchEvent(new TestEvent("change"));
const reportDomainToggle = findAll(reportDomainWindow, (node) => node.dataset?.actionMenuToggle === "print")[0];
assert.equal(reportDomainCalls.length, 0);
reportDomainToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(reportDomainCalls[0], { model: "ir.actions.report", method: "get_valid_action_reports", args: [[domainReportID], "res.partner", [84]] });
assert.equal(reportDomainCalls.length, 1);
assert.equal(reportDomainActionCalls.length, 0);
assert.deepEqual(printLoadedEvent.availableIds, []);
const emptyPrintItem = findAll(reportDomainWindow, (node) => node.dataset?.actionMenuEmpty === "print")[0];
assert.equal(emptyPrintItem.textContent, "No report available.");
assert.equal(emptyPrintItem.disabled, true);
const reportDomainPrintSection = findAll(reportDomainWindow, (node) => String(node.className ?? "").includes("gorp-action-menu-section") && node.dataset?.menu === "print")[0];
const reportDomainPrintItems = findAll(reportDomainPrintSection, (node) => node.dataset?.actionMenuItems === "print")[0];
reportDomainPrintItems.dispatchEvent(new TestEvent("keydown", { key: "ArrowDown" }));
assert.notEqual(globalThis.document.activeElement, emptyPrintItem);
reportDomainPrintItems.dispatchEvent(new TestEvent("keydown", { key: "Escape" }));
assert.equal(reportDomainToggle.attributes["aria-expanded"], "false");
assert.equal(globalThis.document.activeElement, reportDomainToggle);
validDomainReports = [domainReportID];
reportDomainToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(reportDomainCalls[1], { model: "ir.actions.report", method: "get_valid_action_reports", args: [[domainReportID], "res.partner", [84]] });
assert.equal(reportDomainCalls.length, 2);
assert.equal(reportDomainToggle.attributes["aria-expanded"], "true");
reportDomainToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(reportDomainCalls.length, 2);
assert.equal(reportDomainToggle.attributes["aria-expanded"], "false");
reportDomainToggle.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(reportDomainCalls[2], { model: "ir.actions.report", method: "get_valid_action_reports", args: [[domainReportID], "res.partner", [84]] });
assert.equal(reportDomainCalls.length, 3);
const reportDomainButton = findAll(reportDomainWindow, (node) => node.dataset?.actionId === domainReportID)[0];
assert.deepEqual(printLoadedEvent.availableIds, [domainReportID]);
reportDomainButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(reportDomainActionCalls[0].action, domainReportID);
  assert.deepEqual(reportDomainActionCalls[0].options.additionalContext.active_ids, [84]);
  const staticActionOrmCalls = [];
  const staticActionSearchReads = [];
  const staticActionReads = [];
  const staticActionCreates = [];
  const staticActionUnlinks = [];
  const staticActionConfirms = [];
  const staticActionDownloads = [];
  const staticActionFieldRequests = [];
  const staticActionNamelists = [];
  const staticActionNotifications = [];
let staticActionRefreshes = 0;
const staticActionEvents = [];
const staticActionWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Partners" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
      fields: {
        name: { type: "char", string: "Name" },
        active: { type: "boolean", string: "Active" },
        email: { type: "char", string: "Email", default_export_compatible: true },
        company_id: { type: "many2one", string: "Company" }
      },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="active"/></list>`,
        id: 39
      }
    }
  },
  records: [{ id: 85, name: "Echo", active: true }],
  length: 1
}, {
  services: {
    orm: {
        call(model, method, args) {
          staticActionOrmCalls.push({ model, method, args });
          return Promise.resolve(method === "copy" ? [186] : true);
        },
        searchRead(model, domain, fields) {
          staticActionSearchReads.push({ model, domain, fields });
          return Promise.resolve([{ id: 501, name: "Saved Export", export_fields: [701, 702, 703] }]);
        },
        read(model, ids, fields) {
          staticActionReads.push({ model, ids, fields });
          return Promise.resolve([{ name: "email" }, { name: "company_id/name" }]);
        },
        create(model, records) {
          staticActionCreates.push({ model, records });
          return Promise.resolve([777]);
        },
        unlink(model, ids) {
          staticActionUnlinks.push({ model, ids });
          return Promise.resolve(true);
      }
    },
    notification: {
      calls: staticActionNotifications,
      add(message, options) {
        staticActionNotifications.push({ message, ...options });
      }
    }
  },
  confirm(message) {
    staticActionConfirms.push(message);
    return true;
  },
  exportDownload(request) {
    staticActionDownloads.push(request);
    return Promise.resolve({ ok: true, filename: "res_partner.csv" });
  },
  exportGetFields(request) {
    staticActionFieldRequests.push(request);
    if (request.model === "res.company") {
      return Promise.resolve([
        { id: "company_id/name", value: "company_id/name", string: "Company/Name", field_type: "char" },
        { id: "company_id/industry_id", value: "company_id/industry_id/id", string: "Company/Industry", field_type: "many2one", relation: "res.partner.industry", children: true, params: { model: "res.partner.industry", prefix: "company_id/industry_id" } }
      ]);
    }
    if (request.model === "res.partner.industry") {
      return Promise.resolve([
        { id: "company_id/industry_id/name", value: "company_id/industry_id/name", string: "Company/Industry/Name", field_type: "char" },
        { id: "company_id/industry_id/code", value: "company_id/industry_id/code", string: "Code", field_type: "char" },
        { id: "company_id/industry_id/category_id", value: "company_id/industry_id/category_id/id", string: "Company/Industry/Category", field_type: "many2one", relation: "res.partner.industry.category", children: true, params: { model: "res.partner.industry.category", prefix: "company_id/industry_id/category_id" } }
      ]);
    }
    if (request.model !== "res.partner") return Promise.resolve([]);
    return Promise.resolve([
      { id: "name", value: "name", string: "Name", field_type: "char" },
      { id: "active", value: "active", string: "Active", field_type: "boolean" },
      { id: "email", value: "email", string: "Email", field_type: "char", default_export: true },
      { id: "company_id", value: "company_id/id", string: "Company", field_type: "many2one", relation: "res.company", relation_field: "partner_ids", children: true, params: { model: "res.company", prefix: "company_id" } }
    ]);
  },
  exportNamelist(request) {
    staticActionNamelists.push(request);
    return Promise.resolve([
      { id: "email", name: "email", value: "email", string: "Email", field_type: "char" },
      { id: "company_id/name", name: "company_id/name", value: "company_id/name", string: "Company/Name", field_type: "char" }
    ]);
  },
  debug: true,
  activeGroupBy: ["active"],
  onRefresh() {
    staticActionRefreshes += 1;
  }
});
const staticActionShell = staticActionWindow.children[1];
for (const eventName of ["action-menu:export", "action-menu:duplicate", "action-menu:archive", "action-menu:unarchive", "action-menu:delete"]) {
  staticActionShell.addEventListener(eventName, (event) => staticActionEvents.push({ type: event.type, detail: event.detail }));
}
const staticButtons = findAll(staticActionWindow, (node) => node.dataset?.staticAction);
assert.deepEqual(staticButtons.map((button) => [button.dataset.staticAction, button.dataset.sequence, button.dataset.icon, button.disabled]), [
  ["export", "10", "fa fa-upload", true],
  ["duplicate", "30", "fa fa-clone", true],
  ["archive", "40", "oi oi-archive", true],
  ["unarchive", "45", "oi oi-unarchive", true],
  ["delete", "50", "fa fa-trash-o", true]
]);
assert.ok(String(staticButtons[4].className).includes("text-danger"));
const staticCheckbox = findAll(staticActionShell, (node) => node.tag === "input" && node.type === "checkbox")[0];
staticCheckbox.checked = true;
staticCheckbox.dispatchEvent(new TestEvent("change"));
  assert.deepEqual(staticButtons.map((button) => button.disabled), [false, false, false, false, false]);
  staticButtons[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  const exportDialog = findAll(staticActionShell, (node) => node.dataset?.exportDialog === "res.partner").at(-1);
  assert.ok(exportDialog);
  const selectedExportFields = () => findAll(exportDialog, (node) => node.dataset?.exportField).map((node) => node.dataset.exportField);
  const selectedExportRows = () => findAll(exportDialog, (node) => node.dataset?.exportField);
  assert.deepEqual(selectedExportFields(), ["name", "active", "email"]);
  assert.deepEqual(selectedExportRows().map((node) => [node.dataset.field_id, node.getAttribute("draggable"), String(node.className).includes("o_export_field_sortable")]), [
    ["name", "true", true],
    ["active", "true", true],
    ["email", "true", true]
  ]);
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportTreeItem === "name")[0].dataset.field_id, "name");
  assert.deepEqual(findAll(exportDialog, (node) => node.dataset?.exportSortField).map((node) => node.dataset.exportSortField), ["name", "active", "email"]);
  assert.ok(findAll(exportDialog, (node) => node.dataset?.exportSortField === "name")[0].className.includes("mx-1"));
  assert.equal(exportDialog.dataset.exportIsSmall, "false");
  globalThis.window.innerWidth = 390;
  globalThis.window.dispatchEvent(new TestEvent("resize"));
  assert.equal(exportDialog.dataset.exportIsSmall, "true");
  assert.equal(findAll(exportDialog, (node) => String(node.className).includes("o_export_field_sortable")).length, 0);
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportSortField).length, 0);
  globalThis.window.innerWidth = 1024;
  globalThis.window.dispatchEvent(new TestEvent("resize"));
  assert.equal(exportDialog.dataset.exportIsSmall, "false");
  assert.equal(findAll(exportDialog, (node) => String(node.className).includes("o_export_field_sortable")).length, 3);
  assert.deepEqual(findAll(exportDialog, (node) => node.dataset?.exportSortField).map((node) => node.dataset.exportSortField), ["name", "active", "email"]);
  selectedExportRows()[0].dispatchEvent(new TestEvent("dragstart"));
  selectedExportRows()[2].dispatchEvent(new TestEvent("drop", { detail: { previousField: "active", nextField: "email" } }));
  assert.deepEqual(selectedExportFields(), ["active", "name", "email"]);
  selectedExportRows().find((node) => node.dataset.exportField === "name").dispatchEvent(new TestEvent("dragstart"));
  selectedExportRows().find((node) => node.dataset.exportField === "active").dispatchEvent(new TestEvent("drop", { detail: { previousField: "email", nextField: null } }));
  assert.deepEqual(selectedExportFields(), ["active", "email", "name"]);
  selectedExportRows().find((node) => node.dataset.exportField === "name").dispatchEvent(new TestEvent("dragstart"));
  selectedExportRows().find((node) => node.dataset.exportField === "email").dispatchEvent(new TestEvent("drop", { detail: { previousField: null, nextField: "active" } }));
  assert.deepEqual(selectedExportFields(), ["name", "active", "email"]);
  const companyExpandButton = findAll(exportDialog, (node) => node.dataset?.exportExpandField === "company_id")[0];
  assert.ok(companyExpandButton);
  companyExpandButton.dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  const companyFieldRequest = staticActionFieldRequests.find((request) => request.model === "res.company");
  assert.deepEqual(companyFieldRequest, {
    model: "res.company",
    prefix: "company_id",
    parent_name: "Company",
    import_compat: false,
    parent_field_type: "many2one",
    parent_field: {
      id: "company_id",
      name: "company_id",
      string: "Company",
      value: "company_id/id",
      type: "many2one",
      field_type: "many2one",
      relation: "res.company",
      children: true,
      params: { model: "res.company", prefix: "company_id" },
      relation_field: "partner_ids"
    },
    exclude: ["partner_ids"],
    domain: []
  });
  assert.ok(findAll(exportDialog, (node) => node.dataset?.exportAddField === "company_id/name")[0]);
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportExpandField === "company_id/industry_id").length, 1);
  findAll(exportDialog, (node) => node.dataset?.exportExpandField === "company_id/industry_id")[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  const industryFieldRequest = staticActionFieldRequests.find((request) => request.model === "res.partner.industry");
  assert.deepEqual(industryFieldRequest, {
    model: "res.partner.industry",
    prefix: "company_id/industry_id",
    parent_name: "Company/Industry",
    import_compat: false,
    parent_field_type: "many2one",
    parent_field: {
      id: "company_id/industry_id",
      name: "company_id/industry_id",
      string: "Company/Industry",
      value: "company_id/industry_id/id",
      type: "many2one",
      field_type: "many2one",
      relation: "res.partner.industry",
      children: true,
      params: { model: "res.partner.industry", prefix: "company_id/industry_id" }
    },
    exclude: [],
    domain: []
  });
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportExpandField === "company_id/industry_id/category_id").length, 0);
  findAll(exportDialog, (node) => node.dataset?.exportExpandField === "company_id/industry_id")[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  const exportSearch = findAll(exportDialog, (node) => node.dataset?.exportSearch)[0];
  assert.ok(exportSearch);
  const searchRequestCount = staticActionFieldRequests.length;
  exportSearch.value = "Industry";
  exportSearch.dispatchEvent(new TestEvent("input"));
  assert.equal(staticActionFieldRequests.length, searchRequestCount);
  assert.deepEqual(findAll(exportDialog, (node) => node.dataset?.exportTreeItem).map((node) => node.dataset.exportTreeItem), ["company_id", "company_id/industry_id", "company_id/industry_id/name", "company_id/industry_id/category_id"]);
  assert.ok(!findAll(exportDialog, (node) => node.dataset?.exportTreeItem).some((node) => node.dataset.exportTreeItem === "company_id/industry_id/code"));
  findAll(exportDialog, (node) => node.dataset?.exportExpandField === "company_id/industry_id")[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.ok(findAll(exportDialog, (node) => node.dataset?.exportTreeItem).some((node) => node.dataset.exportTreeItem === "company_id/industry_id/code"));
  exportSearch.value = "Ind Cat";
  exportSearch.dispatchEvent(new TestEvent("input"));
  assert.deepEqual(findAll(exportDialog, (node) => node.dataset?.exportTreeItem).map((node) => node.dataset.exportTreeItem), ["company_id", "company_id/industry_id", "company_id/industry_id/category_id"]);
  exportSearch.value = "industry_id/category_id";
  exportSearch.dispatchEvent(new TestEvent("input"));
  assert.deepEqual(findAll(exportDialog, (node) => node.dataset?.exportTreeItem).map((node) => node.dataset.exportTreeItem), ["company_id", "company_id/industry_id", "company_id/industry_id/category_id"]);
  exportSearch.value = "No Such Field";
  exportSearch.dispatchEvent(new TestEvent("input"));
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportNoMatch).length, 1);
  exportSearch.value = "";
  exportSearch.dispatchEvent(new TestEvent("input"));
  findAll(exportDialog, (node) => node.dataset?.exportTreeItem === "company_id/name")[0].dispatchEvent(new TestEvent("dblclick"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.ok(findAll(exportDialog, (node) => node.dataset?.exportField).some((node) => node.dataset.exportField === "company_id/name"));
  findAll(exportDialog, (node) => node.dataset?.exportAddField === "company_id/industry_id")[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  findAll(exportDialog, (node) => node.dataset?.exportConfirm)[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(staticActionDownloads[0].importCompat, false);
  assert.deepEqual(staticActionDownloads[0].fields.map((field) => field.name), ["name", "active", "email", "company_id/name", "company_id/industry_id/id"]);
  staticActionEvents.length = 0;
  findAll(exportDialog, (node) => node.dataset?.exportRemoveField === "company_id/name")[0].dispatchEvent(new TestEvent("click"));
  assert.ok(!findAll(exportDialog, (node) => node.dataset?.exportField).some((node) => node.dataset.exportField === "company_id/name"));
  const companyFetchCount = staticActionFieldRequests.filter((request) => request.model === "res.company").length;
  companyExpandButton.dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  findAll(exportDialog, (node) => node.dataset?.exportExpandField === "company_id")[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(staticActionFieldRequests.filter((request) => request.model === "res.company").length, companyFetchCount);
  assert.deepEqual(staticActionSearchReads, [{
    model: "ir.exports",
    domain: [["resource", "=", "res.partner"]],
    fields: ["id", "name", "export_fields"]
  }]);
  const exportImportCheckbox = findAll(exportDialog, (node) => node.dataset?.exportImportCompat)[0];
  exportImportCheckbox.checked = true;
  exportImportCheckbox.dispatchEvent(new TestEvent("change"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportSearch.value, "");
  assert.deepEqual(findAll(exportDialog, (node) => node.dataset?.exportField).map((node) => node.dataset.exportField), ["name", "active", "email"]);
  const exportTemplateSelect = findAll(exportDialog, (node) => node.dataset?.exportTemplateSelect)[0];
  exportTemplateSelect.value = "501";
  exportTemplateSelect.dispatchEvent(new TestEvent("change"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.deepEqual(staticActionReads, []);
  assert.deepEqual(staticActionNamelists[0], { model: "res.partner", export_id: 501 });
  assert.equal(exportDialog.dataset.exportTemplateId, "501");
  assert.equal(exportDialog.dataset.exportTemplateEditing, "false");
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportTemplateDelete)[0].getAttribute("hidden"), null);
  selectedExportRows().find((node) => node.dataset.exportField === "company_id/name").dispatchEvent(new TestEvent("dragstart"));
  selectedExportRows().find((node) => node.dataset.exportField === "company_id/name").dispatchEvent(new TestEvent("drop", { detail: { previousField: null, nextField: "email" } }));
  assert.equal(exportDialog.dataset.exportTemplateEditing, "true");
  assert.deepEqual(selectedExportFields(), ["company_id/name", "email"]);
  findAll(exportDialog, (node) => node.dataset?.exportTemplateCancel)[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateEditing, "false");
  assert.deepEqual(selectedExportFields(), ["email", "company_id/name"]);
  findAll(exportDialog, (node) => node.dataset?.exportRemoveField === "company_id/name")[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateEditing, "true");
  assert.deepEqual(selectedExportFields(), ["email"]);
  findAll(exportDialog, (node) => node.dataset?.exportTemplateCancel)[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateEditing, "false");
  assert.deepEqual(staticActionNamelists.at(-1), { model: "res.partner", export_id: 501 });
  assert.deepEqual(selectedExportFields(), ["email", "company_id/name"]);
  findAll(exportDialog, (node) => node.dataset?.exportAddField === "company_id")[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateEditing, "true");
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportTemplateCancel)[0].getAttribute("hidden"), null);
  assert.ok(findAll(exportDialog, (node) => node.dataset?.exportField).some((node) => node.dataset.exportField === "company_id"));
  exportImportCheckbox.checked = true;
  exportImportCheckbox.dispatchEvent(new TestEvent("change"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateEditing, "false");
  assert.equal(findAll(exportDialog, (node) => node.dataset?.exportTemplateCancel)[0].getAttribute("hidden"), "");
  assert.ok(!findAll(exportDialog, (node) => node.dataset?.exportField).some((node) => node.dataset.exportField === "company_id"));
  assert.deepEqual(staticActionNamelists.at(-1), { model: "res.partner", export_id: 501 });
  assert.ok(staticActionFieldRequests.some((request) => request.model === "res.partner" && request.import_compat === true));
  const exportFormatSelect = findAll(exportDialog, (node) => node.dataset?.exportFormat)[0];
  exportFormatSelect.value = "csv";
  exportFormatSelect.dispatchEvent(new TestEvent("change"));
  exportTemplateSelect.value = "new_template";
  exportTemplateSelect.dispatchEvent(new TestEvent("change"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateId, "new_template");
  assert.equal(exportDialog.dataset.exportTemplateEditing, "true");
  assert.equal(exportTemplateSelect.getAttribute("hidden"), "");
  const exportSaveName = findAll(exportDialog, (node) => node.dataset?.exportTemplateName)[0];
  assert.equal(exportSaveName.getAttribute("hidden"), null);
  findAll(exportDialog, (node) => node.dataset?.exportTemplateSave)[0].dispatchEvent(new TestEvent("click"));
  assert.deepEqual(staticActionNotifications.at(-1), { message: "Please enter save field list name", type: "danger" });
  findAll(exportDialog, (node) => node.dataset?.exportTemplateCancel)[0].dispatchEvent(new TestEvent("click"));
  assert.equal(exportDialog.dataset.exportTemplateId, "");
  assert.equal(exportTemplateSelect.getAttribute("hidden"), null);
  exportSaveName.value = "Stale Name";
  exportTemplateSelect.value = "new_template";
  exportTemplateSelect.dispatchEvent(new TestEvent("change"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportSaveName.value, "");
  exportSaveName.value = "Selected Fields";
  findAll(exportDialog, (node) => node.dataset?.exportTemplateSave)[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateId, "777");
  assert.equal(exportDialog.dataset.exportTemplateEditing, "false");
  assert.deepEqual(staticActionCreates, [{
    model: "ir.exports",
    records: [{
      name: "Selected Fields",
      resource: "res.partner",
      export_fields: [[0, 0, { name: "email" }], [0, 0, { name: "company_id/name" }]]
    }]
  }]);
  findAll(exportDialog, (node) => node.dataset?.exportTemplateDelete)[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(exportDialog.dataset.exportTemplateId, "");
  assert.deepEqual(findAll(exportDialog, (node) => node.dataset?.exportField).map((node) => node.dataset.exportField), ["name", "active", "email"]);
  findAll(exportDialog, (node) => node.dataset?.exportConfirm)[0].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
  staticButtons[1].dispatchEvent(new TestEvent("click"));
  await new Promise((resolve) => setTimeout(resolve, 0));
staticButtons[2].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
staticButtons[3].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
staticButtons[4].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(staticActionOrmCalls, [
  { model: "res.partner", method: "copy", args: [[85], {}] },
  { model: "res.partner", method: "action_archive", args: [[85]] },
  { model: "res.partner", method: "action_unarchive", args: [[85]] }
  ]);
  assert.deepEqual(staticActionDownloads.at(-1).route, "/web/export/csv");
  assert.deepEqual(staticActionDownloads.at(-1).ids, [85]);
  assert.equal(staticActionDownloads.at(-1).importCompat, true);
  assert.deepEqual(staticActionDownloads.at(-1).groupby, ["active"]);
  assert.deepEqual(staticActionDownloads.at(-1).fields.map((field) => field.name), ["id", "name", "active", "email"]);
assert.deepEqual(staticActionUnlinks, [{ model: "ir.exports", ids: [777] }, { model: "res.partner", ids: [85] }]);
assert.equal(staticActionRefreshes, 4);
assert.deepEqual(staticActionEvents.map((event) => event.type), ["action-menu:export", "action-menu:duplicate", "action-menu:archive", "action-menu:unarchive", "action-menu:delete"]);
assert.equal(staticActionEvents[0].detail.result.ok, true);
assert.equal(staticActionConfirms.length, 3);
const parentTemplateFieldRequests = [];
const parentTemplateNamelists = [];
const parentTemplateDownloads = [];
const parentTemplateWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Parent Template Contacts" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      active: { type: "boolean", string: "Active" },
      company_id: { type: "many2one", string: "Company", relation: "res.company" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="active"/><field name="company_id"/></list>`,
        id: 15
      }
    }
  },
  records: [{ id: 90, name: "Parent", active: true }],
  length: 1
}, {
  services: {
    orm: {
      searchRead(model, domain, fields) {
        assert.deepEqual({ model, domain, fields }, {
          model: "ir.exports",
          domain: [["resource", "=", "res.partner"]],
          fields: ["id", "name", "export_fields"]
        });
        return Promise.resolve([{ id: 601, name: "Nested Template", export_fields: [801] }]);
      }
    }
  },
  exportGetFields(request) {
    parentTemplateFieldRequests.push(request);
    if (request.model === "res.company") {
      return Promise.resolve([
        { id: "company_id/industry_id", value: "company_id/industry_id/id", string: "Company/Industry", field_type: "many2one", relation: "res.partner.industry", children: true, params: { model: "res.partner.industry", prefix: "company_id/industry_id" } }
      ]);
    }
    if (request.model === "res.partner.industry") {
      return Promise.resolve([
        { id: "company_id/industry_id/name", value: "company_id/industry_id/name", string: "Company/Industry/Name", field_type: "char" }
      ]);
    }
    return Promise.resolve([
      { id: "name", value: "name", string: "Name", field_type: "char" },
      { id: "active", value: "active", string: "Active", field_type: "boolean" },
      { id: "company_id", value: "company_id/id", string: "Company", field_type: "many2one", relation: "res.company", relation_field: "partner_ids", children: true, params: { model: "res.company", prefix: "company_id" } }
    ]);
  },
  exportNamelist(request) {
    parentTemplateNamelists.push(request);
    return Promise.resolve([
      { id: "company_id/industry_id", name: "company_id/industry_id", value: "company_id/industry_id", string: "company_id/industry_id", field_type: "many2one" },
      { id: "company_id/industry_id/name", name: "company_id/industry_id/name", value: "company_id/industry_id/name", string: "company_id/industry_id/name", field_type: "char" }
    ]);
  },
  exportDownload(request) {
    parentTemplateDownloads.push(request);
    return Promise.resolve({ ok: true, filename: "res_partner.xlsx" });
  },
  debug: true
});
const parentTemplateShell = parentTemplateWindow.children[1];
findAll(parentTemplateShell, (node) => node.tag === "input" && node.type === "checkbox")[0].checked = true;
findAll(parentTemplateShell, (node) => node.tag === "input" && node.type === "checkbox")[0].dispatchEvent(new TestEvent("change"));
findAll(parentTemplateWindow, (node) => node.dataset?.staticAction === "export")[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const parentTemplateDialog = findAll(parentTemplateShell, (node) => node.dataset?.exportDialog === "res.partner").at(-1);
findAll(parentTemplateDialog, (node) => node.dataset?.exportTemplateSelect)[0].value = "601";
findAll(parentTemplateDialog, (node) => node.dataset?.exportTemplateSelect)[0].dispatchEvent(new TestEvent("change"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(parentTemplateNamelists, [{ model: "res.partner", export_id: 601 }]);
assert.deepEqual(parentTemplateFieldRequests.map((request) => request.model), ["res.partner", "res.company", "res.partner.industry"]);
assert.deepEqual(parentTemplateFieldRequests[2], {
  model: "res.partner.industry",
  prefix: "company_id/industry_id",
  parent_name: "Company/Industry",
  import_compat: false,
  parent_field_type: "many2one",
  parent_field: {
    id: "company_id/industry_id",
    name: "company_id/industry_id",
    string: "Company/Industry",
    value: "company_id/industry_id/id",
    type: "many2one",
    field_type: "many2one",
    relation: "res.partner.industry",
    children: true,
    params: { model: "res.partner.industry", prefix: "company_id/industry_id" }
  },
  exclude: [],
  domain: []
});
assert.deepEqual(findAll(parentTemplateDialog, (node) => node.dataset?.exportField).map((node) => node.dataset.exportField), ["company_id/industry_id", "company_id/industry_id/name"]);
const parentNestedRow = findAll(parentTemplateDialog, (node) => node.dataset?.exportField === "company_id/industry_id/name")[0];
assert.ok(findAll(parentNestedRow, (node) => node.tag === "span").some((node) => node.textContent === "Company/Industry/Name (company_id/industry_id/name)"));
const requestCountAfterTemplate = parentTemplateFieldRequests.length;
assert.equal(parentTemplateFieldRequests.length, requestCountAfterTemplate);
assert.ok(findAll(parentTemplateDialog, (node) => node.dataset?.exportTreeItem).some((node) => node.dataset.exportTreeItem === "company_id/industry_id"));
assert.ok(findAll(parentTemplateDialog, (node) => node.dataset?.exportTreeItem).some((node) => node.dataset.exportTreeItem === "company_id/industry_id/name"));
assert.equal(findAll(parentTemplateDialog, (node) => node.dataset?.exportAddField === "company_id/industry_id")[0].disabled, true);
assert.equal(findAll(parentTemplateDialog, (node) => node.dataset?.exportAddField === "company_id/industry_id/name")[0].disabled, true);
findAll(parentTemplateDialog, (node) => node.dataset?.exportConfirm)[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(parentTemplateDownloads[0].fields.map((field) => field.name), ["company_id/industry_id/id", "company_id/industry_id/name"]);
const mobileActionWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Mobile Contacts" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      active: { type: "boolean", string: "Active" },
      email: { type: "char", string: "Email" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="active"/><field name="email"/></list>`,
        id: 16
      }
    }
  },
  records: [{ id: 91, name: "Mobile", active: true, email: "m@example.com" }],
  length: 1
}, {
  isSmall: true,
  services: {
    orm: {
      searchRead() {
        return Promise.resolve([]);
      }
    }
  },
  exportGetFields() {
    return Promise.resolve([
      { id: "name", value: "name", string: "Name", field_type: "char" },
      { id: "active", value: "active", string: "Active", field_type: "boolean" },
      { id: "email", value: "email", string: "Email", field_type: "char" }
    ]);
  }
});
const mobileActionShell = mobileActionWindow.children[1];
findAll(mobileActionShell, (node) => node.tag === "input" && node.type === "checkbox")[0].checked = true;
findAll(mobileActionShell, (node) => node.tag === "input" && node.type === "checkbox")[0].dispatchEvent(new TestEvent("change"));
findAll(mobileActionWindow, (node) => node.dataset?.staticAction === "export")[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const mobileExportDialog = findAll(mobileActionShell, (node) => node.dataset?.exportDialog === "res.partner").at(-1);
assert.equal(findAll(mobileExportDialog, (node) => String(node.className).includes("o_export_field_sortable")).length, 0);
assert.equal(findAll(mobileExportDialog, (node) => node.dataset?.exportSortField).length, 0);
let responsiveIsSmall = false;
const responsiveActionWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Responsive Contacts" },
  activeView: "list",
  resModel: "res.partner",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Name" },
      active: { type: "boolean", string: "Active" },
      email: { type: "char", string: "Email" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="active"/><field name="email"/></list>`,
        id: 17
      }
    }
  },
  records: [{ id: 92, name: "Responsive", active: true, email: "r@example.com" }],
  length: 1
}, {
  isSmall: () => responsiveIsSmall,
  services: {
    orm: {
      searchRead() {
        return Promise.resolve([]);
      }
    }
  },
  exportGetFields() {
    return Promise.resolve([
      { id: "name", value: "name", string: "Name", field_type: "char" },
      { id: "active", value: "active", string: "Active", field_type: "boolean" },
      { id: "email", value: "email", string: "Email", field_type: "char" }
    ]);
  }
});
const responsiveActionShell = responsiveActionWindow.children[1];
const resizeListenerCount = (windowListeners.resize ?? []).length;
findAll(responsiveActionShell, (node) => node.tag === "input" && node.type === "checkbox")[0].checked = true;
findAll(responsiveActionShell, (node) => node.tag === "input" && node.type === "checkbox")[0].dispatchEvent(new TestEvent("change"));
findAll(responsiveActionWindow, (node) => node.dataset?.staticAction === "export")[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const responsiveExportDialog = findAll(responsiveActionShell, (node) => node.dataset?.exportDialog === "res.partner").at(-1);
assert.equal((windowListeners.resize ?? []).length, resizeListenerCount + 1);
assert.equal(findAll(responsiveExportDialog, (node) => String(node.className).includes("o_export_field_sortable")).length, 3);
responsiveIsSmall = true;
globalThis.window.dispatchEvent(new TestEvent("resize"));
assert.equal(responsiveExportDialog.dataset.exportIsSmall, "true");
assert.equal(findAll(responsiveExportDialog, (node) => node.dataset?.exportSortField).length, 0);
responsiveIsSmall = false;
globalThis.window.dispatchEvent(new TestEvent("resize"));
assert.equal(responsiveExportDialog.dataset.exportIsSmall, "false");
assert.equal(findAll(responsiveExportDialog, (node) => String(node.className).includes("o_export_field_sortable")).length, 3);
findAll(responsiveExportDialog, (node) => node.dataset?.exportClose)[0].dispatchEvent(new TestEvent("click"));
assert.equal((windowListeners.resize ?? []).length, resizeListenerCount);
const accountantExportFieldRequests = [];
const accountantDownloads = [];
const accountantWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Journal Items" },
  activeView: "list",
  resModel: "account.move.line",
  viewDescriptions: {
    fields: {
      name: { type: "char", string: "Label" },
      active: { type: "boolean", string: "Active" },
      analytic_distribution: { type: "json", string: "Analytic Distribution" },
      analytic_line_ids: { type: "one2many", string: "Analytic Lines", relation: "account.analytic.line" }
    },
    relatedModels: {},
    views: {
      list: {
        arch: `<list><field name="name"/><field name="active"/><field name="analytic_distribution"/><field name="analytic_line_ids"/></list>`,
        id: 41
      }
    }
  },
  records: [{ id: 91, name: "Line", active: true, analytic_distribution: "{}", analytic_line_ids: [] }],
  length: 1
}, {
  services: {
    orm: {
      searchRead() {
        return Promise.resolve([]);
      }
    }
  },
  exportGetFields(request) {
    accountantExportFieldRequests.push(request);
    if (request.model === "account.move.line") {
      return Promise.resolve([
        { id: "name", value: "name", string: "Label", field_type: "char" },
        { id: "active", value: "active", string: "Active", field_type: "boolean" },
        { id: "analytic_distribution", value: "analytic_distribution", string: "Analytic Distribution", field_type: "json" },
        { id: "analytic_line_ids", value: "analytic_line_ids/id", string: "Analytic Lines", field_type: "one2many", relation: "account.analytic.line", children: true, params: { model: "account.analytic.line", prefix: "analytic_line_ids" } }
      ]);
    }
    return Promise.resolve([
      { id: "analytic_line_ids/account_id", value: "analytic_line_ids/account_id/id", string: "Analytic Lines/Account", field_type: "many2one", params: { model: "account.analytic.account" } },
      { id: "analytic_line_ids/auto_account_id", value: "analytic_line_ids/auto_account_id/id", string: "Analytic Lines/Automatic Account", field_type: "many2one", params: { model: "account.analytic.account" } },
      { id: "analytic_line_ids/amount", value: "analytic_line_ids/amount", string: "Analytic Lines/Amount", field_type: "float" },
      { id: "analytic_line_ids/name", value: "analytic_line_ids/name", string: "Analytic Lines/Description", field_type: "char" }
    ]);
  },
  exportDownload(request) {
    accountantDownloads.push(request);
    return Promise.resolve({ ok: true });
  }
});
const accountantShell = accountantWindow.children[1];
const accountantCheckbox = findAll(accountantShell, (node) => node.tag === "input" && node.type === "checkbox")[0];
accountantCheckbox.checked = true;
accountantCheckbox.dispatchEvent(new TestEvent("change"));
findAll(accountantWindow, (node) => node.dataset?.staticAction === "export")[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
const accountantDialog = findAll(accountantShell, (node) => node.dataset?.exportDialog === "account.move.line").at(-1);
assert.ok(accountantDialog);
const accountantChildFieldRequest = accountantExportFieldRequests.find((request) => request.model === "account.analytic.line");
assert.deepEqual(accountantExportFieldRequests[0], {
  model: "account.move.line",
  domain: [],
  import_compat: false
});
assert.deepEqual(accountantChildFieldRequest, {
  model: "account.analytic.line",
  prefix: "analytic_line_ids",
  parent_name: "Analytic Lines",
  import_compat: false,
  parent_field_type: "one2many",
  parent_field: {
    id: "analytic_line_ids",
    name: "analytic_line_ids",
    string: "Analytic Lines",
    value: "analytic_line_ids/id",
    type: "one2many",
    field_type: "one2many",
    relation: "account.analytic.line",
    children: true,
    params: { model: "account.analytic.line", prefix: "analytic_line_ids" }
  },
  exclude: [],
  domain: []
});
const accountantSelectedFields = findAll(accountantDialog, (node) => node.dataset?.exportField).map((node) => node.dataset.exportField);
assert.ok(!accountantSelectedFields.includes("analytic_distribution"));
assert.ok(accountantSelectedFields.includes("analytic_line_ids/account_id"));
assert.ok(accountantSelectedFields.includes("analytic_line_ids/amount"));
assert.ok(!accountantSelectedFields.includes("analytic_line_ids/auto_account_id"));
findAll(accountantDialog, (node) => node.dataset?.exportConfirm)[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.ok(accountantDownloads[0].fields.some((field) => field.name === "analytic_line_ids/account_id/id"));
assert.ok(accountantDownloads[0].fields.some((field) => field.name === "analytic_line_ids/amount"));
assert.ok(!accountantDownloads[0].fields.some((field) => field.name === "analytic_distribution"));
const accountantImportCheckbox = findAll(accountantDialog, (node) => node.dataset?.exportImportCompat)[0];
accountantImportCheckbox.checked = true;
accountantImportCheckbox.dispatchEvent(new TestEvent("change"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(accountantExportFieldRequests.at(-1).import_compat, true);
findAll(accountantDialog, (node) => node.dataset?.exportConfirm)[0].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(accountantDownloads.at(-1).fields[0].name, "id");
assert.ok(accountantDownloads.at(-1).fields.some((field) => field.name === "analytic_line_ids/account_id/id"));
const formDuplicateCalls = [];
let formDuplicateEvent;
const formDuplicateWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Partner" },
  activeView: "form",
  resModel: "res.partner",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" }, active: { type: "boolean", string: "Active" } },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="name"/><field name="active"/></sheet></form>`,
        id: 40
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 86, name: "Foxtrot", active: true },
  services: {
    orm: {
      call(model, method, args) {
        formDuplicateCalls.push({ model, method, args });
        return Promise.resolve([187]);
      }
    }
  }
});
const formDuplicateForm = formDuplicateWindow.children[1];
formDuplicateForm.addEventListener("action-menu:duplicate", (event) => { formDuplicateEvent = event.detail; });
const formDuplicateButton = findAll(formDuplicateForm, (node) => node.dataset?.staticAction === "duplicate")[0];
assert.deepEqual([formDuplicateButton.dataset.sequence, formDuplicateButton.dataset.icon], ["30", "fa fa-clone"]);
formDuplicateButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(formDuplicateCalls, [{ model: "res.partner", method: "copy", args: [86, {}] }]);
assert.deepEqual(formDuplicateEvent, { model: "res.partner", ids: [86], newId: [187] });
const formServerMenuCalls = [];
const formServerMenuWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Partner" },
  activeView: "form",
  resModel: "res.partner",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" } },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><sheet><field name="name"/></sheet></form>`,
        id: 37,
        actionMenus: {
          action: [{ id: 410, name: "Partner Action", type: "ir.actions.server" }]
        }
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 83, name: "Gamma" },
  services: {
    action: {
      history: [],
      current: null,
      loadAction(action) {
        return Promise.resolve(typeof action === "object" ? action : { id: action });
      },
      doAction(action, options) {
        formServerMenuCalls.push({ action, options });
        return Promise.resolve(action);
      }
    }
  }
});
const formServerMenu = findAll(formServerMenuWindow.children[0], (node) => String(node.className ?? "").includes("gorp-form-action-menu"))[0];
assert.ok(String(formServerMenu.className).includes("o_cp_action_menus"));
assert.equal(formServerMenu.dataset.controlPanelPlacement, "actions");
const formServerButton = findAll(formServerMenu, (node) => node.dataset?.actionId === "410")[0];
assert.equal(formServerButton.textContent, "Partner Action");
formServerButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(formServerMenuCalls[0].action, 410);
assert.deepEqual(formServerMenuCalls[0].options.additionalContext, {
  active_id: 83,
  active_ids: [83],
  active_model: "res.partner",
  active_domain: []
});
const approvalUserInfoReads = [];
let approvalUserInfoEvent;
const approvalUserInfoWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Purchase Order" },
  activeView: "form",
  resModel: "purchase.order",
  viewDescriptions: {
    fields: { name: { type: "char", string: "Name" } },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><header><button name="" id="approval_user_info" type="action" class="btn-link btn-info" icon="fa-users"/></header><sheet><field name="name"/></sheet></form>`,
        id: 33
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 44, name: "PO002" },
  debug: true,
  location: "/web#menu_id=1",
  services: {
    orm: {
      webRead(model, ids, kwargs) {
        approvalUserInfoReads.push({ model, ids, kwargs });
        return Promise.resolve([{
          approval_done_user_ids: [{ id: 7, display_name: "Current User" }],
          approval_user_ids: [[9, "Approver User"]]
        }]);
      }
    }
  }
});
const approvalUserInfoForm = approvalUserInfoWindow.children[1];
approvalUserInfoForm.addEventListener("workflow:approval-user-info", (event) => { approvalUserInfoEvent = event.detail; });
const approvalUserInfoButton = findAll(approvalUserInfoForm, (node) => node.dataset?.workflowAction === "approval_user_info")[0];
user.userId = 7;
user.isSystem = true;
approvalUserInfoButton.dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(approvalUserInfoReads[0].model, "purchase.order");
assert.deepEqual(approvalUserInfoReads[0].ids, [44]);
assert.deepEqual(Object.keys(approvalUserInfoReads[0].kwargs.specification), ["approval_user_ids", "approval_done_user_ids"]);
const approvalUserInfoPopover = findAll(approvalUserInfoForm, (node) => node.className === "gorp-approval-user-info-popover")[0];
const approvalUserInfoRows = findAll(approvalUserInfoPopover, (node) => node.tag === "tr");
assert.deepEqual(approvalUserInfoRows.map((row) => row.dataset?.userId ?? row.children?.[0]?.textContent), ["7", "Waiting Approval", "9"]);
assert.equal(findAll(approvalUserInfoRows[0], (node) => node.tag === "img")[0].src, "/web/image/res.users/7/avatar_128");
assert.equal(findAll(approvalUserInfoRows[0], (node) => node.tag === "a").length, 0);
const loginAsLink = findAll(approvalUserInfoRows[2], (node) => node.tag === "a")[0];
assert.equal(loginAsLink.href, "/web/login_as/9?redirect=%2Fweb%23menu_id%3D1");
assert.equal(loginAsLink.title, "Login As Approver User");
assert.equal(approvalUserInfoEvent.model, "purchase.order");
assert.equal(approvalUserInfoEvent.id, 44);
const statusbarSaves = [];
const statusbarUpdates = [];
let statusbarRefreshes = 0;
let statusbarUpdateEvent;
const statusbarWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Purchase Order" },
  activeView: "form",
  resModel: "purchase.order",
  viewDescriptions: {
    fields: {
      state: { type: "selection", string: "Status", selection: [["draft", "Draft"], ["pending", "Pending"], ["approved", "Approved"], ["rejected", "Rejected"]] },
      workflow_states: { type: "json" },
      duration_state_tracking: { type: "json" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><header><field name="state" widget="statusbar_state_duration" statusbar_visible="WORKFLOW"/></header></form>`,
        id: 32
      }
    }
  },
  records: [],
  length: 0
}, {
  values: {
    id: 43,
    state: "pending",
    workflow_states: ["draft", "pending", "approved"],
    duration_state_tracking: { draft: 3661, pending: 60, rejected: 20 }
  },
  services: {
    orm: {
      webSave(model, ids, data, kwargs) {
        statusbarSaves.push({ model, ids, data, kwargs });
        return Promise.resolve([{ id: ids[0], ...data }]);
      }
    }
  },
  onUpdate(name, value) {
    statusbarUpdates.push({ name, value });
  },
  onRefresh() {
    statusbarRefreshes += 1;
  }
});
const statusbarForm = statusbarWindow.children[1];
statusbarForm.addEventListener("workflow:statusbar-update", (event) => { statusbarUpdateEvent = event.detail; });
const statusbar = findAll(statusbarForm, (node) => String(node.className ?? "").includes("gorp-statusbar"))[0];
assert.equal(statusbar.dataset.field, "state");
assert.equal(statusbar.dataset.widget, "statusbar_state_duration");
assert.ok(String(statusbar.className).includes("o_statusbar_status"));
const statusbarItems = findAll(statusbar, (node) => String(node.className ?? "").includes("gorp-statusbar-item"));
assert.deepEqual(statusbarItems.map((item) => item.dataset.value), ["draft", "pending", "approved"]);
assert.deepEqual(statusbarItems.map((item) => item.textContent), ["Draft", "Pending", "Approved"]);
assert.deepEqual(statusbarItems.map((item) => item.dataset.selected), ["false", "true", "false"]);
assert.deepEqual(statusbarItems.map((item) => item.disabled), [false, true, false]);
assert.ok(String(statusbarItems[1].className).includes("o_arrow_button_current"));
assert.equal(statusbarItems[0].dataset.durationText, "1:01:01");
assert.equal(statusbarItems[1].dataset.durationText, "1:00");
assert.equal(statusbarItems[2].dataset.durationText, undefined);
statusbarItems[1].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(statusbarSaves, []);
statusbarItems[2].dispatchEvent(new TestEvent("click"));
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(statusbarUpdates, [{ name: "state", value: "approved" }]);
assert.deepEqual(statusbarSaves, [{
  model: "purchase.order",
  ids: [43],
  data: { state: "approved" },
  kwargs: { specification: { state: {} } }
}]);
assert.equal(statusbarRefreshes, 1);
assert.deepEqual(statusbarUpdateEvent, { model: "purchase.order", id: 43, field: "state", value: "approved" });
const nonStateStatusbarWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Task" },
  activeView: "form",
  resModel: "project.task",
  viewDescriptions: {
    fields: {
      phase: { type: "selection", string: "Phase", selection: [["todo", "Todo"], ["doing", "Doing"], ["done", "Done"]] },
      workflow_states: { type: "json" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><header><field name="phase" widget="statusbar" statusbar_visible="WORKFLOW"/></header></form>`,
        id: 38
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 74, phase: "doing", workflow_states: ["todo", "doing"] }
});
const nonStateStatusbar = findAll(nonStateStatusbarWindow, (node) => String(node.className ?? "").includes("gorp-statusbar"))[0];
const nonStateStatusbarItems = findAll(nonStateStatusbar, (node) => String(node.className ?? "").includes("gorp-statusbar-item"));
assert.deepEqual(nonStateStatusbarItems.map((item) => item.dataset.value), ["doing"]);
const disabledStatusbarWindow = renderWindowAction({
  type: "ir.actions.act_window",
  action: { name: "Task" },
  activeView: "form",
  resModel: "project.task",
  viewDescriptions: {
    fields: {
      stage: { type: "selection", string: "Stage", selection: [["new", "New"], ["done", "Done"]] }
    },
    relatedModels: {},
    views: {
      form: {
        arch: `<form><header><field name="stage" widget="statusbar" options="{'clickable': False}"/></header></form>`,
        id: 39
      }
    }
  },
  records: [],
  length: 0
}, {
  values: { id: 75, stage: "new" }
});
const disabledStatusbarItems = findAll(disabledStatusbarWindow, (node) => String(node.className ?? "").includes("gorp-statusbar-item"));
assert.deepEqual(disabledStatusbarItems.map((item) => item.disabled), [true, true]);
const groupUpdates = [];
const groupResult = {
  type: "ir.actions.act_window",
  action: { name: "User" },
  activeView: "form",
  resModel: "res.users",
  viewDescriptions: {
    fields: {
      group_ids: { type: "many2many", relation: "res.groups", string: "Access Rights" },
      view_group_hierarchy: { type: "json" },
      all_group_ids: { type: "many2many", relation: "res.groups" },
      role: { type: "selection" }
    },
    relatedModels: {},
    views: {
      form: {
        arch: "<form><sheet><field name=\"group_ids\" widget=\"res_user_group_ids\"/></sheet></form>",
        id: 19
      }
    }
  },
  records: [],
  length: 0
};
const groupWindow = renderWindowAction(groupResult, {
  values: {
    role: "user",
    group_ids: [[10, "Sales / User"]],
    all_group_ids: [[10, "Sales / User"], [11, "Sales / Manager"], [20, "Inventory / User"], [30, "Export Reports"]],
    view_group_hierarchy: {
      groups: {
        "10": { id: 10, name: "Sales / User", privilege_id: 100 },
        "11": { id: 11, name: "Sales / Manager", privilege_id: 100, implied_ids: [30], all_implied_by_ids: [40], disjoint_ids: [20] },
        "20": { id: 20, name: "Inventory / User", privilege_id: 200 },
        "30": { id: 30, name: "Export Reports" }
      },
      privileges: {
        "100": { id: 100, name: "Sales Access", category_id: 1, placeholder: "No Sales", group_ids: [10, 11] },
        "200": { id: 200, name: "Inventory Access", category_id: 2, group_ids: [20] }
      },
      categories: [
        { id: 1, name: "Sales", privilege_ids: [100] },
        { id: 2, name: "Inventory", privilege_ids: [200] }
      ]
    }
  },
  onUpdate(name, value) {
    groupUpdates.push({ name, value });
  }
});
const groupField = findAll(groupWindow, (node) => node.className === "gorp-form-field gorp-res-user-group-ids")[0];
assert.equal(groupField.dataset.field, "group_ids");
assert.equal(groupField.dataset.role, "user");
assert.deepEqual(findAll(groupField, (node) => node.tag === "h2").map((node) => node.textContent), ["Extra Rights", "Inventory", "Sales"]);
const selects = findAll(groupField, (node) => node.tag === "select");
assert.equal(selects.length, 2);
const salesSelect = selects.find((node) => node.dataset.privilegeId === "100");
assert.equal(salesSelect.value, "10");
assert.equal(findAll(salesSelect, (node) => node.tag === "option")[0].textContent, "No Sales");
const managerOption = findAll(salesSelect, (node) => node.tag === "option").find((node) => node.dataset.groupId === "11");
assert.equal(managerOption.dataset.impliedIds, "30");
assert.equal(managerOption.dataset.impliedByIds, "40");
assert.equal(managerOption.dataset.disjointIds, "20");
assert.equal(managerOption.title, "implies 30; implied by 40; incompatible 20");
salesSelect.value = "11";
salesSelect.dispatchEvent(new TestEvent("change"));
assert.deepEqual(groupUpdates.at(-1), { name: "group_ids", value: [[6, false, [11]]] });
salesSelect.value = "";
salesSelect.dispatchEvent(new TestEvent("change"));
assert.deepEqual(groupUpdates.at(-1), { name: "group_ids", value: [[6, false, []]] });
const checkboxes = findAll(groupField, (node) => node.tag === "input");
assert.equal(checkboxes.length, 1);
const exportCheckbox = checkboxes.find((node) => node.dataset.groupId === "30");
exportCheckbox.checked = true;
exportCheckbox.dispatchEvent(new TestEvent("change"));
assert.deepEqual(groupUpdates.at(-1), { name: "group_ids", value: [[6, false, [30]]] });

const userGroupSpecRequests = [];
const userGroupSpecServices = createWebClientServices({
  transport(request) {
    userGroupSpecRequests.push(request);
    if (request.route === "/web/dataset/call_kw/res.users/get_views") {
      return Promise.resolve({
        views: {
          form: { arch: "<form><sheet><field name=\"group_ids\" widget=\"res_user_group_ids\"/></sheet></form>", id: 19 }
        },
        models: {
          "res.users": {
            fields: {
              group_ids: { type: "many2many", relation: "res.groups", string: "Groups" },
              all_group_ids: { type: "many2many", relation: "res.groups", string: "Groups and implied groups" },
              view_group_hierarchy: { type: "json", string: "Technical field for user group setting" },
              role: { type: "selection", string: "Role" }
            }
          }
        }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.users/web_read") {
      return Promise.resolve([{ id: 7, group_ids: [10], all_group_ids: [10, 11], role: "group_user", view_group_hierarchy: {} }]);
    }
    return Promise.resolve({});
  }
});
await userGroupSpecServices.action.doAction({
  type: "ir.actions.act_window",
  res_model: "res.users",
  res_id: 7,
  target: "current",
  views: [[19, "form"]]
});
assert.deepEqual(userGroupSpecRequests[1].params.kwargs.specification, {
  group_ids: {},
  all_group_ids: {},
  view_group_hierarchy: {},
  role: {}
});

const usersFallbackFields = {
  name: { type: "char", string: "name" },
  login: { type: "char", string: "login" },
  email: { type: "char", string: "email" },
  company_id: { type: "many2one", relation: "res.company", string: "company_id" },
  company_ids: { type: "many2many", relation: "res.company", string: "company_ids" },
  partner_id: { type: "many2one", relation: "res.partner", string: "partner_id" },
  groups_count: { type: "integer", string: "groups_count" },
  role: { type: "selection", string: "role" },
  group_ids: { type: "many2many", relation: "res.groups", string: "group_ids" },
  all_group_ids: { type: "many2many", relation: "res.groups", string: "all_group_ids" },
  view_group_hierarchy: { type: "json", string: "view_group_hierarchy" },
  active: { type: "boolean", string: "active" },
  notification_type: { type: "selection", string: "notification_type" },
  signature: { type: "text", string: "signature" },
  password: { type: "char", string: "password" }
};

const usersExplicitFormRequests = [];
const usersExplicitFormServices = createWebClientServices({
  transport(request) {
    usersExplicitFormRequests.push(request);
    if (request.route === "/web/dataset/call_kw/res.users/get_views") {
      return Promise.resolve({
        views: {
          form: {
            arch: `<form><sheet><group><field name="name"/><field name="login"/><field name="email"/><field name="company_id"/><field name="role" readonly="1"/><field name="active"/><field name="notification_type"/><field name="signature"/></group><notebook name="access_notebook"><page string="Access Rights" name="access_rights"><field name="group_ids" widget="res_user_group_ids"/></page><page string="Preferences" name="preferences"><field name="company_ids"/><field name="partner_id"/></page></notebook></sheet></form>`,
            id: 51
          }
        },
        models: { "res.users": { fields: usersFallbackFields } }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.users/web_read") {
      return Promise.resolve([{
        id: 7,
        display_name: "Administrator",
        name: "Administrator",
        login: "admin",
        email: "admin@example.test",
        company_id: [1, "My Company"],
        role: "group_system",
        group_ids: [[10, "Administration / Settings"]],
        all_group_ids: [[10, "Administration / Settings"]],
        view_group_hierarchy: { groups: {}, privileges: {}, categories: [] },
        active: true,
        notification_type: "email",
        signature: "Admin",
        company_ids: [[1, "My Company"]],
        partner_id: [3, "Administrator"]
      }]);
    }
    return Promise.resolve({});
  }
});
const usersExplicitFormResult = await usersExplicitFormServices.action.doAction({
  type: "ir.actions.act_window",
  name: "Users",
  res_model: "res.users",
  res_id: 7,
  target: "current",
  views: [[51, "form"]]
});
const usersExplicitWebRead = usersExplicitFormRequests.find((request) => request.route === "/web/dataset/call_kw/res.users/web_read");
assert.deepEqual(Object.keys(usersExplicitWebRead.params.kwargs.specification), ["name", "login", "email", "company_id", "role", "active", "notification_type", "signature", "group_ids", "all_group_ids", "view_group_hierarchy", "company_ids", "partner_id"]);
assert.deepEqual(usersExplicitWebRead.params.kwargs.specification.group_ids, {});
assert.deepEqual(usersExplicitWebRead.params.kwargs.specification.all_group_ids, {});
assert.deepEqual(usersExplicitWebRead.params.kwargs.specification.view_group_hierarchy, {});
assert.deepEqual(usersExplicitWebRead.params.kwargs.specification.company_ids, {});
assert.deepEqual(usersExplicitWebRead.params.kwargs.specification.partner_id, { fields: { display_name: {} } });
const usersExplicitFormWindow = renderWindowAction(usersExplicitFormResult);
const usersExplicitNotebook = findAll(usersExplicitFormWindow, (node) => String(node.className ?? "").includes("gorp-form-notebook") && node.dataset?.notebook === "notebook-access_notebook")[0];
assert.ok(usersExplicitNotebook);
assert.deepEqual(findAll(usersExplicitNotebook, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-form-notebook-tab")).map((node) => node.textContent), ["Access Rights", "Preferences"]);
assert.equal(findAll(usersExplicitNotebook, (node) => String(node.className ?? "").includes("gorp-res-user-group-ids")).length, 1);
const usersExplicitMainFields = findAll(usersExplicitFormWindow, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-form-fields"))[0];
assert.equal(usersExplicitMainFields.children.some((node) => String(node.className ?? "").includes("gorp-res-user-group-ids")), false);

const usersListSpecRequests = [];
const usersListSpecServices = createWebClientServices({
  transport(request) {
    usersListSpecRequests.push(request);
    if (request.route === "/web/dataset/call_kw/res.users/get_views") {
      return Promise.resolve({
        views: { list: { arch: "<list/>", id: false } },
        models: { "res.users": { fields: usersFallbackFields } }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.users/web_search_read") {
      return Promise.resolve({
        length: 1,
        records: [{
          id: 7,
          name: "Administrator",
          login: "admin",
          email: "admin@example.test",
          company_id: [1, "My Company"],
          groups_count: 2,
          active: true
        }]
      });
    }
    return Promise.resolve({});
  }
});
const usersListResult = await usersListSpecServices.action.doAction({
  type: "ir.actions.act_window",
  name: "Users",
  res_model: "res.users",
  target: "current",
  views: [[false, "list"]]
});
const usersSearchRead = usersListSpecRequests.find((request) => request.route === "/web/dataset/call_kw/res.users/web_search_read");
assert.deepEqual(Object.keys(usersSearchRead.params.kwargs.specification), ["name", "login", "email", "company_id", "groups_count", "active"]);
assert.deepEqual(usersSearchRead.params.kwargs.specification.company_id, { fields: { display_name: {} } });
const usersListWindow = renderWindowAction(usersListResult);
const usersListTable = findAll(usersListWindow, (node) => String(node.className ?? "").includes("gorp-list-view"))[0];
assert.deepEqual(findAll(usersListTable, (node) => String(node.className ?? "").includes("o_list_header_button")).map((node) => node.textContent), ["Name", "Login", "Email", "Company", "Groups", "Active"]);
assert.deepEqual(findAll(usersListTable.children[1].children[0], (node) => node.tag === "output").map((node) => node.textContent), ["Administrator", "admin", "admin@example.test", "2", "true"]);
assert.equal(findAll(usersListTable.children[1].children[0], (node) => String(node.className ?? "").includes("gorp-many2one-link"))[0].textContent, "My Company");

const usersFormSpecRequests = [];
const usersFormSpecServices = createWebClientServices({
  transport(request) {
    usersFormSpecRequests.push(request);
    if (request.route === "/web/dataset/call_kw/res.users/get_views") {
      return Promise.resolve({
        views: { form: { arch: "<form/>", id: false } },
        models: { "res.users": { fields: usersFallbackFields } }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.users/web_read") {
      return Promise.resolve([{
        id: 7,
        display_name: "Administrator",
        name: "Administrator",
        login: "admin",
        email: "admin@example.test",
        company_id: [1, "My Company"],
        role: "group_system",
        group_ids: [[10, "Administration / Settings"]],
        all_group_ids: [[10, "Administration / Settings"]],
        view_group_hierarchy: { groups: {}, privileges: {}, categories: [] },
        active: true,
        notification_type: "email",
        signature: "Admin"
      }]);
    }
    return Promise.resolve({});
  }
});
const usersFormResult = await usersFormSpecServices.action.doAction({
  type: "ir.actions.act_window",
  name: "Users",
  res_model: "res.users",
  res_id: 7,
  target: "current",
  views: [[false, "form"]]
});
const usersWebRead = usersFormSpecRequests.find((request) => request.route === "/web/dataset/call_kw/res.users/web_read");
assert.deepEqual(Object.keys(usersWebRead.params.kwargs.specification), ["name", "login", "email", "company_id", "role", "group_ids", "all_group_ids", "view_group_hierarchy", "active", "notification_type", "signature"]);
assert.deepEqual(usersWebRead.params.kwargs.specification.company_id, { fields: { display_name: {} } });
const usersFormWindow = renderWindowAction(usersFormResult);
let usersForm = usersFormWindow.children[1];
assert.equal(findAll(usersForm, (node) => String(node.className ?? "").includes("gorp-form-sheet")).length, 1);
assert.deepEqual(findAll(usersForm, (node) => String(node.className ?? "").split(/\s+/).includes("o_form_label")).map((node) => node.textContent).slice(0, 5), ["Name", "Login", "Email", "Company", "Role"]);
const usersAccessNotebook = findAll(usersForm, (node) => String(node.className ?? "").includes("gorp-form-notebook") && node.dataset?.notebook === "res-users-access-rights")[0];
assert.ok(usersAccessNotebook);
assert.deepEqual(findAll(usersAccessNotebook, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-form-notebook-tab")).map((node) => node.textContent), ["Access Rights"]);
assert.equal(findAll(usersAccessNotebook, (node) => String(node.className ?? "").includes("gorp-res-user-group-ids")).length, 1);
const usersMainFields = findAll(usersForm, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-form-fields"))[0];
assert.equal(usersMainFields.children.some((node) => String(node.className ?? "").includes("gorp-res-user-group-ids")), false);
assert.equal(findAll(usersForm, (node) => node.tag === "output" && node.textContent === "Administrator").length, 1);
findAll(usersFormWindow, (node) => node.dataset?.formAction === "edit")[0].dispatchEvent(new TestEvent("click"));
usersForm = usersFormWindow.children[1];
assert.equal(findAll(usersForm, (node) => node.tag === "input" && node.dataset?.field === "login")[0].value, "admin");
assert.equal(findAll(usersForm, (node) => node.tag === "input" && node.dataset?.field === "email")[0].value, "admin@example.test");
assert.equal(findAll(usersForm, (node) => String(node.className ?? "").includes("gorp-form-notebook") && node.dataset?.notebook === "res-users-access-rights").length, 1);

const usersBadArchRequests = [];
const usersBadArchServices = createWebClientServices({
  transport(request) {
    usersBadArchRequests.push(request);
    if (request.route === "/web/dataset/call_kw/res.users/get_views") {
      return Promise.resolve({
        views: {
          list: { arch: "<list><field name=\"accesses_count\"/><field name=\"active_groups_count\"/><field name=\"active_group_ids\"/></list>", id: 25 },
          form: { arch: "<form><sheet><field name=\"accesses_count\"/></sheet></form>", id: 26 }
        },
        models: { "res.users": { fields: usersFallbackFields } }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.users/web_search_read") {
      return Promise.resolve({
        length: 1,
        records: [{
          id: 7,
          name: "Administrator",
          login: "admin",
          email: "admin@example.test",
          company_id: [1, "My Company"],
          groups_count: 2,
          active: true
        }]
      });
    }
    if (request.route === "/web/dataset/call_kw/res.users/web_read") {
      return Promise.resolve([{
        id: 7,
        name: "Administrator",
        login: "admin",
        email: "admin@example.test",
        company_id: [1, "My Company"],
        role: "group_system",
        group_ids: [[10, "Administration / Settings"]],
        all_group_ids: [[10, "Administration / Settings"]],
        view_group_hierarchy: { groups: {}, privileges: {}, categories: [] },
        active: true,
        notification_type: "email",
        signature: "Admin"
      }]);
    }
    return Promise.resolve({});
  }
});
const badUsersListResult = await usersBadArchServices.action.doAction({
  type: "ir.actions.act_window",
  name: "Users",
  res_model: "res.users",
  target: "current",
  views: [[25, "list"]]
});
const badUsersSearchRead = usersBadArchRequests.find((request) => request.route === "/web/dataset/call_kw/res.users/web_search_read");
assert.deepEqual(Object.keys(badUsersSearchRead.params.kwargs.specification), ["name", "login", "email", "company_id", "groups_count", "active"]);
const badUsersListWindow = renderWindowAction(badUsersListResult);
assert.deepEqual(findAll(badUsersListWindow, (node) => String(node.className ?? "").includes("o_list_header_button")).map((node) => node.textContent), ["Name", "Login", "Email", "Company", "Groups", "Active"]);
assert.equal(findAll(badUsersListWindow, (node) => node.tag === "output" && node.textContent === "Administrator").length, 1);
const badUsersFormResult = await usersBadArchServices.action.doAction({
  type: "ir.actions.act_window",
  name: "Users",
  res_model: "res.users",
  res_id: 7,
  target: "current",
  views: [[26, "form"]]
});
const badUsersWebRead = usersBadArchRequests.find((request) => request.route === "/web/dataset/call_kw/res.users/web_read");
assert.deepEqual(Object.keys(badUsersWebRead.params.kwargs.specification), ["name", "login", "email", "company_id", "role", "group_ids", "all_group_ids", "view_group_hierarchy", "active", "notification_type", "signature"]);
const badUsersFormWindow = renderWindowAction(badUsersFormResult);
assert.deepEqual(findAll(badUsersFormWindow, (node) => String(node.className ?? "").split(/\s+/).includes("o_form_label")).map((node) => node.textContent).slice(0, 4), ["Name", "Login", "Email", "Company"]);
const badUsersAccessNotebook = findAll(badUsersFormWindow, (node) => String(node.className ?? "").includes("gorp-form-notebook") && node.dataset?.notebook === "res-users-access-rights")[0];
assert.ok(badUsersAccessNotebook);
assert.deepEqual(findAll(badUsersAccessNotebook, (node) => String(node.className ?? "").split(/\s+/).includes("gorp-form-notebook-tab")).map((node) => node.textContent), ["Access Rights"]);
assert.equal(findAll(badUsersAccessNotebook, (node) => String(node.className ?? "").includes("gorp-res-user-group-ids")).length, 1);
assert.equal(findAll(badUsersFormWindow, (node) => node.tag === "output" && node.textContent === "Administrator").length, 1);

assert.equal(windowActionRequests[0].route, "/web/action/load");
assert.deepEqual(windowActionRequests[0].params.context, { active_id: 42, active_ids: [1, 2], lang: "en_US", from_context: 7 });
assert.equal(windowActionRequests[1].route, "/web/dataset/call_kw/res.partner/get_views");
assert.deepEqual(windowActionRequests[1].params.kwargs.views, [[8, "list"], [false, "form"], [9, "search"]]);
assert.equal(windowActionRequests[1].params.kwargs.options.load_filters, true);
assert.equal(windowActionRequests[1].params.kwargs.options.toolbar, true);
assert.deepEqual(windowActionRequests[1].params.kwargs.context, { lang: "en_US" });
assert.equal(windowActionRequests[2].route, "/web/dataset/call_kw/res.partner/web_search_read");
assert.deepEqual(windowActionRequests[2].params.kwargs.domain, [["active", "=", true], ["id", "in", [1, 2]], ["customer_rank", ">", 0]]);
assert.deepEqual(windowActionRequests[2].params.kwargs.specification, {
  name: {},
  company_id: {
    fields: { display_name: {} },
    context: { default_active_id: 42, from_context: 7, none_value: null }
  },
  legacy_note: {},
  column_note: {},
  line_ids: {
    fields: {
      description: {},
      user_id: { fields: { display_name: {} } }
    },
    context: { default_partner_id: 42, flag: true },
    limit: 2,
    order: "sequence desc"
  }
});
assert.equal(windowActionRequests[2].params.kwargs.limit, 25);
assert.deepEqual(windowActionRequests[2].params.kwargs.context, {
  bin_size: true,
  lang: "en_US",
  search_default_customer: true,
  active_id: 42,
  active_ids: [1, 2],
  from_context: 7,
  group_by: "company_id",
  from_search: true
});
assert.deepEqual(windowActionRequests[2].params.kwargs.groupby, ["company_id"]);

const formActionRequests = [];
const formServices = createWebClientServices({
  transport(request) {
    formActionRequests.push(request);
    if (request.route === "/web/dataset/call_kw/res.partner/get_views") {
      return Promise.resolve({
        views: {
          form: { arch: "<form><sheet><field name=\"name\"/><field name=\"company_id\"/></sheet></form>", id: 10 }
        },
        models: {
          "res.partner": {
            fields: {
              name: { type: "char", string: "Name" },
              company_id: { type: "many2one", relation: "res.company", string: "Company" }
            }
          }
        }
      });
    }
    if (request.route === "/web/dataset/call_kw/res.partner/web_read") {
      return Promise.resolve([{ id: 77, name: "Deco Addict", company_id: [3, "My Company"] }]);
    }
    return Promise.resolve({});
  }
});
const formResult = await formServices.action.doAction({
  id: 8,
  type: "ir.actions.act_window",
  name: "Partner",
  res_model: "res.partner",
  res_id: 77,
  views: [[10, "form"]],
  target: "current",
  context: { lang: "en_US" }
});
assert.equal(formResult.activeView, "form");
assert.equal(formResult.length, 1);
assert.equal(formResult.records[0].name, "Deco Addict");
assert.equal(formActionRequests[0].route, "/web/dataset/call_kw/res.partner/get_views");
assert.equal(formActionRequests[1].route, "/web/dataset/call_kw/res.partner/web_read");
assert.deepEqual(formActionRequests[1].params.args, [[77]]);
assert.deepEqual(formActionRequests[1].params.kwargs.specification, {
  name: {},
  company_id: { fields: { display_name: {} } }
});
assert.deepEqual(formActionRequests[1].params.kwargs.context, { bin_size: true, lang: "en_US" });

const newFormActionRequests = [];
const newFormServices = createWebClientServices({
  transport(request) {
    newFormActionRequests.push(request);
    if (request.route === "/web/dataset/call_kw/ir.actions.server/get_views") {
      return Promise.resolve({
        views: {
          form: { arch: "<form><sheet><field name=\"name\"/><field name=\"state\"/><field name=\"active\"/></sheet></form>", id: 12 }
        },
        models: {
          "ir.actions.server": {
            fields: {
              name: { type: "char", string: "Name" },
              state: { type: "selection", string: "Action To Do" },
              active: { type: "boolean", string: "Active" }
            }
          }
        }
      });
    }
    if (request.route === "/web/dataset/call_kw/ir.actions.server/default_get") {
      return Promise.resolve({ state: "code", active: true });
    }
    return Promise.resolve({});
  }
});
const newServerActionResult = await newFormServices.action.doAction({
  id: 91,
  type: "ir.actions.act_window",
  name: "New Server Action",
  res_model: "ir.actions.server",
  views: [[12, "form"]],
  target: "current",
  context: { default_state: "code" }
});
assert.equal(newServerActionResult.activeView, "form");
assert.equal(newServerActionResult.length, 0);
assert.deepEqual(newServerActionResult.records, [{ state: "code", active: true }]);
assert.equal(newFormActionRequests[1].route, "/web/dataset/call_kw/ir.actions.server/default_get");
assert.deepEqual(newFormActionRequests[1].params.args, [["name", "state", "active"]]);
assert.deepEqual(newFormActionRequests[1].params.kwargs.context, { default_state: "code" });

const workflowSelectionRequests = [];
const workflowSelectionServices = createWebClientServices({
  transport(request) {
    workflowSelectionRequests.push(request);
    if (request.route === "/web/dataset/call_kw/purchase.order/get_views") {
      return Promise.resolve({
        views: {
          form: { arch: "<form><sheet><field name=\"name\"/><field name=\"company_id\"/></sheet></form>", id: 55 }
        },
        models: {
          "purchase.order": {
            fields: {
              name: { type: "char", string: "Name" },
              company_id: { type: "many2one", relation: "res.company", string: "Company" }
            }
          }
        }
      });
    }
    return Promise.resolve({});
  }
});
const selectedWorkflowResult = await workflowSelectionServices.action.doAction(
  {
    type: "ir.actions.act_window",
    name: "New PO",
    res_model: "purchase.order",
    target: "current",
    views: [[55, "form"]]
  },
  { additional_context: { approval_auto_submit: true, default_company_id: 3 } }
);
assert.equal(selectedWorkflowResult.activeView, "form");
assert.equal(selectedWorkflowResult.resModel, "purchase.order");
assert.equal(selectedWorkflowResult.viewDescriptions.views.form.id, 55);
assert.deepEqual(selectedWorkflowResult.action.context, { approval_auto_submit: true, default_company_id: 3 });
assert.equal(workflowSelectionRequests[0].route, "/web/dataset/call_kw/purchase.order/get_views");
assert.deepEqual(workflowSelectionRequests[0].params.kwargs.views, [[55, "form"], [false, "search"]]);
assert.deepEqual(workflowSelectionRequests[0].params.kwargs.context, {});

const workflowOpenRequests = [];
const workflowOpenServices = createWebClientServices({
  transport(request) {
    workflowOpenRequests.push(request);
    if (request.route === "/web/dataset/call_kw/purchase.order/get_views") {
      return Promise.resolve({
        views: {
          form: { arch: "<form><sheet><field name=\"name\"/></sheet></form>", id: 66 }
        },
        models: {
          "purchase.order": {
            fields: {
              name: { type: "char", string: "Name" }
            }
          }
        }
      });
    }
    if (request.route === "/web/dataset/call_kw/purchase.order/web_read") {
      return Promise.resolve([{ id: 101, name: "PO101" }]);
    }
    return Promise.resolve({});
  }
});
const workflowOpenResult = await workflowOpenServices.action.doAction({
  type: "ir.actions.act_window",
  res_model: "purchase.order",
  res_id: 101,
  target: "current",
  views: [[66, "form"]]
});
assert.equal(workflowOpenResult.activeView, "form");
assert.deepEqual(workflowOpenResult.records, [{ id: 101, name: "PO101" }]);
assert.equal(workflowOpenRequests[0].route, "/web/dataset/call_kw/purchase.order/get_views");
assert.equal(workflowOpenRequests[1].route, "/web/dataset/call_kw/purchase.order/web_read");
assert.deepEqual(workflowOpenRequests[1].params.args, [[101]]);
assert.deepEqual(workflowOpenRequests[1].params.kwargs.specification, { name: {} });
