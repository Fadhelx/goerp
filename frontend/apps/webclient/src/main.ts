import {
  createWebClient,
  makeEnv,
  startServices,
  type RPCRequest,
  type SessionService
} from "../../../packages/webclient/src/index.js";
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
  globalThis.document.documentElement.dataset.tsWebclient = "ready";
  if (shouldTakeOverDOM()) {
    const target = ensureRoot();
    target.replaceChildren(createWebClient({
      env: { debug: Boolean(env.debug), isSmall },
      theme: enterpriseLikeTheme,
      session,
      menus
    }).render());
  }
  globalThis.dispatchEvent(new CustomEvent("goerp:webclient-ready", {
    detail: { session, menus }
  }));
  return { session, menus };
}

async function rpcTransport(request: RPCRequest): Promise<unknown> {
  return fetchJSON(request.route, request.params);
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
  return new URLSearchParams(globalThis.location?.search ?? "").get("ts_webclient") === "1";
}

function ensureRoot(): HTMLElement {
  let root = globalThis.document.querySelector<HTMLElement>("#tsWebClientRoot");
  if (!root) {
    root = globalThis.document.createElement("main");
    root.id = "tsWebClientRoot";
    root.className = "o_action_manager";
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
