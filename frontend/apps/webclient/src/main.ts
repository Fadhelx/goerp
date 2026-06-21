import {
  createWebClient,
  makeEnv,
  parseRouteState,
  renderWindowAction,
  renderWindowActionDialog,
  routeStateFromAction,
  startServices,
  updateBrowserRoute,
  type ActionRequest,
  type ActionService,
  type ActionServiceOptions,
  type RPCRequest,
  type SessionService,
  type WebClientRouteState,
  type WebClientServices,
  type WindowActionResult
} from "../../../packages/webclient/src/index.js";
import {
  appIconToken,
  appInitials,
  cleanAppName,
  homeMenuEntry,
  isAppsCatalogApp,
  type HomeMenuApp,
  type HomeMenuEntry,
  type HomeMenuPayload
} from "../../../packages/webclient/src/home_menu/app_metadata.js";
import { enterpriseLikeTheme } from "../../../themes/enterprise-like/src/theme.js";

export interface GoERPWebClientBootstrapResult {
  session: Record<string, unknown>;
  menus: Record<string, unknown>;
}

export async function bootstrapGoERPWebClient(): Promise<GoERPWebClientBootstrapResult> {
  const env = makeEnv({
    debug: new URLSearchParams(globalThis.location?.search ?? "").has("debug"),
    services: {}
  });
  const isSmall = globalThis.matchMedia?.("(max-width: 767px)")?.matches === true;
  env.rpcTransport = rpcTransport;
  await startServices(env);
  let session = await (env.services.session as SessionService).load();
  if (shouldQuickLogin(session)) {
    session = await fetchJSON<Record<string, unknown>>("/web/session/authenticate", {
      login: "admin",
      password: "admin"
    });
  }
  const menus = await fetchJSON<Record<string, unknown>>("/web/webclient/load_menus");
  if (shouldTakeOverDOM()) {
    const target = ensureRoot();
    const shell = createWebClient({
      env: { debug: Boolean(env.debug), isSmall },
      theme: enterpriseLikeTheme,
      session,
      menus,
      onOpenApp: (app, outlet) => {
        void openMenuApp(env, app, outlet).catch((error) => renderActionError(outlet, error));
      }
    }).render();
    target.replaceChildren(shell);
    await restoreActionFromHash(env, menus as HomeMenuPayload, shell);
  }
  globalThis.document.documentElement.dataset.tsWebclient = "ready";
  globalThis.dispatchEvent(new CustomEvent("goerp:webclient-ready", {
    detail: { session, menus }
  }));
  return { session, menus };
}

async function restoreActionFromHash(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  shell: HTMLElement
): Promise<boolean> {
  const route = parseRouteState(globalThis.location?.hash ?? "");
  const actionID = routeActionID(route);
  if (actionID === undefined) return false;
  const outlet = findDescendantByClass(shell, "o_action_manager");
  if (!outlet) return false;
  const app = routeMenuApp(menus, route.menu_id);
  const context = routeActionContext(route);
  const actionHost = createActionHost(env, outlet, app);
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(app?.name ?? "Action"));
  try {
    const action = await actionHost.loadAction(actionID, context);
    await actionHost.doAction(actionWithRouteState(action, route), {
      additionalContext: context,
      stackPosition: "clear"
    });
    return true;
  } catch (error) {
    renderActionError(outlet, error);
    return false;
  }
}

function routeActionID(route: WebClientRouteState): number | string | undefined {
  return routeID(route.action);
}

function routeActionContext(route: WebClientRouteState): Record<string, unknown> {
  const context: Record<string, unknown> = {};
  const menuID = routeID(route.menu_id);
  const activeID = routeID(route.active_id ?? route.id);
  if (menuID !== undefined) context.menu_id = menuID;
  if (activeID !== undefined) context.active_id = activeID;
  if (Array.isArray(route.active_ids)) context.active_ids = [...route.active_ids];
  return context;
}

