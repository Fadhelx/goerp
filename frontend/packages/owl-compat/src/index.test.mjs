import assert from "node:assert/strict";
import {
  Component,
  EventBus,
  OWL_UPSTREAM,
  OwlError,
  SERVICES_METADATA,
  __info__,
  batched,
  blockDom,
  htmlEscape,
  loadFile,
  markRaw,
  markup,
  mount,
  nextTick,
  onError,
  onMounted,
  onPatched,
  onRendered,
  onWillDestroy,
  onWillPatch,
  onWillStart,
  onWillUnmount,
  probeOfficialOwlRuntime,
  reactive,
  status,
  toRaw,
  useComponent,
  useEffect,
  useEnv,
  useExternalListener,
  useService,
  useServiceProtectMethodHandling,
  useState,
  useSubEnv,
  validate,
  validateType,
  whenReady,
  xml
} from "../../../dist/packages/owl-compat/src/index.js";

function createElement() {
  return {
    dataset: {},
    innerHTML: "",
    parentNode: null,
    children: [],
    appendChild(child) {
      child.parentNode = this;
      this.children.push(child);
      return child;
    },
    removeChild(child) {
      this.children = this.children.filter((candidate) => candidate !== child);
      child.parentNode = null;
      return child;
    }
  };
}

globalThis.document = {
  implementation: {
    createDocument() {
      return {};
    }
  },
  readyState: "complete",
  addEventListener() {},
  createElement() {
    return createElement();
  }
};
globalThis.window = {
  requestAnimationFrame(callback) {
    return setTimeout(callback, 0);
  },
  cancelAnimationFrame(handle) {
    clearTimeout(handle);
  }
};

class Demo extends Component {
  static template = xml`<span>demo</span>`;
}

const root = new Demo().render();
assert.equal(root.dataset.component, "Demo");
assert.match(root.innerHTML, /&lt;span&gt;demo&lt;\/span&gt;/);

const state = useState({ count: 0 });
state.count = 1;
await nextTick();
assert.equal(state.count, 1);

let reactiveCalled = false;
const reactiveState = reactive({ value: 1 }, () => {
  reactiveCalled = true;
});
assert.equal(reactiveState.value, 1);
reactiveState.value = 2;
await nextTick();
assert.equal(reactiveCalled, true);

assert.equal(__info__.version, "gorp-owl-compat");
assert.equal(OWL_UPSTREAM.packageName, "@odoo/owl");
assert.equal(OWL_UPSTREAM.license, "LGPL-3.0-only");
assert.equal(OWL_UPSTREAM.version, "2.8.3");
const officialOwl = await probeOfficialOwlRuntime();
assert.equal(officialOwl.available, true);
assert.equal(officialOwl.version, "2.8.3");
assert.equal(officialOwl.exports.includes("Component"), true);
assert.equal(officialOwl.exports.includes("EventBus"), true);
assert.equal(htmlEscape("<b>&</b>"), "&lt;b&gt;&amp;&lt;/b&gt;");
await whenReady();
assert.equal(validate("x", "string"), true);
assert.equal(validate(1, ["string", "number"]), true);
assert.equal(validateType([1], "array"), true);
assert.equal(validateType(1.2, "integer"), false);
assert.equal(blockDom.text("x"), "x");
assert.equal(blockDom.createBlock("tpl")(), "tpl");

let batchedCount = 0;
const batchedIncrement = batched(() => {
  batchedCount += 1;
});
batchedIncrement();
batchedIncrement();
await nextTick();
assert.equal(batchedCount, 1);

globalThis.fetch = async (path) => ({
  ok: path === "/ok.txt",
  async text() {
    return "file";
  }
});
assert.equal(await loadFile("/ok.txt"), "file");
await assert.rejects(() => loadFile("/missing.txt"), OwlError);

const raw = markRaw({ raw: true });
assert.equal(reactive(raw), raw);
const reactiveSource = { value: 1 };
const reactiveProxy = reactive(reactiveSource);
assert.equal(toRaw(reactiveProxy), reactiveSource);
assert.equal(reactive(reactiveSource), reactiveProxy);

let trackedCalls = 0;
const trackedState = reactive({ observed: 1, unread: 1 }, () => {
  trackedCalls += 1;
});
trackedState.unread = 2;
await nextTick();
assert.equal(trackedCalls, 0);
assert.equal(trackedState.observed, 1);
trackedState.observed = 2;
await nextTick();
assert.equal(trackedCalls, 1);
trackedState.observed = 3;
await nextTick();
assert.equal(trackedCalls, 1);
assert.equal(trackedState.observed, 3);
trackedState.observed = 4;
await nextTick();
assert.equal(trackedCalls, 2);

