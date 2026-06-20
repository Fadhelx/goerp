export type RouteScalar = string | number | boolean;
export type RouteValue = RouteScalar | readonly RouteScalar[] | null | undefined;

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
  [key: string]: RouteValue;
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
  const target = options.target ?? (typeof window !== "undefined" ? window : undefined);
  const pathname = target?.location.pathname || "/web";
  const search = target?.location.search || "";
  const url = `${pathname}${search}${serializeRouteState(state)}`;
  if (target?.history) {
    if (options.replace) target.history.replaceState({ ...state }, "", url);
    else target.history.pushState({ ...state }, "", url);
  }
  return url;
}

function routeKeyOrder(key: string): number {
  const index = ROUTE_ORDER.indexOf(key);
  return index === -1 ? ROUTE_ORDER.length : index;
}

function normalizeRouteValue(key: string, value: unknown): RouteValue {
  if (value === undefined || value === null || value === false || value === "") return undefined;
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