function actionWithRouteState(action: Record<string, unknown>, route: WebClientRouteState): Record<string, unknown> {
  const next = { ...action };
  const model = typeof route.model === "string" && route.model.trim() ? route.model.trim() : "";
  const viewType = typeof route.view_type === "string" && route.view_type.trim() ? route.view_type.trim() : "";
  const recordID = routeID(route.id);
  const menuID = routeID(route.menu_id);
  if (model && !next.res_model) next.res_model = model;
  if (recordID !== undefined) next.res_id = recordID;
  if (menuID !== undefined) next.menu_id = menuID;
  if (viewType) {
    next.view_type = viewType;
    next.view_mode = viewType;
    next.views = actionViewsWithFirstType(action, viewType);
  }
  return next;
}

function actionViewsWithFirstType(action: Record<string, unknown>, viewType: string): Array<[number | false, string]> {
  const views = actionViewRefs(action);
  const target = views.find((view) => view[1] === viewType) ?? [false, viewType] as [false, string];
  return [target, ...views.filter((view) => view[1] !== viewType)];
}

function actionViewRefs(action: Record<string, unknown>): Array<[number | false, string]> {
  if (Array.isArray(action.views)) {
    const views: Array<[number | false, string]> = [];
    for (const view of action.views) {
      if (!Array.isArray(view) || typeof view[1] !== "string" || !view[1].trim()) continue;
      views.push([typeof view[0] === "number" && view[0] > 0 ? view[0] : false, view[1].trim()]);
    }
    if (views.length) return views;
  }
  const viewMode = typeof action.view_mode === "string" && action.view_mode.trim() ? action.view_mode : "list,form";
  return viewMode
    .split(",")
    .map((view) => view.trim())
    .filter(Boolean)
    .map((view): [false, string] => [false, view]);
}

function routeMenuApp(menus: HomeMenuPayload, menuID: unknown): HomeMenuApp | undefined {
  const id = routeID(menuID);
  if (id === undefined) return undefined;
  const menu = homeMenuEntry(menus, id);
  if (!menu) return undefined;
  const name = cleanAppName(menu.name);
  return {
    id,
    key: String(id),
    name,
    initials: appInitials(name),
    iconToken: appIconToken(name),
    sequence: 0,
    searchText: name.toLowerCase(),
    menu: menu as HomeMenuEntry
  };
}

function routeID(value: unknown): number | string | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

function findDescendantByClass(root: HTMLElement, className: string): HTMLElement | null {
  if (String(root.className).split(/\s+/).includes(className)) return root;
  for (const child of Array.from(root.children)) {
    const found = findDescendantByClass(child as HTMLElement, className);
    if (found) return found;
  }
  return null;
}

async function rpcTransport(request: RPCRequest): Promise<unknown> {
  return fetchJSON(request.route, request.params);
}

async function openMenuApp(env: ReturnType<typeof makeEnv>, app: HomeMenuApp, outlet: HTMLElement): Promise<void> {
  if (isAppsCatalogApp(app)) {
    await renderAppsCatalog(env, outlet, app.name);
    return;
  }
  const actionID = menuActionID(app);
  if (actionID === undefined) {
    outlet.dataset.tsActionStatus = "no-action";
    return;
  }
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(app.name));
  const actionHost = createActionHost(env, outlet, app);
  await actionHost.doAction(actionID, {
    additionalContext: { menu_id: app.id },
    stackPosition: "clear"
  });
}

interface ActionHostState {
  app?: HomeMenuApp;
  dialogs: ActionDialogMount[];
  env: ReturnType<typeof makeEnv>;
  outlet: HTMLElement;
  service: ActionService;
}

interface ActionDialogMount {
  backdrop: HTMLElement;
  dialog: HTMLElement;
}

