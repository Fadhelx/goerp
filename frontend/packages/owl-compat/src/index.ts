export type Env = Record<string, unknown>;
export type Props = Record<string, unknown>;
export type Template = string;
export type ComponentStatus = "new" | "mounted" | "destroyed" | "cancelled";
export type LifecycleCallback = () => void | Promise<void>;
export type ErrorCallback = (error: unknown) => void;
export type Cleanup = () => void;
export type ServiceAsyncMetadata = true | readonly string[];
type ReactiveKey = PropertyKey | object;

let currentComponent: Component | null = null;
const rawObjects = new WeakSet<object>();
const rawToProxy = new WeakMap<object, WeakMap<() => void, object>>();
const proxyToRaw = new WeakMap<object, object>();
const targetToKeysToCallbacks = new WeakMap<object, Map<ReactiveKey, Set<() => void>>>();
const callbacksToTargets = new WeakMap<() => void, Set<object>>();
export const KEYCHANGES = Symbol("KEYCHANGES");

export const SERVICES_METADATA: Record<string, ServiceAsyncMetadata> = {};

export const useServiceProtectMethodHandling = {
  fn(): Promise<never> {
    return this.original();
  },
  mocked(): Promise<never> {
    return new Promise(() => {});
  },
  original(): Promise<never> {
    return Promise.reject(new Error("Component is destroyed"));
  }
};

export const __info__ = {
  version: "gorp-owl-compat",
  date: "2026-06-17"
};

export const OWL_UPSTREAM = {
  packageName: "@odoo/owl",
  license: "LGPL-3.0-only",
  version: "2.8.3",
  url: "https://github.com/odoo/owl"
};

export interface OfficialOwlRuntimeProbe {
  available: boolean;
  version?: string;
  exports: readonly string[];
  error?: string;
}

export async function loadOfficialOwlRuntime(): Promise<Record<string, unknown>> {
  const mod = await dynamicImport(OWL_UPSTREAM.packageName + "/dist/owl.cjs.js");
  return moduleRuntime(mod);
}

export async function probeOfficialOwlRuntime(): Promise<OfficialOwlRuntimeProbe> {
  try {
    const runtime = await loadOfficialOwlRuntime();
    return {
      available: typeof runtime.Component === "function" && typeof runtime.EventBus === "function",
      version: moduleVersion(runtime),
      exports: Object.keys(runtime).sort()
    };
  } catch (error) {
    return {
      available: false,
      exports: [],
      error: error instanceof Error ? error.message : String(error)
    };
  }
}

export class OwlError extends Error {
  readonly cause?: unknown;

  constructor(message: string, options: { cause?: unknown } = {}) {
    super(message);
    this.name = "OwlError";
    this.cause = options.cause;
  }
}

export class Component<P extends Props = Props> {
  static template?: Template;
  static props?: Record<string, unknown>;

  readonly props: P;
  env: Env;
  childEnv: Env | null = null;
  private readonly cleanups: Array<() => void> = [];
  private readonly hooks: Record<string, LifecycleCallback[]> = {};
  private readonly errorHandlers: ErrorCallback[] = [];
  private setupDone = false;
  private statusValue: ComponentStatus = "new";
  el: HTMLElement | null = null;

  constructor(props = {} as P, env: Env = {}) {
    this.props = props;
    this.env = env;
  }

  setup(): void {}

  render(): HTMLElement {
    this.ensureSetup();
    this.runSyncHooks("willRender");
    const root = document.createElement("div");
    root.dataset.component = this.constructor.name;
    root.innerHTML = escapeText(String((this.constructor as typeof Component).template ?? ""));
    this.el = root;
    this.runSyncHooks("rendered");
    return root;
  }

  async willStart(): Promise<void> {
    this.ensureSetup();
    await this.runHooks("willStart");
  }

  async updateProps(_nextProps: Partial<P>): Promise<void> {
    await this.runHooks("willUpdateProps");
  }

  patch(): HTMLElement {
    this.runSyncHooks("willPatch");
    const previous = this.el;
    const next = this.render();
    replaceMountedElement(previous, next);
    this.runSyncHooks("patched");
    return next;
  }

  requestPatch(): void {
    if (this.statusValue !== "mounted") return;
    this.patch();
  }

