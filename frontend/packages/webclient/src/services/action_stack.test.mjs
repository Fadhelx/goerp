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