function createActionHost(env: ReturnType<typeof makeEnv>, outlet: HTMLElement, app?: HomeMenuApp): ActionService {
  const service = env.services.action as ActionService;
  const state: ActionHostState = { app, dialogs: [], env, outlet, service };
  const host: ActionService = {
    get history() {
      return service.history;
    },
    get current() {
      return service.current;
    },
    get stack() {
      return service.stack;
    },
    get currentRoute() {
      return service.currentRoute;
    },
    get breadcrumbs() {
      return service.breadcrumbs;
    },
    loadAction(action: ActionRequest, context?: Record<string, unknown>) {
      return service.loadAction(action, context);
    },
    async doAction<T = unknown>(action: ActionRequest, options: ActionServiceOptions = {}): Promise<T> {
      const result = await service.doAction<unknown>(action, options);
      renderActionHostResult(state, host, result);
      return result as T;
    },
    closeCurrent() {
      const current = service.closeCurrent();
      removeTopDialog(state);
      return current;
    },
    clearStack() {
      service.clearStack();
      clearDialogs(state);
    },
    restoreStack(entries) {
      return service.restoreStack(entries);
    }
  };
  return host;
}

function renderActionHostResult(state: ActionHostState, host: ActionService, result: unknown): void {
  if (isCloseActionResult(result)) {
    removeTopDialog(state);
    return;
  }
  if (!isWindowActionResult(result)) {
    if (!state.dialogs.length && state.outlet.dataset.tsActionStatus === "loading") {
      state.outlet.dataset.tsActionStatus = "unsupported";
      state.outlet.replaceChildren(renderUnsupportedAction(state.app?.name ?? "Action"));
    }
    return;
  }
  const titledResult = withFallbackActionTitle(result, state.app?.name ?? "");
  const services = actionHostServices(state.env, host);
  if (isDialogWindowAction(titledResult)) {
    const dialog = renderWindowActionDialog(titledResult, { services });
    const backdrop = document.createElement("div");
    backdrop.className = "modal-backdrop gorp-action-dialog-backdrop show";
    dialog.addEventListener("dialog:close", () => {
      void host.doAction({ type: "ir.actions.act_window_close" });
    });
    state.dialogs.push({ backdrop, dialog });
    state.outlet.append(backdrop, dialog);
    setBodyModalOpen(state);
    state.outlet.dataset.tsDialogStatus = "ready";
    return;
  }
  clearDialogs(state);
  updateBrowserRoute(routeStateFromAction(titledResult.action, {
    ...(state.app ? { menu_id: state.app.id } : {}),
    view_type: titledResult.activeView
  }));
  state.outlet.dataset.tsActionStatus = "ready";
  state.outlet.replaceChildren(renderWindowAction(titledResult, { services }));
}

function actionHostServices(env: ReturnType<typeof makeEnv>, action: ActionService): WebClientServices {
  return {
    ...(env.services as unknown as WebClientServices),
    action
  };
}

function isDialogWindowAction(result: WindowActionResult): boolean {
  return result.action.target === "new";
}

function isCloseActionResult(result: unknown): boolean {
  return Boolean(result && typeof result === "object" && (result as Record<string, unknown>).type === "ir.actions.act_window_close");
}

function removeTopDialog(state: ActionHostState): void {
  const mount = state.dialogs.pop();
  mount?.dialog.remove();
  mount?.backdrop.remove();
  setBodyModalOpen(state);
  state.outlet.dataset.tsDialogStatus = state.dialogs.length ? "ready" : "closed";
}

function clearDialogs(state: ActionHostState): void {
  for (const mount of state.dialogs.splice(0)) {
    mount.dialog.remove();
    mount.backdrop.remove();
  }
  setBodyModalOpen(state);
  delete state.outlet.dataset.tsDialogStatus;
}

function setBodyModalOpen(state: ActionHostState): void {
  if (state.dialogs.length) {
    document.body.classList?.add("modal-open");
  } else {
    document.body.classList?.remove("modal-open");
  }
}

function menuActionID(app: HomeMenuApp): string | number | undefined {
  const value = app.menu.actionID ?? app.menu.actionId;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value.trim();
  return undefined;
}

function isWindowActionResult(value: unknown): value is WindowActionResult {
  const payload = value as WindowActionResult | null;
  return Boolean(payload && payload.type === "ir.actions.act_window" && payload.viewDescriptions && payload.resModel);
}

function withFallbackActionTitle(result: WindowActionResult, fallback: string): WindowActionResult {
  if (typeof result.action.name === "string" && result.action.name.trim()) return result;
  return {
    ...result,
    action: {
      ...result.action,
      name: fallback
    }
  };
}