  onWillUnmount(cleanup: () => void): void {
    this.cleanups.push(cleanup);
  }

  addHook(name: string, callback: LifecycleCallback): void {
    this.hooks[name] ??= [];
    this.hooks[name].push(callback);
  }

  addErrorHandler(callback: ErrorCallback): void {
    this.errorHandlers.push(callback);
  }

  triggerHook(name: string): void {
    this.runSyncHooks(name);
  }

  setStatus(value: ComponentStatus): void {
    this.statusValue = value;
  }

  status(): ComponentStatus {
    return this.statusValue;
  }

  unmount(): void {
    this.statusValue = "destroyed";
    this.runSyncHooks("willUnmount");
    for (const cleanup of this.cleanups.splice(0)) cleanup();
    if (this.el?.parentNode) {
      this.el.parentNode.removeChild(this.el);
    }
    this.el = null;
    this.runSyncHooks("willDestroy");
  }

  private ensureSetup(): void {
    if (this.setupDone) return;
    withCurrent(this, () => this.setup());
    this.setupDone = true;
  }

  private async runHooks(name: string): Promise<void> {
    for (const hook of this.hooks[name] ?? []) {
      try {
        await hook();
      } catch (error) {
        this.handleError(error);
      }
    }
  }

  private runSyncHooks(name: string): void {
    for (const hook of this.hooks[name] ?? []) {
      try {
        void hook();
      } catch (error) {
        this.handleError(error);
      }
    }
  }

  private handleError(error: unknown): void {
    const wrapped = error instanceof OwlError ? error : new OwlError("An Owl lifecycle error occurred", { cause: error });
    if (!this.errorHandlers.length) throw wrapped;
    for (const handler of this.errorHandlers) handler(wrapped);
  }
}

export interface AppOptions<P extends Props = Props> {
  props?: P;
  env?: Env;
}

export class App<C extends Component = Component> {
  readonly root: C;
  private target: HTMLElement | null = null;

  constructor(Root: new (props?: Props, env?: Env) => C, options: AppOptions = {}) {
    this.root = new Root(options.props, options.env);
  }

  async mount(target: HTMLElement): Promise<C> {
    this.target = target;
    await this.root.willStart();
    const el = this.root.render();
    append(target, el);
    this.root.setStatus("mounted");
    this.root.addHook("willUnmount", () => {
      if (this.target) clearTarget(this.target, el);
    });
    await nextTick();
    this.root.triggerHook("mounted");
    return this.root;
  }

  destroy(): void {
    this.root.unmount();
    this.target = null;
  }
}

export function mount<C extends Component>(
  Root: new (props?: Props, env?: Env) => C,
  target: HTMLElement,
  options: AppOptions = {}
): Promise<C> {
  return new App(Root, options).mount(target);
}

export function xml(strings: TemplateStringsArray, ...values: unknown[]): Template {
  let out = "";
  for (let i = 0; i < strings.length; i += 1) {
    out += strings[i];
    if (i < values.length) out += String(values[i]);
  }
  return out.trim();
}

export function htmlEscape(value: unknown): string {
  if (isMarkup(value)) return value.toString();
  return escapeText(String(value ?? ""));
}

export function whenReady(callback?: () => void | Promise<void>): Promise<void> {
  const ready = typeof document === "undefined" || document.readyState !== "loading"
    ? Promise.resolve()
    : new Promise<void>((resolve) => document.addEventListener("DOMContentLoaded", () => resolve(), { once: true }));
  return callback ? ready.then(() => callback()).then(() => undefined) : ready;
}

export function batched<T extends (...args: never[]) => unknown>(callback: T): T {
  let scheduled = false;
  let lastArgs: Parameters<T> | null = null;
  return function batchedCallback(this: unknown, ...args: Parameters<T>) {
    lastArgs = args;
    if (scheduled) return undefined;
    scheduled = true;
    queueMicrotask(() => {
      scheduled = false;
      const callArgs = lastArgs ?? ([] as unknown as Parameters<T>);
      lastArgs = null;
      callback.apply(this, callArgs);
    });
    return undefined;
  } as T;
}

