export type RouteScalar = string | number | boolean;
export type RouteValue =
  | RouteScalar
  | readonly RouteScalar[]
  | readonly WebClientRouteActionState[]
  | null
  | undefined;

export interface WebClientRouteActionState {
  action?: number | string;
  model?: string;
  view_type?: string;
  id?: number | string;
  menu_id?: number | string;
  active_id?: number | string;
  active_ids?: readonly (number | string)[];
  displayName?: string;
  target?: string;
  [key: string]: RouteScalar | readonly RouteScalar[] | null | undefined;
}

export interface WebClientRouteState {
  action?: number | string;
  model?: string;
  view_type?: string;
  id?: number | string;
  menu_id?: number | string;
  active_id?: number | string;
  active_ids?: readonly (number | string)[];
  debug?: boolean | string;
  cids?: string;
  actionStack?: readonly WebClientRouteActionState[];
  [key: string]: RouteValue;
}

export interface ActionRouteSource {
  action: Record<string, unknown>;
  title?: string;
  target?: string;
  dialog?: boolean;
  breadcrumbVisible?: boolean;
  route?: Record<string, unknown> | null;
}

export interface BrowserRouteTarget {
  location: {
    pathname: string;
    search: string;
    hash: string;
  };
  history?: {
    pushState(data: unknown, unused: string, url?: string | URL | null): void;
    replaceState(data: unknown, unused: string, url?: string | URL | null): void;
  };
}

const ROUTE_ORDER = [
  "action",
  "model",
  "view_type",
  "id",
  "menu_id",
  "active_id",
  "active_ids",
  "cids",
  "debug"
];

const NUMERIC_KEYS = new Set(["action", "id", "menu_id", "active_id"]);
const ACTION_STACK_ROUTE_KEYS = [
  "action",
  "model",
  "view_type",
  "id",
  "menu_id",
  "active_id",
  "active_ids"
];

export function routeStateFromAction(
  action: Record<string, unknown>,
  extra: WebClientRouteState = {}
): WebClientRouteState {
  const state: WebClientRouteState = { ...extra };
  const actionID = routeID(action.id);
  if (actionID !== undefined) state.action = actionID;
  const model = text(action.res_model);
  if (model) state.model = model;
  const viewType = text(action.view_type) || firstActionViewType(action);
  if (viewType) state.view_type = viewType;
  const resID = routeID(action.res_id);
  if (resID !== undefined) state.id = resID;
  const menuID = routeID(action.menu_id);
  if (menuID !== undefined) state.menu_id = menuID;
  return normalizeRouteState(state);
}

export function routeStateFromStack(
  entries: readonly ActionRouteSource[],
  extra: WebClientRouteState = {}
): WebClientRouteState {
  const { actionStack: _staleActionStack, ...extraState } = extra;
  const actionStack = entries
    .filter((entry) => !isDialogEntry(entry))
    .map(routeActionStateFromEntry)
    .filter((entry) => Object.keys(entry).length > 0);
  const stack = mergeCurrentRouteState(actionStack, extra);
  const current = stack[stack.length - 1];
  return normalizeRouteState({
    ...extraState,
    ...(stack.length ? { actionStack: stack } : {}),
    ...(current ? currentRouteState(current) : {})
  });
}

export function normalizeRouteState(input: Record<string, unknown> | WebClientRouteState): WebClientRouteState {
  const state: WebClientRouteState = {};
  for (const [key, value] of Object.entries(input)) {
    const normalized = normalizeRouteValue(key, value);
    if (normalized !== undefined && normalized !== null && normalized !== "") {
      state[key] = normalized;
    }
  }
  return state;
}

export function serializeRouteState(input: WebClientRouteState): string {
  const state = normalizeRouteState(input);
  const keys = Object.keys(state).sort((left, right) => routeKeyOrder(left) - routeKeyOrder(right) || left.localeCompare(right));
  const pairs: string[] = [];
  for (const key of keys) {
    const value = state[key];
    if (Array.isArray(value)) {
      if (!value.every(isRouteScalar)) continue;
      if (!value.length) continue;
      pairs.push(`${encodeURIComponent(key)}=${encodeURIComponent(value.map(String).join(","))}`);
    } else if (isRouteScalar(value)) {
      pairs.push(`${encodeURIComponent(key)}=${encodeURIComponent(formatRouteScalar(value))}`);
    }
  }
  return pairs.length ? `#${pairs.join("&")}` : "";
}

export function parseRouteState(input: string): WebClientRouteState {
  const source = input.startsWith("#") || input.startsWith("?") ? input.slice(1) : input;
  const state: WebClientRouteState = {};
  if (!source) return state;
  for (const pair of source.split("&")) {
    if (!pair) continue;
    const [rawKey, rawValue = ""] = pair.split("=");
    const key = decodeURIComponent(rawKey);
    const decoded = decodeURIComponent(rawValue.replace(/\+/g, " "));
    state[key] = parseRouteValue(key, decoded);
  }
  return normalizeRouteState(state);
}

export function routeStateToURL(pathname: string, state: WebClientRouteState): string {
  return `${pathname}${serializeRouteState(state)}`;
}

