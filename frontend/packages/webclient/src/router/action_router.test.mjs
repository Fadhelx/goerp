import assert from "node:assert/strict";
import { createActionStack } from "../../../../dist/packages/webclient/src/services/action_stack.js";
import {
  normalizeRouteState,
  parseRouteState,
  routeStateFromAction,
  routeStateFromStack,
  routeStateToURL,
  serializeRouteState,
  updateBrowserRoute
} from "../../../../dist/packages/webclient/src/router/action_router.js";

const state = routeStateFromAction({
  id: 9,
  type: "ir.actions.act_window",
  res_model: "res.partner",
  res_id: 42,
  menu_id: 3,
  views: [[false, "list"], [false, "form"], [false, "search"]]
}, {
  active_ids: [1, 2],
  debug: true
});

assert.deepEqual(state, {
  action: 9,
  model: "res.partner",
  view_type: "list",
  id: 42,
  menu_id: 3,
  active_ids: [1, 2],
  debug: true
});

const hash = serializeRouteState(state);
assert.equal(hash, "#action=9&model=res.partner&view_type=list&id=42&menu_id=3&active_ids=1%2C2&debug=1");
assert.deepEqual(parseRouteState(hash), state);
assert.equal(routeStateToURL("/web", state), `/web${hash}`);
assert.deepEqual(normalizeRouteState({ action: "9", id: "42", model: "res.partner", empty: "" }), {
  action: 9,
  id: 42,
  model: "res.partner"
});

const urls = [];
const target = {
  location: { pathname: "/web", search: "?db=test", hash: "" },
  history: {
    pushState(_data, _unused, url) {
      urls.push(["push", url]);
    },
    replaceState(_data, _unused, url) {
      urls.push(["replace", url]);
    }
  }
};
assert.equal(updateBrowserRoute({ action: 4 }, { target }), "/web?db=test#action=4");
assert.deepEqual(urls[0], ["push", "/web?db=test#action=4"]);
assert.equal(updateBrowserRoute({ action: 5 }, { target, replace: true }), "/web?db=test#action=5");
assert.deepEqual(urls[1], ["replace", "/web?db=test#action=5"]);

const stack = createActionStack();
stack.push({
  id: 9,
  type: "ir.actions.act_window",
  name: "Partners",
  res_model: "res.partner",
  menu_id: 3,
  view_mode: "list,form"
});
stack.push({
  id: 9,
  type: "ir.actions.act_window",
  name: "Azure Interior",
  res_model: "res.partner",
  res_id: 42,
  menu_id: 3,
  views: [[false, "form"]]
});
stack.push({
  id: 10,
  type: "ir.actions.act_window",
  name: "Compose",
  target: "new",
  res_model: "mail.compose.message",
  view_mode: "form"
});

const routeFromStack = routeStateFromStack(stack.entries, { debug: true });
assert.equal(routeFromStack.action, 9);
assert.equal(routeFromStack.model, "res.partner");
assert.equal(routeFromStack.view_type, "form");
assert.equal(routeFromStack.id, 42);
assert.equal(routeFromStack.debug, true);
assert.deepEqual(routeFromStack.actionStack.map((item) => item.displayName), ["Partners", "Azure Interior"]);
assert.equal(routeFromStack.actionStack.length, 2);
assert.equal(serializeRouteState(routeFromStack), "#action=9&model=res.partner&view_type=form&id=42&menu_id=3&debug=1");

const routeFromStackWithActiveIds = routeStateFromStack(stack.entries, {
  active_id: 42,
  active_ids: [42, 43]
});
const activeRouteEntry = routeFromStackWithActiveIds.actionStack.at(-1);
assert.equal(activeRouteEntry.active_id, 42);
assert.deepEqual(activeRouteEntry.active_ids, [42, 43]);
assert.equal(routeFromStackWithActiveIds.active_id, 42);
assert.deepEqual(routeFromStackWithActiveIds.active_ids, [42, 43]);
assert.equal(serializeRouteState(routeFromStackWithActiveIds), "#action=9&model=res.partner&view_type=form&id=42&menu_id=3&active_id=42&active_ids=42%2C43");

assert.deepEqual(routeStateFromStack([], {
  action: 88,
  actionStack: [{ action: 99, displayName: "Stale" }]
}), {
  action: 88
});

const stackUrls = [];
const stackTarget = {
  location: { pathname: "/web", search: "", hash: "" },
  history: {
    pushState(data, _unused, url) {
      stackUrls.push(["push", url, data]);
    },
    replaceState(data, _unused, url) {
      stackUrls.push(["replace", url, data]);
    }
  }
};
assert.equal(updateBrowserRoute(routeFromStack, { target: stackTarget, replace: true }), "/web#action=9&model=res.partner&view_type=form&id=42&menu_id=3&debug=1");
assert.equal(stackUrls[0][0], "replace");
assert.equal(stackUrls[0][1], "/web#action=9&model=res.partner&view_type=form&id=42&menu_id=3&debug=1");
assert.deepEqual(stackUrls[0][2].actionStack.map((item) => item.displayName), ["Partners", "Azure Interior"]);