export function validate(value: unknown, spec?: unknown): boolean {
  if (spec === undefined || spec === null) return true;
  if (typeof spec === "function") return Boolean((spec as (value: unknown) => boolean)(value));
  if (Array.isArray(spec)) return spec.some((item) => validate(value, item));
  if (typeof spec === "string") return validateType(value, spec);
  return true;
}

export function validateType(value: unknown, type: string): boolean {
  switch (type) {
    case "array":
      return Array.isArray(value);
    case "object":
      return typeof value === "object" && value !== null && !Array.isArray(value);
    case "integer":
      return Number.isInteger(value);
    default:
      return typeof value === type;
  }
}

export async function loadFile(path: string): Promise<string> {
  if (typeof fetch !== "function") throw new OwlError("loadFile requires fetch");
  const response = await fetch(path);
  if (!response.ok) throw new OwlError(`Unable to load file: ${path}`);
  return response.text();
}

export const blockDom = {
  createBlock(template: string) {
    return () => template;
  },
  list(items: unknown[]) {
    return items;
  },
  multi(items: unknown[]) {
    return items;
  },
  text(value: unknown) {
    return String(value ?? "");
  }
};

export function useState<T extends object>(initial: T): T {
  const component = currentComponent;
  if (!component) return reactive(initial);
  const callback = () => component.requestPatch();
  component.onWillUnmount(() => clearReactiveSubscriptions(callback));
  return reactive(initial, callback);
}

export function reactive<T extends object>(initial: T, callback: () => void = scheduleNoop): T {
  const target = toRaw(initial);
  if (!canBeMadeReactive(target)) return target;
  const proxyCache = rawToProxy.get(target) ?? new WeakMap<() => void, object>();
  rawToProxy.set(target, proxyCache);
  const existing = proxyCache.get(callback);
  if (existing) return existing as T;
  const proxy = collectionHandler(target, callback) ?? new Proxy(target, {
    get(target, key, receiver) {
      if (key === "__raw__") return target;
      subscribeReactive(target, key, callback);
      const value = Reflect.get(target, key, receiver);
      return reactiveChild(value, callback);
    },
    has(target, key) {
      subscribeReactive(target, key, callback);
      subscribeReactive(target, KEYCHANGES, callback);
      return Reflect.has(target, key);
    },
    ownKeys(target) {
      subscribeReactive(target, KEYCHANGES, callback);
      return Reflect.ownKeys(target);
    },
    set(target, key, value) {
      const hadKey = Reflect.has(target, key);
      const oldValue = Reflect.get(target, key);
      const nextValue = toRaw(value);
      const changed = oldValue !== nextValue;
      const ok = Reflect.set(target, key, nextValue);
      if (ok && changed) {
        notifyReactive(target, key);
        if (!hadKey) notifyReactive(target, KEYCHANGES);
        if (Array.isArray(target) && key !== "length") notifyReactive(target, "length");
      }
      return ok;
    },
    deleteProperty(target, key) {
      const hadKey = Reflect.has(target, key);
      const ok = Reflect.deleteProperty(target, key);
      if (ok && hadKey) {
        notifyReactive(target, key);
        notifyReactive(target, KEYCHANGES);
        if (Array.isArray(target) && key !== "length") notifyReactive(target, "length");
      }
      return true;
    }
  });
  proxyCache.set(callback, proxy);
  proxyToRaw.set(proxy, target);
  return proxy as T;
}

export function markRaw<T extends object>(value: T): T {
  rawObjects.add(value);
  return value;
}

export function toRaw<T>(value: T): T {
  return (proxyToRaw.get(value as object) as T | undefined) ?? value;
}

function reactiveChild(value: unknown, callback: () => void): unknown {
  if (typeof value !== "object" || value === null || !canBeMadeReactive(value)) return value;
  return reactive(value as object, callback);
}

function canBeMadeReactive<T extends object>(value: T): value is T {
  if (rawObjects.has(value)) return false;
  return ["Object", "Array", "Set", "Map", "WeakMap"].includes(rawType(value));
}

function rawType(value: object): string {
  return Object.prototype.toString.call(value).slice(8, -1);
}

function collectionHandler(target: object, callback: () => void): object | null {
  if (target instanceof Map) return reactiveMap(target, callback);
  if (target instanceof Set) return reactiveSet(target, callback);
  if (target instanceof WeakMap) return reactiveWeakMap(target, callback);
  return null;
}

