import {
  appKey,
  homeMenuAppsCatalogApp,
  isAppsCatalogApp,
  normalizeHomeMenuApps,
  type HomeMenuApp,
  type HomeMenuPayload
} from "./app_metadata.js";

export interface HomeMenuRenderOptions {
  query?: string;
  includeAppsCatalog?: boolean;
  onOpenApp?: (app: HomeMenuApp) => void;
  onOpenAppsCatalog?: (app?: HomeMenuApp) => void;
}

export function renderHomeMenu(payload: HomeMenuPayload, options: HomeMenuRenderOptions = {}): HTMLElement {
  const section = document.createElement("section");
  section.className = "o-app-launcher-view o_app_launcher";
  section.dataset.view = "apps";
  section.dataset.mobileSafe = "true";

  const shell = document.createElement("div");
  shell.className = "o-app-shell o_home_menu";

  const searchWrap = document.createElement("div");
  searchWrap.className = "o-app-search o_home_menu_search";
  const search = document.createElement("input");
  search.type = "search";
  search.className = "o_app_search_input o_searchview_input";
  search.setAttribute("placeholder", "Search...");
  search.setAttribute("aria-label", "Search apps and menus");
  search.value = options.query ?? "";
  searchWrap.append(search);

  const grid = document.createElement("div");
  grid.className = "app-grid o_apps";

  const renderGrid = () => {
    const query = search.value.trim().toLowerCase();
    const apps = normalizeHomeMenuApps(payload, { includeDescendantActions: Boolean(query) });
    const visible = query ? apps.filter((app) => app.searchText.includes(query)) : apps;
    const catalogApp = homeMenuAppsCatalogApp(payload);
    grid.replaceChildren();
    for (const app of visible) {
      grid.append(renderHomeMenuApp(app, () => options.onOpenApp?.(app)));
    }

    if (options.includeAppsCatalog !== false && shouldAppendAppsCatalog(query, visible, catalogApp)) {
      const app: HomeMenuApp = catalogApp ?? {
        id: "apps",
        key: "apps",
        name: "Apps",
        initials: "A",
        iconToken: "teal",
        sequence: apps.length,
        searchText: APPS_CATALOG_SEARCH_TEXT,
        menu: { id: "apps", name: "Apps" }
      };
      grid.append(renderHomeMenuApp(app, () => {
        if (catalogApp && options.onOpenApp) {
          options.onOpenApp?.(catalogApp);
        } else {
          options.onOpenAppsCatalog?.(catalogApp ?? undefined);
        }
      }));
    }

    if (!grid.children.length) {
      const empty = document.createElement("p");
      empty.className = "muted";
      empty.textContent = query ? "No apps found." : "No menus loaded.";
      grid.append(empty);
    }
  };
  search.addEventListener("input", renderGrid);
  renderGrid();

  shell.append(searchWrap, grid);
  section.append(shell);
  return section;
}

export function renderHomeMenuApp(app: HomeMenuApp, onClick?: () => void): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "app-card o_app has-icon";
  button.dataset.appName = app.name;
  button.dataset.appKey = app.key || appKey(app.name);
  button.dataset.menuId = String(app.id);
  if (app.rootId !== undefined) button.dataset.rootMenuId = String(app.rootId);
  if (app.parentPath) button.dataset.menuPath = app.parentPath;
  if (app.isMenuAction) button.dataset.menuAction = "true";
  button.title = app.name;
  button.setAttribute("aria-label", app.name);

  const icon = document.createElement("span");
  icon.className = "app-icon o_app_icon";
  icon.dataset.iconToken = app.iconToken;
  icon.textContent = app.initials;

  const name = document.createElement("strong");
  name.className = "o_app_name";
  name.textContent = app.name;
  const iconImage = appIconImage(app);
  button.append(iconImage ?? icon, name);

  if (app.parentPath) {
    const path = document.createElement("span");
    path.className = "o_app_menu_path";
    path.textContent = app.parentPath;
    button.append(path);
  }
  if (onClick) button.addEventListener("click", onClick);
  return button;
}

const APPS_CATALOG_SEARCH_TEXT = "apps applications modules install";

function shouldAppendAppsCatalog(query: string, visible: readonly HomeMenuApp[], catalogApp: HomeMenuApp | null): boolean {
  if (!catalogApp) return false;
  if (visible.some((app) => app.key === "apps" || isAppsCatalogApp(app))) return false;
  if (!query) return true;
  return APPS_CATALOG_SEARCH_TEXT.includes(query) || catalogApp.searchText.includes(query);
}

function appIconImage(app: HomeMenuApp): HTMLImageElement | null {
  const source = typeof app.menu.webIconData === "string" ? app.menu.webIconData.trim() : "";
  const iconSource = appIconSource(source, app.menu.webIconDataMimetype);
  if (!iconSource) return null;
  const image = document.createElement("img");
  image.className = "app-icon o_app_icon";
  image.alt = "";
  image.src = iconSource;
  image.setAttribute("aria-hidden", "true");
  if (typeof app.menu.webIconDataMimetype === "string" && app.menu.webIconDataMimetype.trim()) {
    image.dataset.mimetype = app.menu.webIconDataMimetype.trim();
  }
  return image;
}

function appIconSource(source: string, mimetype: unknown): string {
  if (!source || source.endsWith("/default_icon_app.png")) return "";
  if (/^(data:|https?:\/\/|\/)/.test(source)) return source;
  const mediaType = typeof mimetype === "string" && mimetype.trim() ? mimetype.trim() : "image/png";
  if (source.length >= 32 && /^[A-Za-z0-9+/=]+$/.test(source)) {
    return `data:${mediaType};base64,${source}`;
  }
  return "";
}
