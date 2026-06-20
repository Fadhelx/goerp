import assert from "node:assert/strict";
import {
  normalizeRouteState,
  parseRouteState,
  routeStateFromAction,
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