function reactiveMap<K, V>(target: Map<K, V>, callback: () => void): Map<K, V> {
  let proxy: Map<K, V>;
  proxy = new Proxy(target, {
    get(mapTarget, key) {
      if (key === "__raw__") return mapTarget;
      if (key === "size") {
        subscribeReactive(mapTarget, KEYCHANGES, callback);
        return mapTarget.size;
      }
      if (key === "get") {
        return (itemKey: K) => {
          const rawKey = toRaw(itemKey);
          subscribeReactive(mapTarget, rawKey as ReactiveKey, callback);
          return reactiveChild(mapTarget.get(rawKey), callback);
        };
      }
      if (key === "has") {
        return (itemKey: K) => {
          const rawKey = toRaw(itemKey);
          subscribeReactive(mapTarget, rawKey as ReactiveKey, callback);
          return mapTarget.has(rawKey);
        };
      }
      if (key === "set") {
        return (itemKey: K, itemValue: V) => {
          const rawKey = toRaw(itemKey);
          const rawValue = toRaw(itemValue);
          const hadKey = mapTarget.has(rawKey);
          const oldValue = mapTarget.get(rawKey);
          mapTarget.set(rawKey, rawValue);
          if (!hadKey) {
            notifyReactive(mapTarget, rawKey as ReactiveKey);
            notifyReactive(mapTarget, KEYCHANGES);
          } else if (oldValue !== rawValue) {
            notifyReactive(mapTarget, rawKey as ReactiveKey);
          }
          return proxy;
        };
      }
      if (key === "delete") {
        return (itemKey: K) => {
          const rawKey = toRaw(itemKey);
          const hadKey = mapTarget.has(rawKey);
          const deleted = mapTarget.delete(rawKey);
          if (hadKey && deleted) {
            notifyReactive(mapTarget, rawKey as ReactiveKey);
            notifyReactive(mapTarget, KEYCHANGES);
          }
          return deleted;
        };
      }
      if (key === "clear") {
        return () => {
          if (!mapTarget.size) return undefined;
          mapTarget.clear();
          notifyReactiveAll(mapTarget);
          return undefined;
        };
      }
      if (key === "keys") return () => reactiveMapKeyIterator(mapTarget, mapTarget.keys(), callback);
      if (key === "values") return () => reactiveMapValueIterator(mapTarget, mapTarget.entries(), callback);
      if (key === "entries" || key === Symbol.iterator) return () => reactiveMapEntryIterator(mapTarget, mapTarget.entries(), callback);
      if (key === "forEach") {
        return (fn: (value: V, key: K, map: Map<K, V>) => void, thisArg?: unknown) => {
          subscribeReactive(mapTarget, KEYCHANGES, callback);
          mapTarget.forEach((value, itemKey) => {
            subscribeReactive(mapTarget, itemKey as ReactiveKey, callback);
            fn.call(thisArg, reactiveChild(value, callback) as V, reactiveChild(itemKey, callback) as K, proxy);
          });
        };
      }
      const value = Reflect.get(mapTarget, key, mapTarget);
      return typeof value === "function" ? value.bind(mapTarget) : value;
    }
  });
  return proxy;
}

