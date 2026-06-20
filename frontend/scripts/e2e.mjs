import assert from "node:assert/strict";
import { mountDevShell } from "../dist/apps/dev-shell/src/main.js";
import {
  createApprovalButtonStates,
  executeWorkflowAction
} from "../dist/packages/oi-workflow/src/index.js";
import { createImpersonationContextFromSession } from "../dist/packages/oi-login-as/src/index.js";
import { makeEnv, registries, serviceMetadata, startServices } from "../dist/packages/webclient/src/index.js";

class BrowserEvent {
  constructor(type) {
    this.type = type;
    this.defaultPrevented = false;
    this.target = null;
    this.currentTarget = null;
  }

  preventDefault() {
    this.defaultPrevented = true;
  }
}

class BrowserElement {
  constructor(tag) {
    this.tag = tag;
    this.className = "";
    this.dataset = {};
    this.textContent = "";
    this.children = [];
    this.disabled = false;
    this.attributes = {};
    this.listeners = {};
    this.parentNode = null;
  }

  append(...nodes) {
    for (const node of nodes) {
      node.parentNode = this;
      this.children.push(node);
    }
  }

  replaceChildren(...nodes) {
    for (const child of this.children) child.parentNode = null;
    this.children = [];
    this.append(...nodes);
  }

  setAttribute(name, value) {
    const stringValue = String(value);
    this.attributes[name] = stringValue;
    if (name === "class") this.className = stringValue;
    if (name.startsWith("data-")) this.dataset[dataKey(name)] = stringValue;
  }

  getAttribute(name) {
    if (name === "class") return this.className;
    if (name.startsWith("data-")) return this.dataset[dataKey(name)];
    return this.attributes[name];
  }

  addEventListener(type, listener) {
    this.listeners[type] = [...(this.listeners[type] ?? []), listener];
  }

  dispatchEvent(event) {
    event.target ??= this;
    event.currentTarget = this;
    for (const listener of this.listeners[event.type] ?? []) {
      listener.call(this, event);
    }
    return !event.defaultPrevented;
  }

  click() {
    return this.dispatchEvent(new BrowserEvent("click"));
  }

  querySelector(selector) {
    return this.querySelectorAll(selector)[0] ?? null;
  }

  querySelectorAll(selector) {
    const matches = [];
    const visit = (node) => {
      if (matchesSelector(node, selector)) matches.push(node);
      for (const child of node.children ?? []) visit(child);
    };
    visit(this);
    return matches;
  }
}

function dataKey(name) {
  return name.slice(5).replace(/-([a-z])/g, (_match, letter) => letter.toUpperCase());
}

function matchesSelector(node, selector) {
  if (!node || !selector) return false;
  const dataMatch = selector.match(/^(?:(\w+))?\[data-([\w-]+)="([^"]+)"\]$/);
  if (dataMatch) {
    const [, tag, key, value] = dataMatch;
    return (!tag || node.tag === tag) && node.dataset[dataKey(`data-${key}`)] === value;
  }
  if (selector.startsWith(".")) return node.className.split(/\s+/).includes(selector.slice(1));
  return node.tag === selector;
}

const browserDocument = {
  body: new BrowserElement("body"),
  createElement(tag) {
    return new BrowserElement(tag);
  },
  querySelector(selector) {
    return this.body.querySelector(selector);
  }
};

globalThis.document = browserDocument;
globalThis.window = { document: browserDocument, Event: BrowserEvent };
globalThis.Event = BrowserEvent;
globalThis.HTMLElement = BrowserElement;

const target = document.createElement("div");
document.body.append(target);
mountDevShell(target);

assert.equal(target.children.length, 1);
assert.equal(target.children[0].className, "gorp-webclient");
assert.equal(target.children[0].children.length, 2);

const requests = [];
const actionHost = document.createElement("section");
actionHost.className = "oi-action-host";
document.body.append(actionHost);
const env = makeEnv({
  debug: true,
  services: {}
});
env.userContext = { lang: "en_US" };
env.rpcTransport = (request) => {
  requests.push(request);
  if (request.route === "/web/session/get_session_info") {
    return Promise.resolve({
      uid: 20,
      is_system: true,
      debug: true,
      login_as: {
        active: true,
        original_uid: 1,
        effective_uid: 20,
        banner: "Impersonating Portal User",
        return_to: "/web#menu_id=1",
        back_route: "/web/login_back"
      },
      user_context: {
        login_as: true,
        login_as_original_uid: 1,
        login_as_back_route: "/web/login_back"
      }
    });
  }
  if (request.route.includes("/web/dataset/call_button/")) {
    return Promise.resolve({ type: "ir.actions.client", tag: "soft_reload" });
  }
  if (request.route.includes("/web/dataset/call_kw/")) {
    return Promise.resolve([{ id: 42, display_name: "Manager" }]);
  }
  return Promise.resolve({ ok: true });
};

