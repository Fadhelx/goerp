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
  section.className = "o-app-launcher-view o_app_launcher o_home_menu_background";
  section.dataset.view = "apps";
  section.dataset.mobileSafe = "true";
  section.setAttribute("tabindex", "-1");

  const shell = document.createElement("div");
  shell.className = "o-app-shell o_home_menu h-100 overflow-auto";

  const container = document.createElement("div");
  container.className = "container o_home_menu_container";

  const searchWrap = document.createElement("div");
  searchWrap.className = "o-app-search o_home_menu_search";
  const search = document.createElement("input");
  search.type = "text";
  search.className = "o_app_search_input o_search_hidden visually-hidden";
  search.setAttribute("data-allow-hotkeys", "true");
  search.setAttribute("aria-label", "Search apps and menus");
  search.setAttribute("role", "combobox");
  search.setAttribute("aria-autocomplete", "list");
  search.setAttribute("aria-haspopup", "listbox");
  search.value = options.query ?? "";
  searchWrap.append(search);

  const grid = document.createElement("div");
  grid.className = "o_apps row user-select-none mt-5 mx-0";
  grid.setAttribute("role", "listbox");

  const setSearchActive = (active: boolean) => {
    searchWrap.className = active ? "o-app-search o_home_menu_search is-active" : "o-app-search o_home_menu_search";
    search.className = active ? "o_app_search_input" : "o_app_search_input o_search_hidden visually-hidden";
    searchWrap.dataset.searchActive = active ? "true" : "false";
    search.setAttribute("aria-expanded", active && grid.children.length ? "true" : "false");
  };
  const renderGrid = () => {
    const query = search.value.trim().toLowerCase();
    setSearchActive(Boolean(query));
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
      const launcherApp = query ? app : { ...app, key: "apps", initials: "A", iconToken: "apps", parentPath: undefined };
      grid.append(renderHomeMenuApp(launcherApp, () => {
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
    search.setAttribute("aria-expanded", query && grid.querySelector?.(".o_app") ? "true" : "false");
  };
  const showSearch = () => setSearchActive(true);
  const hideSearchIfEmpty = () => {
    if (!search.value.trim()) setSearchActive(false);
  };
  search.addEventListener("focus", showSearch);
  search.addEventListener("blur", hideSearchIfEmpty);
  search.addEventListener("input", renderGrid);
  search.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    search.value = "";
    renderGrid();
    setSearchActive(false);
  });
  section.addEventListener("keydown", (event) => {
    if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return;
    if (isTextInput(event.target)) return;
    if (event.key === "/") {
      event.preventDefault?.();
      setSearchActive(true);
      search.focus?.();
      return;
    }
    if (event.key.length !== 1) return;
    event.preventDefault?.();
    search.value = event.key;
    setSearchActive(true);
    search.focus?.();
    renderGrid();
  });
  renderGrid();

  container.append(searchWrap, grid);
  shell.append(container);
  section.append(shell);
  return section;
}

function isTextInput(target: EventTarget | null): boolean {
  const tag = (target as HTMLElement | null)?.tagName?.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select";
}

export function renderHomeMenuApp(app: HomeMenuApp, onClick?: () => void): HTMLElement {
  const wrapper = document.createElement("div");
  wrapper.className = "col-3 col-md-2 o_draggable mb-3 px-0";

  const button = document.createElement("a");
  button.className = "o_app o_menuitem has-icon d-flex flex-column rounded-3 justify-content-start align-items-center w-100 p-1 p-md-2";
  button.setAttribute("role", "option");
  button.setAttribute("aria-selected", "false");
  button.setAttribute("href", appHref(app));
  button.dataset.appName = app.name;
  button.dataset.appKey = app.key || appKey(app.name);
  button.dataset.menuId = String(app.id);
  if (typeof app.menu.xmlid === "string" && app.menu.xmlid) button.dataset.menuXmlid = app.menu.xmlid;
  if (app.rootId !== undefined) button.dataset.rootMenuId = String(app.rootId);
  if (app.parentPath) button.dataset.menuPath = app.parentPath;
  if (app.isMenuAction) button.dataset.menuAction = "true";
  button.title = app.name;
  button.setAttribute("aria-label", app.name);

  const icon = document.createElement("span");
  icon.className = "o_app_icon position-relative d-flex justify-content-center align-items-center p-2 rounded-3";
  icon.dataset.iconToken = app.iconToken;
  icon.setAttribute("aria-hidden", "true");

  const name = document.createElement("strong");
  name.className = "o_caption o_app_name w-100 text-center text-truncate mt-2";
  name.textContent = app.name;
  const iconImage = appIconImage(app);
  button.append(iconImage ?? icon, name);

  if (app.parentPath) {
    const path = document.createElement("span");
    path.className = "o_app_menu_path";
    path.textContent = app.parentPath;
    button.append(path);
  }
  if (onClick) button.addEventListener("click", (event) => {
    event?.preventDefault?.();
    onClick();
  });
  wrapper.append(button);
  return wrapper;
}

const APPS_CATALOG_SEARCH_TEXT = "apps applications modules install";

function shouldAppendAppsCatalog(query: string, visible: readonly HomeMenuApp[], catalogApp: HomeMenuApp | null): boolean {
  if (!catalogApp) return false;
  if (visible.some((app) => app.key === "apps" || isAppsCatalogApp(app))) return false;
  if (!query) return true;
  return APPS_CATALOG_SEARCH_TEXT.includes(query) || catalogApp.searchText.includes(query);
}

function appHref(app: HomeMenuApp): string {
  const actionID = app.menu.actionID || app.menu.actionId || app.menu.directActionID;
  const menuID = encodeURIComponent(String(app.id));
  if (actionID) return `#menu_id=${menuID}&action=${encodeURIComponent(String(actionID))}`;
  return `#menu_id=${menuID}`;
}

function appIconImage(app: HomeMenuApp): HTMLImageElement | null {
  const source = typeof app.menu.webIconData === "string" ? app.menu.webIconData.trim() : "";
  const iconSource = appIconSource(source, app.menu.webIconDataMimetype);
  if (!iconSource) return null;
  const image = document.createElement("img");
  image.className = "o_app_icon rounded-3";
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
