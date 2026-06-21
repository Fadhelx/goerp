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