let lastBrowserWorkflowExecution = Promise.resolve(null);
const oiWorkflowActionTag = "oi.workflow.approval_panel";
registries.actions.add(oiWorkflowActionTag, {
  async execute(action, executionEnv) {
    const record = action.params.record;
    const buttons = createApprovalButtonStates(record, action.params.buttons);
    const panel = document.createElement("article");
    panel.className = "oi-workflow-panel";
    panel.dataset.model = record.model;
    for (const buttonState of buttons) {
      const control = document.createElement("button");
      control.className = "oi-workflow-button";
      control.textContent = buttonState.label;
      control.disabled = buttonState.disabled;
      control.dataset.workflowAction = buttonState.name;
      control.dataset.workflowKind = buttonState.kind;
      control.addEventListener("click", () => {
        if (!buttonState.action || control.disabled) return;
        lastBrowserWorkflowExecution = executeWorkflowAction(
          buttonState.action,
          {
            dataset: executionEnv.services.dataset,
            action: executionEnv.services.action
          },
          {
            refresh() {
              panel.dataset.refreshed = "true";
            }
          }
        ).then((result) => {
          control.dataset.resultType = result?.type ?? "object";
          control.dataset.resultTag = result?.tag ?? "";
          return result;
        });
      });
      panel.append(control);
    }
    actionHost.replaceChildren(panel);
    return panel;
  }
}, { force: true });

env.actionExecutor = async (invocation) => {
  const tag = invocation.action.tag;
  if (typeof tag === "string" && registries.actions.contains(tag)) {
    const registeredAction = registries.actions.get(tag);
    return registeredAction.execute(invocation.action, env, invocation.options);
  }
  return invocation.action;
};

await startServices(env);
assert.equal(typeof env.services.rpc.call, "function");
assert.equal(typeof env.services.orm.webRead, "function");
assert.equal(serviceMetadata.orm.includes("webReadGroup"), true);

await env.services.action.doAction({
  type: "ir.actions.client",
  tag: oiWorkflowActionTag,
  params: {
    record: {
      id: 42,
      model: "purchase.request",
      userCanApprove: true,
      activeFields: ["approved_button_clicked", "user_can_approve"]
    },
    buttons: [
      {
        name: "approval_action_button",
        label: "Approve",
        args: "[77]",
        context: { tz: "UTC" }
      },
      {
        name: "approval_log",
        label: "Approval Log",
        kind: "approval_log"
      },
      {
        name: "update_status",
        label: "Update Status",
        kind: "update_status"
      }
    ]
  }
});

const workflowPanel = actionHost.querySelector(".oi-workflow-panel");
assert.equal(workflowPanel.dataset.model, "purchase.request");
assert.equal(workflowPanel.children.length, 3);

const approveButton = workflowPanel.querySelector('button[data-workflow-action="approval_action_button"]');
assert.equal(approveButton.textContent, "Approve");
approveButton.click();
const workflowResult = await lastBrowserWorkflowExecution;
assert.equal(workflowResult.tag, "soft_reload");
assert.equal(env.services.action.history.at(-1).action.tag, "soft_reload");
assert.equal(workflowPanel.dataset.refreshed, "true");
assert.equal(approveButton.dataset.resultTag, "soft_reload");

const logButton = workflowPanel.querySelector('button[data-workflow-action="approval_log"]');
logButton.click();
const logResult = await lastBrowserWorkflowExecution;
assert.equal(logResult.res_model, "approval.log");
assert.deepEqual(logResult.domain[1], ["record_id", "in", [42]]);

const statusButton = workflowPanel.querySelector('button[data-workflow-action="update_status"]');
statusButton.click();
const statusResult = await lastBrowserWorkflowExecution;
assert.equal(statusResult.res_model, "approval.state.update");
assert.equal(statusResult.target, "new");

await env.services.orm.webRead("res.users", [20], {
  specification: { approval_user_ids: { fields: { display_name: {} } } }
});
const session = await env.services.session.load();
const impersonation = createImpersonationContextFromSession(session);
assert.equal(impersonation.active, true);
assert.equal(impersonation.effective.id, 20);
assert.equal(impersonation.backAction.url, "/web/login_back?redirect=%2Fweb%23menu_id%3D1");

const callButton = requests.find((request) => request.route.includes("/web/dataset/call_button/"));
assert.equal(callButton.route, "/web/dataset/call_button/purchase.request/approval_action_button");
assert.deepEqual(callButton.params.args, [[42], 77]);
assert.deepEqual(callButton.params.kwargs, { tz: "UTC" });

const webRead = requests.find((request) => request.params?.method === "web_read");
assert.equal(webRead.route, "/web/dataset/call_kw/res.users/web_read");
assert.deepEqual(webRead.params.kwargs.context, { lang: "en_US" });

await env.services.action.doAction(
  {
    type: "ir.actions.act_window",
    res_model: "purchase.request",
    target: "current",
    views: [[501, "form"]]
  },
  { additional_context: { approval_auto_submit: true, default_company_id: 3 } }
);
const selectedWorkflowAction = env.services.action.history.at(-1);
assert.equal(selectedWorkflowAction.action.res_model, "purchase.request");
assert.deepEqual(selectedWorkflowAction.action.views, [[501, "form"]]);
assert.deepEqual(selectedWorkflowAction.options.additional_context, { approval_auto_submit: true, default_company_id: 3 });

console.log("frontend e2e ok");