let nestedCalls = 0;
const childRaw = { count: 1 };
const nestedState = reactive({ child: childRaw }, () => {
  nestedCalls += 1;
});
assert.equal(nestedState.child.count, 1);
nestedState.child.count = 2;
await nextTick();
assert.equal(nestedCalls, 1);
assert.equal(toRaw(nestedState.child), childRaw);

let keyChangeCalls = 0;
const keyState = reactive({ existing: true }, () => {
  keyChangeCalls += 1;
});
assert.deepEqual(Object.keys(keyState), ["existing"]);
keyState.added = true;
await nextTick();
assert.equal(keyChangeCalls, 1);
assert.deepEqual(Object.keys(keyState), ["existing", "added"]);
delete keyState.added;
await nextTick();
assert.equal(keyChangeCalls, 2);

let hasCalls = 0;
const hasState = reactive({}, () => {
  hasCalls += 1;
});
assert.equal("flag" in hasState, false);
hasState.flag = true;
await nextTick();
assert.equal(hasCalls, 1);
assert.equal("flag" in hasState, true);
delete hasState.flag;
await nextTick();
assert.equal(hasCalls, 2);

let arrayCalls = 0;
const arrayState = reactive([1], () => {
  arrayCalls += 1;
});
assert.equal(arrayState.length, 1);
arrayState.push(2);
await nextTick();
assert.equal(arrayCalls, 1);
assert.equal(arrayState[1], 2);
arrayState[1] = 3;
await nextTick();
assert.equal(arrayCalls, 2);

const assignedChildRaw = { value: 1 };
const assignedChildProxy = reactive(assignedChildRaw);
const assignmentState = reactive({ child: null }, () => undefined);
assignmentState.child = assignedChildProxy;
assert.equal(toRaw(assignmentState.child), assignedChildRaw);

const unsupportedDate = new Date("2026-06-29T00:00:00Z");
assert.equal(reactive(unsupportedDate), unsupportedDate);

let mapCalls = 0;
const mapState = reactive(new Map([["observed", { count: 1 }]]), () => {
  mapCalls += 1;
});
assert.equal(mapState.get("observed").count, 1);
mapState.set("unread", { count: 1 });
await nextTick();
assert.equal(mapCalls, 0);
mapState.get("observed").count = 2;
await nextTick();
assert.equal(mapCalls, 1);
assert.equal(mapState.get("observed").count, 2);
mapState.set("observed", { count: 3 });
await nextTick();
assert.equal(mapCalls, 2);
assert.equal(mapState.get("observed").count, 3);

let mapKeyCalls = 0;
const keyTrackedMap = reactive(new Map([["a", 1]]), () => {
  mapKeyCalls += 1;
});
assert.deepEqual([...keyTrackedMap.keys()], ["a"]);
keyTrackedMap.set("b", 2);
await nextTick();
assert.equal(mapKeyCalls, 1);
assert.deepEqual([...keyTrackedMap.entries()], [["a", 1], ["b", 2]]);
keyTrackedMap.delete("b");
await nextTick();
assert.equal(mapKeyCalls, 2);
assert.deepEqual([...keyTrackedMap.keys()], ["a"]);
keyTrackedMap.clear();
await nextTick();
assert.equal(mapKeyCalls, 3);

let mapExistingKeyCalls = 0;
const existingKeyMap = reactive(new Map([["a", 1]]), () => {
  mapExistingKeyCalls += 1;
});
assert.deepEqual([...existingKeyMap.keys()], ["a"]);
existingKeyMap.set("a", 2);
await nextTick();
assert.equal(mapExistingKeyCalls, 1);

let mapExistingValueCalls = 0;
const existingValueMap = reactive(new Map([["a", 1]]), () => {
  mapExistingValueCalls += 1;
});
assert.deepEqual([...existingValueMap.values()], [1]);
existingValueMap.set("a", 2);
await nextTick();
assert.equal(mapExistingValueCalls, 1);

let mapExistingEntryCalls = 0;
const existingEntryMap = reactive(new Map([["a", 1]]), () => {
  mapExistingEntryCalls += 1;
});
assert.deepEqual([...existingEntryMap.entries()], [["a", 1]]);
existingEntryMap.set("a", 2);
await nextTick();
assert.equal(mapExistingEntryCalls, 1);

