import assert from "node:assert/strict";
import {
  createActionService,
  createActionStack
} from "../../../../dist/packages/webclient/src/index.js";

const stack = createActionStack();
const first = stack.push({
  id: 7,
  type: "ir.actions.act_window",
  name: "Partners",
  res_model: "res.partner",
  view_mode: "list,form"
});
assert.equal(first.title, "Partners");
assert.deepEqual(first.viewTypes, ["list", "form"]);
assert.deepEqual(stack.breadcrumbs.map((item) => item.label), ["Partners"]);

const form = stack.replace({
  id: 7,
  type: "ir.actions.act_window",
  name: "Azure Interior",
  res_model: "res.partner",
  res_id: 42,
  views: [[false, "form"], [false, "search"]]
});
assert.equal(stack.entries.length, 1);
assert.equal(form.resId, 42);
assert.deepEqual(stack.breadcrumbs.map((item) => item.label), ["Azure Interior"]);
assert.deepEqual(stack.currentRoute, {
  action: 7,
  model: "res.partner",
  view_type: "form",
  id: 42
});

const dialog = stack.push({
  type: "ir.actions.act_window",
  name: "Compose",
  target: "new",
  res_model: "mail.compose.message",
  view_mode: "form"
});
assert.equal(stack.entries.length, 2);
assert.equal(dialog.dialog, true);
assert.equal(dialog.parentId, form.id);
assert.equal(dialog.route, null);
assert.deepEqual(stack.breadcrumbs.map((item) => item.label), ["Azure Interior"]);
assert.deepEqual(stack.currentRoute, {
  action: 7,
  model: "res.partner",
  view_type: "form",
  id: 42
});
assert.equal(stack.closeCurrent().title, "Azure Interior");

const nestedStack = createActionStack();
const routeParent = nestedStack.push({
  id: 21,
  type: "ir.actions.act_window",
  name: "Tasks",
  res_model: "project.task",
  view_mode: "list,form"
});
const parentDialog = nestedStack.push({
  type: "ir.actions.act_window",
  name: "Schedule Activity",
  target: "new",
  res_model: "mail.activity",
  view_mode: "form"
});
const childDialog = nestedStack.push({
  type: "ir.actions.act_window",
  name: "Select Template",
  target: "new",
  res_model: "mail.template",
  view_mode: "form"
});
assert.equal(parentDialog.parentId, routeParent.id);
assert.equal(childDialog.parentId, parentDialog.id);
assert.deepEqual(nestedStack.currentRoute, {
  action: 21,
  model: "project.task",
  view_type: "list"
});
assert.equal(nestedStack.closeCurrent().id, parentDialog.id);

const replaceDialogStack = createActionStack();
const replaceRoute = replaceDialogStack.push({
  id: 22,
  type: "ir.actions.act_window",
  name: "Leads",
  res_model: "crm.lead",
  view_mode: "list,form"
});
const replaceWizard = replaceDialogStack.replace({
  type: "ir.actions.act_window",
  name: "Convert Lead",
  target: "new",
  res_model: "crm.lead2opportunity.partner",
  view_mode: "form"
}, { replaceLastAction: true });
assert.equal(replaceDialogStack.entries.length, 2);
assert.equal(replaceWizard.parentId, replaceRoute.id);
assert.deepEqual(replaceDialogStack.currentRoute, {
  action: 22,
  model: "crm.lead",
  view_type: "list"
});
const replacementWizard = replaceDialogStack.replace({
  type: "ir.actions.act_window",
  name: "Convert Lead Options",
  target: "new",
  res_model: "crm.lead2opportunity.partner",
  view_mode: "form"
}, { replaceLastAction: true });
assert.equal(replaceDialogStack.entries.length, 2);
assert.equal(replacementWizard.parentId, replaceRoute.id);
assert.deepEqual(replaceDialogStack.entries.map((entry) => entry.title), ["Leads", "Convert Lead Options"]);

const orders = stack.push({
  id: 12,
  type: "ir.actions.act_window",
  name: "Orders",
  target: "main",
  res_model: "sale.order",
  view_mode: "list,form"
});
assert.equal(stack.entries.length, 1);
assert.equal(orders.parentId, undefined);
assert.deepEqual(stack.breadcrumbs.map((item) => item.label), ["Orders"]);

const orderForm = stack.push({
  id: 12,
  type: "ir.actions.act_window",
  name: "S0001",
  res_model: "sale.order",
  res_id: 99,
  views: [[false, "form"]]
});
stack.push({
  id: 13,
  type: "ir.actions.act_window",
  name: "Hidden",
  res_model: "sale.order.line",
  context: { no_breadcrumbs: true },
  view_mode: "list"
});
assert.deepEqual(stack.breadcrumbs.map((item) => item.label), ["Orders", "S0001"]);
assert.equal(stack.closeTo(orders.id).title, "Orders");
assert.equal(stack.entries.length, 1);
assert.equal(orderForm.resId, 99);

const executed = [];
const action = createActionService((invocation) => {
  executed.push(invocation.action.name);
  return { ok: true };
});
await action.doAction({
  id: 1,
  type: "ir.actions.act_window",
  name: "Partners",
  res_model: "res.partner",
  view_mode: "list,form"
});
await action.doAction({
  id: 1,
  type: "ir.actions.act_window",
  name: "Partner Form",
  res_model: "res.partner",
  res_id: 5,
  view_mode: "form"
}, { replaceLastAction: true });
assert.deepEqual(executed, ["Partners", "Partner Form"]);
assert.equal(action.stack.length, 1);
assert.equal(action.current.action.res_id, 5);
assert.deepEqual(action.currentRoute, {
  action: 1,
  model: "res.partner",
  view_type: "form",
  id: 5
});
assert.deepEqual(action.breadcrumbs.map((item) => item.label), ["Partner Form"]);

await action.doAction({ type: "ir.actions.act_window_close" });
assert.equal(action.stack.length, 0);
assert.equal(action.current, null);

action.restoreStack([first]);
assert.equal(action.current.action.name, "Partners");
action.clearStack();
assert.equal(action.current, null);