function reactiveSet<T>(target: Set<T>, callback: () => void): Set<T> {
  let proxy: Set<T>;
  proxy = new Proxy(target, {
    get(setTarget, key) {
      if (key === "__raw__") return setTarget;
      if (key === "size") {
        subscribeReactive(setTarget, KEYCHANGES, callback);
        return setTarget.size;
      }
      if (key === "has") {
        return (item: T) => {
          const rawItem = toRaw(item);
          subscribeReactive(setTarget, rawItem as ReactiveKey, callback);
          return setTarget.has(rawItem);
        };
      }
      if (key === "add") {
        return (item: T) => {
          const rawItem = toRaw(item);
          const hadItem = setTarget.has(rawItem);
          setTarget.add(rawItem);
          if (!hadItem) {
            notifyReactive(setTarget, rawItem as ReactiveKey);
            notifyReactive(setTarget, KEYCHANGES);
          }
          return proxy;
        };
      }
      if (key === "delete") {
        return (item: T) => {
          const rawItem = toRaw(item);
          const hadItem = setTarget.has(rawItem);
          const deleted = setTarget.delete(rawItem);
          if (hadItem && deleted) {
            notifyReactive(setTarget, rawItem as ReactiveKey);
            notifyReactive(setTarget, KEYCHANGES);
          }
          return deleted;
        };
      }
      if (key === "clear") {
        return () => {
          if (!setTarget.size) return undefined;
          setTarget.clear();
          notifyReactiveAll(setTarget);
          return undefined;
        };
      }
      if (key === "keys" || key === "values" || key === Symbol.iterator) return () => reactiveIterator(setTarget, setTarget.values(), callback);
      if (key === "entries") return () => reactiveEntryIterator(setTarget, setTarget.entries(), callback);
      if (key === "forEach") {
        return (fn: (value: T, key: T, set: Set<T>) => void, thisArg?: unknown) => {
          subscribeReactive(setTarget, KEYCHANGES, callback);
          setTarget.forEach((value) => {
            const reactiveValue = reactiveChild(value, callback) as T;
            fn.call(thisArg, reactiveValue, reactiveValue, proxy);
          });
        };
      }
      const value = Reflect.get(setTarget, key, setTarget);
      return typeof value === "function" ? value.bind(setTarget) : value;
    }
  });
  return proxy;
}

function reactiveWeakMap<K extends object, V>(target: WeakMap<K, V>, callback: () => void): WeakMap<K, V> {
  let proxy: WeakMap<K, V>;
  proxy = new Proxy(target, {
    get(mapTarget, key) {
      if (key === "__raw__") return mapTarget;
      if (key === "get") {
        return (itemKey: K) => {
          const rawKey = toRaw(itemKey);
          subscribeReactive(mapTarget, rawKey as ReactiveKey, callback);
          return reactiveChild(mapTarget.get(rawKey), callback);
        };
      }
      if (key === "has") {
        return (itemKey: K) => {
          const rawKey = toRaw(itemKey);
          subscribeReactive(mapTarget, rawKey as ReactiveKey, callback);
          return mapTarget.has(rawKey);
        };
      }
      if (key === "set") {
        return (itemKey: K, itemValue: V) => {
          const rawKey = toRaw(itemKey);
          const rawValue = toRaw(itemValue);
          const hadKey = mapTarget.has(rawKey);
          const oldValue = mapTarget.get(rawKey);
          mapTarget.set(rawKey, rawValue);
          if (!hadKey || oldValue !== rawValue) notifyReactive(mapTarget, rawKey as ReactiveKey);
          return proxy;
        };
      }
      if (key === "delete") {
        return (itemKey: K) => {
          const rawKey = toRaw(itemKey);
          const hadKey = mapTarget.has(rawKey);
          const deleted = mapTarget.delete(rawKey);
          if (hadKey && deleted) notifyReactive(mapTarget, rawKey as ReactiveKey);
          return deleted;
        };
      }
      const value = Reflect.get(mapTarget, key, mapTarget);
      return typeof value === "function" ? value.bind(mapTarget) : value;
    }
  });
  return proxy;
}

function reactiveIterator<T>(target: object, iterator: IterableIterator<T>, callback: () => void): IterableIterator<T> {
  subscribeReactive(target, KEYCHANGES, callback);
  return {
    [Symbol.iterator]() {
      return this;
    },
    next() {
      const item = iterator.next();
      return item.done ? item : { done: false, value: reactiveChild(item.value, callback) as T };
    }
  };
}

function reactiveMapKeyIterator<K>(target: Map<K, unknown>, iterator: IterableIterator<K>, callback: () => void): IterableIterator<K> {
  subscribeReactive(target, KEYCHANGES, callback);
  return {
    [Symbol.iterator]() {
      return this;
    },
    next() {
      const item = iterator.next();
      if (item.done) return item;
      subscribeReactive(target, item.value as ReactiveKey, callback);
      return { done: false, value: reactiveChild(item.value, callback) as K };
    }
  };
}

