export interface ActionBreadcrumb {
  id: string;
  label: string;
  actionId?: number | string;
  resModel?: string;
  resId?: number | string;
}

export interface ActionStackRouteState {
  action?: number | string;
  model?: string;
  view_type?: string;
  id?: number | string;
  menu_id?: number | string;
}

export interface ActionStackEntry {
  id: string;
  action: Record<string, unknown>;
  options: Record<string, unknown>;
  title: string;
  actionId?: number | string;
  resModel?: string;
  resId?: number | string;
  target: string;
  viewTypes: readonly string[];
  parentId?: string;
  dialog: boolean;
  breadcrumbVisible: boolean;
  route: ActionStackRouteState | null;
}

export interface ActionStackSnapshot {
  entries: readonly ActionStackEntry[];
  current: ActionStackEntry | null;
  currentRoute: ActionStackRouteState | null;
  breadcrumbs: readonly ActionBreadcrumb[];
}

export interface ActionStackOptions {
  clearBreadcrumbs?: unknown;
  replaceLastAction?: unknown;
  stackPosition?: "replace" | "clear" | "push";
  [key: string]: unknown;
}

export interface ActionStackController {
  readonly entries: readonly ActionStackEntry[];
  readonly current: ActionStackEntry | null;
  readonly currentRoute: ActionStackRouteState | null;
  readonly breadcrumbs: readonly ActionBreadcrumb[];
  snapshot(): ActionStackSnapshot;
  push(action: Record<string, unknown>, options?: ActionStackOptions): ActionStackEntry;
  replace(action: Record<string, unknown>, options?: ActionStackOptions): ActionStackEntry;
  clear(): void;
  closeCurrent(): ActionStackEntry | null;
  closeTo(id: string): ActionStackEntry | null;
  restore(entries: readonly ActionStackEntry[]): ActionStackEntry | null;
}

let nextStackID = 0;

export function createActionStack(): ActionStackController {
  let entries: ActionStackEntry[] = [];
  return {
    get entries(): readonly ActionStackEntry[] {
      return entries.map(cloneEntry);
    },
    get current(): ActionStackEntry | null {
      return entries.length ? cloneEntry(entries[entries.length - 1]) : null;
    },
    get currentRoute(): ActionStackRouteState | null {
      return cloneRoute(currentRouteEntry(entries)?.route ?? null);
    },
    get breadcrumbs(): readonly ActionBreadcrumb[] {
      return actionBreadcrumbs(entries);
    },
    snapshot(): ActionStackSnapshot {
      return {
        entries: entries.map(cloneEntry),
        current: entries.length ? cloneEntry(entries[entries.length - 1]) : null,
        currentRoute: cloneRoute(currentRouteEntry(entries)?.route ?? null),
        breadcrumbs: actionBreadcrumbs(entries)
      };
    },
    push(action: Record<string, unknown>, options: ActionStackOptions = {}): ActionStackEntry {
      if (shouldClearStack(action, options)) entries = [];
      const entry = makeActionStackEntry(action, options, undefined, parentIdForPush(action, entries));
      entries = [...entries, entry];
      return cloneEntry(entry);
    },
    replace(action: Record<string, unknown>, options: ActionStackOptions = {}): ActionStackEntry {
      if (shouldClearStack(action, options)) entries = [];
      const previousEntries = entries.slice(0, -1);
      const previous = entries[entries.length - 1];
      if (isDialogAction(action) && previous && !previous.dialog) {
        const entry = makeActionStackEntry(action, options, undefined, previous.id);
        entries = [...entries, entry];
        return cloneEntry(entry);
      }
      const parentId = parentIdForReplace(action, previous, previousEntries);
      const entry = makeActionStackEntry(action, options, undefined, parentId);
      entries = entries.length ? [...previousEntries, entry] : [entry];
      return cloneEntry(entry);
    },
    clear(): void {
      entries = [];
    },
    closeCurrent(): ActionStackEntry | null {
      entries = entries.slice(0, -1);
      return entries.length ? cloneEntry(entries[entries.length - 1]) : null;
    },
    closeTo(id: string): ActionStackEntry | null {
      const index = entries.findIndex((entry) => entry.id === id);
      if (index === -1) return entries.length ? cloneEntry(entries[entries.length - 1]) : null;
      entries = entries.slice(0, index + 1);
      return cloneEntry(entries[index]);
    },
    restore(nextEntries: readonly ActionStackEntry[]): ActionStackEntry | null {
      entries = [];
      for (const entry of nextEntries) {
        entries.push(makeActionStackEntry(
          entry.action,
          entry.options,
          entry.id,
          entry.parentId ?? currentRouteEntry(entries)?.id
        ));
      }
      return entries.length ? cloneEntry(entries[entries.length - 1]) : null;
    }
  };
}

export function shouldReplaceLastAction(options: ActionStackOptions = {}): boolean {
  return options.replaceLastAction === true || options.stackPosition === "replace";
}

export function isCloseAction(action: Record<string, unknown>): boolean {
  return action.type === "ir.actions.act_window_close";
}

