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
}

const ICON_TOKENS = ["teal", "purple", "blue", "terracotta", "green", "slate"] as const;

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

export function normalizeHomeMenuApps(payload: HomeMenuPayload | null | undefined): HomeMenuApp[] {
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
  return apps.sort((left, right) => left.sequence - right.sequence);
}

function homeMenuSearchText(payload: HomeMenuPayload, menu: HomeMenuEntry): string {
  const parts = [cleanAppName(menu.name)];
  for (const childId of menu.children ?? []) {
    const child = homeMenuEntry(payload, childId);
    if (child?.name) parts.push(cleanAppName(child.name));
  }
  return parts.join(" ").toLowerCase();
}

function isBetterMenuCandidate(existing: HomeMenuEntry, candidate: HomeMenuEntry): boolean {
  const existingHasAction = Boolean(existing.actionID ?? existing.actionId);
  const candidateHasAction = Boolean(candidate.actionID ?? candidate.actionId);
  const existingChildCount = existing.children?.length ?? 0;
  const candidateChildCount = candidate.children?.length ?? 0;
  return (!existingHasAction && candidateHasAction) || candidateChildCount > existingChildCount;
}
