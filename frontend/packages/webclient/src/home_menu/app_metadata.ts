export interface HomeMenuEntry {
  id?: number | string;
  name?: string;
  children?: readonly (number | string)[];
  actionID?: number | string | false;
  actionId?: number | string | false;
}

export type HomeMenuPayload = Record<string, unknown> & {
  root?: { children?: readonly (number | string)[] };
  menu_roots?: readonly (number | string)[];
};

export interface HomeMenuApp {
  id: number | string;
  key: string;
  name: string;
  initials: string;
  iconToken: string;
  sequence: number;
  searchText: string;
  menu: HomeMenuEntry;
  parentPath?: string;
  rootId?: number | string;
  isMenuAction?: boolean;
}

const ICON_TOKENS = ["teal", "purple", "blue", "terracotta", "green", "slate"] as const;

export interface NormalizeHomeMenuAppsOptions {
  includeDescendantActions?: boolean;
}

export function cleanAppName(value: unknown): string {
  const name = String(value ?? "App").replace(/\s+/g, " ").trim();
  return name || "App";
}

export function appKey(name: unknown): string {
  return cleanAppName(name).toLowerCase();
}

export function appInitials(name: unknown): string {
  const words = cleanAppName(name).split(" ").filter(Boolean);
  const picked = words.length > 1 ? words.slice(0, 2) : words;
  return picked.map((word) => word.slice(0, 1).toUpperCase()).join("") || "A";
}

export function appIconToken(name: unknown): string {
  let hash = 0;
  for (const char of cleanAppName(name)) {
    hash = (hash * 31 + char.charCodeAt(0)) >>> 0;
  }
  return ICON_TOKENS[hash % ICON_TOKENS.length];
}

export function homeMenuRootIds(payload: HomeMenuPayload | null | undefined): (number | string)[] {
  if (!payload) return [];
  if (Array.isArray(payload.menu_roots)) return [...payload.menu_roots];
  if (payload.root && Array.isArray(payload.root.children)) return [...payload.root.children];
  return [];
}

export function homeMenuEntry(payload: HomeMenuPayload, id: number | string): HomeMenuEntry | null {
  const value = payload[String(id)];
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  return value as HomeMenuEntry;
}

export function normalizeHomeMenuApps(
  payload: HomeMenuPayload | null | undefined,
  options: NormalizeHomeMenuAppsOptions = {}
): HomeMenuApp[] {
  if (!payload) return [];
  const apps: HomeMenuApp[] = [];
  const byKey = new Map<string, HomeMenuApp>();
  let sequence = 0;
  for (const rootId of homeMenuRootIds(payload)) {
    const menu = homeMenuEntry(payload, rootId);
    if (!menu) continue;
    const id = menu.id ?? rootId;
    const name = cleanAppName(menu.name);
    const key = appKey(name);
    const app: HomeMenuApp = {
      id,
      key,
      name,
      initials: appInitials(name),
      iconToken: appIconToken(name),
      sequence: sequence++,
      searchText: homeMenuSearchText(payload, menu),
      menu
    };
    const existing = byKey.get(key);
    if (!existing) {
      byKey.set(key, app);
      apps.push(app);
      continue;
    }
    if (isBetterMenuCandidate(existing.menu, menu)) {
      byKey.set(key, app);
      const index = apps.findIndex((item) => item.key === key);
      if (index >= 0) apps[index] = app;
    }
  }
  const roots = apps.sort((left, right) => left.sequence - right.sequence);
  if (!options.includeDescendantActions) return roots;
  return [
    ...roots,
    ...roots.flatMap((app) => descendantActionApps(payload, app))
  ].sort((left, right) => left.sequence - right.sequence || left.name.localeCompare(right.name));
}

function homeMenuSearchText(payload: HomeMenuPayload, menu: HomeMenuEntry): string {
  const parts = [cleanAppName(menu.name)];
  collectMenuNames(payload, menu.children ?? [], new Set(), parts);
  return parts.join(" ").toLowerCase();
}

function isBetterMenuCandidate(existing: HomeMenuEntry, candidate: HomeMenuEntry): boolean {
  const existingHasAction = menuActionValue(existing) !== undefined;
  const candidateHasAction = menuActionValue(candidate) !== undefined;
  const existingChildCount = existing.children?.length ?? 0;
  const candidateChildCount = candidate.children?.length ?? 0;
  return (!existingHasAction && candidateHasAction) || candidateChildCount > existingChildCount;
}

function descendantActionApps(payload: HomeMenuPayload, root: HomeMenuApp): HomeMenuApp[] {
  const out: HomeMenuApp[] = [];
  const rootName = cleanAppName(root.menu.name ?? root.name);
  const visit = (id: number | string, path: string[], visited: Set<string>) => {
    const key = String(id);
    if (visited.has(key)) return;
    const nextVisited = new Set(visited);
    nextVisited.add(key);
    const menu = homeMenuEntry(payload, id);
    if (!menu) return;
    const name = cleanAppName(menu.name);
    const nextPath = [...path, name];
    const action = menuActionValue(menu);
    if (action !== undefined) {
      out.push({
        id: menu.id ?? id,
        key: `${root.key}:${String(menu.id ?? id)}`,
        name,
        initials: appInitials(name),
        iconToken: root.iconToken,
        sequence: root.sequence + ((out.length + 1) / 1000),
        searchText: nextPath.join(" ").toLowerCase(),
        menu,
        parentPath: path.join(" / "),
        rootId: root.id,
        isMenuAction: true
      });
    }
    for (const childId of menu.children ?? []) visit(childId, nextPath, nextVisited);
  };
  for (const childId of root.menu.children ?? []) visit(childId, [rootName], new Set([String(root.id)]));
  return out;
}

function collectMenuNames(payload: HomeMenuPayload, ids: readonly (number | string)[], visited: Set<string>, out: string[]): void {
  for (const id of ids) {
    const key = String(id);
    if (visited.has(key)) continue;
    visited.add(key);
    const menu = homeMenuEntry(payload, id);
    if (!menu) continue;
    if (menu.name) out.push(cleanAppName(menu.name));
    collectMenuNames(payload, menu.children ?? [], visited, out);
  }
}

function menuActionValue(menu: HomeMenuEntry): number | string | undefined {
  const value = menu.actionID ?? menu.actionId;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}
