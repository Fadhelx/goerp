export interface HomeMenuEntry {
  id?: number | string;
  name?: string;
  children?: readonly (number | string)[];
  actionID?: number | string | false;
  actionId?: number | string | false;
  directActionID?: number | string | false;
  hasDirectAction?: boolean;
  xmlid?: string | false;
  actionPath?: string | false;
  action_path?: string | false;
  webIcon?: string | false;
  webIconData?: string | false;
  webIconDataMimetype?: string | false;
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
  const direct = payload[String(id)];
  const nested = isRecord(payload.children) ? payload.children[String(id)] : undefined;
  const value = richerMenuEntry(direct, nested);
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

export function homeMenuAppsCatalogApp(payload: HomeMenuPayload | null | undefined): HomeMenuApp | null {
  const candidates = normalizeHomeMenuApps(payload, { includeDescendantActions: true })
    .filter(isAppsCatalogApp)
    .sort((left, right) => appsCatalogScore(right) - appsCatalogScore(left) || left.sequence - right.sequence);
  return candidates[0] ?? null;
}

export function isAppsCatalogApp(app: HomeMenuApp): boolean {
  return menuActionValue(app.menu) !== undefined && isAppsCatalogMenu(app.menu);
}

function homeMenuSearchText(payload: HomeMenuPayload, menu: HomeMenuEntry): string {
  const parts = [cleanAppName(menu.name), ...menuSearchAliases(menu)];
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
  const visit = (id: number | string, path: string[], searchPath: string[], visited: Set<string>) => {
    const key = String(id);
    if (visited.has(key)) return;
    const nextVisited = new Set(visited);
    nextVisited.add(key);
    const menu = homeMenuEntry(payload, id);
    if (!menu) return;
    const name = cleanAppName(menu.name);
    const nextPath = [...path, name];
    const nextSearchPath = [...searchPath, name, ...menuSearchAliases(menu)];
    const action = menuActionValue(menu);
    if (action !== undefined) {
      out.push({
        id: menu.id ?? id,
        key: `${root.key}:${String(menu.id ?? id)}`,
        name,
        initials: appInitials(name),
        iconToken: root.iconToken,
        sequence: root.sequence + ((out.length + 1) / 1000),
        searchText: nextSearchPath.join(" ").toLowerCase(),
        menu,
        parentPath: path.join(" / "),
        rootId: root.id,
        isMenuAction: true
      });
    }
    for (const childId of menu.children ?? []) visit(childId, nextPath, nextSearchPath, nextVisited);
  };
  const rootSearchPath = [rootName, ...menuSearchAliases(root.menu)];
  for (const childId of root.menu.children ?? []) visit(childId, [rootName], rootSearchPath, new Set([String(root.id)]));
  return out;
}

function collectMenuNames(payload: HomeMenuPayload, ids: readonly (number | string)[], visited: Set<string>, out: string[]): void {
  for (const id of ids) {
    const key = String(id);
    if (visited.has(key)) continue;
    visited.add(key);
    const menu = homeMenuEntry(payload, id);
    if (!menu) continue;
    if (menu.name) out.push(cleanAppName(menu.name), ...menuSearchAliases(menu));
    collectMenuNames(payload, menu.children ?? [], visited, out);
  }
}

export function menuActionValue(menu: HomeMenuEntry): number | string | undefined {
  const value = menu.actionID ?? menu.actionId;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

export function menuDirectActionValue(menu: HomeMenuEntry): number | string | undefined {
  const value = menu.directActionID ?? (menu.hasDirectAction === true ? menu.actionID ?? menu.actionId : undefined);
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

function isAppsCatalogMenu(menu: HomeMenuEntry): boolean {
  const name = cleanAppName(menu.name).toLowerCase();
  const xmlid = textValue(menu.xmlid).toLowerCase();
  const actionPath = textValue(menu.actionPath ?? menu.action_path).toLowerCase();
  return xmlid === "base.menu_ir_module_module" || actionPath === "apps" || name === "apps";
}

function appsCatalogScore(app: HomeMenuApp): number {
  const menu = app.menu;
  const xmlid = textValue(menu.xmlid).toLowerCase();
  const actionPath = textValue(menu.actionPath ?? menu.action_path).toLowerCase();
  if (xmlid === "base.menu_ir_module_module") return 3;
  if (actionPath === "apps") return 2;
  return 1;
}

function menuSearchAliases(menu: HomeMenuEntry): string[] {
  const name = cleanAppName(menu.name).toLowerCase();
  if (name === "apps") return ["applications", "modules", "install"];
  if (name === "settings") return ["configuration", "preferences"];
  if (name === "technical") return ["developer", "debug", "system"];
  return [];
}

function textValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function richerMenuEntry(left: unknown, right: unknown): unknown {
  if (!isRecord(left)) return right;
  if (!isRecord(right)) return left;
  const leftChildren = Array.isArray(left.children) ? left.children.length : 0;
  const rightChildren = Array.isArray(right.children) ? right.children.length : 0;
  if (rightChildren > leftChildren) return right;
  return left;
}