export function makeActionStackEntry(
  action: Record<string, unknown>,
  options: Record<string, unknown> = {},
  id = `action-${++nextStackID}`,
  parentId?: string
): ActionStackEntry {
  const resModel = text(action.res_model);
  const resID = actionID(action.res_id);
  const target = text(action.target) || "current";
  const route = target === "new" ? null : actionRouteState(action);
  const entry: ActionStackEntry = {
    id,
    action: { ...action },
    options: { ...options },
    title: actionTitle(action),
    target,
    viewTypes: actionViewTypes(action),
    dialog: target === "new",
    breadcrumbVisible: route !== null && !actionNoBreadcrumbs(action),
    route
  };
  if (parentId) entry.parentId = parentId;
  const idValue = actionID(action.id);
  if (idValue !== undefined) entry.actionId = idValue;
  if (resModel) entry.resModel = resModel;
  if (resID !== undefined) entry.resId = resID;
  return entry;
}

export function actionBreadcrumbs(entries: readonly ActionStackEntry[]): ActionBreadcrumb[] {
  return entries
    .filter((entry) => entry.breadcrumbVisible)
    .map((entry) => {
      const breadcrumb: ActionBreadcrumb = {
        id: entry.id,
        label: entry.title
      };
      if (entry.actionId !== undefined) breadcrumb.actionId = entry.actionId;
      if (entry.resModel) breadcrumb.resModel = entry.resModel;
      if (entry.resId !== undefined) breadcrumb.resId = entry.resId;
      return breadcrumb;
    });
}

export function actionTitle(action: Record<string, unknown>): string {
  return text(action.name)
    || text(action.display_name)
    || text(action.title)
    || text(action.tag)
    || text(action.res_model)
    || text(action.type)
    || "Action";
}

export function actionViewTypes(action: Record<string, unknown>): string[] {
  const fromViews: string[] = [];
  if (Array.isArray(action.views)) {
    for (const view of action.views) {
      if (!Array.isArray(view)) continue;
      const type = text(view[1]);
      if (type && type !== "search" && !fromViews.includes(type)) fromViews.push(type);
    }
  }
  if (fromViews.length) return fromViews;
  if (typeof action.view_mode === "string") {
    return action.view_mode
      .split(",")
      .map((view) => view.trim())
      .filter((view, index, views) => Boolean(view) && view !== "search" && views.indexOf(view) === index);
  }
  return [];
}

function cloneEntry(entry: ActionStackEntry): ActionStackEntry {
  const clone: ActionStackEntry = {
    id: entry.id,
    action: { ...entry.action },
    options: { ...entry.options },
    title: entry.title,
    target: entry.target,
    viewTypes: [...entry.viewTypes],
    dialog: entry.dialog,
    breadcrumbVisible: entry.breadcrumbVisible,
    route: cloneRoute(entry.route)
  };
  if (entry.parentId !== undefined) clone.parentId = entry.parentId;
  if (entry.actionId !== undefined) clone.actionId = entry.actionId;
  if (entry.resModel !== undefined) clone.resModel = entry.resModel;
  if (entry.resId !== undefined) clone.resId = entry.resId;
  return clone;
}

function actionRouteState(action: Record<string, unknown>): ActionStackRouteState {
  const state: ActionStackRouteState = {};
  const actionValue = actionID(action.id);
  if (actionValue !== undefined) state.action = actionValue;
  const model = text(action.res_model);
  if (model) state.model = model;
  const viewType = text(action.view_type) || actionViewTypes(action)[0] || "";
  if (viewType) state.view_type = viewType;
  const idValue = actionID(action.res_id);
  if (idValue !== undefined) state.id = idValue;
  const menuID = actionID(action.menu_id);
  if (menuID !== undefined) state.menu_id = menuID;
  return state;
}

function actionNoBreadcrumbs(action: Record<string, unknown>): boolean {
  if (action._noBreadcrumbs === true || action.no_breadcrumbs === true) return true;
  const context = action.context;
  return isRecord(context) && context.no_breadcrumbs === true;
}

function actionID(value: unknown): number | string | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

function cloneRoute(route: ActionStackRouteState | null): ActionStackRouteState | null {
  return route ? { ...route } : null;
}

function currentRouteEntry(entries: readonly ActionStackEntry[]): ActionStackEntry | null {
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const entry = entries[index];
    if (entry.route) return entry;
  }
  return null;
}

function parentIdForPush(action: Record<string, unknown>, entries: readonly ActionStackEntry[]): string | undefined {
  if (isDialogAction(action)) return entries[entries.length - 1]?.id;
  return currentRouteEntry(entries)?.id;
}

function parentIdForReplace(
  action: Record<string, unknown>,
  previous: ActionStackEntry | undefined,
  previousEntries: readonly ActionStackEntry[]
): string | undefined {
  if (isDialogAction(action)) return previous?.dialog ? previous.parentId : previousEntries[previousEntries.length - 1]?.id;
  return previous?.dialog ? previous.parentId : currentRouteEntry(previousEntries)?.id;
}

function isDialogAction(action: Record<string, unknown>): boolean {
  return text(action.target) === "new";
}

function shouldClearStack(action: Record<string, unknown>, options: ActionStackOptions): boolean {
  return options.clearBreadcrumbs === true || options.stackPosition === "clear" || text(action.target) === "main";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}