function reactiveMapValueIterator<K, V>(
  target: Map<K, V>,
  iterator: IterableIterator<[K, V]>,
  callback: () => void
): IterableIterator<V> {
  subscribeReactive(target, KEYCHANGES, callback);
  return {
    [Symbol.iterator]() {
      return this;
    },
    next() {
      const item = iterator.next();
      if (item.done) return item;
      subscribeReactive(target, item.value[0] as ReactiveKey, callback);
      return { done: false, value: reactiveChild(item.value[1], callback) as V };
    }
  };
}

function reactiveMapEntryIterator<K, V>(
  target: Map<K, V>,
  iterator: IterableIterator<[K, V]>,
  callback: () => void
): IterableIterator<[K, V]> {
  subscribeReactive(target, KEYCHANGES, callback);
  return {
    [Symbol.iterator]() {
      return this;
    },
    next() {
      const item = iterator.next();
      if (item.done) return item;
      subscribeReactive(target, item.value[0] as ReactiveKey, callback);
      return {
        done: false,
        value: [reactiveChild(item.value[0], callback) as K, reactiveChild(item.value[1], callback) as V]
      };
    }
  };
}

function reactiveEntryIterator<K, V>(target: object, iterator: IterableIterator<[K, V]>, callback: () => void): IterableIterator<[K, V]> {
  subscribeReactive(target, KEYCHANGES, callback);
  return {
    [Symbol.iterator]() {
      return this;
    },
    next() {
      const item = iterator.next();
      if (item.done) return item;
      return {
        done: false,
        value: [reactiveChild(item.value[0], callback) as K, reactiveChild(item.value[1], callback) as V]
      };
    }
  };
}

function subscribeReactive(target: object, key: ReactiveKey, callback: () => void): void {
  if (callback === scheduleNoop) return;
  let keyMap = targetToKeysToCallbacks.get(target);
  if (!keyMap) {
    keyMap = new Map();
    targetToKeysToCallbacks.set(target, keyMap);
  }
  let callbacks = keyMap.get(key);
  if (!callbacks) {
    callbacks = new Set();
    keyMap.set(key, callbacks);
  }
  callbacks.add(callback);
  let targets = callbacksToTargets.get(callback);
  if (!targets) {
    targets = new Set();
    callbacksToTargets.set(callback, targets);
  }
  targets.add(target);
}

function notifyReactive(target: object, key: ReactiveKey): void {
  const callbacks = targetToKeysToCallbacks.get(target)?.get(key);
  if (!callbacks?.size) return;
  for (const callback of Array.from(callbacks)) {
    clearReactiveSubscriptions(callback);
    queueMicrotask(callback);
  }
}

function notifyReactiveAll(target: object): void {
  const keyMap = targetToKeysToCallbacks.get(target);
  if (!keyMap?.size) return;
  const callbacks = new Set<() => void>();
  for (const subscribers of keyMap.values()) {
    for (const callback of subscribers) callbacks.add(callback);
  }
  for (const callback of callbacks) {
    clearReactiveSubscriptions(callback);
    queueMicrotask(callback);
  }
}

function clearReactiveSubscriptions(callback: () => void): void {
  const targets = callbacksToTargets.get(callback);
  if (!targets) return;
  for (const target of targets) {
    const keyMap = targetToKeysToCallbacks.get(target);
    if (!keyMap) continue;
    for (const callbacks of keyMap.values()) callbacks.delete(callback);
  }
  callbacksToTargets.delete(callback);
}

export function useRef<T = HTMLElement>(name: string): { name: string; el: T | null } {
  return { name, el: null };
}

export function useComponent<C extends Component = Component>(): C {
  if (!currentComponent) throw new Error("useComponent must be called during setup");
  return currentComponent as C;
}

export function useEnv<T extends Env = Env>(): T {
  return (currentComponent?.env ?? {}) as T;
}

export function useSubEnv(env: Env): void {
  if (!currentComponent) throw new Error("useSubEnv must be called during setup");
  currentComponent.env = { ...currentComponent.env, ...env };
}

export function useChildSubEnv(env: Env): void {
  if (!currentComponent) throw new Error("useChildSubEnv must be called during setup");
  currentComponent.childEnv = { ...(currentComponent.childEnv ?? currentComponent.env), ...env };
}

