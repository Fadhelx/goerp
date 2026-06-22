import { EventBus, SERVICES_METADATA, validate, type Env } from "../../owl-compat/src/index.js";
import type { ThemeTokens } from "../../theme-tokens/src/index";
import {
  createActionStack,
  isCloseAction,
  shouldReplaceLastAction,
  type ActionBreadcrumb,
  type ActionStackEntry
} from "./services/action_stack.js";
import {
  createSearchModel as createActionSearchModel,
  type SearchFacet,
  type SearchModelState
} from "./search/search_model.js";
import {
  parseSearchArch as parseActionSearchArch,
  searchItemFacet as parsedSearchItemFacet,
  type ParsedSearchArch,
  type ParsedSearchItem
} from "./search/search_arch_parser.js";
import {
  renderControlPanel as renderActionControlPanel,
  type ControlPanelMenuItem as ActionControlPanelMenuItem,
  type ControlPanelView as ActionControlPanelView
} from "./control_panel/control_panel.js";
import type { HomeMenuApp, HomeMenuPayload } from "./home_menu/app_metadata.js";
import { renderSettingsView } from "./settings/settings_renderer.js";
import { createWebClientShell } from "./webclient/shell.js";
import type { NavbarSystrayAction, NavbarSystrayCompany, NavbarSystrayState } from "./webclient/navbar/navbar.js";

export {
  actionBreadcrumbs,
  actionTitle,
  actionViewTypes,
  createActionStack,
  isCloseAction,
  makeActionStackEntry,
  shouldReplaceLastAction,
  type ActionBreadcrumb,
  type ActionStackController,
  type ActionStackEntry,
  type ActionStackSnapshot
} from "./services/action_stack.js";
export {
  normalizeRouteState,
  parseRouteState,
  routeStateFromAction,
  routeStateFromStack,
  routeStateToURL,
  serializeRouteState,
  updateBrowserRoute,
  type ActionRouteSource,
  type BrowserRouteTarget,
  type RouteScalar,
  type RouteValue,
  type WebClientRouteActionState,
  type WebClientRouteState
} from "./router/action_router.js";
export {
  buildSearchState,
  createDateGroupByFacet,
  createDateRangeFacet,
  createSearchModel,
  groupByDescriptor,
  SEARCH_DATE_INTERVALS,
  searchFacetLabel,
  type SearchDateInterval,
  type SearchFacet,
  type SearchFacetType,
  type SearchModel,
  type SearchModelOptions,
  type SearchModelState
} from "./search/search_model.js";
export {
  parseSearchArch,
  searchItemFacet,
  type ParsedSearchArch,
  type ParsedSearchItem,
  type ParsedSearchItemType,
  type SearchArchParseOptions
} from "./search/search_arch_parser.js";
export {
  createControlPanelState,
  renderControlPanel,
  type ControlPanelCallbacks,
  type ControlPanelMenuItem,
  type ControlPanelPager,
  type ControlPanelSearchState,
  type ControlPanelState,
  type ControlPanelView
} from "./control_panel/control_panel.js";

export interface WebClientOptions {
  env: Env;
  theme: ThemeTokens;
  menus?: HomeMenuPayload;
  session?: Record<string, unknown>;
  systray?: NavbarSystrayState;
  onOpenApp?: (app: HomeMenuApp, outlet: HTMLElement) => unknown;
  onOpenAppsCatalog?: (outlet: HTMLElement) => unknown;
  onSystrayAction?: (action: NavbarSystrayAction, outlet: HTMLElement) => unknown;
}

export interface RPCRequest {
  route: string;
  params: Record<string, unknown>;
  id: number;
}

export type RPCTransport = (request: RPCRequest) => Promise<unknown>;

export interface RPCService {
  call<T = unknown>(route: string, params?: Record<string, unknown>): Promise<T>;
}

export interface DatasetService {
  callKw<T = unknown>(
    model: string,
    method: string,
    args?: readonly unknown[],
    kwargs?: Record<string, unknown>
  ): Promise<T>;
  callButton<T = unknown>(
    model: string,
    method: string,
    args?: readonly unknown[],
    kwargs?: Record<string, unknown>
  ): Promise<T>;
}

export type DomainExpression = readonly unknown[];
export type DomainListRepr = readonly (readonly [string | number, string, unknown] | "&" | "|" | "!")[];
export type DomainRepr = DomainListRepr | string | Domain;

export interface PythonExpressionAST {
  type: "expression";
  source: string;
}

export type PythonValueAST =
  | { type: 0; value: number }
  | { type: 1; value: unknown }
  | { type: 2; value: boolean }
  | { type: 3 }
  | { type: 4; value: PythonValueAST[] }
  | { type: 10; value: PythonValueAST[] }
  | { type: 11; value: Record<string, PythonValueAST> };

export interface ORMService {
  readonly silent: ORMService;
  cache(options?: Record<string, unknown>): ORMService;
  call<T = unknown>(
    model: string,
    method: string,
    args?: readonly unknown[],
    kwargs?: Record<string, unknown>
  ): Promise<T>;
  create<T = unknown>(model: string, records: readonly Record<string, unknown>[], kwargs?: Record<string, unknown>): Promise<T>;
  nameGet<T = unknown>(model: string, ids: readonly number[], kwargs?: Record<string, unknown>): Promise<T>;
  read<T = unknown>(model: string, ids: readonly number[], fields?: readonly string[], kwargs?: Record<string, unknown>): Promise<T>;
  search<T = unknown>(model: string, domain: DomainExpression, kwargs?: Record<string, unknown>): Promise<T>;
  searchRead<T = unknown>(model: string, domain: DomainExpression, fields?: readonly string[], kwargs?: Record<string, unknown>): Promise<T>;
  searchCount<T = unknown>(model: string, domain: DomainExpression, kwargs?: Record<string, unknown>): Promise<T>;
  defaultGet<T = unknown>(model: string, fields: readonly string[], kwargs?: Record<string, unknown>): Promise<T>;
  unlink<T = unknown>(model: string, ids: readonly number[], kwargs?: Record<string, unknown>): Promise<T>;
  webRead<T = unknown>(model: string, ids: readonly number[], kwargs?: Record<string, unknown>): Promise<T>;
  webReadGroup<T = unknown>(
    model: string,
    domain: DomainExpression,
    groupby: readonly string[],
    aggregates: readonly string[],
    kwargs?: Record<string, unknown>
  ): Promise<T>;
  webResequence<T = unknown>(model: string, ids: readonly number[], kwargs?: Record<string, unknown>): Promise<T>;
  webSearchRead<T = unknown>(model: string, domain: DomainExpression, kwargs?: Record<string, unknown>): Promise<T>;
  write<T = unknown>(
    model: string,
    ids: readonly number[],
    data: Record<string, unknown>,
    kwargs?: Record<string, unknown>
  ): Promise<T>;
  webSave<T = unknown>(
    model: string,
    ids: readonly number[],
    data: Record<string, unknown>,
    kwargs?: Record<string, unknown>
  ): Promise<T>;
  webSaveMulti<T = unknown>(
    model: string,
    ids: readonly number[],
    data: readonly Record<string, unknown>[],
    kwargs?: Record<string, unknown>
  ): Promise<T>;
}

export interface ActionServiceOptions {
  onClose?: () => unknown | Promise<unknown>;
  additional_context?: Record<string, unknown>;
  additionalContext?: Record<string, unknown>;
  clearBreadcrumbs?: boolean;
  replaceLastAction?: boolean;
  stackPosition?: "replace" | "clear" | "push";
  [key: string]: unknown;
}

export type ActionRequest = Record<string, unknown> | string | number;
export type ViewRef = [number | false, string];

export interface ActionInvocation {
  action: Record<string, unknown>;
  options: ActionServiceOptions;
}

export type ActionExecutor = (invocation: ActionInvocation) => unknown | Promise<unknown>;
export type ActionLoader = (action: ActionRequest, context?: Record<string, unknown>) => Promise<Record<string, unknown>>;
export type ClientActionFunctionHandler = (
  env: WebClientEnv | null,
  action: Record<string, unknown>,
  options: ActionServiceOptions
) => unknown | Promise<unknown>;

export interface ExecutableClientAction {
  execute(
    action: Record<string, unknown>,
    env: WebClientEnv | null,
    options: ActionServiceOptions
  ): unknown | Promise<unknown>;
}

export type ClientActionHandler = ClientActionFunctionHandler | ExecutableClientAction;

export interface ActionService {
  readonly history: readonly ActionInvocation[];
  readonly current: ActionInvocation | null;
  readonly stack: readonly ActionStackEntry[];
  readonly currentRoute: ActionStackEntry["route"];
  readonly breadcrumbs: readonly ActionBreadcrumb[];
  loadAction(action: ActionRequest, context?: Record<string, unknown>): Promise<Record<string, unknown>>;
  doAction<T = unknown>(action: ActionRequest, options?: ActionServiceOptions): Promise<T>;
  closeCurrent(): ActionInvocation | null;
  clearStack(): void;
  restoreStack(entries: readonly ActionStackEntry[]): ActionInvocation | null;
}

export interface LoadViewsParams {
  resModel: string;
  views: readonly ViewRef[];
  context?: Record<string, unknown>;
}

export interface LoadViewsOptions {
  actionId?: number | false;
  embeddedActionId?: number | false;
  embeddedParentResId?: number | false;
  loadActionMenus?: boolean;
  loadIrFilters?: boolean;
  [key: string]: unknown;
}

export interface ViewDescription {
  arch: string;
  id: number | false;
  custom_view_id?: number | false;
  actionMenus?: Record<string, unknown>;
  irFilters?: unknown[];
}

export interface ViewDescriptions {
  fields: Record<string, unknown>;
  relatedModels: Record<string, unknown>;
  views: Record<string, ViewDescription>;
}

export type ReadSpecification = Record<string, Record<string, unknown>>;

export interface ViewService {
  loadViews(params: LoadViewsParams, options?: LoadViewsOptions): Promise<ViewDescriptions>;
}

export interface WindowActionResult {
  type: "ir.actions.act_window";
  action: Record<string, unknown>;
  activeView: string;
  resModel: string;
  viewDescriptions: ViewDescriptions;
  search?: WindowActionSearchState;
  records: Record<string, unknown>[];
  length: number;
}

export interface WindowActionSearchState {
  parsed: ParsedSearchArch;
  state: SearchModelState;
  filters: readonly ActionControlPanelMenuItem[];
  groupBys: readonly ActionControlPanelMenuItem[];
  favorites: readonly ActionControlPanelMenuItem[];
}

export interface RenderWindowActionOptions {
  records?: readonly Record<string, unknown>[];
  values?: Record<string, unknown>;
  onUpdate?: (name: string, value: unknown) => void;
  validateForm?: (context: FormValidationContext) => boolean | Promise<boolean>;
  debug?: boolean | string;
  location?: string;
  services?: {
    dataset?: DatasetService;
    action?: ActionService;
    orm?: ORMService;
    notification?: NotificationService;
    mail?: PortalMailService;
  };
  confirm?: (message: string) => boolean | Promise<boolean>;
  onRefresh?: () => unknown | Promise<unknown>;
  context?: Record<string, unknown>;
  activeDomain?: DomainExpression;
  activeGroupBy?: readonly string[];
  isDomainSelected?: boolean;
  activeIdsLimit?: number;
  isSmall?: boolean | (() => boolean);
  exportDownload?: (request: ExportDownloadRequest) => unknown | Promise<unknown>;
  exportGetFields?: (request: ExportGetFieldsRequest) => readonly unknown[] | Promise<readonly unknown[]>;
  exportNamelist?: (request: ExportNamelistRequest) => readonly unknown[] | Promise<readonly unknown[]>;
}

export interface RenderWindowActionDialogOptions extends RenderWindowActionOptions {
  title?: string;
}

interface SettingsActionState {
  initialValues: Record<string, unknown>;
  currentValues: Record<string, unknown>;
  dirtyFields: Set<string>;
  saveButton?: HTMLButtonElement;
  discardButton?: HTMLButtonElement;
  status?: HTMLElement;
  renderBody?: () => void;
}

export interface FormValidationContext {
  form: HTMLElement;
  values: Record<string, unknown>;
  button: ViewButtonNode;
}

interface ActionMenuRecord {
  id?: unknown;
  name?: unknown;
  type?: unknown;
  icon?: unknown;
  description?: unknown;
  groupNumber?: unknown;
  domain?: unknown;
  url?: unknown;
  [key: string]: unknown;
}

export interface ExportDownloadRequest {
  route: string;
  model: string;
  ids: readonly number[] | false;
  domain: DomainExpression;
  fields: readonly { name: string; label: string; store?: unknown; type?: unknown }[];
  context: Record<string, unknown>;
  importCompat: boolean;
  groupby: readonly string[];
}

export interface ExportGetFieldsRequest {
  model: string;
  domain: DomainExpression;
  prefix?: string;
  parent_name?: string;
  import_compat?: boolean;
  parent_field_type?: string;
  parent_field?: Record<string, unknown>;
  exclude?: string[];
}

export interface ExportNamelistRequest {
  model: string;
  export_id: number;
}

export interface SessionService {
  readonly info: Record<string, unknown> | null;
  load(): Promise<Record<string, unknown>>;
}

export interface DialogService {
  calls: Array<{ Component: unknown; props: Record<string, unknown> }>;
  add(Component: unknown, props?: Record<string, unknown>): Promise<unknown>;
}

export interface NotificationService {
  calls: Array<Record<string, unknown>>;
  add(message: string, options?: Record<string, unknown>): void;
}

export interface PortalAccessParams {
  token?: unknown;
  access_token?: unknown;
  accessToken?: unknown;
  hash?: unknown;
  _hash?: unknown;
  pid?: unknown;
}

export interface PortalThreadRef {
  threadModel?: string;
  thread_model?: string;
  threadId?: number;
  thread_id?: number;
}

export interface PortalMailUploadRequest {
  route: string;
  formData: FormData;
}

export type PortalMailUploadTransport = (request: PortalMailUploadRequest) => Promise<unknown>;

export interface PortalAttachmentUploadOptions {
  access?: PortalAccessParams;
  activityID?: number;
  activityId?: number;
  isPending?: boolean;
  temporaryID?: string;
  temporaryId?: string;
  tmpUrl?: string;
  fieldName?: string;
  filename?: string;
  extra?: Record<string, unknown>;
}

export interface PortalMailService {
  accessPayload(access?: PortalAccessParams | null): Record<string, unknown>;
  accessFormFields(access?: PortalAccessParams | null): Record<string, string>;
  avatarUrl(messageId: number, access?: PortalAccessParams | null, size?: string): string;
  chatterInit<T = unknown>(thread: PortalThreadRef, access?: PortalAccessParams | null): Promise<T>;
  chatterFetch<T = unknown>(
    thread: PortalThreadRef,
    fetchParams?: Record<string, unknown>,
    access?: PortalAccessParams | null
  ): Promise<T>;
  postMessage<T = unknown>(
    thread: PortalThreadRef,
    postData: Record<string, unknown>,
    options?: { context?: Record<string, unknown>; access?: PortalAccessParams | null }
  ): Promise<T>;
  updateMessageContent<T = unknown>(
    messageId: number,
    updateData: Record<string, unknown>,
    access?: PortalAccessParams | null
  ): Promise<T>;
  reactMessage<T = unknown>(
    messageId: number,
    content: string,
    action: string,
    access?: PortalAccessParams | null
  ): Promise<T>;
  starredMessages<T = unknown>(fetchParams?: Record<string, unknown>): Promise<T>;
  storeData<T = unknown>(fetchParams?: readonly unknown[], context?: Record<string, unknown>): Promise<T>;
  toggleMessageStarred<T = unknown>(messageId: number): Promise<T>;
  unstarAllMessages<T = unknown>(): Promise<T>;
  uploadAttachment<T = unknown>(
    thread: PortalThreadRef,
    file: Blob | string,
    options?: PortalAttachmentUploadOptions
  ): Promise<T>;
  deleteAttachment<T = unknown>(attachmentId: number, ownershipToken: string): Promise<T>;
}

export interface WebClientServices {
  rpc: RPCService;
  dataset: DatasetService;
  orm: ORMService;
  view: ViewService;
  action: ActionService;
  session: SessionService;
  notification: NotificationService;
  mail: PortalMailService;
}

export interface WebClientServiceOptions {
  rpc?: RPCService;
  transport?: RPCTransport;
  actionExecutor?: ActionExecutor;
  uploadTransport?: PortalMailUploadTransport;
}

export interface WebClientEnv extends Env {
  bus: InstanceType<typeof EventBus>;
  isReady: Promise<unknown>;
  services: Record<string, unknown>;
  debug: unknown;
  actionExecutor?: unknown;
  portalMailUploadTransport?: unknown;
  rpcTransport?: unknown;
  userContext?: unknown;
  readonly isSmall: boolean;
}

export interface ServiceDefinition<T = unknown> {
  start(env: WebClientEnv, dependencies: Record<string, unknown>): T | Promise<T>;
  dependencies?: readonly string[];
  async?: true | readonly string[];
  [key: string]: unknown;
}

export const serviceMetadata = SERVICES_METADATA;

export class KeyNotFoundError extends Error {}

export class DuplicatedKeyError extends Error {}

export interface RegistryAddOptions {
  force?: boolean;
  sequence?: number;
}

export interface RegistryUpdate<T> {
  operation: "add" | "delete";
  key: string;
  value?: T | [number, T];
}

export class Registry<T = unknown> extends EventBus {
  readonly name?: string;
  readonly content: Record<string, [number, T]> = {};
  readonly subRegistries: Record<string, Registry<unknown>> = {};
  private elements: T[] | null = null;
  private entries: Array<[string, T]> | null = null;
  private validationSchema: unknown = null;

  constructor(name?: string) {
    super();
    this.name = name;
    this.addEventListener("UPDATE", () => {
      this.elements = null;
      this.entries = null;
    });
  }

  add(key: string, value: T, options: RegistryAddOptions = {}): this {
    if (this.validationSchema) {
      validateRegistryValue(this.name, key, value, this.validationSchema);
    }
    if (!options.force && key in this.content) {
      throw new DuplicatedKeyError(`Cannot add key "${key}" in the "${this.name}" registry: it already exists`);
    }
    const previousSequence = options.force ? this.content[key]?.[0] : undefined;
    const sequence = options.sequence ?? previousSequence ?? 50;
    this.content[key] = [sequence, value];
    this.trigger("UPDATE", { operation: "add", key, value } satisfies RegistryUpdate<T>);
    return this;
  }

  get(key: string): T;
  get<D>(key: string, defaultValue: D): T | D;
  get<D>(key: string, defaultValue?: D): T | D {
    if (arguments.length < 2 && !(key in this.content)) {
      throw new KeyNotFoundError(`Cannot find key "${key}" in the "${this.name}" registry`);
    }
    const info = this.content[key];
    return info ? info[1] : defaultValue as D;
  }

  contains(key: string): boolean {
    return key in this.content;
  }

  getAll(): T[] {
    if (!this.elements) {
      this.elements = Object.values(this.content)
        .sort((left, right) => left[0] - right[0])
        .map((entry) => entry[1]);
    }
    return this.elements.slice();
  }

  getEntries(): Array<[string, T]> {
    if (!this.entries) {
      this.entries = Object.entries(this.content)
        .sort((left, right) => left[1][0] - right[1][0])
        .map(([key, entry]) => [key, entry[1]]);
    }
    return this.entries.slice();
  }

  remove(key: string): void {
    const value = this.content[key];
    delete this.content[key];
    this.trigger("UPDATE", { operation: "delete", key, value } satisfies RegistryUpdate<T>);
  }

  category<C = unknown>(subcategory: string): Registry<C> {
    if (!(subcategory in this.subRegistries)) {
      this.subRegistries[subcategory] = new Registry(subcategory);
    }
    return this.subRegistries[subcategory] as Registry<C>;
  }

  addValidation(schema: unknown): void {
    if (this.validationSchema) {
      throw new Error("Validation schema already set on this registry");
    }
    this.validationSchema = schema;
    for (const [key, value] of this.getEntries()) {
      validateRegistryValue(this.name, key, value, schema);
    }
  }
}

export const registry = new Registry("root");

export const registries = {
  actions: registry.category("actions"),
  fields: registry.category("fields"),
  views: registry.category("views"),
  services: registry.category("services"),
  main_components: registry.category("main_components"),
  systray: registry.category("systray"),
  debug: registry.category("debug"),
  user_menuitems: registry.category("user_menuitems")
};

export const serviceRegistry = registry.category<ServiceDefinition>("services");

export function patch<T extends object>(target: T, extension: Record<string, unknown>): T {
  for (const [key, value] of Object.entries(extension)) {
    Object.defineProperty(target, key, {
      configurable: true,
      writable: true,
      value
    });
  }
  return target;
}

export function useService<T = unknown>(name: string): T {
  const env = (globalThis as unknown as { __gorpWebClientEnv?: WebClientEnv }).__gorpWebClientEnv;
  if (env && name in env.services) {
    return env.services[name] as T;
  }
  throw new Error(`Service "${name}" is not available`);
}

export function _t(value: string, ...args: unknown[]): string {
  return args.length ? value.replace(/%s/g, () => String(args.shift() ?? "")) : value;
}

export function parseExpr(expression: string): PythonExpressionAST {
  const source = String(expression ?? "").trim();
  if (!source) throw new EvalError("Can not parse empty python expression");
  return { type: "expression", source };
}

export function parse(input: string | readonly string[]): PythonExpressionAST {
  return parseExpr(Array.isArray(input) ? input.join("") : String(input));
}

export function formatAST(ast: PythonExpressionAST | unknown): string {
  if (isRecord(ast) && ast.type === "expression" && typeof ast.source === "string") return ast.source;
  if (isRecord(ast) && typeof ast.type === "number") return formatPyAST(ast as PythonValueAST);
  return formatPyAST(toPyValue(ast));
}