export function updateBrowserRoute(
  state: WebClientRouteState,
  options: { replace?: boolean; target?: BrowserRouteTarget } = {}
): string {
  const normalized = normalizeRouteState(state);
  const target = options.target ?? (typeof window !== "undefined" ? window : undefined);
  const pathname = target?.location.pathname || "/web";
  const search = target?.location.search || "";
  const url = `${pathname}${search}${serializeRouteState(normalized)}`;
  if (target?.history) {
    if (options.replace) target.history.replaceState({ ...normalized }, "", url);
    else target.history.pushState({ ...normalized }, "", url);
  }
  return url;
}

function routeKeyOrder(key: string): number {
  const index = ROUTE_ORDER.indexOf(key);
  return index === -1 ? ROUTE_ORDER.length : index;
}

function normalizeRouteValue(key: string, value: unknown): RouteValue {
  if (value === undefined || value === null || value === false || value === "") return undefined;
  if (key === "actionStack" && Array.isArray(value)) {
    const stack = value
      .filter(isRecord)
      .map(normalizeRouteActionState)
      .filter((entry) => Object.keys(entry).length > 0);
    return stack.length ? stack : undefined;
  }
  if (Array.isArray(value)) {
    const values = value
      .map((item) => normalizeRouteValue(key, item))
      .filter((item): item is RouteScalar => isRouteScalar(item));
    return values.length ? values : undefined;
  }
  if (typeof value === "number") return Number.isFinite(value) ? value : undefined;
  if (typeof value === "boolean") return value;
  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) return undefined;
    if (NUMERIC_KEYS.has(key) && /^-?\d+$/.test(trimmed)) return Number(trimmed);
    if (key === "active_ids") {
      const parts = trimmed.split(",").map((part) => part.trim()).filter(Boolean);
      return parts.map((part) => (/^-?\d+$/.test(part) ? Number(part) : part));
    }
    if (key === "debug" && (trimmed === "1" || trimmed === "true")) return true;
    return trimmed;
  }
  return undefined;
}

function normalizeRouteActionState(input: Record<string, unknown>): WebClientRouteActionState {
  const state: WebClientRouteActionState = {};
  for (const [key, value] of Object.entries(input)) {
    const normalized = normalizeRouteValue(key, value);
    if (normalized === undefined || normalized === null || normalized === "") continue;
    if (Array.isArray(normalized) && !normalized.every(isRouteScalar)) continue;
    if (isRouteScalar(normalized) || Array.isArray(normalized)) state[key] = normalized;
  }
  return state;
}

function routeActionStateFromEntry(entry: ActionRouteSource): WebClientRouteActionState {
  const fromRoute = isRecord(entry.route) ? normalizeRouteActionState(entry.route) : routeStateFromAction(entry.action);
  const state = normalizeRouteActionState(fromRoute);
  const title = text(entry.title);
  if (title) state.displayName = title;
  const target = text(entry.target ?? entry.action.target);
  if (target && target !== "current") state.target = target;
  return normalizeRouteActionState(state);
}

function currentRouteState(state: WebClientRouteActionState): WebClientRouteState {
  const { displayName: _displayName, target: _target, ...route } = state;
  return route;
}

function mergeCurrentRouteState(
  actionStack: readonly WebClientRouteActionState[],
  route: WebClientRouteState
): WebClientRouteActionState[] {
  if (!actionStack.length) return [...actionStack];
  const currentRoute = currentActionStackRouteState(route);
  if (!Object.keys(currentRoute).length) return [...actionStack];
  const current = normalizeRouteActionState({
    ...actionStack[actionStack.length - 1],
    ...currentRoute
  });
  return [...actionStack.slice(0, -1), current];
}

function currentActionStackRouteState(route: WebClientRouteState): Record<string, unknown> {
  const state: Record<string, unknown> = {};
  for (const key of ACTION_STACK_ROUTE_KEYS) {
    const value = route[key];
    if (value !== undefined) state[key] = value;
  }
  return state;
}

function isDialogEntry(entry: ActionRouteSource): boolean {
  return entry.dialog === true || text(entry.target ?? entry.action.target) === "new";
}

function parseRouteValue(key: string, value: string): RouteValue {
  if (key === "active_ids") {
    return value
      .split(",")
      .map((part) => part.trim())
      .filter(Boolean)
      .map((part) => (/^-?\d+$/.test(part) ? Number(part) : part));
  }
  if (key === "debug") {
    if (value === "1" || value === "true") return true;
    if (value === "0" || value === "false") return undefined;
  }
  if (NUMERIC_KEYS.has(key) && /^-?\d+$/.test(value)) return Number(value);
  return value;
}

function formatRouteScalar(value: RouteScalar): string {
  if (value === true) return "1";
  if (value === false) return "0";
  return String(value);
}

function isRouteScalar(value: RouteValue): value is RouteScalar {
  return typeof value === "string" || typeof value === "number" || typeof value === "boolean";
}

function routeID(value: unknown): number | string | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function firstActionViewType(action: Record<string, unknown>): string {
  if (Array.isArray(action.views)) {
    for (const view of action.views) {
      if (Array.isArray(view) && typeof view[1] === "string" && view[1] !== "search") return view[1];
    }
  }
  if (typeof action.view_mode === "string") {
    return action.view_mode.split(",").map((view) => view.trim()).find((view) => view && view !== "search") ?? "";
  }
  return "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}
