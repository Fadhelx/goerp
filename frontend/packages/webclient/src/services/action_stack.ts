export interface ActionBreadcrumb {
  id: string;
  label: string;
  actionId?: number | string;
  resModel?: string;
  resId?: number | string;
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
}

export interface ActionStackSnapshot {
  entries: readonly ActionStackEntry[];
  current: ActionStackEntry | null;
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
    get breadcrumbs(): readonly ActionBreadcrumb[] {
      return actionBreadcrumbs(entries);
    },
    snapshot(): ActionStackSnapshot {
      return {
        entries: entries.map(cloneEntry),
        current: entries.length ? cloneEntry(entries[entries.length - 1]) : null,
        breadcrumbs: actionBreadcrumbs(entries)
      };
    },
    push(action: Record<string, unknown>, options: ActionStackOptions = {}): ActionStackEntry {
      if (options.clearBreadcrumbs || options.stackPosition === "clear") entries = [];
      const entry = makeActionStackEntry(action, options);
      entries = [...entries, entry];
      return cloneEntry(entry);
    },
    replace(action: Record<string, unknown>, options: ActionStackOptions = {}): ActionStackEntry {
      const entry = makeActionStackEntry(action, options);
      entries = entries.length ? [...entries.slice(0, -1), entry] : [entry];
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
      entries = nextEntries.map((entry) => makeActionStackEntry(entry.action, entry.options, entry.id));
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
  id = `action-${++nextStackID}`
): ActionStackEntry {
  const resModel = text(action.res_model);
  const resID = actionID(action.res_id);
  const entry: ActionStackEntry = {
    id,
    action: { ...action },
    options: { ...options },
    title: actionTitle(action),
    target: text(action.target) || "current",
    viewTypes: actionViewTypes(action)
  };
  const idValue = actionID(action.id);
  if (idValue !== undefined) entry.actionId = idValue;
  if (resModel) entry.resModel = resModel;
  if (resID !== undefined) entry.resId = resID;
  return entry;
}

export function actionBreadcrumbs(entries: readonly ActionStackEntry[]): ActionBreadcrumb[] {
  return entries
    .filter((entry) => entry.target !== "new")
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
    viewTypes: [...entry.viewTypes]
  };
  if (entry.actionId !== undefined) clone.actionId = entry.actionId;
  if (entry.resModel !== undefined) clone.resModel = entry.resModel;
  if (entry.resId !== undefined) clone.resId = entry.resId;
  return clone;
}

function actionID(value: unknown): number | string | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}
