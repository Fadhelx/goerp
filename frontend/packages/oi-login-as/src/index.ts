export type LoginAsUserId = number | string;

export interface LoginAsUserRef {
  id: LoginAsUserId;
  displayName: string;
  login?: string;
  isSystem?: boolean;
  active?: boolean;
}

export interface LoginAsSessionInput {
  impersonate?: boolean;
  currentUser?: LoginAsUserRef | null;
  originalUser?: LoginAsUserRef | null;
  effectiveUser?: LoginAsUserRef | null;
  redirect?: string;
  debug?: boolean;
  userIsSystem?: boolean;
  allowDebugBecomeSuperuser?: boolean;
}

export type LoginAsSessionInfo = Record<string, unknown>;

export interface LoginAsActionDescriptor {
  id: string;
  label: string;
  url: string;
  method: "GET" | "POST";
  visible: boolean;
  enabled: boolean;
  reason?: string;
}

export interface ImpersonationBannerState {
  visible: boolean;
  actorName?: string;
  effectiveName?: string;
  text: string;
}

export interface DebugRouteGateState {
  debugMenuVisible: boolean;
  becomeSuperuserVisible: boolean;
  routeEnabled: boolean;
  reason?: string;
}

export interface ImpersonationContext {
  active: boolean;
  debug: boolean;
  redirect: string;
  actor?: LoginAsUserRef;
  effective?: LoginAsUserRef;
  banner: ImpersonationBannerState;
  switchAction: LoginAsActionDescriptor;
  backAction: LoginAsActionDescriptor;
  debugGate: DebugRouteGateState;
}

export interface SwitchUserActionOptions {
  redirect?: string;
  visible?: boolean;
  enabled?: boolean;
  allowInactive?: boolean;
  allowSystemTarget?: boolean;
}

export function createImpersonationContext(input: LoginAsSessionInput): ImpersonationContext {
  const active = Boolean(input.impersonate);
  const actor = input.originalUser ?? input.currentUser ?? undefined;
  const effective = input.effectiveUser ?? input.currentUser ?? undefined;
  const redirect = normalizeRedirect(input.redirect);
  const debug = Boolean(input.debug);
  return {
    active,
    debug,
    redirect,
    ...(actor ? { actor } : {}),
    ...(effective ? { effective } : {}),
    banner: createImpersonationBanner({ active, actor, effective }),
    switchAction: createSwitchUserAction(effective ?? actor ?? { id: "", displayName: "" }, {
      redirect,
      visible: debug && Boolean(input.userIsSystem)
    }),
    backAction: createLoginBackAction({ active, redirect }),
    debugGate: createDebugRouteGate({
      debug,
      userIsSystem: Boolean(input.userIsSystem),
      impersonate: active,
      allowDebugBecomeSuperuser: Boolean(input.allowDebugBecomeSuperuser)
    })
  };
}

export function createImpersonationContextFromSession(
  session: LoginAsSessionInfo,
  options: Partial<LoginAsSessionInput> = {}
): ImpersonationContext {
  return createImpersonationContext({ ...normalizeLoginAsSessionInfo(session), ...options });
}

export function normalizeLoginAsSessionInfo(session: LoginAsSessionInfo): LoginAsSessionInput {
  const loginAs = asRecord(session.login_as);
  const userContext = asRecord(session.user_context);
  const uid = readUserId(session.uid) ?? readUserId(userContext.uid);
  const originalUID =
    readUserId(loginAs.original_uid) ??
    readUserId(session.login_as_original_uid) ??
    readUserId(userContext.login_as_original_uid);
  const effectiveUID =
    readUserId(loginAs.effective_uid) ??
    readUserId(session.login_as_user_id) ??
    readUserId(userContext.login_as_user_id) ??
    uid;
  const active =
    readBool(loginAs.active) ??
    readBool(session.impersonate) ??
    readBool(session.login_as) ??
    readBool(userContext.login_as) ??
    originalUID !== undefined;
  const redirect =
    readText(loginAs.return_to) ??
    readText(session.login_as_return_to) ??
    readText(userContext.login_as_return_to) ??
    readText(userContext.login_as_back_route) ??
    "/web";
  const banner = readText(loginAs.banner) ?? readText(session.login_as_banner);
  const effectiveName = readText(session.login_as_effective_name) ?? banner?.replace(/^Impersonating\s+/i, "");

  return {
    impersonate: active,
    currentUser: effectiveUID !== undefined ? userRef(effectiveUID, effectiveName ?? `User ${effectiveUID}`) : null,
    originalUser: originalUID !== undefined ? userRef(originalUID, `User ${originalUID}`) : null,
    effectiveUser: effectiveUID !== undefined ? userRef(effectiveUID, effectiveName ?? `User ${effectiveUID}`) : null,
    redirect,
    debug: readBool(session.debug) ?? readBool(session.debug_mode) ?? false,
    userIsSystem: readBool(session.is_system) ?? readBool(session.is_admin) ?? false,
    allowDebugBecomeSuperuser: readBool(session.allow_debug_become_superuser) ?? readBool(userContext.allow_debug_become_superuser) ?? false
  };
}