let mapForEachCalls = 0;
const forEachMap = reactive(new Map([["a", 1]]), () => {
  mapForEachCalls += 1;
});
const forEachSeen = [];
forEachMap.forEach((value, key, map) => {
  forEachSeen.push([key, value, map === forEachMap]);
});
assert.deepEqual(forEachSeen, [["a", 1, true]]);
forEachMap.set("a", 2);
await nextTick();
assert.equal(mapForEachCalls, 1);

const rawMapKey = { id: 1 };
const reactiveMapKey = reactive(rawMapKey);
const objectKeyMap = reactive(new Map([[rawMapKey, "raw"]]));
assert.equal(objectKeyMap.get(reactiveMapKey), "raw");
assert.equal(objectKeyMap.set(reactiveMapKey, "updated"), objectKeyMap);
assert.equal(toRaw(objectKeyMap).get(rawMapKey), "updated");

let setCalls = 0;
const setState = reactive(new Set(["observed"]), () => {
  setCalls += 1;
});
assert.equal(setState.has("observed"), true);
setState.add("unread");
await nextTick();
assert.equal(setCalls, 0);
setState.delete("observed");
await nextTick();
assert.equal(setCalls, 1);

let setKeyCalls = 0;
const keyTrackedSet = reactive(new Set(["a"]), () => {
  setKeyCalls += 1;
});
assert.deepEqual([...keyTrackedSet], ["a"]);
assert.equal(keyTrackedSet.add("b"), keyTrackedSet);
await nextTick();
assert.equal(setKeyCalls, 1);
assert.deepEqual([...keyTrackedSet.entries()], [["a", "a"], ["b", "b"]]);
keyTrackedSet.clear();
await nextTick();
assert.equal(setKeyCalls, 2);

const rawSetValue = { id: 2 };
const reactiveSetValue = reactive(rawSetValue);
const objectSet = reactive(new Set([rawSetValue]));
assert.equal(objectSet.has(reactiveSetValue), true);
objectSet.delete(reactiveSetValue);
assert.equal(toRaw(objectSet).has(rawSetValue), false);

let weakMapCalls = 0;
const rawWeakKey = { id: 3 };
const weakMapState = reactive(new WeakMap([[rawWeakKey, { count: 1 }]]), () => {
  weakMapCalls += 1;
});
const reactiveWeakKey = reactive(rawWeakKey);
assert.equal(weakMapState.has(reactiveWeakKey), true);
assert.equal(weakMapState.get(reactiveWeakKey).count, 1);
weakMapState.get(reactiveWeakKey).count = 2;
await nextTick();
assert.equal(weakMapCalls, 1);
assert.equal(weakMapState.has(reactiveWeakKey), true);
weakMapState.set(reactiveWeakKey, { count: 3 });
await nextTick();
assert.equal(weakMapCalls, 2);
assert.equal(toRaw(weakMapState).get(rawWeakKey).count, 3);

const events = [];
const externalTarget = {
  added: false,
  removed: false,
  addEventListener() {
    this.added = true;
  },
  removeEventListener() {
    this.removed = true;
  }
};

class RuntimeDemo extends Component {
  static template = xml`<div>runtime</div>`;

  setup() {
    assert.equal(useComponent(), this);
    onWillStart(() => events.push("willStart"));
    onRendered(() => events.push("rendered"));
    onMounted(() => events.push("mounted"));
    onWillPatch(() => events.push("willPatch"));
    onPatched(() => events.push("patched"));
    onWillUnmount(() => events.push("willUnmount"));
    onWillDestroy(() => events.push("willDestroy"));
    useEffect(() => {
      events.push("effect");
      return () => events.push("effectCleanup");
    });
    useSubEnv({ extra: "value" });
    useExternalListener(externalTarget, "change", () => undefined);
    assert.equal(useEnv().extra, "value");
    assert.equal(useService("rpc")("ping"), "pong:ping");
  }
}

const target = createElement();
const component = await mount(RuntimeDemo, target, {
  env: {
    services: {
      rpc: (value) => `pong:${value}`
    }
  }
});
assert.equal(status(component), "mounted");
assert.equal(target.children.length, 1);
assert.deepEqual(events.slice(0, 4), ["willStart", "rendered", "mounted", "effect"]);
component.patch();
assert.equal(events.includes("willPatch"), true);
assert.equal(events.includes("patched"), true);
component.unmount();
assert.equal(status(component), "destroyed");
assert.equal(events.includes("willUnmount"), true);
assert.equal(events.includes("willDestroy"), true);
assert.equal(events.includes("effectCleanup"), true);
assert.equal(externalTarget.added, true);
assert.equal(externalTarget.removed, true);