export function tokenize(expression: string): string[] {
  return String(expression ?? "").match(/\s+|[A-Za-z_][A-Za-z0-9_]*|\d+(?:\.\d+)?|==|!=|<>|<=|>=|\/\/|\*\*|[-+*/%()[\]{},.:<>='"]/g) ?? [];
}

export function evaluate(ast: PythonExpressionAST | string, context: Record<string, unknown> = {}): unknown {
  const expression = typeof ast === "string" ? ast : ast.source;
  const parsed = parsePythonLiteral(expression, context);
  if (parsed.ok) return parsed.value;
  throw new EvalError(`Can not evaluate python expression: (${expression})`);
}

export function evaluateExpr(expression: string, context: Record<string, unknown> = {}): unknown {
  try {
    return evaluate(parseExpr(expression), context);
  } catch (error) {
    if (error instanceof EvalError) throw error;
    throw new EvalError(`Can not evaluate python expression: (${expression})\nError: ${String(error)}`);
  }
}

export function evaluateBooleanExpr(expression: string, context: Record<string, unknown> = {}): boolean {
  if (!expression || expression === "False" || expression === "0") return false;
  if (expression === "True" || expression === "1") return true;
  const parsed = parsePythonLiteral(expression, context);
  if (parsed.ok) return pythonTruthy(parsed.value);
  return Boolean(evaluateExpr(`bool(${expression})`, context));
}

export function formatDuration(value: number): string {
  const totalSeconds = Math.max(0, Math.round(value));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours) return `${hours}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

export const browser = {
  setTimeout: globalThis.setTimeout.bind(globalThis),
  clearTimeout: globalThis.clearTimeout.bind(globalThis),
  open(url?: string | URL, target?: string, features?: string): Window | null {
    return typeof window === "undefined" ? null : window.open(url, target, features);
  },
  get location(): Location | undefined {
    return typeof window === "undefined" ? undefined : window.location;
  }
};

export const session: Record<string, unknown> = {};

export const user: {
  context: Record<string, unknown>;
  userId: number;
  isSystem: boolean;
  isAdmin: boolean;
  hasGroup(group: string): Promise<boolean>;
} = {
  context: {},
  userId: 0,
  isSystem: false,
  isAdmin: false,
  hasGroup(group: string): Promise<boolean> {
    const groups = this.context.groups;
    if (Array.isArray(groups)) return Promise.resolve(groups.includes(group));
    if (isRecord(groups)) {
      const value = groups[group];
      if (typeof value === "boolean") return Promise.resolve(value);
      return Promise.resolve(value !== undefined);
    }
    return Promise.resolve(false);
  }
};

function hydrateSessionGlobals(info: Record<string, unknown>): void {
  for (const key of Object.keys(session)) {
    delete session[key];
  }
  Object.assign(session, info);
  user.userId = typeof info.uid === "number" ? info.uid : 0;
  user.isSystem = info.is_system === true;
  user.isAdmin = info.is_admin === true;
  user.context = isRecord(info.user_context) ? { ...info.user_context } : {};
  user.context.groups = info.groups;
}

export function uniqueId(prefix = "id"): string {
  uniqueIDCounter += 1;
  return `${prefix}${uniqueIDCounter}`;
}

let uniqueIDCounter = 0;

export function usePopover() {
  return {
    open() {},
    close() {}
  };
}

export class ConfirmationDialog {}

export class ListController {}

export class FormController {}

export class ViewButton {}

export class StatusBarField {}

export const statusBarField = {
  component: StatusBarField,
  displayName: "StatusBar"
};

export const x2ManyCommands = {
  CREATE: 0,
  UPDATE: 1,
  DELETE: 2,
  UNLINK: 3,
  LINK: 4,
  CLEAR: 5,
  SET: 6,
  create(virtualID: number | false, values: Record<string, unknown>): [number, number | false, Record<string, unknown>] {
    const copy = { ...values };
    delete copy.id;
    return [0, virtualID || false, copy];
  },
  update(id: number, values: Record<string, unknown>): [number, number, Record<string, unknown>] {
    const copy = { ...values };
    delete copy.id;
    return [1, id, copy];
  },
  delete(id: number): [number, number, false] {
    return [2, id, false];
  },
  unlink(id: number): [number, number, false] {
    return [3, id, false];
  },
  link(id: number): [number, number, false] {
    return [4, id, false];
  },
  clear(): [number, false, false] {
    return [5, false, false];
  },
  set(ids: readonly number[]): [number, false, number[]] {
    return [6, false, [...ids]];
  }
};

export const UPDATE_METHODS = [
  "unlink",
  "create",
  "write",
  "web_save",
  "web_save_multi",
  "action_archive",
  "action_unarchive"
] as const;

let startServicesPromise: Promise<void> | null = null;

serviceRegistry.addValidation({
  start: Function,
  dependencies: { type: Array, element: String, optional: true },
  async: { type: [{ type: Array, element: String }, { value: true }], optional: true },
  "*": true
});

export function makeEnv(options: { debug?: unknown; services?: Record<string, unknown> } = {}): WebClientEnv {
  const bus = new EventBus();
  const isReady = new Promise((resolve) => {
    const listener = ((event: Event) => {
      bus.removeEventListener("SERVICES-LOADED", listener);
      resolve((event as CustomEvent).detail);
    }) as EventListener;
    bus.addEventListener("SERVICES-LOADED", listener);
  });
  return {
    bus,
    isReady,
    services: options.services ?? {},
    debug: options.debug ?? false,
    get isSmall(): boolean {
      throw new Error("UI service not initialized!");
    }
  };
}

export async function startServices(env: WebClientEnv, source: Registry<ServiceDefinition> = serviceRegistry): Promise<void> {
  await Promise.resolve();
  const toStart = new Map<string, NamedServiceDefinition>();
  source.addEventListener("UPDATE", async (event) => {
    await Promise.resolve();
    const detail = (event as CustomEvent).detail as RegistryUpdate<ServiceDefinition>;
    if (detail.operation === "delete") return;
    const service = detail.value as ServiceDefinition;
    if (toStart.size) {
      toStart.set(detail.key, namedService(detail.key, service));
    } else {
      await startServiceBatch(env, source, toStart);
    }
  });
  await startServiceBatch(env, source, toStart);
}

export function createRPCService(options: { transport?: RPCTransport } = {}): RPCService {
  let nextID = 0;
  const transport = options.transport ?? fetchRPCTransport;
  return {
    async call<T = unknown>(route: string, params: Record<string, unknown> = {}): Promise<T> {
      return transport({ route, params, id: ++nextID }) as Promise<T>;
    }
  };
}

export function createDatasetService(rpc: RPCService): DatasetService {
  return {
    callKw<T = unknown>(
      model: string,
      method: string,
      args: readonly unknown[] = [],
      kwargs: Record<string, unknown> = {}
    ): Promise<T> {
      const route = `/web/dataset/call_kw/${encodeURIComponent(model)}/${encodeURIComponent(method)}`;
      return rpc.call<T>(route, { model, method, args: [...args], kwargs: { ...kwargs } });
    },
    callButton<T = unknown>(
      model: string,
      method: string,
      args: readonly unknown[] = [],
      kwargs: Record<string, unknown> = {}
    ): Promise<T> {
      const route = `/web/dataset/call_button/${encodeURIComponent(model)}/${encodeURIComponent(method)}`;
      return rpc.call<T>(route, { model, method, args: [...args], kwargs: { ...kwargs } });
    }
  };
}

export function portalAccessPayload(access: PortalAccessParams | null = null): Record<string, unknown> {
  const payload: Record<string, unknown> = {};
  const token = firstText(access?.token, access?.access_token, access?.accessToken);
  const hash = firstText(access?.hash, access?._hash);
  const pid = firstValue(access?.pid);
  if (token !== undefined) payload.token = token;
  if (hash !== undefined) payload.hash = hash;
  if (pid !== undefined) payload.pid = pid;
  return payload;
}

export function portalAccessFormFields(access: PortalAccessParams | null = null): Record<string, string> {
  const fields: Record<string, string> = {};
  const token = firstText(access?.token, access?.access_token, access?.accessToken);
  const hash = firstText(access?.hash, access?._hash);
  const pid = firstValue(access?.pid);
  if (token !== undefined) fields.token = token;
  if (hash !== undefined) fields.hash = hash;
  if (pid !== undefined) fields.pid = String(pid);
  return fields;
}

export function portalMailAvatarUrl(messageId: number, access: PortalAccessParams | null = null, size = "50x50"): string {
  const params = new URLSearchParams();
  const token = firstText(access?.token, access?.access_token, access?.accessToken);
  const hash = firstText(access?.hash, access?._hash);
  const pid = firstValue(access?.pid);
  if (token !== undefined) params.set("access_token", token);
  if (hash !== undefined) params.set("_hash", hash);
  if (pid !== undefined) params.set("pid", String(pid));
  const query = params.toString();
  const path = `/mail/avatar/mail.message/${encodeURIComponent(String(messageId))}/author_avatar/${encodeURIComponent(size)}`;
  return query ? `${path}?${query}` : path;
}

export function createPortalMailService(
  rpc: RPCService,
  options: { uploadTransport?: PortalMailUploadTransport } = {}
): PortalMailService {
  const uploadTransport = options.uploadTransport ?? fetchPortalMailUpload;
  return {
    accessPayload: portalAccessPayload,
    accessFormFields: portalAccessFormFields,
    avatarUrl: portalMailAvatarUrl,
    chatterInit<T = unknown>(thread: PortalThreadRef, access: PortalAccessParams | null = null): Promise<T> {
      return rpc.call<T>("/portal/chatter_init", { ...threadPayload(thread), ...portalAccessPayload(access) });
    },
    chatterFetch<T = unknown>(
      thread: PortalThreadRef,
      fetchParams: Record<string, unknown> = {},
      access: PortalAccessParams | null = null
    ): Promise<T> {
      return rpc.call<T>("/mail/chatter_fetch", {
        ...threadPayload(thread),
        fetch_params: { ...fetchParams },
        ...portalAccessPayload(access)
      });
    },
    postMessage<T = unknown>(
      thread: PortalThreadRef,
      postData: Record<string, unknown>,
      options: { context?: Record<string, unknown>; access?: PortalAccessParams | null } = {}
    ): Promise<T> {
      validatePlainObject("postData", postData);
      return rpc.call<T>("/mail/message/post", {
        ...threadPayload(thread),
        post_data: { ...postData },
        context: { ...(options.context ?? {}) },
        ...portalAccessPayload(options.access)
      });
    },
    updateMessageContent<T = unknown>(
      messageId: number,
      updateData: Record<string, unknown>,
      access: PortalAccessParams | null = null
    ): Promise<T> {
      validatePlainObject("updateData", updateData);
      return rpc.call<T>("/mail/message/update_content", {
        message_id: messageId,
        update_data: { ...updateData },
        ...portalAccessPayload(access)
      });
    },
    reactMessage<T = unknown>(
      messageId: number,
      content: string,
      action: string,
      access: PortalAccessParams | null = null
    ): Promise<T> {
      return rpc.call<T>("/mail/message/reaction", {
        message_id: messageId,
        content,
        action,
        ...portalAccessPayload(access)
      });
    },
    starredMessages<T = unknown>(fetchParams: Record<string, unknown> = {}): Promise<T> {
      return rpc.call<T>("/mail/starred/messages", { fetch_params: { ...fetchParams } });
    },
    storeData<T = unknown>(fetchParams: readonly unknown[] = [], context: Record<string, unknown> = {}): Promise<T> {
      return rpc.call<T>("/mail/data", {
        fetch_params: [...fetchParams],
        context: { ...context }
      });
    },
    toggleMessageStarred<T = unknown>(messageId: number): Promise<T> {
      return rpc.call<T>("/web/dataset/call_kw/mail.message/toggle_message_starred", {
        model: "mail.message",
        method: "toggle_message_starred",
        args: [[messageId]],
        kwargs: {}
      });
    },
    unstarAllMessages<T = unknown>(): Promise<T> {
      return rpc.call<T>("/web/dataset/call_kw/mail.message/unstar_all", {
        model: "mail.message",
        method: "unstar_all",
        args: [],
        kwargs: {}
      });
    },
    async uploadAttachment<T = unknown>(
      thread: PortalThreadRef,
      file: Blob | string,
      options: PortalAttachmentUploadOptions = {}
    ): Promise<T> {
      const formData = new FormData();
      appendFormFields(formData, threadFormFields(thread));
      appendFormFields(formData, options.extra ?? {});
      appendFormFields(formData, portalAccessFormFields(options.access));
      if (options.isPending) formData.set("is_pending", "true");
      if (options.temporaryID || options.temporaryId) formData.set("temporary_id", String(options.temporaryID ?? options.temporaryId));
      if (options.tmpUrl) formData.set("tmp_url", options.tmpUrl);
      if (options.activityID || options.activityId) formData.set("activity_id", String(options.activityID ?? options.activityId));
      const fieldName = options.fieldName ?? "ufile";
      if (typeof file === "string") {
        formData.set(fieldName, file);
      } else if (options.filename) {
        formData.set(fieldName, file, options.filename);
      } else {
        formData.set(fieldName, file);
      }
      return uploadTransport({ route: "/mail/attachment/upload", formData }) as Promise<T>;
    },
    deleteAttachment<T = unknown>(attachmentId: number, ownershipToken: string): Promise<T> {
      return rpc.call<T>("/mail/attachment/delete", { attachment_id: attachmentId, access_token: ownershipToken });
    }
  };
}

export class ORM implements ORMService {
  private readonly rpc: RPCService;
  private readonly silentValue: boolean;
  private readonly cacheValue: Record<string, unknown> | false;
  private readonly userContext: Record<string, unknown>;

  constructor(
    rpc: RPCService,
    options: { silent?: boolean; cache?: Record<string, unknown> | false; userContext?: Record<string, unknown> } = {}
  ) {
    this.rpc = rpc;
    this.silentValue = Boolean(options.silent);
    this.cacheValue = options.cache ?? false;
    this.userContext = { ...(options.userContext ?? {}) };
  }

  get silent(): ORMService {
    return new ORM(this.rpc, { silent: true, cache: this.cacheValue, userContext: this.userContext });
  }

  cache(options: Record<string, unknown> = {}): ORMService {
    return new ORM(this.rpc, { silent: this.silentValue, cache: { ...options }, userContext: this.userContext });
  }

  call<T = unknown>(
    model: string,
    method: string,
    args: readonly unknown[] = [],
    kwargs: Record<string, unknown> = {}
  ): Promise<T> {
    validateModel(model);
    const fullContext = { ...this.userContext, ...(isRecord(kwargs.context) ? kwargs.context : {}) };
    const fullKwargs = { ...kwargs, context: fullContext };
    return this.rpc.call<T>(`/web/dataset/call_kw/${encodeURIComponent(model)}/${encodeURIComponent(method)}`, {
      model,
      method,
      args: [...args],
      kwargs: fullKwargs,
      ...(this.silentValue ? { silent: true } : {}),
      ...(this.cacheValue ? { cache: this.cacheValue } : {})
    });
  }

  create<T = unknown>(model: string, records: readonly Record<string, unknown>[], kwargs: Record<string, unknown> = {}): Promise<T> {
    validateRecordList(records);
    return this.call<T>(model, "create", [records.map((record) => ({ ...record }))], kwargs);
  }

  nameGet<T = unknown>(model: string, ids: readonly number[], kwargs: Record<string, unknown> = {}): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    return ids.length ? this.call<T>(model, "name_get", [[...ids]], kwargs) : Promise.resolve([] as T);
  }

  read<T = unknown>(model: string, ids: readonly number[], fields: readonly string[] = [], kwargs: Record<string, unknown> = {}): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    validatePrimitiveList("fields", "string", fields);
    return ids.length ? this.call<T>(model, "read", [[...ids], [...fields]], kwargs) : Promise.resolve([] as T);
  }

  search<T = unknown>(model: string, domain: DomainExpression, kwargs: Record<string, unknown> = {}): Promise<T> {
    validateArray("domain", domain);
    return this.call<T>(model, "search", [[...domain]], kwargs);
  }

  searchRead<T = unknown>(model: string, domain: DomainExpression, fields: readonly string[] = [], kwargs: Record<string, unknown> = {}): Promise<T> {
    validateArray("domain", domain);
    validatePrimitiveList("fields", "string", fields);
    return this.call<T>(model, "search_read", [], { ...kwargs, domain: [...domain], fields: [...fields] });
  }

  searchCount<T = unknown>(model: string, domain: DomainExpression, kwargs: Record<string, unknown> = {}): Promise<T> {
    validateArray("domain", domain);
    return this.call<T>(model, "search_count", [[...domain]], kwargs);
  }

  defaultGet<T = unknown>(model: string, fields: readonly string[], kwargs: Record<string, unknown> = {}): Promise<T> {
    validatePrimitiveList("fields", "string", fields);
    return this.call<T>(model, "default_get", [[...fields]], kwargs);
  }

  unlink<T = unknown>(model: string, ids: readonly number[], kwargs: Record<string, unknown> = {}): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    return ids.length ? this.call<T>(model, "unlink", [[...ids]], kwargs) : Promise.resolve(true as T);
  }

  webRead<T = unknown>(model: string, ids: readonly number[], kwargs: Record<string, unknown> = {}): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    return this.call<T>(model, "web_read", [[...ids]], kwargs);
  }

  webReadGroup<T = unknown>(
    model: string,
    domain: DomainExpression,
    groupby: readonly string[],
    aggregates: readonly string[],
    kwargs: Record<string, unknown> = {}
  ): Promise<T> {
    validateArray("domain", domain);
    validatePrimitiveList("groupby", "string", groupby);
    validatePrimitiveList("aggregates", "string", aggregates);
    return this.call<T>(model, "web_read_group", [], {
      ...kwargs,
      domain: [...domain],
      groupby: [...groupby],
      aggregates: [...aggregates]
    });
  }

  webResequence<T = unknown>(model: string, ids: readonly number[], kwargs: Record<string, unknown> = {}): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    return this.call<T>(model, "web_resequence", [[...ids]], { ...kwargs, specification: kwargs.specification ?? {} });
  }

  webSearchRead<T = unknown>(model: string, domain: DomainExpression, kwargs: Record<string, unknown> = {}): Promise<T> {
    validateArray("domain", domain);
    return this.call<T>(model, "web_search_read", [], { ...kwargs, domain: [...domain] });
  }

  write<T = unknown>(
    model: string,
    ids: readonly number[],
    data: Record<string, unknown>,
    kwargs: Record<string, unknown> = {}
  ): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    validatePlainObject("data", data);
    return this.call<T>(model, "write", [[...ids], { ...data }], kwargs);
  }

  webSave<T = unknown>(
    model: string,
    ids: readonly number[],
    data: Record<string, unknown>,
    kwargs: Record<string, unknown> = {}
  ): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    validatePlainObject("data", data);
    return this.call<T>(model, "web_save", [[...ids], { ...data }], kwargs);
  }

  webSaveMulti<T = unknown>(
    model: string,
    ids: readonly number[],
    data: readonly Record<string, unknown>[],
    kwargs: Record<string, unknown> = {}
  ): Promise<T> {
    validatePrimitiveList("ids", "number", ids);
    validateRecordList(data);
    return this.call<T>(model, "web_save_multi", [[...ids], data.map((record) => ({ ...record }))], kwargs);
  }
}

export function createORMService(rpc: RPCService, options: { userContext?: Record<string, unknown> } = {}): ORMService {
  return new ORM(rpc, options);
}

export function createViewService(orm: ORMService, env: WebClientEnv | null = null): ViewService {
  return {
    async loadViews(params: LoadViewsParams, options: LoadViewsOptions = {}): Promise<ViewDescriptions> {
      const context = { ...(params.context ?? {}) };
      const loadViewsOptions: Record<string, unknown> = {
        action_id: options.actionId || false,
        embedded_action_id: options.embeddedActionId || false,
        embedded_parent_res_id: options.embeddedParentResId || false,
        load_filters: Boolean(options.loadIrFilters),
        toolbar: !context.disable_toolbar && Boolean(options.loadActionMenus)
      };
      for (const [key, value] of Object.entries(options)) {
        if (!["actionId", "embeddedActionId", "embeddedParentResId", "loadIrFilters", "loadActionMenus"].includes(key)) {
          loadViewsOptions[key] = value;
        }
      }
      if (envIsSmall(env)) loadViewsOptions.mobile = true;
      if (env?.debug) loadViewsOptions.debug = true;
      const filteredContext = Object.fromEntries(
        Object.entries(context).filter(([key]) => key === "lang" || key.endsWith("_view_ref"))
      );
      const result = await orm.cache({ type: "disk" }).call<Record<string, unknown>>(params.resModel, "get_views", [], {
        context: filteredContext,
        views: params.views.map(([id, type]) => [id, type]),
        options: loadViewsOptions
      });
      return normalizeViewDescriptions(result, params.resModel);
    }
  };
}

export function createActionLoader(
  rpc: RPCService,
  options: { actions?: Registry<unknown>; userContext?: Record<string, unknown> } = {}
): ActionLoader {
  const actions = options.actions ?? registries.actions;
  const userContext = { ...(options.userContext ?? {}) };
  return async (actionRequest: ActionRequest, context: Record<string, unknown> = {}) => {
    if (typeof actionRequest === "string" && actions.contains(actionRequest)) {
      return {
        target: "current",
        tag: actionRequest,
        type: "ir.actions.client"
      };
    }
    if (typeof actionRequest === "string" || typeof actionRequest === "number") {
      const ctx = { ...userContext, ...context };
      delete ctx.params;
      const action = await rpc.call<Record<string, unknown>>("/web/action/load", {
        action_id: actionRequest,
        context: ctx
      });
      return { ...action };
    }
    if (isRecord(actionRequest)) {
      return { ...actionRequest };
    }
    throw new Error(`Invalid action request: ${String(actionRequest)}`);
  };
}

export function createActionService(
  executor: ActionExecutor = createClientActionExecutor(),
  loader: ActionLoader = defaultActionLoader
): ActionService {
  const history: ActionInvocation[] = [];
  let current: ActionInvocation | null = null;
  const stack = createActionStack();
  const invocationFromEntry = (entry: ActionStackEntry | null): ActionInvocation | null => {
    return entry
      ? {
          action: { ...entry.action },
          options: { ...entry.options }
        }
      : null;
  };
  return {
    get history(): readonly ActionInvocation[] {
      return history.map((item) => ({
        action: { ...item.action },
        options: { ...item.options }
      }));
    },
    get current(): ActionInvocation | null {
      return current
        ? {
            action: { ...current.action },
            options: { ...current.options }
          }
        : null;
    },
    get stack(): readonly ActionStackEntry[] {
      return stack.entries;
    },
    get currentRoute(): ActionStackEntry["route"] {
      return stack.currentRoute;
    },
    get breadcrumbs(): readonly ActionBreadcrumb[] {
      return stack.breadcrumbs;
    },
    loadAction(action: ActionRequest, context: Record<string, unknown> = {}): Promise<Record<string, unknown>> {
      return loader(action, context);
    },
    async doAction<T = unknown>(action: ActionRequest, options: ActionServiceOptions = {}): Promise<T> {
      const loadedAction = await loader(action, actionOptionsContext(options));
      const invocation = { action: loadedAction, options: { ...options } };
      history.push(invocation);
      let closingEntry: ActionStackEntry | null = null;
      if (isCloseAction(loadedAction)) {
        closingEntry = stack.current;
        current = invocationFromEntry(stack.closeCurrent());
      } else {
        const stackEntry = shouldReplaceLastAction(options)
          ? stack.replace(loadedAction, options)
          : stack.push(loadedAction, options);
        current = invocationFromEntry(stackEntry);
      }
      const result = (await executor(invocation)) as T;
      if (closingEntry) {
        await runActionOnClose(closingEntry);
      } else if (isServerAction(loadedAction) && isWindowActionExecutionResult(result)) {
        stack.closeCurrent();
        current = invocationFromEntry(stack.push(result.action, options));
      } else if (isServerAction(loadedAction) && isCloseActionResult(result)) {
        const serverEntry = stack.current;
        current = invocationFromEntry(stack.closeCurrent());
        if (serverEntry) await runActionOnClose(serverEntry);
      }
      return result;
    },
    closeCurrent(): ActionInvocation | null {
      const closingEntry = stack.current;
      current = invocationFromEntry(stack.closeCurrent());
      if (closingEntry) void runActionOnClose(closingEntry);
      return current
        ? {
            action: { ...current.action },
            options: { ...current.options }
          }
        : null;
    },
    clearStack(): void {
      stack.clear();
      current = null;
    },
    restoreStack(entries: readonly ActionStackEntry[]): ActionInvocation | null {
      current = invocationFromEntry(stack.restore(entries));
      return current
        ? {
            action: { ...current.action },
            options: { ...current.options }
          }
        : null;
    }
  };
}

async function runActionOnClose(entry: ActionStackEntry): Promise<void> {
  const callback = entry.options.onClose;
  if (typeof callback === "function") await callback();
}

function isServerAction(action: Record<string, unknown>): boolean {
  return action.type === "ir.actions.server";
}

function isWindowActionExecutionResult(value: unknown): value is WindowActionResult {
  return isRecord(value)
    && value.type === "ir.actions.act_window"
    && isRecord(value.action);
}

function isCloseActionResult(value: unknown): boolean {
  return isRecord(value) && value.type === "ir.actions.act_window_close";
}

export function createClientActionExecutor(
  actions: Registry<unknown> = registries.actions,
  fallback: ActionExecutor = defaultActionExecutor,
  env: WebClientEnv | null = null
): ActionExecutor {
  return (invocation) => {
    const tag = typeof invocation.action.tag === "string" ? invocation.action.tag : "";
    if (invocation.action.type === "ir.actions.client" && tag && actions.contains(tag)) {
      const handler = actions.get(tag);
      if (typeof handler === "function") {
        return (handler as ClientActionFunctionHandler)(env, invocation.action, invocation.options);
      }
      if (isExecutableClientAction(handler)) {
        return handler.execute(invocation.action, env, invocation.options);
      }
      {
        throw new Error(`Client action "${tag}" is not executable`);
      }
    }
    return fallback(invocation);
  };
}

export function createServerActionExecutor(
  rpc: RPCService,
  fallback: ActionExecutor = defaultActionExecutor,
  returnedActionExecutor: ActionExecutor = fallback
): ActionExecutor {
  return async (invocation) => {
    if (invocation.action.type !== "ir.actions.server") {
      return fallback(invocation);
    }
    const actionID = invocation.action.id;
    if (typeof actionID !== "number" && typeof actionID !== "string") {
      throw new Error("Server action requires id");
    }
    const context = {
      ...(isRecord(invocation.action.context) ? invocation.action.context : {}),
      ...actionOptionsContext(invocation.options)
    };
    const result = await rpc.call<unknown>("/web/action/run", {
      action_id: actionID,
      context
    });
    const nextAction: Record<string, unknown> | null = isRecord(result) && typeof result.type === "string"
      ? { ...result }
      : result === false || result === null || result === undefined
        ? { type: "ir.actions.act_window_close" }
        : null;
    if (nextAction) {
      if (!("path" in nextAction) && typeof invocation.action.path === "string") {
        nextAction["path"] = invocation.action.path;
      }
      return returnedActionExecutor({
        action: nextAction,
        options: { ...invocation.options }
      });
    }
    return result;
  };
}

export function createWindowActionExecutor(
  viewService: ViewService,
  orm: ORMService | null = null,
  fallback: ActionExecutor = defaultActionExecutor
): ActionExecutor {
  return async (invocation) => {
    const action = invocation.action;
    if (action.type !== "ir.actions.act_window") {
      return fallback(invocation);
    }
    const resModel = typeof action.res_model === "string" ? action.res_model : "";
    if (!resModel) throw new Error("Window action requires res_model");
    const views = normalizeActionViews(action);
    const activeView = firstRenderableView(views);
    const context = {
      ...(isRecord(action.context) ? action.context : {}),
      ...actionOptionsContext(invocation.options)
    };
    const viewDescriptions = await viewService.loadViews(
      { resModel, views, context },
      {
        actionId: typeof action.id === "number" ? action.id : false,
        loadActionMenus: action.target !== "new" && resModel !== "res.config.settings",
        loadIrFilters: views.some((view) => view[1] === "search")
      }
    );
    const search = buildWindowActionSearch(action, context, viewDescriptions);
    const data = orm
      ? await loadWindowActionRecords(orm, action, activeView, resModel, context, viewDescriptions, search.state)
      : { records: [], length: 0 };
    return {
      type: "ir.actions.act_window",
      action: { ...action, context },
      activeView,
      resModel,
      viewDescriptions,
      search,
      records: data.records,
      length: data.length
    } satisfies WindowActionResult;
  };
}

export function createSessionService(rpc: RPCService): SessionService {
  let info: Record<string, unknown> | null = null;
  return {
    get info(): Record<string, unknown> | null {
      return info ? { ...info } : null;
    },
    async load(): Promise<Record<string, unknown>> {
      const result = await rpc.call<Record<string, unknown>>("/web/session/get_session_info");
      info = { ...result };
      hydrateSessionGlobals(info);
      return { ...info };
    }
  };
}

export function createDialogService(): DialogService {
  const calls: Array<{ Component: unknown; props: Record<string, unknown> }> = [];
  return {
    calls,
    async add(Component: unknown, props: Record<string, unknown> = {}): Promise<unknown> {
      calls.push({ Component, props: { ...props } });
      if (typeof props.confirm === "function") {
        return props.confirm();
      }
      return undefined;
    }
  };
}

export function createNotificationService(): NotificationService {
  const calls: Array<Record<string, unknown>> = [];
  return {
    calls,
    add(message: string, options: Record<string, unknown> = {}): void {
      calls.push({ message, ...options });
    }
  };
}

export function createWebClientServices(options: WebClientServiceOptions = {}): WebClientServices {
  const rpc = options.rpc ?? createRPCService({ transport: options.transport });
  const orm = createORMService(rpc);
  const view = createViewService(orm);
  const windowExecutor = createWindowActionExecutor(view, orm);
  let executor: ActionExecutor = windowExecutor;
  const serverExecutor = createServerActionExecutor(rpc, windowExecutor, (invocation) => executor(invocation));
  executor = options.actionExecutor ?? createClientActionExecutor(registries.actions, serverExecutor);
  return {
    rpc,
    dataset: createDatasetService(rpc),
    orm,
    view,
    action: createActionService(executor, createActionLoader(rpc)),
    session: createSessionService(rpc),
    notification: createNotificationService(),
    mail: createPortalMailService(rpc, { uploadTransport: options.uploadTransport })
  };
}

serviceRegistry
  .add("rpc", {
    start(env) {
      const transport = isRPCTransport(env.rpcTransport) ? env.rpcTransport : undefined;
      return createRPCService({ transport });
    }
  })
  .add("dataset", {
    dependencies: ["rpc"],
    start(_env, { rpc }) {
      return createDatasetService(rpc as RPCService);
    }
  })
  .add("orm", {
    dependencies: ["rpc"],
    async: [
      "call",
      "create",
      "nameGet",
      "read",
      "webReadGroup",
      "search",
      "searchRead",
      "unlink",
      "webResequence",
      "webSearchRead",
      "write"
    ],
    start(env, { rpc }) {
      const userContext = isRecord(env.userContext) ? env.userContext : undefined;
      return createORMService(rpc as RPCService, { userContext });
    }
  })
  .add("view", {
    dependencies: ["orm"],
    async: ["loadViews"],
    start(env, { orm }) {
      return createViewService(orm as ORMService, env);
    }
  })
  .add("action", {
    dependencies: ["rpc", "orm", "view"],
    start(env) {
      const rpc = env.services.rpc as RPCService;
      const orm = env.services.orm as ORMService;
      const view = env.services.view as ViewService;
      const userContext = isRecord(env.userContext) ? env.userContext : undefined;
      const windowExecutor = createWindowActionExecutor(view, orm);
      let executor: ActionExecutor = windowExecutor;
      const serverExecutor = createServerActionExecutor(rpc, windowExecutor, (invocation) => executor(invocation));
      executor = isActionExecutor(env.actionExecutor) ? env.actionExecutor : createClientActionExecutor(registries.actions, serverExecutor, env);
      return createActionService(executor, createActionLoader(rpc, { userContext }));
    }
  })
  .add("session", {
    dependencies: ["rpc"],
    start(_env, { rpc }) {
      return createSessionService(rpc as RPCService);
    }
  })
  .add("mail", {
    dependencies: ["rpc"],
    async: [
      "chatterInit",
      "chatterFetch",
      "postMessage",
      "updateMessageContent",
      "reactMessage",
      "starredMessages",
      "toggleMessageStarred",
      "unstarAllMessages",
      "uploadAttachment",
      "deleteAttachment"
    ],
    start(env, { rpc }) {
      const uploadTransport = typeof env.portalMailUploadTransport === "function"
        ? env.portalMailUploadTransport as PortalMailUploadTransport
        : undefined;
      return createPortalMailService(rpc as RPCService, { uploadTransport });
    }
  })
  .add("dialog", {
    start() {
      return createDialogService();
    }
  })
  .add("notification", {
    start() {
      return createNotificationService();
    }
  });

export function createWebClient(options: WebClientOptions) {
  return {
    render(): HTMLElement {
      return createWebClientShell({
        theme: options.theme,
        debug: Boolean(options.env.debug),
        menus: options.menus,
        userName: sessionUserName(options.session),
        companyName: sessionCompanyName(options.session),
        systray: options.systray ?? sessionSystrayState(options.session),
        onOpenApp: options.onOpenApp,
        onOpenAppsCatalog: options.onOpenAppsCatalog,
        onSystrayAction: options.onSystrayAction
      });
    }
  };
}

function sessionUserName(sessionInfo: Record<string, unknown> | undefined): string {
  return firstText(sessionInfo?.name, sessionInfo?.display_name, sessionInfo?.username) ?? "Administrator";
}

function sessionCompanyName(sessionInfo: Record<string, unknown> | undefined): string {
  const company = isRecord(sessionInfo?.company_id) ? sessionInfo.company_id : undefined;
  const currentCompany = currentSessionCompany(sessionInfo);
  return firstText(
    sessionInfo?.company_name,
    currentCompany?.name,
    company?.name,
    sessionInfo?.db
  ) ?? "My Company";
}

function sessionSystrayState(sessionInfo: Record<string, unknown> | undefined): NavbarSystrayState {
  return {
    store: isRecord(sessionInfo?.Store) ? sessionInfo.Store : undefined,
    companies: sessionCompanies(sessionInfo),
    currentCompanyId: sessionCurrentCompanyId(sessionInfo),
    displaySwitchCompanyMenu: sessionInfo?.display_switch_company_menu === true
  };
}

function currentSessionCompany(sessionInfo: Record<string, unknown> | undefined): NavbarSystrayCompany | undefined {
  const companies = sessionCompanies(sessionInfo);
  const current = sessionCurrentCompanyId(sessionInfo);
  if (current === undefined) return undefined;
  return companies.find((company) => String(company.id) === String(current));
}

function sessionCurrentCompanyId(sessionInfo: Record<string, unknown> | undefined): number | string | undefined {
  const userCompanies = isRecord(sessionInfo?.user_companies) ? sessionInfo.user_companies : undefined;
  const current = firstValue(userCompanies?.current_company);
  if (typeof current === "number" || typeof current === "string") return current;
  const company = isRecord(sessionInfo?.company_id) ? sessionInfo.company_id : undefined;
  const companyID = firstValue(company?.id) ?? firstValue(sessionInfo?.company_id);
  return typeof companyID === "number" || typeof companyID === "string" ? companyID : undefined;
}

function sessionCompanies(sessionInfo: Record<string, unknown> | undefined): NavbarSystrayCompany[] {
  const userCompanies = isRecord(sessionInfo?.user_companies) ? sessionInfo.user_companies : undefined;
  const allowed = isRecord(userCompanies?.allowed_companies) ? userCompanies.allowed_companies : undefined;
  const current = sessionCurrentCompanyId(sessionInfo);
  const active = sessionActiveCompanyIds(sessionInfo);
  if (!allowed) return [];
  return Object.values(allowed)
    .filter(isRecord)
    .map((company): NavbarSystrayCompany | undefined => {
      const id = firstValue(company.id);
      if (typeof id !== "number" && typeof id !== "string") return undefined;
      return {
        id,
        name: firstText(company.name, `Company ${id}`) ?? `Company ${id}`,
        current: current !== undefined && String(id) === String(current),
        active: active.has(String(id))
      };
    })
    .filter((company): company is NavbarSystrayCompany => company !== undefined)
    .sort((left, right) => String(left.name).localeCompare(String(right.name)));
}

function sessionActiveCompanyIds(sessionInfo: Record<string, unknown> | undefined): Set<string> {
  const context = isRecord(sessionInfo?.user_context) ? sessionInfo.user_context : undefined;
  const ids = Array.isArray(context?.allowed_company_ids) ? context.allowed_company_ids : [];
  return new Set(ids.map((id) => String(id)));
}

export function renderWindowAction(result: WindowActionResult, options: RenderWindowActionOptions = {}): HTMLElement {
  const root = document.createElement("section");
  root.className = "gorp-window-action";
  root.dataset.model = result.resModel;
  root.dataset.view = result.activeView;
  const viewDescription = result.viewDescriptions.views[result.activeView];
  const fields = result.viewDescriptions.fields;
  const records = options.records ?? result.records;
  const values = options.values ?? records?.[0] ?? result.records[0] ?? {};
  const settingsState = settingsActionState(result, values);
  const body = settingsState
    ? renderSettingsActionView(result, viewDescription, fields, settingsState, root)
    : result.activeView === "form"
      ? renderFormView(viewDescription, fields, values, result.resModel, options)
      : result.activeView === "kanban"
        ? renderKanbanView(viewDescription, fields, records, result.resModel, result.action, options)
        : renderListView(viewDescription, fields, records, result.resModel, result.action, options);
  const controlPanel = renderWindowActionControlPanel(result, root, options);
  if (settingsState) appendSettingsActionButtons(controlPanel, root, result, settingsState, options);
  root.append(controlPanel, body);
  return root;
}

export function renderWindowActionDialog(result: WindowActionResult, options: RenderWindowActionDialogOptions = {}): HTMLElement {
  const overlay = document.createElement("section");
  overlay.className = "o_dialog gorp-action-dialog";
  overlay.dataset.target = "new";
  overlay.dataset.model = result.resModel;
  overlay.setAttribute("tabindex", "-1");
  const modal = document.createElement("div");
  modal.className = "modal o_dialog_container show d-block";
  modal.setAttribute("role", "dialog");
  modal.setAttribute("aria-modal", "true");
  const titleID = uniqueId("dialog-title-");
  modal.setAttribute("aria-labelledby", titleID);
  const dialog = document.createElement("div");
  dialog.className = "modal-dialog modal-lg";
  const content = document.createElement("div");
  content.className = "modal-content";
  const header = document.createElement("header");
  header.className = "modal-header";
  const title = document.createElement("h1");
  title.className = "modal-title";
  title.id = titleID;
  title.textContent = options.title || (typeof result.action.name === "string" && result.action.name.trim()) || result.resModel;
  const close = document.createElement("button");
  close.type = "button";
  close.className = "btn-close";
  close.setAttribute("aria-label", "Close");
  close.addEventListener("click", () => {
    overlay.dispatchEvent(new CustomEvent("dialog:close", {
      bubbles: true,
      detail: { model: result.resModel }
    }));
  });
  overlay.addEventListener("keydown", (event) => {
    if ((event as KeyboardEvent).key !== "Escape") return;
    event.preventDefault();
    close.dispatchEvent(new CustomEvent("click"));
  });
  header.append(title, close);
  const body = document.createElement("div");
  body.className = "modal-body o_act_window";
  body.append(renderWindowAction(result, options));
  content.append(header, body);
  dialog.append(content);
  modal.append(dialog);
  overlay.append(modal);
  return overlay;
}

function settingsActionState(result: WindowActionResult, values: Record<string, unknown>): SettingsActionState | null {
  if (result.resModel !== "res.config.settings" || result.activeView !== "form") return null;
  const initialValues = cloneRecord(values);
  return {
    initialValues,
    currentValues: cloneRecord(values),
    dirtyFields: new Set()
  };
}

function renderSettingsActionView(
  result: WindowActionResult,
  viewDescription: ViewDescription | undefined,
  fields: Record<string, unknown>,
  state: SettingsActionState,
  eventRoot: HTMLElement
): HTMLElement {
  const wrapper = document.createElement("section");
  wrapper.className = "gorp-settings-action o_settings_form_view";
  wrapper.dataset.model = result.resModel;
  const render = () => {
    wrapper.replaceChildren(renderSettingsView({
      arch: viewDescription?.arch ?? "",
      fields,
      values: state.currentValues,
      activeApp: settingsActiveApp(result.action)
    }, {
      onFieldChange(name, value) {
        updateSettingsPendingValue(state, name, value);
        eventRoot.dispatchEvent(new CustomEvent("settings:field-change", {
          bubbles: true,
          detail: { name, value, dirty: state.dirtyFields.size > 0 }
        }));
      }
    }));
  };
  state.renderBody = render;
  render();
  return wrapper;
}

function appendSettingsActionButtons(
  controlPanel: HTMLElement,
  eventRoot: HTMLElement,
  result: WindowActionResult,
  state: SettingsActionState,
  options: RenderWindowActionOptions
): void {
  const mainButtons = findDescendantByClass(controlPanel, "o_control_panel_main_buttons");
  if (!mainButtons) return;
  const save = document.createElement("button");
  save.type = "button";
  save.className = "btn btn-primary o_form_button_save";
  save.dataset.settingsAction = "save";
  save.textContent = "Save";
  const discard = document.createElement("button");
  discard.type = "button";
  discard.className = "btn btn-secondary o_form_button_cancel";
  discard.dataset.settingsAction = "discard";
  discard.textContent = "Discard";
  const status = document.createElement("span");
  status.className = "o_settings_dirty_status text-muted";
  state.saveButton = save;
  state.discardButton = discard;
  state.status = status;
  save.addEventListener("click", () => {
    void saveSettingsAction(eventRoot, result, state, options).catch((error) => {
      setSettingsStatus(state, error instanceof Error ? error.message : String(error));
      eventRoot.dispatchEvent(new CustomEvent("settings:save-error", {
        bubbles: true,
        detail: { error }
      }));
    });
  });
  discard.addEventListener("click", () => {
    discardSettingsAction(eventRoot, state);
  });
  mainButtons.append(save, discard, status);
  updateSettingsButtons(state);
}

function updateSettingsPendingValue(state: SettingsActionState, name: string, value: unknown): void {
  state.currentValues = { ...state.currentValues, [name]: value };
  if (sameSettingsValue(state.initialValues[name], value)) {
    state.dirtyFields.delete(name);
  } else {
    state.dirtyFields.add(name);
  }
  setSettingsStatus(state, state.dirtyFields.size ? "Unsaved changes" : "");
  updateSettingsButtons(state);
}

async function saveSettingsAction(
  eventRoot: HTMLElement,
  result: WindowActionResult,
  state: SettingsActionState,
  options: RenderWindowActionOptions
): Promise<void> {
  const changes = settingsChangedValues(state);
  if (!Object.keys(changes).length) return;
  state.saveButton?.setAttribute("aria-busy", "true");
  try {
    const orm = options.services?.orm;
    if (orm) {
      const context = isRecord(result.action.context) ? result.action.context : options.context ?? {};
      const recordID = numberRecordID(state.currentValues.id);
      if (recordID !== undefined) {
        await orm.webSave(result.resModel, [recordID], changes, { context });
      } else {
        const created = await orm.create<unknown>(result.resModel, [{ ...state.currentValues }], { context });
        const createdID = createdRecordID(created);
        if (createdID !== undefined) state.currentValues = { ...state.currentValues, id: createdID };
      }
    }
  } finally {
    state.saveButton?.removeAttribute("aria-busy");
  }
  state.initialValues = cloneRecord(state.currentValues);
  state.dirtyFields.clear();
  setSettingsStatus(state, "Saved");
  updateSettingsButtons(state);
  eventRoot.dispatchEvent(new CustomEvent("settings:save", {
    bubbles: true,
    detail: { model: result.resModel, values: cloneRecord(state.currentValues), changes }
  }));
}

function discardSettingsAction(eventRoot: HTMLElement, state: SettingsActionState): void {
  state.currentValues = cloneRecord(state.initialValues);
  state.dirtyFields.clear();
  state.renderBody?.();
  setSettingsStatus(state, "");
  updateSettingsButtons(state);
  eventRoot.dispatchEvent(new CustomEvent("settings:discard", {
    bubbles: true,
    detail: { values: cloneRecord(state.currentValues) }
  }));
}

function settingsChangedValues(state: SettingsActionState): Record<string, unknown> {
  const changes: Record<string, unknown> = {};
  for (const name of state.dirtyFields) changes[name] = state.currentValues[name];
  return changes;
}

function updateSettingsButtons(state: SettingsActionState): void {
  const dirty = state.dirtyFields.size > 0;
  if (state.saveButton) state.saveButton.disabled = !dirty;
  if (state.discardButton) state.discardButton.disabled = !dirty;
}

function setSettingsStatus(state: SettingsActionState, value: string): void {
  if (state.status) state.status.textContent = value;
}

function sameSettingsValue(left: unknown, right: unknown): boolean {
  return JSON.stringify(left ?? null) === JSON.stringify(right ?? null);
}

function settingsActiveApp(action: Record<string, unknown>): string | undefined {
  const context = isRecord(action.context) ? action.context : {};
  for (const key of ["active_app", "settings_app", "module"]) {
    const value = context[key];
    if (typeof value === "string" && value.trim()) return value.trim();
  }
  return undefined;
}

function cloneRecord(values: Record<string, unknown>): Record<string, unknown> {
  return { ...values };
}

function createdRecordID(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (Array.isArray(value)) {
    for (const item of value) {
      const id = createdRecordID(item);
      if (id !== undefined) return id;
    }
  }
  if (isRecord(value)) return numberRecordID(value.id);
  return undefined;
}

function renderWindowActionControlPanel(result: WindowActionResult, root: HTMLElement, options: RenderWindowActionOptions): HTMLElement {
  const views = normalizeActionViews(result.action)
    .filter((view) => view[1] !== "search")
    .map<ActionControlPanelView>((view) => ({
      type: view[1],
      label: viewLabel(view[1]),
      active: view[1] === result.activeView
    }));
  const controlPanel = renderActionControlPanel({
    title: typeof result.action.name === "string" ? result.action.name : result.resModel,
    pager: result.activeView === "form"
      ? undefined
      : { offset: 0, limit: numberActionValue(result.action.limit, 80), total: result.length },
    views,
    search: {
      query: result.search?.state.query ?? "",
      facets: result.search?.state.facets ?? []
    },
    filters: result.search?.filters ?? [],
    groupBys: result.search?.groupBys ?? [],
    favorites: result.search?.favorites ?? []
  }, {
    onViewSwitch: (viewType) => {
      if (options.services?.action && viewType !== result.activeView) {
        void options.services.action.doAction(actionWithViewType(result.action, viewType), replaceActionOptions(options));
        return;
      }
      root.dispatchEvent(new CustomEvent("action:view-switch", {
        bubbles: true,
        detail: { viewType, model: result.resModel }
      }));
    },
    onSearch: (query) => {
      if (rerunActionWithSearchQuery(result, query, options)) return;
      root.dispatchEvent(new CustomEvent("action:search", {
        bubbles: true,
        detail: { query, model: result.resModel }
      }));
    },
    onFilter: (item) => {
      if (rerunActionWithSearchMenuItem(result, item, "toggle", options)) return;
      dispatchSearchMenuEvent(root, "action:search-filter", result, item);
    },
    onGroupBy: (item) => {
      if (rerunActionWithSearchMenuItem(result, item, "toggle", options)) return;
      dispatchSearchMenuEvent(root, "action:search-group-by", result, item);
    },
    onFavorite: (item) => {
      if (rerunActionWithSearchMenuItem(result, item, "replace", options)) return;
      dispatchSearchMenuEvent(root, "action:search-favorite", result, item);
    },
    onFacetRemove: (facet) => {
      if (rerunActionWithFacets(result, withoutSearchFacet(result.search?.state.facets ?? [], facet.id), options)) return;
      root.dispatchEvent(new CustomEvent("action:search-facet-remove", {
        bubbles: true,
        detail: { facet, model: result.resModel }
      }));
    },
    onAddCustomFilter: () => dispatchSearchUtilityEvent(root, "action:search-custom-filter", result),
    onAddCustomGroup: () => dispatchSearchUtilityEvent(root, "action:search-custom-group", result),
    onAddFavorite: () => {
      if (persistCurrentSearchFavorite(result, root, options)) return;
      dispatchSearchUtilityEvent(root, "action:search-add-favorite", result);
    }
  });
  const createButton = renderWindowActionCreateButton(result, root, options);
  const mainButtons = findDescendantByClass(controlPanel, "o_control_panel_main_buttons");
  if (createButton && mainButtons) mainButtons.append(createButton);
  return controlPanel;
}

function actionWithViewType(action: Record<string, unknown>, viewType: string): Record<string, unknown> {
  const cleanViewType = viewType.trim();
  const views = normalizeActionViews(action);
  const target = views.find((view) => view[1] === cleanViewType) ?? [false, cleanViewType] as ViewRef;
  const reordered = [target, ...views.filter((view) => view[1] !== cleanViewType)];
  const next: Record<string, unknown> = {
    ...action,
    view_mode: reordered.filter((view) => view[1] !== "search").map((view) => view[1]).join(","),
    views: reordered,
    view_type: cleanViewType
  };
  if (cleanViewType !== "form") delete next.res_id;
  return next;
}

function rerunActionWithSearchQuery(result: WindowActionResult, query: string, options: RenderWindowActionOptions): boolean {
  if (!options.services?.action) return false;
  const nextAction = actionWithCurrentSearch(result, result.search?.state.facets ?? []);
  nextAction.__search_query = String(query ?? "").trim();
  void options.services.action.doAction(nextAction, replaceActionOptions(options));
  return true;
}

function rerunActionWithSearchMenuItem(
  result: WindowActionResult,
  item: ActionControlPanelMenuItem,
  mode: "toggle" | "replace",
  options: RenderWindowActionOptions
): boolean {
  if (!item.facet || !options.services?.action) return false;
  const currentFacets = result.search?.state.facets ?? [];
  const nextFacets = mode === "replace"
    ? [cloneSearchFacet(item.facet)]
    : toggleSearchFacet(currentFacets, item.facet);
  return rerunActionWithFacets(result, nextFacets, options);
}

function rerunActionWithFacets(result: WindowActionResult, facets: readonly SearchFacet[], options: RenderWindowActionOptions): boolean {
  if (!options.services?.action) return false;
  void options.services.action.doAction(actionWithCurrentSearch(result, facets), replaceActionOptions(options));
  return true;
}

function persistCurrentSearchFavorite(result: WindowActionResult, root: HTMLElement, options: RenderWindowActionOptions): boolean {
  const orm = options.services?.orm;
  if (!orm) return false;
  const state = result.search?.state;
  if (!state) return false;
  const values: Record<string, unknown> = {
    name: currentSearchFavoriteName(result),
    model_id: result.resModel,
    domain: JSON.stringify(state.domain ?? []),
    context: JSON.stringify(currentSearchFavoriteContext(state)),
    sort: "[]",
    active: true,
    is_default: false
  };
  const actionID = numericActionID(result.action.id);
  if (actionID !== undefined) values.action_id = actionID;
  if (user.userId > 0) values.user_id = user.userId;
  void orm.create("ir.filters", [values]).then(() => {
    options.services?.notification?.add("Favorite saved", { type: "success" });
    if (options.services?.action) {
      return options.services.action.doAction(actionWithCurrentSearch(result, state.facets), replaceActionOptions(options));
    }
    root.dispatchEvent(new CustomEvent("action:search-favorite-saved", {
      bubbles: true,
      detail: { model: result.resModel, values }
    }));
    return undefined;
  }).catch((error) => {
    options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
    root.dispatchEvent(new CustomEvent("action:search-favorite-error", {
      bubbles: true,
      detail: { model: result.resModel, error }
    }));
  });
  return true;
}

function actionWithCurrentSearch(result: WindowActionResult, facets: readonly SearchFacet[]): Record<string, unknown> {
  const nextAction: Record<string, unknown> = {
    ...result.action,
    __search_facets: facets.map(cloneSearchFacet)
  };
  const query = String(result.search?.state.query ?? "").trim();
  if (query) nextAction.__search_query = query;
  else delete nextAction.__search_query;
  return nextAction;
}

function currentSearchFavoriteName(result: WindowActionResult): string {
  const query = String(result.search?.state.query ?? "").trim();
  if (query) return query;
  const facets = result.search?.state.facets ?? [];
  const labels = facets.map((facet) => searchFacetLabelValue(facet)).filter(Boolean);
  if (labels.length) return labels.join(", ");
  return "Current Search";
}

function searchFacetLabelValue(facet: SearchFacet): string {
  const labels = facet.valueLabels?.length ? facet.valueLabels : [facet.label];
  return labels.map((item) => String(item ?? "").trim()).filter(Boolean).join(" or ");
}

function currentSearchFavoriteContext(state: SearchModelState): Record<string, unknown> {
  const context = { ...state.context };
  if (state.groupBy.length) context.group_by = [...state.groupBy];
  return context;
}

function numericActionID(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;
}

function replaceActionOptions(options: RenderWindowActionOptions): ActionServiceOptions {
  return {
    additionalContext: { ...(options.context ?? {}) },
    replaceLastAction: true
  };
}

function toggleSearchFacet(currentFacets: readonly SearchFacet[], facet: SearchFacet): SearchFacet[] {
  if (currentFacets.some((item) => item.id === facet.id)) return withoutSearchFacet(currentFacets, facet.id);
  return [...currentFacets.map(cloneSearchFacet), cloneSearchFacet(facet)];
}

function withoutSearchFacet(facets: readonly SearchFacet[], id: string): SearchFacet[] {
  return facets.filter((facet) => facet.id !== id).map(cloneSearchFacet);
}

function cloneSearchFacet(facet: SearchFacet): SearchFacet {
  return {
    ...facet,
    domain: facet.domain ? [...facet.domain] : undefined,
    context: facet.context ? { ...facet.context } : undefined,
    groupBy: facet.groupBy ? [...facet.groupBy] : undefined,
    valueLabels: facet.valueLabels ? [...facet.valueLabels] : undefined
  };
}

function dispatchSearchMenuEvent(root: HTMLElement, type: string, result: WindowActionResult, item: ActionControlPanelMenuItem): void {
  root.dispatchEvent(new CustomEvent(type, {
    bubbles: true,
    detail: { item, model: result.resModel }
  }));
}

function dispatchSearchUtilityEvent(root: HTMLElement, type: string, result: WindowActionResult): void {
  root.dispatchEvent(new CustomEvent(type, {
    bubbles: true,
    detail: { model: result.resModel }
  }));
}

function viewLabel(viewType: string): string {
  if (viewType === "list") return "List";
  if (viewType === "kanban") return "Kanban";
  if (viewType === "form") return "Form";
  if (viewType === "calendar") return "Calendar";
  if (viewType === "pivot") return "Pivot";
  if (viewType === "graph") return "Graph";
  return viewType;
}

function findDescendantByClass(root: HTMLElement, className: string): HTMLElement | null {
  if (classNameIncludes(root.className, className)) return root;
  for (const child of Array.from(root.children)) {
    const found = findDescendantByClass(child as HTMLElement, className);
    if (found) return found;
  }
  return null;
}

function classNameIncludes(className: string, target: string): boolean {
  return className.split(/\s+/).includes(target);
}

function renderWindowActionCreateButton(result: WindowActionResult, root: HTMLElement, options: RenderWindowActionOptions): HTMLElement | null {
  if (result.activeView === "form" || actionCreateDisabled(result.action)) return null;
  const formView = formViewRef(result.action);
  if (!formView) return null;
  const button = document.createElement("button");
  button.type = "button";
  button.className = result.activeView === "kanban" ? "btn btn-primary o-kanban-button-new" : "btn btn-primary o_list_button_add";
  button.dataset.createAction = "true";
  if (result.activeView === "kanban") button.setAttribute("accesskey", "c");
  button.textContent = "New";
  button.addEventListener("click", async () => {
    const action = createFormAction(result.action, formView);
    if (options.services?.action) {
      await options.services.action.doAction(action, {
        additionalContext: { ...(options.context ?? {}) },
        replaceLastAction: true
      });
      return;
    }
    root.dispatchEvent(new CustomEvent("action:create", {
      bubbles: true,
      detail: { action, model: result.resModel }
    }));
  });
  return button;
}

function actionCreateDisabled(action: Record<string, unknown>): boolean {
  const context = isRecord(action.context) ? action.context : {};
  return context.create === false;
}

function formViewRef(action: Record<string, unknown>): ViewRef | null {
  for (const ref of normalizeActionViews(action)) {
    if (ref[1] === "form") return ref;
  }
  return null;
}

function createFormAction(action: Record<string, unknown>, formView: ViewRef): Record<string, unknown> {
  const nextAction: Record<string, unknown> = {
    ...action,
    view_mode: "form",
    views: [[formView[0], "form"]]
  };
  delete nextAction.res_id;
  return nextAction;
}

function renderListView(
  viewDescription: ViewDescription | undefined,
  fields: Record<string, unknown>,
  records: readonly Record<string, unknown>[],
  model?: string,
  action: Record<string, unknown> = {},
  options: RenderWindowActionOptions = {}
): HTMLElement {
  const arch = viewDescription?.arch ?? "";
  const listAttrs = viewRootAttrs(arch, "list", "tree");
  const showApproveAll = listShowsActionApproveAll(arch);
  const activeFieldNames = new Set(parseViewFieldNodes(arch).map((node) => node.name));
  const showUpdateStatus = Boolean(model && user.isSystem && activeFieldNames.has("state"));
  const showApprovalLog = Boolean(model && workflowFieldAvailable(fields, "user_can_approve") && !workflowFieldRelated(fields.user_can_approve));
  const showStaticActions = Boolean(model && activeFieldNameForView(activeFieldNames, fields));
  const showToolbar = showApproveAll || showUpdateStatus || showApprovalLog || showStaticActions || actionMenusHaveItems(viewDescription?.actionMenus);
  const selectedIds = new Set<number>();
  const shell = document.createElement("section");
  shell.className = "gorp-list-shell o-list-view";
  if (model) shell.dataset.model = model;
  const table = document.createElement("table");
  table.className = "gorp-list-view o_list_renderer o_list_table";
  const fieldNodes = listViewFieldNodes(arch, fields, records[0] ?? {});
  const names = fieldNodes.map((node) => node.name);
  const thead = document.createElement("thead");
  const headerRow = document.createElement("tr");
  if (showToolbar) {
    const selectHead = document.createElement("th");
    selectHead.textContent = "";
    headerRow.append(selectHead);
  }
  for (const node of fieldNodes) {
    const th = document.createElement("th");
    th.textContent = fieldLabel(fields, node.name);
    headerRow.append(th);
  }
  thead.append(headerRow);
  const tbody = document.createElement("tbody");
  for (const record of records) {
    const row = document.createElement("tr");
    const recordID = numberRecordID(record.id);
    row.className = listDecorationClassName(listAttrs, record);
    if (model && recordID !== undefined) {
      row.className = row.className ? `${row.className} o_data_row` : "o_data_row";
      row.dataset.id = String(recordID);
      row.dataset.model = model;
      row.setAttribute("role", "link");
      row.setAttribute("tabindex", "0");
      row.addEventListener("click", async (event) => {
        if (listRowClickIgnored(event)) return;
        await openListRecord(model, recordID, action, options, table);
      });
      row.addEventListener("keydown", async (event) => {
        if (event.key !== "Enter") return;
        await openListRecord(model, recordID, action, options, table);
      });
    }
    if (showToolbar) {
      const selectCell = document.createElement("td");
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      if (recordID !== undefined) {
        checkbox.dataset.recordId = String(recordID);
      } else {
        checkbox.disabled = true;
      }
      checkbox.addEventListener("click", (event) => event.stopPropagation());
      checkbox.addEventListener("change", () => {
        if (recordID === undefined) return;
        if (checkbox.checked) selectedIds.add(recordID);
        else selectedIds.delete(recordID);
        updateListToolbarButtons(shell, selectedIds);
      });
      selectCell.append(checkbox);
      row.append(selectCell);
    }
    for (const node of fieldNodes) {
      const cell = document.createElement("td");
      cell.dataset.field = node.name;
      cell.append(renderReadonlyFieldValue(node, fields[node.name], record[node.name], record));
      row.append(cell);
    }
    tbody.append(row);
  }
  table.append(thead, tbody);
  if (showToolbar) {
    const workflowButtons: HTMLElement[] = [];
    if (showUpdateStatus) workflowButtons.push(renderUpdateStatusListButton(model ?? "", selectedIds, shell, options));
    if (showApproveAll) workflowButtons.push(renderApproveAllListButton(model, selectedIds, shell, options));
    if (showApprovalLog) workflowButtons.push(renderApprovalLogListButton(model ?? "", selectedIds, shell, options));
    const staticButtons = showStaticActions && model ? renderListStaticActionButtons(model, selectedIds, shell, options, names, fields) : [];
    const toolbar = renderActionMenus({
      className: "gorp-list-toolbar",
      model: model ?? "",
      actionMenus: viewDescription?.actionMenus,
      staticActionButtons: [...staticButtons, ...workflowButtons],
      getActiveIds: () => Array.from(selectedIds),
      requiresSelection: true,
      root: shell,
      options
    });
    shell.append(toolbar, table, renderMobileListCards(fieldNodes, fields, records, model, action, options));
    return shell;
  }
  shell.append(table, renderMobileListCards(fieldNodes, fields, records, model, action, options));
  return shell;
}

function renderMobileListCards(
  fieldNodes: readonly ViewFieldNode[],
  fields: Record<string, unknown>,
  records: readonly Record<string, unknown>[],
  model: string | undefined,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions
): HTMLElement {
  const cards = document.createElement("div");
  cards.className = "o_mobile_list_cards";
  for (const record of records) {
    const card = document.createElement("article");
    card.className = "o_mobile_record_card";
    const recordID = numberRecordID(record.id);
    if (recordID !== undefined) card.dataset.id = String(recordID);
    if (model) card.dataset.model = model;
    for (const node of fieldNodes) {
      const line = document.createElement("div");
      line.className = "o_mobile_record_line";
      line.dataset.field = node.name;
      const label = document.createElement("span");
      label.className = "o_mobile_record_label";
      label.textContent = fieldLabel(fields, node.name);
      const value = document.createElement("span");
      value.className = "o_mobile_record_value";
      value.append(renderReadonlyFieldValue(node, fields[node.name], record[node.name], record));
      line.append(label, value);
      card.append(line);
    }
    if (model && recordID !== undefined) {
      const openButton = document.createElement("button");
      openButton.type = "button";
      openButton.className = "btn btn-secondary o_mobile_record_open";
      openButton.dataset.recordId = String(recordID);
      openButton.textContent = "Open";
      openButton.addEventListener("click", async () => {
        await openListRecord(model, recordID, action, options, cards);
      });
      card.append(openButton);
    }
    cards.append(card);
  }
  return cards;
}

function listRowClickIgnored(event: Event): boolean {
  const target = event.target;
  if (!target || typeof (target as { closest?: unknown }).closest !== "function") return false;
  return Boolean((target as Element).closest("button, input, select, textarea, a, [role='button']"));
}

function renderKanbanView(
  viewDescription: ViewDescription | undefined,
  fields: Record<string, unknown>,
  records: readonly Record<string, unknown>[],
  model: string,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions = {}
): HTMLElement {
  const arch = viewDescription?.arch ?? "";
  const fieldNodes = kanbanViewFieldNodes(arch, fields, records[0] ?? {});
  const titleField = kanbanTitleField(fieldNodes, fields);
  const renderer = document.createElement("div");
  renderer.className = "o_kanban_renderer o_renderer o_kanban_ungrouped";
  renderer.dataset.model = model;
  if (!records.length) {
    const empty = document.createElement("div");
    empty.className = "o_view_nocontent";
    empty.textContent = "No records";
    renderer.append(empty);
    return renderer;
  }
  for (const record of records) {
    const card = document.createElement("article");
    card.className = "o_kanban_record oe_kanban_global_click o_kanban_global_click d-flex cursor-pointer o_record_selection_available";
    card.setAttribute("role", "link");
    card.setAttribute("tabindex", "0");
    const recordID = numberRecordID(record.id);
    if (recordID !== undefined) card.dataset.id = String(recordID);
    card.dataset.model = model;
    card.addEventListener("click", async () => {
      if (recordID === undefined) return;
      await openKanbanRecord(model, recordID, action, options, renderer);
    });
    const main = document.createElement("div");
    main.className = "oe_kanban_details";
    const title = document.createElement("strong");
    title.className = "o_kanban_record_title";
    title.textContent = fieldDisplayText(fields[titleField], record[titleField] ?? record.display_name ?? record.name ?? record.id);
    main.append(title);
    for (const node of fieldNodes) {
      if (node.name === titleField || node.name === "id") continue;
      const line = document.createElement("div");
      line.className = "o_kanban_record_field";
      line.dataset.field = node.name;
      const label = document.createElement("span");
      label.className = "o_kanban_field_label";
      label.textContent = fieldLabel(fields, node.name);
      const value = document.createElement("span");
      value.className = "o_kanban_field_value";
      value.append(renderReadonlyFieldValue(node, fields[node.name], record[node.name], record));
      line.append(label, value);
      main.append(line);
    }
    card.append(main);
    renderer.append(card);
  }
  return renderer;
}

function kanbanViewFieldNodes(arch: string, fields: Record<string, unknown>, evalContext: Record<string, unknown>): ViewFieldNode[] {
  const nodes = parseViewFieldNodes(arch).filter((node) => !fieldInvisible(node, evalContext));
  if (nodes.length) return nodes;
  const preferred = ["display_name", "name", ...Object.keys(fields)];
  return preferred
    .filter((name, index, list) => name !== "id" && Boolean(fields[name]) && list.indexOf(name) === index)
    .slice(0, 6)
    .map((name) => ({ name, attrs: {}, children: [], childViewAttrs: {} }));
}

function kanbanTitleField(nodes: readonly ViewFieldNode[], fields: Record<string, unknown>): string {
  if (nodes.some((node) => node.name === "display_name")) return "display_name";
  if (nodes.some((node) => node.name === "name")) return "name";
  return nodes[0]?.name ?? (fields.display_name ? "display_name" : "name");
}

async function openKanbanRecord(
  model: string,
  id: number,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): Promise<void> {
  await openRecordAction(model, id, action, options, root);
}

async function openListRecord(
  model: string,
  id: number,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): Promise<void> {
  await openRecordAction(model, id, action, options, root);
}

async function openRecordAction(
  model: string,
  id: number,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): Promise<void> {
  const formView = formViewRef(action) ?? [false, "form"];
  const nextAction: Record<string, unknown> = {
    ...action,
    res_id: id,
    res_model: model,
    view_mode: "form",
    views: [[formView[0], "form"]]
  };
  if (options.services?.action) {
    await options.services.action.doAction(nextAction, {
      additionalContext: { ...(options.context ?? {}) },
      replaceLastAction: true
    });
    return;
  }
  root.dispatchEvent(new CustomEvent("action:open-record", {
    bubbles: true,
    detail: { action: nextAction, model, id }
  }));
}

function listViewFieldNodes(arch: string, fields: Record<string, unknown>, evalContext: Record<string, unknown>): ViewFieldNode[] {
  const nodes = parseViewFieldNodes(arch).filter((node) => !fieldInvisible(node, evalContext));
  if (nodes.length) return nodes;
  return Object.keys(fields)
    .filter((name) => name !== "id" && name !== "display_name")
    .slice(0, 6)
    .map((name) => ({ name, attrs: {}, children: [], childViewAttrs: {} }));
}

function renderActionMenus(params: {
  className: string;
  model: string;
  actionMenus: Record<string, unknown> | undefined;
  staticActionButtons: readonly HTMLElement[];
  getActiveIds: () => number[];
  requiresSelection: boolean;
  root: HTMLElement;
  options: RenderWindowActionOptions;
}): HTMLElement {
  const toolbar = document.createElement("div");
  toolbar.className = `${params.className} gorp-action-menus o_cp_action_menus`;
  toolbar.setAttribute("role", "toolbar");
  toolbar.dataset.model = params.model;
  const printItems = actionMenuRecords(params.actionMenus, "print");
  if (printItems.length) {
    toolbar.append(renderPrintActionMenuSection(
      params.model,
      printItems,
      params.getActiveIds,
      params.requiresSelection,
      params.root,
      params.options
    ));
  }
  const actionButtons = [
    ...params.staticActionButtons,
    ...actionMenuRecords(params.actionMenus, "action").map((item) =>
      renderServerActionMenuButton("action", params.model, item, params.getActiveIds, params.requiresSelection, params.root, params.options)
    )
  ].sort(compareActionMenuButtons);
  if (actionButtons.length) {
    toolbar.append(renderActionMenuSection("action", "Actions", "fa fa-cog", actionButtons));
  }
  return toolbar;
}

function renderPrintActionMenuSection(
  model: string,
  printItems: readonly ActionMenuRecord[],
  getActiveIds: () => number[],
  requiresSelection: boolean,
  root: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement {
  const section = renderActionMenuSection("print", "Print", "fa fa-print", []);
  const toggle = section.children[0] as HTMLElement;
  const menu = section.children[1] as HTMLElement;
  toggle.addEventListener("click", async () => {
    const activeIds = await actionMenuActiveIds(model, getActiveIds(), options);
    const items = await availablePrintActionButtons(model, printItems, activeIds, getActiveIds, requiresSelection, root, options);
    clearElementChildren(menu);
    appendActionMenuItems(menu, items.length ? items : [renderNoPrintReportsItem()]);
    root.dispatchEvent(new CustomEvent("action-menu:print-loaded", {
      bubbles: true,
      detail: {
        model,
        ids: activeIds,
        availableIds: items.map((item) => item.dataset.actionId).filter(Boolean)
      }
    }));
  });
  return section;
}

function renderActionMenuSection(kind: string, title: string, iconClass: string, items: readonly HTMLElement[]): HTMLElement {
  const section = document.createElement("div");
  section.className = "gorp-action-menu-section";
  section.dataset.menu = kind;
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "gorp-action-menu-toggle";
  toggle.dataset.actionMenuToggle = kind;
  toggle.setAttribute("aria-haspopup", "menu");
  toggle.textContent = title;
  const icon = document.createElement("i");
  icon.className = iconClass;
  icon.setAttribute("aria-hidden", "true");
  toggle.append(icon);
  const menu = document.createElement("div");
  menu.className = "gorp-action-menu-items";
  menu.dataset.actionMenuItems = kind;
  menu.setAttribute("role", "menu");
  appendActionMenuItems(menu, items);
  section.append(toggle, menu);
  return section;
}

function appendActionMenuItems(menu: HTMLElement, items: readonly HTMLElement[]): void {
  let previousGroup: string | undefined;
  for (const item of items) {
    const group = item.dataset.groupNumber ?? "100";
    if (previousGroup !== undefined && previousGroup !== group) {
      const separator = document.createElement("div");
      separator.className = "gorp-action-menu-separator";
      separator.setAttribute("role", "separator");
      menu.append(separator);
    }
    item.setAttribute("role", "menuitem");
    menu.append(item);
    previousGroup = group;
  }
}

function clearElementChildren(node: HTMLElement): void {
  while (node.firstChild) node.removeChild(node.firstChild);
  const testChildren = (node as unknown as { children?: unknown[] }).children;
  if (Array.isArray(testChildren)) {
    testChildren.length = 0;
  }
}

async function availablePrintActionButtons(
  model: string,
  printItems: readonly ActionMenuRecord[],
  activeIds: readonly number[],
  getActiveIds: () => number[],
  requiresSelection: boolean,
  root: HTMLElement,
  options: RenderWindowActionOptions
): Promise<HTMLElement[]> {
  const validIDs: unknown[] = [];
  const domainIDs: unknown[] = [];
  for (const item of printItems) {
    if ("domain" in item) {
      domainIDs.push(item.id);
    } else {
      validIDs.push(item.id);
    }
  }
  if (domainIDs.length && options.services?.orm) {
    const validated = await options.services.orm.call<unknown[]>(
      "ir.actions.report",
      "get_valid_action_reports",
      [domainIDs, model, [...activeIds]]
    );
    validIDs.push(...validated);
  } else if (domainIDs.length) {
    validIDs.push(...domainIDs);
  }
  const hasSelection = activeIds.length > 0 || getActiveIds().length > 0 || Boolean(options.isDomainSelected);
  return printItems
    .filter((item) => validIDs.some((id) => actionIDMatches(id, item.id)))
    .map((item) => {
      const button = renderServerActionMenuButton("print", model, item, getActiveIds, requiresSelection, root, options, false);
      if (button.dataset.requiresSelection === "true") {
        (button as HTMLButtonElement).disabled = !hasSelection;
      }
      return button;
    });
}

function renderNoPrintReportsItem(): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "gorp-action-menu-item o_menu_item disabled";
  button.dataset.actionMenuEmpty = "print";
  button.dataset.groupNumber = "100";
  button.disabled = true;
  button.textContent = "No report available.";
  button.setAttribute("role", "menuitem");
  return button;
}

function renderListStaticActionButtons(
  model: string,
  selectedIds: ReadonlySet<number>,
  shell: HTMLElement,
  options: RenderWindowActionOptions,
  exportFields: readonly string[],
  fields: Record<string, unknown>
): HTMLElement[] {
  return [
    renderStaticActionMenuButton("export", "Export", "fa fa-upload", 10, async () => {
      const ids = Array.from(selectedIds);
      if (!ids.length) return;
      await openExportDataDialog(model, ids, exportFields, fields, shell, options);
    }, { requiresSelection: true }),
    renderStaticActionMenuButton("duplicate", "Duplicate", "fa fa-clone", 30, async () => {
      const ids = Array.from(selectedIds);
      if (!ids.length) return;
      const newIds = await options.services?.orm?.call(model, "copy", [ids, {}]);
      await options.onRefresh?.();
      shell.dispatchEvent(new CustomEvent("action-menu:duplicate", {
        bubbles: true,
        detail: { model, ids, newIds }
      }));
    }, { requiresSelection: true }),
    renderStaticActionMenuButton("archive", "Archive", "oi oi-archive", 40, async () => {
      const ids = Array.from(selectedIds);
      if (!ids.length) return;
      const accepted = await confirmStaticAction(options, "Are you sure that you want to archive these records?");
      if (!accepted) return;
      await options.services?.orm?.call(model, "action_archive", [ids]);
      await options.onRefresh?.();
      shell.dispatchEvent(new CustomEvent("action-menu:archive", {
        bubbles: true,
        detail: { model, ids }
      }));
    }, { requiresSelection: true }),
    renderStaticActionMenuButton("unarchive", "Unarchive", "oi oi-unarchive", 45, async () => {
      const ids = Array.from(selectedIds);
      if (!ids.length) return;
      await options.services?.orm?.call(model, "action_unarchive", [ids]);
      await options.onRefresh?.();
      shell.dispatchEvent(new CustomEvent("action-menu:unarchive", {
        bubbles: true,
        detail: { model, ids }
      }));
    }, { requiresSelection: true }),
    renderStaticActionMenuButton("delete", "Delete", "fa fa-trash-o", 50, async () => {
      const ids = Array.from(selectedIds);
      if (!ids.length) return;
      const accepted = await confirmStaticAction(options, "Are you sure you want to delete these records?");
      if (!accepted) return;
      await options.services?.orm?.unlink(model, ids);
      await options.onRefresh?.();
      shell.dispatchEvent(new CustomEvent("action-menu:delete", {
        bubbles: true,
        detail: { model, ids }
      }));
    }, { requiresSelection: true, className: "text-danger" })
  ];
}

interface ExportDialogField {
  name: string;
  value: string;
  label: string;
  type: string;
  relation?: string;
  relationField?: string;
  children: boolean;
  params?: Record<string, unknown>;
  defaultExport?: boolean;
  defaultExportCompatible?: boolean;
}

interface ExportDialogState {
  importCompat: boolean;
  format: string;
  selected: ExportDialogField[];
  availableFields: ExportDialogField[];
  templates: ExportTemplateRecord[];
  templateID: number | "new_template" | null;
  isEditingTemplate: boolean;
  renderTemplateControls?: () => void;
  expandedFieldIds: Set<string>;
  expandedFields: Map<string, ExportDialogField[]>;
  searchTerm: string;
  searchExpandedFieldIds: Set<string>;
  isSmall: boolean;
}

interface ExportTemplateRecord {
  id: number;
  name: string;
  exportFields: number[];
}

async function openExportDataDialog(
  model: string,
  ids: readonly number[],
  defaultFieldNames: readonly string[],
  fields: Record<string, unknown>,
  shell: HTMLElement,
  options: RenderWindowActionOptions
): Promise<void> {
  const dialog = document.createElement("section");
  dialog.className = "gorp-export-dialog o_export_data_dialog";
  dialog.dataset.exportDialog = model;
  const availableFields = await exportDialogAvailableFields(model, fields, false, options);
  const selectedFields = await exportDialogDefaultFields(model, defaultFieldNames, fields, availableFields, false, options);
  const state: ExportDialogState = {
    importCompat: false,
    format: "xlsx",
    selected: selectedFields,
    availableFields,
    templates: await loadExportTemplates(model, options),
    templateID: null,
    isEditingTemplate: false,
    expandedFieldIds: new Set<string>(),
    expandedFields: new Map<string, ExportDialogField[]>(),
    searchTerm: "",
    searchExpandedFieldIds: new Set<string>(),
    isSmall: exportDialogIsSmall(options)
  };

  const title = document.createElement("h3");
  title.textContent = "Export Data";
  const importLabel = document.createElement("label");
  importLabel.className = "o_import_compat";
  const importCheckbox = document.createElement("input");
  importCheckbox.type = "checkbox";
  importCheckbox.dataset.exportImportCompat = "true";
  importCheckbox.addEventListener("change", async () => {
    state.importCompat = importCheckbox.checked;
    dialog.dataset.importCompat = String(state.importCompat);
    exportButton.disabled = true;
    state.expandedFieldIds.clear();
    state.expandedFields.clear();
    state.searchTerm = "";
    state.searchExpandedFieldIds.clear();
    searchInput.value = "";
    state.availableFields = await exportDialogAvailableFields(model, fields, state.importCompat, options);
    const template = typeof state.templateID === "number"
      ? state.templates.find((item) => item.id === state.templateID)
      : undefined;
    state.selected = template
      ? await loadExportTemplateFields(model, template, state, fields, options)
      : await exportDialogDefaultFields(model, defaultFieldNames, fields, state.availableFields, state.importCompat, options);
    if (typeof state.templateID === "number") {
      state.isEditingTemplate = false;
    }
    renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fields, options);
    renderExportDialogSelectedFields(selectedList, state, availableList, fields, options);
    renderTemplateControls();
    exportButton.disabled = false;
  });
  importLabel.append(importCheckbox, textNode(" I want to update data (import-compatible export)"));

  const formatSelect = document.createElement("select");
  formatSelect.dataset.exportFormat = "true";
  for (const format of [{ tag: "xlsx", label: "XLSX" }, { tag: "csv", label: "CSV" }]) {
    const option = document.createElement("option");
    option.value = format.tag;
    option.textContent = format.label;
    formatSelect.append(option);
  }
  formatSelect.value = state.format;
  formatSelect.addEventListener("change", () => {
    state.format = formatSelect.value || "xlsx";
  });

  const templateSelect = document.createElement("select");
  templateSelect.dataset.exportTemplateSelect = "true";
  templateSelect.addEventListener("change", async () => {
    if (templateSelect.value === "new_template") {
      state.templateID = "new_template";
      state.isEditingTemplate = true;
      saveInput.value = "";
      renderTemplateControls();
      return;
    }
    const templateID = Number(templateSelect.value);
    if (!templateID) {
      state.templateID = null;
      state.isEditingTemplate = false;
      renderTemplateControls();
      return;
    }
    const template = state.templates.find((item) => item.id === templateID);
    if (!template) return;
    state.templateID = template.id;
    state.isEditingTemplate = false;
    state.selected = await loadExportTemplateFields(model, template, state, fields, options);
    renderExportDialogSelectedFields(selectedList, state, availableList, fields, options);
    renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fields, options);
    renderTemplateControls();
  });

  const saveInput = document.createElement("input");
  saveInput.dataset.exportTemplateName = "true";
  saveInput.className = "o_save_list_name";
  saveInput.placeholder = "New template";
  const saveButton = document.createElement("button");
  saveButton.type = "button";
  saveButton.dataset.exportTemplateSave = "true";
  saveButton.className = "o_save_list_btn";
  saveButton.textContent = "Save";
  saveButton.addEventListener("click", async () => {
    if (!(state.isEditingTemplate && state.templateID === "new_template")) return;
    const name = saveInput.value.trim();
    if (!name) {
      notifyExportDialog(options, "Please enter save field list name", "danger");
      return;
    }
    if (!options.services?.orm) return;
    const created = await options.services.orm.create<unknown>("ir.exports", [{
      name,
      resource: model,
      export_fields: state.selected.map((field) => [0, 0, { name: field.name }])
    }], { context: options.context ?? {} });
    const id = firstCreatedID(created);
    if (id === undefined) return;
    state.templates.push({ id, name, exportFields: [] });
    state.templateID = id;
    state.isEditingTemplate = false;
    renderTemplateControls();
    templateSelect.value = String(id);
    saveInput.value = "";
  });

  const cancelTemplateButton = document.createElement("button");
  cancelTemplateButton.type = "button";
  cancelTemplateButton.dataset.exportTemplateCancel = "true";
  cancelTemplateButton.className = "o_cancel_list_btn";
  cancelTemplateButton.textContent = "Cancel";
  cancelTemplateButton.addEventListener("click", async () => {
    state.isEditingTemplate = false;
    if (state.templateID === "new_template") {
      state.templateID = null;
      saveInput.value = "";
      renderTemplateControls();
      return;
    }
    if (typeof state.templateID === "number") {
      const template = state.templates.find((item) => item.id === state.templateID);
      if (template) {
        state.selected = await loadExportTemplateFields(model, template, state, fields, options);
        renderExportDialogSelectedFields(selectedList, state, availableList, fields, options);
        renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fields, options);
      }
    }
    renderTemplateControls();
  });

  const deleteButton = document.createElement("button");
  deleteButton.type = "button";
  deleteButton.dataset.exportTemplateDelete = "true";
  deleteButton.className = "o_delete_exported_list";
  deleteButton.textContent = "Delete";
  deleteButton.addEventListener("click", async () => {
    const id = typeof state.templateID === "number" ? state.templateID : 0;
    if (!id || !options.services?.orm) return;
    const accepted = await confirmStaticAction(options, "Do you really want to delete this export template?");
    if (!accepted) return;
    await options.services.orm.unlink("ir.exports", [id], { context: options.context ?? {} });
    state.templates = state.templates.filter((item) => item.id !== id);
    state.templateID = null;
    state.isEditingTemplate = false;
    state.selected = await exportDialogDefaultFields(model, defaultFieldNames, fields, state.availableFields, state.importCompat, options);
    renderTemplateControls();
    renderExportDialogSelectedFields(selectedList, state, availableList, fields, options);
  });

  const availableList = document.createElement("div");
  availableList.className = "o_left_field_panel";
  availableList.dataset.exportAvailableFields = "true";

  const searchInput = document.createElement("input");
  searchInput.type = "search";
  searchInput.className = "form-control mb-3 o_export_search_input";
  searchInput.id = "o-export-search-filter";
  searchInput.placeholder = "Search";
  searchInput.dataset.exportSearch = "true";
  searchInput.addEventListener("input", () => {
    const nextSearchTerm = searchInput.value.trim();
    if (state.searchTerm !== nextSearchTerm) {
      state.searchExpandedFieldIds.clear();
    }
    state.searchTerm = nextSearchTerm;
    renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fields, options);
  });

  const selectedList = document.createElement("ul");
  selectedList.className = "o_fields_list list-unstyled";
  selectedList.dataset.exportSelectedFields = "true";
  state.renderTemplateControls = renderTemplateControls;
  renderTemplateControls();
  renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fields, options);
  renderExportDialogSelectedFields(selectedList, state, availableList, fields, options);
  const browser = exportDialogBrowser();
  const onResize = () => {
    const nextIsSmall = exportDialogIsSmall(options);
    if (state.isSmall === nextIsSmall) return;
    state.isSmall = nextIsSmall;
    renderExportDialogSelectedFields(selectedList, state, availableList, fields, options);
    renderTemplateControls();
  };
  browser?.addEventListener?.("resize", onResize);

  const exportButton = document.createElement("button");
  exportButton.type = "button";
  exportButton.className = "btn btn-primary o_select_button";
  exportButton.dataset.exportConfirm = "true";
  exportButton.textContent = "Export";
  exportButton.addEventListener("click", async () => {
    const result = await exportDialogDownload(model, ids, state.selected, state.format, state.importCompat, options);
    shell.dispatchEvent(new CustomEvent("action-menu:export", {
      bubbles: true,
      detail: { model, ids: [...ids], fields: state.selected.map((field) => field.name), result }
    }));
  });

  const closeButton = document.createElement("button");
  closeButton.type = "button";
  closeButton.dataset.exportClose = "true";
  closeButton.textContent = "Close";
  closeButton.addEventListener("click", () => {
    browser?.removeEventListener?.("resize", onResize);
    clearElementChildren(dialog);
    dialog.dataset.closed = "true";
  });

  dialog.append(title, importLabel, formatSelect, templateSelect, saveInput, saveButton, cancelTemplateButton, deleteButton, searchInput, availableList, selectedList, exportButton, closeButton);
  shell.append(dialog);

  function renderTemplateControls(): void {
    renderExportTemplateOptions(templateSelect, state.templates);
    templateSelect.value = state.templateID === "new_template" ? "new_template" : state.templateID ? String(state.templateID) : "";
    setHTMLElementHidden(templateSelect, state.templateID === "new_template");
    setHTMLElementHidden(saveInput, state.templateID !== "new_template");
    setHTMLElementHidden(saveButton, !(state.isEditingTemplate && state.templateID === "new_template"));
    setHTMLElementHidden(cancelTemplateButton, !state.isEditingTemplate);
    setHTMLElementHidden(deleteButton, state.isEditingTemplate || typeof state.templateID !== "number");
    dialog.dataset.exportTemplateId = state.templateID === null ? "" : String(state.templateID);
    dialog.dataset.exportTemplateEditing = String(state.isEditingTemplate);
    dialog.dataset.exportIsSmall = String(state.isSmall);
  }
}

function exportDialogIsSmall(options: RenderWindowActionOptions): boolean {
  if (typeof options.isSmall === "function") return Boolean(options.isSmall());
  if (typeof options.isSmall === "boolean") return options.isSmall;
  const browser = exportDialogBrowser();
  if (typeof browser?.matchMedia === "function") return browser.matchMedia("(max-width: 767px)").matches;
  return typeof browser?.innerWidth === "number" ? browser.innerWidth < 768 : false;
}

function exportDialogBrowser(): (Window & typeof globalThis) | undefined {
  return typeof globalThis.window === "object" ? globalThis.window : undefined;
}

function renderAvailableExportFields(
  list: HTMLElement,
  fields: readonly ExportDialogField[],
  state: ExportDialogState,
  selectedList: HTMLElement,
  fallbackFields: Record<string, unknown>,
  options: RenderWindowActionOptions
): void {
  clearElementChildren(list);
  const visible = exportDialogVisibleFieldNames(state, options);
  const rootFields = exportDialogRootFields(fields, visible);
  if (!rootFields.length && state.searchTerm) {
    const empty = document.createElement("h3");
    empty.className = "text-center text-muted mt-5 o_no_match";
    empty.dataset.exportNoMatch = "true";
    empty.textContent = "No match found.";
    list.append(empty);
    return;
  }
  renderAvailableExportFieldRows(list, rootFields, state, selectedList, fallbackFields, options, 0, visible, false);
}

function renderAvailableExportFieldRows(
  list: HTMLElement,
  fields: readonly ExportDialogField[],
  state: ExportDialogState,
  selectedList: HTMLElement,
  fallbackFields: Record<string, unknown>,
  options: RenderWindowActionOptions,
  depth: number,
  visible: Set<string> | undefined,
  forceCurrentLevel: boolean
): void {
  for (const field of fields) {
    if (visible && !forceCurrentLevel && !visible.has(field.name)) continue;
    const canExpand = exportDialogFieldExpandable(field);
    const row = document.createElement("div");
    row.className = "o_export_tree_item";
    row.dataset.exportTreeItem = field.name;
    row.dataset.fieldId = field.name;
    row.dataset.field_id = field.name;
    row.setAttribute("style", `padding-left:${depth * 16}px`);
    row.addEventListener("dblclick", () => {
      if (!canExpand) {
        addExportDialogSelectedField(field, state, selectedList, list, fallbackFields, options);
      }
    });
    if (canExpand) {
      const expandButton = document.createElement("button");
      expandButton.type = "button";
      expandButton.className = "o_expand_parent";
      expandButton.dataset.exportExpandField = field.name;
      expandButton.textContent = state.expandedFieldIds.has(field.name) ? "-" : "+";
      expandButton.addEventListener("click", async (event) => {
        event.stopPropagation?.();
        await toggleExportDialogFieldExpansion(field, state, list, selectedList, fallbackFields, options);
      });
      row.append(expandButton);
    }
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_add_field";
    button.dataset.exportAddField = field.name;
    button.textContent = options.debug && field.name ? `${field.label} (${field.name})` : field.label;
    button.disabled = state.selected.some((item) => item.name === field.name);
    button.addEventListener("click", (event) => {
      event.stopPropagation?.();
      addExportDialogSelectedField(field, state, selectedList, list, fallbackFields, options);
    });
    row.append(button);
    list.append(row);
    if (state.searchTerm || state.expandedFieldIds.has(field.name)) {
      const children = state.expandedFields.get(field.name) ?? [];
      const showAllLoadedChildren = Boolean(state.searchTerm && state.searchExpandedFieldIds.has(field.name));
      renderAvailableExportFieldRows(list, children, state, selectedList, fallbackFields, options, depth + 1, visible, showAllLoadedChildren);
    }
  }
}

function addExportDialogSelectedField(
  field: ExportDialogField,
  state: ExportDialogState,
  selectedList: HTMLElement,
  availableList: HTMLElement,
  fallbackFields: Record<string, unknown>,
  options: RenderWindowActionOptions
): void {
  if (!state.selected.some((item) => item.name === field.name)) {
    state.selected.push(field);
    enterExportTemplateEdition(state);
    renderExportDialogSelectedFields(selectedList, state, availableList, fallbackFields, options);
    renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fallbackFields, options);
  }
}

async function toggleExportDialogFieldExpansion(
  field: ExportDialogField,
  state: ExportDialogState,
  availableList: HTMLElement,
  selectedList: HTMLElement,
  fallbackFields: Record<string, unknown>,
  options: RenderWindowActionOptions
): Promise<void> {
  if (!exportDialogFieldExpandable(field)) return;
  if (state.searchTerm) {
    if (state.searchExpandedFieldIds.has(field.name)) {
      state.searchExpandedFieldIds.delete(field.name);
      renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fallbackFields, options);
      return;
    }
    state.searchExpandedFieldIds.add(field.name);
    if (!state.expandedFields.has(field.name)) {
      const request = exportDialogChildFieldRequest(field, state);
      const rows = request ? await fetchExportFields(request, options) : [];
      state.expandedFields.set(field.name, rows.map((row) => exportDialogFieldFromInfo(row, fallbackFields)));
    }
    renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fallbackFields, options);
    return;
  }
  if (state.expandedFieldIds.has(field.name)) {
    state.expandedFieldIds.delete(field.name);
    renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fallbackFields, options);
    return;
  }
  state.expandedFieldIds.add(field.name);
  if (!state.expandedFields.has(field.name)) {
    const request = exportDialogChildFieldRequest(field, state);
    const rows = request ? await fetchExportFields(request, options) : [];
    state.expandedFields.set(field.name, rows.map((row) => exportDialogFieldFromInfo(row, fallbackFields)));
  }
  renderAvailableExportFields(availableList, state.availableFields, state, selectedList, fallbackFields, options);
}

function textNode(text: string): HTMLElement {
  const span = document.createElement("span");
  span.textContent = text;
  return span;
}

function exportDialogField(fields: Record<string, unknown>, name: string): ExportDialogField {
  const description = fields[name];
  const type = isRecord(description) && typeof description.type === "string" ? description.type : "char";
  const relation = isRecord(description) && typeof description.relation === "string" ? description.relation : undefined;
  const relationField = isRecord(description) && typeof description.relation_field === "string" ? description.relation_field : undefined;
  const params = isRecord(description) && isRecord(description.params)
    ? { ...description.params }
    : relation
      ? { model: relation, prefix: name }
      : undefined;
  const children = Boolean(params) && name.split("/").length < 3;
  const defaultExport = isRecord(description) && description.default_export === true;
  const defaultExportCompatible = isRecord(description) && description.default_export_compatible === true;
  return { name, value: name, label: fieldLabel(fields, name), type, relation, relationField, children, params, defaultExport, defaultExportCompatible };
}

function exportDialogFieldFromInfo(info: unknown, fallbackFields: Record<string, unknown>): ExportDialogField {
  if (!isRecord(info)) return exportDialogField(fallbackFields, "");
  const name = String(info.id ?? info.name ?? "");
  const value = String(info.value ?? name);
  const label = String(info.string ?? info.label ?? name);
  const type = String(info.field_type ?? info.type ?? "char");
  const relation = typeof info.relation === "string" ? info.relation : undefined;
  const relationField = typeof info.relation_field === "string" ? info.relation_field : undefined;
  const params = isRecord(info.params) ? { ...info.params } : undefined;
  const children = info.children === true && name.split("/").length < 3 && Boolean(params);
  const defaultExport = info.default_export === true;
  const defaultExportCompatible = info.default_export_compatible === true;
  return { name, value, label, type, relation, relationField, children, params, defaultExport, defaultExportCompatible };
}

function exportDialogFields(fields: Record<string, unknown>): ExportDialogField[] {
  return Object.keys(fields)
    .filter((name) => name !== "display_name")
    .sort((left, right) => fieldLabel(fields, left).localeCompare(fieldLabel(fields, right)) || left.localeCompare(right))
    .map((name) => exportDialogField(fields, name));
}

async function exportDialogAvailableFields(model: string, fields: Record<string, unknown>, importCompat: boolean, options: RenderWindowActionOptions): Promise<ExportDialogField[]> {
  const serverFields = await fetchExportFields({
    model,
    domain: options.activeDomain ? [...options.activeDomain] : [],
    import_compat: importCompat
  }, options);
  let available = serverFields.length
    ? serverFields.map((field) => exportDialogFieldFromInfo(field, fields))
    : exportDialogFields(fields);
  if (model !== "account.move.line") return available;
  const analyticLineField = available.find((field) => field.name === "analytic_line_ids");
  if (!analyticLineField) return available;
  const children = await fetchExportFields({
    model: analyticLineField.relation || "account.analytic.line",
    prefix: analyticLineField.name,
    parent_name: analyticLineField.label,
    import_compat: importCompat,
    parent_field_type: analyticLineField.type,
    parent_field: exportDialogParentFieldPayload(analyticLineField),
    exclude: exportDialogRelationExclude(analyticLineField),
    domain: []
  }, options);
  const accountantFields = children
    .map((field) => exportDialogFieldFromInfo(field, fields))
    .filter(accountantAnalyticExportField);
  return mergeExportDialogFields(available, accountantFields);
}

async function exportDialogDefaultFields(
  model: string,
  defaultFieldNames: readonly string[],
  fields: Record<string, unknown>,
  availableFields: readonly ExportDialogField[],
  importCompat: boolean,
  options: RenderWindowActionOptions
): Promise<ExportDialogField[]> {
  const byName = exportDialogFieldMap(availableFields);
  const explicitDefaults = defaultFieldNames.flatMap((name) => {
    const field = byName.get(name);
    if (field) return [field];
    return importCompat ? [] : [exportDialogField(fields, name)];
  });
  const metadataDefaults = availableFields.filter((field) => field.defaultExport || (importCompat && field.defaultExportCompatible));
  let selected = mergeExportDialogFields(explicitDefaults, metadataDefaults);
  if (model !== "account.move.line" || !byName.has("analytic_line_ids")) return selected;
  selected = selected.filter((field) => field.name !== "analytic_distribution");
  const additions = availableFields.filter(accountantAnalyticExportField);
  return mergeExportDialogFields(selected, additions);
}

function accountantAnalyticExportField(field: ExportDialogField): boolean {
  const paramsModel = isRecord(field.params) ? field.params.model : undefined;
  return (paramsModel === "account.analytic.account" && !field.name.includes("auto_account_id")) || field.name === "analytic_line_ids/amount";
}

function exportDialogParentFieldPayload(field: ExportDialogField): Record<string, unknown> {
  const payload: Record<string, unknown> = {
    ...(isRecord(field.params?.parent_field) ? field.params.parent_field : {}),
    id: field.name,
    name: field.name,
    string: field.label,
    value: field.value,
    type: field.type,
    field_type: field.type,
    relation: field.relation,
    children: Boolean(field.params),
    params: field.params ? { ...field.params } : undefined
  };
  if (field.relationField) {
    payload.relation_field = field.relationField;
  }
  return payload;
}

function exportDialogRelationExclude(field: ExportDialogField): string[] {
  return field.relationField ? [field.relationField] : [];
}

function exportDialogFieldExpandable(field: ExportDialogField): boolean {
  return field.children && Boolean(field.params) && field.name.split("/").length < 3;
}

function exportDialogChildFieldRequest(field: ExportDialogField, state: ExportDialogState): ExportGetFieldsRequest | undefined {
  if (!exportDialogFieldExpandable(field)) return undefined;
  const params = isRecord(field.params) ? field.params : {};
  const modelName = typeof params.model === "string" ? params.model : field.relation;
  if (!modelName) return undefined;
  return {
    ...params,
    model: modelName,
    domain: [],
    prefix: typeof params.prefix === "string" ? params.prefix : field.name,
    parent_name: field.label,
    import_compat: state.importCompat,
    parent_field_type: field.type,
    parent_field: exportDialogParentFieldPayload(field),
    exclude: exportDialogRelationExclude(field)
  };
}

function exportDialogKnownFields(state: ExportDialogState): ExportDialogField[] {
  return [...state.availableFields, ...Array.from(state.expandedFields.values()).flat()];
}

function exportDialogVisibleFieldNames(state: ExportDialogState, options: RenderWindowActionOptions): Set<string> | undefined {
  const rawTerm = state.searchTerm.trim();
  const term = rawTerm.toLowerCase();
  if (!term) return undefined;
  const knownByName = exportDialogFieldMap(exportDialogKnownFields(state));
  const visible = new Set<string>();
  for (const field of knownByName.values()) {
    const labelMatch = exportDialogSearchMatches(field.label, term);
    const technicalMatch = Boolean(options.debug) && field.name.includes(rawTerm);
    if (!labelMatch && !technicalMatch) continue;
    let name = field.name;
    while (name) {
      visible.add(name);
      const parent = exportDialogParentPath(name);
      if (!parent || !knownByName.has(parent)) break;
      name = parent;
    }
  }
  return visible;
}

function exportDialogSearchMatches(label: string, term: string): boolean {
  const normalizedTerm = exportDialogNormalizeSearch(term);
  if (!normalizedTerm) return false;
  const reversedLabel = label.split("/").reverse().join("/");
  const normalizedLabel = exportDialogNormalizeSearch(reversedLabel);
  if (normalizedLabel.includes(normalizedTerm)) return true;
  return normalizedTerm.split(" ").every((token) => exportDialogFuzzyIncludes(normalizedLabel, token));
}

function exportDialogNormalizeSearch(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, " ").trim();
}

function exportDialogFuzzyIncludes(value: string, token: string): boolean {
  if (!token) return true;
  let offset = 0;
  for (const char of token) {
    const index = value.indexOf(char, offset);
    if (index === -1) return false;
    offset = index + 1;
  }
  return true;
}

function exportDialogRootFields(fields: readonly ExportDialogField[], visible: Set<string> | undefined): ExportDialogField[] {
  if (!visible) return [...fields];
  return fields.filter((field) => visible.has(field.name));
}

function exportDialogParentPath(name: string): string {
  const parts = name.split("/");
  if (parts.length <= 1) return "";
  return parts.slice(0, -1).join("/");
}

function mergeExportDialogFields(base: readonly ExportDialogField[], additions: readonly ExportDialogField[]): ExportDialogField[] {
  const seen = new Set<string>();
  const out: ExportDialogField[] = [];
  for (const field of [...base, ...additions]) {
    if (!field.name || seen.has(field.name)) continue;
    seen.add(field.name);
    out.push(field);
  }
  return out;
}

function exportDialogFieldMap(fields: readonly ExportDialogField[]): Map<string, ExportDialogField> {
  return new Map(fields.map((field) => [field.name, field]));
}

async function fetchExportFields(request: ExportGetFieldsRequest, options: RenderWindowActionOptions): Promise<readonly unknown[]> {
  if (options.exportGetFields) return options.exportGetFields(request);
  const response = await fetch("/web/export/get_fields", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: Date.now(), params: request })
  });
  const payload = await response.json();
  return Array.isArray(payload?.result) ? payload.result : [];
}

async function loadExportTemplates(model: string, options: RenderWindowActionOptions): Promise<ExportTemplateRecord[]> {
  if (!options.services?.orm) return [];
  const rows = await options.services.orm.searchRead<unknown[]>("ir.exports", [["resource", "=", model]], ["id", "name", "export_fields"], { context: options.context ?? {} });
  return rows.filter(isRecord).map((row) => ({
    id: Number(row.id),
    name: String(row.name ?? ""),
    exportFields: numberList(row.export_fields)
  })).filter((row) => Number.isFinite(row.id) && row.id > 0);
}

function renderExportTemplateOptions(select: HTMLElement, templates: readonly ExportTemplateRecord[]): void {
  clearElementChildren(select);
  const empty = document.createElement("option");
  empty.value = "";
  empty.textContent = "";
  select.append(empty);
  for (const template of templates) {
    const option = document.createElement("option");
    option.value = String(template.id);
    option.textContent = template.name || "undefined";
    select.append(option);
  }
  const newTemplate = document.createElement("option");
  newTemplate.value = "new_template";
  newTemplate.textContent = "New template";
  select.append(newTemplate);
}

async function fetchExportNamelist(request: ExportNamelistRequest, options: RenderWindowActionOptions): Promise<readonly unknown[]> {
  if (options.exportNamelist) return options.exportNamelist(request);
  const response = await fetch("/web/export/namelist", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: Date.now(), params: request })
  });
  const payload = await response.json();
  return Array.isArray(payload?.result) ? payload.result : [];
}

async function loadExportTemplateFields(
  model: string,
  template: ExportTemplateRecord,
  state: ExportDialogState,
  fields: Record<string, unknown>,
  options: RenderWindowActionOptions
): Promise<ExportDialogField[]> {
  if (!template.id) return [];
  const rows = await fetchExportNamelist({ model, export_id: template.id }, options);
  const serverFields = rows.filter(isRecord).map((row) => {
    return exportDialogFieldFromInfo(row, fields);
  }).filter((field) => field.name);
  for (const field of serverFields) {
    await hydrateExportDialogTemplateFieldParents(field.name, state, fields, options);
  }
  const byName = exportDialogFieldMap(exportDialogKnownFields(state));
  return serverFields.map((field) => byName.get(field.name) ?? field);
}

async function hydrateExportDialogTemplateFieldParents(
  name: string,
  state: ExportDialogState,
  fallbackFields: Record<string, unknown>,
  options: RenderWindowActionOptions
): Promise<void> {
  const parts = name.split("/").filter(Boolean);
  for (let index = 1; index < parts.length; index += 1) {
    const parentName = parts.slice(0, index).join("/");
    state.expandedFieldIds.add(parentName);
    if (state.expandedFields.has(parentName)) continue;
    const parentField = exportDialogFieldMap(exportDialogKnownFields(state)).get(parentName);
    if (!parentField || !exportDialogFieldExpandable(parentField)) continue;
    const request = exportDialogChildFieldRequest(parentField, state);
    const rows = request ? await fetchExportFields(request, options) : [];
    state.expandedFields.set(parentName, rows.map((row) => exportDialogFieldFromInfo(row, fallbackFields)));
  }
}

function renderExportDialogSelectedFields(
  list: HTMLElement,
  state: ExportDialogState,
  availableList: HTMLElement,
  fallbackFields: Record<string, unknown>,
  options: RenderWindowActionOptions
): void {
  renderSelectedExportFields(list, state.selected, state.isSmall, options, {
    remove(field) {
      state.selected = state.selected.filter((item) => item.name !== field.name);
      enterExportTemplateEdition(state);
      renderExportDialogSelectedFields(list, state, availableList, fallbackFields, options);
      renderAvailableExportFields(availableList, state.availableFields, state, list, fallbackFields, options);
    },
    reorder(field, previous, next) {
      reorderExportDialogSelectedField(state, field, previous, next);
      renderExportDialogSelectedFields(list, state, availableList, fallbackFields, options);
    },
  });
}

function enterExportTemplateEdition(state: ExportDialogState): void {
  if (state.templateID && !state.isEditingTemplate) {
    state.isEditingTemplate = true;
    state.renderTemplateControls?.();
  }
}

function setHTMLElementHidden(element: HTMLElement, hidden: boolean): void {
  if (hidden) {
    element.setAttribute("hidden", "");
    element.dataset.hidden = "true";
  } else {
    element.removeAttribute("hidden");
    delete element.dataset.hidden;
  }
}

function notifyExportDialog(options: RenderWindowActionOptions, message: string, type = "info"): void {
  options.services?.notification?.add(message, { type });
}

function reorderExportDialogSelectedField(state: ExportDialogState, field: string, previous?: string | null, next?: string | null): void {
  const from = state.selected.findIndex((item) => item.name === field);
  const to = exportDialogSortableTargetIndex(state.selected, field, previous, next);
  if (from === to || from < 0 || to < 0 || from >= state.selected.length || to >= state.selected.length) return;
  state.selected.splice(to, 0, state.selected.splice(from, 1)[0]);
  enterExportTemplateEdition(state);
}

function exportDialogSortableTargetIndex(
  fields: readonly ExportDialogField[],
  field: string,
  previous?: string | null,
  next?: string | null
): number {
  const item = fields.findIndex((candidate) => candidate.name === field);
  const previousIndex = previous ? fields.findIndex((candidate) => candidate.name === previous) : -1;
  const nextIndex = next ? fields.findIndex((candidate) => candidate.name === next) : -1;
  if (item === -1) return -1;
  if (item < previousIndex) return previous ? previousIndex : 0;
  return next ? nextIndex : fields.length - 1;
}

function exportDialogInferredDropNeighbors(
  fields: readonly ExportDialogField[],
  from: number,
  to: number
): { previous?: string; next?: string } {
  if (from === -1 || to < 0 || to >= fields.length || from === to) return {};
  if (from < to) {
    return {
      previous: fields[to]?.name,
      next: fields[to + 1]?.name,
    };
  }
  return {
    previous: fields[to - 1]?.name,
    next: fields[to]?.name,
  };
}

function exportDialogNativeDropNeighbors(
  list: HTMLElement,
  target: HTMLElement,
  event: DragEvent
): { previous?: string; next?: string } | undefined {
  if (typeof event.clientY !== "number" || typeof target.getBoundingClientRect !== "function") return undefined;
  const draggedField = list.dataset.exportDraggingField;
  const siblings = Array.from(list.children)
    .filter((node): node is HTMLElement => Boolean((node as HTMLElement).dataset))
    .filter((node) => node.dataset.field_id && node.dataset.field_id !== draggedField);
  const targetIndex = siblings.findIndex((node) => node.dataset.field_id === target.dataset.field_id);
  if (targetIndex === -1) return undefined;
  const rect = target.getBoundingClientRect();
  const afterTarget = event.clientY > rect.top + rect.height / 2;
  const previous = afterTarget ? siblings[targetIndex] : siblings[targetIndex - 1];
  const next = afterTarget ? siblings[targetIndex + 1] : siblings[targetIndex];
  return {
    previous: previous?.dataset.field_id,
    next: next?.dataset.field_id,
  };
}

interface SelectedExportFieldHandlers {
  remove?: (field: ExportDialogField) => void;
  reorder?: (field: string, previous?: string, next?: string) => void;
}

interface ExportDropDetail {
  previousField?: string | null;
  nextField?: string | null;
}

function renderSelectedExportFields(
  list: HTMLElement,
  fields: readonly ExportDialogField[],
  isSmall: boolean,
  options: RenderWindowActionOptions,
  handlers: SelectedExportFieldHandlers = {}
): void {
  clearElementChildren(list);
  const sortable = !isSmall;
  fields.forEach((field, index) => {
    const item = document.createElement("li");
    item.className = `o_export_field d-inline-block w-100${sortable ? " o_export_field_sortable" : ""}`;
    item.dataset.exportField = field.name;
    item.dataset.field_id = field.name;
    if (handlers.reorder && sortable) {
      item.draggable = fields.length > 1;
      item.setAttribute("draggable", fields.length > 1 ? "true" : "false");
      item.addEventListener("dragstart", () => {
        list.dataset.exportDraggingIndex = String(index);
        list.dataset.exportDraggingField = field.name;
      });
      item.addEventListener("dragover", (event) => {
        event.preventDefault();
      });
      item.addEventListener("drop", (event) => {
        event.preventDefault();
        const from = fields.findIndex((candidate) => candidate.name === list.dataset.exportDraggingField);
        const dropDetail = (event as unknown as CustomEvent<ExportDropDetail>).detail;
        const nativeDrop = exportDialogNativeDropNeighbors(list, item, event);
        const fallback = exportDialogInferredDropNeighbors(fields, from, index);
        delete list.dataset.exportDraggingIndex;
        delete list.dataset.exportDraggingField;
        if (from !== -1) {
          const previous = dropDetail && "previousField" in dropDetail
            ? dropDetail.previousField ?? undefined
            : nativeDrop?.previous ?? fallback.previous;
          const next = dropDetail && "nextField" in dropDetail
            ? dropDetail.nextField ?? undefined
            : nativeDrop?.next ?? fallback.next;
          handlers.reorder?.(
            fields[from].name,
            previous,
            next
          );
        }
      });
      item.addEventListener("dragend", () => {
        delete list.dataset.exportDraggingIndex;
        delete list.dataset.exportDraggingField;
      });
      const sortHandle = document.createElement("span");
      sortHandle.className = "fa fa-sort o_sort_field mx-1";
      sortHandle.dataset.exportSortField = field.name;
      sortHandle.setAttribute("style", `opacity:${fields.length === 1 ? 0 : 1}`);
      item.append(sortHandle);
    }
    const label = document.createElement("span");
    label.textContent = options.debug && field.name ? `${field.label} (${field.name})` : field.label;
    item.append(label);
    if (handlers.remove) {
      const removeButton = document.createElement("span");
      removeButton.className = "fa fa-trash m-1 pe-2 float-end o_remove_field cursor-pointer";
      removeButton.dataset.exportRemoveField = field.name;
      removeButton.addEventListener("click", () => handlers.remove?.(field));
      item.append(removeButton);
    }
    list.append(item);
  });
}

async function exportDialogDownload(
  model: string,
  ids: readonly number[],
  fields: readonly ExportDialogField[],
  format: string,
  importCompat: boolean,
  options: RenderWindowActionOptions
): Promise<unknown> {
  const downloadFields = fields.map((field) => ({ name: field.value || field.name, label: field.label, store: true, type: field.type }));
  if (importCompat && !downloadFields.some((field) => field.name === "id")) {
    downloadFields.unshift({ name: "id", label: "External ID", store: true, type: "integer" });
  }
  const request: ExportDownloadRequest = {
    route: `/web/export/${format || "xlsx"}`,
    model,
    ids: [...ids],
    domain: options.activeDomain ? [...options.activeDomain] : [["id", "in", [...ids]]],
    fields: downloadFields,
    context: options.context ?? {},
    importCompat,
    groupby: exportDialogGroupBy(options)
  };
  if (options.exportDownload) {
    return options.exportDownload(request);
  }
  const formData = new FormData();
  formData.set("data", JSON.stringify({
    import_compat: request.importCompat,
    context: request.context,
    domain: request.domain,
    fields: request.fields,
    groupby: request.groupby,
    ids: request.ids,
    model: request.model
  }));
  const response = await fetch(request.route, { method: "POST", body: formData });
  return { status: response.status, ok: response.ok };
}

function exportDialogGroupBy(options: RenderWindowActionOptions): string[] {
  const active = normalizeStringList(options.activeGroupBy);
  if (active.length) return active;
  const contextGroupBy = options.context?.group_by ?? options.context?.groupby;
  if (typeof contextGroupBy === "string" && contextGroupBy.trim() && !contextGroupBy.trim().startsWith("[") && !contextGroupBy.trim().startsWith("(")) {
    return contextGroupBy.split(",").map((item) => item.trim()).filter(Boolean);
  }
  return normalizeStringList(contextGroupBy);
}

function firstCreatedID(value: unknown): number | undefined {
  if (Array.isArray(value) && value.length) return numberRecordID(value[0]);
  if (isRecord(value)) return numberRecordID(value.id);
  return numberRecordID(value);
}

function numberList(value: unknown): number[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => Number(item)).filter((item) => Number.isFinite(item) && item > 0);
}

function renderStaticActionMenuButton(
  key: string,
  label: string,
  iconClass: string,
  sequence: number,
  callback: () => unknown | Promise<unknown>,
  options: { requiresSelection?: boolean; className?: string } = {}
): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `gorp-action-menu-item${options.className ? ` ${options.className}` : ""}`;
  button.dataset.actionMenuType = "static";
  button.dataset.staticAction = key;
  button.dataset.sequence = String(sequence);
  button.dataset.groupNumber = "1";
  button.dataset.icon = iconClass;
  if (options.requiresSelection) {
    button.dataset.requiresSelection = "true";
    button.disabled = true;
  }
  button.textContent = label;
  const icon = document.createElement("i");
  icon.className = iconClass;
  icon.setAttribute("aria-hidden", "true");
  button.append(icon);
  button.addEventListener("click", async () => {
    await callback();
  });
  return button;
}

function renderServerActionMenuButton(
  kind: "action" | "print",
  model: string,
  item: ActionMenuRecord,
  getActiveIds: () => number[],
  requiresSelection: boolean,
  root: HTMLElement,
  options: RenderWindowActionOptions,
  validateReportOnClick = true
): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "gorp-action-menu-item";
  button.dataset.actionMenuType = kind;
  button.dataset.sequence = String(numberActionValue(item.sequence, 100));
  button.dataset.groupNumber = String(numberActionValue(item.groupNumber, 100));
  const id = item.id;
  if (typeof id === "number" || typeof id === "string") button.dataset.actionId = String(id);
  if (requiresSelection) {
    button.dataset.requiresSelection = "true";
    button.disabled = !options.isDomainSelected;
  }
  const description = stringActionValue(item.description, stringActionValue(item.name, kind === "print" ? "Print" : "Action"));
  button.textContent = description;
  const iconClass = stringActionValue(item.icon, kind === "print" ? "fa fa-print" : "");
  if (iconClass) {
    button.dataset.icon = iconClass;
    const icon = document.createElement("i");
    icon.className = iconClass;
    icon.setAttribute("aria-hidden", "true");
    button.append(icon);
  }
  button.addEventListener("click", async () => {
    const selectedIds = getActiveIds();
    if (requiresSelection && selectedIds.length === 0 && !options.isDomainSelected) return;
    if (validateReportOnClick && kind === "print" && "domain" in item && !(await reportActionAvailable(options, model, item.id, selectedIds))) {
      root.dispatchEvent(new CustomEvent("action-menu:report-unavailable", {
        bubbles: true,
        detail: { model, ids: selectedIds, action: item }
      }));
      return;
    }
    const ids = await actionMenuActiveIds(model, selectedIds, options);
    if (typeof item.url === "string" && item.url) {
      root.dispatchEvent(new CustomEvent("action-menu:url", {
        bubbles: true,
        detail: { model, ids, url: item.url, type: kind }
      }));
      return;
    }
    const actionRequest: ActionRequest = typeof id === "number" || typeof id === "string" ? id : { ...item };
    await options.services?.action?.doAction(actionRequest, {
      additionalContext: activeIdsContext(model, ids, options.activeDomain),
      onClose: () => options.onRefresh?.()
    });
    root.dispatchEvent(new CustomEvent("action-menu:execute", {
      bubbles: true,
      detail: { model, ids, action: item, type: kind }
    }));
  });
  return button;
}

async function reportActionAvailable(
  options: RenderWindowActionOptions,
  model: string,
  actionId: unknown,
  ids: readonly number[]
): Promise<boolean> {
  if (!(typeof actionId === "number" || typeof actionId === "string")) return true;
  const orm = options.services?.orm;
  if (!orm) return true;
  const validIds = await orm.call<unknown[]>("ir.actions.report", "get_valid_action_reports", [[actionId], model, [...ids]]);
  return validIds.some((candidate) => actionIDMatches(candidate, actionId));
}

function actionIDMatches(candidate: unknown, expected: unknown): boolean {
  if (candidate === expected) return true;
  if (typeof candidate === "number" && typeof expected === "number") return candidate === expected;
  return String(candidate) === String(expected);
}

function actionMenusHaveItems(actionMenus: Record<string, unknown> | undefined): boolean {
  return actionMenuRecords(actionMenus, "action").length > 0 || actionMenuRecords(actionMenus, "print").length > 0;
}

function actionMenuRecords(actionMenus: Record<string, unknown> | undefined, key: "action" | "print"): ActionMenuRecord[] {
  const rawItems = actionMenus?.[key];
  if (!Array.isArray(rawItems)) return [];
  return rawItems.filter(isRecord).map((item) => item as ActionMenuRecord);
}

function compareActionMenuButtons(left: HTMLElement, right: HTMLElement): number {
  const leftGroup = Number(left.dataset.groupNumber ?? "100");
  const rightGroup = Number(right.dataset.groupNumber ?? "100");
  if (leftGroup !== rightGroup) return leftGroup - rightGroup;
  return Number(left.dataset.sequence ?? "100") - Number(right.dataset.sequence ?? "100");
}

async function actionMenuActiveIds(model: string, ids: readonly number[], options: RenderWindowActionOptions): Promise<number[]> {
  if (options.isDomainSelected && options.activeDomain && options.services?.orm) {
    return options.services.orm.search<number[]>(model, options.activeDomain, {
      limit: options.activeIdsLimit ?? 20000,
      context: options.context ?? {}
    });
  }
  return [...ids];
}

function activeIdsContext(model: string, ids: readonly number[], activeDomain?: DomainExpression): Record<string, unknown> {
  return {
    active_id: ids[0],
    active_ids: [...ids],
    active_model: model,
    active_domain: activeDomain ? [...activeDomain] : []
  };
}

function activeFieldNameForView(activeFieldNames: ReadonlySet<string>, fields: Record<string, unknown>): string | undefined {
  for (const name of ["active", "x_active"]) {
    if (activeFieldNames.has(name) && fields[name]) return name;
  }
  return undefined;
}

function recordActiveValue(values: Record<string, unknown>, fieldName: string): boolean {
  return values[fieldName] !== false;
}

async function confirmStaticAction(options: RenderWindowActionOptions, message: string): Promise<boolean> {
  if (options.confirm) return Boolean(await options.confirm(message));
  if (typeof globalThis.confirm === "function") return globalThis.confirm(message);
  return true;
}

function stringActionValue(value: unknown, fallback: string): string {
  return typeof value === "string" && value.trim() ? value : fallback;
}


function renderApproveAllListButton(model: string | undefined, selectedIds: ReadonlySet<number>, shell: HTMLElement, options: RenderWindowActionOptions): HTMLElement {
  const approve = document.createElement("button");
  approve.type = "button";
  approve.className = "gorp-list-approve-all";
  setWorkflowActionMetadata(approve, "approve", "fa fa-thumbs-up", 110, "Approve");
  approve.dataset.requiresSelection = "true";
  approve.disabled = true;
  approve.addEventListener("click", async () => {
    if (!model || selectedIds.size === 0) return;
    const ids = Array.from(selectedIds);
    const message = "Are you sure you want to approve selected documents?";
    const accepted = options.confirm
      ? await options.confirm(message)
      : typeof globalThis.confirm === "function"
        ? globalThis.confirm(message)
        : true;
    if (!accepted) return;
    const result = await options.services?.dataset?.callButton(model, "action_approve_all", [ids], {});
    if (isRecord(result) && options.services?.action) {
      await options.services.action.doAction(result);
    }
    await options.onRefresh?.();
    shell.dispatchEvent(new CustomEvent("workflow:approve-all", {
      bubbles: true,
      detail: { model, ids, result }
    }));
  });
  return approve;
}

function renderUpdateStatusListButton(model: string, selectedIds: ReadonlySet<number>, shell: HTMLElement, options: RenderWindowActionOptions): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "gorp-list-update-status";
  setWorkflowActionMetadata(button, "update_status", "fa fa-code", 100, "Update Status");
  button.dataset.requiresSelection = "true";
  button.disabled = true;
  button.addEventListener("click", async () => {
    if (selectedIds.size === 0) return;
    const ids = Array.from(selectedIds);
    await options.services?.action?.doAction(updateStatusAction(model, ids), actionRefreshOptions(options));
    shell.dispatchEvent(new CustomEvent("workflow:update-status", {
      bubbles: true,
      detail: { model, ids }
    }));
  });
  return button;
}

function renderApprovalLogListButton(model: string, selectedIds: ReadonlySet<number>, shell: HTMLElement, options: RenderWindowActionOptions): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "gorp-list-approval-log";
  setWorkflowActionMetadata(button, "approve_log", "fa fa-arrows-h", 120, "Approval Log");
  button.dataset.requiresSelection = "true";
  button.disabled = true;
  button.addEventListener("click", async () => {
    if (selectedIds.size === 0) return;
    const ids = Array.from(selectedIds);
    await options.services?.action?.doAction(approvalLogAction(model, ids, false));
    shell.dispatchEvent(new CustomEvent("workflow:approval-log", {
      bubbles: true,
      detail: { model, ids }
    }));
  });
  return button;
}

function listShowsActionApproveAll(arch: string): boolean {
  return /<list\b[^>]*\bshow_action_approve_all=(["'])true\1/.test(arch);
}

function updateListToolbarButtons(root: HTMLElement, selectedIds: ReadonlySet<number>) {
  const toolbar = root.children[0] as HTMLElement | undefined;
  updateSelectionButtons(toolbar, selectedIds.size > 0);
  clearLoadedPrintMenus(toolbar);
}

function updateSelectionButtons(node: HTMLElement | undefined, hasSelection: boolean): void {
  if (!node) return;
  for (const child of Array.from(node.children ?? [])) {
    const element = child as HTMLElement;
    if (element.dataset?.requiresSelection === "true") {
      (element as HTMLButtonElement).disabled = !hasSelection;
    }
    updateSelectionButtons(element, hasSelection);
  }
}

function clearLoadedPrintMenus(node: HTMLElement | undefined): void {
  if (!node) return;
  for (const child of Array.from(node.children ?? [])) {
    const element = child as HTMLElement;
    if (element.dataset?.actionMenuItems === "print") {
      clearElementChildren(element);
      continue;
    }
    clearLoadedPrintMenus(element);
  }
}

function numberRecordID(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) return value;
  return undefined;
}

function renderFormView(
  viewDescription: ViewDescription | undefined,
  fields: Record<string, unknown>,
  values: Record<string, unknown>,
  model: string,
  options: RenderWindowActionOptions = {}
): HTMLElement {
  const form = document.createElement("form");
  form.className = "gorp-form-view o_form_view";
  const arch = viewDescription?.arch ?? "";
  const recordValues = values;
  const allFieldNodes = parseViewFieldNodes(arch);
  const activeFieldNames = new Set(allFieldNodes.map((node) => node.name));
  const recordID = numberRecordID(recordValues.id);
  const actionMenu = renderFormWorkflowActionMenu(viewDescription, model, recordID, fields, activeFieldNames, recordValues, form, options);
  if (actionMenu) form.append(actionMenu);
  const buttons = parseViewButtonNodes(arch).filter((node) => !nodeInvisible(node.attrs, recordValues));
  const nodes = allFieldNodes.filter((node) => !fieldInvisible(node, recordValues));
  const fieldNodes: ViewFieldNode[] = nodes.length
    ? nodes
    : Object.keys(fields)
      .filter((name) => name !== "id" && name !== "display_name")
      .slice(0, 6)
      .map((name) => ({ name, attrs: {}, children: [], childViewAttrs: {} }));
  const statusbarNodes = fieldNodes.filter((node) => isStatusbarFieldNode(node, fields[node.name]));
  if (buttons.length || statusbarNodes.length) {
    const header = document.createElement("div");
    header.className = "gorp-form-header o_form_statusbar";
    for (const node of statusbarNodes) {
      header.append(renderStatusbarField(model, node, fields[node.name], recordValues, form, options));
    }
    for (const node of buttons) {
      header.append(renderFormButton(model, node, recordValues, activeFieldNames, form, options));
    }
    form.append(header);
  }
  const body = document.createElement("div");
  body.className = "gorp-form-body o-list-content o-form-content o_form_sheet_bg";
  body.setAttribute("style", "padding:14px 18px 24px;background:#eef0f3;");
  const sheet = document.createElement("section");
  sheet.className = "gorp-form-sheet o-form-sheet o_form_sheet";
  sheet.setAttribute("style", "max-width:980px;margin:0 auto;background:#fff;border:1px solid var(--line);padding:20px;");
  const title = renderFormTitle(recordValues);
  if (title) sheet.append(title);
  const group = document.createElement("div");
  group.className = "gorp-form-fields record-grid o_group";
  for (const node of fieldNodes) {
    const name = node.name;
    if (statusbarNodes.includes(node)) continue;
    if (node.attrs.widget === "res_user_group_ids" && name === "group_ids") {
      group.append(renderResUserGroupIdsField(node, fields, recordValues, form, options.onUpdate));
      continue;
    }
    const label = document.createElement("label");
    label.className = "gorp-form-field o_wrap_field";
    label.dataset.field = name;
    const caption = document.createElement("span");
    caption.className = "o_form_label";
    caption.textContent = fieldLabel(fields, name);
    const required = formFieldRequired(node, fields[name], recordValues);
    if (required) label.dataset.required = "true";
    const value = required && formFieldEditable(node, fields[name], recordValues)
      ? renderEditableFormField(node, fields[name], recordValues, form, options)
      : renderReadonlyFieldValue(node, fields[name], recordValues[name], recordValues);
    label.append(caption, value);
    group.append(label);
  }
  sheet.append(group);
  body.append(sheet);
  form.append(body);
  if (viewHasChatter(arch)) form.append(renderChatterContainer(model, recordID, options));
  return form;
}

function renderFormTitle(values: Record<string, unknown>): HTMLElement | null {
  const titleText = firstText(values.display_name, values.name);
  if (!titleText) return null;
  const title = document.createElement("div");
  title.className = "oe_title";
  const heading = document.createElement("h1");
  heading.textContent = titleText;
  title.append(heading);
  return title;
}

const decorationOrder = ["danger", "warning", "success", "info", "primary", "muted", "bf", "it"] as const;

function renderReadonlyFieldValue(
  node: ViewFieldNode,
  description: unknown,
  value: unknown,
  evalContext: Record<string, unknown>
): HTMLElement {
  if (node.attrs.widget === "many2one_avatar_employee" && fieldTypeValue(description) === "many2one") {
    return renderMany2OneAvatarValue(node.name, fieldRelationValue(description) || "hr.employee", value);
  }
  if (node.attrs.widget === "badge" || node.attrs.widget === "selection_badge") {
    return renderBadgeValue(node, description, value, evalContext);
  }
  const output = document.createElement("output");
  output.className = "gorp-field-value o_field_widget o_readonly_modifier";
  output.textContent = fieldDisplayText(description, value);
  return output;
}

function renderMany2OneAvatarValue(fieldName: string, relation: string, value: unknown): HTMLElement {
  const data = many2OneDisplayData(value);
  const root = document.createElement("span");
  root.className = "gorp-many2one-avatar o_field_widget o_field_many2one_avatar";
  root.dataset.field = fieldName;
  root.dataset.relation = relation;
  if (data.id !== undefined) {
    root.dataset.resId = String(data.id);
    const image = document.createElement("img");
    image.className = "gorp-many2one-avatar-img o_avatar o_m2o_avatar rounded-circle";
    image.src = `/web/image/${relation}/${data.id}/avatar_128`;
    image.alt = data.displayName;
    root.append(image);
  }
  const label = document.createElement("span");
  label.className = "gorp-many2one-avatar-name";
  label.textContent = data.displayName;
  root.append(label);
  return root;
}

function renderBadgeValue(
  node: ViewFieldNode,
  description: unknown,
  value: unknown,
  evalContext: Record<string, unknown>
): HTMLElement {
  const badge = document.createElement("span");
  const decoration = activeDecoration(node.attrs, evalContext);
  badge.className = ["gorp-badge", "badge", "rounded-pill", badgeDecorationClass(decoration)].join(" ");
  badge.dataset.field = node.name;
  badge.dataset.widget = node.attrs.widget || "badge";
  if (decoration) badge.dataset.decoration = decoration;
  badge.textContent = fieldDisplayText(description, value);
  return badge;
}

function activeDecoration(attrs: Record<string, string>, evalContext: Record<string, unknown>): string {
  for (const name of decorationOrder) {
    const expression = attrs[`decoration-${name}`];
    if (!expression) continue;
    const evaluated = safeEvaluateBooleanAttr(expression, evalContext);
    if (evaluated === true || (evaluated === undefined && pythonTruthy(safeEvaluateAttr(expression, evalContext)))) {
      return name === "bf" || name === "it" ? "" : name;
    }
  }
  return "";
}

function badgeDecorationClass(decoration: string): string {
  return decoration && decoration !== "muted" ? `text-bg-${decoration}` : "text-bg-300";
}

function listDecorationClassName(attrs: Record<string, string>, evalContext: Record<string, unknown>): string {
  const classes = ["gorp-list-row"];
  for (const name of decorationOrder) {
    const expression = attrs[`decoration-${name}`];
    if (!expression) continue;
    const evaluated = safeEvaluateBooleanAttr(expression, evalContext);
    const active = evaluated === true || (evaluated === undefined && pythonTruthy(safeEvaluateAttr(expression, evalContext)));
    if (!active) continue;
    if (name === "bf") classes.push("fw-bold");
    else if (name === "it") classes.push("fst-italic");
    else classes.push(`text-bg-${name}`, `o_list_record_${name}`);
  }
  return classes.join(" ");
}

function fieldDisplayText(description: unknown, value: unknown): string {
  const fieldType = fieldTypeValue(description);
  if (fieldType === "selection") {
    const key = String(value ?? "");
    const found = selectionOptions(description).find(([candidate]) => candidate === key);
    if (found) return found[1];
  }
  if (fieldType === "many2one" || fieldType === "reference") {
    return many2OneDisplayData(value).displayName;
  }
  return formatCellValue(value);
}

function many2OneDisplayData(value: unknown): { id?: number; displayName: string } {
  if (Array.isArray(value)) {
    const id = numberRecordID(value[0]);
    return { id, displayName: String(value[1] ?? id ?? "") };
  }
  if (isRecord(value)) {
    const id = numberRecordID(value.id);
    return { id, displayName: String(value.display_name ?? value.name ?? id ?? "") };
  }
  const id = numberRecordID(value);
  return { id, displayName: value === null || value === undefined || value === false ? "" : String(value) };
}

function viewHasChatter(arch: string): boolean {
  return /<chatter(?:\s|\/|>)/.test(arch);
}

function renderChatterContainer(model: string, recordID: number | undefined, options: RenderWindowActionOptions): HTMLElement {
  const chatter = document.createElement("aside");
  chatter.className = "gorp-chatter o-mail-ChatterContainer o-mail-Form-chatter o-mail-Chatter";
  chatter.dataset.threadModel = model;
  if (recordID !== undefined) chatter.dataset.threadId = String(recordID);
  const header = document.createElement("div");
  header.className = "gorp-chatter-header";
  header.textContent = "Chatter";
  const composer = document.createElement("div");
  composer.className = "gorp-chatter-composer o-mail-Composer";
  for (const label of ["Send message", "Log note", "Activities"]) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "gorp-chatter-tab";
    button.dataset.chatterAction = label.toLowerCase().replace(/\s+/g, "-");
    button.textContent = label;
    composer.append(button);
  }
  const thread = document.createElement("div");
  thread.className = "gorp-chatter-thread o-mail-Thread";
  thread.dataset.chatterThread = "true";
  chatter.append(header, composer, thread);
  if (recordID !== undefined && options.services?.mail) {
    thread.textContent = "Loading...";
    void loadChatterThread(thread, model, recordID, options);
  }
  return chatter;
}

async function loadChatterThread(
  thread: HTMLElement,
  model: string,
  recordID: number,
  options: RenderWindowActionOptions
): Promise<void> {
  try {
    const payload = await options.services?.mail?.chatterFetch(
      { thread_model: model, thread_id: recordID },
      { limit: 30 },
      chatterAccessParams(options.context)
    );
    renderChatterThread(thread, payload);
  } catch {
    clearElementChildren(thread);
    thread.textContent = "Chatter unavailable";
  }
}

function chatterAccessParams(context?: Record<string, unknown>): PortalAccessParams | null {
  if (!context) return null;
  const token = firstValue(context.token ?? context.access_token ?? context.accessToken);
  const hash = firstValue(context.hash ?? context._hash);
  const pid = firstValue(context.pid);
  if (token === undefined && hash === undefined && pid === undefined) return null;
  return { token, hash, pid };
}

function renderChatterThread(thread: HTMLElement, payload: unknown): void {
  clearElementChildren(thread);
  const messages = chatterMessages(payload);
  if (messages.length === 0) {
    const empty = document.createElement("p");
    empty.className = "gorp-chatter-empty text-muted";
    empty.textContent = "No messages.";
    thread.append(empty);
    return;
  }
  for (const message of messages) thread.append(renderChatterMessage(message));
}

function chatterMessages(payload: unknown): Record<string, unknown>[] {
  if (!isRecord(payload)) return [];
  const data = isRecord(payload.data) ? payload.data : {};
  const rows = Array.isArray(data["mail.message"])
    ? (data["mail.message"] as unknown[]).filter(isRecord)
    : Array.isArray(payload.messages) && payload.messages.every(isRecord)
      ? (payload.messages as Record<string, unknown>[])
      : [];
  if (!Array.isArray(payload.messages) || rows.length === 0) return rows;
  const byID = new Map(rows.map((row) => [String(row.id ?? ""), row]));
  const ordered = payload.messages
    .map((id) => byID.get(String(id)))
    .filter((row): row is Record<string, unknown> => row !== undefined);
  return ordered.length ? ordered : rows;
}

function renderChatterMessage(message: Record<string, unknown>): HTMLElement {
  const article = document.createElement("article");
  article.className = ["gorp-chatter-message", "o-mail-Message", message.is_message_subtype_note ? "o-note" : ""]
    .filter(Boolean)
    .join(" ");
  if (firstValue(message.id) !== undefined) article.dataset.messageId = String(message.id);
  const avatarURL = firstText(message.author_avatar_url);
  if (avatarURL) {
    const avatar = document.createElement("img");
    avatar.className = "gorp-chatter-avatar o_avatar o-mail-Message-avatar";
    avatar.src = avatarURL;
    avatar.alt = chatterAuthorName(message);
    article.append(avatar);
  }
  const content = document.createElement("div");
  content.className = "gorp-chatter-message-content";
  const meta = document.createElement("div");
  meta.className = "gorp-chatter-message-meta";
  const author = document.createElement("span");
  author.className = "o-mail-Message-author";
  author.textContent = chatterAuthorName(message);
  meta.append(author);
  const published = firstText(message.published_date_str, message.date);
  if (published) {
    const date = document.createElement("time");
    date.className = "o-mail-Message-date";
    date.textContent = published;
    meta.append(date);
  }
  const body = document.createElement("div");
  body.className = "o-mail-Message-body";
  body.textContent = chatterBodyText(message.body);
  content.append(meta, body);
  const attachments = renderChatterAttachments(message.attachment_ids);
  if (attachments) content.append(attachments);
  const reactions = renderChatterReactions(message.reactions);
  if (reactions) content.append(reactions);
  article.append(content);
  return article;
}

function chatterAuthorName(message: Record<string, unknown>): string {
  const author = isRecord(message.author_id) ? message.author_id : isRecord(message.author_guest_id) ? message.author_guest_id : {};
  return firstText(author.name, message.email_from) ?? "OdooBot";
}

function chatterBodyText(value: unknown): string {
  const body = Array.isArray(value) && value[0] === "markup" ? value[1] : value;
  if (body === null || body === undefined || body === false) return "";
  return String(body)
    .replace(/<br\s*\/?>/gi, "\n")
    .replace(/<\/p>/gi, "\n")
    .replace(/<[^>]+>/g, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function renderChatterAttachments(value: unknown): HTMLElement | null {
  if (!Array.isArray(value) || value.length === 0) return null;
  const list = document.createElement("div");
  list.className = "gorp-chatter-attachments o-mail-AttachmentList";
  for (const item of value) {
    const attachment: Record<string, unknown> = isRecord(item) ? item : { id: item };
    const chip = document.createElement("span");
    chip.className = "gorp-chatter-attachment o-mail-Attachment";
    chip.textContent = firstText(attachment.filename, attachment.name, attachment.id) ?? "Attachment";
    list.append(chip);
  }
  return list;
}

function renderChatterReactions(value: unknown): HTMLElement | null {
  if (!Array.isArray(value) || value.length === 0) return null;
  const list = document.createElement("div");
  list.className = "gorp-chatter-reactions o-mail-ReactionList";
  for (const item of value) {
    if (!isRecord(item)) continue;
    const chip = document.createElement("span");
    chip.className = "gorp-chatter-reaction o-mail-Reaction";
    chip.textContent = `${firstText(item.content) ?? ""} ${firstText(item.count) ?? ""}`.trim();
    list.append(chip);
  }
  return list.children.length ? list : null;
}

function renderFormWorkflowActionMenu(
  viewDescription: ViewDescription | undefined,
  model: string,
  recordID: number | undefined,
  fields: Record<string, unknown>,
  activeFieldNames: ReadonlySet<string>,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement | undefined {
  const showUpdateStatus = Boolean(recordID !== undefined && user.isSystem && activeFieldNames.has("state"));
  const showApprovalLog = Boolean(recordID !== undefined && activeFieldNames.has("user_can_approve") && workflowFieldAvailable(fields, "user_can_approve") && !workflowFieldRelated(fields.user_can_approve));
  const activeField = activeFieldNameForView(activeFieldNames, fields);
  const showStaticActions = Boolean(recordID !== undefined && activeField);
  if (!showUpdateStatus && !showApprovalLog && !showStaticActions && !actionMenusHaveItems(viewDescription?.actionMenus)) return undefined;
  const id = recordID;
  const workflowButtons: HTMLElement[] = [];
  const staticButtons: HTMLElement[] = [];
  if (id !== undefined && activeField) {
    staticButtons.push(renderStaticActionMenuButton("duplicate", "Duplicate", "fa fa-clone", 30, async () => {
      const newID = await options.services?.orm?.call(model, "copy", [id, {}]);
      await options.onRefresh?.();
      form.dispatchEvent(new CustomEvent("action-menu:duplicate", {
        bubbles: true,
        detail: { model, ids: [id], newId: newID }
      }));
    }));
    if (recordActiveValue(values, activeField)) {
      staticButtons.push(renderStaticActionMenuButton("archive", "Archive", "oi oi-archive", 40, async () => {
        const accepted = await confirmStaticAction(options, "Are you sure that you want to archive this record?");
        if (!accepted) return;
        await options.services?.orm?.call(model, "action_archive", [[id]]);
        await options.onRefresh?.();
        form.dispatchEvent(new CustomEvent("action-menu:archive", {
          bubbles: true,
          detail: { model, ids: [id] }
        }));
      }));
    } else {
      staticButtons.push(renderStaticActionMenuButton("unarchive", "Unarchive", "oi oi-unarchive", 45, async () => {
        await options.services?.orm?.call(model, "action_unarchive", [[id]]);
        await options.onRefresh?.();
        form.dispatchEvent(new CustomEvent("action-menu:unarchive", {
          bubbles: true,
          detail: { model, ids: [id] }
        }));
      }));
    }
  }
  if (id !== undefined && showStaticActions) {
    staticButtons.push(renderStaticActionMenuButton("delete", "Delete", "fa fa-trash-o", 50, async () => {
      const accepted = await confirmStaticAction(options, "Are you sure you want to delete this record?");
      if (!accepted) return;
      await options.services?.orm?.unlink(model, [id]);
      await options.onRefresh?.();
      form.dispatchEvent(new CustomEvent("action-menu:delete", {
        bubbles: true,
        detail: { model, ids: [id] }
      }));
    }, { className: "text-danger" }));
  }
  if (showApprovalLog) {
    const button = document.createElement("button");
    button.type = "button";
    setWorkflowActionMetadata(button, "approval_log", "fa fa-arrows-h", 100, "Approval Log");
    button.addEventListener("click", async () => {
      if (id === undefined) return;
      await options.services?.action?.doAction(approvalLogAction(model, [id], true));
      form.dispatchEvent(new CustomEvent("workflow:approval-log", {
        bubbles: true,
        detail: { model, ids: [id] }
      }));
    });
    workflowButtons.push(button);
  }
  if (showUpdateStatus) {
    const button = document.createElement("button");
    button.type = "button";
    setWorkflowActionMetadata(button, "update_status", "fa fa-code", 100, "Update Status");
    button.addEventListener("click", async () => {
      if (id === undefined) return;
      await options.services?.action?.doAction(updateStatusAction(model, [id]), actionRefreshOptions(options));
      form.dispatchEvent(new CustomEvent("workflow:update-status", {
        bubbles: true,
        detail: { model, ids: [id] }
      }));
    });
    workflowButtons.push(button);
  }
  return renderActionMenus({
    className: "gorp-form-action-menu",
    model,
    actionMenus: viewDescription?.actionMenus,
    staticActionButtons: [...staticButtons, ...workflowButtons],
    getActiveIds: () => id === undefined ? [] : [id],
    requiresSelection: false,
    root: form,
    options
  });
}

function setWorkflowActionMetadata(button: HTMLElement, key: string, iconClass: string, sequence: number, label: string): void {
  button.dataset.workflowAction = key;
  button.dataset.sequence = String(sequence);
  button.dataset.groupNumber = "1";
  button.dataset.icon = iconClass;
  button.textContent = label;
  const icon = document.createElement("i");
  icon.className = iconClass;
  icon.setAttribute("aria-hidden", "true");
  button.append(icon);
}

function updateStatusAction(model: string, ids: readonly number[]): Record<string, unknown> {
  return {
    name: "Change Document Status",
    res_model: "approval.state.update",
    type: "ir.actions.act_window",
    views: [[false, "form"]],
    view_mode: "form",
    target: "new",
    context: {
      default_res_model: model,
      default_res_ids: [...ids]
    }
  };
}

function approvalLogAction(model: string, ids: readonly number[], single: boolean): Record<string, unknown> {
  return {
    name: "Approval Log",
    res_model: "approval.log",
    type: "ir.actions.act_window",
    views: [[false, "list"]],
    view_mode: "list",
    domain: single
      ? [["model", "=", model], ["record_id", "=", ids[0]]]
      : [["model", "=", model], ["record_id", "in", [...ids]]],
    context: {
      hide_record: single,
      hide_model: true
    }
  };
}

function actionRefreshOptions(options: RenderWindowActionOptions): ActionServiceOptions {
  return {
    onClose: () => options.onRefresh?.()
  };
}

function workflowFieldAvailable(fields: Record<string, unknown>, name: string): boolean {
  return fields[name] !== undefined;
}

function workflowFieldRelated(description: unknown): boolean {
  return isRecord(description) && description.related === true;
}

function isStatusbarFieldNode(node: ViewFieldNode, description: unknown): boolean {
  return fieldTypeValue(description) === "selection" && (
    node.attrs.widget === "statusbar" ||
    node.attrs.widget === "statusbar_state_duration" ||
    node.attrs.statusbar_visible !== undefined
  );
}

function renderStatusbarField(
  model: string,
  node: ViewFieldNode,
  description: unknown,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement {
  const currentValue = String(values[node.name] ?? "");
  const visibleSelection = statusbarVisibleSelection(node.attrs.statusbar_visible);
  const workflowStates = node.name === "state" && visibleSelection.includes("WORKFLOW")
    ? normalizeStringList(values.workflow_states)
    : [];
  const durationTracking = node.attrs.widget === "statusbar_state_duration"
    ? normalizeDurationTracking(values.duration_state_tracking)
    : {};
  const items = selectionOptions(description)
    .filter(([value]) => statusbarItemVisible(value, currentValue, visibleSelection, workflowStates));
  const root = document.createElement("div");
  root.className = "gorp-statusbar o_statusbar_status";
  root.dataset.field = node.name;
  root.dataset.widget = node.attrs.widget || "statusbar";
  root.setAttribute("role", "radiogroup");
  root.setAttribute("aria-label", "Statusbar");
  const disabled = !statusbarClickable(node, values) || safeEvaluateBooleanAttr(node.attrs.readonly, values) === true;
  for (const [value, label] of items) {
    const item = document.createElement("button");
    item.type = "button";
    item.className = ["gorp-statusbar-item", "btn", "btn-secondary", "o_arrow_button", value === currentValue ? "is-selected o_arrow_button_current" : ""].filter(Boolean).join(" ");
    item.dataset.value = value;
    item.dataset.selected = value === currentValue ? "true" : "false";
    item.setAttribute("role", "radio");
    item.setAttribute("aria-checked", value === currentValue ? "true" : "false");
    if (value === currentValue) item.setAttribute("aria-current", "step");
    item.disabled = disabled || value === currentValue;
    item.textContent = label;
    item.addEventListener("click", async () => {
      if (item.disabled || value === String(values[node.name] ?? "")) return;
      await selectStatusbarItem(model, node.name, value, values, form, options);
    });
    const duration = durationTracking[value];
    if (duration !== undefined) {
      item.dataset.duration = String(duration);
      if (duration > 0) {
        const durationText = formatDuration(duration);
        item.dataset.durationText = durationText;
        item.title = durationText;
        const durationNode = document.createElement("small");
        durationNode.className = "gorp-statusbar-duration ms-2 text-muted small";
        durationNode.textContent = durationText;
        item.append(durationNode);
      }
    }
    root.append(item);
  }
  return root;
}

async function selectStatusbarItem(
  model: string,
  fieldName: string,
  value: string,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): Promise<void> {
  const recordID = numberRecordID(values.id);
  updateFormValue(values, fieldName, value, options);
  if (recordID !== undefined && options.services?.orm) {
    const specification = { [fieldName]: {} };
    if (options.services.orm.webSave) {
      await options.services.orm.webSave(model, [recordID], { [fieldName]: value }, { specification });
    } else {
      await options.services.orm.write(model, [recordID], { [fieldName]: value });
    }
    await options.onRefresh?.();
  }
  form.dispatchEvent(new CustomEvent("workflow:statusbar-update", {
    bubbles: true,
    detail: { model, id: recordID, field: fieldName, value }
  }));
}

function statusbarClickable(node: ViewFieldNode, evalContext: Record<string, unknown>): boolean {
  const parsed = node.attrs.options ? parseObjectLiteral(node.attrs.options, evalContext) : undefined;
  if (!parsed || parsed.clickable === undefined) return true;
  return boolOptionValue(parsed.clickable);
}

function boolOptionValue(value: unknown): boolean {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    return normalized !== "" && normalized !== "0" && normalized !== "false";
  }
  return Boolean(value);
}

function statusbarItemVisible(
  value: string,
  currentValue: string,
  visibleSelection: readonly string[],
  workflowStates: readonly string[]
): boolean {
  if (!visibleSelection.length) return true;
  return value === currentValue || visibleSelection.includes(value) || workflowStates.includes(value);
}

function renderFormButton(
  model: string,
  node: ViewButtonNode,
  values: Record<string, unknown>,
  activeFieldNames: ReadonlySet<string>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement {
  if (node.attrs.id === "approval_user_info") {
    return renderApprovalUserInfoButton(model, values, form, options);
  }
  const button = document.createElement("button");
  button.type = "button";
  button.className = ["gorp-form-button", node.attrs.class].filter(Boolean).join(" ");
  button.dataset.workflowAction = node.attrs.name;
  if (node.attrs.id) button.dataset.buttonId = node.attrs.id;
  if (node.attrs.validate_form) button.dataset.validateForm = node.attrs.validate_form;
  button.textContent = node.attrs.string || node.attrs.name || "";
  button.addEventListener("click", async (event) => {
    event.preventDefault?.();
    if (node.attrs.confirm) {
      const accepted = options.confirm
        ? await options.confirm(node.attrs.confirm)
        : typeof globalThis.confirm === "function"
          ? globalThis.confirm(node.attrs.confirm)
          : true;
      if (!accepted) return;
    }
    if ((node.attrs.type || "object") === "action") {
      const recordID = numberRecordID(values.id);
      const ids = recordID === undefined ? [] : [recordID];
      const context = parseContextAttribute(node.attrs.context, values);
      await options.services?.action?.doAction(actionButtonRequest(node.attrs.name), {
        additionalContext: {
          ...(context ?? {}),
          ...activeIdsContext(model, ids, options.activeDomain)
        }
      });
      await options.onRefresh?.();
      form.dispatchEvent(new CustomEvent("workflow:action-button", {
        bubbles: true,
        detail: { model, action: node.attrs.name, id: recordID }
      }));
      return;
    }
    const proceed = await beforeExecuteApprovalButton(node, values, activeFieldNames, form, options);
    if (!proceed) return;
    const recordID = numberRecordID(values.id);
    const buttonArgs = parseButtonArgs(node.attrs.args, values);
    const context = parseContextAttribute(node.attrs.context, values);
    const result = await options.services?.dataset?.callButton(
      model,
      node.attrs.name,
      [[...(recordID === undefined ? [] : [recordID])], ...buttonArgs],
      context ? { context } : {}
    );
    if (isRecord(result) && options.services?.action) {
      await options.services.action.doAction(result);
    }
    await options.onRefresh?.();
    form.dispatchEvent(new CustomEvent("workflow:button", {
      bubbles: true,
      detail: { model, method: node.attrs.name, id: recordID, args: buttonArgs, result }
    }));
  });
  return button;
}

function actionButtonRequest(name: string | undefined): ActionRequest {
  const raw = (name ?? "").trim();
  const interpolated = raw.match(/^%\((.+)\)[ds]$/);
  const value = (interpolated ? interpolated[1] : raw).trim();
  if (/^\d+$/.test(value)) return Number.parseInt(value, 10);
  return value;
}

function renderApprovalUserInfoButton(
  model: string,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "gorp-form-button gorp-approval-user-info-button";
  button.dataset.workflowAction = "approval_user_info";
  button.dataset.buttonId = "approval_user_info";
  button.textContent = "";
  button.addEventListener("click", async (event) => {
    event.preventDefault?.();
    const recordID = numberRecordID(values.id);
    const specification = {
      approval_user_ids: { fields: { display_name: {} } },
      approval_done_user_ids: { fields: { display_name: {} } }
    };
    let props: Record<string, unknown> = values;
    if (recordID !== undefined && options.services?.orm) {
      const rows = await options.services.orm.webRead<Record<string, unknown>[]>(model, [recordID], { specification });
      if (Array.isArray(rows) && rows[0]) props = rows[0];
    }
    const previous = findDirectChildByClass(form, "gorp-approval-user-info-popover");
    if (previous) removeChildElement(form, previous);
    const popover = renderApprovalUserInfoPopover(props, {
      showLoginAs: Boolean(options.debug && user.isSystem),
      uid: user.userId,
      redirect: approvalUserInfoRedirect(options)
    });
    form.append(popover);
    form.dispatchEvent(new CustomEvent("workflow:approval-user-info", {
      bubbles: true,
      detail: { model, id: recordID, props }
    }));
  });
  return button;
}

function renderApprovalUserInfoPopover(
  props: Record<string, unknown>,
  options: { showLoginAs: boolean; uid: number | null; redirect: string }
): HTMLElement {
  const root = document.createElement("div");
  root.className = "gorp-approval-user-info-popover";
  const table = document.createElement("table");
  table.className = "gorp-approval-user-info-table";
  const tbody = document.createElement("tbody");
  for (const item of normalizeUserInfoRecords(props.approval_done_user_ids)) {
    tbody.append(renderApprovalUserInfoRow(item, true, options));
  }
  const waiting = document.createElement("tr");
  const waitingCell = document.createElement("td");
  waitingCell.colSpan = 4;
  waitingCell.textContent = "Waiting Approval";
  waiting.append(waitingCell);
  tbody.append(waiting);
  for (const item of normalizeUserInfoRecords(props.approval_user_ids)) {
    tbody.append(renderApprovalUserInfoRow(item, false, options));
  }
  table.append(tbody);
  root.append(table);
  return root;
}

function renderApprovalUserInfoRow(
  item: { id: number; displayName: string },
  done: boolean,
  options: { showLoginAs: boolean; uid: number | null; redirect: string }
): HTMLElement {
  const row = document.createElement("tr");
  row.dataset.userId = String(item.id);
  row.dataset.done = done ? "true" : "false";
  const avatarCell = document.createElement("td");
  const avatar = document.createElement("img");
  avatar.className = "gorp-approval-user-avatar";
  avatar.src = `/web/image/res.users/${item.id}/avatar_128`;
  avatarCell.append(avatar);
  const nameCell = document.createElement("td");
  nameCell.textContent = item.displayName;
  const doneCell = document.createElement("td");
  if (done) {
    const icon = document.createElement("i");
    icon.className = "fa fa-thumbs-up text-success";
    doneCell.append(icon);
  }
  const loginCell = document.createElement("td");
  if (options.showLoginAs && item.id !== options.uid) {
    const link = document.createElement("a");
    link.className = "fa fa-sign-in";
    link.href = `/web/login_as/${encodeURIComponent(String(item.id))}?redirect=${encodeURIComponent(options.redirect)}`;
    link.title = `Login As ${item.displayName}`;
    loginCell.append(link);
  }
  row.append(avatarCell, nameCell, doneCell, loginCell);
  return row;
}

function normalizeUserInfoRecords(value: unknown): Array<{ id: number; displayName: string }> {
  if (!Array.isArray(value)) return [];
  const out: Array<{ id: number; displayName: string }> = [];
  for (const item of value) {
    if (Array.isArray(item)) {
      const id = numberRecordID(item[0]);
      if (id !== undefined) out.push({ id, displayName: String(item[1] ?? id) });
    } else if (isRecord(item)) {
      const id = numberRecordID(item.id);
      if (id !== undefined) out.push({ id, displayName: String(item.display_name ?? item.name ?? id) });
    }
  }
  return out;
}

function approvalUserInfoRedirect(options: RenderWindowActionOptions): string {
  if (typeof options.location === "string" && options.location) return options.location;
  const location = browser.location;
  if (!location) return "/web";
  return `${location.pathname}${location.search}${location.hash}`;
}

function findDirectChildByClass(parent: HTMLElement, className: string): HTMLElement | undefined {
  return Array.from(parent.children).find((child) => String((child as HTMLElement).className ?? "").split(/\s+/).includes(className)) as HTMLElement | undefined;
}

function removeChildElement(parent: HTMLElement, child: HTMLElement) {
  const children = (parent as unknown as { children?: unknown }).children;
  if (Array.isArray(children)) {
    const index = children.indexOf(child);
    if (index >= 0) children.splice(index, 1);
    return;
  }
  child.remove?.();
}

async function beforeExecuteApprovalButton(
  node: ViewButtonNode,
  values: Record<string, unknown>,
  activeFieldNames: ReadonlySet<string>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): Promise<boolean> {
  const validateForm = safeEvaluateBooleanAttr(node.attrs.validate_form, values);
  if (validateForm === false) {
    return true;
  }
  if (node.attrs.name?.startsWith("approval") && activeFieldNames.has("approved_button_clicked")) {
    const args = parseButtonArgs(node.attrs.args, values);
    updateFormValue(values, "approved_button_clicked", args.length ? args[0] : true, options);
  }
  if (node.attrs.approved_button_clicked !== undefined) {
    updateFormValue(values, "approved_button_clicked", safeEvaluateAttr(node.attrs.approved_button_clicked, values), options);
  }
  if (!validateForm) {
    return validateRequiredFormFields(form);
  }
  if (!validateRequiredFormFields(form)) {
    return false;
  }
  if (options.validateForm) {
    return Boolean(await options.validateForm({ form, values, button: node }));
  }
  if (typeof (form as HTMLFormElement).checkValidity === "function") {
    return (form as HTMLFormElement).checkValidity();
  }
  return true;
}

type RequiredFormControl = (HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement) & { dataset: DOMStringMap };

function renderEditableFormField(
  node: ViewFieldNode,
  description: unknown,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement {
  const fieldType = fieldTypeValue(description);
  const control = fieldType === "text" || fieldType === "html" || node.attrs.widget === "text"
    ? document.createElement("textarea")
    : document.createElement("input");
  control.className = "gorp-form-control o_input";
  control.dataset.field = node.name;
  control.dataset.requiredField = node.name;
  control.setAttribute("aria-required", "true");
  control.setAttribute("aria-invalid", "false");
  control.required = true;
  control.name = node.name;
  control.value = formatCellValue(values[node.name]);
  if (control.tagName.toLowerCase() === "input") (control as HTMLInputElement).type = "text";
  if (control.tagName.toLowerCase() === "textarea") (control as HTMLTextAreaElement).rows = 3;
  control.addEventListener("input", () => {
    values[node.name] = control.value;
    if (!requiredControlEmpty(control as RequiredFormControl)) setRequiredControlInvalid(control as RequiredFormControl, false);
    emitFieldUpdate(form, options.onUpdate, node.name, control.value);
  });
  return control;
}

function formFieldRequired(node: ViewFieldNode, description: unknown, evalContext: Record<string, unknown>): boolean {
  const attrValue = safeEvaluateBooleanAttr(node.attrs.required, evalContext);
  if (attrValue !== undefined) return attrValue;
  return isRecord(description) && description.required === true;
}

function formFieldEditable(node: ViewFieldNode, description: unknown, evalContext: Record<string, unknown>): boolean {
  const readonly = safeEvaluateBooleanAttr(node.attrs.readonly, evalContext);
  if (readonly === true) return false;
  const fieldType = fieldTypeValue(description);
  return fieldType === "" || fieldType === "char" || fieldType === "text" || fieldType === "html" || node.attrs.widget === "text";
}

function validateRequiredFormFields(form: HTMLElement): boolean {
  const controls = collectRequiredFormControls(form);
  let firstInvalid: RequiredFormControl | undefined;
  for (const control of controls) {
    const invalid = !control.disabled && requiredControlEmpty(control);
    setRequiredControlInvalid(control, invalid);
    if (invalid && !firstInvalid) firstInvalid = control;
  }
  firstInvalid?.focus?.();
  return firstInvalid === undefined;
}

function collectRequiredFormControls(root: HTMLElement): RequiredFormControl[] {
  const out: RequiredFormControl[] = [];
  const visit = (node: Element) => {
    if (isRequiredFormControl(node)) out.push(node);
    for (const child of Array.from(node.children)) visit(child);
  };
  visit(root);
  return out;
}

function isRequiredFormControl(node: unknown): node is RequiredFormControl {
  if (!isRecord(node)) return false;
  const dataset = node.dataset;
  return isRecord(dataset) && typeof dataset.requiredField === "string" && typeof node.value === "string";
}

function requiredControlEmpty(control: RequiredFormControl): boolean {
  return control.value.trim().length === 0;
}

function setRequiredControlInvalid(control: RequiredFormControl, invalid: boolean): void {
  control.setAttribute("aria-invalid", invalid ? "true" : "false");
  control.className = toggleClassToken(String(control.className ?? ""), "is-invalid", invalid);
}

function toggleClassToken(value: string, token: string, enabled: boolean): string {
  const tokens = value.split(/\s+/).filter(Boolean);
  const hasToken = tokens.includes(token);
  if (enabled && !hasToken) tokens.push(token);
  if (!enabled && hasToken) return tokens.filter((item) => item !== token).join(" ");
  return tokens.join(" ");
}

function updateFormValue(values: Record<string, unknown>, name: string, value: unknown, options: RenderWindowActionOptions) {
  values[name] = value;
  options.onUpdate?.(name, value);
}

function renderResUserGroupIdsField(
  node: ViewFieldNode,
  fields: Record<string, unknown>,
  values: Record<string, unknown>,
  form: HTMLElement,
  onUpdate?: (name: string, value: unknown) => void
): HTMLElement {
  const field = document.createElement("fieldset");
  field.className = "gorp-form-field gorp-res-user-group-ids";
  field.dataset.field = node.name;
  if (typeof values.role === "string") field.dataset.role = values.role;
  const legend = document.createElement("legend");
  legend.textContent = fieldLabel(fields, node.name);
  field.append(legend);
  const selected = new Set(normalizeGroupIds(values[node.name]));
  const controls = normalizeResUserGroupControls(values, selected);
  for (const section of groupControlsBySection(controls)) {
    const group = document.createElement("section");
    group.className = "gorp-res-user-group-section";
    group.dataset.section = section.name;
    const heading = document.createElement("h2");
    heading.textContent = section.name;
    group.append(heading);
    for (const privilege of section.privileges) {
      const label = document.createElement("label");
      label.className = "gorp-res-user-group-privilege";
      label.dataset.privilegeId = String(privilege.id);
      const caption = document.createElement("span");
      caption.textContent = privilege.name;
      const select = document.createElement("select");
      select.dataset.privilegeId = String(privilege.id);
      const empty = document.createElement("option");
      empty.value = "";
      empty.textContent = privilege.placeholder;
      select.append(empty);
      for (const option of privilege.options) {
        const item = document.createElement("option");
        item.value = String(option.id);
        item.textContent = option.name;
        item.dataset.groupId = String(option.id);
        applyGroupDebugMetadata(item, option);
        select.append(item);
      }
      const current = privilege.options.find((option) => selected.has(option.id));
      select.value = current ? String(current.id) : "";
      select.addEventListener("change", () => {
        for (const option of privilege.options) selected.delete(option.id);
        const id = Number(select.value);
        if (Number.isFinite(id) && id > 0) selected.add(id);
        emitFieldUpdate(form, onUpdate, node.name, [x2ManyCommands.set(orderedSelectedGroupIds(controls.options, selected))]);
      });
      label.append(caption, select);
      group.append(label);
    }
    for (const option of section.extras) {
      const label = document.createElement("label");
      label.className = "gorp-res-user-group-option";
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.value = String(option.id);
      checkbox.checked = selected.has(option.id);
      checkbox.dataset.groupId = String(option.id);
      applyGroupDebugMetadata(checkbox, option);
      checkbox.addEventListener("change", () => {
        if (checkbox.checked) {
          selected.add(option.id);
        } else {
          selected.delete(option.id);
        }
        emitFieldUpdate(form, onUpdate, node.name, [x2ManyCommands.set(orderedSelectedGroupIds(controls.options, selected))]);
      });
      const caption = document.createElement("span");
      caption.textContent = option.name;
      applyGroupDebugMetadata(label, option);
      label.append(checkbox, caption);
      group.append(label);
    }
    field.append(group);
  }
  return field;
}

function threadPayload(thread: PortalThreadRef): Record<string, unknown> {
  const threadModel = typeof thread.thread_model === "string" ? thread.thread_model : thread.threadModel;
  const threadID = typeof thread.thread_id === "number" ? thread.thread_id : thread.threadId;
  if (!threadModel) throw new Error("thread model is required");
  if (!threadID) throw new Error("thread id is required");
  return { thread_model: threadModel, thread_id: threadID };
}

function threadFormFields(thread: PortalThreadRef): Record<string, string> {
  const payload = threadPayload(thread);
  return {
    thread_model: String(payload.thread_model),
    thread_id: String(payload.thread_id)
  };
}

function appendFormFields(formData: FormData, fields: Record<string, unknown>): void {
  for (const [key, value] of Object.entries(fields)) {
    const normalized = firstValue(value);
    if (normalized !== undefined) formData.set(key, String(normalized));
  }
}

function firstText(...values: unknown[]): string | undefined {
  for (const value of values) {
    const normalized = firstValue(value);
    if (normalized !== undefined) return String(normalized);
  }
  return undefined;
}

function firstValue(value: unknown): unknown {
  if (value === null || value === undefined || value === false) return undefined;
  if (typeof value === "string" && value.trim() === "") return undefined;
  return value;
}

async function fetchRPCTransport(request: RPCRequest): Promise<unknown> {
  if (typeof fetch !== "function") {
    throw new Error("fetch is required for RPC transport");
  }
  const response = await fetch(request.route, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: request.id, params: request.params })
  });
  const payload = (await response.json()) as unknown;
  if (isRecord(payload) && "error" in payload) {
    throw rpcPayloadError(payload.error);
  }
  if (isRecord(payload) && "result" in payload) return payload.result;
  return payload;
}

async function fetchPortalMailUpload(request: PortalMailUploadRequest): Promise<unknown> {
  if (typeof fetch !== "function") {
    throw new Error("fetch is required for portal mail attachment upload");
  }
  const response = await fetch(request.route, {
    method: "POST",
    body: request.formData
  });
  const payload = (await response.json()) as unknown;
  if (isRecord(payload) && "error" in payload) {
    throw rpcPayloadError(payload.error);
  }
  if (isRecord(payload) && "result" in payload) return payload.result;
  return payload;
}

function rpcPayloadError(errorPayload: unknown): Error {
  let message = "RPC Error";
  if (typeof errorPayload === "string" && errorPayload) {
    message = errorPayload;
  } else if (isRecord(errorPayload)) {
    const data = isRecord(errorPayload.data) ? errorPayload.data : {};
    const dataMessage = typeof data.message === "string" ? data.message : "";
    const errorMessage = typeof errorPayload.message === "string" ? errorPayload.message : "";
    message = dataMessage || errorMessage || message;
  } else if (errorPayload !== undefined && errorPayload !== null) {
    message = String(errorPayload);
  }
  const error = new Error(message) as Error & { rpcError?: unknown };
  error.rpcError = errorPayload;
  return error;
}

function defaultActionExecutor(invocation: ActionInvocation): unknown {
  return invocation.action;
}

async function defaultActionLoader(actionRequest: ActionRequest): Promise<Record<string, unknown>> {
  if (typeof actionRequest === "string" && registries.actions.contains(actionRequest)) {
    return {
      target: "current",
      tag: actionRequest,
      type: "ir.actions.client"
    };
  }
  if (isRecord(actionRequest)) return { ...actionRequest };
  throw new Error("Action loader requires an RPC service for string or numeric action requests");
}

function normalizeViewDescriptions(result: Record<string, unknown>, resModel: string): ViewDescriptions {
  const models = isRecord(result.models) ? result.models : {};
  const modelInfo = isRecord(models[resModel]) ? models[resModel] as Record<string, unknown> : {};
  const fields = isRecord(modelInfo.fields) ? modelInfo.fields as Record<string, unknown> : {};
  const rawViews = isRecord(result.views)
    ? result.views
    : isRecord(result.fields_views)
      ? result.fields_views
      : {};
  const views: Record<string, ViewDescription> = {};
  for (const [viewType, rawView] of Object.entries(rawViews)) {
    if (!isRecord(rawView)) continue;
    const description: ViewDescription = {
      arch: typeof rawView.arch === "string" ? rawView.arch : "",
      id: typeof rawView.id === "number" ? rawView.id : false
    };
    if (typeof rawView.custom_view_id === "number" || rawView.custom_view_id === false) {
      description.custom_view_id = rawView.custom_view_id;
    }
    if (isRecord(rawView.toolbar)) description.actionMenus = rawView.toolbar;
    if (Array.isArray(rawView.filters)) description.irFilters = [...rawView.filters];
    views[viewType] = description;
  }
  return { fields, relatedModels: models, views };
}

function normalizeActionViews(action: Record<string, unknown>): ViewRef[] {
  const rawViews = Array.isArray(action.views) ? action.views : [];
  const views: ViewRef[] = [];
  for (const rawView of rawViews) {
    if (!Array.isArray(rawView) || rawView.length < 2) continue;
    const type = typeof rawView[1] === "string" ? rawView[1].trim() : "";
    if (!type) continue;
    const id = typeof rawView[0] === "number" && rawView[0] > 0 ? rawView[0] : false;
    views.push([id, type]);
  }
  if (views.length) return ensureSearchActionView(action, views);
  const viewMode = typeof action.view_mode === "string" && action.view_mode.trim() ? action.view_mode : "list,form";
  const modes = viewMode
    .split(",")
    .map((type) => type.trim())
    .filter(Boolean);
  const viewID = actionViewID(action.view_id);
  if (modes.length > 1 && viewID !== false) {
    throw new Error(`Non-db action dictionaries should provide either multiple view modes or a single view mode and an optional view id: got view modes ${modes.join(", ")} and view id ${viewID}`);
  }
  return ensureSearchActionView(action, modes.map((type) => [modes.length === 1 ? viewID : false, type]));
}

function actionViewID(value: unknown): number | false {
  if (typeof value === "number" && value > 0) return value;
  if (Array.isArray(value) && typeof value[0] === "number" && value[0] > 0) return value[0];
  return false;
}

function actionSearchViewID(action: Record<string, unknown>): number | false {
  return actionViewID(action.search_view_id);
}

function ensureSearchActionView(action: Record<string, unknown>, views: ViewRef[]): ViewRef[] {
  if (views.some((view) => view[1] === "search")) return views;
  return [...views, [actionSearchViewID(action), "search"]];
}

function firstRenderableView(views: readonly ViewRef[]): string {
  return views.find((view) => view[1] !== "search")?.[1] ?? views[0]?.[1] ?? "form";
}

function actionOptionsContext(options: ActionServiceOptions): Record<string, unknown> {
  return {
    ...(isRecord(options.additional_context) ? options.additional_context : {}),
    ...(isRecord(options.additionalContext) ? options.additionalContext : {})
  };
}

async function loadWindowActionRecords(
  orm: ORMService,
  action: Record<string, unknown>,
  activeView: string,
  resModel: string,
  context: Record<string, unknown>,
  viewDescriptions: ViewDescriptions,
  searchState?: SearchModelState
): Promise<{ records: Record<string, unknown>[]; length: number }> {
  const specification = readSpecification(viewDescriptions.views[activeView]?.arch ?? "", viewDescriptions, context);
  const readContext = { bin_size: true, ...context, ...(searchState?.context ?? {}) };
  if (activeView === "form" && typeof action.res_id === "number" && Number.isFinite(action.res_id)) {
    const records = await orm.webRead<Record<string, unknown>[]>(resModel, [action.res_id], { context: readContext, specification });
    return { records, length: records.length };
  }
  if (activeView === "form") {
    const fieldNames = Object.keys(specification);
    const defaults = fieldNames.length
      ? await orm.defaultGet<Record<string, unknown>>(resModel, fieldNames, { context })
      : {};
    return { records: [isRecord(defaults) ? defaults : {}], length: 0 };
  }
  const result = await orm.webSearchRead<{ records?: Record<string, unknown>[]; length?: number }>(
    resModel,
    searchState ? [...searchState.domain] : normalizeDomainExpression(action.domain, context),
    {
      context: readContext,
      specification,
      limit: numberActionValue(action.limit, 80),
      ...(searchState?.groupBy.length ? { groupby: [...searchState.groupBy] } : {}),
      ...(typeof action.order === "string" ? { order: action.order } : {})
    }
  );
  const records = Array.isArray(result.records) ? result.records : [];
  return { records, length: typeof result.length === "number" ? result.length : records.length };
}

function buildWindowActionSearch(
  action: Record<string, unknown>,
  context: Record<string, unknown>,
  viewDescriptions: ViewDescriptions
): WindowActionSearchState {
  const searchView = viewDescriptions.views.search;
  const parsed = parseActionSearchArch(searchView?.arch ?? "", {
    context,
    irFilters: searchView?.irFilters ?? []
  });
  const explicitFacets = searchFacetsFromAction(action);
  const model = createActionSearchModel({
    facets: explicitFacets ?? parsed.defaultFacets,
    query: typeof action.__search_query === "string" ? action.__search_query : "",
    searchFields: validSearchFields(parsed.searchFields, viewDescriptions.fields),
    baseDomain: normalizeDomainExpression(action.domain, context),
    baseContext: context
  });
  const state = model.state;
  const activeFacetIDs = new Set(state.facets.map((facet) => facet.id));
  const filters = searchMenuItems(parsed.filters, activeFacetIDs);
  const groupBys = searchMenuItems(parsed.groupBys, activeFacetIDs);
  return {
    parsed,
    state,
    filters: filters.length ? filters : fallbackFilterMenuItems(viewDescriptions.fields),
    groupBys: groupBys.length ? groupBys : fallbackGroupByMenuItems(viewDescriptions.fields),
    favorites: searchMenuItems(parsed.favorites, activeFacetIDs)
  };
}

function validSearchFields(searchFields: readonly string[], fields: Record<string, unknown>): string[] | undefined {
  const out = searchFields
    .map((field) => String(field ?? "").trim())
    .filter((field, index, all) => field && all.indexOf(field) === index && (field === "display_name" || fields[field]));
  return out.length ? out : undefined;
}

function searchMenuItems(items: readonly ParsedSearchItem[], activeFacetIDs: ReadonlySet<string>): ActionControlPanelMenuItem[] {
  return items.map((item) => ({
    id: item.id,
    label: item.label,
    facet: parsedSearchItemFacet(item),
    active: activeFacetIDs.has(parsedSearchItemFacet(item).id)
  }));
}

function fallbackFilterMenuItems(fields: Record<string, unknown>): ActionControlPanelMenuItem[] {
  const items: ActionControlPanelMenuItem[] = [];
  if (fields.active) {
    items.push(
      { id: "filter-active", label: "Active", facet: { id: "filter-active", type: "filter", label: "Active", domain: [["active", "=", true]] } },
      { id: "filter-archived", label: "Archived", facet: { id: "filter-archived", type: "filter", label: "Archived", domain: [["active", "=", false]], context: { active_test: false } } }
    );
  }
  if (fields.state) {
    const codeLabel = selectionOptions(fields.state).find(([value]) => value === "code")?.[1];
    items.push({ id: "filter-code", label: codeLabel || fieldLabel(fields, "state"), facet: { id: "filter-code", type: "filter", label: codeLabel || fieldLabel(fields, "state"), domain: [["state", "=", "code"]] } });
  }
  return dedupeMenuItems(items);
}

function fallbackGroupByMenuItems(fields: Record<string, unknown>): ActionControlPanelMenuItem[] {
  const preferred: Array<[string, string]> = [
    ["model_id", "group-by-group_model"],
    ["binding_model_id", "group-by-binding_model"],
    ["state", "group-by-state"],
    ["create_uid", "group-by-create_uid"],
    ["write_uid", "group-by-write_uid"],
    ["user_id", "group-by-user_id"]
  ];
  const items: ActionControlPanelMenuItem[] = [];
  for (const [name, id] of preferred) {
    if (fields[name]) items.push({ id, label: fieldLabel(fields, name), facet: { id, type: "groupBy", label: fieldLabel(fields, name), field: name } });
  }
  if (!items.length) {
    for (const [name, description] of Object.entries(fields)) {
      if (name === "id" || name === "display_name") continue;
      if (!["many2one", "selection", "boolean"].includes(fieldTypeValue(description))) continue;
      items.push({ id: `group-by-${name}`, label: fieldLabel(fields, name), facet: { id: `group-by-${name}`, type: "groupBy", label: fieldLabel(fields, name), field: name } });
      if (items.length >= 3) break;
    }
  }
  return dedupeMenuItems(items);
}

function searchFacetsFromAction(action: Record<string, unknown>): SearchFacet[] | undefined {
  if (!Array.isArray(action.__search_facets)) return undefined;
  const facets: SearchFacet[] = [];
  for (const raw of action.__search_facets) {
    if (!isRecord(raw) || typeof raw.id !== "string" || typeof raw.type !== "string" || typeof raw.label !== "string") continue;
    facets.push({
      id: raw.id,
      type: raw.type as SearchFacet["type"],
      label: raw.label,
      ...(typeof raw.categoryLabel === "string" ? { categoryLabel: raw.categoryLabel } : {}),
      ...(Array.isArray(raw.valueLabels) ? { valueLabels: raw.valueLabels.map((item) => String(item)) } : {}),
      ...(typeof raw.field === "string" ? { field: raw.field } : {}),
      ...(typeof raw.operator === "string" ? { operator: raw.operator } : {}),
      ...("value" in raw ? { value: raw.value } : {}),
      ...(Array.isArray(raw.domain) ? { domain: [...raw.domain] } : {}),
      ...(isRecord(raw.context) ? { context: { ...raw.context } } : {}),
      ...(Array.isArray(raw.groupBy) ? { groupBy: raw.groupBy.map((item) => String(item)) } : {}),
      ...(typeof raw.interval === "string" ? { interval: raw.interval as SearchFacet["interval"] } : {}),
      ...(raw.group !== undefined ? { group: raw.group as string | number } : {})
    });
  }
  return facets;
}

function dedupeMenuItems(items: readonly ActionControlPanelMenuItem[]): ActionControlPanelMenuItem[] {
  const seen = new Set<string>();
  const out: ActionControlPanelMenuItem[] = [];
  for (const item of items) {
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    out.push(item);
  }
  return out;
}

function readSpecification(arch: string, viewDescriptions: ViewDescriptions, evalContext: Record<string, unknown>): ReadSpecification {
  return fieldNodesToSpecification(parseViewFieldNodes(arch), viewDescriptions.fields, viewDescriptions, evalContext);
}

function normalizeDomainExpression(value: unknown, evalContext: Record<string, unknown> = {}): DomainExpression {
  if (Array.isArray(value)) return [...value];
  if (typeof value === "string") {
    const parsed = parsePythonLiteral(value, evalContext);
    if (parsed.ok && Array.isArray(parsed.value)) return parsed.value;
  }
  return [];
}

function numberActionValue(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function envIsSmall(env: WebClientEnv | null): boolean {
  try {
    return Boolean(env?.isSmall);
  } catch {
    return false;
  }
}

function viewFieldNames(arch: string, fields: Record<string, unknown>, evalContext: Record<string, unknown> = {}): string[] {
  const names = parseViewFieldNodes(arch)
    .filter((node) => !fieldInvisible(node, evalContext))
    .map((node) => node.name);
  if (names.length) return names;
  return Object.keys(fields)
    .filter((name) => name !== "id" && name !== "display_name")
    .slice(0, 6);
}

interface ViewFieldNode {
  name: string;
  attrs: Record<string, string>;
  children: ViewFieldNode[];
  childViewAttrs: Record<string, string>;
}

export interface ViewButtonNode {
  attrs: Record<string, string>;
}

function parseViewFieldNodes(arch: string): ViewFieldNode[] {
  if (!arch) return [];
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      const root = doc.documentElement;
      return fieldNodesFromElement(root);
    } catch {
      return fieldNodesFromXMLText(arch);
    }
  }
  return fieldNodesFromXMLText(arch);
}

function parseViewButtonNodes(arch: string): ViewButtonNode[] {
  if (!arch) return [];
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      return buttonNodesFromElement(doc.documentElement);
    } catch {
      return buttonNodesFromXMLText(arch);
    }
  }
  return buttonNodesFromXMLText(arch);
}

function viewRootAttrs(arch: string, ...tags: string[]): Record<string, string> {
  if (!arch) return {};
  const tagSet = new Set(tags.map((tag) => tag.toLowerCase()));
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      const root = doc.documentElement;
      if (root && tagSet.has(root.tagName.toLowerCase())) return elementAttributes(root);
    } catch {
      return viewRootAttrsFromXMLText(arch, tagSet);
    }
  }
  return viewRootAttrsFromXMLText(arch, tagSet);
}

function viewRootAttrsFromXMLText(arch: string, tags: ReadonlySet<string>): Record<string, string> {
  const match = arch.match(/<([\w:.-]+)(?:\s+[^<>]*)?>/);
  if (!match || !tags.has(match[1].toLowerCase())) return {};
  return xmlAttributes(match[0]);
}

function fieldNodesFromElement(element: Element): ViewFieldNode[] {
  const out: ViewFieldNode[] = [];
  for (const child of Array.from(element.children)) {
    if (child.tagName.toLowerCase() === "field" && child.getAttribute("name")) {
      const nestedView = Array.from(child.children).find((node) => viewContainerTags.has(node.tagName.toLowerCase()));
      const node: ViewFieldNode = {
        name: child.getAttribute("name") ?? "",
        attrs: elementAttributes(child),
        children: nestedView ? fieldNodesFromElement(nestedView) : [],
        childViewAttrs: nestedView ? elementAttributes(nestedView) : {}
      };
      out.push(node);
    } else {
      out.push(...fieldNodesFromElement(child));
    }
  }
  return out;
}

function buttonNodesFromElement(element: Element): ViewButtonNode[] {
  const out: ViewButtonNode[] = [];
  for (const child of Array.from(element.children)) {
    if (child.tagName.toLowerCase() === "button" && (child.getAttribute("name") || child.getAttribute("id"))) {
      const attrs = elementAttributes(child);
      if (attrs.id === "approval_user_info" || viewButtonTypeSupported(attrs)) out.push({ attrs });
    }
    out.push(...buttonNodesFromElement(child));
  }
  return out;
}

function fieldNodesFromXMLText(arch: string): ViewFieldNode[] {
  const roots: ViewFieldNode[] = [];
  const fieldStack: ViewFieldNode[] = [];
  const elementStack: Array<{ tag: string; field?: ViewFieldNode }> = [];
  const tokenPattern = /<\/?[\w:.-]+(?:\s+[^<>]*)?\/?>/g;
  for (const match of arch.matchAll(tokenPattern)) {
    const token = match[0];
    if (/^<\//.test(token)) {
      const tag = token.replace(/^<\//, "").replace(/>$/, "").trim().toLowerCase();
      while (elementStack.length) {
        const popped = elementStack.pop();
        if (popped?.field && fieldStack[fieldStack.length - 1] === popped.field) {
          fieldStack.pop();
        }
        if (popped?.tag === tag) break;
      }
      continue;
    }
    const tagMatch = token.match(/^<([\w:.-]+)/);
    if (!tagMatch) continue;
    const tag = tagMatch[1].toLowerCase();
    const attrs = xmlAttributes(token);
    const selfClosing = /\/>$/.test(token);
    let fieldNode: ViewFieldNode | undefined;
    if (tag === "field" && attrs.name) {
      fieldNode = { name: attrs.name, attrs, children: [], childViewAttrs: {} };
      const parent = fieldStack[fieldStack.length - 1];
      if (parent) {
        parent.children.push(fieldNode);
      } else {
        roots.push(fieldNode);
      }
      if (!selfClosing) fieldStack.push(fieldNode);
    } else if (viewContainerTags.has(tag) && fieldStack.length) {
      Object.assign(fieldStack[fieldStack.length - 1].childViewAttrs, attrs);
    }
    if (!selfClosing) elementStack.push({ tag, field: fieldNode });
  }
  return roots;
}

function buttonNodesFromXMLText(arch: string): ViewButtonNode[] {
  const out: ViewButtonNode[] = [];
  const tokenPattern = /<button\b(?:\s+[^<>]*)?\/?>/g;
  for (const match of arch.matchAll(tokenPattern)) {
    const attrs = xmlAttributes(match[0]);
    if (attrs.id === "approval_user_info" || (attrs.name && viewButtonTypeSupported(attrs))) {
      out.push({ attrs });
    }
  }
  return out;
}

function viewButtonTypeSupported(attrs: Record<string, string>): boolean {
  const type = attrs.type || "object";
  return type === "object" || type === "action";
}

const viewContainerTags = new Set(["list", "tree", "form", "kanban"]);

function elementAttributes(element: Element): Record<string, string> {
  const attrs: Record<string, string> = {};
  for (const attr of Array.from(element.attributes)) {
    attrs[attr.name] = attr.value;
  }
  return attrs;
}

function xmlAttributes(token: string): Record<string, string> {
  const attrs: Record<string, string> = {};
  const attrPattern = /([\w:.-]+)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;
  for (const match of token.matchAll(attrPattern)) {
    attrs[match[1]] = xmlDecodeAttribute(match[2] ?? match[3] ?? "");
  }
  return attrs;
}

function xmlDecodeAttribute(value: string): string {
  return value.replace(/&(?:#(\d+)|#x([0-9a-fA-F]+)|amp|lt|gt|quot|apos);/g, (match, decimal: string | undefined, hex: string | undefined) => {
    if (decimal) return String.fromCodePoint(Number.parseInt(decimal, 10));
    if (hex) return String.fromCodePoint(Number.parseInt(hex, 16));
    switch (match) {
      case "&amp;":
        return "&";
      case "&lt;":
        return "<";
      case "&gt;":
        return ">";
      case "&quot;":
        return "\"";
      case "&apos;":
        return "'";
      default:
        return match;
    }
  });
}

function fieldLabel(fields: Record<string, unknown>, name: string): string {
  const description = fields[name];
  if (isRecord(description) && typeof description.string === "string") return description.string;
  return name;
}

function selectionOptions(description: unknown): Array<[string, string]> {
  if (!isRecord(description) || !Array.isArray(description.selection)) return [];
  const out: Array<[string, string]> = [];
  for (const item of description.selection) {
    if (Array.isArray(item) && item.length >= 2) {
      out.push([String(item[0]), String(item[1])]);
    } else if (isRecord(item) && item.value !== undefined) {
      out.push([String(item.value), String(item.label ?? item.string ?? item.value)]);
    }
  }
  return out;
}

function statusbarVisibleSelection(value: string | undefined): string[] {
  if (!value) return [];
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function normalizeStringList(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item));
  if (typeof value === "string" && value.trim()) {
    const json = parseJSONValue(value);
    if (Array.isArray(json)) return json.map((item) => String(item));
    const parsed = parsePythonLiteral(value);
    if (parsed.ok && Array.isArray(parsed.value)) return parsed.value.map((item) => String(item));
  }
  return [];
}

function normalizeDurationTracking(value: unknown): Record<string, number> {
  let source = value;
  if (typeof value === "string" && value.trim()) {
    const json = parseJSONValue(value);
    if (json !== undefined) {
      source = json;
    } else {
      const parsed = parsePythonLiteral(value);
      source = parsed.ok ? parsed.value : undefined;
    }
  }
  if (!isRecord(source)) return {};
  const out: Record<string, number> = {};
  for (const [key, item] of Object.entries(source)) {
    const number = Number(item);
    if (Number.isFinite(number) && number >= 0) out[key] = number;
  }
  return out;
}

function parseJSONValue(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
}

function formatCellValue(value: unknown): string {
  if (value === null || value === undefined || value === false) return "";
  if (Array.isArray(value)) return value.map(formatCellValue).filter(Boolean).join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

interface ResUserGroupOption {
  id: number;
  name: string;
  section: string;
  privilegeID?: number;
  impliedIDs: number[];
  impliedByIDs: number[];
  disjointIDs: number[];
}

interface ResUserGroupPrivilegeControl {
  id: number;
  name: string;
  section: string;
  placeholder: string;
  options: ResUserGroupOption[];
}

interface ResUserGroupControls {
  privileges: ResUserGroupPrivilegeControl[];
  extras: ResUserGroupOption[];
  options: ResUserGroupOption[];
}

function emitFieldUpdate(
  form: HTMLElement,
  onUpdate: ((name: string, value: unknown) => void) | undefined,
  name: string,
  value: unknown
): void {
  onUpdate?.(name, value);
  const detail = { name, value };
  if (typeof CustomEvent === "function") {
    form.dispatchEvent(new CustomEvent("field-update", { detail, bubbles: true }));
  }
}

function normalizeResUserGroupControls(values: Record<string, unknown>, selected: ReadonlySet<number>): ResUserGroupControls {
  const options = new Map<number, ResUserGroupOption>();
  const privileges = new Map<number, ResUserGroupPrivilegeControl>();
  if (!collectResUserHierarchyControls(values.view_group_hierarchy, options, privileges)) {
    collectGroupOptions(values.view_group_hierarchy, "Access Rights", options);
  }
  collectGroupOptions(values.all_group_ids, "Other", options);
  for (const id of selected) {
    if (!options.has(id)) options.set(id, groupOption(id, String(id), "Selected"));
  }
  const sortedOptions = sortGroupOptions([...options.values()]);
  const sortedPrivileges = [...privileges.values()]
    .map((privilege) => ({ ...privilege, options: sortGroupOptions(privilege.options) }))
    .sort((a, b) => a.section.localeCompare(b.section) || a.name.localeCompare(b.name) || a.id - b.id);
  const privilegedIDs = new Set(sortedPrivileges.flatMap((privilege) => privilege.options.map((option) => option.id)));
  return {
    privileges: sortedPrivileges,
    extras: sortedOptions.filter((option) => !privilegedIDs.has(option.id)),
    options: sortedOptions
  };
}

function collectResUserHierarchyControls(
  value: unknown,
  out: Map<number, ResUserGroupOption>,
  privilegeOut: Map<number, ResUserGroupPrivilegeControl>
): boolean {
  if (!isRecord(value)) return false;
  const groups = recordsByID(value.groups);
  const privileges = recordsByID(value.privileges);
  if (!groups.size) return false;
  const usedGroups = new Set<number>();
  const usedPrivileges = new Set<number>();
  const categories = Array.isArray(value.categories) ? value.categories.filter(isRecord) : [];
  for (const category of categories) {
    const section = stringRecordValue(category, "name", "display_name") ?? "Access Rights";
    for (const privilegeID of normalizeGroupIds(category.privilege_ids)) {
      const privilege = privileges.get(privilegeID);
      if (!privilege) continue;
      usedPrivileges.add(privilegeID);
      collectPrivilegeControl(privilegeID, privilege, section, groups, out, privilegeOut, usedGroups);
    }
  }
  for (const [privilegeID, privilege] of privileges) {
    if (usedPrivileges.has(privilegeID)) continue;
    const section = stringRecordValue(privilege, "name", "display_name") ?? "Access Rights";
    collectPrivilegeControl(privilegeID, privilege, section, groups, out, privilegeOut, usedGroups);
  }
  for (const [groupID, group] of groups) {
    if (usedGroups.has(groupID)) continue;
    const privilegeID = numericRecordValue(group, "privilege_id");
    const privilege = privilegeID === undefined ? undefined : privileges.get(privilegeID);
    if (privilegeID !== undefined && privilege) {
      const section = stringRecordValue(privilege, "name", "display_name") ?? "Access Rights";
      addPrivilegeGroupOption(privilegeID, privilege, section, groupID, group, out, privilegeOut);
    } else {
      addGroupOption(out, groupID, stringRecordValue(group, "name", "display_name", "full_name") ?? String(groupID), "Extra Rights", group);
    }
  }
  return out.size > 0;
}

function collectPrivilegeControl(
  privilegeID: number,
  privilege: Record<string, unknown>,
  section: string,
  groups: Map<number, Record<string, unknown>>,
  out: Map<number, ResUserGroupOption>,
  privilegeOut: Map<number, ResUserGroupPrivilegeControl>,
  usedGroups: Set<number>
): void {
  for (const groupID of normalizeGroupIds(privilege.group_ids)) {
    const group = groups.get(groupID);
    if (!group) continue;
    addPrivilegeGroupOption(privilegeID, privilege, section, groupID, group, out, privilegeOut);
    usedGroups.add(groupID);
  }
}

function addPrivilegeGroupOption(
  privilegeID: number,
  privilege: Record<string, unknown>,
  section: string,
  groupID: number,
  group: Record<string, unknown>,
  out: Map<number, ResUserGroupOption>,
  privilegeOut: Map<number, ResUserGroupPrivilegeControl>
): void {
  const option = addGroupOption(
    out,
    groupID,
    stringRecordValue(group, "name", "display_name", "full_name") ?? String(groupID),
    section,
    group,
    privilegeID
  );
  if (!option) return;
  const control = privilegeOut.get(privilegeID) ?? {
    id: privilegeID,
    name: stringRecordValue(privilege, "name", "display_name") ?? String(privilegeID),
    section,
    placeholder: stringRecordValue(privilege, "placeholder") ?? "None",
    options: []
  };
  if (!control.options.some((item) => item.id === option.id)) control.options.push(option);
  privilegeOut.set(privilegeID, control);
}

function recordsByID(value: unknown): Map<number, Record<string, unknown>> {
  const out = new Map<number, Record<string, unknown>>();
  const add = (item: unknown): void => {
    if (!isRecord(item)) return;
    const id = numericRecordValue(item, "id");
    if (id !== undefined) out.set(id, item);
  };
  if (Array.isArray(value)) {
    for (const item of value) add(item);
  } else if (isRecord(value)) {
    for (const item of Object.values(value)) add(item);
  }
  return out;
}

function collectGroupOptions(value: unknown, section: string, out: Map<number, ResUserGroupOption>): void {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) {
    addGroupOption(out, value, String(value), section);
    return;
  }
  if (Array.isArray(value)) {
    const pairID = typeof value[0] === "number" ? value[0] : undefined;
    const pairName = typeof value[1] === "string" ? value[1] : undefined;
    if (pairID !== undefined && pairName !== undefined) {
      addGroupOption(out, pairID, pairName, section);
      return;
    }
    for (const item of value) collectGroupOptions(item, section, out);
    return;
  }
  if (!isRecord(value)) return;
  const id = numericRecordValue(value, "id");
  const name = stringRecordValue(value, "name", "display_name", "full_name", "string");
  const ownSection = stringRecordValue(value, "category", "category_name", "application", "section", "title") ?? section;
  if (id !== undefined) addGroupOption(out, id, name ?? String(id), ownSection, value);
  for (const key of ["groups", "children", "items", "entries", "group_ids", "all_group_ids"]) {
    collectGroupOptions(value[key], ownSection, out);
  }
}

function addGroupOption(
  out: Map<number, ResUserGroupOption>,
  id: number,
  name: string,
  section: string,
  record?: Record<string, unknown>,
  privilegeID?: number
): ResUserGroupOption | undefined {
  if (!Number.isFinite(id) || id <= 0) return undefined;
  const existing = out.get(id);
  if (existing) {
    if (privilegeID !== undefined && existing.privilegeID === undefined) existing.privilegeID = privilegeID;
    return existing;
  }
  const option = groupOption(id, name, section, record, privilegeID);
  out.set(id, option);
  return option;
}

function groupOption(
  id: number,
  name: string,
  section: string,
  record?: Record<string, unknown>,
  privilegeID?: number
): ResUserGroupOption {
  return {
    id,
    name,
    section,
    privilegeID,
    impliedIDs: normalizeGroupIds(record?.implied_ids),
    impliedByIDs: normalizeGroupIds(record?.all_implied_by_ids),
    disjointIDs: normalizeGroupIds(record?.disjoint_ids)
  };
}

function normalizeGroupIds(value: unknown): number[] {
  const ids: number[] = [];
  collectGroupIds(value, ids);
  return [...new Set(ids)];
}

function collectGroupIds(value: unknown, ids: number[]): void {
  if (typeof value === "number" && Number.isFinite(value) && value > 0) {
    ids.push(value);
    return;
  }
  if (Array.isArray(value)) {
    if (value[0] === x2ManyCommands.SET && Array.isArray(value[2])) {
      collectGroupIds(value[2], ids);
      return;
    }
    if (typeof value[0] === "number" && typeof value[1] === "string") {
      ids.push(value[0]);
      return;
    }
    for (const item of value) collectGroupIds(item, ids);
    return;
  }
  if (isRecord(value)) {
    const id = numericRecordValue(value, "id");
    if (id !== undefined) ids.push(id);
  }
}

function groupControlsBySection(controls: ResUserGroupControls): Array<{ name: string; privileges: ResUserGroupPrivilegeControl[]; extras: ResUserGroupOption[] }> {
  const sections = new Map<string, { name: string; privileges: ResUserGroupPrivilegeControl[]; extras: ResUserGroupOption[] }>();
  for (const privilege of controls.privileges) {
    const section = sections.get(privilege.section) ?? { name: privilege.section, privileges: [], extras: [] };
    section.privileges.push(privilege);
    sections.set(privilege.section, section);
  }
  for (const option of controls.extras) {
    const section = sections.get(option.section) ?? { name: option.section, privileges: [], extras: [] };
    section.extras.push(option);
    sections.set(option.section, section);
  }
  return [...sections.values()].sort((a, b) => a.name.localeCompare(b.name));
}

function orderedSelectedGroupIds(options: readonly ResUserGroupOption[], selected: ReadonlySet<number>): number[] {
  const ordered = options.map((option) => option.id).filter((id) => selected.has(id));
  for (const id of selected) {
    if (!ordered.includes(id)) ordered.push(id);
  }
  return ordered.sort((a, b) => a - b);
}

function sortGroupOptions(options: ResUserGroupOption[]): ResUserGroupOption[] {
  return options.sort((a, b) => a.section.localeCompare(b.section) || a.name.localeCompare(b.name) || a.id - b.id);
}

function applyGroupDebugMetadata(node: HTMLElement, option: ResUserGroupOption): void {
  if (option.impliedIDs.length) node.dataset.impliedIds = option.impliedIDs.join(",");
  if (option.impliedByIDs.length) node.dataset.impliedByIds = option.impliedByIDs.join(",");
  if (option.disjointIDs.length) node.dataset.disjointIds = option.disjointIDs.join(",");
  const parts = [
    option.impliedIDs.length ? `implies ${option.impliedIDs.join(",")}` : "",
    option.impliedByIDs.length ? `implied by ${option.impliedByIDs.join(",")}` : "",
    option.disjointIDs.length ? `incompatible ${option.disjointIDs.join(",")}` : ""
  ].filter(Boolean);
  if (parts.length) node.title = parts.join("; ");
}

function numericRecordValue(record: Record<string, unknown>, key: string): number | undefined {
  const value = record[key];
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function stringRecordValue(record: Record<string, unknown>, ...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value;
    if (Array.isArray(value) && typeof value[1] === "string" && value[1].trim()) return value[1];
  }
  return undefined;
}

export class InvalidDomainError extends Error {}

export class EvaluationError extends Error {}

export class AssertionError extends Error {}

export class ValueError extends Error {}

export class NotSupportedError extends Error {}

export function execOnIterable<T>(iterable: unknown, func: (iterable: Iterable<unknown>) => T): T {
  if (iterable === null || iterable === undefined) throw new EvaluationError("value not iterable");
  let normalized = iterable;
  if (typeof normalized === "object" && !Array.isArray(normalized) && !(normalized instanceof Set)) {
    normalized = Object.keys(normalized as Record<string, unknown>);
  }
  if (typeof (normalized as { [Symbol.iterator]?: unknown })[Symbol.iterator] !== "function") {
    throw new EvaluationError("value not iterable");
  }
  return func(normalized as Iterable<unknown>);
}

function normalizeDateArgs(args: unknown[], names: readonly string[]): Record<string, unknown> {
  if (args.length === 1 && isRecord(args[0]) && !isPythonKeywordArg(args[0])) return args[0];
  const values: Record<string, unknown> = {};
  let positionalIndex = 0;
  for (const arg of args) {
    if (isPythonKeywordArg(arg)) {
      values[arg.__pythonKeyword] = arg.value;
    } else if (positionalIndex < names.length) {
      values[names[positionalIndex]] = arg;
      positionalIndex += 1;
    }
  }
  return values;
}

function numberObjectValue(value: unknown): number {
  return value === undefined || value === null ? 0 : pythonNumber(value);
}

export class PyTimeDelta {
  readonly days: number;
  readonly seconds: number;
  readonly microseconds: number;

  constructor(days = 0, seconds = 0, microseconds = 0) {
    this.days = Math.trunc(days);
    this.seconds = Math.trunc(seconds);
    this.microseconds = Math.trunc(microseconds);
  }

  static create(...args: unknown[]): PyTimeDelta {
    const values = normalizeDateArgs(args, ["days", "seconds", "microseconds", "milliseconds", "minutes", "hours", "weeks"]);
    const days = numberObjectValue(values.days) + numberObjectValue(values.weeks) * 7;
    const seconds = numberObjectValue(values.seconds) + numberObjectValue(values.minutes) * 60 + numberObjectValue(values.hours) * 3600;
    const microseconds = numberObjectValue(values.microseconds) + numberObjectValue(values.milliseconds) * 1000;
    return new PyTimeDelta(days, seconds, microseconds);
  }

  totalSeconds(): number {
    return this.days * 86400 + this.seconds + this.microseconds / 1000000;
  }

  isTrue(): boolean {
    return this.days !== 0 || this.seconds !== 0 || this.microseconds !== 0;
  }

  toString(): string {
    return `${this.days} days, ${this.seconds} seconds`;
  }
}

export class PyRelativeDelta extends PyTimeDelta {}

export class PyDate {
  readonly year: number;
  readonly month: number;
  readonly day: number;

  constructor(year: number, month: number, day: number) {
    this.year = Math.trunc(year);
    this.month = Math.trunc(month);
    this.day = Math.trunc(day);
  }

  static today(): PyDate {
    return PyDate.convertDate(new Date());
  }

  static convertDate(date: Date): PyDate {
    return new PyDate(date.getFullYear(), date.getMonth() + 1, date.getDate());
  }

  static create(...args: unknown[]): PyDate {
    const values = normalizeDateArgs(args, ["year", "month", "day"]);
    return new PyDate(numberObjectValue(values.year), numberObjectValue(values.month), numberObjectValue(values.day));
  }

  plus(delta: PyTimeDelta | Record<string, unknown>): PyDate {
    const days = delta instanceof PyTimeDelta ? delta.days + Math.trunc(delta.seconds / 86400) : numberObjectValue(delta.__pythonDeltaDays);
    return PyDate.convertDate(new Date(Date.UTC(this.year, this.month - 1, this.day + days)));
  }

  strftime(format: string): string {
    return pythonStrftime(new Date(Date.UTC(this.year, this.month - 1, this.day)), format);
  }

  toString(): string {
    return this.strftime("%Y-%m-%d");
  }
}

export class PyDateTime extends PyDate {
  readonly hour: number;
  readonly minute: number;
  readonly second: number;
  readonly microsecond: number;

  constructor(year: number, month: number, day: number, hour = 0, minute = 0, second = 0, microsecond = 0) {
    super(year, month, day);
    this.hour = Math.trunc(hour);
    this.minute = Math.trunc(minute);
    this.second = Math.trunc(second);
    this.microsecond = Math.trunc(microsecond);
  }

  static now(): PyDateTime {
    return PyDateTime.convertDate(new Date());
  }

  static override convertDate(date: Date): PyDateTime {
    return new PyDateTime(date.getFullYear(), date.getMonth() + 1, date.getDate(), date.getHours(), date.getMinutes(), date.getSeconds(), date.getMilliseconds() * 1000);
  }

  static override create(...args: unknown[]): PyDateTime {
    const values = normalizeDateArgs(args, ["year", "month", "day", "hour", "minute", "second", "microsecond"]);
    return new PyDateTime(
      numberObjectValue(values.year),
      numberObjectValue(values.month),
      numberObjectValue(values.day),
      numberObjectValue(values.hour),
      numberObjectValue(values.minute),
      numberObjectValue(values.second),
      numberObjectValue(values.microsecond)
    );
  }

  override plus(delta: PyTimeDelta | Record<string, unknown>): PyDateTime {
    const days = delta instanceof PyTimeDelta ? delta.days + Math.trunc(delta.seconds / 86400) : numberObjectValue(delta.__pythonDeltaDays);
    return PyDateTime.convertDate(new Date(Date.UTC(this.year, this.month - 1, this.day + days, this.hour, this.minute, this.second, Math.trunc(this.microsecond / 1000))));
  }

  override strftime(format: string): string {
    return pythonStrftime(new Date(Date.UTC(this.year, this.month - 1, this.day, this.hour, this.minute, this.second, Math.trunc(this.microsecond / 1000))), format);
  }

  override toString(): string {
    return this.strftime("%Y-%m-%d %H:%M:%S");
  }
}

export class PyTime {
  readonly hour: number;
  readonly minute: number;
  readonly second: number;
  readonly microsecond: number;

  constructor(hour = 0, minute = 0, second = 0, microsecond = 0) {
    this.hour = Math.trunc(hour);
    this.minute = Math.trunc(minute);
    this.second = Math.trunc(second);
    this.microsecond = Math.trunc(microsecond);
  }

  static create(...args: unknown[]): PyTime {
    const values = normalizeDateArgs(args, ["hour", "minute", "second", "microsecond"]);
    return new PyTime(numberObjectValue(values.hour), numberObjectValue(values.minute), numberObjectValue(values.second), numberObjectValue(values.microsecond));
  }

  strftime(format: string): string {
    return pythonStrftime(new Date(Date.UTC(1970, 0, 1, this.hour, this.minute, this.second, Math.trunc(this.microsecond / 1000))), format);
  }

  toString(): string {
    return this.strftime("%H:%M:%S");
  }
}

function builtinPositionalArgs(args: unknown[]): unknown[] {
  const last = args[args.length - 1];
  if (args.length > 1 && isRecord(last) && Object.keys(last).length === 0) return args.slice(0, -1);
  return args;
}

export const BUILTINS = {
  bool(value: unknown): boolean {
    return pythonTruthy(value);
  },

  set(iterable: unknown, kwargs?: unknown): Set<unknown> {
    if (arguments.length > 2) throw new EvaluationError(`set expected at most 1 argument, got (${arguments.length - 1}`);
    void kwargs;
    return execOnIterable(iterable, (value) => new Set(value));
  },

  max(...args: unknown[]): number {
    return Math.max(...builtinPositionalArgs(args).map(pythonNumber));
  },

  min(...args: unknown[]): number {
    return Math.min(...builtinPositionalArgs(args).map(pythonNumber));
  },

  time: {
    strftime(format: string): string {
      return PyDateTime.now().strftime(format);
    }
  },

  context_today(): PyDate {
    return PyDate.today();
  },

  get current_date(): string {
    return this.today;
  },

  get today(): string {
    return PyDate.today().strftime("%Y-%m-%d");
  },

  get now(): string {
    return PyDateTime.now().strftime("%Y-%m-%d %H:%M:%S");
  },

  datetime: {
    time: PyTime,
    timedelta: PyTimeDelta,
    datetime: PyDateTime,
    date: PyDate
  },

  relativedelta: PyRelativeDelta,
  true: true,
  false: false
};

export class Domain {
  static TRUE: Domain;
  static FALSE: Domain;

  static combine(domains: readonly DomainRepr[], operator: "AND" | "OR"): Domain {
    const normalized = domains.map((domain) => new Domain(domain).toList());
    if (!normalized.length) return new Domain([]);
    if (normalized.length === 1) return new Domain(normalized[0]);
    const op = operator === "AND" ? "&" : "|";
    const out: unknown[] = [];
    for (const part of normalized) {
      if (!part.length) continue;
      if (out.length) out.unshift(op);
      out.push(...part);
    }
    return new Domain(out as DomainListRepr);
  }

  static and(domains: readonly DomainRepr[]): Domain {
    return Domain.combine(domains, "AND");
  }

  static or(domains: readonly DomainRepr[]): Domain {
    return Domain.combine(domains, "OR");
  }

  static not(domain: DomainRepr): Domain {
    return new Domain(["!", ...new Domain(domain).toList()] as DomainListRepr);
  }

  static removeDomainLeaves(domain: DomainRepr, keysToRemove: readonly string[]): Domain {
    const leaves = new Domain(domain).toList().map((leaf) => {
      if (!Array.isArray(leaf)) return leaf;
      return keysToRemove.includes(String(leaf[0])) ? [1, "=", 1] : leaf;
    });
    return new Domain(leaves as DomainListRepr);
  }

  private readonly description: DomainRepr;

  constructor(description: DomainRepr = []) {
    if (description instanceof Domain) {
      this.description = description.description;
      return;
    }
    if (typeof description !== "string" && !Array.isArray(description)) {
      throw new InvalidDomainError(`Invalid domain representation: ${String(description)}`);
    }
    this.description = description;
    if (Array.isArray(description)) normalizeDomainList(description);
  }

  contains(record: Record<string, unknown>): boolean {
    return matchDomain(record, this.toList(record));
  }

  toString(): string {
    return typeof this.description === "string" ? this.description : formatAST(toPyValue(this.description));
  }

  toList(context: Record<string, unknown> = {}): DomainListRepr {
    if (this.description instanceof Domain) return this.description.toList(context);
    if (typeof this.description === "string") {
      const value = evaluateExpr(this.description, context);
      if (!Array.isArray(value)) throw new InvalidDomainError(`Invalid domain representation: ${this.description}`);
      return normalizeDomainList(value);
    }
    return normalizeDomainList(this.description);
  }

  toJson(): DomainListRepr | string {
    try {
      return this.toList({});
    } catch {
      return this.toString();
    }
  }
}

Domain.TRUE = new Domain([[1, "=", 1]]);
Domain.FALSE = new Domain([[0, "=", 1]]);

export function makeContext(
  contexts: readonly (Record<string, unknown> | string | undefined)[],
  initialEvaluationContext: Record<string, unknown> = {}
): Record<string, unknown> {
  const evaluationContext = { ...initialEvaluationContext };
  const context: Record<string, unknown> = {};
  for (const rawContext of contexts) {
    if (!rawContext) continue;
    const evaluated = typeof rawContext === "string" ? evaluateExpr(rawContext, evaluationContext) : rawContext;
    if (!isRecord(evaluated)) continue;
    Object.assign(context, evaluated);
    Object.assign(evaluationContext, context);
  }
  return context;
}

export function evalPartialContext(
  contextExpression: string,
  evaluationContext: Record<string, unknown> = {}
): Record<string, unknown> {
  const source = contextExpression.trim();
  if (!source || source === "{}") return {};
  const entries = splitTopLevelDictEntries(source);
  if (!entries) return {};
  const context: Record<string, unknown> = {};
  for (const entry of entries) {
    const parts = splitTopLevelKeyValue(entry);
    if (!parts) continue;
    const key = parsePythonLiteral(parts[0], evaluationContext);
    if (!key.ok || (typeof key.value !== "string" && typeof key.value !== "number")) continue;
    const value = parsePythonLiteral(parts[1], evaluationContext);
    if (!value.ok) continue;
    context[String(key.value)] = value.value;
  }
  return context;
}

export function toPyValue(value: unknown): PythonValueAST {
  if (value === null || value === undefined) return { type: 3 };
  if (typeof value === "number") return { type: 0, value };
  if (typeof value === "string" || value instanceof PyDate || value instanceof PyDateTime || value instanceof PyTime || value instanceof PyTimeDelta) {
    return { type: 1, value };
  }
  if (typeof value === "boolean") return { type: 2, value };
  if (Array.isArray(value)) return { type: 4, value: value.map(toPyValue) };
  if (value instanceof Date) return { type: 1, value: PyDateTime.convertDate(value) };
  if (isRecord(value)) {
    const content: Record<string, PythonValueAST> = {};
    for (const [key, item] of Object.entries(value)) content[key] = toPyValue(item);
    return { type: 11, value: content };
  }
  throw new Error("Invalid type");
}

export const PY_DICT = Object.create(null);

export function toPyDict<T extends object>(obj: T): T {
  return new Proxy(obj, {
    getPrototypeOf() {
      return PY_DICT;
    }
  });
}

function formatPyAST(ast: PythonValueAST): string {
  switch (ast.type) {
    case 0:
      return String(ast.value);
    case 1:
      return JSON.stringify(String(ast.value));
    case 2:
      return ast.value ? "True" : "False";
    case 3:
      return "None";
    case 4:
      return `[${ast.value.map(formatPyAST).join(", ")}]`;
    case 10:
      return `(${ast.value.map(formatPyAST).join(", ")})`;
    case 11:
      return `{${Object.entries(ast.value).map(([key, item]) => `${JSON.stringify(key)}: ${formatPyAST(item)}`).join(", ")}}`;
  }
}

function normalizeDomainList(domain: readonly unknown[]): DomainListRepr {
  let expected = 1;
  for (const item of domain) {
    if (item === "&" || item === "|") {
      expected += 1;
    } else if (item === "!") {
      continue;
    } else if (Array.isArray(item) && item.length === 3) {
      expected -= 1;
    } else {
      throw new InvalidDomainError("Invalid domain representation");
    }
  }
  if (domain.length && expected > 0) throw new InvalidDomainError("Invalid domain representation");
  const out = [...domain];
  while (expected < 0) {
    out.unshift("&");
    expected += 1;
  }
  return out as DomainListRepr;
}

function matchDomain(record: Record<string, unknown>, domain: DomainListRepr): boolean {
  if (!domain.length) return true;
  const stack: boolean[] = [];
  for (const item of [...domain].reverse()) {
    if (item === "!") {
      const operand = stack.pop() ?? false;
      stack.push(!operand);
    } else if (item === "&" || item === "|") {
      const left = stack.pop() ?? false;
      const right = stack.pop() ?? false;
      stack.push(item === "&" ? left && right : left || right);
    } else {
      stack.push(matchDomainCondition(record, item));
    }
  }
  return stack.pop() ?? true;
}

function matchDomainCondition(record: Record<string, unknown>, condition: readonly unknown[]): boolean {
  const [field, operator, value] = condition;
  const fieldValue = typeof field === "number" ? field : readDottedValue(record, String(field));
  const op = String(operator);
  const not = op.startsWith("not ");
  switch (op) {
    case "=?":
      if (value === false || value === null) return true;
      return pythonEquals(fieldValue, value);
    case "=":
    case "==":
      return pythonEquals(fieldValue, value);
    case "!=":
    case "<>":
      return !pythonEquals(fieldValue, value);
    case "<":
      return pythonNumber(fieldValue) < pythonNumber(value);
    case "<=":
      return pythonNumber(fieldValue) <= pythonNumber(value);
    case ">":
      return pythonNumber(fieldValue) > pythonNumber(value);
    case ">=":
      return pythonNumber(fieldValue) >= pythonNumber(value);
    case "in":
    case "not in": {
      const values = Array.isArray(value) ? value : [value];
      const fieldValues = Array.isArray(fieldValue) ? fieldValue : [fieldValue];
      return fieldValues.some((item) => values.some((candidate) => pythonEquals(item, candidate))) !== not;
    }
    case "like":
    case "not like":
    case "ilike":
    case "not ilike":
    case "=like":
    case "not =like":
    case "=ilike":
    case "not =ilike":
      return matchLike(fieldValue, String(value ?? ""), op) !== not;
    case "any":
    case "not any":
    case "child_of":
    case "parent_of":
      return true;
    default:
      throw new InvalidDomainError(`Unsupported domain operator: ${op}`);
  }
}

function readDottedValue(record: unknown, path: string): unknown {
  let current = record;
  for (const part of path.split(".")) {
    if (!isRecord(current)) return undefined;
    current = current[part];
  }
  return current;
}

function matchLike(fieldValue: unknown, pattern: string, operator: string): boolean {
  if (fieldValue === false || fieldValue === null || fieldValue === undefined) return false;
  const escaped = pattern.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replace(/%/g, ".*");
  const flags = operator.includes("ilike") ? "i" : "";
  return new RegExp(operator.includes("=like") || operator.includes("=ilike") ? `^${escaped}$` : escaped, flags).test(String(fieldValue));
}

function splitTopLevelDictEntries(source: string): string[] | undefined {
  const trimmed = source.trim();
  if (!trimmed.startsWith("{") || !trimmed.endsWith("}")) return undefined;
  const body = trimmed.slice(1, -1).trim();
  if (!body) return [];
  return splitTopLevel(body, ",");
}

function splitTopLevelKeyValue(source: string): [string, string] | undefined {
  const parts = splitTopLevel(source, ":");
  if (parts.length < 2) return undefined;
  return [parts[0], parts.slice(1).join(":")];
}

function splitTopLevel(source: string, separator: string): string[] {
  const out: string[] = [];
  let start = 0;
  let depth = 0;
  let quote = "";
  let escaped = false;
  for (let index = 0; index < source.length; index += 1) {
    const char = source[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (quote) {
      if (char === "\\") escaped = true;
      else if (char === quote) quote = "";
      continue;
    }
    if (char === "'" || char === "\"") {
      quote = char;
    } else if (char === "[" || char === "(" || char === "{") {
      depth += 1;
    } else if (char === "]" || char === ")" || char === "}") {
      depth -= 1;
    } else if (char === separator && depth === 0) {
      const part = source.slice(start, index).trim();
      if (part) out.push(part);
      start = index + 1;
    }
  }
  const part = source.slice(start).trim();
  if (part) out.push(part);
  return out;
}

function fieldNodesToSpecification(
  nodes: readonly ViewFieldNode[],
  fields: Record<string, unknown>,
  viewDescriptions: ViewDescriptions,
  evalContext: Record<string, unknown>
): ReadSpecification {
  const specification: ReadSpecification = {};
  for (const node of nodes) {
    const description = fields[node.name];
    const spec: Record<string, unknown> = {};
    const fieldType = fieldTypeValue(description);
    const invisible = fieldInvisible(node, evalContext);
    if (fieldType === "many2one" || fieldType === "reference") {
      spec.fields = invisible ? {} : {
        ...nestedRelationSpecification(node, description, viewDescriptions, evalContext),
        display_name: {}
      };
      const context = parseContextAttribute(node.attrs.context, evalContext);
      if (context) spec.context = context;
    } else if (fieldType === "one2many" || fieldType === "many2many") {
      if (node.children.length && !invisible) {
        spec.fields = nestedRelationSpecification(node, description, viewDescriptions, evalContext);
        const context = parseContextAttribute(node.attrs.context, evalContext);
        if (context) spec.context = context;
        const limit = numericAttribute(node.attrs.limit);
        if (limit !== undefined) spec.limit = limit;
        const order = node.attrs.default_order ?? node.childViewAttrs.default_order ?? node.attrs.order;
        if (order) spec.order = order;
      }
    }
    specification[node.name] = spec;
    if (!invisible && node.name === "group_ids" && node.attrs.widget === "res_user_group_ids") {
      for (const dependency of ["all_group_ids", "view_group_hierarchy", "role"]) {
        if (fields[dependency] !== undefined && specification[dependency] === undefined) {
          specification[dependency] = {};
        }
      }
    }
    if (!invisible && node.attrs.widget === "statusbar_state_duration" && fields.duration_state_tracking !== undefined && specification.duration_state_tracking === undefined) {
      specification.duration_state_tracking = {};
    }
  }
  return specification;
}

function nestedRelationSpecification(
  node: ViewFieldNode,
  description: unknown,
  viewDescriptions: ViewDescriptions,
  evalContext: Record<string, unknown>
): ReadSpecification {
  const relation = fieldRelationValue(description);
  const relatedFields = relation ? relationFields(viewDescriptions, relation) : {};
  if (!node.children.length || !Object.keys(relatedFields).length) return {};
  return fieldNodesToSpecification(node.children, relatedFields, viewDescriptions, evalContext);
}

function relationFields(viewDescriptions: ViewDescriptions, relation: string): Record<string, unknown> {
  const modelInfo = viewDescriptions.relatedModels[relation];
  if (isRecord(modelInfo) && isRecord(modelInfo.fields)) return modelInfo.fields as Record<string, unknown>;
  return {};
}

function fieldTypeValue(description: unknown): string {
  return isRecord(description) && typeof description.type === "string" ? description.type : "";
}

function fieldRelationValue(description: unknown): string {
  return isRecord(description) && typeof description.relation === "string" ? description.relation : "";
}

function fieldInvisible(node: ViewFieldNode, evalContext: Record<string, unknown>): boolean {
  return nodeInvisible(node.attrs, evalContext);
}

function nodeInvisible(attrs: Record<string, string>, evalContext: Record<string, unknown>): boolean {
  const value = attrs.invisible ?? attrs.column_invisible;
  if (!value) return false;
  if (value === "1" || value === "True" || value === "true") return true;
  if (value === "0" || value === "False" || value === "false") return false;
  const parsed = parsePythonLiteral(value, evalContext);
  return parsed.ok ? pythonTruthy(parsed.value) : false;
}

function numericAttribute(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const number = Number.parseInt(value, 10);
  return Number.isFinite(number) ? number : undefined;
}

function parseContextAttribute(value: string | undefined, evalContext: Record<string, unknown>): Record<string, unknown> | undefined {
  if (!value || value === "{}") return undefined;
  const parsed = evalPartialContext(value, evalContext);
  return parsed && Object.keys(parsed).length ? parsed : undefined;
}

function parseObjectLiteral(value: string, evalContext: Record<string, unknown>): Record<string, unknown> | undefined {
  const parsed = parsePythonLiteral(value, evalContext);
  return parsed.ok && isRecord(parsed.value) ? parsed.value : undefined;
}

function parseButtonArgs(value: string | undefined, evalContext: Record<string, unknown>): unknown[] {
  if (!value) return [];
  const parsed = parsePythonLiteral(value, evalContext);
  if (!parsed.ok) return [];
  return Array.isArray(parsed.value) ? parsed.value : [parsed.value];
}

function safeEvaluateBooleanAttr(value: string | undefined, evalContext: Record<string, unknown>): boolean | undefined {
  if (value === undefined || value === "") return undefined;
  try {
    return evaluateBooleanExpr(value, evalContext);
  } catch {
    return undefined;
  }
}

function safeEvaluateAttr(value: string, evalContext: Record<string, unknown>): unknown {
  try {
    return evaluateExpr(value, evalContext);
  } catch {
    return value;
  }
}

type PythonLiteralResult = { ok: true; value: unknown } | { ok: false };

function parsePythonLiteral(expression: string, evalContext: Record<string, unknown> = {}): PythonLiteralResult {
  try {
    const parser = new PythonLiteralParser(expression, evalContext);
    const value = parser.parse();
    return { ok: true, value };
  } catch {
    return { ok: false };
  }
}

type PythonCallable = { __pythonCallable: string };
type PythonKeywordArg = { __pythonKeyword: string; value: unknown };
type PythonDateValue = { __pythonDate: Date };
type PythonModule = { __pythonModule: "time" | "datetime" };

function pythonCallable(name: string): PythonCallable {
  return { __pythonCallable: name };
}

function isPythonCallable(value: unknown): value is PythonCallable {
  return isRecord(value) && typeof value.__pythonCallable === "string";
}

function pythonKeywordArg(name: string, value: unknown): PythonKeywordArg {
  return { __pythonKeyword: name, value };
}

function isPythonKeywordArg(value: unknown): value is PythonKeywordArg {
  return isRecord(value) && typeof value.__pythonKeyword === "string";
}

function pythonDateValue(date: Date): PythonDateValue {
  return { __pythonDate: date };
}

function isPythonDateValue(value: unknown): value is PythonDateValue {
  return isRecord(value) && value.__pythonDate instanceof Date;
}

function pythonModule(name: "time" | "datetime"): PythonModule {
  return { __pythonModule: name };
}

function isPythonModule(value: unknown, name?: "time" | "datetime"): value is PythonModule {
  return isRecord(value) && (name ? value.__pythonModule === name : typeof value.__pythonModule === "string");
}

function pythonTruthy(value: unknown): boolean {
  if (value === null || value === undefined || value === false) return false;
  if (typeof value === "number") return value !== 0 && Number.isFinite(value);
  if (typeof value === "string" || Array.isArray(value)) return value.length > 0;
  if (value instanceof Set) return value.size > 0;
  if (isRecord(value) && typeof value.isTrue === "function") return Boolean(value.isTrue());
  if (isRecord(value)) return Object.keys(value).length > 0;
  return true;
}

function pythonEquals(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (Array.isArray(left) && Array.isArray(right)) {
    return left.length === right.length && left.every((value, index) => pythonEquals(value, right[index]));
  }
  if (isRecord(left) && isRecord(right)) {
    const leftKeys = Object.keys(left);
    const rightKeys = Object.keys(right);
    return leftKeys.length === rightKeys.length && leftKeys.every((key) => key in right && pythonEquals(left[key], right[key]));
  }
  return false;
}

function pythonContains(container: unknown, item: unknown): boolean {
  if (typeof container === "string") return container.includes(String(item));
  if (Array.isArray(container)) return container.some((value) => pythonEquals(value, item));
  if (isRecord(container)) return String(item) in container;
  return false;
}

function pythonCompare(left: unknown, operator: string, right: unknown): boolean {
  switch (operator) {
    case "==":
      return pythonEquals(left, right);
    case "!=":
      return !pythonEquals(left, right);
    case "is":
      return Object.is(left, right);
    case "is not":
      return !Object.is(left, right);
    case "in":
      return pythonContains(right, left);
    case "not in":
      return !pythonContains(right, left);
    case "<":
      return Number(left) < Number(right);
    case "<=":
      return Number(left) <= Number(right);
    case ">":
      return Number(left) > Number(right);
    case ">=":
      return Number(left) >= Number(right);
    default:
      throw new Error(`unsupported comparison ${operator}`);
  }
}

function pythonNumber(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "boolean") return value ? 1 : 0;
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) throw new Error("number expected");
  return parsed;
}

function pythonAdd(left: unknown, right: unknown): unknown {
  if (isPythonDateValue(left) && isRecord(right) && right.__pythonDeltaDays !== undefined) {
    return pythonDateValue(new Date(left.__pythonDate.getTime() + pythonNumber(right.__pythonDeltaDays) * 86400000));
  }
  if ((left instanceof PyDate || left instanceof PyDateTime) && (right instanceof PyTimeDelta || isRecord(right))) return left.plus(right);
  if (typeof left === "string" || typeof right === "string") return `${left ?? ""}${right ?? ""}`;
  if (Array.isArray(left) && Array.isArray(right)) return [...left, ...right];
  return pythonNumber(left) + pythonNumber(right);
}

function pythonStrftime(value: Date, format: string): string {
  const pad = (number: number, size = 2) => String(number).padStart(size, "0");
  const replacements: Record<string, string> = {
    "%Y": String(value.getUTCFullYear()),
    "%y": pad(value.getUTCFullYear() % 100),
    "%m": pad(value.getUTCMonth() + 1),
    "%d": pad(value.getUTCDate()),
    "%H": pad(value.getUTCHours()),
    "%M": pad(value.getUTCMinutes()),
    "%S": pad(value.getUTCSeconds())
  };
  return Object.entries(replacements).reduce((out, [token, replacement]) => out.replaceAll(token, replacement), format);
}

class PythonLiteralParser {
  private index = 0;
  private readonly source: string;
  private readonly context: Record<string, unknown>;

  constructor(source: string, context: Record<string, unknown>) {
    this.source = source.trim();
    this.context = context;
  }

  parse(): unknown {
    const value = this.parseExpression();
    this.skipWhitespace();
    if (!this.done()) throw new Error("unexpected trailing input");
    return value;
  }

  private parseExpression(): unknown {
    return this.parseConditional();
  }

  private parseConditional(): unknown {
    const whenTrue = this.parseOr();
    if (!this.consumeKeyword("if")) return whenTrue;
    const condition = this.parseOr();
    this.expectKeyword("else");
    const whenFalse = this.parseConditional();
    return pythonTruthy(condition) ? whenTrue : whenFalse;
  }

  private parseOr(): unknown {
    let value = this.parseAnd();
    while (this.consumeKeyword("or")) {
      const left = value;
      const right = this.parseAnd();
      value = pythonTruthy(left) ? left : right;
    }
    return value;
  }

  private parseAnd(): unknown {
    let value = this.parseNot();
    while (this.consumeKeyword("and")) {
      const left = value;
      const right = this.parseNot();
      value = pythonTruthy(left) ? right : left;
    }
    return value;
  }

  private parseNot(): unknown {
    const start = this.index;
    if (this.consumeKeyword("not")) {
      this.skipWhitespace();
      if (this.keywordAhead("in")) {
        this.index = start;
        return this.parseComparison();
      }
      return !pythonTruthy(this.parseNot());
    }
    return this.parseComparison();
  }

  private parseComparison(): unknown {
    let left = this.parseAdditive();
    let result = true;
    let compared = false;
    while (true) {
      const operator = this.parseComparisonOperator();
      if (!operator) return compared ? result : left;
      const right = this.parseAdditive();
      compared = true;
      if (!pythonCompare(left, operator, right)) result = false;
      left = right;
    }
  }

  private parseComparisonOperator(): string | undefined {
    this.skipWhitespace();
    for (const operator of ["==", "!=", "<>", "<=", ">=", "<", ">"]) {
      if (this.source.startsWith(operator, this.index)) {
        this.index += operator.length;
        return operator === "<>" ? "!=" : operator;
      }
    }
    if (this.consumeKeyword("not")) {
      this.expectKeyword("in");
      return "not in";
    }
    if (this.consumeKeyword("is")) {
      if (this.consumeKeyword("not")) return "is not";
      return "is";
    }
    if (this.consumeKeyword("in")) return "in";
    return undefined;
  }

  private parseAdditive(): unknown {
    let value = this.parseMultiplicative();
    while (true) {
      this.skipWhitespace();
      if (this.consume("+")) {
        value = pythonAdd(value, this.parseMultiplicative());
      } else if (this.consume("-")) {
        value = pythonNumber(value) - pythonNumber(this.parseMultiplicative());
      } else {
        return value;
      }
    }
  }

  private parseMultiplicative(): unknown {
    let value = this.parsePower();
    while (true) {
      this.skipWhitespace();
      if (this.consume("//")) {
        value = Math.trunc(pythonNumber(value) / pythonNumber(this.parsePower()));
      } else if (this.consume("*")) {
        value = pythonNumber(value) * pythonNumber(this.parsePower());
      } else if (this.consume("/")) {
        value = pythonNumber(value) / pythonNumber(this.parsePower());
      } else if (this.consume("%")) {
        value = pythonNumber(value) % pythonNumber(this.parsePower());
      } else {
        return value;
      }
    }
  }

  private parsePower(): unknown {
    const value = this.parseUnary();
    this.skipWhitespace();
    if (this.consume("**")) return pythonNumber(value) ** pythonNumber(this.parsePower());
    return value;
  }

  private parseUnary(): unknown {
    this.skipWhitespace();
    if (this.consume("+")) return pythonNumber(this.parseUnary());
    if (this.consume("-")) return -pythonNumber(this.parseUnary());
    return this.parsePostfix();
  }

  private parsePostfix(): unknown {
    let value = this.parsePrimary();
    while (true) {
      this.skipWhitespace();
      if (this.consume(".")) {
        const property = this.parseIdentifier();
        this.skipWhitespace();
        if (this.consume("(")) {
          value = this.callMethod(value, property, this.parseArgumentsAfterOpen());
        } else {
          value = this.readAttribute(value, property);
        }
      } else if (this.consume("[")) {
        const key = this.parseExpression();
        this.expect("]");
        value = this.readIndex(value, key);
      } else if (this.consume("(")) {
        value = this.callValue(value, this.parseArgumentsAfterOpen());
      } else {
        return value;
      }
    }
  }

  private parsePrimary(): unknown {
    this.skipWhitespace();
    const char = this.peek();
    if (char === "\"" || char === "'") return this.parseString();
    if (char === "[") return this.parseArray("]", false);
    if (char === "(") return this.parseParen();
    if (char === "{") return this.parseDict();
    if (char === "-" || this.isDigit(char)) return this.parseNumber();
    if (this.isIdentifierStart(char)) return this.parseIdentifierValue();
    throw new Error("unexpected token");
  }

  private parseArray(close: string, tuple: boolean): unknown[] {
    this.index += 1;
    const out: unknown[] = [];
    this.skipWhitespace();
    if (this.consume(close)) return out;
    while (!this.done()) {
      out.push(this.parseExpression());
      this.skipWhitespace();
      if (this.consume(close)) return out;
      this.expect(",");
      this.skipWhitespace();
      if (tuple && this.consume(close)) return out;
    }
    throw new Error("unterminated array");
  }

  private parseParen(): unknown {
    this.index += 1;
    const out: unknown[] = [];
    this.skipWhitespace();
    if (this.consume(")")) return out;
    const first = this.parseExpression();
    this.skipWhitespace();
    if (this.consume(")")) return first;
    out.push(first);
    this.expect(",");
    this.skipWhitespace();
    if (this.consume(")")) return out;
    while (!this.done()) {
      out.push(this.parseExpression());
      this.skipWhitespace();
      if (this.consume(")")) return out;
      this.expect(",");
      this.skipWhitespace();
      if (this.consume(")")) return out;
    }
    throw new Error("unterminated tuple");
  }

  private parseDict(): Record<string, unknown> {
    this.index += 1;
    const out: Record<string, unknown> = {};
    this.skipWhitespace();
    if (this.consume("}")) return out;
    while (!this.done()) {
      const key = this.parseExpression();
      if (typeof key !== "string" && typeof key !== "number") throw new Error("invalid dict key");
      this.skipWhitespace();
      this.expect(":");
      out[String(key)] = this.parseExpression();
      this.skipWhitespace();
      if (this.consume("}")) return out;
      this.expect(",");
      this.skipWhitespace();
      if (this.consume("}")) return out;
    }
    throw new Error("unterminated dict");
  }

  private parseString(): string {
    const quote = this.peek();
    this.index += 1;
    let out = "";
    while (!this.done()) {
      const char = this.source[this.index++];
      if (char === quote) return out;
      if (char === "\\") {
        if (this.done()) throw new Error("unterminated escape");
        const escaped = this.source[this.index++];
        out += escaped === "n" ? "\n" : escaped === "t" ? "\t" : escaped === "r" ? "\r" : escaped;
      } else {
        out += char;
      }
    }
    throw new Error("unterminated string");
  }

  private parseNumber(): number {
    const start = this.index;
    if (this.peek() === "-") this.index += 1;
    while (this.isDigit(this.peek())) this.index += 1;
    if (this.peek() === ".") {
      this.index += 1;
      while (this.isDigit(this.peek())) this.index += 1;
    }
    const raw = this.source.slice(start, this.index);
    if (raw === "-" || raw === "-.") throw new Error("invalid number");
    const number = Number(raw);
    if (!Number.isFinite(number)) throw new Error("invalid number");
    return number;
  }

  private parseIdentifierValue(): unknown {
    const name = this.parseIdentifier();
    switch (name) {
      case "True":
      case "true":
        return true;
      case "False":
      case "false":
        return false;
      case "None":
      case "none":
      case "null":
        return null;
      case "len":
      case "str":
      case "int":
      case "float":
      case "bool":
      case "set":
      case "sum":
      case "min":
      case "max":
      case "round":
      case "abs":
      case "today":
      case "now":
      case "context_today":
      case "relativedelta":
        return pythonCallable(name);
      case "time":
        return pythonModule("time");
      case "datetime":
        return pythonModule("datetime");
      default:
        return this.resolveIdentifier(name);
    }
  }

  private resolveIdentifier(name: string): unknown {
    if (name in this.context) return this.context[name];
    if (name === "context") return this.contextObject();
    throw new Error(`unknown identifier ${name}`);
  }

  private contextObject(): Record<string, unknown> {
    return isRecord(this.context.context) ? this.context.context : this.context;
  }

  private parseArgumentsAfterOpen(): unknown[] {
    const args: unknown[] = [];
    this.skipWhitespace();
    if (this.consume(")")) return args;
    while (!this.done()) {
      args.push(this.parseArgument());
      this.skipWhitespace();
      if (this.consume(")")) return args;
      this.expect(",");
      this.skipWhitespace();
      if (this.consume(")")) return args;
    }
    throw new Error("unterminated call");
  }

  private parseArgument(): unknown {
    this.skipWhitespace();
    const start = this.index;
    if (this.isIdentifierStart(this.peek())) {
      const name = this.parseIdentifier();
      this.skipWhitespace();
      if (this.source.startsWith("=", this.index) && !this.source.startsWith("==", this.index)) {
        this.index += 1;
        return pythonKeywordArg(name, this.parseExpression());
      }
      this.index = start;
    }
    return this.parseExpression();
  }

  private readAttribute(value: unknown, property: string): unknown {
    if (isRecord(value) && property in value) return value[property];
    if (property === "id" && Array.isArray(value) && typeof value[0] === "number") return value[0];
    if (property === "display_name" && Array.isArray(value) && typeof value[1] === "string") return value[1];
    throw new Error(`unknown attribute ${property}`);
  }

  private readIndex(value: unknown, key: unknown): unknown {
    if ((Array.isArray(value) || typeof value === "string") && typeof key === "number") {
      const index = key < 0 ? value.length + key : key;
      return value[index];
    }
    if (isRecord(value) && (typeof key === "string" || typeof key === "number")) {
      const property = String(key);
      if (property in value) return value[property];
    }
    throw new Error("invalid index");
  }

  private callMethod(receiver: unknown, method: string, args: unknown[]): unknown {
    if (typeof receiver === "string") {
      if (method === "lower") return receiver.toLowerCase();
      if (method === "upper") return receiver.toUpperCase();
      if (method === "startswith") return receiver.startsWith(String(args[0] ?? ""));
      if (method === "endswith") return receiver.endsWith(String(args[0] ?? ""));
    }
    if (isPythonDateValue(receiver) && method === "strftime") {
      return pythonStrftime(receiver.__pythonDate, String(args[0] ?? "%Y-%m-%d"));
    }
    if ((receiver instanceof PyDate || receiver instanceof PyDateTime || receiver instanceof PyTime) && method === "strftime") {
      return receiver.strftime(String(args[0] ?? "%Y-%m-%d"));
    }
    if (isPythonModule(receiver, "time") && method === "strftime") {
      return pythonStrftime(new Date(), String(args[0] ?? "%Y-%m-%d"));
    }
    if (isPythonModule(receiver, "datetime")) {
      if (method === "date") return PyDate.create(...args);
      if (method === "datetime") return PyDateTime.create(...args);
      if (method === "time") return PyTime.create(...args);
      if (method === "timedelta") return PyTimeDelta.create(...args);
    }
    if (typeof receiver === "function" && method === "create") {
      const maybeFactory = receiver as unknown as { create?: (...factoryArgs: unknown[]) => unknown };
      if (typeof maybeFactory.create === "function") return maybeFactory.create(...args);
      return receiver(...args);
    }
    if (receiver instanceof PyTimeDelta && method === "totalSeconds") return receiver.totalSeconds();
    if (method === "get" && isRecord(receiver)) {
      const key = args[0];
      if (typeof key !== "string" && typeof key !== "number") throw new Error("invalid get key");
      return String(key) in receiver ? receiver[String(key)] : args[1];
    }
    if (method === "has_group" || method === "hasGroup") {
      if (isRecord(receiver) && typeof receiver.has_group === "function") return Boolean(receiver.has_group(args[0]));
      if (isRecord(receiver) && typeof receiver.hasGroup === "function") return Boolean(receiver.hasGroup(args[0]));
      const groups = isRecord(receiver) && Array.isArray(receiver.groups) ? receiver.groups : [];
      return groups.some((group) => group === args[0]);
    }
    throw new Error(`unsupported method ${method}`);
  }

  private callValue(value: unknown, args: unknown[]): unknown {
    if (typeof value === "function") {
      const maybeFactory = value as unknown as { create?: (...factoryArgs: unknown[]) => unknown };
      if (typeof maybeFactory.create === "function") return maybeFactory.create(...args);
      return value(...args);
    }
    if (!isPythonCallable(value)) throw new Error("unsupported call");
    switch (value.__pythonCallable) {
      case "len":
        return this.builtinLen(args);
      case "str":
        return String(args[0] ?? "");
      case "int":
        return Math.trunc(pythonNumber(args[0] ?? 0));
      case "float":
        return pythonNumber(args[0] ?? 0);
      case "bool":
        return pythonTruthy(args[0]);
      case "set":
        return this.builtinSet(args);
      case "sum":
        return this.builtinSum(args);
      case "min":
        return Math.min(...this.numericArgs(args));
      case "max":
        return Math.max(...this.numericArgs(args));
      case "round":
        return Math.round(pythonNumber(args[0] ?? 0));
      case "abs":
        return Math.abs(pythonNumber(args[0] ?? 0));
      case "today":
      case "context_today":
        return PyDate.today();
      case "now":
        return PyDateTime.now();
      case "relativedelta":
        return PyRelativeDelta.create(...args);
      default:
        throw new Error("unsupported builtin");
    }
  }

  private builtinLen(args: unknown[]): number {
    const value = args[0];
    if (typeof value === "string" || Array.isArray(value)) return value.length;
    if (isRecord(value)) return Object.keys(value).length;
    return 0;
  }

  private builtinSum(args: unknown[]): number {
    const value = args[0];
    if (!Array.isArray(value)) return 0;
    return value.reduce((total, item) => total + pythonNumber(item), 0);
  }

  private builtinSet(args: unknown[]): unknown[] {
    const value = args[0];
    if (!Array.isArray(value)) return [];
    const out: unknown[] = [];
    for (const item of value) {
      if (!out.some((existing) => pythonEquals(existing, item))) out.push(item);
    }
    return out;
  }

  private relativeDelta(args: unknown[]): Record<string, unknown> {
    const values: Record<string, unknown> = {};
    const positional = args.filter((arg) => !isPythonKeywordArg(arg));
    for (const arg of args) {
      if (isPythonKeywordArg(arg)) values[arg.__pythonKeyword] = arg.value;
    }
    const days = pythonNumber(values.days ?? 0) + pythonNumber(values.weeks ?? 0) * 7 + pythonNumber(positional[0] ?? 0);
    return { __pythonDeltaDays: days };
  }

  private numericArgs(args: unknown[]): number[] {
    const values = args.length === 1 && Array.isArray(args[0]) ? args[0] : args;
    return values.map((value) => pythonNumber(value));
  }

  private parseIdentifier(): string {
    const start = this.index;
    this.index += 1;
    while (this.isIdentifierPart(this.peek())) this.index += 1;
    return this.source.slice(start, this.index);
  }

  private skipWhitespace(): void {
    while (/\s/.test(this.peek())) this.index += 1;
  }

  private expect(char: string): void {
    if (!this.consume(char)) throw new Error(`expected ${char}`);
  }

  private consume(char: string): boolean {
    if (!this.source.startsWith(char, this.index)) return false;
    this.index += char.length;
    return true;
  }

  private consumeKeyword(keyword: string): boolean {
    this.skipWhitespace();
    if (!this.keywordAhead(keyword)) return false;
    this.index += keyword.length;
    return true;
  }

  private expectKeyword(keyword: string): void {
    if (!this.consumeKeyword(keyword)) throw new Error(`expected ${keyword}`);
  }

  private keywordAhead(keyword: string): boolean {
    if (!this.source.startsWith(keyword, this.index)) return false;
    const before = this.source[this.index - 1] ?? "";
    const after = this.source[this.index + keyword.length] ?? "";
    return !this.isIdentifierPart(before) && !this.isIdentifierPart(after);
  }

  private peek(): string {
    return this.source[this.index] ?? "";
  }

  private done(): boolean {
    return this.index >= this.source.length;
  }

  private isDigit(char: string): boolean {
    return char >= "0" && char <= "9";
  }

  private isIdentifierStart(char: string): boolean {
    return /[A-Za-z_]/.test(char);
  }

  private isIdentifierPart(char: string): boolean {
    return /[A-Za-z0-9_]/.test(char);
  }
}

interface NamedServiceDefinition extends ServiceDefinition {
  name: string;
}

async function startServiceBatch(
  env: WebClientEnv,
  source: Registry<ServiceDefinition>,
  toStart: Map<string, NamedServiceDefinition>
): Promise<void> {
  if (startServicesPromise) {
    await startServicesPromise;
    return startServiceBatch(env, source, toStart);
  }

  for (const [name, service] of source.getEntries()) {
    if (!(name in env.services)) {
      toStart.set(name, namedService(name, service));
    }
  }

  async function start(): Promise<void> {
    const promises: Promise<void>[] = [];
    let service: NamedServiceDefinition | null;
    while ((service = findNextService(toStart, env.services))) {
      const name = service.name;
      toStart.delete(name);
      if (name in env.services) continue;
      const dependencies = Object.fromEntries(
        (service.dependencies ?? []).map((dependency) => [dependency, env.services[dependency]])
      );
      if ("async" in service && service.async !== undefined) {
        serviceMetadata[name] = service.async;
      }
      promises.push(Promise.resolve(service.start(env, dependencies)).then((value) => {
        env.services[name] = value ?? null;
      }));
    }
    await Promise.all(promises);
    if (promises.length) await start();
  }

  startServicesPromise = start().finally(() => {
    startServicesPromise = null;
  });
  await startServicesPromise;
  env.bus.trigger("SERVICES-LOADED");
  if (toStart.size) {
    const missingDeps = new Set<string>();
    for (const service of toStart.values()) {
      for (const dependency of service.dependencies ?? []) {
        if (!(dependency in env.services) && !toStart.has(dependency)) {
          missingDeps.add(dependency);
        }
      }
    }
    throw new Error(
      `Some services could not be started: ${[...toStart.keys()]}. Missing dependencies: ${[...missingDeps].join(", ")}`
    );
  }
}

function namedService(name: string, service: ServiceDefinition): NamedServiceDefinition {
  return Object.assign(Object.create(service), { ...service, name }) as NamedServiceDefinition;
}

function findNextService(
  toStart: Map<string, NamedServiceDefinition>,
  services: Record<string, unknown>
): NamedServiceDefinition | null {
  for (const service of toStart.values()) {
    if ((service.dependencies ?? []).every((dependency) => dependency in services)) {
      return service;
    }
  }
  return null;
}

function isRPCTransport(value: unknown): value is RPCTransport {
  return typeof value === "function";
}

function isActionExecutor(value: unknown): value is ActionExecutor {
  return typeof value === "function";
}

function isExecutableClientAction(value: unknown): value is ExecutableClientAction {
  return isRecord(value) && typeof value.execute === "function";
}

function validateModel(value: string): void {
  if (typeof value !== "string" || value.length === 0) {
    throw new Error(`Invalid model name: ${value}`);
  }
}

function validateArray(name: string, value: readonly unknown[]): void {
  if (!Array.isArray(value)) throw new Error(`${name} should be an array`);
}

function validatePrimitiveList(name: string, type: "number" | "string", value: readonly unknown[]): void {
  if (!Array.isArray(value) || value.some((item) => typeof item !== type)) {
    throw new Error(`Invalid ${name} list: ${value}`);
  }
}

function validatePlainObject(name: string, value: unknown): asserts value is Record<string, unknown> {
  if (!isRecord(value)) throw new Error(`${name} should be an object`);
}

function validateRecordList(records: readonly Record<string, unknown>[]): void {
  validateArray("records", records);
  for (const record of records) validatePlainObject("record", record);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function validateRegistryValue(name: string | undefined, key: string, value: unknown, schema: unknown): void {
  try {
    validate(value, schema);
  } catch (error) {
    throw new Error(`Validation error for key "${key}" in registry "${name}": ${error}`);
  }
}