export function createImpersonationBanner(input: {
  active?: boolean;
  actor?: LoginAsUserRef | null;
  effective?: LoginAsUserRef | null;
}): ImpersonationBannerState {
  const visible = Boolean(input.active);
  const actorName = input.actor?.displayName;
  const effectiveName = input.effective?.displayName;
  return {
    visible,
    ...(actorName ? { actorName } : {}),
    ...(effectiveName ? { effectiveName } : {}),
    text: visible ? `Logged in as ${effectiveName ?? "user"}` : ""
  };
}

export function createSwitchUserAction(
  target: LoginAsUserRef,
  options: SwitchUserActionOptions = {}
): LoginAsActionDescriptor {
  const redirect = normalizeRedirect(options.redirect);
  let enabled = options.enabled !== false;
  let reason = "";
  if (!target.id && target.id !== 0) {
    enabled = false;
    reason = "target user is required";
  } else if (target.active === false && !options.allowInactive) {
    enabled = false;
    reason = "target user is inactive";
  } else if (target.isSystem && !options.allowSystemTarget) {
    enabled = false;
    reason = "system user target is blocked";
  }
  return {
    id: "login_as",
    label: target.displayName ? `Login as ${target.displayName}` : "Login as",
    url: `/web/login_as/${encodeURIComponent(String(target.id))}?redirect=${encodeURIComponent(redirect)}`,
    method: "GET",
    visible: options.visible !== false,
    enabled,
    ...(reason ? { reason } : {})
  };
}

export function createLoginBackAction(input: { active?: boolean; redirect?: string } = {}): LoginAsActionDescriptor {
  const redirect = normalizeRedirect(input.redirect);
  const visible = Boolean(input.active);
  return {
    id: "login_back",
    label: "Login back",
    url: `/web/login_back?redirect=${encodeURIComponent(redirect)}`,
    method: "GET",
    visible,
    enabled: visible,
    ...(!visible ? { reason: "not impersonating" } : {})
  };
}

export function createDebugRouteGate(input: {
  debug?: boolean;
  userIsSystem?: boolean;
  impersonate?: boolean;
  allowDebugBecomeSuperuser?: boolean;
}): DebugRouteGateState {
  const debug = Boolean(input.debug);
  const userIsSystem = Boolean(input.userIsSystem);
  const impersonate = Boolean(input.impersonate);
  const debugMenuVisible = (debug && userIsSystem) || impersonate;
  const becomeSuperuserVisible = debug && userIsSystem && !impersonate;
  const routeEnabled = becomeSuperuserVisible && Boolean(input.allowDebugBecomeSuperuser);
  let reason = "";
  if (!debug) reason = "debug mode is disabled";
  else if (!userIsSystem) reason = "system user is required";
  else if (impersonate) reason = "already impersonating";
  else if (!input.allowDebugBecomeSuperuser) reason = "debug become-superuser route is disabled";
  return {
    debugMenuVisible,
    becomeSuperuserVisible,
    routeEnabled,
    ...(routeEnabled ? {} : { reason })
  };
}

export function createDebugBecomeSuperuserAction(input: {
  redirect?: string;
  gate: DebugRouteGateState;
}): LoginAsActionDescriptor {
  const redirect = normalizeRedirect(input.redirect);
  return {
    id: "become_debug_superuser",
    label: "Become superuser",
    url: `/web/become/debug?redirect=${encodeURIComponent(redirect)}`,
    method: "GET",
    visible: input.gate.becomeSuperuserVisible,
    enabled: input.gate.routeEnabled,
    ...(input.gate.reason ? { reason: input.gate.reason } : {})
  };
}

export function normalizeRedirect(value: string | undefined): string {
  const redirect = typeof value === "string" ? value.trim() : "";
  if (!redirect) return "/web";
  if (redirect.startsWith("/") && !redirect.startsWith("//") && !redirect.includes("\n")) return redirect;
  return "/web";
}

function userRef(id: LoginAsUserId, displayName: string): LoginAsUserRef {
  return { id, displayName };
}

function asRecord(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function readUserId(value: unknown): LoginAsUserId | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim()) return value;
  return undefined;
}

function readText(value: unknown): string | undefined {
  const text = typeof value === "string" ? value.trim() : "";
  return text ? text : undefined;
}

function readBool(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}
