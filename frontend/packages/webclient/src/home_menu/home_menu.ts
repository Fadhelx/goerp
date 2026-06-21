import {
  appKey,
  normalizeHomeMenuApps,
  type HomeMenuApp,
  type HomeMenuPayload
} from "./app_metadata.js";

export interface HomeMenuRenderOptions {
  query?: string;
  includeAppsCatalog?: boolean;
  onOpenApp?: (app: HomeMenuApp) => void;
  onOpenAppsCatalog?: () => void;
}

export function renderHomeMenu(payload: HomeMenuPayload, options: HomeMenuRenderOptions = {}): HTMLElement {
  const section = document.createElement("section");
  section.className = "o-app-launcher-view o_app_launcher";
  section.dataset.view = "apps";
  section.dataset.mobileSafe = "true";

  const shell = document.createElement("div");
  shell.className = "o-app-shell o_home_menu";

  const search = document.createElement("input");
  search.type = "search";
  search.className = "o_app_search_input o_searchview_input";
  search.setAttribute("placeholder", "Search...");
  search.setAttribute("aria-label", "Search apps and menus");
  search.value = options.query ?? "";

  const grid = document.createElement("div");
  grid.className = "app-grid o_apps";

  const renderGrid = () => {
    const query = search.value.trim().toLowerCase();
    const apps = normalizeHomeMenuApps(payload, { includeDescendantActions: Boolean(query) });
    const visible = query ? apps.filter((app) => app.searchText.includes(query)) : apps;
    grid.replaceChildren();
    for (const app of visible) {
      grid.append(renderHomeMenuApp(app, () => options.onOpenApp?.(app)));
    }

    if (options.includeAppsCatalog !== false && (!query || "apps".includes(query)) && !apps.some((app) => app.key === "apps")) {
      grid.append(renderHomeMenuApp({
        id: "apps",
        key: "apps",
        name: "Apps",
        initials: "A",
        iconToken: "teal",
        sequence: apps.length,
        searchText: "apps",
        menu: { id: "apps", name: "Apps" }
      }, () => options.onOpenAppsCatalog?.()));
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

  shell.append(search, grid);
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
  button.append(icon, name);

  if (app.parentPath) {
    const path = document.createElement("span");
    path.className = "o_app_menu_path";
    path.textContent = app.parentPath;
    button.append(path);
  }
  if (onClick) button.addEventListener("click", onClick);
  return button;
}