function renderActionLoading(label: string): HTMLElement {
  const node = globalThis.document.createElement("section");
  node.className = "o_view_nocontent";
  node.textContent = `Loading ${label}...`;
  return node;
}

function renderUnsupportedAction(label: string): HTMLElement {
  const node = globalThis.document.createElement("section");
  node.className = "o_view_nocontent";
  node.textContent = `${label} action is not supported yet.`;
  return node;
}

function renderActionError(outlet: HTMLElement, error: unknown): void {
  outlet.dataset.tsActionStatus = "error";
  const node = globalThis.document.createElement("section");
  node.className = "o_view_nocontent text-danger";
  node.textContent = error instanceof Error ? error.message : String(error);
  outlet.replaceChildren(node);
}

export interface AppsCatalogModule {
  application?: boolean;
  auto_install?: boolean;
  category?: string;
  depends?: readonly string[];
  installable?: boolean;
  name?: string;
  state?: string;
  technical_name?: string;
}

export interface AppsCatalogPayload {
  modules?: Record<string, AppsCatalogModule>;
}

export interface AppsCatalogRenderOptions {
  onInstall?: (technicalName: string) => unknown;
  onModuleAction?: (technicalName: string, method: AppsCatalogActionMethod, query: string) => unknown;
  query?: string;
  title?: string;
}

export type AppsCatalogActionMethod =
  | "button_immediate_install"
  | "button_immediate_upgrade"
  | "button_immediate_uninstall"
  | "button_cancel_install"
  | "button_cancel_upgrade"
  | "button_cancel_uninstall";

interface AppsCatalogAction {
  className: string;
  label: string;
  method: AppsCatalogActionMethod;
  runningLabel: string;
}

export function renderAppsCatalogView(payload: AppsCatalogPayload, options: AppsCatalogRenderOptions = {}): HTMLElement {
  const root = document.createElement("section");
  root.className = "gorp-apps-catalog o_apps_view";
  root.dataset.model = "ir.module.module";
  const control = document.createElement("div");
  control.className = "o-control-panel o_control_panel";
  const main = document.createElement("div");
  main.className = "o_control_panel_main";
  const breadcrumbs = document.createElement("div");
  breadcrumbs.className = "o_control_panel_breadcrumbs";
  const title = document.createElement("h2");
  title.className = "o_breadcrumb active";
  title.textContent = options.title || "Apps";
  breadcrumbs.append(title);
  const actions = document.createElement("div");
  actions.className = "o_control_panel_actions";
  const search = document.createElement("input");
  search.type = "search";
  search.className = "o_searchview_input o_input";
  search.placeholder = "Search...";
  search.setAttribute("aria-label", "Search apps");
  search.value = options.query || "";
  actions.append(search);
  const navigation = document.createElement("div");
  navigation.className = "o_control_panel_navigation";
  const pager = document.createElement("div");
  pager.className = "o_cp_pager o_pager";
  navigation.append(pager);
  main.append(breadcrumbs, actions, navigation);
  control.append(main);
  const content = document.createElement("div");
  content.className = "o-list-content";
  const grid = document.createElement("div");
  grid.className = "gorp-apps-catalog-grid o_apps";
  content.append(grid);
  root.append(control, content);

  const renderGrid = () => {
    const query = search.value.trim().toLowerCase();
    grid.replaceChildren();
    const modules = appsCatalogModules(payload).filter((item) => !query || item.searchText.includes(query));
    for (const item of modules) {
      grid.append(renderAppsCatalogCard(item, { ...options, query }));
    }
    if (!grid.children.length) {
      const empty = document.createElement("p");
      empty.className = "o_view_nocontent";
      empty.textContent = query ? "No apps found." : "No apps available.";
      grid.append(empty);
    }
    pager.textContent = modules.length ? `1-${modules.length} / ${modules.length}` : "0 / 0";
  };
  search.addEventListener("input", renderGrid);
  renderGrid();
  return root;
}

