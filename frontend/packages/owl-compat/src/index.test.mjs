import assert from "node:assert/strict";
import {
  Component,
  EventBus,
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
  readyState: "complete",
  createElement() {
    return createElement();
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
assert.equal(htmlEscape("<b>&</b>"), "&lt;b&gt;&amp;&lt;/b&gt;");
await whenReady();
assert.equal(validate("x", "string"), true);
assert.equal(validate(1, ["string", "number"]), true);
assert.equal(validateType([1], "array"), true);
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
assert.deepEqual(events.slice(0, 4), ["willStart", "rendered", "effect", "mounted"]);
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

assert.equal(markup("<b>x</b>").toString(), "<b>x</b>");