export function useService<T = unknown>(name: string): T {
  const component = useComponent();
  const env = component.env as { services?: Record<string, T> };
  if (!env.services || !(name in env.services)) {
    throw new Error(`Service ${name} is not available`);
  }
  const service = env.services[name] as T;
  const metadata = SERVICES_METADATA[name];
  if (!metadata) return service;
  if (typeof service === "function") {
    return protectServiceMethod(component, service as (...args: unknown[]) => unknown) as T;
  }
  const methods = metadata === true ? Object.keys(service as Record<string, unknown>) : metadata;
  const result = Object.create(service as object) as Record<string, unknown>;
  for (const method of methods) {
    const value = (service as Record<string, unknown>)[method];
    if (typeof value === "function") {
      result[method] = protectServiceMethod(component, value as (...args: unknown[]) => unknown);
    }
  }
  return result as T;
}

export function onMounted(callback: () => void): void {
  addLifecycleHook("mounted", callback);
}

export function onWillStart(callback: LifecycleCallback): void {
  addLifecycleHook("willStart", callback);
}

export function onRendered(callback: LifecycleCallback): void {
  addLifecycleHook("rendered", callback);
}

export function onWillRender(callback: LifecycleCallback): void {
  addLifecycleHook("willRender", callback);
}

export function onWillPatch(callback: LifecycleCallback): void {
  addLifecycleHook("willPatch", callback);
}

export function onPatched(callback: LifecycleCallback): void {
  addLifecycleHook("patched", callback);
}

export function onWillUpdateProps(callback: LifecycleCallback): void {
  addLifecycleHook("willUpdateProps", callback);
}

export function onWillUnmount(callback: () => void): void {
  addLifecycleHook("willUnmount", callback);
}

export function onWillDestroy(callback: () => void): void {
  addLifecycleHook("willDestroy", callback);
}

export function onError(callback: ErrorCallback): void {
  if (!currentComponent) throw new Error("error hook must be registered during setup");
  currentComponent.addErrorHandler(callback);
}

export function useEffect(effect: (...deps: unknown[]) => void | Cleanup, _deps?: () => unknown[]): void {
  const component = currentComponent;
  let cleanup: Cleanup | null = null;
  let previousDeps: unknown[] | null = null;
  const runEffect = () => {
    const nextDeps = _deps?.() ?? null;
    if (previousDeps && nextDeps && sameDeps(previousDeps, nextDeps)) return;
    cleanup?.();
    previousDeps = nextDeps ? [...nextDeps] : null;
    const nextCleanup = effect(...(nextDeps ?? []));
    cleanup = typeof nextCleanup === "function" ? nextCleanup : null;
  };
  addLifecycleHook("mounted", runEffect);
  addLifecycleHook("patched", runEffect);
  component?.onWillUnmount(() => {
    cleanup?.();
    cleanup = null;
  });
}

export function useExternalListener(
  target: EventTarget,
  type: string,
  listener: EventListenerOrEventListenerObject,
  options?: AddEventListenerOptions | boolean
): void {
  onMounted(() => target.addEventListener(type, listener, options));
  onWillUnmount(() => target.removeEventListener(type, listener, options));
}

export function nextTick(): Promise<void> {
  return new Promise((resolve) => queueMicrotask(resolve));
}

export function status(component: Component): ComponentStatus {
  return component.status();
}

export class EventBus {
  private readonly listeners = new Map<string, Set<(event: CustomEvent) => void>>();

  addEventListener(type: string, listener: (event: CustomEvent) => void): void {
    this.listeners.get(type)?.add(listener) ?? this.listeners.set(type, new Set([listener]));
  }

  removeEventListener(type: string, listener: (event: CustomEvent) => void): void {
    this.listeners.get(type)?.delete(listener);
  }

  dispatchEvent(event: CustomEvent): boolean {
    for (const listener of this.listeners.get(event.type) ?? []) listener(event);
    return true;
  }

  trigger(type: string, detail?: unknown): void {
    this.dispatchEvent(makeCustomEvent(type, detail));
  }

  on(type: string, listener: (event: CustomEvent) => void): void {
    this.addEventListener(type, listener);
  }

  off(type: string, listener: (event: CustomEvent) => void): void {
    this.removeEventListener(type, listener);
  }
}

export interface Markup {
  readonly __owlMarkup: true;
  readonly value: string;
  toString(): string;
}