interface AppsCatalogDisplayModule {
  displayName: string;
  installable: boolean;
  searchText: string;
  state: string;
  technicalName: string;
}

async function renderAppsCatalog(_env: ReturnType<typeof makeEnv>, outlet: HTMLElement, title: string, query = ""): Promise<void> {
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(title || "Apps"));
  const payload = await fetchJSON<AppsCatalogPayload>("/web/session/modules");
  const view = renderAppsCatalogView(payload, {
    title,
    query,
    onModuleAction: async (technicalName, method, currentQuery) => {
      await runAppsCatalogModuleAction(technicalName, method);
      await renderAppsCatalog(_env, outlet, title, currentQuery);
    }
  });
  outlet.dataset.tsActionStatus = "ready";
  outlet.replaceChildren(view);
}

export async function installAppsCatalogModule(technicalName: string): Promise<void> {
  await runAppsCatalogModuleAction(technicalName, "button_immediate_install");
}

async function runAppsCatalogModuleAction(technicalName: string, method: AppsCatalogActionMethod): Promise<void> {
  const rows = await fetchJSON<Array<Record<string, unknown>>>("/web/dataset/call_kw", {
    model: "ir.module.module",
    method: "search_read",
    args: [[["name", "=", technicalName]]],
    kwargs: { fields: ["id"], limit: 1 }
  });
  const id = numericUserID(rows[0]?.id);
  if (!id) throw new Error(`Module ${technicalName} is not available`);
  await fetchJSON("/web/dataset/call_kw", {
    model: "ir.module.module",
    method,
    args: [[id]]
  });
}

function appsCatalogModules(payload: AppsCatalogPayload): AppsCatalogDisplayModule[] {
  const modules = payload.modules && typeof payload.modules === "object" ? payload.modules : {};
  return Object.entries(modules)
    .map(([key, module]) => {
      const technicalName = firstText(module.technical_name, key) || key;
      const displayName = firstText(module.name, moduleDisplayName(technicalName)) || technicalName;
      const state = firstText(module.state, "uninstalled") || "uninstalled";
      const category = firstText(module.category, "");
      return {
        displayName,
        installable: module.installable !== false,
        searchText: [displayName, technicalName, category].join(" ").toLowerCase(),
        state,
        technicalName
      };
    })
    .sort((left, right) => left.displayName.localeCompare(right.displayName) || left.technicalName.localeCompare(right.technicalName));
}

function renderAppsCatalogCard(module: AppsCatalogDisplayModule, options: AppsCatalogRenderOptions): HTMLElement {
  const card = document.createElement("article");
  card.className = "gorp-apps-catalog-card module-card o_app";
  card.dataset.moduleName = module.technicalName;
  card.dataset.appName = module.displayName;
  card.dataset.state = module.state;
  const icon = document.createElement("span");
  icon.className = "app-icon o_app_icon";
  icon.dataset.iconToken = appIconToken(module.displayName);
  icon.textContent = appInitials(module.displayName);
  const title = document.createElement("strong");
  title.className = "o_app_name";
  title.textContent = module.displayName;
  const technical = document.createElement("span");
  technical.className = "text-muted o_app_technical_name";
  technical.textContent = module.technicalName;
  const state = document.createElement("span");
  state.className = "badge o_module_state";
  state.textContent = module.state;
  const actions = document.createElement("div");
  actions.className = "o_module_actions";
  const actionHandler = options.onModuleAction || ((technicalName: string, method: AppsCatalogActionMethod) => {
    if (method === "button_immediate_install") return options.onInstall?.(technicalName);
    return undefined;
  });
  for (const action of appsCatalogActions(module)) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = action.className;
    button.dataset.moduleAction = action.method;
    button.textContent = action.label;
    button.disabled = !module.installable || !actionHandler;
    button.addEventListener("click", async () => {
      button.disabled = true;
      button.textContent = action.runningLabel;
      await actionHandler(module.technicalName, action.method, options.query || "");
    });
    actions.append(button);
  }
  if (!actions.children.length) {
    const locked = document.createElement("button");
    locked.type = "button";
    locked.className = "btn btn-secondary o_module_state_button";
    locked.disabled = true;
    locked.textContent = module.installable ? "Installed" : "Not installable";
    actions.append(locked);
  }
  card.append(icon, title, technical, state, actions);
  return card;
}