const effectEvents = [];
class EffectDepsDemo extends Component {
  static template = xml`<div>effect</div>`;

  setup() {
    this.state = useState({ value: "a" });
    onMounted(() => effectEvents.push("mounted"));
    onPatched(() => effectEvents.push("patched"));
    useEffect((value) => {
      effectEvents.push(`effect:${value}`);
      return () => effectEvents.push(`cleanup:${value}`);
    }, () => [this.state.value]);
  }
}

const effectComponent = await mount(EffectDepsDemo, createElement());
assert.deepEqual(effectEvents, ["mounted", "effect:a"]);
effectComponent.patch();
assert.deepEqual(effectEvents, ["mounted", "effect:a", "patched"]);
effectComponent.state.value = "b";
await nextTick();
assert.deepEqual(effectEvents, ["mounted", "effect:a", "patched", "patched", "cleanup:a", "effect:b"]);
effectComponent.unmount();
assert.deepEqual(effectEvents, ["mounted", "effect:a", "patched", "patched", "cleanup:a", "effect:b", "cleanup:b"]);

let protectedService;
class ProtectedServiceDemo extends Component {
  setup() {
    protectedService = useService("orm");
  }
}
SERVICES_METADATA.orm = ["read"];
const protectedComponent = await mount(ProtectedServiceDemo, createElement(), {
  env: {
    services: {
      orm: {
        read(value) {
          return Promise.resolve(`read:${value}`);
        },
        plain(value) {
          return `plain:${value}`;
        }
      }
    }
  }
});
assert.equal(await protectedService.read("ok"), "read:ok");
assert.equal(protectedService.plain("ok"), "plain:ok");
protectedComponent.unmount();
await assert.rejects(() => protectedService.read("late"), /Component is destroyed/);
delete SERVICES_METADATA.orm;

let protectedFunction;
class ProtectedFunctionDemo extends Component {
  setup() {
    protectedFunction = useService("rpc");
  }
}
SERVICES_METADATA.rpc = true;
const protectedFunctionComponent = await mount(ProtectedFunctionDemo, createElement(), {
  env: {
    services: {
      rpc(value) {
        return Promise.resolve(`rpc:${value}`);
      }
    }
  }
});
assert.equal(await protectedFunction("ok"), "rpc:ok");
const originalDestroyedHandler = useServiceProtectMethodHandling.fn;
useServiceProtectMethodHandling.fn = useServiceProtectMethodHandling.mocked;
protectedFunctionComponent.unmount();
const unresolved = protectedFunction("late");
assert.equal(typeof unresolved.then, "function");
useServiceProtectMethodHandling.fn = originalDestroyedHandler;
delete SERVICES_METADATA.rpc;

let lifecycleError = null;
class ErrorDemo extends Component {
  setup() {
    onError((error) => {
      lifecycleError = error;
    });
    onWillStart(() => {
      throw new Error("boom");
    });
  }
}

await mount(ErrorDemo, createElement());
assert.equal(lifecycleError instanceof OwlError, true);
assert.equal(lifecycleError.cause.message, "boom");

const bus = new EventBus();
let detail = null;
bus.on("ready", (event) => {
  detail = event.detail;
});
bus.trigger("ready", { ok: true });
assert.deepEqual(detail, { ok: true });

class AutoPatchDemo extends Component {
  static template = xml`<div></div>`;

  setup() {
    this.state = useState({ count: 0 });
  }

  render() {
    const el = super.render();
    el.innerHTML = `count:${this.state.count}`;
    return el;
  }
}

const autoPatchTarget = createElement();
const autoPatchComponent = await mount(AutoPatchDemo, autoPatchTarget);
const firstAutoPatchEl = autoPatchTarget.children[0];
assert.equal(firstAutoPatchEl.innerHTML, "count:0");
autoPatchComponent.state.count = 2;
await nextTick();
assert.equal(autoPatchTarget.children.length, 1);
assert.equal(autoPatchTarget.children[0].innerHTML, "count:2");
assert.notEqual(autoPatchTarget.children[0], firstAutoPatchEl);
autoPatchComponent.unmount();

assert.equal(markup("<b>x</b>").toString(), "<b>x</b>");