class MarkupString implements Markup {
  readonly __owlMarkup = true;
  readonly value: string;

  constructor(value = "") {
    this.value = String(value);
  }

  toString(): string {
    return this.value;
  }

  valueOf(): string {
    return this.value;
  }
}

export function markup(value?: string | TemplateStringsArray | Markup, ...values: unknown[]): Markup {
  if (Array.isArray(value) && "raw" in value) {
    let out = "";
    for (let i = 0; i < value.length; i += 1) {
      out += value[i];
      if (i < values.length) out += htmlEscape(values[i]);
    }
    return new MarkupString(out);
  }
  if (isMarkup(value)) return value;
  return new MarkupString(value == null ? "" : String(value));
}

function isMarkup(value: unknown): value is Markup {
  return Boolean(value && typeof value === "object" && (value as Markup).__owlMarkup === true && typeof (value as Markup).toString === "function");
}

function addLifecycleHook(name: string, callback: LifecycleCallback): void {
  if (!currentComponent) throw new Error(`${name} hook must be registered during setup`);
  currentComponent.addHook(name, callback);
}

function withCurrent<T>(component: Component, callback: () => T): T {
  const previous = currentComponent;
  currentComponent = component;
  try {
    return callback();
  } finally {
    currentComponent = previous;
  }
}

function append(target: HTMLElement, el: HTMLElement): void {
  if ("appendChild" in target && typeof target.appendChild === "function") {
    target.appendChild(el);
    return;
  }
  target.innerHTML = el.innerHTML;
}

function clearTarget(target: HTMLElement, el: HTMLElement): void {
  if (el.parentNode === target) {
    target.removeChild(el);
  }
}

function replaceMountedElement(previous: HTMLElement | null, next: HTMLElement): void {
  if (!previous || previous === next || !previous.parentNode) return;
  const parent = previous.parentNode as unknown as {
    replaceChild?: (newChild: HTMLElement, oldChild: HTMLElement) => unknown;
    removeChild?: (child: HTMLElement) => unknown;
    appendChild?: (child: HTMLElement) => unknown;
  };
  if (typeof parent.replaceChild === "function") {
    parent.replaceChild(next, previous);
    return;
  }
  if (typeof parent.removeChild === "function") parent.removeChild(previous);
  if (typeof parent.appendChild === "function") parent.appendChild(next);
}

function scheduleNoop(): void {}

function dynamicImport(specifier: string): Promise<unknown> {
  const importer = new Function("specifier", "return import(specifier)") as (specifier: string) => Promise<unknown>;
  return importer(specifier);
}

function moduleRuntime(mod: unknown): Record<string, unknown> {
  if (isRecord(mod) && isRecord(mod.default)) return mod.default;
  return isRecord(mod) ? mod : {};
}

function moduleVersion(runtime: Record<string, unknown>): string | undefined {
  const info = runtime.__info__;
  return isRecord(info) && typeof info.version === "string" ? info.version : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function protectServiceMethod(component: Component, fn: (...args: unknown[]) => unknown): (...args: unknown[]) => unknown {
  return function protectedMethod(this: unknown, ...args: unknown[]) {
    if (status(component) === "destroyed") {
      return useServiceProtectMethodHandling.fn();
    }
    const promise = Promise.resolve(fn.apply(this, args));
    const protectedPromise = promise.then((result) => (
      status(component) === "destroyed" ? new Promise(() => {}) : result
    ));
    const withControls = protectedPromise as Promise<unknown> & { abort?: unknown; cancel?: unknown };
    const original = promise as Promise<unknown> & { abort?: unknown; cancel?: unknown };
    withControls.abort = original.abort;
    withControls.cancel = original.cancel;
    return withControls;
  };
}

function makeCustomEvent(type: string, detail: unknown): CustomEvent {
  if (typeof CustomEvent !== "undefined") return new CustomEvent(type, { detail });
  return { type, detail } as CustomEvent;
}

function escapeText(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#x27;")
    .replaceAll("`", "&#x60;");
}

function sameDeps(left: unknown[], right: unknown[]): boolean {
  if (left.length !== right.length) return false;
  return left.every((value, index) => Object.is(value, right[index]));
}