function appsCatalogActions(module: AppsCatalogDisplayModule): AppsCatalogAction[] {
  switch (module.state) {
    case "installed":
      return [
        {
          className: "btn btn-secondary o_module_upgrade_button",
          label: "Upgrade",
          method: "button_immediate_upgrade",
          runningLabel: "Upgrading"
        },
        {
          className: "btn btn-outline-secondary o_module_uninstall_button",
          label: "Uninstall",
          method: "button_immediate_uninstall",
          runningLabel: "Uninstalling"
        }
      ];
    case "to install":
      return [
        {
          className: "btn btn-outline-secondary o_module_cancel_button",
          label: "Cancel Install",
          method: "button_cancel_install",
          runningLabel: "Canceling"
        }
      ];
    case "to upgrade":
      return [
        {
          className: "btn btn-outline-secondary o_module_cancel_button",
          label: "Cancel Upgrade",
          method: "button_cancel_upgrade",
          runningLabel: "Canceling"
        }
      ];
    case "to remove":
      return [
        {
          className: "btn btn-outline-secondary o_module_cancel_button",
          label: "Cancel Uninstall",
          method: "button_cancel_uninstall",
          runningLabel: "Canceling"
        }
      ];
    default:
      return [
        {
          className: "btn btn-primary o_module_install_button",
          label: "Install",
          method: "button_immediate_install",
          runningLabel: "Installing"
        }
      ];
  }
}

function moduleDisplayName(name: string): string {
  const cleaned = String(name || "").replace(/^oi_/, "").replace(/^base_/, "").replace(/_/g, " ").replace(/\s+/g, " ").trim();
  return cleaned ? cleaned.split(" ").map((part) => part.slice(0, 1).toUpperCase() + part.slice(1)).join(" ") : "Module";
}

function firstText(...values: unknown[]): string {
  for (const value of values) {
    const text = typeof value === "string" ? value.trim() : "";
    if (text) return text;
  }
  return "";
}

async function fetchJSON<T>(route: string, params: Record<string, unknown> = {}): Promise<T> {
  const response = await fetch(route, {
    method: Object.keys(params).length ? "POST" : "GET",
    headers: Object.keys(params).length ? { "Content-Type": "application/json" } : {},
    body: Object.keys(params).length ? JSON.stringify(params) : undefined,
    credentials: "same-origin"
  });
  if (!response.ok) throw new Error(`${route}: HTTP ${response.status}`);
  return await response.json() as T;
}

function shouldQuickLogin(session: Record<string, unknown>): boolean {
  return session.quick_login === true && !numericUserID(session.uid);
}

function numericUserID(value: unknown): number {
  if (typeof value === "number") return value;
  if (typeof value === "string") return Number.parseInt(value, 10) || 0;
  return 0;
}

function shouldTakeOverDOM(): boolean {
  const params = new URLSearchParams(globalThis.location?.search ?? "");
  if (params.get("legacy_webclient") === "1") return false;
  if (params.get("ts_webclient") === "0") return false;
  return true;
}

function ensureRoot(): HTMLElement {
  let root = globalThis.document.querySelector<HTMLElement>("#tsWebClientRoot");
  if (!root) {
    root = globalThis.document.createElement("main");
    root.id = "tsWebClientRoot";
    root.className = "o_web_client_root";
    globalThis.document.body.replaceChildren(root);
  }
  return root;
}

if (typeof document !== "undefined" && shouldTakeOverDOM()) {
  void bootstrapGoERPWebClient().catch((error) => {
    globalThis.document.documentElement.dataset.tsWebclient = "error";
    globalThis.dispatchEvent(new CustomEvent("goerp:webclient-error", {
      detail: { message: error instanceof Error ? error.message : String(error) }
    }));
  });
} else if (typeof document !== "undefined") {
  globalThis.document.documentElement.dataset.tsWebclient = "available";
}
