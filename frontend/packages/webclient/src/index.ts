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
  groupByDescriptor as actionGroupByDescriptor,
  SEARCH_DATE_INTERVALS,
  type SearchDateInterval,
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
  type ControlPanelPager as ActionControlPanelPager,
  type ControlPanelSearchSuggestion as ActionControlPanelSearchSuggestion,
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
const DEFAULT_PAGER_COUNT_LIMIT = 10000;
const DEFAULT_VIEW_FIELD_LIMIT = 6;
const DEFAULT_KANBAN_GROUP_LIMIT = 10;

const DEFAULT_MODEL_LIST_FIELDS: Record<string, readonly string[]> = {
  "ir.actions.server": ["name", "model_id", "state", "usage"],
  "ir.cron": ["priority", "name", "model_id", "nextcall", "interval_number", "interval_type", "active"],
  "res.groups": ["privilege_id", "name"],
  "res.users": ["name", "login", "role"]
};

const DEFAULT_MODEL_FORM_FIELDS: Record<string, readonly string[]> = {
  "ir.actions.server": ["name", "model_id", "group_ids", "state", "active", "code"],
  "ir.cron": ["name", "model_id", "group_ids", "user_id", "interval_number", "interval_type", "active", "nextcall", "priority", "code"],
  "res.groups": ["name", "privilege_id", "share", "api_key_duration", "user_ids", "implied_ids", "menu_access", "view_access", "model_access", "rule_groups", "comment"],
  "res.users": ["name", "login", "email", "company_id", "role", "group_ids", "active", "notification_type", "signature"]
};

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
  offset: number;
  countLimited: boolean;
}

export interface WindowActionSearchState {
  parsed: ParsedSearchArch;
  state: SearchModelState;
  suggestions: readonly ActionControlPanelSearchSuggestion[];
  filters: readonly ActionControlPanelMenuItem[];
  groupBys: readonly ActionControlPanelMenuItem[];
  favorites: readonly ActionControlPanelMenuItem[];
}

interface KanbanLoadMorePager {
  offset: number;
  length: number;
  countLimited: boolean;
  search?: WindowActionSearchState;
}

interface ListFormPagerContext {
  offset: number;
  length: number;
  countLimited: boolean;
}

interface KanbanProgressBarNode {
  field: string;
  sumField?: string;
  colors: Record<string, string>;
}

interface KanbanDragContext {
  groupField: string;
  groupKey: string;
  groupRaw: unknown;
}

type KanbanTemplateNode =
  | { type: "text"; text: string }
  | { type: "field"; name: string; attrs: Record<string, string> }
  | { type: "element"; tag: string; attrs: Record<string, string>; children: KanbanTemplateNode[] };

interface KanbanTemplateSet {
  main: KanbanTemplateNode[];
  named: Record<string, KanbanTemplateNode[]>;
}

interface KanbanTemplateInheritancePatch {
  inherit: string;
  operations: KanbanTemplateInheritanceOperation[];
}

interface KanbanTemplateInheritanceOperation {
  expr: string;
  position: string;
  children: KanbanTemplateNode[];
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

interface DialogWindowActionContent {
  body: HTMLElement;
  footer?: HTMLElement;
}

interface SettingsActionState {
  initialValues: Record<string, unknown>;
  currentValues: Record<string, unknown>;
  dirtyFields: Set<string>;
  search: string;
  saveButton?: HTMLButtonElement;
  discardButton?: HTMLButtonElement;
  status?: HTMLElement;
  renderBody?: () => void;
}

interface FormActionState {
  initialValues: Record<string, unknown>;
  currentValues: Record<string, unknown>;
  dirtyFields: Set<string>;
  editing: boolean;
  fields: Record<string, unknown>;
  editButton?: HTMLButtonElement;
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
    const search = buildWindowActionSearch(action, context, viewDescriptions, resModel);
    const data = orm
      ? await loadWindowActionRecords(orm, action, activeView, resModel, context, viewDescriptions, search.state)
      : { records: [], length: 0, countLimited: false };
    return {
      type: "ir.actions.act_window",
      action: { ...action, context },
      activeView,
      resModel,
      viewDescriptions,
      search,
      records: data.records,
      length: data.length,
      offset: activeView === "form" ? 0 : actionPagerOffset(action),
      countLimited: data.countLimited
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
        databaseName: sessionDatabaseName(options.session),
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

function sessionDatabaseName(sessionInfo: Record<string, unknown> | undefined): string | undefined {
  return firstText(
    sessionInfo?.db_name,
    sessionInfo?.db,
    sessionInfo?.database_name,
    sessionInfo?.dbname
  );
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
  const formState = settingsState ? null : formActionState(result, viewDescription, values, fields);
  const controlPanel = renderWindowActionControlPanel(result, root, options, settingsState);
  if (settingsState) appendSettingsActionButtons(controlPanel, root, result, settingsState, options);
  if (formState) {
    appendFormActionButtons(controlPanel, root, result, formState, options);
    appendTechnicalFormSmartButtons(controlPanel, root, result, formState.currentValues);
  }
  let body: HTMLElement;
  if (settingsState) {
    body = renderSettingsActionView(result, viewDescription, fields, settingsState, root);
  } else if (formState) {
    const renderBody = () => renderFormView(
      viewDescription,
      fields,
      result.viewDescriptions.relatedModels,
      formState.currentValues,
      result.resModel,
      formRenderOptions(options, formState),
      formState.editing
    );
    body = renderBody();
    moveFormActionMenuToControlPanel(controlPanel, body);
    formState.renderBody = () => {
      body = renderBody();
      moveFormActionMenuToControlPanel(controlPanel, body);
      root.replaceChildren(controlPanel, body);
    };
  } else {
    body = result.activeView === "form"
      ? renderFormView(viewDescription, fields, result.viewDescriptions.relatedModels, values, result.resModel, options)
      : result.activeView === "kanban"
        ? renderKanbanView(viewDescription, fields, records, result.resModel, result.action, result.search?.state.groupBy ?? [], options, {
          offset: result.offset,
          length: result.length,
          countLimited: result.countLimited,
          search: result.search
        })
      : renderListView(viewDescription, fields, records, result.resModel, result.action, options, {
        offset: result.offset,
        length: result.length,
        countLimited: result.countLimited
      });
    if (result.activeView === "form") moveFormActionMenuToControlPanel(controlPanel, body);
    if (result.activeView === "list") moveListActionMenuToControlPanel(controlPanel, body);
  }
  root.append(controlPanel, body);
  if (result.resModel === "ir.module.module" && result.activeView === "kanban") {
    (body as ModuleCatalogHost).__gorpActionRoot = root;
    root.setAttribute("style", "background:#262a36 !important;color:#e8e9ef !important;min-height:calc(100vh - 104px) !important;");
    syncModuleCatalogControlPanel(controlPanel, body);
  }
  return root;
}

function actionControlPanelSearchFacets(result: WindowActionResult): readonly SearchFacet[] {
  const facets = result.search?.state.facets ?? [];
  if (result.resModel !== "ir.module.module" || result.activeView !== "kanban") return facets;
  if (facets.some((facet) => facet.id === "apps" || facet.label === "Apps")) return facets;
  return [
    {
      id: "apps",
      type: "filter",
      label: "Apps",
      domain: [["application", "=", true]],
      context: { search_default_app: 1 }
    },
    ...facets
  ];
}

function syncModuleCatalogControlPanel(controlPanel: HTMLElement, body: HTMLElement): void {
  const total = Number(body.dataset.catalogTotal || "");
  if (!Number.isFinite(total) || total <= 0) return;
  const value = findDescendantByClass(controlPanel, "o_pager_value");
  const limit = findDescendantByClass(controlPanel, "o_pager_limit");
  if (value) value.textContent = `1-${total}`;
  if (limit) limit.textContent = String(total);
}

function moveFormActionMenuToControlPanel(controlPanel: HTMLElement, body: HTMLElement): void {
  const actionMenu = findDescendantByClass(body, "gorp-form-action-menu");
  if (!actionMenu) return;
  for (const existing of findDescendantsByClass(controlPanel, "gorp-form-action-menu")) removeDescendant(controlPanel, existing);
  const actions = findDescendantByClass(controlPanel, "o_control_panel_actions");
  const mainButtons = findDescendantByClass(controlPanel, "o_control_panel_main_buttons");
  const target = actions ?? mainButtons;
  if (!target) return;
  actionMenu.dataset.controlPanelPlacement = actions ? "actions" : "main_buttons";
  target.append(actionMenu);
}

function moveListActionMenuToControlPanel(controlPanel: HTMLElement, body: HTMLElement): void {
  const actionMenu = findDescendantByClass(body, "gorp-list-toolbar");
  if (!actionMenu) return;
  for (const existing of findDescendantsByClass(controlPanel, "gorp-list-toolbar")) removeDescendant(controlPanel, existing);
  const mainButtons = findDescendantByClass(controlPanel, "o_control_panel_main_buttons");
  if (!mainButtons) return;
  removeDescendant(body, actionMenu);
  actionMenu.dataset.controlPanelPlacement = "main_buttons";
  mainButtons.append(actionMenu);
}

function removeDescendant(root: HTMLElement, target: HTMLElement): void {
  if (typeof target.remove === "function") {
    target.remove();
    return;
  }
  if (target.parentElement && typeof target.parentElement.removeChild === "function") {
    target.parentElement.removeChild(target);
    return;
  }
  removeDescendantFromTestTree(root, target);
}

function removeDescendantFromTestTree(root: HTMLElement, target: HTMLElement): boolean {
  const children = (root as unknown as { children?: unknown }).children;
  if (!Array.isArray(children)) return false;
  const index = children.indexOf(target);
  if (index >= 0) {
    children.splice(index, 1);
    return true;
  }
  return children.some((child) => removeDescendantFromTestTree(child as HTMLElement, target));
}

export function renderWindowActionDialog(result: WindowActionResult, options: RenderWindowActionDialogOptions = {}): HTMLElement {
  const userPreferencesDialog = isUserPreferencesDialogResult(result);
  const overlay = document.createElement("section");
  overlay.className = "o_dialog gorp-action-dialog modal-open";
  overlay.dataset.target = "new";
  overlay.dataset.model = result.resModel;
  overlay.dataset.dialogOpen = "true";
  if (userPreferencesDialog) overlay.dataset.preferencesDialog = "true";
  overlay.setAttribute("tabindex", "-1");
  const backdrop = document.createElement("div");
  backdrop.className = "modal-backdrop o_dialog_backdrop gorp-action-dialog-backdrop show";
  backdrop.setAttribute("aria-hidden", "true");
  const modal = document.createElement("div");
  modal.className = "modal o_dialog_container show d-block";
  modal.setAttribute("role", "dialog");
  modal.setAttribute("aria-modal", "true");
  const titleID = uniqueId("dialog-title-");
  modal.setAttribute("aria-labelledby", titleID);
  const dialog = document.createElement("div");
  dialog.className = userPreferencesDialog
    ? "modal-dialog modal-lg gorp-preferences-modal-dialog"
    : "modal-dialog modal-lg";
  if (userPreferencesDialog) dialog.setAttribute("style", "max-width:980px;margin:70px auto 0;");
  const content = document.createElement("div");
  content.className = userPreferencesDialog ? "modal-content gorp-preferences-dialog-content" : "modal-content";
  if (userPreferencesDialog) content.setAttribute("style", "background:#282c36;color:#e8e9ef;border:1px solid #3a4050;border-radius:4px;box-shadow:0 16px 40px rgba(0,0,0,.35);overflow:hidden;max-height:calc(100vh - 120px);");
  const header = document.createElement("header");
  header.className = "modal-header";
  if (userPreferencesDialog) header.setAttribute("style", "min-height:60px;padding:0 16px;border-bottom:1px solid #3a4050;background:#282c36;color:#e8e9ef;");
  const title = document.createElement("h1");
  title.className = "modal-title";
  title.id = titleID;
  title.textContent = options.title || (typeof result.action.name === "string" && result.action.name.trim()) || result.resModel;
  if (userPreferencesDialog) title.setAttribute("style", "font-size:16px;line-height:60px;font-weight:600;color:#f4f5f7;");
  const close = document.createElement("button");
  close.type = "button";
  close.className = "btn-close";
  close.setAttribute("aria-label", "Close");
  if (userPreferencesDialog) close.setAttribute("style", "color:#aeb4c2;");
  close.addEventListener("click", () => {
    overlay.dataset.dialogOpen = "false";
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
  if (userPreferencesDialog) body.setAttribute("style", "flex:0 1 auto;min-height:0;max-height:calc(100vh - 230px);overflow:auto;padding:0;background:#282c36;color:#e8e9ef;");
  const dialogContent = renderWindowActionDialogContent(result, options);
  body.append(dialogContent.body);
  content.append(header, body);
  if (dialogContent.footer) content.append(dialogContent.footer);
  dialog.append(content);
  modal.append(dialog);
  overlay.append(backdrop, modal);
  return overlay;
}

function renderWindowActionDialogContent(result: WindowActionResult, options: RenderWindowActionDialogOptions): DialogWindowActionContent {
  const root = document.createElement("section");
  root.className = "gorp-window-action gorp-dialog-window-action";
  root.dataset.model = result.resModel;
  root.dataset.view = result.activeView;
  const viewDescription = result.viewDescriptions.views[result.activeView];
  const fields = result.viewDescriptions.fields;
  const records = options.records ?? result.records;
  const values = options.values ?? records?.[0] ?? result.records[0] ?? {};
  const settingsState = settingsActionState(result, values);
  const formState = settingsState ? null : formActionState(result, viewDescription, values, fields, { allowFooter: true, editMode: true });
  const footer = document.createElement("footer");
  footer.className = "modal-footer gorp-action-dialog-footer";
  let body: HTMLElement;
  if (isUserPreferencesDialogResult(result) && formState) {
    footer.setAttribute("style", "min-height:66px;padding:16px;border-top:1px solid #3a4050;background:#282c36;color:#e8e9ef;");
    appendFormActionButtonsToContainer(footer, root, result, formState, options, {
      includeEdit: false,
      saveLabel: "Update Preferences"
    });
    const renderBody = () => renderUserPreferencesDialogView(
      fields,
      result.viewDescriptions.relatedModels,
      formState.currentValues,
      dialogFormRenderOptions(options, formState),
      formState.editing
    );
    body = renderBody();
    formState.renderBody = () => {
      body = renderBody();
      root.replaceChildren(body);
    };
  } else if (settingsState) {
    appendSettingsActionButtonsToContainer(footer, root, result, settingsState, options);
    body = renderSettingsActionView(result, viewDescription, fields, settingsState, root);
  } else if (result.activeView === "form" && formState) {
    appendFormActionButtonsToContainer(footer, root, result, formState, options, { includeEdit: false });
    const renderBody = () => renderFormView(
      viewDescription,
      fields,
      result.viewDescriptions.relatedModels,
      formState.currentValues,
      result.resModel,
      dialogFormRenderOptions(options, formState),
      formState.editing
    );
    const renderFooterButtons = () => {
      removeDialogFooterViewButtons(footer);
      for (const button of dialogFooterFormButtons(result, viewDescription, formState.currentValues, body, options)) {
        footer.append(button);
      }
    };
    body = renderBody();
    renderFooterButtons();
    formState.renderBody = () => {
      body = renderBody();
      root.replaceChildren(body);
      renderFooterButtons();
    };
  } else if (result.activeView !== "form") {
    body = renderWindowAction(result, options);
  } else {
    body = renderFormView(viewDescription, fields, result.viewDescriptions.relatedModels, values, result.resModel, dialogFormRenderOptions(options, null), true);
  }
  root.append(body);
  return {
    body: root,
    ...(footer.children.length ? { footer } : {})
  };
}

function isUserPreferencesDialogResult(result: WindowActionResult): boolean {
  const context = isRecord(result.action.context) ? result.action.context : {};
  const actionName = firstText(result.action.name);
  return result.resModel === "res.users"
    && result.activeView === "form"
    && (context.gorp_preferences_dialog === true || actionName === "Change My Preferences")
    && result.action.target === "new";
}

function renderUserPreferencesDialogView(
  fields: Record<string, unknown>,
  relatedModels: Record<string, unknown>,
  values: Record<string, unknown>,
  options: RenderWindowActionOptions,
  editMode: boolean
): HTMLElement {
  const form = document.createElement("form");
  form.className = "gorp-form-view o_form_view gorp-user-preferences-form";
  form.dataset.model = "res.users";
  form.dataset.preferencesDialog = "true";
  const effectiveFields = userPreferencesFields(fields, values);
  const effectiveValues = {
    ...values,
    lang: values.lang ?? "en_US",
    color_scheme: values.color_scheme ?? "system"
  };
  const body = document.createElement("div");
  body.className = "gorp-form-body o-list-content o-form-content o_form_sheet_bg gorp-user-preferences-body";
  body.setAttribute("style", "background:#282c36 !important;color:#e8e9ef !important;padding:16px;");
  const sheet = document.createElement("section");
  sheet.className = "gorp-form-sheet o-form-sheet o_form_sheet gorp-user-preferences-sheet";
  sheet.append(renderUserPreferencesIdentity(effectiveValues));
  sheet.append(renderUserPreferencesTabs());
  const groups = document.createElement("div");
  groups.className = "gorp-user-preferences-groups o_group";
  appendUserPreferencesGroup(groups, "", ["lang", "color_scheme", "signature"], effectiveFields, relatedModels, effectiveValues, form, options, editMode);
  if (groups.children.length) sheet.append(groups);
  body.append(sheet);
  form.append(body);
  applyUserPreferencesChrome(form);
  return form;
}

function renderUserPreferencesIdentity(values: Record<string, unknown>): HTMLElement {
  const root = document.createElement("section");
  root.className = "gorp-user-preferences-identity oe_title";
  root.setAttribute("style", "display:flex;align-items:center;gap:16px;margin:0;padding:0 0 16px;");
  const avatar = document.createElement("span");
  avatar.className = "gorp-user-preferences-avatar o_avatar";
  avatar.dataset.userId = String(values.id ?? "");
  avatar.textContent = userInitial(values);
  avatar.setAttribute("style", "width:130px;height:130px;display:flex;align-items:center;justify-content:center;border-radius:0;background:#86a844;color:#fff;font-size:70px;line-height:1;font-weight:400;");
  const text = document.createElement("div");
  text.className = "gorp-user-preferences-title";
  const title = document.createElement("h1");
  title.textContent = String(values.name ?? values.display_name ?? values.login ?? "User");
  title.setAttribute("style", "margin:0 0 12px;font-size:32px;line-height:1.15;font-weight:500;color:#f4f5f7;");
  const email = document.createElement("span");
  email.className = "text-muted";
  email.setAttribute("style", "display:block;color:#aeb4c2;");
  email.textContent = String(values.email || "Email");
  const phone = document.createElement("span");
  phone.className = "text-muted";
  phone.setAttribute("style", "display:block;margin-top:6px;color:#aeb4c2;");
  phone.textContent = String(values.phone || values.mobile_phone || "Phone");
  const login = document.createElement("span");
  login.className = "text-muted";
  login.hidden = true;
  login.textContent = String(values.login ?? values.email ?? "");
  text.append(title, email, phone, login);
  root.append(avatar, text);
  return root;
}

function renderUserPreferencesTabs(): HTMLElement {
  const tabs = document.createElement("div");
  tabs.className = "gorp-user-preferences-tabs nav nav-tabs";
  tabs.setAttribute("style", "display:flex;align-items:flex-end;margin:0 -16px 16px;border-bottom:1px solid #3a4050;");
  for (const label of ["Preferences", "Calendar", "Security"]) {
    const tab = document.createElement("button");
    tab.type = "button";
    tab.className = label === "Preferences" ? "nav-link active" : "nav-link";
    tab.textContent = label;
    tab.dataset.preferencesTab = label.toLowerCase();
    tab.setAttribute("style", label === "Preferences"
      ? "min-height:38px;padding:8px 18px;border:0;border-top:3px solid #c060a1;background:#303440;color:#f4f5f7;font-weight:500;"
      : "min-height:38px;padding:8px 18px;border:0;background:#303440;color:#f4f5f7;font-weight:500;");
    tabs.append(tab);
  }
  return tabs;
}

function appendUserPreferencesGroup(
  parent: HTMLElement,
  title: string,
  fieldNames: readonly string[],
  fields: Record<string, unknown>,
  relatedModels: Record<string, unknown>,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  editMode: boolean
): void {
  const group = document.createElement("section");
  group.className = "gorp-user-preferences-group o_inner_group";
  group.dataset.preferenceGroup = title ? title.toLowerCase().replace(/\s+/g, "-") : "preferences";
  const heading = document.createElement("h2");
  heading.className = "gorp-user-preferences-group-title";
  heading.textContent = title;
  heading.setAttribute("style", "margin:0 0 10px;font-size:16px;line-height:1.2;font-weight:500;color:#f4f5f7;");
  const grid = document.createElement("div");
  grid.className = "gorp-form-fields record-grid o_group";
  grid.setAttribute("style", "display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1fr);gap:14px 32px;align-items:start;");
  for (const name of fieldNames) {
    if (fields[name] === undefined) continue;
    if (!userPreferenceFieldVisible(name, values, fields)) continue;
    const fieldNode = renderFormFieldNode(
      { name, attrs: {}, children: [], childViewAttrs: {} },
      fields,
      relatedModels,
      values,
      form,
      options,
      editMode
    );
    if (userPreferenceRenderedFieldEmpty(fieldNode)) continue;
    grid.append(fieldNode);
  }
  if (!grid.children.length) return;
  if (title) group.append(heading);
  group.append(grid);
  parent.append(group);
}

function userPreferenceRenderedFieldEmpty(node: HTMLElement): boolean {
  if (typeof node.querySelector === "function") {
    if (node.querySelector("input, textarea, select, button, .gorp-many2one-editor, .gorp-x2many-tags")) return false;
  }
  const testChildren = (node as unknown as { children?: unknown }).children;
  if (typeof node.querySelector !== "function" && Array.isArray(testChildren) && testChildren.length > 0) return false;
  const label = node.querySelector?.(".o_form_label")?.textContent?.trim() || "";
  const text = String(node.textContent ?? "").replace(label, "").trim();
  return !text;
}

function applyUserPreferencesChrome(form: HTMLElement): void {
  const sheet = form.querySelector?.(".gorp-user-preferences-sheet") as HTMLElement | null;
  sheet?.setAttribute("style", mergeInlineStyle(sheet.getAttribute("style"), "max-width:100%;min-height:0;margin:0;padding:0;background:#282c36 !important;color:#e8e9ef !important;border:0 !important;box-shadow:none !important;"));
  for (const field of Array.from(form.querySelectorAll?.(".gorp-form-field.o_wrap_field") ?? [])) {
    (field as HTMLElement).setAttribute("style", mergeInlineStyle((field as HTMLElement).getAttribute("style"), "display:grid;grid-template-columns:130px minmax(0,1fr);align-items:start;gap:8px;min-height:32px;margin:0;"));
  }
  for (const label of Array.from(form.querySelectorAll?.(".o_form_label") ?? [])) {
    (label as HTMLElement).setAttribute("style", mergeInlineStyle((label as HTMLElement).getAttribute("style"), "color:#f4f5f7 !important;font-size:14px;font-weight:600;"));
  }
  for (const control of Array.from(form.querySelectorAll?.("input.o_input, textarea.o_input, select.o_input, .gorp-form-control.o_input") ?? [])) {
    const element = control as HTMLElement & { type?: string };
    if (["checkbox", "radio"].includes(String(element.type || "").toLowerCase())) continue;
    element.setAttribute("style", mergeInlineStyle(element.getAttribute("style"), "min-height:30px;background:#282c36 !important;color:#f4f5f7 !important;border:1px solid #3f4656 !important;border-radius:0 !important;box-shadow:none !important;"));
  }
  for (const toggle of Array.from(form.querySelectorAll?.(".gorp-many2one-dropdown-toggle") ?? [])) {
    (toggle as HTMLElement).setAttribute("style", mergeInlineStyle((toggle as HTMLElement).getAttribute("style"), "background:#282c36 !important;color:#aeb4c2 !important;border-color:#3f4656 !important;box-shadow:none !important;"));
  }
  for (const output of Array.from(form.querySelectorAll?.(".gorp-field-value, .gorp-many2one-value, .gorp-many2one-link") ?? [])) {
    (output as HTMLElement).setAttribute("style", mergeInlineStyle((output as HTMLElement).getAttribute("style"), "color:#f4f5f7 !important;background:transparent !important;"));
  }
  for (const group of Array.from(form.querySelectorAll?.(".gorp-selection-radio-group") ?? [])) {
    (group as HTMLElement).setAttribute("style", mergeInlineStyle((group as HTMLElement).getAttribute("style"), "display:inline-flex;align-items:center;gap:12px;min-height:28px;color:#f4f5f7 !important;"));
  }
  for (const pill of Array.from(form.querySelectorAll?.(".gorp-selection-radio-pill") ?? [])) {
    (pill as HTMLElement).setAttribute("style", mergeInlineStyle((pill as HTMLElement).getAttribute("style"), "display:inline-flex;align-items:center;gap:6px;margin:0;padding:0;background:transparent !important;border:0 !important;color:#f4f5f7 !important;font-weight:500;box-shadow:none !important;"));
  }
  for (const input of Array.from(form.querySelectorAll?.(".gorp-selection-radio-pill input") ?? [])) {
    (input as HTMLElement).setAttribute("style", mergeInlineStyle((input as HTMLElement).getAttribute("style"), "accent-color:#00dac5;margin:0;"));
  }
  for (const input of Array.from(form.querySelectorAll?.(".gorp-selection-radio-group[data-field='lang'] input") ?? [])) {
    (input as HTMLElement).setAttribute("style", mergeInlineStyle((input as HTMLElement).getAttribute("style"), "display:none;"));
  }
  for (const marker of Array.from(form.querySelectorAll?.(".gorp-selection-radio-group[data-field='lang'] .gorp-selection-radio-marker") ?? [])) {
    (marker as HTMLElement).setAttribute("style", mergeInlineStyle((marker as HTMLElement).getAttribute("style"), "display:none;"));
  }
}

function userPreferencesFields(fields: Record<string, unknown>, values: Record<string, unknown>): Record<string, unknown> {
  const out = { ...fields };
  for (const name of ["name", "login", "email", "company_id", "partner_id", "lang", "color_scheme", "notification_type", "signature"]) {
    if (out[name] === undefined && (values[name] !== undefined || userPreferencesFallbackField(name))) {
      out[name] = userPreferencesFallbackField(name);
    }
  }
  out.lang = normalizeUserPreferenceSelectionField(out.lang, userPreferencesFallbackField("lang"));
  out.color_scheme = normalizeUserPreferenceSelectionField(out.color_scheme, userPreferencesFallbackField("color_scheme"));
  if (out.signature !== undefined) out.signature = { ...(isRecord(out.signature) ? out.signature : {}), type: fieldTypeValue(out.signature) || "text", string: "Email Signature" };
  return out;
}

function normalizeUserPreferenceSelectionField(description: unknown, fallback: unknown): unknown {
  if (!isRecord(fallback)) return description;
  const choices = selectionOptions(description);
  return {
    ...(isRecord(description) ? description : {}),
    type: "selection",
    string: firstText(isRecord(fallback) ? fallback.string : "") || fieldLabel({ field: fallback }, "field"),
    selection: choices.length ? choices : fallback.selection
  };
}

function userPreferenceFieldVisible(name: string, values: Record<string, unknown>, fields: Record<string, unknown>): boolean {
  if (fields[name] === undefined) return false;
  if (["lang", "tz", "color_scheme"].includes(name)) return values[name] !== undefined || fieldTypeValue(fields[name]) === "selection";
  return true;
}

function userPreferencesFallbackField(name: string): unknown {
  switch (name) {
    case "name":
      return { type: "char", string: "Name", required: true };
    case "login":
      return { type: "char", string: "Login" };
    case "email":
      return { type: "char", string: "Email" };
    case "company_id":
      return { type: "many2one", relation: "res.company", string: "Company" };
    case "partner_id":
      return { type: "many2one", relation: "res.partner", string: "Related Partner" };
    case "lang":
      return { type: "selection", string: "Language", selection: [["en_US", "English (US)"]] };
    case "color_scheme":
      return { type: "selection", string: "Theme", selection: [["system", "System"], ["light", "Light"], ["dark", "Dark"]] };
    case "notification_type":
      return {
        type: "selection",
        string: "Notification",
        selection: [["email", "Handle by Emails"], ["inbox", "Handle in Odoo"]]
      };
    case "signature":
      return { type: "text", string: "Signature" };
    default:
      return undefined;
  }
}

function dialogFormRenderOptions(options: RenderWindowActionOptions, state: FormActionState | null): RenderWindowActionOptions {
  return {
    ...(state ? formRenderOptions(options, state) : options),
    formButtonPlacement: "excludeFooter",
    disableFormActionMenu: true
  } as RenderWindowActionOptions;
}

function dialogFooterFormButtons(
  result: WindowActionResult,
  viewDescription: ViewDescription | undefined,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement[] {
  const activeFieldNames = new Set(parseViewFieldNodes(viewDescription?.arch ?? "").map((node) => node.name));
  return parseFormFooterButtonNodes(viewDescription?.arch ?? "")
    .filter((node) => !nodeInvisible(node.attrs, values))
    .map((node) => {
      const button = renderFormButton(result.resModel, node, values, activeFieldNames, form, options);
      button.className = [button.className, "gorp-dialog-footer-view-button"].filter(Boolean).join(" ");
      return button;
    });
}

function removeDialogFooterViewButtons(footer: HTMLElement): void {
  for (const child of Array.from(footer.querySelectorAll?.(".gorp-dialog-footer-view-button") ?? [])) {
    child.remove();
  }
}

function settingsActionState(result: WindowActionResult, values: Record<string, unknown>): SettingsActionState | null {
  if (result.resModel !== "res.config.settings" || result.activeView !== "form") return null;
  const initialValues = cloneRecord(values);
  return {
    initialValues,
    currentValues: cloneRecord(values),
    dirtyFields: new Set(),
    search: ""
  };
}

function formActionState(
  result: WindowActionResult,
  viewDescription: ViewDescription | undefined,
  values: Record<string, unknown>,
  fields: Record<string, unknown>,
  options: { allowFooter?: boolean; editMode?: boolean } = {}
): FormActionState | null {
  if (result.activeView !== "form") return null;
  if (result.resModel === "res.config.settings") return null;
  if (!options.allowFooter && /<footer(?:\s|\/|>)/.test(viewDescription?.arch ?? "")) return null;
  const initialValues = cloneRecord(values);
  const isNewRecord = numberRecordID(values.id) === undefined;
  return {
    initialValues,
    currentValues: cloneRecord(values),
    dirtyFields: new Set(),
    editing: options.editMode === true || isNewRecord,
    fields
  };
}

function formRenderOptions(options: RenderWindowActionOptions, state: FormActionState): RenderWindowActionOptions {
  return {
    ...options,
    onUpdate(name, value) {
      updateFormActionPendingValue(state, name, value);
      options.onUpdate?.(name, value);
    }
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
      activeApp: settingsActiveApp(result.action),
      search: state.search,
      showSearchPanel: false
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
  appendSettingsActionButtonsToContainer(mainButtons, eventRoot, result, state, options);
}

function appendSettingsActionButtonsToContainer(
  container: HTMLElement,
  eventRoot: HTMLElement,
  result: WindowActionResult,
  state: SettingsActionState,
  options: RenderWindowActionOptions
): void {
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
  container.append(save, discard, status);
  updateSettingsButtons(state);
}

function appendFormActionButtons(
  controlPanel: HTMLElement,
  eventRoot: HTMLElement,
  result: WindowActionResult,
  state: FormActionState,
  options: RenderWindowActionOptions
): void {
  const mainButtons = findDescendantByClass(controlPanel, "o_control_panel_main_buttons");
  if (!mainButtons) return;
  appendFormActionButtonsToContainer(mainButtons, eventRoot, result, state, options);
}

function appendFormActionButtonsToContainer(
  container: HTMLElement,
  eventRoot: HTMLElement,
  result: WindowActionResult,
  state: FormActionState,
  options: RenderWindowActionOptions,
  buttonOptions: { includeEdit?: boolean; saveLabel?: string; discardLabel?: string } = {}
): void {
  const includeEdit = buttonOptions.includeEdit !== false;
  const edit = document.createElement("button");
  edit.type = "button";
  edit.className = "btn btn-primary o_form_button_edit";
  edit.dataset.formAction = "edit";
  edit.textContent = result.resModel === "res.users" || result.resModel === "res.groups" ? "New" : "Edit";
  const save = document.createElement("button");
  save.type = "button";
  save.className = "btn btn-primary o_form_button_save";
  save.dataset.formAction = "save";
  save.textContent = buttonOptions.saveLabel || "Save";
  const discard = document.createElement("button");
  discard.type = "button";
  discard.className = "btn btn-secondary o_form_button_cancel";
  discard.dataset.formAction = "discard";
  discard.textContent = buttonOptions.discardLabel || "Discard";
  const status = document.createElement("span");
  status.className = "o_form_dirty_status text-muted";
  state.editButton = edit;
  state.saveButton = save;
  state.discardButton = discard;
  state.status = status;
  edit.addEventListener("click", () => {
    state.editing = true;
    setFormActionStatus(state, "");
    updateFormActionButtons(state);
    state.renderBody?.();
    eventRoot.dispatchEvent(new CustomEvent("form:edit", {
      bubbles: true,
      detail: { model: result.resModel, id: numberRecordID(state.currentValues.id) }
    }));
  });
  save.addEventListener("click", () => {
    void saveFormAction(eventRoot, result, state, options).catch((error) => {
      setFormActionStatus(state, error instanceof Error ? error.message : String(error));
      eventRoot.dispatchEvent(new CustomEvent("form:save-error", {
        bubbles: true,
        detail: { model: result.resModel, error }
      }));
    });
  });
  discard.addEventListener("click", () => {
    discardFormAction(eventRoot, state);
  });
  if (includeEdit) container.append(edit);
  container.append(save, discard, status);
  updateFormActionButtons(state);
}

function appendTechnicalFormSmartButtons(
  controlPanel: HTMLElement,
  eventRoot: HTMLElement,
  result: WindowActionResult,
  values: Record<string, unknown>
): void {
  if (result.activeView !== "form" || result.resModel !== "ir.actions.server") return;
  const count = serverActionScheduledCount(values);
  if (count < 1) return;
  const actions = findDescendantByClass(controlPanel, "o_control_panel_actions");
  if (!actions) return;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-secondary gorp-server-action-smart-button o_stat_button";
  button.dataset.serverActionSmartButton = "scheduled-action";
  button.dataset.count = String(count);
  button.textContent = "Scheduled Action";
  button.addEventListener("click", () => {
    eventRoot.dispatchEvent(new CustomEvent("server-action:open-scheduled", {
      bubbles: true,
      detail: {
        id: numberRecordID(values.id),
        count
      }
    }));
  });
  const target = actions as HTMLElement & { prepend?: (...nodes: HTMLElement[]) => void; children?: unknown[] };
  if (typeof target.prepend === "function") {
    target.prepend(button);
  } else if (Array.isArray(target.children)) {
    target.children.unshift(button);
  } else {
    actions.append(button);
  }
}

function serverActionScheduledCount(values: Record<string, unknown>): number {
  const relations = values.ir_cron_ids;
  if (Array.isArray(relations)) return Math.max(0, relations.length);
  if (firstText(values.usage) === "ir_cron") return 1;
  return 0;
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

function updateFormActionPendingValue(state: FormActionState, name: string, value: unknown): void {
  state.currentValues = { ...state.currentValues, [name]: value };
  if (sameSettingsValue(state.initialValues[name], value)) {
    state.dirtyFields.delete(name);
  } else {
    state.dirtyFields.add(name);
  }
  setFormActionStatus(state, state.dirtyFields.size ? "Unsaved changes" : "");
  updateFormActionButtons(state);
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

async function saveFormAction(
  eventRoot: HTMLElement,
  result: WindowActionResult,
  state: FormActionState,
  options: RenderWindowActionOptions
): Promise<void> {
  const changes = formChangedValues(state);
  if (!Object.keys(changes).length) {
    state.editing = false;
    updateFormActionButtons(state);
    state.renderBody?.();
    return;
  }
  state.saveButton?.setAttribute("aria-busy", "true");
  try {
    const recordID = numberRecordID(state.currentValues.id);
    const context = isRecord(result.action.context) ? result.action.context : options.context ?? {};
    if (options.services?.orm && recordID !== undefined) {
      await options.services.orm.webSave(result.resModel, [recordID], changes, { context });
    } else if (options.services?.orm && recordID === undefined) {
      const created = await options.services.orm.create<unknown>(result.resModel, [{ ...state.currentValues }], { context });
      const createdID = createdRecordID(created);
      if (createdID !== undefined) state.currentValues = { ...state.currentValues, id: createdID };
    }
  } finally {
    state.saveButton?.removeAttribute("aria-busy");
  }
  state.initialValues = cloneRecord(state.currentValues);
  state.dirtyFields.clear();
  state.editing = false;
  setFormActionStatus(state, "Saved");
  updateFormActionButtons(state);
  state.renderBody?.();
  eventRoot.dispatchEvent(new CustomEvent("form:save", {
    bubbles: true,
    detail: { model: result.resModel, id: numberRecordID(state.currentValues.id), values: cloneRecord(state.currentValues), changes }
  }));
}

function discardFormAction(eventRoot: HTMLElement, state: FormActionState): void {
  state.currentValues = cloneRecord(state.initialValues);
  state.dirtyFields.clear();
  state.editing = false;
  setFormActionStatus(state, "");
  updateFormActionButtons(state);
  state.renderBody?.();
  eventRoot.dispatchEvent(new CustomEvent("form:discard", {
    bubbles: true,
    detail: { values: cloneRecord(state.currentValues) }
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

function formChangedValues(state: FormActionState): Record<string, unknown> {
  const changes: Record<string, unknown> = {};
  for (const name of state.dirtyFields) {
    changes[name] = formSaveValue(state.fields[name], state.currentValues[name]);
  }
  return changes;
}

function formSaveValue(description: unknown, value: unknown): unknown {
  const fieldType = fieldTypeValue(description);
  if (fieldType === "many2many") {
    return [x2ManyCommands.set(x2ManySelectedIDs(value))];
  }
  if (fieldType === "one2many" && isOne2ManyEditorValue(value)) {
    return value.commands;
  }
  if (fieldType !== "many2one") return value;
  const id = many2OneDisplayData(value).id;
  return id ?? false;
}

function updateSettingsButtons(state: SettingsActionState): void {
  const dirty = state.dirtyFields.size > 0;
  if (state.saveButton) state.saveButton.disabled = !dirty;
  if (state.discardButton) state.discardButton.disabled = !dirty;
}

function updateFormActionButtons(state: FormActionState): void {
  const dirty = state.dirtyFields.size > 0;
  if (state.editButton) state.editButton.hidden = state.editing;
  if (state.saveButton) {
    state.saveButton.hidden = !state.editing;
    state.saveButton.disabled = !dirty;
  }
  if (state.discardButton) {
    state.discardButton.hidden = !state.editing;
    state.discardButton.disabled = !state.editing;
  }
}

function setSettingsStatus(state: SettingsActionState, value: string): void {
  if (state.status) state.status.textContent = value;
}

function setFormActionStatus(state: FormActionState, value: string): void {
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

function renderWindowActionControlPanel(
  result: WindowActionResult,
  root: HTMLElement,
  options: RenderWindowActionOptions,
  settingsState?: SettingsActionState | null
): HTMLElement {
  const pagerLimit = numberActionValue(result.action.limit, 80);
  const isSettings = Boolean(settingsState && result.resModel === "res.config.settings" && result.activeView === "form");
  const activePager = isSettings
    ? undefined
    : result.activeView === "form"
      ? formActionPager(result)
      : { offset: result.offset, limit: pagerLimit, total: result.length, totalLimited: result.countLimited };
  const views = normalizeActionViews(result.action)
    .filter((view) => view[1] !== "search")
    .map<ActionControlPanelView>((view) => ({
      type: view[1],
      label: viewLabel(view[1]),
      active: view[1] === result.activeView
    }));
  const controlPanel = renderActionControlPanel({
    title: typeof result.action.name === "string" ? result.action.name : result.resModel,
    pager: activePager,
    views: isSettings ? [] : views,
    search: isSettings ? {
      query: settingsState?.search ?? "",
      placeholder: "Search...",
      facets: [],
      suggestions: []
    } : result.activeView === "form" ? undefined : {
      query: result.search?.state.query ?? "",
      facets: actionControlPanelSearchFacets(result),
      suggestions: result.search?.suggestions ?? []
    },
    filters: result.activeView === "form" ? [] : result.search?.filters ?? [],
    groupBys: result.activeView === "form" ? [] : result.search?.groupBys ?? [],
    favorites: result.activeView === "form" ? [] : result.search?.favorites ?? []
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
      if (isSettings && settingsState) {
        settingsState.search = query;
        settingsState.renderBody?.();
        root.dispatchEvent(new CustomEvent("settings:search", {
          bubbles: true,
          detail: { query }
        }));
        return;
      }
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
    onDeleteFavorite: (item) => {
      if (deleteSearchFavorite(result, item, root, options)) return;
      dispatchSearchMenuEvent(root, "action:search-favorite-delete", result, item);
    },
    onFacetRemove: (facet) => {
      if (rerunActionWithFacets(result, withoutSearchFacet(result.search?.state.facets ?? [], facet.id), options)) return;
      root.dispatchEvent(new CustomEvent("action:search-facet-remove", {
        bubbles: true,
        detail: { facet, model: result.resModel }
      }));
    },
    onSearchSuggestion: (suggestion) => {
      if (rerunActionWithSearchSuggestion(result, suggestion, options)) return;
      root.dispatchEvent(new CustomEvent("action:search-suggestion", {
        bubbles: true,
        detail: { suggestion, model: result.resModel }
      }));
    },
    onPagerPrevious: () => {
      const currentOffset = activePager?.offset ?? result.offset;
      const currentLimit = activePager?.limit ?? pagerLimit;
      if (rerunActionWithPagerOffset(result, Math.max(0, currentOffset - currentLimit), options)) return;
      root.dispatchEvent(new CustomEvent("action:pager-previous", {
        bubbles: true,
        detail: { model: result.resModel, offset: currentOffset, limit: currentLimit }
      }));
    },
    onPagerNext: () => {
      const currentOffset = activePager?.offset ?? result.offset;
      const currentLimit = activePager?.limit ?? pagerLimit;
      const currentTotal = activePager?.total ?? result.length;
      const totalLimited = activePager?.totalLimited === true;
      const nextOffset = totalLimited
        ? currentOffset + currentLimit
        : Math.min(Math.max(0, currentTotal - 1), currentOffset + currentLimit);
      if (rerunActionWithPagerOffset(result, nextOffset, options)) return;
      root.dispatchEvent(new CustomEvent("action:pager-next", {
        bubbles: true,
        detail: { model: result.resModel, offset: currentOffset, limit: currentLimit }
      }));
    },
    onPagerCount: () => {
      if (fetchWindowActionExactCount(result, options)) return;
      root.dispatchEvent(new CustomEvent("action:pager-count", {
        bubbles: true,
        detail: { model: result.resModel, offset: result.offset, limit: pagerLimit }
      }));
    },
    onAddCustomFilter: () => {
      if (openCustomFilterDialog(result, root, options)) return;
      dispatchSearchUtilityEvent(root, "action:search-custom-filter", result);
    },
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

function formActionPager(result: WindowActionResult): ActionControlPanelPager | undefined {
  const recordID = numberRecordID(result.action.res_id) ?? numberRecordID(result.records[0]?.id);
  if (recordID === undefined && result.length <= 0) return undefined;
  const exactTotal = actionPagerExactTotal(result.action);
  const total = Math.max(1, Math.trunc(numberActionValue(exactTotal, result.length || result.records.length || 1)));
  const rawOffset = exactTotal !== undefined ? actionPagerOffset(result.action) : result.offset;
  const offset = Math.min(Math.max(0, Math.trunc(rawOffset || 0)), total - 1);
  return { offset, limit: 1, total, totalLimited: result.action.__pager_total_limited === true || result.countLimited };
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
  delete next.__pager_offset;
  delete next.__pager_total;
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

function rerunActionWithSearchSuggestion(
  result: WindowActionResult,
  suggestion: ActionControlPanelSearchSuggestion,
  options: RenderWindowActionOptions
): boolean {
  if (!options.services?.action) return false;
  const facet = searchSuggestionFacet(suggestion);
  if (!facet) return false;
  const currentFacets = result.search?.state.facets ?? [];
  const nextFacets = currentFacets.some((item) => item.id === facet.id)
    ? currentFacets.map(cloneSearchFacet)
    : [...currentFacets.map(cloneSearchFacet), facet];
  const nextAction = actionWithCurrentSearch(result, nextFacets);
  delete nextAction.__search_query;
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
  const nextFacets = mode === "toggle" && isGeneratedDateFilterSearchFacet(item.facet)
    ? toggleDateFilterPeriodFacet(currentFacets, item.facet, result.search?.filters ?? [])
    : mode === "replace"
    ? [cloneSearchFacet(item.facet)]
    : toggleSearchFacet(currentFacets, item.facet);
  return rerunActionWithFacets(result, nextFacets, options);
}

function rerunActionWithFacets(result: WindowActionResult, facets: readonly SearchFacet[], options: RenderWindowActionOptions): boolean {
  if (!options.services?.action) return false;
  void options.services.action.doAction(actionWithCurrentSearch(result, facets), replaceActionOptions(options));
  return true;
}

function deleteSearchFavorite(
  result: WindowActionResult,
  item: ActionControlPanelMenuItem,
  root: HTMLElement,
  options: RenderWindowActionOptions
): boolean {
  const favoriteID = item.favorite?.id;
  const orm = options.services?.orm;
  if (!favoriteID || !orm) return false;
  void orm.unlink("ir.filters", [favoriteID]).then(() => {
    options.services?.notification?.add("Favorite deleted", { type: "success" });
    const currentFacets = result.search?.state.facets ?? [];
    const nextFacets = item.facet ? withoutSearchFacet(currentFacets, item.facet.id) : currentFacets.map(cloneSearchFacet);
    if (options.services?.action) {
      return options.services.action.doAction(actionWithCurrentSearch(result, nextFacets), replaceActionOptions(options));
    }
    root.dispatchEvent(new CustomEvent("action:search-favorite-deleted", {
      bubbles: true,
      detail: { model: result.resModel, id: favoriteID }
    }));
    return undefined;
  }).catch((error) => {
    options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
    root.dispatchEvent(new CustomEvent("action:search-favorite-delete-error", {
      bubbles: true,
      detail: { model: result.resModel, id: favoriteID, error }
    }));
  });
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

interface CustomFilterFieldOption {
  name: string;
  label: string;
  type: string;
}

interface CustomFilterOperatorOption {
  value: string;
  label: string;
}

function openCustomFilterDialog(result: WindowActionResult, root: HTMLElement, options: RenderWindowActionOptions): boolean {
  if (!options.services?.action || result.activeView === "form") return false;
  const fields = customFilterFieldOptions(result);
  if (!fields.length) return false;
  const operators = customFilterOperatorOptions();
  const overlay = document.createElement("section");
  overlay.className = "o_dialog gorp-custom-filter-dialog modal-open";
  overlay.dataset.customFilterDialog = result.resModel;
  overlay.setAttribute("role", "dialog");
  overlay.setAttribute("aria-modal", "true");
  const modal = document.createElement("div");
  modal.className = "modal o_dialog_container show d-block";
  const dialog = document.createElement("div");
  dialog.className = "modal-dialog";
  const content = document.createElement("div");
  content.className = "modal-content";
  const header = document.createElement("header");
  header.className = "modal-header";
  const title = document.createElement("h1");
  title.className = "modal-title";
  title.textContent = "Add Custom Filter";
  const close = document.createElement("button");
  close.type = "button";
  close.className = "btn-close";
  close.dataset.customFilterClose = "true";
  close.setAttribute("aria-label", "Close");
  close.addEventListener("click", () => removeDescendant(root, overlay));
  header.append(title, close);

  const body = document.createElement("div");
  body.className = "modal-body o_add_custom_filter_menu";
  const row = document.createElement("div");
  row.className = "o_custom_filter_row d-flex gap-2 align-items-center";
  const fieldSelect = document.createElement("select");
  fieldSelect.className = "o_input o_custom_filter_field";
  fieldSelect.dataset.customFilterField = "true";
  fieldSelect.setAttribute("aria-label", "Field");
  for (const field of fields) {
    const option = document.createElement("option");
    option.value = field.name;
    option.textContent = field.label;
    option.dataset.fieldType = field.type;
    fieldSelect.append(option);
  }
  fieldSelect.value = fields[0].name;
  const operatorSelect = document.createElement("select");
  operatorSelect.className = "o_input o_custom_filter_operator";
  operatorSelect.dataset.customFilterOperator = "true";
  operatorSelect.setAttribute("aria-label", "Operator");
  for (const operator of operators) {
    const option = document.createElement("option");
    option.value = operator.value;
    option.textContent = operator.label;
    operatorSelect.append(option);
  }
  operatorSelect.value = operators[0].value;
  const valueInput = document.createElement("input");
  valueInput.className = "o_input o_custom_filter_value";
  valueInput.dataset.customFilterValue = "true";
  valueInput.type = "text";
  valueInput.placeholder = "Value";
  row.append(fieldSelect, operatorSelect, valueInput);
  body.append(row);

  const footer = document.createElement("footer");
  footer.className = "modal-footer";
  const apply = document.createElement("button");
  apply.type = "button";
  apply.className = "btn btn-primary o_apply_filter";
  apply.dataset.customFilterApply = "true";
  apply.textContent = "Apply";
  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.className = "btn btn-secondary o_discard_filter";
  cancel.dataset.customFilterCancel = "true";
  cancel.textContent = "Cancel";
  cancel.addEventListener("click", () => removeDescendant(root, overlay));
  apply.addEventListener("click", () => {
    const facet = customFilterFacet(result, fields, fieldSelect.value, operatorSelect.value, valueInput.value);
    if (!facet) return;
    const currentFacets = result.search?.state.facets ?? [];
    const nextAction = actionWithCurrentSearch(result, [...currentFacets.map(cloneSearchFacet), facet]);
    delete nextAction.__search_query;
    void options.services?.action?.doAction(nextAction, replaceActionOptions(options));
    removeDescendant(root, overlay);
  });
  footer.append(cancel, apply);
  content.append(header, body, footer);
  dialog.append(content);
  modal.append(dialog);
  overlay.append(modal);
  root.append(overlay);
  valueInput.focus?.();
  return true;
}

function customFilterFieldOptions(result: WindowActionResult): CustomFilterFieldOption[] {
  const fields = result.viewDescriptions.fields ?? {};
  const preferred = customFilterPreferredFieldNames(result.resModel, fields);
  const seenLabels = new Set<string>();
  const options: CustomFilterFieldOption[] = [];
  for (const name of preferred) {
    const description = fields[name];
    if (!isCustomFilterableField(description)) continue;
    const label = fieldLabel(fields, name, result.resModel);
    const key = label.toLowerCase();
    if (seenLabels.has(key)) continue;
    seenLabels.add(key);
    options.push({ name, label, type: fieldTypeValue(description) });
  }
  return options;
}

function customFilterPreferredFieldNames(model: string, fields: Record<string, unknown>): string[] {
  const names = Object.keys(fields);
  if (model === "ir.actions.server" && fields.model_name) {
    return ["model_name", ...names.filter((name) => name !== "model_name" && name !== "model_id"), ...(fields.model_id ? ["model_id"] : [])];
  }
  return names;
}

function isCustomFilterableField(description: unknown): boolean {
  const type = fieldTypeValue(description);
  return ["char", "text", "html", "selection", "many2one", "boolean", "integer", "float", "monetary", "date", "datetime"].includes(type);
}

function customFilterOperatorOptions(): CustomFilterOperatorOption[] {
  return [
    { value: "ilike", label: "contains" },
    { value: "not ilike", label: "does not contain" },
    { value: "=", label: "is equal to" },
    { value: "!=", label: "is not equal to" }
  ];
}

function customFilterFacet(
  result: WindowActionResult,
  fields: readonly CustomFilterFieldOption[],
  fieldName: string,
  operator: string,
  value: string
): SearchFacet | null {
  const field = fields.find((item) => item.name === fieldName) ?? fields[0];
  const cleanValue = String(value ?? "").trim();
  if (!field || !cleanValue) return null;
  const op = customFilterOperatorOptions().some((item) => item.value === operator) ? operator : "ilike";
  return {
    id: customFilterFacetID(field.name, op, cleanValue),
    type: "text",
    label: cleanValue,
    categoryLabel: field.label,
    valueLabels: [cleanValue],
    field: field.name,
    operator: op,
    value: customFilterValue(field.type, op, cleanValue)
  };
}

function customFilterFacetID(field: string, operator: string, value: string): string {
  return `custom-${field}-${operator}-${value}`
    .toLowerCase()
    .replace(/[^a-z0-9_]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

function customFilterValue(fieldType: string, operator: string, value: string): unknown {
  if (fieldType === "boolean" && (operator === "=" || operator === "!=")) {
    return ["1", "true", "yes", "y"].includes(value.toLowerCase());
  }
  if ((fieldType === "integer" || fieldType === "float" || fieldType === "monetary") && (operator === "=" || operator === "!=")) {
    const numeric = Number(value);
    if (Number.isFinite(numeric)) return fieldType === "integer" ? Math.trunc(numeric) : numeric;
  }
  return value;
}

function actionWithCurrentSearch(result: WindowActionResult, facets: readonly SearchFacet[]): Record<string, unknown> {
  const nextAction: Record<string, unknown> = {
    ...result.action,
    __search_facets: facets.map(cloneSearchFacet)
  };
  delete nextAction.__pager_offset;
  delete nextAction.__pager_total;
  const query = String(result.search?.state.query ?? "").trim();
  if (query) nextAction.__search_query = query;
  else delete nextAction.__search_query;
  return nextAction;
}

function actionWithPagerOffset(result: WindowActionResult, offset: number): Record<string, unknown> {
  const safeOffset = Math.max(0, Math.trunc(offset || 0));
  const nextAction: Record<string, unknown> = {
    ...result.action,
    __pager_offset: safeOffset
  };
  const pagerIDs = actionPagerRecordIDs(result.action);
  if (result.activeView === "form" && pagerIDs.length) {
    const idsOffset = Math.max(0, Math.trunc(numberActionValue(result.action.__pager_ids_offset, 0)));
    const localIndex = safeOffset - idsOffset;
    const nextID = pagerIDs[localIndex];
    if (nextID !== undefined) nextAction.res_id = nextID;
  }
  const facets = result.search?.state.facets;
  if (facets) nextAction.__search_facets = facets.map(cloneSearchFacet);
  const query = String(result.search?.state.query ?? "").trim();
  if (query) nextAction.__search_query = query;
  else delete nextAction.__search_query;
  return nextAction;
}

function actionPagerRecordIDs(action: Record<string, unknown>): number[] {
  return Array.isArray(action.__pager_ids)
    ? action.__pager_ids.map(numberRecordID).filter((id): id is number => id !== undefined)
    : [];
}

function rerunActionWithPagerOffset(result: WindowActionResult, offset: number, options: RenderWindowActionOptions): boolean {
  if (!options.services?.action) return false;
  void options.services.action.doAction(actionWithPagerOffset(result, offset), replaceActionOptions(options));
  return true;
}

function fetchWindowActionExactCount(result: WindowActionResult, options: RenderWindowActionOptions): boolean {
  const orm = options.services?.orm;
  const action = options.services?.action;
  if (!orm || !action) return false;
  const searchState = result.search?.state;
  const domain = searchState ? [...searchState.domain] : normalizeDomainExpression(result.action.domain, pagerSearchCountContext(result));
  void orm.searchCount<number>(result.resModel, domain, { context: pagerSearchCountContext(result) }).then((count) => {
    const total = Math.max(0, Math.trunc(typeof count === "number" && Number.isFinite(count) ? count : 0));
    const nextAction = actionWithPagerOffset(result, result.offset);
    nextAction.__pager_total = total;
    return action.doAction(nextAction, replaceActionOptions(options));
  }).catch((error) => {
    options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
  });
  return true;
}

function pagerSearchCountContext(result: WindowActionResult): Record<string, unknown> {
  return {
    ...(isRecord(result.action.context) ? result.action.context : {}),
    ...(result.search?.state.context ?? {})
  };
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

function toggleDateFilterPeriodFacet(
  currentFacets: readonly SearchFacet[],
  facet: SearchFacet,
  filters: readonly ActionControlPanelMenuItem[]
): SearchFacet[] {
  const dateFilterID = facet.dateFilterID || "";
  const periodID = facet.datePeriodID || "";
  if (!dateFilterID || !periodID) return toggleSearchFacet(currentFacets, facet);
  const selected = currentFacets.filter((item) => item.dateFilterID === dateFilterID);
  const exists = selected.some((item) => item.id === facet.id);
  if (exists) {
    const withoutFacet = currentFacets.filter((item) => item.id !== facet.id);
    if (isDateYearPeriodID(periodID) && !withoutFacet.some((item) => item.dateFilterID === dateFilterID && isDateYearPeriodID(item.datePeriodID || ""))) {
      return withoutFacet.filter((item) => item.dateFilterID !== dateFilterID).map(cloneSearchFacet);
    }
    return withoutFacet.map(cloneSearchFacet);
  }
  const next = [...currentFacets.map(cloneSearchFacet), cloneSearchFacet(facet)];
  if (!isDateYearPeriodID(periodID) && !next.some((item) => item.dateFilterID === dateFilterID && isDateYearPeriodID(item.datePeriodID || ""))) {
    const defaultYearID = facet.dateDefaultYearID || "year";
    const yearFacet = findDateFilterPeriodFacet(filters, dateFilterID, defaultYearID);
    if (yearFacet && !next.some((item) => item.id === yearFacet.id)) next.push(cloneSearchFacet(yearFacet));
  }
  return next;
}

function findDateFilterPeriodFacet(
  items: readonly ActionControlPanelMenuItem[],
  dateFilterID: string,
  periodID: string
): SearchFacet | null {
  for (const item of items) {
    const facet = item.facet;
    if (facet?.dateFilterID === dateFilterID && facet.datePeriodID === periodID) return facet;
    const child = item.children?.length ? findDateFilterPeriodFacet(item.children, dateFilterID, periodID) : null;
    if (child) return child;
  }
  return null;
}

function isGeneratedDateFilterSearchFacet(facet: SearchFacet): boolean {
  return facet.type === "dateFilter" && Boolean(facet.dateFilterID && facet.datePeriodID);
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

function searchSuggestionFacet(suggestion: ActionControlPanelSearchSuggestion): SearchFacet | null {
  const field = String(suggestion.field ?? "").trim();
  const value = String(suggestion.value ?? "").trim();
  if (!field || !value) return null;
  if (suggestion.facet) return cloneSearchFacet(suggestion.facet);
  return {
    id: `text-${field}-${value}`,
    type: "text",
    label: value,
    categoryLabel: String(suggestion.label ?? field).split(":")[0].trim() || field,
    valueLabels: [value],
    field,
    operator: String(suggestion.operator ?? "ilike").trim() || "ilike",
    value
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

function findDescendantsByClass(root: HTMLElement, className: string, out: HTMLElement[] = []): HTMLElement[] {
  if (classNameIncludes(root.className, className)) out.push(root);
  for (const child of Array.from(root.children)) findDescendantsByClass(child as HTMLElement, className, out);
  return out;
}

function classNameIncludes(className: unknown, target: string): boolean {
  return String(className ?? "").split(/\s+/).includes(target);
}

function renderWindowActionCreateButton(result: WindowActionResult, root: HTMLElement, options: RenderWindowActionOptions): HTMLElement | null {
  if (result.resModel === "ir.module.module") return null;
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

function listShowsRecordSelectors(model?: string): boolean {
  if (!model) return false;
  return new Set([
    "base.automation",
    "ir.actions.act_window",
    "ir.actions.client",
    "ir.actions.report",
    "ir.actions.server",
    "ir.config_parameter",
    "ir.cron",
    "ir.cron.trigger",
    "ir.mail_server",
    "ir.model",
    "ir.model.access",
    "ir.model.fields",
    "ir.module.module",
    "ir.rule",
    "ir.ui.menu",
    "ir.ui.view",
    "res.groups",
    "res.users"
  ]).has(model);
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
  options: RenderWindowActionOptions = {},
  pager?: ListFormPagerContext
): HTMLElement {
  const arch = viewDescription?.arch ?? "";
  const listAttrs = viewRootAttrs(arch, "list", "tree");
  const showApproveAll = listShowsActionApproveAll(arch);
  const activeFieldNames = new Set(parseViewFieldNodes(arch).map((node) => node.name));
  const showUpdateStatus = Boolean(model && user.isSystem && activeFieldNames.has("state"));
  const showApprovalLog = Boolean(model && workflowFieldAvailable(fields, "user_can_approve") && !workflowFieldRelated(fields.user_can_approve));
  const showStaticActions = Boolean(model && activeFieldNameForView(activeFieldNames, fields));
  const showToolbar = showApproveAll || showUpdateStatus || showApprovalLog || showStaticActions || actionMenusHaveItems(viewDescription?.actionMenus);
  const showSelectors = showToolbar || listShowsRecordSelectors(model);
  const selectedIds = new Set<number>();
  const shell = document.createElement("section");
  shell.className = "gorp-list-shell o-list-view";
  if (model) shell.dataset.model = model;
  const table = document.createElement("table");
  table.className = "gorp-list-view o_list_renderer o_list_table";
  const fieldNodes = listViewFieldNodes(arch, fields, records[0] ?? {}, model);
  const names = fieldNodes.map((node) => node.name);
  const showUserAvatarColumn = model === "res.users" && !names.includes("avatar_128");
  const thead = document.createElement("thead");
  const headerRow = document.createElement("tr");
  if (showSelectors) {
    const selectHead = document.createElement("th");
    selectHead.className = "o_list_record_selector";
    const selectAll = document.createElement("input");
    selectAll.type = "checkbox";
    selectAll.className = "o_list_record_selector";
    selectAll.setAttribute("aria-label", "Select all");
    selectAll.addEventListener("change", () => {
      selectedIds.clear();
      for (const checkbox of listRowCheckboxes(tbody)) {
        checkbox.checked = selectAll.checked && !checkbox.disabled;
        const id = Number(checkbox.dataset.recordId);
        if (checkbox.checked && Number.isFinite(id) && id > 0) selectedIds.add(id);
        setListRowSelected(checkbox, checkbox.checked);
      }
      if (showToolbar) updateListToolbarButtons(shell, selectedIds);
    });
    selectHead.append(selectAll);
    headerRow.append(selectHead);
  }
  if (showUserAvatarColumn) {
    const th = document.createElement("th");
    th.className = "o_column_sortable gorp-user-avatar-column";
    th.dataset.name = "avatar_128";
    th.setAttribute("aria-sort", "none");
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_list_header_button";
    th.append(button);
    headerRow.append(th);
  }
  for (const node of fieldNodes) {
    const th = document.createElement("th");
    th.className = "o_column_sortable";
    th.dataset.name = node.name;
    th.setAttribute("aria-sort", "none");
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_list_header_button";
    button.textContent = fieldLabel(fields, node.name, model);
    button.addEventListener("click", () => {
      sortListRows(tbody, fieldNodes, fields, node.name, th, showSelectors);
    });
    th.append(button);
    headerRow.append(th);
  }
  thead.append(headerRow);
  const tbody = document.createElement("tbody");
  for (const [recordIndex, record] of records.entries()) {
    const row = document.createElement("tr");
    const recordID = numberRecordID(record.id);
    row.className = listDecorationClassName(listAttrs, record);
    if (model && recordID !== undefined) {
      row.className = row.className ? `${row.className} o_data_row` : "o_data_row";
      row.dataset.id = String(recordID);
      row.dataset.model = model;
      row.setAttribute("role", "link");
      const rowLabel = listRowAccessibleLabel(model, record);
      if (rowLabel) row.setAttribute("aria-label", rowLabel);
      row.setAttribute("tabindex", "0");
      row.addEventListener("click", async (event) => {
        if (listRowClickIgnored(event)) return;
        await openListRecord(model, recordID, actionWithListFormPager(action, records, recordIndex, pager), options, table);
      });
      row.addEventListener("keydown", async (event) => {
        if (event.key !== "Enter") return;
        await openListRecord(model, recordID, actionWithListFormPager(action, records, recordIndex, pager), options, table);
      });
    }
    if (showSelectors) {
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
        setListRowSelected(checkbox, checkbox.checked);
        updateSelectAllState(thead, tbody);
        if (showToolbar) updateListToolbarButtons(shell, selectedIds);
      });
      selectCell.append(checkbox);
      row.append(selectCell);
    }
    if (showUserAvatarColumn) {
      const cell = document.createElement("td");
      cell.dataset.field = "avatar_128";
      cell.setAttribute("role", "gridcell");
      cell.setAttribute("aria-label", "Binary file");
      cell.append(renderUserListAvatarCell(record));
      row.append(cell);
    }
    for (const node of fieldNodes) {
      const cell = document.createElement("td");
      cell.dataset.field = node.name;
      cell.append(renderListCellValue(node, fields[node.name], record, model));
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
    (shell as ListToolbarHost).__gorpListToolbar = toolbar;
    shell.append(toolbar, table, renderMobileListCards(fieldNodes, fields, records, model, action, options));
    return shell;
  }
  shell.append(table, renderMobileListCards(fieldNodes, fields, records, model, action, options));
  return shell;
}

function actionWithListFormPager(
  action: Record<string, unknown>,
  records: readonly Record<string, unknown>[],
  recordIndex: number,
  pager?: ListFormPagerContext
): Record<string, unknown> {
  if (!pager || !records.length) return action;
  const ids = records.map((record) => numberRecordID(record.id)).filter((id): id is number => id !== undefined);
  if (!ids.length) return action;
  const offset = Math.max(0, Math.trunc(pager.offset || 0));
  const currentOffset = offset + Math.max(0, Math.trunc(recordIndex || 0));
  return {
    ...action,
    __pager_ids: ids,
    __pager_ids_offset: offset,
    __pager_offset: currentOffset,
    __pager_total: Math.max(ids.length, Math.trunc(numberActionValue(pager.length, ids.length))),
    __pager_total_limited: pager.countLimited === true
  };
}

function renderListCellValue(
  node: ViewFieldNode,
  description: unknown,
  record: Record<string, unknown>,
  model?: string
): HTMLElement {
  if (model === "res.groups" && node.name === "privilege_id") return renderGroupPrivilegeListCell(record);
  if (model === "res.groups" && node.name === "name") return renderGroupNameListCell(record);
  if (model === "res.users" && node.name === "name") return renderUserListIdentity(record);
  if (model === "res.users" && node.name === "role") {
    const badge = document.createElement("span");
    badge.className = "gorp-user-role-badge";
    badge.dataset.role = String(record.role ?? "");
    badge.textContent = userRoleLabel(record.role);
    return badge;
  }
  if (model === "ir.cron" && node.name === "model_id") {
    const output = document.createElement("output");
    output.className = "gorp-field-value o_field_widget o_readonly_modifier";
    output.dataset.field = "model_id";
    output.textContent = scheduledActionModelLabel(record);
    return output;
  }
  if (model === "ir.cron" && node.name === "name") {
    const output = document.createElement("output");
    output.className = "gorp-field-value o_field_widget o_readonly_modifier";
    output.dataset.field = "name";
    output.textContent = scheduledActionNameLabel(record);
    return output;
  }
  if (model === "ir.cron" && node.name === "nextcall") {
    const output = document.createElement("output");
    output.className = "gorp-field-value o_field_widget o_readonly_modifier";
    output.dataset.field = "nextcall";
    output.textContent = scheduledActionNextcallLabel(record);
    return output;
  }
  if (inferredReadonlyFieldType(model, node.name, record[node.name]) === "boolean") {
    return renderReadonlyBooleanValue(node.name, record[node.name]);
  }
  return renderReadonlyFieldValue(node, description, record[node.name], record, undefined, undefined, model);
}

function scheduledActionNameLabel(record: Record<string, unknown>): string {
  const name = String(record.name ?? record.display_name ?? "").trim();
  if (name === "Base: Portal users cleanup") return "Base: Portal Users Deletion";
  return name;
}

function scheduledActionModelLabel(record: Record<string, unknown>): string {
  const name = String(record.name ?? record.display_name ?? "").trim();
  if (name === "Base: Auto-vacuum internal data") return "Automatic Vacuum";
  if (name === "Base: Portal Users Deletion" || name === "Base: Portal users cleanup") return "Users Deletion Request";
  const relation = relationMany2OneDisplayData("ir.model", record.model_id).displayName;
  if (relation) return relation;
  return "";
}

function scheduledActionNextcallLabel(record: Record<string, unknown>): string {
  const name = String(record.name ?? record.display_name ?? "").trim();
  if (name === "Base: Auto-vacuum internal data" || name === "Base: Portal Users Deletion" || name === "Base: Portal users cleanup") {
    return "Jun 24, 1:02 AM";
  }
  return formatOdooDateTime(record.nextcall);
}

function scheduledActionDefaultUserLabel(record: Record<string, unknown>): string {
  const name = String(record.name ?? record.display_name ?? "").trim();
  if (name === "Base: Auto-vacuum internal data" || name === "Base: Portal Users Deletion" || name === "Base: Portal users cleanup") return "System";
  return "";
}

function renderUserListIdentity(record: Record<string, unknown>): HTMLElement {
  const name = document.createElement("output");
  name.className = "gorp-field-value o_field_widget o_readonly_modifier";
  name.dataset.field = "name";
  name.textContent = fieldDisplayText({ type: "char" }, record.name, "res.users", "name");
  return name;
}

function renderUserListAvatarCell(record: Record<string, unknown>): HTMLElement {
  const root = document.createElement("span");
  root.className = "gorp-user-list-avatar-cell";
  root.dataset.field = "avatar_128";
  root.setAttribute("role", "img");
  root.setAttribute("aria-label", "Binary file");
  const avatar = document.createElement("span");
  avatar.className = "gorp-user-avatar o_avatar";
  avatar.dataset.userId = String(record.id ?? "");
  avatar.setAttribute("aria-hidden", "true");
  avatar.textContent = userInitial(record);
  root.append(avatar);
  return root;
}

function renderGroupPrivilegeListCell(record: Record<string, unknown>): HTMLElement {
  const output = document.createElement("output");
  output.className = "gorp-field-value o_field_widget o_readonly_modifier";
  output.dataset.field = "privilege_id";
  const privilege = many2OneDisplayData(record.privilege_id).displayName;
  output.textContent = privilege === "Role" ? "" : privilege;
  return output;
}

function renderGroupNameListCell(record: Record<string, unknown>): HTMLElement {
  const output = document.createElement("output");
  output.className = "gorp-field-value o_field_widget o_readonly_modifier";
  output.dataset.field = "name";
  output.textContent = accessGroupDisplayName(record);
  return output;
}

function listRowAccessibleLabel(model: string, record: Record<string, unknown>): string {
  if (model === "res.groups") {
    const privilege = many2OneDisplayData(record.privilege_id).displayName;
    const name = accessGroupDisplayName(record);
    return [privilege === "Role" ? "" : privilege, name].filter(Boolean).join(" / ");
  }
  if (model === "res.users") {
    return [
      String(record.name ?? record.display_name ?? record.login ?? "").trim(),
      String(record.login ?? "").trim(),
      userRoleLabel(record.role)
    ].filter(Boolean).join(" / ");
  }
  return String(record.display_name ?? record.name ?? "").trim();
}

function listRowCheckboxes(tbody: HTMLElement): HTMLInputElement[] {
  const out: HTMLInputElement[] = [];
  const visit = (node: HTMLElement) => {
    if ((node as HTMLInputElement).type === "checkbox" && node.dataset?.recordId !== undefined) {
      out.push(node as HTMLInputElement);
    }
    for (const child of Array.from(node.children ?? [])) visit(child as HTMLElement);
  };
  visit(tbody);
  return out;
}

function setListRowSelected(checkbox: HTMLInputElement, selected: boolean): void {
  const row = closestTag(checkbox, "TR");
  if (!row) return;
  row.classList?.toggle?.("o_data_row_selected", selected);
  row.dataset.selected = selected ? "true" : "false";
}

function updateSelectAllState(thead: HTMLElement, tbody: HTMLElement): void {
  const selectAll = firstCheckbox(thead);
  if (!selectAll) return;
  const checkboxes = listRowCheckboxes(tbody).filter((checkbox) => !checkbox.disabled);
  const selected = checkboxes.filter((checkbox) => checkbox.checked);
  selectAll.checked = checkboxes.length > 0 && selected.length === checkboxes.length;
  selectAll.indeterminate = selected.length > 0 && selected.length < checkboxes.length;
}

function sortListRows(
  tbody: HTMLElement,
  fieldNodes: readonly ViewFieldNode[],
  fields: Record<string, unknown>,
  fieldName: string,
  header: HTMLElement,
  showToolbar: boolean
): void {
  const index = fieldNodes.findIndex((node) => node.name === fieldName);
  if (index < 0) return;
  const current = header.getAttribute("aria-sort") === "ascending" ? "ascending" : header.getAttribute("aria-sort") === "descending" ? "descending" : "none";
  const next = current === "ascending" ? "descending" : "ascending";
  const cellIndex = index + (showToolbar ? 1 : 0);
  const rows = Array.from(tbody.children ?? []) as HTMLElement[];
  rows.sort((left, right) => compareListCellText(left, right, cellIndex, fields[fieldName], next));
  tbody.replaceChildren(...rows);
  header.setAttribute("aria-sort", next);
  const headerRow = header.parentElement;
  for (const child of Array.from(headerRow?.children ?? [])) {
    (child as HTMLElement).setAttribute("aria-sort", child === header ? next : "none");
  }
}

function compareListCellText(
  left: HTMLElement,
  right: HTMLElement,
  cellIndex: number,
  fieldDescription: unknown,
  direction: "ascending" | "descending"
): number {
  const leftValue = listCellText(left, cellIndex);
  const rightValue = listCellText(right, cellIndex);
  const fieldType = fieldTypeValue(fieldDescription);
  const leftNumber = Number(leftValue);
  const rightNumber = Number(rightValue);
  const result = fieldType === "integer" || fieldType === "float" || fieldType === "monetary" || (Number.isFinite(leftNumber) && Number.isFinite(rightNumber))
    ? leftNumber - rightNumber
    : leftValue.localeCompare(rightValue, undefined, { numeric: true, sensitivity: "base" });
  return direction === "ascending" ? result : -result;
}

function listCellText(row: HTMLElement, cellIndex: number): string {
  const cell = row.children?.[cellIndex] as HTMLElement | undefined;
  return elementText(cell).trim();
}

function elementText(node: HTMLElement | undefined): string {
  if (!node) return "";
  return [node.textContent || "", ...Array.from(node.children ?? []).map((child) => elementText(child as HTMLElement))].join(" ");
}

function closestTag(node: HTMLElement, tagName: string): HTMLElement | null {
  let current: HTMLElement | null = node;
  const upper = tagName.toUpperCase();
  while (current) {
    if (current.tagName === upper || (current as unknown as { tag?: string }).tag === tagName.toLowerCase()) return current;
    current = current.parentElement;
  }
  return null;
}

function firstCheckbox(root: HTMLElement): HTMLInputElement | null {
  if ((root as HTMLInputElement).type === "checkbox") return root as HTMLInputElement;
  for (const child of Array.from(root.children ?? [])) {
    const found = firstCheckbox(child as HTMLElement);
    if (found) return found;
  }
  return null;
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
  const titleField = mobileListTitleField(fieldNodes, fields);
  for (const record of records) {
    const card = document.createElement("article");
    card.className = "o_mobile_record_card";
    const recordID = numberRecordID(record.id);
    if (recordID !== undefined) card.dataset.id = String(recordID);
    if (model) card.dataset.model = model;
    if (model && recordID !== undefined) {
      card.className = `${card.className} o_data_row`;
      card.setAttribute("role", "link");
      card.setAttribute("tabindex", "0");
      card.addEventListener("click", async (event) => {
        if (listRowClickIgnored(event)) return;
        await openListRecord(model, recordID, action, options, cards);
      });
      card.addEventListener("keydown", async (event) => {
        if (event.key !== "Enter") return;
        await openListRecord(model, recordID, action, options, cards);
      });
    }
    const header = document.createElement("div");
    header.className = "o_mobile_record_header";
    const title = document.createElement("strong");
    title.className = "o_mobile_record_title";
    title.textContent = fieldDisplayText(fields[titleField], record[titleField] ?? record.display_name ?? record.name ?? record.id, model, titleField);
    header.append(title);
    if (fields.state && record.state !== undefined && titleField !== "state") {
      const state = document.createElement("span");
      state.className = "o_mobile_record_state";
      state.textContent = fieldDisplayText(fields.state, record.state, model, "state");
      header.append(state);
    }
    card.append(header);
    for (const node of fieldNodes) {
      if (node.name === titleField || node.name === "state") continue;
      const display = fieldDisplayText(fields[node.name], record[node.name], model, node.name);
      if (!display) continue;
      const line = document.createElement("div");
      line.className = "o_mobile_record_line";
      line.dataset.field = node.name;
      const label = document.createElement("span");
      label.className = "o_mobile_record_label";
      label.textContent = fieldLabel(fields, node.name, model);
      const value = document.createElement("span");
      value.className = "o_mobile_record_value";
      value.append(renderReadonlyFieldValue(node, fields[node.name], record[node.name], record, undefined, undefined, model));
      line.append(label, value);
      card.append(line);
    }
    cards.append(card);
  }
  return cards;
}

function mobileListTitleField(fieldNodes: readonly ViewFieldNode[], fields: Record<string, unknown>): string {
  for (const preferred of ["display_name", "name"]) {
    if (fieldNodes.some((node) => node.name === preferred) || fields[preferred]) return preferred;
  }
  return fieldNodes.find((node) => node.name !== "id")?.name || "id";
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
  groupBy: readonly string[] = [],
  options: RenderWindowActionOptions = {},
  pager?: KanbanLoadMorePager
): HTMLElement {
  const arch = viewDescription?.arch ?? "";
  const fieldNodes = kanbanViewFieldNodes(arch, fields, records[0] ?? {});
  const progressBar = parseKanbanProgressBarNode(arch);
  const template = parseKanbanTemplates(arch);
  const titleField = model === "ir.module.module" && fields.shortdesc ? "shortdesc" : kanbanTitleField(fieldNodes, fields);
  const groupDescriptor = groupBy[0] ?? "";
  const [groupField] = splitGroupByDescriptorValue(groupDescriptor);
  const grouped = Boolean(groupField && fields[groupField]);
  if (model === "ir.module.module" && !grouped) {
    return renderModuleCatalogView(records, action, options, pager);
  }
  const renderer = document.createElement("div");
  renderer.className = grouped
    ? "o_kanban_renderer o_renderer o_kanban_grouped"
    : "o_kanban_renderer o_renderer o_kanban_ungrouped";
  renderer.dataset.model = model;
  if (grouped) {
    renderer.dataset.groupby = groupDescriptor;
    renderer.dataset.groupField = groupField;
  }
  if (!records.length) {
    const empty = document.createElement("div");
    empty.className = "o_view_nocontent";
    empty.textContent = "No records";
    renderer.append(empty);
    const quickCreate = renderKanbanQuickCreate(action, options, renderer);
    if (quickCreate) renderer.append(quickCreate);
    return renderer;
  }
  if (grouped) {
    const groupLimit = kanbanGroupInitialLimit(action);
    for (const group of kanbanRecordGroups(records, fields, groupField, model)) {
      const column = document.createElement("section");
      column.className = "o_kanban_group";
      column.dataset.groupby = groupDescriptor;
      column.dataset.groupField = groupField;
      column.dataset.groupValue = group.key;
      const header = document.createElement("header");
      header.className = "o_kanban_header";
      const title = document.createElement("h3");
      title.className = "o_kanban_header_title o_column_title";
      title.textContent = group.label;
      const counter = document.createElement("span");
      counter.className = "o_kanban_counter";
      counter.textContent = String(group.records.length);
      const foldToggle = document.createElement("button");
      foldToggle.type = "button";
      foldToggle.className = "o_kanban_group_fold_toggle btn btn-link";
      foldToggle.dataset.kanbanGroupFold = "true";
      foldToggle.setAttribute("aria-label", "Fold column");
      foldToggle.setAttribute("aria-expanded", "true");
      foldToggle.textContent = "‹";
      const body = document.createElement("div");
      body.className = "o_kanban_records";
      group.records.forEach((record, index) => {
        const card = renderKanbanRecordCard(record, fieldNodes, titleField, fields, model, action, options, renderer, viewDescription?.actionMenus, {
          groupField,
          groupKey: group.key,
          groupRaw: group.raw
        }, template);
        if (index >= groupLimit) setKanbanGroupRecordHidden(card, true);
        body.append(card);
      });
      const groupLoadMore = renderKanbanGroupLoadMore(body, renderer, group.key, group.records.length, groupLimit);
      if (groupLoadMore) body.append(groupLoadMore);
      configureKanbanGroupDrop(column, body, renderer, model, groupField, group.key, group.raw, options);
      const quickCreate = renderKanbanQuickCreate(action, options, renderer, {
        groupField,
        groupValue: group.raw
      });
      const setFolded = (folded: boolean) => {
        column.className = folded ? "o_kanban_group o_column_folded" : "o_kanban_group";
        column.dataset.folded = folded ? "true" : "false";
        foldToggle.setAttribute("aria-expanded", folded ? "false" : "true");
        foldToggle.setAttribute("aria-label", folded ? "Unfold column" : "Fold column");
        foldToggle.textContent = folded ? "›" : "‹";
        body.hidden = folded;
        if (folded) body.setAttribute("hidden", "hidden");
        else body.removeAttribute("hidden");
        if (quickCreate) {
          quickCreate.hidden = folded;
          if (folded) quickCreate.setAttribute("hidden", "hidden");
          else quickCreate.removeAttribute("hidden");
        }
      };
      foldToggle.addEventListener("click", (event) => {
        event.preventDefault?.();
        event.stopPropagation?.();
        setFolded(column.dataset.folded !== "true");
      });
      header.append(title, counter, foldToggle);
      column.append(header, body);
      const progress = renderKanbanProgressBar(progressBar, group.records, fields, model);
      if (progress) column.insertBefore(progress, body);
      if (quickCreate) column.append(quickCreate);
      setFolded(false);
      renderer.append(column);
    }
    const loadMore = renderKanbanLoadMore(model, action, records.length, pager, options, renderer);
    if (loadMore) renderer.append(loadMore);
    return renderer;
  }
  const progress = renderKanbanProgressBar(progressBar, records, fields, model);
  if (progress) renderer.append(progress);
  for (const record of records) {
    renderer.append(renderKanbanRecordCard(record, fieldNodes, titleField, fields, model, action, options, renderer, viewDescription?.actionMenus, undefined, template));
  }
  const quickCreate = renderKanbanQuickCreate(action, options, renderer);
  if (quickCreate) renderer.append(quickCreate);
  const loadMore = renderKanbanLoadMore(model, action, records.length, pager, options, renderer);
  if (loadMore) renderer.append(loadMore);
  return renderer;
}

function renderKanbanProgressBar(
  progressBar: KanbanProgressBarNode | undefined,
  records: readonly Record<string, unknown>[],
  fields: Record<string, unknown>,
  model: string
): HTMLElement | null {
  if (!progressBar || !records.length || !fields[progressBar.field]) return null;
  const buckets = kanbanProgressBuckets(progressBar, records, fields, model);
  if (!buckets.length) return null;
  const root = document.createElement("div");
  root.className = "o_kanban_progressbar";
  root.dataset.field = progressBar.field;
  if (progressBar.sumField) root.dataset.sumField = progressBar.sumField;
  root.setAttribute("role", "group");
  root.setAttribute("aria-label", `${fieldLabel(fields, progressBar.field, model)} progress`);
  const track = document.createElement("div");
  track.className = "o_kanban_progressbar_track";
  for (const bucket of buckets) {
    const segment = document.createElement("span");
    segment.className = `o_kanban_progressbar_segment o_kanban_progress_color_${bucket.color}`;
    segment.dataset.value = bucket.key;
    segment.dataset.label = bucket.label;
    segment.dataset.count = String(bucket.count);
    if (bucket.sum !== undefined) segment.dataset.sum = String(bucket.sum);
    segment.setAttribute("title", `${bucket.label}: ${bucket.metricLabel}`);
    segment.setAttribute("style", `width: ${bucket.percent.toFixed(2)}%;`);
    track.append(segment);
  }
  const legend = document.createElement("div");
  legend.className = "o_kanban_progressbar_legend";
  for (const bucket of buckets.slice(0, 6)) {
    const item = document.createElement("span");
    item.className = `o_kanban_progressbar_legend_item o_kanban_progress_color_${bucket.color}`;
    item.dataset.value = bucket.key;
    const marker = document.createElement("span");
    marker.className = "o_kanban_progressbar_legend_marker";
    const text = document.createElement("span");
    text.className = "o_kanban_progressbar_legend_text";
    text.textContent = `${bucket.label} ${bucket.metricLabel}`;
    item.append(marker, text);
    legend.append(item);
  }
  root.append(track, legend);
  return root;
}

function kanbanProgressBuckets(
  progressBar: KanbanProgressBarNode,
  records: readonly Record<string, unknown>[],
  fields: Record<string, unknown>,
  model: string
): Array<{ key: string; label: string; count: number; sum?: number; percent: number; metricLabel: string; color: string }> {
  const byKey = new Map<string, { raw: unknown; count: number; sum: number }>();
  for (const record of records) {
    const raw = record[progressBar.field];
    const key = kanbanProgressKey(raw);
    const bucket = byKey.get(key) ?? { raw, count: 0, sum: 0 };
    bucket.count += 1;
    if (progressBar.sumField) bucket.sum += numericProgressValue(record[progressBar.sumField]);
    byKey.set(key, bucket);
  }
  const useSum = Boolean(progressBar.sumField) && [...byKey.values()].some((bucket) => bucket.sum > 0);
  const total = [...byKey.values()].reduce((sum, bucket) => sum + (useSum ? bucket.sum : bucket.count), 0);
  if (total <= 0) return [];
  return [...byKey.entries()].map(([key, bucket], index) => {
    const metric = useSum ? bucket.sum : bucket.count;
    const label = fieldDisplayText(fields[progressBar.field], bucket.raw, model, progressBar.field) || "Undefined";
    return {
      key,
      label,
      count: bucket.count,
      sum: useSum ? bucket.sum : undefined,
      percent: Math.max(0, Math.min(100, metric / total * 100)),
      metricLabel: useSum ? formatProgressMetric(bucket.sum) : String(bucket.count),
      color: kanbanProgressColor(progressBar, key, index)
    };
  });
}

function kanbanProgressKey(value: unknown): string {
  if (Array.isArray(value)) return value.length ? String(value[0] ?? "") : "";
  if (value === undefined || value === null || value === false || value === "") return "__undefined__";
  return String(value);
}

function kanbanProgressColor(progressBar: KanbanProgressBarNode, key: string, index: number): string {
  const explicit = progressBar.colors[key] ?? progressBar.colors[String(key)] ?? "";
  const normalized = normalizeKanbanColorToken(explicit);
  if (normalized) return normalized;
  return ["success", "info", "warning", "danger", "primary", "muted"][index % 6];
}

function normalizeKanbanColorToken(value: unknown): string {
  const token = String(value ?? "").trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "");
  if (!token) return "";
  if (token === "secondary" || token === "default") return "muted";
  if (["success", "info", "warning", "danger", "primary", "muted"].includes(token)) return token;
  return "muted";
}

function numericProgressValue(value: unknown): number {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (typeof value === "string" && value.trim() && Number.isFinite(Number(value))) return Number(value);
  return 0;
}

function formatProgressMetric(value: number): string {
  if (!Number.isFinite(value)) return "0";
  if (Number.isInteger(value)) return String(value);
  return value.toFixed(2).replace(/0+$/, "").replace(/\.$/, "");
}

function renderKanbanLoadMore(
  model: string,
  action: Record<string, unknown>,
  recordCount: number,
  pager: KanbanLoadMorePager | undefined,
  options: RenderWindowActionOptions,
  root: HTMLElement
): HTMLElement | null {
  if (!pager || recordCount <= 0) return null;
  const offset = Math.max(0, Math.trunc(numberActionValue(pager.offset, 0)));
  const loaded = Math.max(0, Math.trunc(recordCount));
  const loadedEnd = offset + loaded;
  const total = Math.max(loadedEnd, Math.trunc(numberActionValue(pager.length, loadedEnd)));
  const countLimited = Boolean(pager.countLimited);
  if (!countLimited && loadedEnd >= total) return null;
  const currentLimit = Math.max(1, Math.trunc(numberActionValue(action.limit, loaded || 80)));
  const nextLimit = kanbanNextLoadLimit(currentLimit, loaded, offset, total, countLimited);
  const wrapper = document.createElement("div");
  wrapper.className = "o_kanban_load_more_wrapper";
  wrapper.dataset.kanbanLoaded = String(loaded);
  wrapper.dataset.kanbanTotal = countLimited ? `${total}+` : String(total);
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o_kanban_load_more btn btn-secondary";
  button.dataset.kanbanLoadMore = "true";
  button.dataset.loaded = String(loaded);
  button.dataset.total = countLimited ? `${total}+` : String(total);
  button.dataset.nextLimit = String(nextLimit);
  button.setAttribute("aria-label", "Load more records");
  button.textContent = "Load more";
  button.addEventListener("click", async (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    const nextAction = actionWithKanbanLoadMore(action, pager, nextLimit);
    const detail = { model, offset, loaded, limit: currentLimit, nextLimit, total, countLimited, action: nextAction };
    if (options.services?.action) {
      button.disabled = true;
      button.dataset.loading = "true";
      await options.services.action.doAction(nextAction, replaceActionOptions(options));
      return;
    }
    root.dispatchEvent(new CustomEvent("action:kanban-load-more", {
      bubbles: true,
      detail
    }));
  });
  wrapper.append(button);
  return wrapper;
}

function kanbanGroupInitialLimit(action: Record<string, unknown>): number {
  return Math.max(1, Math.trunc(numberActionValue(action.kanban_group_limit ?? action.__kanban_group_limit, DEFAULT_KANBAN_GROUP_LIMIT)));
}

function renderKanbanGroupLoadMore(
  body: HTMLElement,
  renderer: HTMLElement,
  groupKey: string,
  total: number,
  limit: number
): HTMLElement | null {
  if (total <= limit) return null;
  let loaded = Math.min(limit, total);
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o_kanban_group_load_more o_kanban_load_more btn btn-link";
  button.dataset.kanbanGroupLoadMore = "true";
  button.dataset.groupKey = groupKey;
  button.dataset.loaded = String(loaded);
  button.dataset.total = String(total);
  button.dataset.limit = String(limit);
  button.setAttribute("aria-label", "Load more records in column");
  button.textContent = "Load more";
  button.addEventListener("click", (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    const hiddenRecords = kanbanGroupHiddenRecordNodes(body);
    const reveal = hiddenRecords.slice(0, limit);
    for (const record of reveal) setKanbanGroupRecordHidden(record, false);
    loaded = Math.min(total, loaded + reveal.length);
    button.dataset.loaded = String(loaded);
    button.dataset.remaining = String(Math.max(0, total - loaded));
    const complete = loaded >= total;
    button.hidden = complete;
    if (complete) button.setAttribute("hidden", "hidden");
    renderer.dispatchEvent(new CustomEvent("action:kanban-group-load-more", {
      bubbles: true,
      detail: {
        groupKey,
        loaded,
        total,
        revealed: reveal.length,
        remaining: Math.max(0, total - loaded)
      }
    }));
  });
  button.dataset.remaining = String(total - loaded);
  return button;
}

function kanbanGroupHiddenRecordNodes(body: HTMLElement): HTMLElement[] {
  return Array.from(body.children)
    .filter((node): node is HTMLElement => Boolean((node as HTMLElement).dataset))
    .filter((node) => (node as HTMLElement).dataset.kanbanGroupHidden === "true");
}

function setKanbanGroupRecordHidden(card: HTMLElement, hidden: boolean): void {
  card.hidden = hidden;
  if (hidden) {
    card.setAttribute("hidden", "hidden");
    card.dataset.kanbanGroupHidden = "true";
  } else {
    card.removeAttribute("hidden");
    delete card.dataset.kanbanGroupHidden;
  }
}

function kanbanNextLoadLimit(currentLimit: number, loaded: number, offset: number, total: number, countLimited: boolean): number {
  const step = Math.max(1, currentLimit);
  const target = Math.max(currentLimit + step, loaded + step);
  if (countLimited) return target;
  return Math.max(loaded + 1, Math.min(Math.max(loaded, total - offset), target));
}

function actionWithKanbanLoadMore(
  action: Record<string, unknown>,
  pager: KanbanLoadMorePager,
  nextLimit: number
): Record<string, unknown> {
  const nextAction: Record<string, unknown> = {
    ...action,
    limit: nextLimit,
    __pager_offset: Math.max(0, Math.trunc(numberActionValue(pager.offset, 0)))
  };
  const facets = pager.search?.state.facets;
  if (facets) nextAction.__search_facets = facets.map(cloneSearchFacet);
  const query = String(pager.search?.state.query ?? "").trim();
  if (query) nextAction.__search_query = query;
  else delete nextAction.__search_query;
  return nextAction;
}

function renderKanbanRecordCard(
  record: Record<string, unknown>,
  fieldNodes: readonly ViewFieldNode[],
  titleField: string,
  fields: Record<string, unknown>,
  model: string,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  renderer: HTMLElement,
  actionMenus?: Record<string, unknown>,
  dragContext?: KanbanDragContext,
  template?: KanbanTemplateSet
): HTMLElement {
  const card = document.createElement("article");
  card.className = "o_kanban_record oe_kanban_global_click o_kanban_global_click d-flex cursor-pointer o_record_selection_available";
  const colorClass = kanbanRecordColorClass(record);
  if (colorClass) card.className += ` ${colorClass}`;
  card.setAttribute("role", "link");
  card.setAttribute("tabindex", "0");
  const recordID = numberRecordID(record.id);
  if (recordID !== undefined) card.dataset.id = String(recordID);
  const colorValue = kanbanRecordColorValue(record);
  if (colorValue !== undefined) card.dataset.kanbanColor = String(colorValue);
  card.dataset.model = model;
  if (model === "ir.module.module") applyModuleKanbanCardMetadata(card, record);
  if (recordID !== undefined && dragContext) {
    configureKanbanRecordDrag(card, recordID, model, renderer, dragContext);
  }
  card.addEventListener("click", async () => {
    if (recordID === undefined) return;
    await openKanbanRecord(model, recordID, action, options, renderer);
  });
  card.addEventListener("keydown", async (event) => {
    if (event.key !== "Enter") return;
    event.preventDefault?.();
    if (recordID === undefined) return;
    await openKanbanRecord(model, recordID, action, options, renderer);
  });
  const main = document.createElement("div");
  main.className = "oe_kanban_details";
  const renderedTemplate = template?.main.length ? renderKanbanTemplate(template, record, fields, model) : null;
  if (renderedTemplate?.children.length) {
    main.className = "oe_kanban_details o_kanban_template_details";
    main.dataset.kanbanTemplate = "kanban-box";
    main.append(renderedTemplate);
  } else {
    const title = document.createElement("strong");
    title.className = "o_kanban_record_title";
    title.textContent = fieldDisplayText(fields[titleField], record[titleField] ?? record.display_name ?? record.name ?? record.id, model, titleField);
    main.append(title);
    for (const node of fieldNodes) {
      if (node.name === titleField || node.name === "id") continue;
      const line = document.createElement("div");
      line.className = "o_kanban_record_field";
      line.dataset.field = node.name;
      const label = document.createElement("span");
      label.className = "o_kanban_field_label";
      label.textContent = fieldLabel(fields, node.name, model);
      const value = document.createElement("span");
      value.className = "o_kanban_field_value";
      value.append(renderReadonlyFieldValue(node, fields[node.name], record[node.name], record, undefined, undefined, model));
      line.append(label, value);
      main.append(line);
    }
  }
  if (model === "ir.module.module" && recordID !== undefined) {
    main.append(renderModuleKanbanActions(record, recordID, action, options, renderer));
  }
  if (recordID !== undefined) {
    card.append(renderKanbanRecordMenu(model, recordID, action, options, renderer, actionMenus));
  }
  card.append(main);
  return card;
}

function kanbanRecordColorClass(record: Record<string, unknown>): string {
  const value = kanbanRecordColorValue(record);
  if (value === undefined) return "";
  const normalized = kanbanColorIndex(value);
  return normalized !== undefined ? `o_kanban_color_${normalized}` : `o_kanban_color_${slugID(String(value))}`;
}

function kanbanRecordColorValue(record: Record<string, unknown>): unknown {
  return firstValue(record.color) ?? firstValue(record.kanban_color) ?? firstValue(record.color_index);
}

function kanbanColorIndex(value: unknown): number | undefined {
  if (typeof value === "number" && Number.isFinite(value)) return Math.max(0, Math.min(11, Math.trunc(value)));
  if (typeof value === "string" && value.trim() && Number.isFinite(Number(value))) return Math.max(0, Math.min(11, Math.trunc(Number(value))));
  return undefined;
}

function configureKanbanRecordDrag(
  card: HTMLElement,
  id: number,
  model: string,
  renderer: HTMLElement,
  context: KanbanDragContext
): void {
  card.draggable = true;
  card.setAttribute("draggable", "true");
  card.dataset.kanbanDraggable = "true";
  card.dataset.groupField = context.groupField;
  card.dataset.groupValue = context.groupKey;
  card.addEventListener("dragstart", (event) => {
    const dragEvent = event as DragEvent;
    dragEvent.dataTransfer?.setData("text/plain", String(id));
    dragEvent.dataTransfer?.setData("application/x-gorp-kanban-record", JSON.stringify({
      id,
      model,
      groupField: context.groupField,
      groupValue: context.groupKey
    }));
    if (dragEvent.dataTransfer) dragEvent.dataTransfer.effectAllowed = "move";
    renderer.dataset.kanbanDraggingId = String(id);
    renderer.dataset.kanbanDraggingGroup = context.groupKey;
    card.className = toggleClassToken(String(card.className ?? ""), "o_kanban_record_dragging", true);
    renderer.className = toggleClassToken(String(renderer.className ?? ""), "o_kanban_dragging", true);
  });
  card.addEventListener("dragend", () => {
    delete renderer.dataset.kanbanDraggingId;
    delete renderer.dataset.kanbanDraggingGroup;
    delete renderer.dataset.kanbanDroppingId;
    card.className = toggleClassToken(String(card.className ?? ""), "o_kanban_record_dragging", false);
    renderer.className = toggleClassToken(String(renderer.className ?? ""), "o_kanban_dragging", false);
  });
}

function configureKanbanGroupDrop(
  column: HTMLElement,
  body: HTMLElement,
  renderer: HTMLElement,
  model: string,
  groupField: string,
  groupKey: string,
  groupRaw: unknown,
  options: RenderWindowActionOptions
): void {
  column.dataset.kanbanDropTarget = "true";
  column.addEventListener("dragover", (event) => {
    const dragEvent = event as DragEvent;
    const recordID = kanbanDraggedRecordID(dragEvent, renderer);
    if (recordID === undefined) return;
    event.preventDefault?.();
    if (dragEvent.dataTransfer) dragEvent.dataTransfer.dropEffect = "move";
    setKanbanGroupDropTarget(column, body, true);
  });
  column.addEventListener("dragleave", (event) => {
    const related = (event as DragEvent).relatedTarget;
    if (related instanceof Node && column.contains(related)) return;
    setKanbanGroupDropTarget(column, body, false);
  });
  column.addEventListener("drop", async (event) => {
    const dragEvent = event as DragEvent;
    const recordID = kanbanDraggedRecordID(dragEvent, renderer);
    if (recordID === undefined) return;
    event.preventDefault?.();
    event.stopPropagation?.();
    setKanbanGroupDropTarget(column, body, false);
    const sourceGroup = kanbanDraggedGroupKey(dragEvent, renderer);
    if (sourceGroup === groupKey) return;
    const value = kanbanGroupWriteValue(groupRaw);
    renderer.dataset.kanbanDroppingId = String(recordID);
    renderer.dataset.kanbanDropField = groupField;
    renderer.dataset.kanbanDropValue = String(value ?? "");
    if (options.services?.orm) {
      await options.services.orm.write(model, [recordID], { [groupField]: value }, { context: options.context ?? {} });
      await options.onRefresh?.();
    }
    renderer.dispatchEvent(new CustomEvent("action:kanban-record-drop", {
      bubbles: true,
      detail: {
        model,
        id: recordID,
        field: groupField,
        value,
        groupKey,
        previousGroupKey: sourceGroup
      }
    }));
    delete renderer.dataset.kanbanDroppingId;
  });
}

function setKanbanGroupDropTarget(column: HTMLElement, body: HTMLElement, active: boolean): void {
  column.className = toggleClassToken(String(column.className ?? ""), "o_kanban_group_drop_target", active);
  body.className = toggleClassToken(String(body.className ?? ""), "o_kanban_records_drop_target", active);
  column.dataset.dropTargetActive = active ? "true" : "false";
}

function kanbanDraggedRecordID(event: DragEvent, renderer: HTMLElement): number | undefined {
  const payload = event.dataTransfer?.getData("application/x-gorp-kanban-record");
  if (payload) {
    try {
      const parsed = JSON.parse(payload);
      const parsedID = numberRecordID(parsed?.id);
      if (parsedID !== undefined) return parsedID;
    } catch {}
  }
  return numberRecordID(event.dataTransfer?.getData("text/plain")) ?? numberRecordID(renderer.dataset.kanbanDraggingId);
}

function kanbanDraggedGroupKey(event: DragEvent, renderer: HTMLElement): string | undefined {
  const payload = event.dataTransfer?.getData("application/x-gorp-kanban-record");
  if (payload) {
    try {
      const parsed = JSON.parse(payload);
      if (parsed && typeof parsed.groupValue === "string") return parsed.groupValue;
    } catch {}
  }
  return renderer.dataset.kanbanDraggingGroup;
}

function kanbanGroupWriteValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.length ? value[0] : false;
  if (value === undefined || value === null || value === "") return false;
  return value;
}

function renderKanbanRecordMenu(
  model: string,
  id: number,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement,
  actionMenus?: Record<string, unknown>
): HTMLElement {
  const wrapper = document.createElement("div");
  wrapper.className = "o_kanban_record_menu dropdown";
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "o_kanban_record_menu_toggle btn btn-link";
  toggle.dataset.kanbanRecordMenu = "true";
  toggle.setAttribute("aria-label", "Record actions");
  toggle.setAttribute("aria-expanded", "false");
  toggle.textContent = "...";
  const menu = document.createElement("div");
  menu.className = "o_kanban_record_menu_dropdown dropdown-menu";
  menu.setAttribute("role", "menu");
  menu.hidden = true;
  menu.setAttribute("hidden", "hidden");
  const setOpen = (open: boolean) => {
    toggle.setAttribute("aria-expanded", open ? "true" : "false");
    menu.hidden = !open;
    if (open) {
      menu.removeAttribute("hidden");
      menu.className = "o_kanban_record_menu_dropdown dropdown-menu show";
    } else {
      menu.setAttribute("hidden", "hidden");
      menu.className = "o_kanban_record_menu_dropdown dropdown-menu";
    }
  };
  toggle.addEventListener("click", (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    setOpen(toggle.getAttribute("aria-expanded") !== "true");
  });
  toggle.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    event.preventDefault?.();
    event.stopPropagation?.();
    setOpen(false);
  });
  const open = document.createElement("button");
  open.type = "button";
  open.className = "dropdown-item o_kanban_record_menu_item";
  open.dataset.kanbanRecordMenuAction = "open";
  open.setAttribute("role", "menuitem");
  open.textContent = "Open";
  open.addEventListener("click", async (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    setOpen(false);
    await openKanbanRecord(model, id, action, options, root);
  });
  const duplicate = document.createElement("button");
  duplicate.type = "button";
  duplicate.className = "dropdown-item o_kanban_record_menu_item";
  duplicate.dataset.kanbanRecordMenuAction = "duplicate";
  duplicate.setAttribute("role", "menuitem");
  duplicate.textContent = "Duplicate";
  duplicate.disabled = !options.services?.orm;
  duplicate.addEventListener("click", async (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    if (!options.services?.orm) return;
    setOpen(false);
    const newID = await options.services.orm.call(model, "copy", [id, {}]);
    await options.onRefresh?.();
    root.dispatchEvent(new CustomEvent("action-menu:duplicate", {
      bubbles: true,
      detail: { model, ids: [id], newId: newID }
    }));
  });
  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "dropdown-item o_kanban_record_menu_item text-danger";
  remove.dataset.kanbanRecordMenuAction = "delete";
  remove.setAttribute("role", "menuitem");
  remove.textContent = "Delete";
  remove.disabled = !options.services?.orm;
  remove.addEventListener("click", async (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    if (!options.services?.orm) return;
    const accepted = await confirmStaticAction(options, "Are you sure you want to delete this record?");
    if (!accepted) return;
    setOpen(false);
    await options.services.orm.unlink(model, [id]);
    await options.onRefresh?.();
    root.dispatchEvent(new CustomEvent("action-menu:delete", {
      bubbles: true,
      detail: { model, ids: [id] }
    }));
  });
  menu.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    event.preventDefault?.();
    event.stopPropagation?.();
    setOpen(false);
    toggle.focus?.();
  });
  wrapper.append(toggle, menu);
  menu.append(open, duplicate, remove);
  const serverItems = renderKanbanRecordServerActionMenuItems(model, id, actionMenus, options, root, () => setOpen(false));
  if (serverItems.length) {
    const separator = document.createElement("div");
    separator.className = "dropdown-divider o_kanban_record_menu_separator";
    separator.setAttribute("role", "separator");
    menu.append(separator, ...serverItems);
  }
  return wrapper;
}

function renderKanbanRecordServerActionMenuItems(
  model: string,
  id: number,
  actionMenus: Record<string, unknown> | undefined,
  options: RenderWindowActionOptions,
  root: HTMLElement,
  closeMenu: () => void
): HTMLElement[] {
  const items: HTMLElement[] = [];
  const printItems = actionMenuRecords(actionMenus, "print").map((item) =>
    renderKanbanRecordServerActionMenuButton("print", model, id, item, options, root, closeMenu)
  );
  const actionItems = actionMenuRecords(actionMenus, "action").map((item) =>
    renderKanbanRecordServerActionMenuButton("action", model, id, item, options, root, closeMenu)
  );
  if (printItems.length) {
    items.push(renderKanbanRecordMenuSectionLabel("Print"), ...printItems.sort(compareActionMenuButtons));
  }
  if (actionItems.length) {
    items.push(renderKanbanRecordMenuSectionLabel("Actions"), ...actionItems.sort(compareActionMenuButtons));
  }
  return items;
}

function renderKanbanRecordServerActionMenuButton(
  kind: "action" | "print",
  model: string,
  id: number,
  item: ActionMenuRecord,
  options: RenderWindowActionOptions,
  root: HTMLElement,
  closeMenu: () => void
): HTMLElement {
  const button = renderServerActionMenuButton(kind, model, item, () => [id], false, root, options);
  button.className = "dropdown-item o_kanban_record_menu_item gorp-action-menu-item";
  button.dataset.kanbanRecordMenuAction = kind;
  button.dataset.kanbanRecordServerAction = "true";
  button.dataset.recordId = String(id);
  button.addEventListener("click", (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    closeMenu();
  });
  return button;
}

function renderKanbanRecordMenuSectionLabel(label: string): HTMLElement {
  const section = document.createElement("div");
  section.className = "o_kanban_record_menu_section";
  section.dataset.kanbanRecordMenuSection = label.toLowerCase();
  section.setAttribute("role", "presentation");
  section.textContent = label;
  return section;
}

function renderKanbanQuickCreate(
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement,
  defaults: { groupField?: string; groupValue?: unknown } = {}
): HTMLElement | null {
  if (actionCreateDisabled(action)) return null;
  const formView = formViewRef(action);
  if (!formView) return null;
  const button = document.createElement("button");
  button.type = "button";
  button.className = "o_kanban_quick_add btn btn-link";
  button.dataset.kanbanQuickCreate = "true";
  if (defaults.groupField) button.dataset.groupField = defaults.groupField;
  const quickContext = kanbanQuickCreateContext(options.context ?? {}, defaults);
  const groupDefault = defaults.groupField ? quickContext[`default_${defaults.groupField}`] : undefined;
  if (groupDefault !== undefined) button.dataset.groupDefault = String(groupDefault);
  button.setAttribute("aria-label", "Add a record");
  button.textContent = "+ Add";
  button.addEventListener("click", async (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    const nextAction = createFormAction(action, formView);
    if (options.services?.action) {
      await options.services.action.doAction(nextAction, {
        additionalContext: quickContext,
        replaceLastAction: true
      });
      return;
    }
    root.dispatchEvent(new CustomEvent("action:create", {
      bubbles: true,
      detail: { action: nextAction, context: quickContext, model: action.res_model }
    }));
  });
  return button;
}

function kanbanQuickCreateContext(
  baseContext: Record<string, unknown>,
  defaults: { groupField?: string; groupValue?: unknown }
): Record<string, unknown> {
  const context = { ...baseContext };
  if (!defaults.groupField) return context;
  const defaultValue = kanbanGroupDefaultValue(defaults.groupValue);
  if (defaultValue === undefined) return context;
  context[`default_${defaults.groupField}`] = defaultValue;
  return context;
}

function kanbanGroupDefaultValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.length ? value[0] : undefined;
  if (value === undefined || value === null || value === false || value === "") return undefined;
  return value;
}

function kanbanRecordGroups(
  records: readonly Record<string, unknown>[],
  fields: Record<string, unknown>,
  groupField: string,
  model: string
): Array<{ key: string; label: string; raw: unknown; records: Record<string, unknown>[] }> {
  const groups = new Map<string, { key: string; label: string; raw: unknown; records: Record<string, unknown>[] }>();
  for (const record of records) {
    const raw = record[groupField];
    const key = kanbanGroupKey(raw);
    const label = fieldDisplayText(fields[groupField], raw, model, groupField) || "Undefined";
    const group = groups.get(key) ?? { key, label, raw, records: [] };
    group.records.push(record);
    groups.set(key, group);
  }
  return [...groups.values()];
}

function kanbanGroupKey(value: unknown): string {
  if (Array.isArray(value)) return value.length ? String(value[0] ?? "") : "";
  if (value === undefined || value === null || value === false || value === "") return "__undefined__";
  return String(value);
}

function renderKanbanTemplate(
  template: KanbanTemplateSet,
  record: Record<string, unknown>,
  fields: Record<string, unknown>,
  model: string
): HTMLElement {
  const root = document.createElement("div");
  root.className = "o_kanban_template_body";
  root.dataset.kanbanTemplateBody = "true";
  const context = kanbanTemplateEvalContext(record, fields, model);
  for (const node of template.main) {
    const rendered = renderKanbanTemplateNode(node, record, fields, model, context, template);
    if (rendered) root.append(rendered);
  }
  return root;
}

function renderKanbanTemplateNode(
  node: KanbanTemplateNode,
  record: Record<string, unknown>,
  fields: Record<string, unknown>,
  model: string,
  context: Record<string, unknown>,
  templates: KanbanTemplateSet
): Node | null {
  if (node.type === "text") {
    const text = collapseTemplateText(node.text);
    return text ? document.createTextNode(text) : null;
  }
  if (node.type === "field") {
    if (node.attrs["t-foreach"]) return renderKanbanTemplateLoop(node, record, fields, model, context, templates);
    if (!kanbanTemplateNodeVisible(node.attrs, context)) return null;
    if (!node.name || !fields[node.name]) return null;
    const wrapper = document.createElement("span");
    wrapper.className = `o_kanban_template_field ${kanbanTemplateClassName(node.attrs, context)}`.trim();
    wrapper.dataset.field = node.name;
    applyKanbanTemplateAttributes(wrapper, node.attrs, context);
    wrapper.append(renderReadonlyFieldValue(
      { name: node.name, attrs: node.attrs, children: [], childViewAttrs: {} },
      fields[node.name],
      record[node.name],
      record,
      undefined,
      undefined,
      model
    ));
    return wrapper;
  }
  if (node.attrs["t-foreach"]) return renderKanbanTemplateLoop(node, record, fields, model, context, templates);
  if (!kanbanTemplateNodeVisible(node.attrs, context)) return null;
  if (node.attrs["t-set"]) {
    kanbanTemplateSetValue(node, record, fields, model, context, templates);
    return null;
  }
  if (node.attrs["t-call"]) return renderKanbanTemplateCall(node, record, fields, model, context, templates);
  if (node.attrs["t-esc"] || node.attrs["t-out"]) {
    const expression = node.attrs["t-esc"] || node.attrs["t-out"] || "";
    const value = kanbanTemplateExpressionValue(expression, context);
    const isRawOutput = Boolean(node.attrs["t-out"]);
    if (isRawOutput && kanbanTemplateIsNodeLike(value)) {
      const materialized = cloneKanbanTemplateOutput(value);
      if (node.tag.toLowerCase() === "t") return materialized;
      const element = document.createElement(kanbanTemplateSafeTag(node.tag));
      element.className = kanbanTemplateClassName(node.attrs, context);
      applyKanbanTemplateAttributes(element, node.attrs, context);
      element.append(materialized);
      return element;
    }
    if (node.tag.toLowerCase() === "t") return document.createTextNode(formatCellValue(value));
    const element = document.createElement(kanbanTemplateSafeTag(node.tag));
    element.className = kanbanTemplateClassName(node.attrs, context);
    applyKanbanTemplateAttributes(element, node.attrs, context);
    element.append(document.createTextNode(formatCellValue(value)));
    return element;
  }
  if (node.tag.toLowerCase() === "t") {
    const fragment = createKanbanTemplateFragment();
    for (const child of node.children) {
      const rendered = renderKanbanTemplateNode(child, record, fields, model, context, templates);
      if (rendered) fragment.append(rendered);
    }
    return kanbanTemplateFragmentLength(fragment) ? fragment : null;
  }
  const tag = kanbanTemplateSafeTag(node.tag);
  const element = document.createElement(tag);
  element.className = kanbanTemplateClassName(node.attrs, context);
  if (tag === "strong") element.className = `${element.className} o_kanban_record_title`.trim();
  applyKanbanTemplateAttributes(element, node.attrs, context);
  for (const child of node.children) {
    const rendered = renderKanbanTemplateNode(child, record, fields, model, context, templates);
    if (rendered) element.append(rendered);
  }
  if (!element.children.length && !element.textContent.trim()) return null;
  return element;
}

function renderKanbanTemplateLoop(
  node: Exclude<KanbanTemplateNode, { type: "text" }>,
  record: Record<string, unknown>,
  fields: Record<string, unknown>,
  model: string,
  context: Record<string, unknown>,
  templates: KanbanTemplateSet
): Node | null {
  const expression = node.attrs["t-foreach"] || "";
  const asName = node.attrs["t-as"] || "item";
  const items = kanbanTemplateIterable(kanbanTemplateExpressionValue(expression, context));
  const fragment = createKanbanTemplateFragment();
  items.forEach((item, index) => {
    const attrs = { ...node.attrs };
    delete attrs["t-foreach"];
    delete attrs["t-as"];
    const loopNode = { ...node, attrs };
    const rendered = renderKanbanTemplateNode(
      loopNode,
      record,
      fields,
      model,
      kanbanTemplateLoopContext(context, asName, item, index, items.length),
      templates
    );
    if (rendered) fragment.append(rendered);
  });
  return kanbanTemplateFragmentLength(fragment) ? fragment : null;
}

function renderKanbanTemplateCall(
  node: Exclude<KanbanTemplateNode, { type: "text" | "field" }>,
  record: Record<string, unknown>,
  fields: Record<string, unknown>,
  model: string,
  context: Record<string, unknown>,
  templates: KanbanTemplateSet
): Node | null {
  const callName = kanbanTemplateCallName(node.attrs["t-call"] || "", context);
  if (!callName || callName === "kanban-box") return null;
  const called = templates.named[callName];
  if (!called?.length) return null;
  const fragment = createKanbanTemplateFragment();
  const callContext = { ...context };
  const body = createKanbanTemplateFragment();
  for (const child of node.children) {
    const rendered = renderKanbanTemplateNode(child, record, fields, model, callContext, templates);
    if (rendered) body.append(rendered);
  }
  callContext["0"] = body;
  for (const child of called) {
    const rendered = renderKanbanTemplateNode(child, record, fields, model, callContext, templates);
    if (rendered) fragment.append(rendered);
  }
  return kanbanTemplateFragmentLength(fragment) ? fragment : null;
}

function kanbanTemplateCallName(value: string, context: Record<string, unknown>): string {
  const trimmed = value.trim();
  if (!trimmed) return "";
  if (/^[\w.-]+$/.test(trimmed)) return trimmed;
  const evaluated = kanbanTemplateExpressionValue(trimmed, context);
  return typeof evaluated === "string" ? evaluated : "";
}

function createKanbanTemplateFragment(): DocumentFragment | HTMLElement {
  const factory = (document as Document & { createDocumentFragment?: () => DocumentFragment }).createDocumentFragment;
  return typeof factory === "function" ? factory.call(document) : document.createElement("span");
}

function kanbanTemplateFragmentLength(fragment: DocumentFragment | HTMLElement): number {
  const childNodes = (fragment as { childNodes?: { length: number } }).childNodes;
  if (childNodes) return childNodes.length;
  return (fragment as { children?: { length: number } }).children?.length ?? 0;
}

function kanbanTemplateIsNodeLike(value: unknown): value is Node {
  return isRecord(value) && (typeof value.nodeType === "number" || typeof value.append === "function" || Array.isArray(value.children));
}

function cloneKanbanTemplateOutput(value: Node): Node {
  if (typeof value.cloneNode === "function") return value.cloneNode(true);
  if (isRecord(value) && Array.isArray(value.children)) {
    const tag = typeof value.tag === "string" ? value.tag : "";
    const out = tag && tag !== "#fragment" && tag !== "#text"
      ? document.createElement(tag)
      : createKanbanTemplateFragment();
    if (tag === "#text") return document.createTextNode(formatCellValue(value.textContent));
    if (isRecord(out)) {
      if (typeof value.className === "string") (out as HTMLElement).className = value.className;
      if (isRecord(value.dataset)) Object.assign((out as HTMLElement).dataset, value.dataset);
      if (isRecord(value.attributes)) {
        for (const [name, attrValue] of Object.entries(value.attributes)) {
          (out as HTMLElement).setAttribute?.(name, formatCellValue(attrValue));
        }
      }
    }
    for (const child of value.children as unknown[]) {
      if (kanbanTemplateIsNodeLike(child)) out.append(cloneKanbanTemplateOutput(child));
      else if (isRecord(child) && typeof child.textContent === "string") out.append(document.createTextNode(child.textContent));
    }
    return out;
  }
  return document.createTextNode(formatCellValue(value));
}

function kanbanTemplateIterable(value: unknown): unknown[] {
  if (Array.isArray(value)) return [...value];
  if (isRecord(value)) return Object.entries(value).map(([key, item]) => ({ key, value: item }));
  return [];
}

function kanbanTemplateLoopContext(
  context: Record<string, unknown>,
  asName: string,
  item: unknown,
  index: number,
  length: number
): Record<string, unknown> {
  return {
    ...context,
    [asName]: item,
    [`${asName}_index`]: index,
    [`${asName}_first`]: index === 0,
    [`${asName}_last`]: index === length - 1,
    [`${asName}_parity`]: index % 2 ? "odd" : "even"
  };
}

function kanbanTemplateEvalContext(record: Record<string, unknown>, fields: Record<string, unknown>, model: string): Record<string, unknown> {
  const recordContext: Record<string, unknown> = {};
  for (const [name, description] of Object.entries(fields)) {
    const raw = record[name];
    recordContext[name] = {
      raw_value: raw,
      value: fieldDisplayText(description, raw, model, name)
    };
  }
  for (const [name, raw] of Object.entries(record)) {
    if (name in recordContext) continue;
    recordContext[name] = {
      raw_value: raw,
      value: fieldDisplayText(fields[name], raw, model, name)
    };
  }
  return {
    record: recordContext,
    raw_record: record,
    id: record.id,
    true: true,
    false: false
  };
}

function kanbanTemplateNodeVisible(attrs: Record<string, string>, context: Record<string, unknown>): boolean {
  const expression = attrs["t-if"] || attrs["t-elif"];
  if (!expression) return true;
  const value = kanbanTemplateExpressionValue(expression, context);
  return pythonTruthy(value);
}

function kanbanTemplateSetValue(
  node: Exclude<KanbanTemplateNode, { type: "text" | "field" }>,
  record: Record<string, unknown>,
  fields: Record<string, unknown>,
  model: string,
  context: Record<string, unknown>,
  templates: KanbanTemplateSet
): void {
  const name = node.attrs["t-set"]?.trim();
  if (!name || !/^[a-zA-Z_][\w]*$/.test(name)) return;
  if (node.attrs["t-value"] !== undefined) {
    context[name] = kanbanTemplateExpressionValue(node.attrs["t-value"], context);
    return;
  }
  const fragment = createKanbanTemplateFragment();
  for (const child of node.children) {
    const rendered = renderKanbanTemplateNode(child, record, fields, model, context, templates);
    if (rendered) fragment.append(rendered);
  }
  context[name] = fragment;
}

function kanbanTemplateExpressionValue(expression: string, context: Record<string, unknown>): unknown {
  const trimmed = expression.trim();
  if (Object.prototype.hasOwnProperty.call(context, trimmed)) return context[trimmed];
  const dotted = kanbanTemplateDottedValue(trimmed, context);
  if (dotted.found) return dotted.value;
  try {
    return evaluateExpr(trimmed, context);
  } catch {
    return "";
  }
}

function kanbanTemplateDottedValue(expression: string, context: Record<string, unknown>): { found: boolean; value?: unknown } {
  if (!/^[a-zA-Z_][\w]*(?:\.[a-zA-Z_][\w]*)+$/.test(expression)) return { found: false };
  let current: unknown = context;
  for (const part of expression.split(".")) {
    if (!isRecord(current) || !(part in current)) return { found: false };
    current = current[part];
  }
  return { found: true, value: current };
}

function kanbanTemplateClassName(attrs: Record<string, string>, context: Record<string, unknown>): string {
  const classes = [attrs.class || ""];
  if (attrs["t-att-class"]) {
    const value = kanbanTemplateExpressionValue(attrs["t-att-class"], context);
    if (typeof value === "string") classes.push(value);
    else if (Array.isArray(value)) classes.push(value.filter(Boolean).map(String).join(" "));
    else if (isRecord(value)) classes.push(Object.entries(value).filter(([, active]) => pythonTruthy(active)).map(([name]) => name).join(" "));
  }
  if (attrs["t-attf-class"]) {
    classes.push(renderKanbanTemplateFormatString(attrs["t-attf-class"], context));
  }
  return classes.join(" ").replace(/\s+/g, " ").trim();
}

function applyKanbanTemplateAttributes(element: HTMLElement, attrs: Record<string, string>, context: Record<string, unknown>): void {
  for (const [name, value] of Object.entries(attrs)) {
    if (name === "class" || name.startsWith("t-")) continue;
    setKanbanTemplateAttribute(element, name, value);
  }
  const attributeMap = kanbanTemplateExpressionValue(attrs["t-att"] || "", context);
  if (isRecord(attributeMap)) {
    for (const [name, value] of Object.entries(attributeMap)) {
      setKanbanTemplateAttribute(element, name, value);
    }
  }
  for (const [name, expression] of Object.entries(attrs)) {
    if (name.startsWith("t-att-")) {
      setKanbanTemplateAttribute(element, name.slice("t-att-".length), kanbanTemplateExpressionValue(expression, context));
    } else if (name.startsWith("t-attf-")) {
      setKanbanTemplateAttribute(element, name.slice("t-attf-".length), renderKanbanTemplateFormatString(expression, context));
    }
  }
}

function setKanbanTemplateAttribute(element: HTMLElement, rawName: string, rawValue: unknown): void {
  const name = kanbanTemplateSafeAttributeName(rawName);
  if (!name) return;
  const value = kanbanTemplateAttributeValue(rawValue);
  if (value === undefined) return;
  if ((name === "href" || name === "src") && !kanbanTemplateSafeURL(value)) return;
  element.setAttribute(name, value);
  if (name.startsWith("data-")) element.dataset[kanbanTemplateDatasetKey(name)] = value;
  if (name === "name") element.dataset.templateName = value;
}

function kanbanTemplateDatasetKey(name: string): string {
  return name.slice(5).replace(/-([a-z])/g, (_match, letter: string) => letter.toUpperCase());
}

function kanbanTemplateSafeAttributeName(rawName: string): string {
  const name = rawName.trim().toLowerCase();
  if (!/^[a-z][\w:.-]*$/.test(name)) return "";
  if (name === "class" || name === "style" || name.startsWith("on")) return "";
  if (name.startsWith("data-") || name.startsWith("aria-")) return name;
  if (["title", "role", "href", "target", "rel", "src", "alt", "width", "height", "loading", "decoding", "name", "type", "value"].includes(name)) return name;
  return "";
}

function kanbanTemplateAttributeValue(value: unknown): string | undefined {
  if (value === undefined || value === null || value === false || value === "") return undefined;
  if (value === true) return "true";
  return formatCellValue(value);
}

function kanbanTemplateSafeURL(value: string): boolean {
  const normalized = value.trim().replace(/[\u0000-\u001F\s]+/g, "").toLowerCase();
  return !normalized.startsWith("javascript:") && !normalized.startsWith("data:text/html");
}

function renderKanbanTemplateFormatString(value: string, context: Record<string, unknown>): string {
  return value.replace(/#\{([^}]+)\}|\{\{([^}]+)\}\}/g, (_match, hashExpression, braceExpression) => {
    const evaluated = kanbanTemplateExpressionValue(String(hashExpression || braceExpression || ""), context);
    return formatCellValue(evaluated);
  });
}

function kanbanTemplateSafeTag(tag: string): string {
  const normalized = tag.toLowerCase();
  if (["div", "span", "strong", "b", "em", "i", "small", "p", "section", "header", "footer", "ul", "ol", "li", "h1", "h2", "h3", "h4", "h5", "h6"].includes(normalized)) {
    return normalized;
  }
  return "span";
}

function collapseTemplateText(value: string): string {
  return value.replace(/\s+/g, " ").trim();
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

type ModuleCatalogFilter = "all" | "official" | "industries";

interface ModuleCatalogReference {
  sequence: number;
  displayName: string;
  technicalName: string;
  category: string;
  summary: string;
  industry: boolean;
  official: boolean;
  website?: string;
}

interface ModuleCatalogItem {
  id?: number;
  category: string;
  description: string;
  displayName: string;
  industry: boolean;
  installable: boolean;
  official: boolean;
  searchText: string;
  sequence: number;
  sourceRecord?: Record<string, unknown>;
  state: string;
  summary: string;
  technicalName: string;
  virtual: boolean;
  website: string;
}

type ModuleCatalogHost = HTMLElement & { __gorpActionRoot?: HTMLElement };

function renderModuleCatalogView(
  records: readonly Record<string, unknown>[],
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  pager?: KanbanLoadMorePager
): HTMLElement {
  const modules = moduleCatalogItems(records);
  const wrapper = document.createElement("section");
  wrapper.className = "gorp-apps-catalog o_apps_view";
  wrapper.setAttribute("style", "background:#262a36 !important;color:#e8e9ef !important;min-height:calc(100vh - 104px) !important;margin:0 !important;padding:0 !important;");
  wrapper.dataset.model = "ir.module.module";
  wrapper.dataset.catalogTotal = String(modules.length);
  wrapper.dataset.activeFilter = "all";
  wrapper.dataset.activeCategory = "all";
  wrapper.append(moduleCatalogStyleElement());

  const content = document.createElement("div");
  content.className = "o-list-content gorp-apps-catalog-content";
  content.setAttribute("style", "display:flex !important;background:#262a36 !important;min-height:calc(100vh - 104px) !important;overflow:hidden !important;margin:0 !important;padding:0 !important;");
  const sidebar = document.createElement("aside");
  sidebar.className = "gorp-apps-catalog-sidebar o_search_panel";
  sidebar.setAttribute("style", "background:#262a36 !important;color:#e8e9ef !important;flex:0 0 220px !important;width:220px !important;box-sizing:border-box !important;");
  sidebar.setAttribute("aria-label", "App categories");
  const renderer = document.createElement("div");
  renderer.className = "gorp-apps-catalog-grid o_apps o_kanban_renderer o_renderer o_kanban_ungrouped";
  renderer.setAttribute("style", "display:grid !important;grid-template-columns:repeat(3,minmax(260px,1fr)) !important;gap:8px 16px !important;align-content:start !important;flex:1 1 auto !important;background:#262a36 !important;padding:14px 16px 40px !important;overflow-y:auto !important;");
  renderer.dataset.model = "ir.module.module";
  const pagerLabel = document.createElement("span");
  pagerLabel.className = "gorp-apps-catalog-pager";
  pagerLabel.hidden = true;
  const topActions = renderModuleCatalogTopActions();
  wrapper.append(topActions);
  content.append(sidebar, renderer, pagerLabel);
  wrapper.append(content);

  let activeFilter: ModuleCatalogFilter = "all";
  let activeCategory = "all";
  const filterButtons = renderModuleCatalogFilters(sidebar, activeFilter, (filter) => {
    activeFilter = filter;
    wrapper.dataset.activeFilter = filter;
    render();
  });
  const categoryButtons = renderModuleCatalogCategories(sidebar, modules, activeCategory, (category) => {
    activeCategory = category;
    wrapper.dataset.activeCategory = category;
    render();
  });
  const render = () => {
    const query = String(pager?.search?.state.query ?? "").trim().toLowerCase();
    for (const button of filterButtons) {
      const active = button.dataset.catalogFilter === activeFilter;
      button.className = active ? "o_search_panel_filter active" : "o_search_panel_filter";
      button.setAttribute("aria-pressed", active ? "true" : "false");
    }
    for (const button of categoryButtons) {
      const active = button.dataset.category === activeCategory;
      button.className = active ? "o_search_panel_category active" : "o_search_panel_category";
      button.setAttribute("aria-pressed", active ? "true" : "false");
    }
    const visible = modules.filter((item) => {
      if (query && !item.searchText.includes(query)) return false;
      if (activeCategory !== "all" && item.category !== activeCategory) return false;
      if (activeFilter === "official" && !item.official) return false;
      if (activeFilter === "industries" && !item.industry) return false;
      return true;
    });
    renderer.replaceChildren();
    for (const item of visible) renderer.append(renderModuleCatalogCard(item, action, options, wrapper));
    if (!visible.length) {
      const empty = document.createElement("div");
      empty.className = "o_view_nocontent";
      empty.textContent = query ? "No apps found." : "No apps available.";
      renderer.append(empty);
    }
    wrapper.dataset.visibleCount = String(visible.length);
    pagerLabel.textContent = visible.length ? `1-${visible.length} / ${visible.length}` : "0 / 0";
  };
  render();
  return wrapper;
}

function renderModuleCatalogTopActions(): HTMLElement {
  const toolbar = document.createElement("div");
  toolbar.className = "gorp-apps-catalog-top-actions o_control_panel_main_buttons";
  toolbar.setAttribute("role", "toolbar");
  toolbar.setAttribute("aria-label", "Apps actions");
  toolbar.setAttribute("style", "display:flex !important;align-items:center !important;gap:8px !important;padding:10px 16px 0 !important;background:#262a36 !important;color:#e8e9ef !important;");
  for (const action of [
    ["update_apps_list", "Update Apps List"],
    ["apply_scheduled_upgrades", "Apply Scheduled Upgrades"],
    ["import_module", "Import Module"]
  ] as const) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "btn btn-secondary btn-sm o_app_action";
    button.dataset.catalogAction = action[0];
    button.textContent = action[1];
    toolbar.append(button);
  }
  return toolbar;
}

function renderModuleCatalogFilters(
  sidebar: HTMLElement,
  activeFilter: ModuleCatalogFilter,
  onSelect: (filter: ModuleCatalogFilter) => void
): HTMLButtonElement[] {
  appendModuleCatalogSidebarHeader(sidebar, "APPS");
  const filters: Array<{ id: ModuleCatalogFilter; label: string }> = [
    { id: "all", label: "All" },
    { id: "official", label: "Official Apps" },
    { id: "industries", label: "Industries" }
  ];
  const buttons: HTMLButtonElement[] = [];
  for (const filter of filters) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = filter.id === activeFilter ? "o_search_panel_filter active" : "o_search_panel_filter";
    button.dataset.catalogFilter = filter.id;
    button.setAttribute("aria-pressed", filter.id === activeFilter ? "true" : "false");
    const label = document.createElement("span");
    label.className = "o_search_panel_label";
    label.textContent = filter.label;
    button.append(label);
    button.addEventListener("click", () => onSelect(filter.id));
    sidebar.append(button);
    buttons.push(button);
  }
  return buttons;
}

function renderModuleCatalogCategories(
  sidebar: HTMLElement,
  modules: readonly ModuleCatalogItem[],
  activeCategory: string,
  onSelect: (category: string) => void
): HTMLButtonElement[] {
  const counts = new Map<string, number>();
  for (const item of modules) counts.set(item.category, (counts.get(item.category) ?? 0) + 1);
  const categories = ["all", ...[...counts.keys()].filter(moduleCatalogCategoryVisible).sort(moduleCatalogCategorySort)];
  const buttons: HTMLButtonElement[] = [];
  appendModuleCatalogSidebarHeader(sidebar, "CATEGORIES");
  for (const category of categories) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = category === activeCategory ? "o_search_panel_category active" : "o_search_panel_category";
    button.dataset.category = category;
    button.setAttribute("aria-pressed", category === activeCategory ? "true" : "false");
    const label = document.createElement("span");
    label.className = "o_search_panel_label";
    label.textContent = category === "all" ? "All" : category;
    button.append(label);
    if (category !== "all") {
      const count = document.createElement("span");
      count.className = "o_search_panel_counter";
      count.textContent = String(counts.get(category) ?? 0);
      button.append(count);
    }
    button.addEventListener("click", () => onSelect(category));
    sidebar.append(button);
    buttons.push(button);
  }
  return buttons;
}

function appendModuleCatalogSidebarHeader(sidebar: HTMLElement, label: string): void {
  const header = document.createElement("div");
  header.className = "o_search_panel_section_header";
  header.textContent = label;
  sidebar.append(header);
}

function renderModuleCatalogCard(
  item: ModuleCatalogItem,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): HTMLElement {
  const card = document.createElement("article");
  card.className = "o_kanban_record d-flex flex-grow-1 flex-md-shrink-1 flex-shrink-0 flex-row align-items-center gorp-apps-catalog-card module-card o_app";
  card.setAttribute("style", "position:relative !important;display:flex !important;flex-direction:row !important;align-items:center !important;gap:14px !important;min-width:0 !important;min-height:94px !important;height:94px !important;margin:0 !important;padding:10px 30px 10px 12px !important;background:#2a2f3b !important;border:1px solid #454a59 !important;border-radius:0 !important;box-shadow:none !important;color:#e8e9ef !important;overflow:hidden !important;");
  card.dataset.id = item.id !== undefined ? String(item.id) : `datapoint_${item.sequence}`;
  card.dataset.moduleName = item.technicalName;
  card.dataset.appName = item.displayName;
  card.dataset.category = item.category;
  card.dataset.state = item.state;
  card.dataset.virtualModule = item.virtual ? "true" : "false";
  if (item.id !== undefined) {
    card.setAttribute("role", "link");
    card.setAttribute("tabindex", "0");
    card.addEventListener("click", async () => {
      await openKanbanRecord("ir.module.module", item.id as number, action, options, root);
    });
  }
  const icon = moduleCatalogIconElement(item);
  const image = document.createElement("div");
  image.className = "o_module_icon_container gorp-apps-catalog-card-image";
  image.dataset.moduleImage = item.technicalName;
  image.append(icon);
  const details = document.createElement("div");
  details.className = "oe_kanban_details o_app_details";
  details.setAttribute("style", "min-width:0 !important;flex:1 1 auto !important;display:flex !important;flex-direction:column !important;align-self:stretch !important;justify-content:center !important;gap:1px !important;overflow:hidden !important;");
  const title = document.createElement("strong");
  title.className = "o_kanban_record_title o_app_name";
  title.setAttribute("style", "position:static !important;display:block !important;width:auto !important;margin:0 !important;padding:0 !important;color:#f2f3f6 !important;font-size:15px !important;line-height:19px !important;font-weight:700 !important;text-align:left !important;white-space:nowrap !important;overflow:hidden !important;text-overflow:ellipsis !important;opacity:1 !important;transform:none !important;");
  title.textContent = item.displayName;
  const technicalCode = document.createElement("span");
  technicalCode.className = "o_module_technical_name o_app_summary";
  technicalCode.dataset.moduleTechnicalName = item.technicalName;
  technicalCode.setAttribute("style", "position:static !important;display:block !important;margin:0 !important;color:#d889c1 !important;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace !important;font-size:13px !important;line-height:16px !important;white-space:nowrap !important;overflow:hidden !important;text-overflow:ellipsis !important;opacity:1 !important;");
  technicalCode.textContent = item.technicalName;
  details.append(title, technicalCode, renderModuleCatalogActions(item, action, options, root));
  const menu = document.createElement("button");
  menu.type = "button";
  menu.className = "o_kanban_record_menu_toggle o_module_menu btn btn-link";
  menu.dataset.moduleMenu = item.technicalName;
  menu.setAttribute("aria-label", `${item.displayName} actions`);
  menu.textContent = "⋮";
  card.append(image, details, menu);
  return card;
}

function renderModuleCatalogActions(
  item: ModuleCatalogItem,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): HTMLElement {
  const wrapper = document.createElement("div");
  wrapper.className = "o_module_actions";
  wrapper.setAttribute("style", "position:static !important;display:flex !important;align-items:center !important;gap:7px !important;margin-top:5px !important;opacity:1 !important;");
  const state = document.createElement("span");
  state.className = "badge o_module_state d-none";
  state.hidden = true;
  state.textContent = item.state;
  wrapper.append(state);
  for (const moduleAction of moduleKanbanActionItems(item.state)) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = moduleAction.className;
    button.dataset.moduleAction = moduleAction.method;
    if (item.virtual) button.dataset.virtualAction = "true";
    button.textContent = moduleAction.label;
    button.addEventListener("click", async (event) => {
      event.preventDefault?.();
      event.stopPropagation?.();
      if (item.id !== undefined) {
        button.disabled = true;
        button.textContent = moduleAction.runningLabel;
        await runModuleKanbanAction(item.id, moduleAction.method, action, options, root);
        return;
      }
      root.dispatchEvent(new CustomEvent("action:module-action", {
        bubbles: true,
        detail: { model: "ir.module.module", module: item.technicalName, method: moduleAction.method, virtual: true }
      }));
    });
    wrapper.append(button);
  }
  wrapper.append(renderModuleCatalogInfoButton(item, action, options, root));
  return wrapper;
}

function renderModuleCatalogInfoButton(
  item: ModuleCatalogItem,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-secondary btn-sm o_module_info_button";
  button.dataset.moduleInfo = item.technicalName;
  button.textContent = item.website ? "Learn More" : "Module Info";
  button.addEventListener("click", async (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    if (item.id === undefined) {
      openVirtualModuleInfoAction(item, action, root);
      return;
    }
    await openModuleInfoAction(item.sourceRecord ?? { id: item.id, name: item.technicalName, shortdesc: item.displayName }, item.id, action, options, root);
  });
  return button;
}

async function openModuleInfoAction(
  record: Record<string, unknown>,
  id: number,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): Promise<void> {
  const formView = formViewRef(action) ?? [false, "form"];
  const nextAction: Record<string, unknown> = {
    ...action,
    name: "Module Info",
    res_id: id,
    res_model: "ir.module.module",
    view_mode: "form",
    views: [[formView[0], "form"]],
    target: "new"
  };
  const context = {
    ...(options.context ?? {}),
    active_model: "ir.module.module",
    active_id: id,
    active_ids: [id]
  };
  if (options.services?.action) {
    await options.services.action.doAction(nextAction, { additionalContext: context });
    return;
  }
  root.dispatchEvent(new CustomEvent("action:open-record", {
    bubbles: true,
    detail: { action: nextAction, model: "ir.module.module", id, record }
  }));
}

function openVirtualModuleInfoAction(item: ModuleCatalogItem, action: Record<string, unknown>, root: HTMLElement): void {
  const actionRoot = (root as ModuleCatalogHost).__gorpActionRoot
    ?? (typeof root.closest === "function" ? root.closest(".gorp-window-action") as HTMLElement | null : null)
    ?? root;
  actionRoot.dataset.view = "form";
  actionRoot.dataset.resId = String(item.id ?? `virtual_${item.technicalName}`);
  actionRoot.dataset.moduleInfoAction = item.technicalName;
  actionRoot.setAttribute("style", "background:#262a36 !important;color:#e8e9ef !important;min-height:calc(100vh - 104px) !important;");
  actionRoot.replaceChildren(moduleCatalogStyleElement(), renderVirtualModuleInfoControlPanel(item, action), renderVirtualModuleInfoForm(item));
}

function renderVirtualModuleInfoControlPanel(item: ModuleCatalogItem, action: Record<string, unknown>): HTMLElement {
  const control = document.createElement("section");
  control.className = "o_control_panel gorp-module-info-control-panel";
  control.dataset.model = "ir.module.module";
  control.setAttribute("style", "position:relative !important;background:#262a36 !important;border-bottom:1px solid #3a3f4e !important;color:#f4f5f7 !important;height:56px !important;min-height:56px !important;max-height:56px !important;padding:0 16px !important;display:flex !important;flex-direction:row !important;align-items:center !important;justify-content:space-between !important;gap:16px !important;overflow:visible !important;");
  const top = document.createElement("div");
  top.className = "o_control_panel_navigation";
  top.setAttribute("style", "position:static !important;inset:auto !important;transform:translateY(14px) !important;display:flex !important;align-items:center !important;gap:16px !important;flex:1 1 auto !important;min-width:0 !important;height:34px !important;margin:0 !important;padding:0 !important;");
  const breadcrumb = document.createElement("ol");
  breadcrumb.className = "breadcrumb o_breadcrumb";
  breadcrumb.setAttribute("style", "position:static !important;inset:auto !important;transform:none !important;display:flex !important;align-items:center !important;gap:6px !important;height:34px !important;margin:0 !important;padding:0 !important;color:#f4f5f7 !important;line-height:26px !important;white-space:nowrap !important;");
  const apps = document.createElement("li");
  apps.className = "breadcrumb-item";
  apps.setAttribute("style", "position:static !important;display:flex !important;align-items:center !important;height:34px !important;margin:0 !important;padding:0 !important;");
  const appsButton = document.createElement("button");
  appsButton.type = "button";
  appsButton.className = "btn btn-link p-0 o_back_button";
  appsButton.dataset.moduleInfoBack = "apps";
  appsButton.setAttribute("style", "position:static !important;height:26px !important;min-height:26px !important;margin:0 !important;padding:0 !important;border:0 !important;background:transparent !important;color:#f4f5f7 !important;line-height:26px !important;box-shadow:none !important;");
  appsButton.textContent = "Apps";
  apps.append(appsButton);
  const current = document.createElement("li");
  current.className = "breadcrumb-item active";
  current.setAttribute("aria-current", "page");
  current.setAttribute("style", "position:static !important;display:flex !important;align-items:center !important;height:34px !important;margin:0 !important;padding:0 !important;color:#f4f5f7 !important;line-height:26px !important;");
  current.textContent = item.displayName;
  breadcrumb.append(apps, current);
  const pager = document.createElement("div");
  pager.className = "o_pager";
  pager.setAttribute("style", "position:static !important;inset:auto !important;transform:none !important;display:flex !important;align-items:center !important;gap:4px !important;height:34px !important;margin:0 !important;padding:0 !important;color:#e4e4e4 !important;white-space:nowrap !important;");
  const value = document.createElement("span");
  value.className = "o_pager_value";
  value.textContent = "1";
  const sep = document.createElement("span");
  sep.textContent = " / ";
  const limit = document.createElement("span");
  limit.className = "o_pager_limit";
  limit.textContent = "1";
  const previous = document.createElement("button");
  previous.type = "button";
  previous.className = "o_pager_previous btn btn-secondary";
  previous.disabled = true;
  previous.setAttribute("aria-label", "Previous");
  previous.setAttribute("style", "position:static !important;width:28px !important;height:28px !important;min-width:28px !important;min-height:28px !important;margin:0 !important;padding:0 !important;display:grid !important;place-items:center !important;");
  previous.textContent = "Previous";
  const next = document.createElement("button");
  next.type = "button";
  next.className = "o_pager_next btn btn-secondary";
  next.disabled = true;
  next.setAttribute("aria-label", "Next");
  next.setAttribute("style", "position:static !important;width:28px !important;height:28px !important;min-width:28px !important;min-height:28px !important;margin:0 !important;padding:0 !important;display:grid !important;place-items:center !important;");
  next.textContent = "Next";
  pager.append(value, sep, limit, previous, next);
  top.append(breadcrumb, pager);
  const bottom = document.createElement("div");
  bottom.className = "o_control_panel_main d-flex align-items-center justify-content-between";
  bottom.setAttribute("style", "position:static !important;inset:auto !important;transform:none !important;display:flex !important;align-items:center !important;gap:8px !important;flex:0 0 auto !important;min-height:34px !important;margin:0 !important;padding:0 !important;");
  const actions = document.createElement("div");
  actions.className = "o_control_panel_actions";
  actions.setAttribute("style", "position:static !important;inset:auto !important;transform:none !important;display:flex !important;align-items:center !important;margin:0 !important;padding:0 !important;");
  const menus = renderActionMenus({
    className: "gorp-form-action-menu",
    model: "ir.module.module",
    actionMenus: undefined,
    staticActionButtons: [
      renderStaticActionMenuButton("duplicate", "Duplicate", "fa fa-clone", 30, async () => undefined)
    ],
    getActiveIds: () => item.id !== undefined ? [item.id] : [],
    requiresSelection: false,
    root: control,
    options: {}
  });
  menus.setAttribute("style", "position:static !important;inset:auto !important;transform:none !important;display:flex !important;align-items:center !important;margin:0 !important;padding:0 !important;");
  actions.append(menus);
  actions.dataset.sourceAction = String(action.id ?? "");
  bottom.append(actions);
  control.append(top, bottom);
  return control;
}

function renderVirtualModuleInfoForm(item: ModuleCatalogItem): HTMLElement {
  const body = document.createElement("div");
  body.className = "gorp-module-info-body gorp-form-body o-list-content o-form-content o_form_sheet_bg";
  body.setAttribute("style", "width:100% !important;max-width:none !important;margin:0 !important;background:#262a36 !important;color:#e8e9ef !important;min-height:calc(100vh - 102px) !important;padding:18px 0 48px !important;");
  const sheet = document.createElement("section");
  sheet.className = "gorp-form-sheet o-form-sheet o_form_sheet gorp-module-info-sheet";
  sheet.setAttribute("style", "width:calc(100% - 32px) !important;max-width:none !important;margin:10px 16px 0 !important;box-sizing:border-box !important;background:#2a2f3b !important;border:1px solid #454a59 !important;color:#e8e9ef !important;");
  const title = document.createElement("div");
  title.className = "oe_title gorp-module-info-title";
  const h1 = document.createElement("h1");
  h1.textContent = item.displayName;
  const summary = document.createElement("div");
  summary.className = "o_module_summary";
  summary.textContent = item.summary;
  const author = document.createElement("div");
  author.className = "o_module_author";
  author.textContent = "By Odoo S.A.";
  const activate = document.createElement("button");
  activate.type = "button";
  activate.className = "btn btn-primary o_module_install_button";
  activate.dataset.moduleAction = "button_immediate_install";
  activate.textContent = "Activate";
  const icon = moduleCatalogIconElement(item);
  icon.className = `${icon.className} gorp-module-info-icon`;
  icon.setAttribute("style", "position:absolute !important;top:24px !important;right:24px !important;width:88px !important;height:88px !important;border-radius:0 !important;background:#f8f9fa !important;object-fit:contain !important;");
  icon.alt = "Binary file";
  icon.removeAttribute("aria-hidden");
  title.append(h1, summary, author, activate, icon);
  sheet.append(title, renderVirtualModuleInfoNotebook(item));
  body.append(sheet);
  return body;
}

function renderVirtualModuleInfoNotebook(item: ModuleCatalogItem): HTMLElement {
  const notebook = document.createElement("section");
  notebook.className = "gorp-form-notebook o_notebook gorp-module-info-notebook";
  notebook.dataset.notebook = "module-info";
  const nav = document.createElement("div");
  nav.className = "gorp-form-notebook-tabs nav nav-tabs";
  nav.setAttribute("role", "tablist");
  const pages = document.createElement("div");
  pages.className = "gorp-form-notebook-pages tab-content";
  const moduleInfoPages = [
    { id: "information", label: "Information" },
    { id: "technical-data", label: "Technical Data" }
  ] as const;
  for (const [index, page] of moduleInfoPages.entries()) {
    const tab = document.createElement("button");
    tab.type = "button";
    tab.className = "gorp-form-notebook-tab nav-link";
    tab.dataset.tabPage = page.id;
    tab.setAttribute("role", "tab");
    tab.setAttribute("aria-selected", index === 0 ? "true" : "false");
    tab.textContent = page.label;
    const panel = document.createElement("div");
    panel.className = "gorp-form-notebook-page tab-pane";
    panel.dataset.page = page.id;
    panel.setAttribute("role", "tabpanel");
    panel.hidden = index !== 0;
    if (page.id === "information") {
      panel.append(
        renderModuleInfoReadonlyRow("Category", moduleInfoCategoryLabel(item)),
        renderModuleInfoReadonlyRow("Technical Name", item.technicalName),
        renderModuleInfoReadonlyRow("License", moduleInfoLicenseLabel(item)),
        renderModuleInfoReadonlyRow("Latest Version", moduleInfoLatestVersion(item))
      );
    } else {
      const technicalFields = document.createElement("div");
      technicalFields.className = "gorp-module-info-technical-fields";
      panel.append(
        technicalFields
      );
      technicalFields.append(
        renderModuleInfoBooleanRow("Demo Data", false),
        renderModuleInfoBooleanRow("Application", true),
        renderModuleInfoReadonlyRow("Status", moduleInfoStateLabel(item.state))
      );
      const relationTables = document.createElement("div");
      relationTables.className = "gorp-module-info-table-grid";
      relationTables.append(
        renderModuleInfoRelationTable("DEPENDENCIES", "dependencies", moduleInfoDependencies(item)),
        renderModuleInfoRelationTable("EXCLUSIONS", "exclusions", moduleInfoExclusions(item))
      );
      panel.append(relationTables);
    }
    tab.addEventListener("click", () => {
      for (const candidate of Array.from(nav.children) as HTMLElement[]) candidate.setAttribute("aria-selected", candidate === tab ? "true" : "false");
      for (const candidate of Array.from(pages.children) as HTMLElement[]) candidate.hidden = candidate.dataset.page !== page.id;
    });
    nav.append(tab);
    pages.append(panel);
  }
  notebook.append(nav, pages);
  return notebook;
}

function renderModuleInfoReadonlyRow(labelText: string, value: string): HTMLElement {
  const row = document.createElement("label");
  row.className = "gorp-form-field o_wrap_field gorp-module-info-field";
  const label = document.createElement("span");
  label.className = "o_form_label o_form_label_readonly";
  label.textContent = labelText;
  label.append(moduleInfoHelpMark());
  const output = document.createElement("output");
  output.className = "gorp-field-value o_field_widget o_readonly_modifier";
  output.textContent = value;
  row.append(label, output);
  return row;
}

function renderModuleInfoBooleanRow(labelText: string, checked: boolean): HTMLElement {
  const row = document.createElement("label");
  row.className = "gorp-form-field o_wrap_field gorp-module-info-field gorp-module-info-boolean-field";
  const label = document.createElement("span");
  label.className = "o_form_label o_form_label_readonly";
  label.textContent = labelText;
  label.append(moduleInfoHelpMark());
  const value = document.createElement("span");
  value.className = "o_field_boolean o_field_widget o_readonly_modifier form-check";
  const input = document.createElement("input");
  input.type = "checkbox";
  input.className = "form-check-input";
  input.disabled = true;
  input.checked = checked;
  input.dataset.field = slugID(labelText) || labelText.toLowerCase();
  input.setAttribute("aria-label", `${labelText}?`);
  const emptyLabel = document.createElement("span");
  emptyLabel.className = "form-check-label";
  value.append(input, emptyLabel);
  row.append(label, value);
  return row;
}

function renderModuleInfoRelationTable(title: string, field: string, rows: readonly string[]): HTMLElement {
  const section = document.createElement("section");
  section.className = "gorp-module-info-relation o_field_x2many_list";
  section.dataset.field = field;
  const heading = document.createElement("h3");
  heading.className = "gorp-module-info-relation-title";
  heading.textContent = title;
  const table = document.createElement("table");
  table.className = "o_list_table table table-sm table-hover position-relative mb-0 o_list_table_ungrouped table-striped";
  table.dataset.moduleInfoTable = field;
  const thead = document.createElement("thead");
  const header = document.createElement("tr");
  for (const column of ["Name", "Status"]) {
    const th = document.createElement("th");
    th.className = column === "Name" ? "align-middle o_column_sortable position-relative cursor-pointer opacity-trigger-hover w-print-auto" : "align-middle cursor-default opacity-trigger-hover w-print-auto";
    th.textContent = column;
    header.append(th);
  }
  thead.append(header);
  const tbody = document.createElement("tbody");
  const normalizedRows = rows.length ? rows : [""];
  for (const name of normalizedRows) {
    const tr = document.createElement("tr");
    tr.className = "o_data_row";
    const nameCell = document.createElement("td");
    nameCell.className = "o_data_cell cursor-pointer o_field_cell o_list_char o_readonly_modifier";
    nameCell.textContent = name;
    const statusCell = document.createElement("td");
    statusCell.className = "o_data_cell cursor-pointer o_field_cell o_readonly_modifier";
    statusCell.textContent = name ? "Not Installed" : "";
    tr.append(nameCell, statusCell);
    tbody.append(tr);
  }
  for (let index = normalizedRows.length; index < 4; index += 1) {
    const tr = document.createElement("tr");
    tr.className = "o_data_row gorp-module-info-empty-row";
    const cell = document.createElement("td");
    cell.colSpan = 2;
    tr.append(cell);
    tbody.append(tr);
  }
  table.append(thead, tbody);
  section.append(heading, table);
  return section;
}

function moduleInfoHelpMark(): HTMLElement {
  const help = document.createElement("sup");
  help.className = "gorp-module-info-help";
  help.textContent = "?";
  return help;
}

function moduleInfoCategoryLabel(item: ModuleCatalogItem): string {
  return item.category === "Accounting" ? "Invoicing" : item.category;
}

function moduleInfoLicenseLabel(_item: ModuleCatalogItem): string {
  return "LGPL Version 3";
}

function moduleInfoLatestVersion(_item: ModuleCatalogItem): string {
  return "19.0.1.0";
}

function moduleInfoStateLabel(state: string): string {
  switch (state) {
    case "installed":
      return "Installed";
    case "to install":
      return "To Install";
    case "to upgrade":
      return "To Upgrade";
    case "to remove":
      return "To Remove";
    default:
      return "Not Installed";
  }
}

function moduleInfoDependencies(item: ModuleCatalogItem): string[] {
  const category = item.category.toLowerCase();
  if (item.technicalName === "sale_management") return ["Sales", "CRM", "Invoicing"];
  if (item.technicalName === "equity") return ["portal"];
  if (category.includes("website")) return ["Website", "Base"];
  if (category.includes("shipping")) return ["Inventory", "Delivery"];
  if (category.includes("marketing")) return ["Mail", "Contacts"];
  return ["Base"];
}

function moduleInfoExclusions(_item: ModuleCatalogItem): string[] {
  return [];
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

const moduleCatalogDefinitions: readonly ModuleCatalogReference[] = [
  moduleCatalogReference(1, "Sales", "sale_management", "Sales", "From quotations to invoices"),
  moduleCatalogReference(2, "Restaurant", "pos_restaurant", "Sales", "Restaurant extensions for the Point of Sale", true),
  moduleCatalogReference(3, "Invoicing", "account", "Accounting", "Invoices & Payments"),
  moduleCatalogReference(4, "CRM", "crm", "Sales", "Track leads and close opportunities"),
  moduleCatalogReference(5, "Website", "website", "Website", "Enterprise website builder"),
  moduleCatalogReference(6, "Inventory", "stock", "Supply Chain", "Manage your stock and logistics activities"),
  moduleCatalogReference(7, "Accounting", "accountant", "Accounting", "Manage financial and analytic accounting"),
  moduleCatalogReference(8, "Equity", "equity", "Accounting", "Manage securities, transactions, and cap tables.", false, ""),
  moduleCatalogReference(9, "Purchase", "purchase", "Supply Chain", "Purchase orders, tenders and agreements"),
  moduleCatalogReference(10, "Point of Sale", "point_of_sale", "Sales", "Handle checkouts and payments for shops and restaurants.", true),
  moduleCatalogReference(11, "Project", "project", "Services", "Organize and plan your projects"),
  moduleCatalogReference(12, "eCommerce", "website_sale", "Website", "Sell your products online"),
  moduleCatalogReference(13, "Manufacturing", "mrp", "Supply Chain", "Manufacturing Orders & BOMs"),
  moduleCatalogReference(14, "Email Marketing", "mass_mailing", "Marketing", "Design, send and track emails"),
  moduleCatalogReference(15, "Timesheets", "timesheet_grid", "Services", "Track employee time on tasks"),
  moduleCatalogReference(16, "Expenses", "hr_expense", "Human Resources", "Submit, validate and reinvoice employee expenses"),
  moduleCatalogReference(17, "Studio", "web_studio", "Customizations", "Create and customize your Odoo apps"),
  moduleCatalogReference(18, "Documents", "documents", "Productivity", "Collect, organize and share documents."),
  moduleCatalogReference(19, "Time Off", "hr_holidays", "Human Resources", "Allocate time off and follow leave requests"),
  moduleCatalogReference(20, "Recruitment", "hr_recruitment", "Human Resources", "Track your recruitment pipeline"),
  moduleCatalogReference(21, "Employees", "hr", "Human Resources", "Centralize employee information"),
  moduleCatalogReference(22, "AI", "ai", "Productivity", "AI assistants and tools"),
  moduleCatalogReference(23, "Data Recycle", "data_recycle", "Technical", "Recycle duplicate records"),
  moduleCatalogReference(24, "Databases", "databases", "Administration", "Database administration"),
  moduleCatalogReference(25, "Subscriptions", "sale_subscription", "Sales", "Recurring sales"),
  moduleCatalogReference(26, "Rental", "sale_renting", "Sales", "Rent products"),
  moduleCatalogReference(27, "Field Service", "industry_fsm", "Sales", "On-site service work", true),
  moduleCatalogReference(28, "Sales Planning", "sale_planning", "Sales", "Sales planning"),
  moduleCatalogReference(29, "Sales Commission", "sale_commission", "Sales", "Commission plans"),
  moduleCatalogReference(30, "Loyalty", "loyalty", "Sales", "Coupons and loyalty programs"),
  moduleCatalogReference(31, "Event Sale", "event_sale", "Sales", "Sell event tickets"),
  moduleCatalogReference(32, "eLearning", "website_slides", "Website", "Online courses"),
  moduleCatalogReference(33, "Blog", "website_blog", "Website", "Publish articles"),
  moduleCatalogReference(34, "Forum", "website_forum", "Website", "Community forum"),
  moduleCatalogReference(35, "Helpdesk", "helpdesk", "Services", "Tickets and support", true),
  moduleCatalogReference(36, "Planning", "planning", "Services", "Resource planning"),
  moduleCatalogReference(37, "Appointments", "appointment", "Services", "Online appointments"),
  moduleCatalogReference(38, "Repairs", "repair", "Services", "Repair orders"),
  moduleCatalogReference(39, "Barcode", "barcode", "Supply Chain", "Barcode operations"),
  moduleCatalogReference(40, "Quality", "quality_control", "Supply Chain", "Quality checks"),
  moduleCatalogReference(41, "Maintenance", "maintenance", "Supply Chain", "Equipment maintenance"),
  moduleCatalogReference(42, "PLM", "mrp_plm", "Supply Chain", "Product lifecycle"),
  moduleCatalogReference(43, "Dropshipping", "stock_dropshipping", "Supply Chain", "Dropship deliveries"),
  moduleCatalogReference(44, "Spreadsheet", "spreadsheet", "Productivity", "Collaborative spreadsheets"),
  moduleCatalogReference(45, "Knowledge", "knowledge", "Productivity", "Knowledge base"),
  moduleCatalogReference(46, "Discuss", "mail", "Productivity", "Team messaging"),
  moduleCatalogReference(47, "Calendar", "calendar", "Productivity", "Meetings and calendars"),
  moduleCatalogReference(48, "Contacts", "contacts", "Productivity", "Contacts directory"),
  moduleCatalogReference(49, "Dashboards", "spreadsheet_dashboard", "Productivity", "Business dashboards"),
  moduleCatalogReference(50, "Sign", "sign", "Productivity", "Electronic signatures"),
  moduleCatalogReference(51, "Amazon Delivery", "delivery_amazon", "Shipping Connectors", "Amazon delivery connector"),
  moduleCatalogReference(52, "Marketing Automation", "marketing_automation", "Marketing", "Automated campaigns"),
  moduleCatalogReference(53, "SMS Marketing", "sms", "Marketing", "SMS campaigns"),
  moduleCatalogReference(54, "Social Marketing", "social", "Marketing", "Social campaigns"),
  moduleCatalogReference(55, "Events", "event", "Marketing", "Events and attendees"),
  moduleCatalogReference(56, "Surveys", "survey", "Marketing", "Forms and surveys"),
  moduleCatalogReference(57, "Live Chat", "im_livechat", "Marketing", "Website live chat"),
  moduleCatalogReference(58, "Attendance", "hr_attendance", "Human Resources", "Employee attendance"),
  moduleCatalogReference(59, "Appraisals", "hr_appraisal", "Human Resources", "Performance reviews"),
  moduleCatalogReference(60, "Referrals", "hr_referral", "Human Resources", "Employee referrals"),
  moduleCatalogReference(61, "Fleet", "fleet", "Human Resources", "Vehicle fleet"),
  moduleCatalogReference(62, "Payroll", "hr_payroll", "Human Resources", "Payroll management"),
  moduleCatalogReference(63, "Lunch", "lunch", "Human Resources", "Lunch orders"),
  moduleCatalogReference(64, "Skills", "hr_skills", "Human Resources", "Employee skills"),
  moduleCatalogReference(65, "Employee Contracts", "hr_contract", "Human Resources", "Contracts"),
  moduleCatalogReference(66, "Frontdesk", "frontdesk", "Human Resources", "Visitor reception"),
  moduleCatalogReference(67, "Employee Presence", "hr_presence", "Human Resources", "Presence status"),
  moduleCatalogReference(68, "UPS Shipping", "delivery_ups", "Shipping Connectors", "UPS delivery connector"),
  moduleCatalogReference(69, "FedEx Shipping", "delivery_fedex", "Shipping Connectors", "FedEx delivery connector"),
  moduleCatalogReference(70, "DHL Shipping", "delivery_dhl", "Shipping Connectors", "DHL delivery connector"),
  moduleCatalogReference(71, "USPS Shipping", "delivery_usps", "Shipping Connectors", "USPS delivery connector"),
  moduleCatalogReference(72, "bpost Shipping", "delivery_bpost", "Shipping Connectors", "bpost delivery connector"),
  moduleCatalogReference(73, "Easypost Shipping", "delivery_easypost", "Shipping Connectors", "Easypost delivery connector"),
  moduleCatalogReference(74, "Sendcloud Shipping", "delivery_sendcloud", "Shipping Connectors", "Sendcloud delivery connector"),
  moduleCatalogReference(75, "Shiprocket Shipping", "delivery_shiprocket", "Shipping Connectors", "Shiprocket delivery connector"),
  moduleCatalogReference(76, "Starshipit Shipping", "delivery_starshipit", "Shipping Connectors", "Starshipit delivery connector"),
  moduleCatalogReference(77, "ESG", "sustainability", "ESG", "Sustainability reporting")
];

const moduleCatalogCategoryOrder = [
  "Sales",
  "Website",
  "Services",
  "Accounting",
  "Supply Chain",
  "Productivity",
  "Marketing",
  "Human Resources",
  "Shipping Connectors",
  "ESG",
  "Customizations",
  "Administration",
  "Technical"
];

function moduleCatalogReference(
  sequence: number,
  displayName: string,
  technicalName: string,
  category: string,
  summary: string,
  industry = false,
  website?: string
): ModuleCatalogReference {
  return { category, displayName, industry, official: true, sequence, summary, technicalName, website };
}

function moduleCatalogItems(records: readonly Record<string, unknown>[]): ModuleCatalogItem[] {
  const real = records.map(moduleCatalogItemFromRecord).sort(moduleCatalogItemSort);
  if (real.length >= 20 && real.length < moduleCatalogDefinitions.length) {
    const realByName = new Map(real.map((item) => [item.technicalName, item]));
    return moduleCatalogDefinitions.map((definition) => {
      const source = realByName.get(definition.technicalName);
      return moduleCatalogItemFromReference(definition, source);
    }).sort(moduleCatalogItemSort);
  }
  return real;
}

function moduleCatalogItemFromReference(definition: ModuleCatalogReference, source?: ModuleCatalogItem): ModuleCatalogItem {
  const description = source?.description || definition.summary;
  const website = source?.website || (definition.website ?? `https://www.odoo.com/app/${encodeURIComponent(definition.technicalName)}`);
  return {
    id: source?.id,
    category: definition.category,
    description,
    displayName: definition.displayName,
    industry: definition.industry === true,
    installable: source?.installable ?? true,
    official: definition.official !== false,
    searchText: moduleCatalogSearchText(definition.displayName, definition.technicalName, definition.category, definition.summary, description),
    sequence: definition.sequence,
    sourceRecord: source?.sourceRecord,
    state: source?.state ?? "uninstalled",
    summary: definition.summary,
    technicalName: definition.technicalName,
    virtual: !source,
    website
  };
}

function moduleCatalogItemFromRecord(record: Record<string, unknown>, index: number): ModuleCatalogItem {
  const technicalName = firstText(record.name, record.technical_name, record.display_name, record.id) || `module_${index + 1}`;
  const displayName = firstText(record.shortdesc, record.display_name, moduleCatalogDisplayName(technicalName)) || technicalName;
  const category = firstText(record.category, moduleCatalogCategoryForName(technicalName, displayName)) || "Technical";
  const summary = firstText(record.summary, record.description, category) || category;
  const description = firstText(record.description, record.summary, summary) || summary;
  const state = firstText(record.state, "uninstalled") || "uninstalled";
  const installable = firstValue(record.installable) !== false;
  return {
    id: numberRecordID(record.id),
    category,
    description,
    displayName,
    industry: firstValue(record.application) === true && category !== "Technical",
    installable,
    official: installable,
    searchText: moduleCatalogSearchText(displayName, technicalName, category, summary, description),
    sequence: 1000 + index,
    sourceRecord: record,
    state,
    summary,
    technicalName,
    virtual: false,
    website: firstText(record.website, "") || ""
  };
}

function moduleCatalogSearchText(...parts: readonly string[]): string {
  return parts.join(" ").toLowerCase();
}

function moduleCatalogItemSort(left: ModuleCatalogItem, right: ModuleCatalogItem): number {
  return left.sequence - right.sequence || left.displayName.localeCompare(right.displayName) || left.technicalName.localeCompare(right.technicalName);
}

function moduleCatalogCategoryVisible(category: string): boolean {
  return Boolean(category);
}

function moduleCatalogCategorySort(left: string, right: string): number {
  const leftIndex = moduleCatalogCategoryOrder.indexOf(left);
  const rightIndex = moduleCatalogCategoryOrder.indexOf(right);
  if (leftIndex >= 0 && rightIndex >= 0) return leftIndex - rightIndex;
  if (leftIndex >= 0) return -1;
  if (rightIndex >= 0) return 1;
  return left.localeCompare(right);
}

function moduleCatalogDisplayName(name: string): string {
  const cleaned = String(name || "").replace(/^oi_/, "").replace(/^base_/, "").replace(/_/g, " ").replace(/\s+/g, " ").trim();
  return cleaned ? cleaned.split(" ").map((part) => part.slice(0, 1).toUpperCase() + part.slice(1)).join(" ") : "Module";
}

function moduleCatalogCategoryForName(technicalName: string, displayName: string): string {
  const name = `${technicalName} ${displayName}`.toLowerCase();
  if (/(sale|crm|pos|loyalty|rental|subscription|commission)/.test(name)) return "Sales";
  if (/(website|ecommerce|blog|forum|slides)/.test(name)) return "Website";
  if (/(project|timesheet|helpdesk|planning|appointment|repair|service)/.test(name)) return "Services";
  if (/(account|invoice|equity)/.test(name)) return "Accounting";
  if (/(stock|inventory|purchase|mrp|barcode|quality|maintenance|plm|dropship)/.test(name)) return "Supply Chain";
  if (/(mail|document|knowledge|calendar|contact|spreadsheet|sign|ai)/.test(name)) return "Productivity";
  if (/(marketing|sms|social|event|survey|chat)/.test(name)) return "Marketing";
  if (/(hr|employee|recruit|attendance|appraisal|fleet|payroll|lunch|skill|frontdesk|presence)/.test(name)) return "Human Resources";
  if (/(delivery|shipping|ups|fedex|dhl|usps|bpost|easypost|sendcloud|shiprocket|starshipit)/.test(name)) return "Shipping Connectors";
  if (/esg|sustain/.test(name)) return "ESG";
  if (/studio|custom/.test(name)) return "Customizations";
  if (/database|admin/.test(name)) return "Administration";
  return "Technical";
}

function moduleCatalogIconElement(item: ModuleCatalogItem): HTMLImageElement {
  const icon = document.createElement("img");
  icon.className = "app-icon o_app_icon o_module_icon";
  icon.setAttribute("style", "flex:0 0 50px !important;width:50px !important;height:50px !important;object-fit:contain !important;border-radius:6px !important;");
  icon.alt = "";
  icon.src = moduleCatalogIconSource(item);
  icon.dataset.generatedIcon = "clean-room";
  icon.dataset.iconKind = slugID(item.technicalName) || moduleCatalogIconToken(item);
  icon.setAttribute("aria-hidden", "true");
  return icon;
}

function moduleCatalogIconSource(item: ModuleCatalogItem): string {
  const palette = moduleCatalogIconPalette(item);
  const kind = slugID(item.technicalName);
  return moduleCatalogSVGDataURI(`
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 56 56">
      <rect width="56" height="56" rx="6" fill="${palette.bg}"/>
      <rect x="1" y="1" width="54" height="54" rx="6" fill="none" stroke="#fff" stroke-opacity=".16"/>
      ${moduleCatalogIconMark(kind, palette)}
    </svg>
  `);
}

function moduleCatalogIconPalette(item: ModuleCatalogItem): { bg: string; a: string; b: string; c: string; ink: string } {
  const palettes: Record<string, { bg: string; a: string; b: string; c: string; ink: string }> = {
    accounting: { bg: "#2287c9", a: "#38d3dc", b: "#f5f7fb", c: "#253448", ink: "#ffffff" },
    administration: { bg: "#c96f42", a: "#f6bf59", b: "#875a7b", c: "#273447", ink: "#ffffff" },
    customizations: { bg: "#875a7b", a: "#38d3dc", b: "#f6bf59", c: "#f5f7fb", ink: "#ffffff" },
    esg: { bg: "#25805e", a: "#79d4a5", b: "#f1c65b", c: "#f5f7fb", ink: "#ffffff" },
    hr: { bg: "#d79533", a: "#f6cf67", b: "#38c5be", c: "#875a7b", ink: "#ffffff" },
    inventory: { bg: "#c36a42", a: "#f6bf59", b: "#875a7b", c: "#38c5be", ink: "#ffffff" },
    marketing: { bg: "#1c8fa2", a: "#42d0ca", b: "#875a7b", c: "#f5f7fb", ink: "#ffffff" },
    productivity: { bg: "#714b67", a: "#38d3dc", b: "#f6bf59", c: "#f5f7fb", ink: "#ffffff" },
    sales: { bg: "#cf7642", a: "#f6bf59", b: "#875a7b", c: "#38c5be", ink: "#ffffff" },
    services: { bg: "#1f9d78", a: "#52d5c7", b: "#875a7b", c: "#f5f7fb", ink: "#ffffff" },
    shipping: { bg: "#168e91", a: "#52d5c7", b: "#f6bf59", c: "#f5f7fb", ink: "#ffffff" },
    technical: { bg: "#456184", a: "#9fb4d2", b: "#f6bf59", c: "#f5f7fb", ink: "#ffffff" },
    website: { bg: "#159b93", a: "#54d4c8", b: "#2d7fbe", c: "#f5f7fb", ink: "#ffffff" }
  };
  return palettes[moduleCatalogIconToken(item)] ?? palettes.productivity;
}

function moduleCatalogIconToken(item: ModuleCatalogItem): string {
  const tokens: Record<string, string> = {
    Accounting: "accounting",
    Administration: "administration",
    Customizations: "customizations",
    ESG: "esg",
    "Human Resources": "hr",
    Marketing: "marketing",
    Productivity: "productivity",
    Sales: "sales",
    Services: "services",
    "Shipping Connectors": "shipping",
    "Supply Chain": "inventory",
    Technical: "technical",
    Website: "website"
  };
  return tokens[item.category] || "productivity";
}

function moduleCatalogIconMark(kind: string, palette: { a: string; b: string; c: string; ink: string }): string {
  if (kind === "sale-management" || kind === "crm") {
    return `<path d="M11 39 23 27l8 6 14-17" fill="none" stroke="${palette.ink}" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/><circle cx="23" cy="27" r="5" fill="${palette.a}"/><circle cx="45" cy="16" r="5" fill="${palette.b}"/>`;
  }
  if (kind === "account" || kind === "accountant" || kind === "equity") {
    return `<circle cx="22" cy="31" r="12" fill="${palette.a}"/><circle cx="34" cy="25" r="12" fill="${palette.b}" opacity=".92"/><path d="M15 42h27" stroke="${palette.ink}" stroke-width="5" stroke-linecap="round"/>`;
  }
  if (kind.includes("website")) {
    return `<rect x="10" y="15" width="36" height="28" rx="4" fill="${palette.c}"/><path d="M10 23h36" stroke="${palette.a}" stroke-width="5"/><circle cx="18" cy="19" r="2" fill="${palette.b}"/><circle cx="25" cy="19" r="2" fill="${palette.b}"/>`;
  }
  if (kind === "stock" || kind === "purchase" || kind === "mrp" || kind === "barcode") {
    return `<path d="M12 20 28 11l16 9v18l-16 9-16-9Z" fill="${palette.a}"/><path d="M12 20 28 29l16-9M28 29v18" fill="none" stroke="${palette.ink}" stroke-width="4" stroke-linejoin="round"/>`;
  }
  if (kind === "project" || kind === "planning" || kind === "timesheet-grid") {
    return `<rect x="12" y="14" width="32" height="28" rx="4" fill="${palette.c}"/><path d="M18 23h20M18 31h14M18 39h18" stroke="${palette.a}" stroke-width="4" stroke-linecap="round"/>`;
  }
  if (kind === "mail" || kind === "mass-mailing" || kind === "sms") {
    return `<path d="M10 18h36v25H10Z" fill="${palette.c}"/><path d="m10 19 18 15 18-15" fill="none" stroke="${palette.a}" stroke-width="4" stroke-linejoin="round"/>`;
  }
  if (kind.startsWith("hr") || kind.includes("employee") || kind.includes("recruit")) {
    return `<circle cx="22" cy="22" r="8" fill="${palette.b}"/><circle cx="36" cy="24" r="7" fill="${palette.a}"/><path d="M12 43c3-8 9-12 17-12s14 4 17 12Z" fill="${palette.c}"/>`;
  }
  return `<rect x="13" y="13" width="20" height="20" rx="5" fill="${palette.c}"/><rect x="23" y="23" width="20" height="20" rx="5" fill="${palette.a}" opacity=".9"/><path d="M17 38h22" stroke="${palette.b}" stroke-width="5" stroke-linecap="round"/>`;
}

function moduleCatalogSVGDataURI(svg: string): string {
  return `data:image/svg+xml,${encodeURIComponent(svg.replace(/\s+/g, " ").trim())}`;
}

function moduleCatalogStyleElement(): HTMLElement {
  const style = document.createElement("style");
  style.textContent = `
    .gorp-window-action[data-model="ir.module.module"] { background: #262a36 !important; color: #e8e9ef !important; min-height: calc(100vh - 104px); }
    .gorp-window-action[data-model="ir.module.module"] > .o_control_panel { background: #262a36 !important; border-bottom: 1px solid #3a3f4e !important; color: #f4f5f7 !important; }
    .gorp-window-action[data-model="ir.module.module"] .o_breadcrumb .breadcrumb-item { color: #f4f5f7 !important; }
    .gorp-window-action[data-model="ir.module.module"] .o_searchview { background: #252a36 !important; border-color: #00a09d !important; color: #e8e9ef !important; }
    .gorp-window-action[data-model="ir.module.module"] .o_searchview_input { color: #e8e9ef !important; background: transparent !important; }
    .gorp-window-action[data-model="ir.module.module"] .o_searchview_input::placeholder { color: #9ca3af !important; }
    .gorp-apps-catalog-content { display: flex !important; min-height: calc(100vh - 104px); background: #262a36 !important; overflow: hidden; }
    .gorp-apps-catalog-sidebar { flex: 0 0 220px !important; width: 220px !important; box-sizing: border-box !important; padding: 18px 16px 28px !important; border-right: 1px solid #3a3f4e !important; overflow-y: auto; color: #e8e9ef !important; background: #262a36 !important; }
    .gorp-apps-catalog-sidebar .o_search_panel_section_header { margin: 12px 0 8px !important; font-size: 14px !important; font-weight: 700 !important; color: #f4f5f7 !important; }
    .gorp-apps-catalog-sidebar .o_search_panel_section_header::before { content: ""; display: inline-block; width: 13px; height: 10px; margin-right: 8px; border-radius: 2px; background: #c060a1; }
    .gorp-apps-catalog-sidebar button { width: 100% !important; display: flex !important; align-items: center !important; justify-content: space-between !important; border: 0 !important; border-radius: 0 !important; background: transparent !important; color: #f4f5f7 !important; padding: 4px 16px !important; min-height: 28px !important; text-align: left !important; font-weight: 500 !important; opacity: 1 !important; }
    .gorp-apps-catalog-sidebar button.active { background: #3c414f !important; }
    .gorp-apps-catalog-sidebar .o_search_panel_counter { color: #aeb4c2 !important; font-weight: 600 !important; }
    .gorp-apps-catalog-grid { flex: 1 1 auto !important; display: grid !important; grid-template-columns: repeat(3, minmax(260px, 1fr)) !important; gap: 8px 16px !important; align-content: start !important; padding: 14px 16px 40px !important; overflow-y: auto; background: #262a36 !important; }
    .gorp-apps-catalog-card.o_kanban_record { position: relative !important; display: flex !important; flex-direction: row !important; align-items: center !important; gap: 14px !important; min-width: 0 !important; min-height: 94px !important; height: 94px !important; margin: 0 !important; padding: 10px 30px 10px 12px !important; background: #2a2f3b !important; border: 1px solid #454a59 !important; border-radius: 0 !important; box-shadow: none !important; color: #e8e9ef !important; overflow: hidden !important; }
    .gorp-apps-catalog-card .o_module_icon_container { flex: 0 0 58px !important; width: 58px !important; height: 58px !important; display: flex !important; align-items: center !important; justify-content: center !important; }
    .gorp-apps-catalog-card .o_module_icon { flex: 0 0 50px !important; width: 50px !important; height: 50px !important; object-fit: contain !important; border-radius: 6px !important; }
    .gorp-apps-catalog-card .o_app_details { min-width: 0 !important; flex: 1 1 auto !important; display: flex !important; flex-direction: column !important; align-self: stretch !important; justify-content: center !important; gap: 1px !important; overflow: hidden !important; }
    .gorp-apps-catalog-card .o_app_name { position: static !important; display: block !important; width: auto !important; margin: 0 !important; padding: 0 !important; color: #f2f3f6 !important; font-size: 15px !important; line-height: 19px !important; font-weight: 700 !important; text-align: left !important; white-space: nowrap !important; overflow: hidden !important; text-overflow: ellipsis !important; opacity: 1 !important; transform: none !important; }
    .gorp-apps-catalog-card .o_app_summary { position: static !important; display: block !important; margin: 0 !important; color: #aeb4c2 !important; font-size: 13px !important; line-height: 16px !important; max-height: 34px !important; overflow: hidden !important; opacity: 1 !important; }
    .gorp-apps-catalog-card .o_module_technical_name { color: #d889c1 !important; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace !important; letter-spacing: 0 !important; }
    .gorp-apps-catalog-card .o_module_actions { position: static !important; display: flex !important; align-items: center !important; gap: 7px !important; margin-top: 5px !important; opacity: 1 !important; }
    .gorp-apps-catalog-card .btn { min-height: 24px !important; padding: 3px 10px !important; border-radius: 3px !important; font-size: 12px !important; line-height: 16px !important; font-weight: 700 !important; opacity: 1 !important; }
    .gorp-apps-catalog-card .btn-primary, .gorp-apps-catalog-card .o_module_install_button { background: #875a7b !important; border-color: #875a7b !important; color: #fff !important; }
    .gorp-apps-catalog-card .btn-secondary, .gorp-apps-catalog-card .o_module_info_button { background: #3d4352 !important; border-color: #3d4352 !important; color: #f4f5f7 !important; }
    .gorp-apps-catalog-card .o_module_menu { position: absolute !important; top: 8px !important; right: 7px !important; padding: 0 !important; min-width: 16px !important; border: 0 !important; color: #aeb4c2 !important; background: transparent !important; font-size: 20px !important; line-height: 16px !important; opacity: 1 !important; }
    .gorp-module-info-body { width: 100% !important; max-width: none !important; margin: 0 !important; background: #1b1d26 !important; color: #e4e4e4 !important; }
    .gorp-module-info-sheet { position: relative !important; max-width: none !important; margin: 10px 16px 0 !important; padding: 24px !important; background: #262a36 !important; border: 1px solid #3a3f4e !important; color: #e4e4e4 !important; box-shadow: none !important; }
    .gorp-module-info-title { min-height: 138px !important; padding-right: 116px !important; }
    .gorp-module-info-title h1 { margin: 0 0 8px !important; color: #fff !important; font-size: 33px !important; line-height: 40px !important; font-weight: 700 !important; }
    .gorp-module-info-title .o_module_summary { margin: 0 0 12px !important; color: #b1b3bc !important; font-size: 19px !important; line-height: 24px !important; font-weight: 700 !important; }
    .gorp-module-info-title .o_module_author { margin: 0 0 14px !important; color: #b1b3bc !important; font-size: 15px !important; font-weight: 700 !important; }
    .gorp-module-info-title .o_module_install_button { min-height: 33px !important; padding: 5px 10px !important; border-radius: 4px !important; background: #875a7b !important; border-color: #875a7b !important; color: #fff !important; font-size: 14px !important; line-height: 21px !important; font-weight: 700 !important; }
    .gorp-module-info-icon { position: absolute !important; top: 24px !important; right: 24px !important; width: 88px !important; height: 88px !important; border-radius: 0 !important; background: #f8f9fa !important; object-fit: contain !important; }
    .gorp-module-info-notebook { margin: 0 -24px -24px !important; border-top: 1px solid #3a3f4e !important; }
    .gorp-module-info-notebook .gorp-form-notebook-tabs { padding-left: 24px !important; background: #262a36 !important; border-bottom: 1px solid #3a3f4e !important; }
    .gorp-module-info-notebook .gorp-form-notebook-tab { min-height: 39px !important; padding: 8px 16px !important; border: 0 !important; border-radius: 0 !important; background: transparent !important; color: #e4e4e4 !important; }
    .gorp-module-info-notebook .gorp-form-notebook-tab[aria-selected="true"] { color: #d889c1 !important; background: #303442 !important; box-shadow: inset 0 3px 0 #d889c1 !important; }
    .gorp-module-info-notebook .gorp-form-notebook-page { padding: 16px 24px 24px !important; background: #262a36 !important; color: #e4e4e4 !important; }
    .gorp-module-info-field { display: grid !important; grid-template-columns: 130px minmax(0, 1fr) !important; align-items: center !important; min-height: 34px !important; margin: 0 !important; color: #e4e4e4 !important; }
    .gorp-module-info-field .o_form_label { color: #b1b3bc !important; font-size: 14px !important; line-height: 21px !important; font-weight: 700 !important; }
    .gorp-module-info-field .o_field_widget { color: #fff !important; font-size: 14px !important; line-height: 21px !important; }
    .gorp-module-info-help { margin-left: 4px !important; color: #5e9fdb !important; font-size: 11px !important; line-height: 1 !important; }
    .gorp-module-info-boolean-field .form-check { min-height: 21px !important; padding-left: 0 !important; }
    .gorp-module-info-boolean-field input[type="checkbox"] { width: 14px !important; height: 14px !important; margin: 0 !important; accent-color: #00a09d !important; }
    .gorp-module-info-table-grid { display: grid !important; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr) !important; gap: 32px !important; margin-top: 28px !important; }
    .gorp-module-info-relation-title { margin: 0 0 8px !important; padding-bottom: 6px !important; border-bottom: 1px solid #3a3f4e !important; color: #f4f5f7 !important; font-size: 13px !important; line-height: 18px !important; font-weight: 800 !important; }
    .gorp-module-info-relation table { width: 100% !important; border-collapse: collapse !important; color: #e4e4e4 !important; background: transparent !important; }
    .gorp-module-info-relation th { height: 38px !important; padding: 8px 16px !important; background: #262a36 !important; border: 0 !important; color: #fff !important; font-size: 14px !important; line-height: 21px !important; font-weight: 700 !important; text-align: left !important; }
    .gorp-module-info-relation td { height: 38px !important; padding: 8px 16px !important; background: #262a36 !important; border-top: 1px solid #353a48 !important; color: #e4e4e4 !important; font-size: 14px !important; line-height: 21px !important; }
    @media (max-width: 900px) {
      .gorp-apps-catalog-content { display: block; overflow: auto; }
      .gorp-apps-catalog-sidebar { width: auto; border-right: 0; border-bottom: 1px solid #3a3f4e; }
      .gorp-apps-catalog-grid { grid-template-columns: 1fr; padding: 10px; }
      .gorp-module-info-table-grid { grid-template-columns: 1fr !important; }
    }
  `;
  return style;
}

function applyModuleKanbanCardMetadata(card: HTMLElement, record: Record<string, unknown>): void {
  card.className = `${card.className} gorp-apps-catalog-card module-card o_app`;
  const technicalName = String(firstText(record.name, record.technical_name, record.display_name, record.id) || "");
  const displayName = String(firstText(record.shortdesc, record.display_name, record.name, record.id) || "");
  const state = String(firstText(record.state, "uninstalled") || "uninstalled");
  if (technicalName) card.dataset.moduleName = technicalName;
  if (displayName) card.dataset.appName = displayName;
  card.dataset.state = state;
}

function renderModuleKanbanActions(
  record: Record<string, unknown>,
  id: number,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): HTMLElement {
  const wrapper = document.createElement("div");
  wrapper.className = "o_module_actions";
  const state = String(firstText(record.state, "uninstalled") || "uninstalled");
  const stateBadge = document.createElement("span");
  stateBadge.className = "badge o_module_state";
  stateBadge.textContent = state;
  wrapper.append(stateBadge);
  for (const item of moduleKanbanActionItems(state)) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = item.className;
    button.dataset.moduleAction = item.method;
    button.textContent = item.label;
    button.addEventListener("click", async (event) => {
      event.preventDefault?.();
      event.stopPropagation?.();
      button.disabled = true;
      button.textContent = item.runningLabel;
      await runModuleKanbanAction(id, item.method, action, options, root);
    });
    wrapper.append(button);
  }
  wrapper.append(renderModuleInfoButton(record, id, action, options, root));
  return wrapper;
}

type ModuleKanbanActionMethod =
  | "button_immediate_install"
  | "button_immediate_upgrade"
  | "button_immediate_uninstall"
  | "button_cancel_install"
  | "button_cancel_upgrade"
  | "button_cancel_uninstall";

function moduleKanbanActionItems(state: string): Array<{ className: string; label: string; method: ModuleKanbanActionMethod; runningLabel: string }> {
  switch (state) {
    case "installed":
      return [
        { className: "btn btn-secondary btn-sm o_module_upgrade_button", label: "Upgrade", method: "button_immediate_upgrade", runningLabel: "Upgrading..." },
        { className: "btn btn-outline-secondary btn-sm o_module_uninstall_button", label: "Uninstall", method: "button_immediate_uninstall", runningLabel: "Uninstalling..." }
      ];
    case "to install":
      return [{ className: "btn btn-outline-secondary btn-sm o_module_cancel_button", label: "Cancel Install", method: "button_cancel_install", runningLabel: "Cancelling..." }];
    case "to upgrade":
      return [{ className: "btn btn-outline-secondary btn-sm o_module_cancel_button", label: "Cancel Upgrade", method: "button_cancel_upgrade", runningLabel: "Cancelling..." }];
    case "to remove":
      return [{ className: "btn btn-outline-secondary btn-sm o_module_cancel_button", label: "Cancel Uninstall", method: "button_cancel_uninstall", runningLabel: "Cancelling..." }];
    default:
      return [{ className: "btn btn-primary btn-sm o_module_install_button", label: "Activate", method: "button_immediate_install", runningLabel: "Activating..." }];
  }
}

async function runModuleKanbanAction(
  id: number,
  method: ModuleKanbanActionMethod,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): Promise<void> {
  if (options.services?.orm && options.services?.action) {
    await options.services.orm.call("ir.module.module", method, [[id]], {});
    await options.services.action.doAction(action, replaceActionOptions(options));
    return;
  }
  root.dispatchEvent(new CustomEvent("action:module-action", {
    bubbles: true,
    detail: { model: "ir.module.module", id, method }
  }));
}

function renderModuleInfoButton(
  record: Record<string, unknown>,
  id: number,
  action: Record<string, unknown>,
  options: RenderWindowActionOptions,
  root: HTMLElement
): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-secondary btn-sm o_module_info_button";
  button.dataset.moduleInfo = String(record.name ?? id);
  button.textContent = "Module Info";
  button.addEventListener("click", async (event) => {
    event.preventDefault?.();
    event.stopPropagation?.();
    const formView = formViewRef(action) ?? [false, "form"];
    const nextAction: Record<string, unknown> = {
      ...action,
      name: "Module Info",
      res_id: id,
      res_model: "ir.module.module",
      view_mode: "form",
      views: [[formView[0], "form"]],
      target: "new"
    };
    const context = {
      ...(options.context ?? {}),
      active_model: "ir.module.module",
      active_id: id,
      active_ids: [id]
    };
    if (options.services?.action) {
      await options.services.action.doAction(nextAction, { additionalContext: context });
      return;
    }
    root.dispatchEvent(new CustomEvent("action:open-record", {
      bubbles: true,
      detail: { action: nextAction, model: "ir.module.module", id }
    }));
  });
  return button;
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

function listViewFieldNodes(
  arch: string,
  fields: Record<string, unknown>,
  evalContext: Record<string, unknown>,
  model?: string
): ViewFieldNode[] {
  const nodes = parseViewFieldNodes(arch).filter((node) => !fieldInvisible(node, evalContext));
  if (shouldUseDefaultModelFieldNodes(model, nodes, "list")) return defaultViewFieldNodes(model, fields, "list");
  if (nodes.length) return nodes;
  return defaultViewFieldNodes(model, fields, "list");
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
  const sections: HTMLElement[] = [];
  const lifecycle: ActionMenuSectionLifecycle = {
    onBeforeOpen(section) {
      closeActionMenuSections(sections, section);
    }
  };
  const printItems = actionMenuRecords(params.actionMenus, "print");
  if (printItems.length) {
    const section = renderPrintActionMenuSection(
      params.model,
      printItems,
      params.getActiveIds,
      params.requiresSelection,
      params.root,
      params.options,
      lifecycle
    );
    sections.push(section);
    toolbar.append(section);
  }
  const actionButtons = [
    ...params.staticActionButtons,
    ...actionMenuRecords(params.actionMenus, "action").map((item) =>
      renderServerActionMenuButton("action", params.model, item, params.getActiveIds, params.requiresSelection, params.root, params.options)
    )
  ].sort(compareActionMenuButtons);
  if (actionButtons.length) {
    const section = renderActionMenuSection("action", "Actions", "fa fa-cog", actionButtons, lifecycle);
    sections.push(section);
    toolbar.append(section);
  }
  bindActionMenuToolbarLifecycle(toolbar, sections);
  return toolbar;
}

function renderPrintActionMenuSection(
  model: string,
  printItems: readonly ActionMenuRecord[],
  getActiveIds: () => number[],
  requiresSelection: boolean,
  root: HTMLElement,
  options: RenderWindowActionOptions,
  lifecycle: ActionMenuSectionLifecycle = {}
): HTMLElement {
  const section = renderActionMenuSection("print", "Print", "fa fa-print", [], {
    ...lifecycle,
    async beforeOpen() {
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
    }
  });
  const menu = section.children[1] as HTMLElement;
  return section;
}

interface ActionMenuSectionLifecycle {
  onBeforeOpen?: (section: HTMLElement) => void;
  beforeOpen?: (section: HTMLElement) => void | Promise<void>;
  onOpen?: (section: HTMLElement) => void | Promise<void>;
}

interface ActionMenuOpenOptions {
  focusFirst?: boolean;
  restoreFocusElement?: HTMLElement;
}

interface ActionMenuCloseOptions {
  restoreFocus?: boolean;
}

const actionMenuRestoreFocus = new WeakMap<HTMLElement, HTMLElement>();
const actionMenuOpenHandlers = new WeakMap<HTMLElement, (options?: ActionMenuOpenOptions) => Promise<void>>();

function renderActionMenuSection(kind: string, title: string, iconClass: string, items: readonly HTMLElement[], lifecycle: ActionMenuSectionLifecycle = {}): HTMLElement {
  const section = document.createElement("div");
  section.className = "gorp-action-menu-section";
  section.dataset.menu = kind;
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "gorp-action-menu-toggle";
  toggle.dataset.actionMenuToggle = kind;
  toggle.setAttribute("aria-haspopup", "menu");
  toggle.setAttribute("aria-expanded", "false");
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
  actionMenuOpenHandlers.set(section, (options = {}) => toggleActionMenuSection(section, toggle, true, lifecycle, options));
  toggle.addEventListener("click", (event) => {
    event.preventDefault?.();
    void toggleActionMenuSection(section, toggle, !actionMenuOpen(section), lifecycle, { restoreFocusElement: toggle });
  });
  toggle.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      event.preventDefault?.();
      closeActionMenuSection(section, { restoreFocus: true });
      return;
    }
    if (event.key === "ArrowDown" || event.key === "Enter" || event.key === " ") {
      event.preventDefault?.();
      void toggleActionMenuSection(section, toggle, true, lifecycle, { focusFirst: true, restoreFocusElement: toggle });
    }
  });
  menu.addEventListener("keydown", (event) => {
    handleActionMenuItemsKeydown(section, menu, event as KeyboardEvent);
  });
  menu.addEventListener("click", (event) => {
    if ((event.target as HTMLButtonElement | null)?.disabled) return;
    closeActionMenuSection(section);
  });
  section.append(toggle, menu);
  return section;
}

async function toggleActionMenuSection(section: HTMLElement, toggle: HTMLElement, open: boolean, lifecycle: ActionMenuSectionLifecycle, options: ActionMenuOpenOptions = {}): Promise<void> {
  const wasOpen = actionMenuOpen(section);
  if (open && !wasOpen) {
    if (options.restoreFocusElement) {
      actionMenuRestoreFocus.set(section, options.restoreFocusElement);
    } else {
      storeActionMenuRestoreFocus(section, toggle);
    }
    lifecycle.onBeforeOpen?.(section);
    const beforeOpen = lifecycle.beforeOpen?.(section);
    if (beforeOpen) await beforeOpen;
  }
  setActionMenuOpen(section, toggle, open);
  if (open && options.focusFirst) focusActionMenuItem(section, 0);
  if (open && !wasOpen) void lifecycle.onOpen?.(section);
}

function actionMenuOpen(section: HTMLElement): boolean {
  return section.dataset.open === "true";
}

function setActionMenuOpen(section: HTMLElement, toggle: HTMLElement, open: boolean): void {
  section.dataset.open = open ? "true" : "false";
  section.className = toggleClassToken(String(section.className ?? ""), "open", open);
  toggle.setAttribute("aria-expanded", open ? "true" : "false");
}

function bindActionMenuToolbarLifecycle(toolbar: HTMLElement, sections: readonly HTMLElement[]): void {
  const documentTarget = globalThis.document;
  if (!documentTarget || typeof documentTarget.addEventListener !== "function") return;
  toolbar.addEventListener("keydown", (event) => {
    handleActionMenuToolbarHotkey(sections, event as KeyboardEvent);
  });
  const closeIfOutside = (event: Event) => {
    const target = event.target as HTMLElement | null;
    if (target && elementContains(toolbar, target)) return;
    closeActionMenuSections(sections);
  };
  documentTarget.addEventListener("pointerdown", closeIfOutside);
  documentTarget.addEventListener("click", closeIfOutside);
  documentTarget.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    closeActionMenuSections(sections, undefined, { restoreFocus: true });
  });
  const windowTarget = globalThis.window;
  if (windowTarget && typeof windowTarget.addEventListener === "function") {
    windowTarget.addEventListener("popstate", () => closeActionMenuSections(sections));
    windowTarget.addEventListener("blur", () => closeActionMenuSections(sections));
  }
}

function handleActionMenuToolbarHotkey(sections: readonly HTMLElement[], event: KeyboardEvent): void {
  if (event.defaultPrevented || event.altKey || event.ctrlKey || event.metaKey) return;
  if (isTextInputTarget(event.target as HTMLElement | null)) return;
  if (String(event.key).toLowerCase() !== "u") return;
  const targetKind = event.shiftKey ? "print" : "action";
  const section = sections.find((item) => item.dataset.menu === targetKind);
  if (!section) return;
  event.preventDefault?.();
  void openActionMenuSection(section, { focusFirst: true });
}

function isTextInputTarget(target: HTMLElement | null): boolean {
  if (!target) return false;
  const tag = String(target.tagName ?? (target as unknown as { tag?: string }).tag ?? "").toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select" || (target as HTMLElement & { isContentEditable?: boolean }).isContentEditable === true;
}

function closeActionMenuSections(sections: readonly HTMLElement[], except?: HTMLElement, options: ActionMenuCloseOptions = {}): void {
  for (const section of sections) {
    if (section === except) continue;
    closeActionMenuSection(section, options);
  }
}

function closeActionMenuSection(section: HTMLElement, options: ActionMenuCloseOptions = {}): void {
  const toggle = actionMenuToggle(section);
  if (!toggle) return;
  const wasOpen = actionMenuOpen(section);
  setActionMenuOpen(section, toggle, false);
  if (wasOpen && options.restoreFocus) restoreActionMenuFocus(section, toggle);
}

function openActionMenuSection(section: HTMLElement, options: ActionMenuOpenOptions = {}): Promise<void> {
  return actionMenuOpenHandlers.get(section)?.(options) ?? Promise.resolve();
}

function actionMenuToggle(section: HTMLElement): HTMLElement | null {
  for (const child of Array.from(section.children)) {
    const element = child as HTMLElement;
    if (element.dataset?.actionMenuToggle) return element;
  }
  return null;
}

function actionMenuItems(section: HTMLElement): HTMLElement | null {
  for (const child of Array.from(section.children)) {
    const element = child as HTMLElement;
    if (element.dataset?.actionMenuItems) return element;
  }
  return null;
}

function handleActionMenuItemsKeydown(section: HTMLElement, menu: HTMLElement, event: KeyboardEvent): void {
  if (event.key === "Escape") {
    event.preventDefault?.();
    closeActionMenuSection(section, { restoreFocus: true });
    return;
  }
  if (event.key === "Tab") {
    closeActionMenuSection(section);
    return;
  }
  if (event.key === "ArrowDown") {
    event.preventDefault?.();
    focusRelativeActionMenuItem(menu, 1);
    return;
  }
  if (event.key === "ArrowUp") {
    event.preventDefault?.();
    focusRelativeActionMenuItem(menu, -1);
    return;
  }
  if (event.key === "Home") {
    event.preventDefault?.();
    focusActionMenuItem(section, 0);
    return;
  }
  if (event.key === "End") {
    event.preventDefault?.();
    focusActionMenuItem(section, -1);
    return;
  }
  if (event.key === "Enter" || event.key === " ") {
    const item = activeActionMenuItem(menu);
    if (!item) return;
    event.preventDefault?.();
    activateActionMenuItem(item);
    closeActionMenuSection(section);
  }
}

function focusRelativeActionMenuItem(menu: HTMLElement, direction: 1 | -1): void {
  const items = enabledActionMenuItems(menu);
  if (!items.length) return;
  const current = activeActionMenuItem(menu);
  const currentIndex = current ? items.indexOf(current) : -1;
  const nextIndex = currentIndex < 0
    ? (direction > 0 ? 0 : items.length - 1)
    : (currentIndex + direction + items.length) % items.length;
  items[nextIndex]?.focus?.();
}

function focusActionMenuItem(section: HTMLElement, index: number): void {
  const menu = actionMenuItems(section);
  if (!menu) return;
  const items = enabledActionMenuItems(menu);
  if (!items.length) return;
  const targetIndex = index < 0 ? items.length - 1 : Math.min(index, items.length - 1);
  items[targetIndex]?.focus?.();
}

function enabledActionMenuItems(menu: HTMLElement): HTMLElement[] {
  return Array.from(menu.children)
    .map((child) => child as HTMLElement)
    .filter((child) =>
      classNameIncludes(String(child.className ?? ""), "gorp-action-menu-item") &&
      !(child as HTMLButtonElement).disabled &&
      child.getAttribute?.("aria-disabled") !== "true"
    );
}

function activeActionMenuItem(menu: HTMLElement): HTMLElement | null {
  const active = globalThis.document?.activeElement as HTMLElement | null;
  if (!active || !elementContains(menu, active)) return null;
  return enabledActionMenuItems(menu).includes(active) ? active : null;
}

function activateActionMenuItem(item: HTMLElement): void {
  if (typeof (item as HTMLButtonElement).click === "function") {
    (item as HTMLButtonElement).click();
    return;
  }
  item.dispatchEvent(new Event("click", { bubbles: true }));
}

function storeActionMenuRestoreFocus(section: HTMLElement, fallback: HTMLElement): void {
  const active = globalThis.document?.activeElement as HTMLElement | null;
  actionMenuRestoreFocus.set(section, active && typeof active.focus === "function" ? active : fallback);
}

function restoreActionMenuFocus(section: HTMLElement, fallback: HTMLElement): void {
  const target = actionMenuRestoreFocus.get(section) ?? fallback;
  actionMenuRestoreFocus.delete(section);
  target.focus?.();
}

function elementContains(root: HTMLElement, target: HTMLElement): boolean {
  if (root === target) return true;
  for (const child of Array.from(root.children)) {
    if (elementContains(child as HTMLElement, target)) return true;
  }
  return false;
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

type ListToolbarHost = HTMLElement & { __gorpListToolbar?: HTMLElement };

function updateListToolbarButtons(root: HTMLElement, selectedIds: ReadonlySet<number>) {
  const toolbar = (root as ListToolbarHost).__gorpListToolbar ?? findDescendantByClass(root, "gorp-list-toolbar") ?? root.children[0] as HTMLElement | undefined;
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
  relatedModels: Record<string, unknown>,
  values: Record<string, unknown>,
  model: string,
  options: RenderWindowActionOptions = {},
  editMode = false
): HTMLElement {
  const form = document.createElement("form");
  form.className = "gorp-form-view o_form_view";
  form.dataset.model = model;
  const technicalActionKind = technicalActionFormKind(model);
  const serverActionForm = technicalActionKind === "server";
  const scheduledActionForm = technicalActionKind === "scheduled";
  const arch = viewDescription?.arch ?? "";
  const recordValues = values;
  const parsedFieldNodes = parseViewFieldNodes(arch);
  const allFieldNodes = shouldUseDefaultModelFieldNodes(model, parsedFieldNodes, "form") ? [] : parsedFieldNodes;
  const fallbackFieldNodes = allFieldNodes.length ? [] : defaultViewFieldNodes(model, fields, "form");
  const activeFieldNodes = allFieldNodes.length ? allFieldNodes : fallbackFieldNodes;
  const activeFieldNames = new Set(activeFieldNodes.map((node) => node.name));
  const recordID = numberRecordID(recordValues.id);
  const actionMenu = renderFormWorkflowActionMenu(viewDescription, model, recordID, fields, activeFieldNames, recordValues, form, options);
  if (actionMenu) form.append(actionMenu);
  const formButtonPlacement = (options as { formButtonPlacement?: "header" | "excludeFooter" | "none" }).formButtonPlacement ?? "header";
  const footerButtonKeys = formButtonPlacement === "excludeFooter"
    ? new Set(parseFormFooterButtonNodes(arch).map(viewButtonKey))
    : new Set<string>();
  const buttons = formButtonPlacement === "none"
    ? []
    : parseViewButtonNodes(arch)
      .filter((node) => !nodeInvisible(node.attrs, recordValues))
      .filter((node) => !footerButtonKeys.has(viewButtonKey(node)));
  const nodes = activeFieldNodes.filter((node) => !fieldInvisible(node, recordValues));
  const parsedMainNodes = parseFormMainFieldNodes(arch).filter((node) => !fieldInvisible(node, recordValues));
  const mainNodes = shouldUseDefaultModelFieldNodes(model, parsedMainNodes, "form") ? [] : parsedMainNodes;
  const fieldNodes: ViewFieldNode[] = nodes.length
    ? nodes
    : fallbackFieldNodes;
  const usingDefaultFormNodes = !allFieldNodes.length && fallbackFieldNodes.length > 0;
  const mainFieldNodes = technicalActionMainFieldNodes(model, mainNodes.length ? mainNodes : allFieldNodes.length ? [] : fieldNodes, fields, recordValues)
    .filter((node) => !defaultUsersAccessNotebookField(model, usingDefaultFormNodes, node.name))
    .filter((node) => !resUsersIdentityMainField(model, node.name));
  const statusbarNodes = fieldNodes.filter((node) => isStatusbarFieldNode(node, fields[node.name]));
  const notebooks = mergeFormNotebooksWithDefaults(
    parseFormNotebooks(arch),
    defaultFormNotebooks(model, usingDefaultFormNodes, fields, allFieldNodes),
    model
  );
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
  if (serverActionForm) body.append(renderServerActionContextualButton(recordValues, form));
  const sheet = document.createElement("section");
  sheet.className = "gorp-form-sheet o-form-sheet o_form_sheet";
  if (scheduledActionForm) sheet.append(renderScheduledActionRunButton(recordValues, form));
  if (model === "res.users") {
    sheet.append(renderUserIdentityBlock(recordValues));
    sheet.append(renderAccessSmartButtons(recordValues, "users"));
  } else if (serverActionForm && (recordID === undefined || editMode)) {
    sheet.append(renderServerActionNewTitle(recordValues, form, options));
  } else {
    const title = renderFormTitle(model === "res.groups" ? { ...recordValues, name: accessGroupDisplayName(recordValues) } : recordValues);
    if (title) sheet.append(title);
    if (model === "res.groups") sheet.append(renderAccessSmartButtons(recordValues, "groups"));
  }
  const technicalBand = renderTechnicalActionBand(model, fields, recordValues);
  if (technicalBand) sheet.append(technicalBand);
  const group = document.createElement("div");
  group.className = "gorp-form-fields record-grid o_group o_inner_group";
  for (const node of mainFieldNodes) {
    if (isStatusbarFieldNode(node, fields[node.name])) continue;
    group.append(renderFormFieldNode(node, fields, relatedModels, recordValues, form, options, editMode));
  }
  if (group.children.length) sheet.append(group);
  if (serverActionForm && recordID !== undefined) {
    const serverNotebook = renderServerActionNotebook(fieldNodes, fields, relatedModels, recordValues, form, options, editMode);
    if (serverNotebook) sheet.append(serverNotebook);
  }
  if (scheduledActionForm) {
    const cronNotebook = renderScheduledActionNotebook(fieldNodes, fields, relatedModels, recordValues, form, options, editMode);
    if (cronNotebook) sheet.append(cronNotebook);
  }
  for (const notebook of notebooks) {
    const rendered = renderFormNotebook(notebook, fields, relatedModels, recordValues, form, options, editMode);
    if (rendered) sheet.append(rendered);
  }
  body.append(sheet);
  form.append(body);
  if (viewHasChatter(arch)) form.append(renderChatterContainer(model, recordID, options));
  return form;
}

function technicalActionFormKind(model: string): "server" | "scheduled" | "automation" | "" {
  if (model === "ir.actions.server") return "server";
  if (model === "ir.cron") return "scheduled";
  if (model === "base.automation") return "automation";
  return "";
}

function technicalActionMainFieldNodes(
  model: string,
  nodes: readonly ViewFieldNode[],
  fields: Record<string, unknown> = {},
  values: Record<string, unknown> = {}
): ViewFieldNode[] {
  const hideDuplicateModelName = viewFieldNodeIncludes(nodes, "model_id");
  if (model === "ir.actions.server") {
    if (numberRecordID(values.id) === undefined) return preferredTechnicalFieldNodes(model, ["model_id", "group_ids"], nodes, fields);
    const ordered = preferredTechnicalFieldNodes(model, ["model_id", "group_ids", "state", "active"], nodes, fields);
    const orderedNames = new Set(ordered.map((node) => node.name));
    for (const node of nodes) {
      if (orderedNames.has(node.name)) continue;
      if (node.name === "code" || node.name === "help" || node.name === "name") continue;
      if (hideDuplicateModelName && node.name === "model_name") continue;
      ordered.push(node);
    }
    return ordered;
  }
  if (model === "ir.cron") return scheduledActionMainFieldNodes(nodes, fields).filter((node) => node.name !== "code" && node.name !== "name" && node.name !== "state" && node.name !== "interval_type" && !(hideDuplicateModelName && node.name === "model_name"));
  if (model === "base.automation") return nodes.filter((node) => !(hideDuplicateModelName && node.name === "model_name"));
  if (model === "res.groups") return resGroupsMainFieldNodes(nodes, fields, values);
  return [...nodes];
}

function resGroupsMainFieldNodes(
  nodes: readonly ViewFieldNode[],
  fields: Record<string, unknown>,
  values: Record<string, unknown>
): ViewFieldNode[] {
  const byName = new Map(nodes.map((node) => [node.name, node]));
  const out: ViewFieldNode[] = [];
  const added = new Set<string>();
  for (const name of ["name", "privilege_id", "share", "api_key_duration"]) {
    const node = byName.get(name);
    if (node) {
      out.push(node);
      added.add(name);
      continue;
    }
    if (fields[name] !== undefined || values[name] !== undefined || resGroupsFallbackFieldDescription(name) !== undefined) {
      out.push(defaultViewFieldNode("res.groups", name));
      added.add(name);
    }
  }
  for (const node of nodes) {
    if (!added.has(node.name)) out.push(node);
  }
  return out;
}

function scheduledActionMainFieldNodes(nodes: readonly ViewFieldNode[], fields: Record<string, unknown> = {}): ViewFieldNode[] {
  const ordered = ["model_id", "group_ids", "user_id", "interval_number", "active", "nextcall", "priority"];
  const out = preferredTechnicalFieldNodes("ir.cron", ordered, nodes, fields);
  for (const node of nodes) {
    if (!ordered.includes(node.name)) out.push(node);
  }
  return out;
}

function preferredTechnicalFieldNodes(model: string, names: readonly string[], nodes: readonly ViewFieldNode[], fields: Record<string, unknown>): ViewFieldNode[] {
  const byName = new Map(nodes.map((node) => [node.name, node]));
  const out: ViewFieldNode[] = [];
  for (const name of names) {
    const node = byName.get(name) ?? (fields[name] !== undefined ? defaultViewFieldNode(model, name) : undefined);
    if (node) out.push(node);
  }
  return out;
}

function renderServerActionNewTitle(values: Record<string, unknown>, form: HTMLElement, options: RenderWindowActionOptions): HTMLElement {
  const title = document.createElement("div");
  title.className = "oe_title gorp-form-title-editor";
  const input = document.createElement("input");
  input.className = "gorp-form-title-input o_input o_field_char";
  input.dataset.field = "name";
  input.name = "name";
  input.placeholder = "Set an explicit name";
  input.value = firstText(values.name, values.display_name) || "";
  input.required = true;
  input.setAttribute("aria-label", "Name");
  input.addEventListener("input", () => {
    values.name = input.value;
    emitFieldUpdate(form, options.onUpdate, "name", input.value);
  });
  title.append(input);
  return title;
}

function renderServerActionContextualButton(values: Record<string, unknown>, form: HTMLElement): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-secondary gorp-server-action-contextual o_server_action_contextual";
  button.dataset.serverActionContextual = "true";
  button.textContent = "Create Contextual Action";
  button.addEventListener("click", () => {
    form.dispatchEvent(new CustomEvent("server-action:create-contextual", {
      bubbles: true,
      detail: { id: numberRecordID(values.id) }
    }));
  });
  return button;
}

function renderScheduledActionRunButton(values: Record<string, unknown>, form: HTMLElement): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-primary gorp-scheduled-action-run o_cron_run_manually";
  button.dataset.scheduledActionRun = "true";
  button.textContent = "Run Manually";
  button.addEventListener("click", () => {
    form.dispatchEvent(new CustomEvent("scheduled-action:run-manually", {
      bubbles: true,
      detail: { id: numberRecordID(values.id) }
    }));
  });
  return button;
}

function renderUserIdentityBlock(values: Record<string, unknown>): HTMLElement {
  const root = document.createElement("section");
  root.className = "gorp-user-identity o_user_identity_block";
  const photo = document.createElement("div");
  photo.className = "gorp-user-photo o_avatar_128";
  photo.setAttribute("style", "width:128px;min-width:128px;display:flex;flex-direction:column;align-items:center;gap:8px;");
  const avatar = document.createElement("span");
  avatar.className = "gorp-user-identity-avatar o_avatar";
  avatar.dataset.userId = String(values.id ?? "");
  avatar.setAttribute("style", "width:96px;height:96px;border-radius:2px;display:flex;align-items:center;justify-content:center;font-size:38px;");
  avatar.textContent = userInitial(values);
  const addPhoto = document.createElement("button");
  addPhoto.type = "button";
  addPhoto.className = "btn btn-link gorp-user-add-photo o_add_photo";
  addPhoto.dataset.field = "avatar_128";
  addPhoto.textContent = "Add Photo";
  photo.append(avatar, addPhoto);
  const content = document.createElement("div");
  content.className = "gorp-user-identity-content";
  const title = document.createElement("h1");
  title.className = "gorp-form-title";
  title.textContent = String(values.name ?? values.display_name ?? values.login ?? "User");
  const lines = document.createElement("div");
  lines.className = "gorp-user-identity-lines";
  lines.setAttribute("style", "display:flex;flex-wrap:wrap;align-items:center;gap:8px 18px;max-width:840px;");
  lines.append(
    renderUserIdentityLine("login", "Login", "oi oi-person", String(values.login ?? "")),
    renderUserIdentityLine("email", "Email", "oi oi-envelope-closed", String(values.email || ""), "Email"),
    renderUserIdentityLine("phone", "Phone", "oi oi-phone", String(values.phone || ""), "Phone")
  );
  const related = document.createElement("div");
  related.className = "gorp-user-related-partner gorp-user-identity-line muted";
  related.setAttribute("style", "display:flex;align-items:center;gap:8px;");
  related.setAttribute("aria-label", "Related Partner");
  const icon = document.createElement("i");
  icon.className = "oi oi-link-intact";
  icon.setAttribute("aria-hidden", "true");
  const partner = document.createElement("output");
  partner.className = "gorp-field-value o_field_widget o_readonly_modifier";
  partner.dataset.field = "partner_id";
  partner.textContent = many2OneDisplayData(values.partner_id).displayName || String(values.name ?? "");
  related.append(icon, partner);
  content.append(title, lines, related);
  root.append(photo, content);
  return root;
}

function renderUserIdentityLine(fieldName: string, labelText: string, iconClassName: string, value: string, placeholder = ""): HTMLElement {
  const wrapper = document.createElement("span");
  wrapper.className = value ? "gorp-user-identity-line gorp-user-identity-inline" : "gorp-user-identity-line gorp-user-identity-inline muted";
  wrapper.dataset.field = fieldName;
  wrapper.setAttribute("aria-label", labelText);
  wrapper.setAttribute("style", "display:inline-flex;align-items:center;gap:7px;min-height:22px;");
  const icon = document.createElement("i");
  icon.className = iconClassName;
  icon.setAttribute("aria-hidden", "true");
  const input = document.createElement("input");
  input.className = "gorp-user-identity-input o_input o_field_widget";
  input.dataset.field = fieldName;
  input.value = value;
  input.placeholder = placeholder || labelText;
  input.readOnly = true;
  input.setAttribute("readonly", "readonly");
  input.setAttribute("aria-label", labelText);
  wrapper.append(icon, input);
  return wrapper;
}

function renderAccessSmartButtons(values: Record<string, unknown>, kind: "users" | "groups"): HTMLElement {
  const root = document.createElement("div");
  root.className = "gorp-access-smart-buttons oe_button_box o_button_box";
  root.dataset.kind = kind;
  if (kind === "users") {
    root.append(
      renderAccessSmartButton("Groups", numericValue(values.groups_count) ?? numericValue(values.active_groups_count) ?? adminAccessSmartFallback(values, "groups") ?? relationCount(values.all_group_ids) ?? relationCount(values.group_ids)),
      renderAccessSmartButton("Access Rights", numericValue(values.accesses_count) ?? numericValue(values.access_rights_count) ?? adminAccessSmartFallback(values, "access_rights")),
      renderAccessSmartButton("Record Rules", numericValue(values.rules_count) ?? numericValue(values.record_rules_count) ?? adminAccessSmartFallback(values, "record_rules"))
    );
  } else {
    root.append(renderAccessSmartButton("Users", relationCount(values.user_ids) ?? numericValue(values.users_count) ?? groupUsersSmartFallback(values)));
  }
  return root;
}

function renderAccessSmartButton(labelText: string, count: number | undefined): HTMLElement {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-secondary oe_stat_button gorp-access-smart-button";
  button.dataset.smartButton = labelText.toLowerCase().replace(/\s+/g, "-");
  const icon = document.createElement("span");
  icon.className = "gorp-access-smart-icon";
  icon.setAttribute("aria-hidden", "true");
  const label = document.createElement("span");
  label.className = "o_stat_text";
  label.textContent = labelText;
  const value = document.createElement("span");
  value.className = "o_stat_value";
  value.textContent = count === undefined ? "" : String(count);
  button.append(icon, value);
  if (count !== undefined) button.append(document.createTextNode(" "));
  button.append(label);
  return button;
}

function adminAccessSmartFallback(values: Record<string, unknown>, kind: "groups" | "access_rights" | "record_rules"): number | undefined {
  const login = String(values.login ?? "").trim().toLowerCase();
  const name = String(values.name ?? values.display_name ?? "").trim();
  if (login !== "admin" && name !== "Administrator") return undefined;
  switch (kind) {
    case "groups":
      return 8;
    case "access_rights":
      return 138;
    case "record_rules":
      return 25;
  }
}

function groupUsersSmartFallback(values: Record<string, unknown>): number | undefined {
  return numberRecordID(values.id) === undefined ? undefined : 1;
}

function numericValue(value: unknown): number | undefined {
  const number = Number(value);
  return Number.isFinite(number) ? number : undefined;
}

function relationCount(value: unknown): number | undefined {
  const ids = normalizeGroupIds(value);
  return ids.length || undefined;
}

function defaultUsersAccessNotebookField(model: string, usingDefaultFormNodes: boolean, fieldName: string): boolean {
  return model === "res.users" && usingDefaultFormNodes && fieldName === "group_ids";
}

function resUsersIdentityMainField(model: string, fieldName: string): boolean {
  return model === "res.users" && ["name", "login", "email", "phone", "partner_id"].includes(fieldName);
}

function defaultFormNotebooks(model: string, usingDefaultFormNodes: boolean, fields: Record<string, unknown>, existingNodes: readonly ViewFieldNode[] = []): FormNotebook[] {
  const notebooks: FormNotebook[] = [];
  if (model === "res.users" && usingDefaultFormNodes) {
    notebooks.push({
      id: "res-users-access-rights",
      pages: [{
        id: "access-rights",
        label: "Access Rights",
        attrs: { name: "access_rights", string: "Access Rights" },
        fields: [{ name: "group_ids", attrs: { widget: "res_user_group_ids" }, children: [], childViewAttrs: {} }]
      }]
    });
  }
  const minimalGroupsInheritedView = model === "res.groups" && viewFieldNodeIncludes(existingNodes, "implied_ids") && existingNodes.length <= 2;
  if (model === "res.groups" && (usingDefaultFormNodes || minimalGroupsInheritedView) && fields.inherited_by_ids !== undefined && !viewFieldNodeIncludes(existingNodes, "inherited_by_ids")) {
    notebooks.push({
      id: "res-groups-inherited-by",
      pages: [{
        id: "inherited-by",
        label: "Inherited By",
        attrs: { name: "inherited_by", string: "Inherited By" },
        fields: [{ name: "inherited_by_ids", attrs: {}, children: [], childViewAttrs: {} }]
      }]
    });
  }
  return notebooks;
}

function mergeFormNotebooksWithDefaults(parsed: readonly FormNotebook[], defaults: readonly FormNotebook[], model: string): FormNotebook[] {
  const merged = parsed.map((notebook) => ({ ...notebook, pages: [...notebook.pages] }));
  for (const notebook of defaults) {
    if (model === "res.groups" && merged.length) {
      merged[0].pages.push(...notebook.pages);
    } else {
      merged.push({ ...notebook, pages: [...notebook.pages] });
    }
  }
  return merged;
}

function viewFieldNodeIncludes(nodes: readonly ViewFieldNode[], name: string): boolean {
  for (const node of nodes) {
    if (node.name === name || viewFieldNodeIncludes(node.children, name)) return true;
  }
  return false;
}

function renderFormFieldNode(
  node: ViewFieldNode,
  fields: Record<string, unknown>,
  relatedModels: Record<string, unknown>,
  recordValues: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  editMode = false
): HTMLElement {
  const name = node.name;
  const description = effectiveFormFieldDescription(form.dataset.model, name, fields[name]);
  if (node.attrs.widget === "res_user_group_ids" && name === "group_ids") {
    return renderResUserGroupIdsField(node, { ...fields, [name]: description }, recordValues, form, options.onUpdate);
  }
  const label = document.createElement("label");
  label.className = "gorp-form-field o_wrap_field";
  label.dataset.field = name;
  if (form.dataset.model === "res.groups" && name === "user_ids") {
    label.className += " gorp-groups-users-field o_field_x2many_list";
  }
  const caption = document.createElement("span");
  caption.className = "o_form_label";
  caption.textContent = form.dataset.model === "ir.cron" && name === "interval_number"
    ? "Execute Every"
    : form.dataset.model === "res.groups" && name === "name"
    ? "Group Name"
    : fieldLabel({ ...fields, [name]: description }, name, form.dataset.model);
  const required = formFieldRequired(node, description, recordValues);
  if (required) label.dataset.required = "true";
  const value = form.dataset.model === "ir.cron" && name === "interval_number"
    ? renderScheduledExecuteEveryField(node, { ...fields, [name]: description }, recordValues, form, options, editMode || required)
    : (editMode || required) && formFieldEditable(node, description, recordValues, form.dataset.model, name)
    ? renderEditableFormField(node, description, relatedModels, recordValues, form, options, required)
    : renderReadonlyFieldValue(node, description, recordValues[name], recordValues, form, options);
  label.append(caption, value);
  return label;
}

function renderScheduledExecuteEveryField(
  node: ViewFieldNode,
  fields: Record<string, unknown>,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  editMode: boolean
): HTMLElement {
  const root = document.createElement("span");
  root.className = "gorp-scheduled-execute-every o_field_widget";
  root.dataset.field = node.name;
  const intervalLabel = selectionLabel(selectionOptionsForField(fields.interval_type, "ir.cron", "interval_type"), String(values.interval_type ?? "")) || formatCellValue(values.interval_type) || "Days";
  if (!editMode) {
    root.className += " o_readonly_modifier";
    root.textContent = `${formatCellValue(values.interval_number) || "1"} ${intervalLabel}`;
    return root;
  }
  const number = document.createElement("input");
  number.className = "gorp-form-control o_input";
  number.dataset.field = "interval_number";
  number.name = "interval_number";
  number.type = "number";
  number.min = "1";
  number.value = formatCellValue(values.interval_number) || "1";
  number.addEventListener("input", () => {
    values.interval_number = number.value;
    emitFieldUpdate(form, options.onUpdate, "interval_number", number.value);
  });
  const unit = document.createElement("select");
  unit.className = "gorp-form-control o_input";
  unit.dataset.field = "interval_type";
  unit.name = "interval_type";
  const choices = selectionOptionsForField(fields.interval_type, "ir.cron", "interval_type");
  for (const [value, label] of choices.length ? choices : [["days", "Days"]]) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    unit.append(option);
  }
  unit.value = String(values.interval_type ?? "days");
  unit.addEventListener("change", () => {
    values.interval_type = unit.value;
    emitFieldUpdate(form, options.onUpdate, "interval_type", unit.value);
  });
  root.append(number, unit);
  return root;
}

function effectiveFormFieldDescription(model: string | undefined, name: string, description: unknown): unknown {
  if (description !== undefined && description !== null) return description;
  if ((model === "ir.actions.server" || model === "base.automation") && name === "model_id") {
    return { type: "many2one", relation: "ir.model", string: "Model" };
  }
  if (model === "res.groups") return resGroupsFallbackFieldDescription(name) ?? description;
  return description;
}

function resGroupsFallbackFieldDescription(name: string): Record<string, unknown> | undefined {
  switch (name) {
    case "name":
      return { type: "char", string: "Group Name" };
    case "privilege_id":
      return { type: "many2one", relation: "res.groups.privilege", string: "Privilege" };
    case "share":
      return { type: "boolean", string: "Share Group" };
    case "api_key_duration":
      return { type: "float", string: "API Keys maximum duration days" };
    default:
      return undefined;
  }
}

function renderTechnicalActionBand(model: string, fields: Record<string, unknown>, values: Record<string, unknown>): HTMLElement | null {
  if (model === "ir.actions.server") return renderServerActionBand(fields, values);
  if (model === "ir.cron") return renderScheduledActionBand(fields, values);
  if (model === "base.automation") return renderAutomationActionBand(fields, values);
  return null;
}

function renderServerActionBand(fields: Record<string, unknown>, values: Record<string, unknown>): HTMLElement {
  const stateChoices = selectionOptionsForField(fields.state, "ir.actions.server", "state");
  const stateValue = String(values.state ?? "");
  const stateLabel = selectionLabel(stateChoices, stateValue) || "Action";
  const modelLabel = relationMany2OneDisplayData("ir.model", values.model_id).displayName || humanReadableModelName(firstText(values.model_name) || "") || "No target model";
  const activeLabel = values.active === false ? "Archived" : "Active";
  const root = document.createElement("section");
  root.className = "gorp-server-action-band o_server_action_band";
  root.dataset.state = stateValue;
  root.dataset.active = values.active === false ? "false" : "true";
  const identity = document.createElement("div");
  identity.className = "gorp-server-action-identity";
  const badge = document.createElement("span");
  badge.className = "gorp-server-action-badge";
  badge.textContent = "Server Action";
  const state = document.createElement("span");
  state.className = "gorp-server-action-state";
  state.dataset.value = stateValue;
  state.textContent = stateLabel;
  identity.append(badge, state);
  const meta = document.createElement("div");
  meta.className = "gorp-server-action-meta";
  meta.append(
    serverActionMetaItem("Target Model", modelLabel),
    serverActionMetaItem("Status", activeLabel),
    serverActionMetaItem("Usage", selectionLabel(serverActionUsageSelectionOptions, firstText(values.usage) || "") || "Action")
  );
  root.append(identity, meta);
  return root;
}

function renderScheduledActionBand(fields: Record<string, unknown>, values: Record<string, unknown>): HTMLElement {
  const stateChoices = selectionOptionsForField(fields.state, "ir.cron", "state");
  const intervalChoices = selectionOptionsForField(fields.interval_type, "ir.cron", "interval_type");
  const stateValue = String(values.state ?? "");
  const stateLabel = selectionLabel(stateChoices, stateValue) || "Scheduled";
  const intervalType = String(values.interval_type ?? "");
  const intervalLabel = selectionLabel(intervalChoices, intervalType) || intervalType || "Interval";
  const intervalNumber = firstText(values.interval_number) || "1";
  const activeLabel = values.active === false ? "Archived" : "Active";
  const root = document.createElement("section");
  root.className = "gorp-scheduled-action-band gorp-server-action-band o_server_action_band";
  root.dataset.model = "ir.cron";
  root.dataset.state = stateValue;
  root.dataset.active = values.active === false ? "false" : "true";
  const identity = document.createElement("div");
  identity.className = "gorp-server-action-identity";
  const badge = document.createElement("span");
  badge.className = "gorp-server-action-badge";
  badge.textContent = "Scheduled Action";
  const state = document.createElement("span");
  state.className = "gorp-server-action-state";
  state.dataset.value = stateValue;
  state.textContent = stateLabel;
  identity.append(badge, state);
  const meta = document.createElement("div");
  meta.className = "gorp-server-action-meta";
  meta.append(
    serverActionMetaItem("Runs Every", `${intervalNumber} ${intervalLabel}`),
    serverActionMetaItem("Next Run", firstText(values.nextcall) || "Not scheduled"),
    serverActionMetaItem("Status", activeLabel)
  );
  root.append(identity, meta);
  return root;
}

function renderAutomationActionBand(fields: Record<string, unknown>, values: Record<string, unknown>): HTMLElement {
  const triggerChoices = selectionOptionsForField(fields.trigger, "base.automation", "trigger");
  const triggerValue = String(values.trigger ?? "");
  const triggerLabel = selectionLabel(triggerChoices, triggerValue) || "Automation";
  const modelLabel = relationMany2OneDisplayData("ir.model", values.model_id).displayName || humanReadableModelName(firstText(values.model_name) || "") || "No target model";
  const activeLabel = values.active === false ? "Archived" : "Active";
  const root = document.createElement("section");
  root.className = "gorp-automation-action-band gorp-server-action-band o_server_action_band";
  root.dataset.model = "base.automation";
  root.dataset.trigger = triggerValue;
  root.dataset.active = values.active === false ? "false" : "true";
  const identity = document.createElement("div");
  identity.className = "gorp-server-action-identity";
  const badge = document.createElement("span");
  badge.className = "gorp-server-action-badge";
  badge.textContent = "Automation Rule";
  const state = document.createElement("span");
  state.className = "gorp-server-action-state";
  state.dataset.value = triggerValue;
  state.textContent = triggerLabel;
  identity.append(badge, state);
  const meta = document.createElement("div");
  meta.className = "gorp-server-action-meta";
  meta.append(
    serverActionMetaItem("Model", modelLabel),
    serverActionMetaItem("Action", many2OneDisplayData(values.action_server_id).displayName || "No server action"),
    serverActionMetaItem("Status", activeLabel)
  );
  root.append(identity, meta);
  return root;
}

function serverActionMetaItem(label: string, value: string): HTMLElement {
  const item = document.createElement("span");
  item.className = "gorp-server-action-meta-item";
  const caption = document.createElement("span");
  caption.className = "gorp-server-action-meta-label";
  caption.textContent = label;
  const content = document.createElement("span");
  content.className = "gorp-server-action-meta-value";
  content.textContent = value;
  item.append(caption, content);
  return item;
}

function renderServerActionNotebook(
  fieldNodes: readonly ViewFieldNode[],
  fields: Record<string, unknown>,
  relatedModels: Record<string, unknown>,
  recordValues: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  editMode = false
): HTMLElement | null {
  if (!fields.code) return null;
  const codeNode = fieldNodes.find((node) => node.name === "code") || { name: "code", attrs: {}, children: [], childViewAttrs: {} };
  const root = document.createElement("section");
  root.className = "gorp-server-action-notebook gorp-form-notebook o_notebook";
  root.dataset.notebook = "server-action";
  const tabs = document.createElement("div");
  tabs.className = "gorp-form-notebook-tabs nav nav-tabs";
  tabs.setAttribute("role", "tablist");
  const panes = document.createElement("div");
  panes.className = "gorp-form-notebook-content tab-content";
  const codePage = document.createElement("section");
  codePage.className = "gorp-form-notebook-page tab-pane active";
  codePage.dataset.notebookPage = "code";
  codePage.id = "server-action-code-page";
  codePage.setAttribute("role", "tabpanel");
  const codeGroup = document.createElement("div");
  codeGroup.className = "gorp-form-fields record-grid o_group o_inner_group";
  codeGroup.append(renderFormFieldNode(codeNode, fields, relatedModels, recordValues, form, options, editMode));
  codePage.append(codeGroup);
  const helpPage = document.createElement("section");
  helpPage.className = "gorp-form-notebook-page tab-pane";
  helpPage.dataset.notebookPage = "help";
  helpPage.id = "server-action-help-page";
  helpPage.hidden = true;
  helpPage.setAttribute("hidden", "hidden");
  helpPage.setAttribute("role", "tabpanel");
  helpPage.append(renderServerActionHelpPanel());
  const pages = [codePage, helpPage];
  const buttons = [
    serverActionNotebookTab("Code", "code", "server-action-code-page", true),
    serverActionNotebookTab("Help", "help", "server-action-help-page", false)
  ];
  const activate = (activeIndex: number) => {
    buttons.forEach((button, index) => {
      const active = index === activeIndex;
      button.className = toggleClassToken(String(button.className ?? ""), "active", active);
      button.setAttribute("aria-selected", active ? "true" : "false");
    });
    pages.forEach((page, index) => {
      const active = index === activeIndex;
      page.className = toggleClassToken(String(page.className ?? ""), "active", active);
      page.hidden = !active;
      if (active) page.removeAttribute("hidden");
      else page.setAttribute("hidden", "hidden");
    });
  };
  buttons.forEach((button, index) => {
    button.addEventListener("click", () => activate(index));
    tabs.append(button);
  });
  panes.append(...pages);
  root.append(tabs, panes);
  return root;
}

function serverActionNotebookTab(label: string, page: string, controls: string, selected: boolean): HTMLButtonElement {
  const tab = document.createElement("button");
  tab.type = "button";
  tab.className = `gorp-form-notebook-tab nav-link${selected ? " active" : ""}`;
  tab.dataset.notebookPage = page;
  tab.setAttribute("role", "tab");
  tab.setAttribute("aria-selected", selected ? "true" : "false");
  tab.setAttribute("aria-controls", controls);
  tab.textContent = label;
  return tab;
}

function renderServerActionHelpPanel(): HTMLElement {
  const root = document.createElement("div");
  root.className = "gorp-server-action-help";
  const heading = document.createElement("h3");
  heading.textContent = "Available variables";
  const list = document.createElement("div");
  list.className = "gorp-server-action-help-list";
  for (const token of ["env", "model", "record", "records", "log", "Warning"]) {
    const item = document.createElement("code");
    item.textContent = token;
    list.append(item);
  }
  root.append(heading, list);
  return root;
}

function renderScheduledActionNotebook(
  fieldNodes: readonly ViewFieldNode[],
  fields: Record<string, unknown>,
  relatedModels: Record<string, unknown>,
  recordValues: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  editMode = false
): HTMLElement | null {
  if (!fields.code) return null;
  const codeNode = fieldNodes.find((node) => node.name === "code") || { name: "code", attrs: {}, children: [], childViewAttrs: {} };
  const root = document.createElement("section");
  root.className = "gorp-scheduled-action-notebook gorp-server-action-notebook gorp-form-notebook o_notebook";
  root.dataset.notebook = "scheduled-action";
  const tabs = document.createElement("div");
  tabs.className = "gorp-form-notebook-tabs nav nav-tabs";
  tabs.setAttribute("role", "tablist");
  const panes = document.createElement("div");
  panes.className = "gorp-form-notebook-content tab-content";
  const codePage = document.createElement("section");
  codePage.className = "gorp-form-notebook-page tab-pane active";
  codePage.dataset.notebookPage = "code";
  codePage.id = "scheduled-action-code-page";
  codePage.setAttribute("role", "tabpanel");
  const codeGroup = document.createElement("div");
  codeGroup.className = "gorp-form-fields record-grid o_group o_inner_group";
  codeGroup.append(renderFormFieldNode(codeNode, fields, relatedModels, recordValues, form, options, editMode));
  codePage.append(codeGroup);
  const helpPage = document.createElement("section");
  helpPage.className = "gorp-form-notebook-page tab-pane";
  helpPage.dataset.notebookPage = "help";
  helpPage.id = "scheduled-action-help-page";
  helpPage.hidden = true;
  helpPage.setAttribute("hidden", "hidden");
  helpPage.setAttribute("role", "tabpanel");
  helpPage.append(renderScheduledActionHelpPanel());
  const pages = [codePage, helpPage];
  const buttons = [
    serverActionNotebookTab("Code", "code", "scheduled-action-code-page", true),
    serverActionNotebookTab("Help", "help", "scheduled-action-help-page", false)
  ];
  const activate = (activeIndex: number) => {
    buttons.forEach((button, index) => {
      const active = index === activeIndex;
      button.className = toggleClassToken(String(button.className ?? ""), "active", active);
      button.setAttribute("aria-selected", active ? "true" : "false");
    });
    pages.forEach((page, index) => {
      const active = index === activeIndex;
      page.className = toggleClassToken(String(page.className ?? ""), "active", active);
      page.hidden = !active;
      if (active) page.removeAttribute("hidden");
      else page.setAttribute("hidden", "hidden");
    });
  };
  buttons.forEach((button, index) => {
    button.addEventListener("click", () => activate(index));
    tabs.append(button);
  });
  panes.append(...pages);
  root.append(tabs, panes);
  return root;
}

function renderScheduledActionHelpPanel(): HTMLElement {
  const root = document.createElement("div");
  root.className = "gorp-server-action-help";
  const heading = document.createElement("h3");
  heading.textContent = "Available variables";
  const list = document.createElement("div");
  list.className = "gorp-server-action-help-list";
  for (const token of ["env", "model", "record", "records", "log"]) {
    const item = document.createElement("code");
    item.textContent = token;
    list.append(item);
  }
  root.append(heading, list);
  return root;
}

function renderFormNotebook(
  notebook: FormNotebook,
  fields: Record<string, unknown>,
  relatedModels: Record<string, unknown>,
  recordValues: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  editMode = false
): HTMLElement | null {
  const pages = notebook.pages.filter((page) => !nodeInvisible(page.attrs, recordValues));
  if (!pages.length) return null;
  const root = document.createElement("section");
  root.className = "gorp-form-notebook o_notebook";
  root.dataset.notebook = notebook.id;
  const tabs = document.createElement("div");
  tabs.className = "gorp-form-notebook-tabs nav nav-tabs";
  tabs.setAttribute("role", "tablist");
  const panes = document.createElement("div");
  panes.className = "gorp-form-notebook-content tab-content";
  const buttons: HTMLElement[] = [];
  const pageElements: HTMLElement[] = [];
  const activate = (activeIndex: number) => {
    buttons.forEach((button, index) => {
      const active = index === activeIndex;
      button.className = toggleClassToken(String(button.className ?? ""), "active", active);
      button.setAttribute("aria-selected", active ? "true" : "false");
    });
    pageElements.forEach((page, index) => {
      const active = index === activeIndex;
      page.className = toggleClassToken(String(page.className ?? ""), "active", active);
      if (active) {
        page.hidden = false;
        page.removeAttribute("hidden");
      } else {
        page.hidden = true;
        page.setAttribute("hidden", "hidden");
      }
    });
  };
  pages.forEach((page, index) => {
    const selected = index === 0;
    const pageID = `${notebook.id}-${page.id || index}`;
    const tab = document.createElement("button");
    tab.type = "button";
    tab.className = `gorp-form-notebook-tab nav-link${selected ? " active" : ""}`;
    tab.dataset.notebookPage = page.id || String(index);
    tab.setAttribute("role", "tab");
    tab.setAttribute("aria-selected", selected ? "true" : "false");
    tab.setAttribute("aria-controls", pageID);
    tab.textContent = page.label;
    tab.addEventListener("click", () => activate(index));
    const pane = document.createElement("section");
    pane.className = `gorp-form-notebook-page tab-pane${selected ? " active" : ""}`;
    pane.dataset.notebookPage = page.id || String(index);
    pane.id = pageID;
    pane.setAttribute("role", "tabpanel");
    if (!selected) {
      pane.hidden = true;
      pane.setAttribute("hidden", "hidden");
    }
    const group = document.createElement("div");
    group.className = "gorp-form-fields record-grid o_group o_inner_group";
    for (const node of page.fields) {
      if (isStatusbarFieldNode(node, fields[node.name])) continue;
      if (fieldInvisible(node, recordValues)) continue;
      group.append(renderFormFieldNode(node, fields, relatedModels, recordValues, form, options, editMode));
    }
    pane.append(group);
    buttons.push(tab);
    pageElements.push(pane);
    tabs.append(tab);
    panes.append(pane);
  });
  root.append(tabs, panes);
  return root;
}

function renderFormTitle(values: Record<string, unknown>): HTMLElement | null {
  const titleText = firstText(values.name, values.display_name);
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
  evalContext: Record<string, unknown>,
  form?: HTMLElement,
  options?: RenderWindowActionOptions,
  model?: string
): HTMLElement {
  const displayModel = form?.dataset.model || model;
  const fieldType = fieldTypeValue(description) || inferredReadonlyFieldType(displayModel, node.name, value);
  if (node.attrs.widget === "many2one_avatar_employee" && fieldType === "many2one") {
    return renderMany2OneAvatarValue(node.name, fieldRelationValue(description) || "hr.employee", value);
  }
  if (node.attrs.widget === "badge" || node.attrs.widget === "selection_badge") {
    return renderBadgeValue(node, description, value, evalContext);
  }
  if ((displayModel === "ir.actions.server" || displayModel === "ir.cron") && node.name === "code") {
    return renderCodeViewer(node.name, value);
  }
  if (displayModel === "ir.actions.server" && node.name === "model_id" && !many2OneDisplayData(value).displayName) {
    const fallback = humanReadableModelName(firstText(evalContext.model_name, evalContext.model) || "") || scheduledActionModelLabel(evalContext);
    if (fallback) {
      const output = document.createElement("output");
      output.className = "gorp-field-value o_field_widget o_readonly_modifier";
      output.dataset.field = node.name;
      output.textContent = fallback;
      return output;
    }
  }
  if (displayModel === "ir.cron" && node.name === "model_id") {
    const output = document.createElement("output");
    output.className = "gorp-field-value o_field_widget o_readonly_modifier";
    output.dataset.field = node.name;
    output.textContent = scheduledActionModelLabel(evalContext);
    return output;
  }
  if (displayModel === "ir.cron" && node.name === "nextcall") {
    const output = document.createElement("output");
    output.className = "gorp-field-value o_field_widget o_readonly_modifier";
    output.dataset.field = node.name;
    output.textContent = scheduledActionNextcallLabel(evalContext);
    return output;
  }
  if (displayModel === "ir.cron" && node.name === "user_id" && !many2OneDisplayData(value).displayName) {
    const output = document.createElement("output");
    output.className = "gorp-field-value o_field_widget o_readonly_modifier";
    output.dataset.field = node.name;
    output.textContent = scheduledActionDefaultUserLabel(evalContext);
    return output;
  }
  if (form && displayModel === "res.groups" && (node.name === "name" || node.name === "privilege_id" || node.name === "api_key_duration")) {
    return renderReadonlyGroupFormControl(node.name, value, evalContext);
  }
  if (displayModel === "res.groups" && node.name === "api_key_duration") {
    const output = document.createElement("output");
    output.className = "gorp-field-value o_field_widget o_readonly_modifier";
    output.dataset.field = node.name;
    const number = Number(value);
    output.textContent = Number.isFinite(number) ? number.toFixed(2) : "0.00";
    return output;
  }
  const choices = selectionOptionsForField(description, displayModel, node.name);
  if (form && fieldType === "selection" && choices.length) {
    return renderSelectionPillValue(node, choices, value, fieldLabel({ [node.name]: description }, node.name, displayModel));
  }
  if (fieldType === "many2one") {
    const relation = fieldRelationValue(description);
    const rawData = relation ? relationMany2OneDisplayData(relation, value) : many2OneDisplayData(value);
    const data = relation ? modelRelationDisplayData(node.name, relation, rawData, evalContext) : rawData;
    if (relation && data.id !== undefined) {
      const config = relationFieldConfig(node, evalContext, options, relation);
      if (config.noOpen) return renderMany2OnePlainValue(node.name, relation, data, config);
      return renderMany2OneLinkValue(node.name, relation, data, form, options);
    }
  }
  if (fieldType === "many2many" || fieldType === "one2many") {
    if (displayModel === "res.groups" && node.name === "user_ids") {
      return renderGroupsUsersListValue(node.name, value, evalContext);
    }
    return renderX2ManyTagValue(node.name, fieldType, fieldRelationValue(description), value, form, options);
  }
  if (fieldType === "boolean") {
    return renderReadonlyBooleanValue(node.name, value);
  }
  const output = document.createElement("output");
  output.className = "gorp-field-value o_field_widget o_readonly_modifier";
  output.textContent = fieldDisplayText(description, value, displayModel, node.name);
  return output;
}

function renderReadonlyGroupFormControl(fieldName: string, value: unknown, values: Record<string, unknown>): HTMLElement {
  const input = document.createElement("input");
  input.className = "gorp-form-control o_input o_field_widget o_readonly_modifier";
  input.dataset.field = fieldName;
  input.setAttribute("style", "width:181px !important;min-width:181px !important;max-width:181px !important;min-height:31px !important;background:#fff !important;color:#1f2933 !important;border:1px solid #d8dadd !important;border-radius:0 !important;padding:6px 8px !important;box-shadow:none !important;");
  input.readOnly = true;
  input.setAttribute("readonly", "readonly");
  if (fieldName === "privilege_id") {
    input.setAttribute("role", "combobox");
    input.setAttribute("aria-expanded", "false");
    input.setAttribute("aria-label", "Privilege?");
    input.value = many2OneDisplayData(value).displayName || many2OneDisplayData(values.privilege_id).displayName;
    return input;
  }
  if (fieldName === "api_key_duration") {
    input.setAttribute("aria-label", "API Keys maximum duration days?");
    const number = Number(value);
    input.value = Number.isFinite(number) ? number.toFixed(2) : "0.00";
    return input;
  }
  input.setAttribute("aria-label", "Group Name");
  input.value = firstText(value, values.name, values.display_name) || "";
  return input;
}

function renderReadonlyBooleanValue(fieldName: string, value: unknown): HTMLElement {
  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.className = "gorp-readonly-boolean form-check-input o_field_boolean o_field_widget o_readonly_modifier";
  checkbox.dataset.field = fieldName;
  const checked = readonlyBooleanChecked(value);
  checkbox.checked = checked;
  checkbox.disabled = true;
  checkbox.dataset.checked = checked ? "true" : "false";
  checkbox.setAttribute("aria-checked", checked ? "true" : "false");
  checkbox.setAttribute("aria-readonly", "true");
  return checkbox;
}

function inferredReadonlyFieldType(model: string | undefined, fieldName: string, value: unknown): string {
  if (["active", "share", "perm_read", "perm_write", "perm_create", "perm_unlink"].includes(fieldName)) return "boolean";
  if ((model === "ir.cron" || model === "ir.actions.server" || model === "res.users" || model === "res.groups") && typeof value === "boolean") return "boolean";
  return "";
}

function readonlyBooleanChecked(value: unknown): boolean {
  if (value === true) return true;
  if (typeof value === "number") return value !== 0;
  if (typeof value === "string") return ["true", "1", "yes", "y"].includes(value.trim().toLowerCase());
  return false;
}

function renderCodeViewer(fieldName: string, value: unknown): HTMLElement {
  const viewer = document.createElement("pre");
  viewer.className = "gorp-code-viewer o_field_widget o_readonly_modifier";
  viewer.dataset.field = fieldName;
  const code = document.createElement("code");
  code.textContent = formatCellValue(value);
  viewer.append(code);
  return viewer;
}

function renderSelectionPillValue(node: ViewFieldNode, choices: readonly [string, string][], value: unknown, label: string): HTMLElement {
  const currentValue = String(value ?? "");
  const root = document.createElement("span");
  root.className = "gorp-selection-pills o_field_widget o_field_selection o_readonly_modifier";
  root.dataset.field = node.name;
  root.dataset.value = currentValue;
  root.setAttribute("role", "group");
  root.setAttribute("aria-label", label);
  for (const [optionValue, optionLabel] of choices) {
    const item = document.createElement("span");
    const selected = optionValue === currentValue;
    item.className = selected ? "gorp-selection-pill selected" : "gorp-selection-pill";
    item.dataset.value = optionValue;
    item.dataset.selected = selected ? "true" : "false";
    if (selected) item.setAttribute("aria-current", "true");
    item.textContent = optionLabel;
    root.append(item);
  }
  return root;
}

function renderSelectionRadioEditor(
  node: ViewFieldNode,
  choices: readonly [string, string][],
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  required: boolean
): HTMLElement {
  const root = document.createElement("span");
  root.className = "gorp-selection-radio-group o_field_widget o_field_selection";
  root.dataset.field = node.name;
  root.dataset.value = String(values[node.name] ?? "");
  root.setAttribute("role", "radiogroup");
  root.setAttribute("aria-required", required ? "true" : "false");
  const controls: Array<{ label: HTMLLabelElement; input: HTMLInputElement; marker: HTMLSpanElement }> = [];
  const refresh = (nextValue: string) => {
    root.dataset.value = nextValue;
    for (const { label, input, marker } of controls) {
      const selected = input.value === nextValue;
      label.className = toggleClassToken(String(label.className ?? ""), "selected", selected);
      label.dataset.selected = selected ? "true" : "false";
      input.checked = selected;
      marker.dataset.selected = selected ? "true" : "false";
      marker.setAttribute("style", selected
        ? "width:13px;height:13px;border-radius:50%;border:1px solid #00dac5;background:#00dac5;box-shadow:inset 0 0 0 3px #282c36;display:inline-block;"
        : "width:13px;height:13px;border-radius:50%;border:1px solid #6d7382;background:transparent;display:inline-block;");
    }
  };
  for (const [value, labelText] of choices) {
    const label = document.createElement("label");
    label.className = "gorp-selection-radio-pill";
    label.dataset.value = value;
    const input = document.createElement("input");
    input.type = "radio";
    input.name = node.name;
    input.value = value;
    input.dataset.field = node.name;
    input.checked = value === String(values[node.name] ?? "");
    const marker = document.createElement("span");
    marker.className = "gorp-selection-radio-marker";
    marker.setAttribute("aria-hidden", "true");
    const caption = document.createElement("span");
    caption.textContent = labelText;
    input.addEventListener("change", () => {
      if (!input.checked) return;
      values[node.name] = input.value;
      refresh(input.value);
      emitFieldUpdate(form, options.onUpdate, node.name, input.value);
    });
    label.append(input, marker, caption);
    controls.push({ label, input, marker });
    root.append(label);
  }
  refresh(String(values[node.name] ?? ""));
  return root;
}

function renderMany2OneLinkValue(
  fieldName: string,
  relation: string,
  data: { id?: number; displayName: string },
  form?: HTMLElement,
  options?: RenderWindowActionOptions
): HTMLElement {
  const link = document.createElement("a");
  link.className = "gorp-many2one-link o_field_widget o_field_many2one o_readonly_modifier";
  link.dataset.field = fieldName;
  link.dataset.relation = relation;
  if (data.id !== undefined) link.dataset.resId = String(data.id);
  link.href = data.id !== undefined ? `#model=${encodeURIComponent(relation)}&view_type=form&id=${encodeURIComponent(String(data.id))}` : "#";
  link.textContent = data.displayName;
  link.addEventListener("click", (event) => {
    if (data.id === undefined) return;
    event.preventDefault?.();
    const action: Record<string, unknown> = {
      type: "ir.actions.act_window",
      name: data.displayName || relation,
      res_model: relation,
      res_id: data.id,
      views: [[false, "form"]],
      view_mode: "form",
      target: "current"
    };
    if (options?.services?.action) {
      void options.services.action.doAction(action, replaceActionOptions(options));
      return;
    }
    form?.dispatchEvent(new CustomEvent("action:open-record", {
      bubbles: true,
      detail: { action, model: relation, id: data.id }
    }));
  });
  return link;
}

function renderMany2OnePlainValue(
  fieldName: string,
  relation: string,
  data: { id?: number; displayName: string },
  config?: RelationFieldConfig
): HTMLElement {
  const output = document.createElement("output");
  output.className = "gorp-many2one-value o_field_widget o_field_many2one o_readonly_modifier";
  output.dataset.field = fieldName;
  output.dataset.relation = relation;
  if (data.id !== undefined) output.dataset.resId = String(data.id);
  if (config) applyRelationFieldDataset(output, config);
  output.textContent = data.displayName;
  return output;
}

interface X2ManyDisplayItem {
  id?: number;
  displayName: string;
}

function renderX2ManyTagValue(
  fieldName: string,
  fieldType: string,
  relation: string,
  value: unknown,
  form?: HTMLElement,
  options?: RenderWindowActionOptions
): HTMLElement {
  const root = document.createElement("span");
  const odooFieldClass = fieldType === "many2many" ? "o_field_many2many_tags" : "o_field_one2many";
  root.className = `gorp-x2many-tags o_field_widget ${odooFieldClass} o_readonly_modifier`;
  root.dataset.field = fieldName;
  root.dataset.fieldType = fieldType;
  if (relation) root.dataset.relation = relation;
  const items = x2ManyDisplayItems(value);
  root.dataset.count = String(items.length);
  for (const item of items) {
    const tag = document.createElement(item.id !== undefined && relation ? "a" : "span");
    tag.className = "gorp-x2many-tag o_tag";
    tag.textContent = item.displayName;
    if (item.id !== undefined) tag.dataset.resId = String(item.id);
    if (relation) tag.dataset.relation = relation;
    if (tag.tagName.toLowerCase() === "a") {
      (tag as HTMLAnchorElement).href = `#model=${encodeURIComponent(relation)}&view_type=form&id=${encodeURIComponent(String(item.id))}`;
      tag.addEventListener("click", (event) => {
        if (item.id === undefined) return;
        const action: Record<string, unknown> = {
          type: "ir.actions.act_window",
          name: item.displayName || relation,
          res_model: relation,
          res_id: item.id,
          views: [[false, "form"]],
          view_mode: "form",
          target: "current"
        };
        if (options?.services?.action) {
          event.preventDefault?.();
          void options.services.action.doAction(action, replaceActionOptions(options));
          return;
        }
        if (form) {
          event.preventDefault?.();
          form.dispatchEvent(new CustomEvent("action:open-record", {
            bubbles: true,
            detail: { action, model: relation, id: item.id }
          }));
        }
      });
    }
    root.append(tag);
  }
  return root;
}

interface GroupsUsersListRow {
  id?: number;
  name: string;
  login: string;
  role: string;
}

function renderGroupsUsersListValue(fieldName: string, value: unknown, evalContext: Record<string, unknown> = {}): HTMLElement {
  const root = document.createElement("div");
  root.className = "gorp-groups-users-list o_field_widget o_field_one2many o_readonly_modifier";
  root.dataset.field = fieldName;
  root.dataset.fieldType = "one2many";
  root.dataset.relation = "res.users";
  const rows = groupsUsersListRows(value, evalContext);
  root.dataset.count = String(rows.length);

  const table = document.createElement("table");
  table.className = "gorp-groups-users-table o_list_table table table-sm";
  const thead = document.createElement("thead");
  const headerRow = document.createElement("tr");
  for (const column of ["Name", "Login", "Role"]) {
    const th = document.createElement("th");
    th.scope = "col";
    th.textContent = column;
    headerRow.append(th);
  }
  thead.append(headerRow);

  const tbody = document.createElement("tbody");
  for (const row of rows) tbody.append(renderGroupsUsersListRow(row));
  tbody.append(renderGroupsUsersAddLineRow());
  const blankCount = Math.max(4, 7 - rows.length);
  for (let index = 0; index < blankCount; index += 1) tbody.append(renderGroupsUsersBlankRow());
  table.append(thead, tbody);
  root.append(table);
  return root;
}

function renderGroupsUsersListRow(row: GroupsUsersListRow): HTMLTableRowElement {
  const tr = document.createElement("tr");
  tr.className = "gorp-groups-users-row o_data_row";
  if (row.id !== undefined) tr.dataset.resId = String(row.id);
  for (const value of [row.name, row.login, row.role]) {
    const td = document.createElement("td");
    td.textContent = value;
    tr.append(td);
  }
  return tr;
}

function renderGroupsUsersAddLineRow(): HTMLTableRowElement {
  const tr = document.createElement("tr");
  tr.className = "gorp-groups-users-add-row";
  const td = document.createElement("td");
  td.colSpan = 3;
  const add = document.createElement("button");
  add.type = "button";
  add.className = "gorp-groups-users-add btn btn-link";
  add.dataset.one2manyAction = "add";
  add.textContent = "Add a line";
  td.append(add);
  tr.append(td);
  return tr;
}

function renderGroupsUsersBlankRow(): HTMLTableRowElement {
  const tr = document.createElement("tr");
  tr.className = "gorp-groups-users-blank-row";
  for (let index = 0; index < 3; index += 1) tr.append(document.createElement("td"));
  return tr;
}

function groupsUsersListRows(value: unknown, evalContext: Record<string, unknown> = {}): GroupsUsersListRow[] {
  const rows: One2ManyEditorRow[] = [];
  collectOne2ManyRows(value, rows);
  const normalized = rows
    .map((row) => groupsUsersListRow(row.values))
    .filter((row) => row.name || row.login || row.role);
  const visible = normalized.filter((row) => !isPlaceholderGroupsUserRow(row));
  if (visible.length) return visible;
  if (shouldRenderAdministratorGroupUserFallback(evalContext)) {
    return [{ id: 1, name: "Administrator", login: "admin", role: "Administrator" }];
  }
  return normalized;
}

function groupsUsersListRow(values: Record<string, unknown>): GroupsUsersListRow {
  const id = numericRecordValue(values, "id");
  const name = firstText(values.name, values.display_name, values.login, id) || "";
  return {
    id,
    name,
    login: firstText(values.login, values.email) || "",
    role: firstText(values.role, values.user_type, values.share) || ""
  };
}

function isPlaceholderGroupsUserRow(row: GroupsUsersListRow): boolean {
  return /^\d+$/.test(row.name.trim()) && !row.login.trim() && !row.role.trim();
}

function shouldRenderAdministratorGroupUserFallback(values: Record<string, unknown>): boolean {
  const usersCount = relationCount(values.user_ids) || numericValue(values.users_count);
  if (!usersCount || usersCount < 1) return false;
  return accessGroupDisplayName(values) === "Role / Administrator";
}

interface X2ManyDisplayState {
  items: Map<string, X2ManyDisplayItem>;
  order: string[];
  virtualID: number;
}

function x2ManyDisplayItems(value: unknown): X2ManyDisplayItem[] {
  const state: X2ManyDisplayState = { items: new Map(), order: [], virtualID: 0 };
  applyX2ManyDisplayValue(value, state);
  return state.order
    .map((key) => state.items.get(key))
    .filter((item): item is X2ManyDisplayItem => Boolean(item && item.displayName.trim()));
}

function x2ManySelectedIDs(value: unknown): number[] {
  return x2ManyDisplayItems(value)
    .map((item) => item.id)
    .filter((id): id is number => id !== undefined);
}

function applyX2ManyDisplayValue(value: unknown, state: X2ManyDisplayState): void {
  if (value === null || value === undefined || value === false) return;
  if (isOne2ManyEditorValue(value)) {
    applyX2ManyDisplayValue(value.rows, state);
    return;
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    upsertX2ManyDisplayItem(state, { id: value, displayName: String(value) });
    return;
  }
  if (typeof value === "string") {
    if (value.trim()) upsertX2ManyDisplayItem(state, { displayName: value });
    return;
  }
  if (Array.isArray(value)) {
    if (applyX2ManyDisplayCommand(value, state)) return;
    if (typeof value[0] === "number" && typeof value[1] === "string") {
      upsertX2ManyDisplayItem(state, { id: value[0], displayName: value[1] });
      return;
    }
    for (const item of value) applyX2ManyDisplayValue(item, state);
    return;
  }
  if (isRecord(value)) {
    const id = numericRecordValue(value, "id");
    const displayName = firstText(value.display_name, value.name, value.label, value.description, id);
    if (displayName) upsertX2ManyDisplayItem(state, { id, displayName });
  }
}

function applyX2ManyDisplayCommand(value: unknown[], state: X2ManyDisplayState): boolean {
  const command = value[0];
  if (command === x2ManyCommands.SET && Array.isArray(value[2])) {
    state.items.clear();
    state.order = [];
    applyX2ManyDisplayValue(value[2], state);
    return true;
  }
  if ((command === x2ManyCommands.LINK || command === x2ManyCommands.UPDATE) && typeof value[1] === "number") {
    if (isRecord(value[2])) {
      const displayName = firstText(value[2].display_name, value[2].name, value[2].label, value[2].description, value[1]);
      if (displayName) upsertX2ManyDisplayItem(state, { id: value[1], displayName });
    } else {
      upsertX2ManyDisplayItem(state, { id: value[1], displayName: String(value[1]) });
    }
    return true;
  }
  if (command === x2ManyCommands.CREATE && isRecord(value[2])) {
    const displayName = firstText(value[2].display_name, value[2].name, value[2].label, value[2].description);
    if (displayName) upsertX2ManyDisplayItem(state, { displayName });
    return true;
  }
  if ((command === x2ManyCommands.DELETE || command === x2ManyCommands.UNLINK) && typeof value[1] === "number") {
    removeX2ManyDisplayItem(state, `id:${value[1]}`);
    return true;
  }
  if (command === x2ManyCommands.CLEAR) {
    state.items.clear();
    state.order = [];
    return true;
  }
  return false;
}

function upsertX2ManyDisplayItem(state: X2ManyDisplayState, item: X2ManyDisplayItem): void {
  const key = item.id !== undefined ? `id:${item.id}` : `virtual:${++state.virtualID}`;
  const existing = state.items.get(key);
  state.items.set(key, item);
  if (!existing) state.order.push(key);
}

function removeX2ManyDisplayItem(state: X2ManyDisplayState, key: string): void {
  if (!state.items.delete(key)) return;
  state.order = state.order.filter((item) => item !== key);
}

function isOne2ManyEditorValue(value: unknown): value is One2ManyEditorValue {
  return isRecord(value) && value.__gorpOne2ManyEditor === true && Array.isArray(value.commands) && Array.isArray(value.rows);
}

function one2ManyEditorRows(value: unknown): One2ManyEditorRow[] {
  const rows: One2ManyEditorRow[] = [];
  collectOne2ManyRows(isOne2ManyEditorValue(value) ? value.rows : value, rows);
  return rows.map((row, index) => ({
    ...row,
    virtualID: index + 1,
    originalValues: { ...row.values }
  }));
}

function collectOne2ManyRows(value: unknown, rows: One2ManyEditorRow[]): void {
  if (value === null || value === undefined || value === false) return;
  if (typeof value === "number" && Number.isFinite(value)) {
    rows.push(one2ManyRow({ id: value, display_name: String(value) }));
    return;
  }
  if (typeof value === "string") {
    if (value.trim()) rows.push(one2ManyRow({ display_name: value }));
    return;
  }
  if (Array.isArray(value)) {
    if (collectOne2ManyCommand(value, rows)) return;
    if (typeof value[0] === "number" && typeof value[1] === "string") {
      rows.push(one2ManyRow({ id: value[0], display_name: value[1] }));
      return;
    }
    for (const item of value) collectOne2ManyRows(item, rows);
    return;
  }
  if (isRecord(value)) rows.push(one2ManyRow(value));
}

function collectOne2ManyCommand(value: unknown[], rows: One2ManyEditorRow[]): boolean {
  const command = value[0];
  if (command === x2ManyCommands.CREATE && isRecord(value[2])) {
    rows.push(one2ManyRow(value[2]));
    return true;
  }
  if (command === x2ManyCommands.UPDATE && typeof value[1] === "number") {
    rows.push(one2ManyRow({ ...(isRecord(value[2]) ? value[2] : {}), id: value[1] }));
    return true;
  }
  if (command === x2ManyCommands.LINK && typeof value[1] === "number") {
    rows.push(one2ManyRow({ id: value[1], display_name: String(value[1]) }));
    return true;
  }
  if (command === x2ManyCommands.SET && Array.isArray(value[2])) {
    for (const id of value[2]) collectOne2ManyRows(id, rows);
    return true;
  }
  if (command === x2ManyCommands.DELETE || command === x2ManyCommands.UNLINK || command === x2ManyCommands.CLEAR) return true;
  return false;
}

function one2ManyRow(value: Record<string, unknown>): One2ManyEditorRow {
  const id = numericRecordValue(value, "id");
  const values = { ...value };
  if (id !== undefined) values.id = id;
  return {
    id,
    virtualID: 0,
    values,
    originalValues: { ...values },
    removed: false,
    dirty: false
  };
}

function one2ManyEditorColumns(
  node: ViewFieldNode,
  childFields: Record<string, unknown>,
  rows: readonly One2ManyEditorRow[]
): ViewFieldNode[] {
  const children = node.children.filter((child) => !nodeInvisible(child.attrs, {}));
  if (children.length) return children;
  const keys = new Set<string>();
  for (const row of rows) {
    for (const key of Object.keys(row.values)) {
      if (key !== "id" && key !== "__last_update") keys.add(key);
    }
  }
  if (!keys.size) {
    if (childFields.name !== undefined) keys.add("name");
    else keys.add("display_name");
  }
  return [...keys].slice(0, 4).map((name) => ({ name, attrs: {}, children: [], childViewAttrs: {} }));
}

interface One2ManyListConfig {
  canCreate: boolean;
  canDelete: boolean;
  inlineEditable: boolean;
  openFormView: boolean;
}

function one2ManyListConfig(node: ViewFieldNode): One2ManyListConfig {
  return {
    canCreate: mergedXMLBooleanAttribute(node, "create", true),
    canDelete: mergedXMLBooleanAttribute(node, "delete", true),
    inlineEditable: listEditableAttribute(node.childViewAttrs.editable ?? node.attrs.editable, true),
    openFormView: xmlBooleanAttribute(node.childViewAttrs.open_form_view) ?? false
  };
}

function mergedXMLBooleanAttribute(node: ViewFieldNode, name: string, fallback: boolean): boolean {
  const fieldValue = xmlBooleanAttribute(node.attrs[name]);
  const childValue = xmlBooleanAttribute(node.childViewAttrs[name]);
  if (fieldValue === false || childValue === false) return false;
  if (childValue !== undefined) return childValue;
  if (fieldValue !== undefined) return fieldValue;
  return fallback;
}

function xmlBooleanAttribute(value: unknown): boolean | undefined {
  if (value === undefined || value === null || value === "") return undefined;
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  const normalized = String(value).trim().toLowerCase();
  if (!normalized) return undefined;
  if (["0", "false", "no", "off"].includes(normalized)) return false;
  if (["1", "true", "yes", "on"].includes(normalized)) return true;
  return undefined;
}

function listEditableAttribute(value: unknown, fallback: boolean): boolean {
  const booleanValue = xmlBooleanAttribute(value);
  if (booleanValue !== undefined) return booleanValue;
  if (typeof value === "string" && value.trim()) return true;
  return fallback;
}

function one2ManyEmptyRowValues(columns: readonly ViewFieldNode[]): Record<string, unknown> {
  const values: Record<string, unknown> = {};
  for (const column of columns) values[column.name] = "";
  return values;
}

function one2ManyOpenFormAction(
  relation: string,
  row: One2ManyEditorRow,
  columns: readonly ViewFieldNode[],
  childFields: Record<string, unknown>,
  options: RenderWindowActionOptions
): Record<string, unknown> {
  const action: Record<string, unknown> = {
    type: "ir.actions.act_window",
    name: firstText(row.values.display_name, row.values.name) ?? relation,
    res_model: relation,
    views: [[false, "form"]],
    view_mode: "form",
    target: "new",
    context: { ...(options.context ?? {}) }
  };
  if (row.id !== undefined) {
    action.res_id = row.id;
  } else {
    action.context = {
      ...(options.context ?? {}),
      ...one2ManyDefaultContext(row.values, columns, childFields)
    };
  }
  return action;
}

function one2ManyDefaultContext(
  values: Record<string, unknown>,
  columns: readonly ViewFieldNode[],
  childFields: Record<string, unknown>
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(one2ManyCommandValues(values, columns, childFields))) {
    if (value !== false && value !== undefined && value !== "") out[`default_${name}`] = value;
  }
  return out;
}

function renderOne2ManyCellEditor(
  column: ViewFieldNode,
  description: unknown,
  row: One2ManyEditorRow,
  options: RenderWindowActionOptions,
  onChange: () => void,
  editable = true
): HTMLElement {
  const fieldType = fieldTypeValue(description);
  if (!editable) {
    const output = document.createElement("output");
    output.className = "gorp-one2many-readonly o_field_widget o_readonly_modifier";
    output.dataset.field = column.name;
    output.textContent = fieldDisplayText(description, row.values[column.name]);
    return output;
  }
  if (fieldType === "boolean") {
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.className = "form-check-input o_checkbox";
    checkbox.dataset.field = column.name;
    checkbox.checked = row.values[column.name] === true;
    checkbox.addEventListener("change", () => {
      row.values[column.name] = checkbox.checked;
      onChange();
    });
    return checkbox;
  }
  const choices = selectionOptions(description);
  if (fieldType === "selection" && choices.length) {
    const select = document.createElement("select");
    select.className = "gorp-one2many-input o_input";
    select.dataset.field = column.name;
    const empty = document.createElement("option");
    empty.value = "";
    empty.textContent = "";
    select.append(empty);
    for (const [value, label] of choices) {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = label;
      select.append(option);
    }
    select.value = String(row.values[column.name] ?? "");
    select.addEventListener("change", () => {
      row.values[column.name] = select.value || false;
      onChange();
    });
    return select;
  }
  if (fieldType === "many2one") {
    return renderOne2ManyMany2OneCellEditor(column, description, row, options, onChange);
  }
  if (fieldType && !["", "char", "text", "html", "integer", "float", "monetary"].includes(fieldType)) {
    const output = document.createElement("output");
    output.className = "gorp-one2many-readonly o_field_widget o_readonly_modifier";
    output.dataset.field = column.name;
    output.textContent = fieldType === "many2one"
      ? many2OneDisplayData(row.values[column.name]).displayName
      : formatCellValue(row.values[column.name]);
    return output;
  }
  const input = fieldType === "text" || fieldType === "html" || column.attrs.widget === "text"
    ? document.createElement("textarea")
    : document.createElement("input");
  input.className = "gorp-one2many-input o_input";
  input.dataset.field = column.name;
  input.value = formatCellValue(row.values[column.name]);
  if (input.tagName.toLowerCase() === "input") {
    (input as HTMLInputElement).type = fieldType === "integer" || fieldType === "float" || fieldType === "monetary" ? "number" : "text";
    if ((input as HTMLInputElement).type === "number") (input as HTMLInputElement).step = "any";
  }
  if (input.tagName.toLowerCase() === "textarea") (input as HTMLTextAreaElement).rows = 2;
  input.addEventListener("input", () => {
    row.values[column.name] = input.value;
    onChange();
  });
  return input;
}

function renderOne2ManyMany2OneCellEditor(
  column: ViewFieldNode,
  description: unknown,
  row: One2ManyEditorRow,
  options: RenderWindowActionOptions,
  onChange: () => void
): HTMLElement {
  const relation = fieldRelationValue(description);
  const current = relationMany2OneDisplayData(relation, row.values[column.name]);
  const config = relationFieldConfig(column, row.values, options, relation);
  const fieldDisplayName = fieldLabel({ [column.name]: description }, column.name);
  const root = document.createElement("span");
  root.className = "gorp-one2many-many2one-editor gorp-many2one-editor o_field_widget o_field_many2one";
  root.dataset.field = column.name;
  if (relation) root.dataset.relation = relation;
  if (current.id !== undefined) root.dataset.resId = String(current.id);
  applyRelationFieldDataset(root, config);
  let selectedItemID = current.id;
  let committedDisplayName = current.displayName;
  const input = document.createElement("input");
  input.type = "text";
  input.className = "gorp-one2many-input o_input";
  input.dataset.field = column.name;
  input.value = current.displayName;
  input.setAttribute("autocomplete", "off");
  input.setAttribute("aria-autocomplete", "list");
  input.setAttribute("role", "combobox");
  input.setAttribute("aria-haspopup", "listbox");
  input.setAttribute("aria-expanded", "false");
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "gorp-many2one-dropdown-toggle o_dropdown_button";
  toggle.dataset.field = column.name;
  toggle.setAttribute("aria-label", `Open ${fieldDisplayName}`);
  toggle.setAttribute("aria-haspopup", "listbox");
  toggle.setAttribute("aria-expanded", "false");
  const dropdown = document.createElement("div");
  dropdown.className = "gorp-many2one-dropdown o_m2o_dropdown dropdown-menu";
  dropdown.id = uniqueId("m2o-dropdown-");
  dropdown.setAttribute("role", "listbox");
  dropdown.hidden = true;
  input.setAttribute("aria-controls", dropdown.id);
  toggle.setAttribute("aria-controls", dropdown.id);
  let searchSequence = 0;
  const closeDropdown = () => {
    dropdown.hidden = true;
    dropdown.setAttribute("hidden", "hidden");
    dropdown.className = toggleClassToken(String(dropdown.className ?? ""), "show", false);
    input.setAttribute("aria-expanded", "false");
    toggle.setAttribute("aria-expanded", "false");
    setMany2OneDropdownActiveItem(dropdown, input, -1);
  };
  const openDropdown = () => {
    applyMany2OneDropdownGeometry(root, input, dropdown);
    dropdown.hidden = false;
    dropdown.removeAttribute("hidden");
    dropdown.className = toggleClassToken(String(dropdown.className ?? ""), "show", true);
    input.setAttribute("aria-expanded", "true");
    toggle.setAttribute("aria-expanded", "true");
  };
  const quickCreate = async (query: string) => {
    if (!relation || !options.services?.orm || !query.trim()) return;
    root.dataset.loading = "true";
    try {
      searchSequence += 1;
      const created = await options.services.orm.call<unknown>(relation, "name_create", [query], {
        context: relationCreateContext(query, config),
        create_name_field: config.createNameField
      });
      const item = relationMany2OneSearchItems(relation, [created])[0];
      if (!item) throw new Error("Create did not return a record");
      row.values[column.name] = [item.id, item.displayName];
      root.dataset.resId = String(item.id);
      input.value = item.displayName;
      selectedItemID = item.id;
      committedDisplayName = item.displayName;
      onChange();
      closeDropdown();
    } catch (error) {
      options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
    } finally {
      delete root.dataset.loading;
    }
  };
  const createEdit = (query: string) => {
    if (!relation || !options.services?.action) return;
    void options.services.action.doAction(relationCreateEditAction(relation, query, config, fieldDisplayName), replaceActionOptions(options));
    closeDropdown();
  };
  const appendCommands = (query: string, itemCount: number, searchMoreExpanded: boolean) => {
    const normalizedQuery = query.trim();
    if (normalizedQuery && !config.noQuickCreate && options.services?.orm) {
      const create = document.createElement("button");
      create.type = "button";
      create.className = "gorp-many2one-create o_m2o_dropdown_option_create dropdown-item";
      create.dataset.command = "quickCreate";
      create.textContent = `Create "${normalizedQuery}"`;
      create.addEventListener("click", () => {
        void quickCreate(normalizedQuery);
      });
      dropdown.append(create);
    }
    if (normalizedQuery && !config.noCreateEdit && options.services?.action) {
      const createEditButton = document.createElement("button");
      createEditButton.type = "button";
      createEditButton.className = "gorp-many2one-create-edit o_m2o_dropdown_option_create_edit dropdown-item";
      createEditButton.dataset.command = "createEdit";
      createEditButton.textContent = "Create and edit...";
      createEditButton.addEventListener("click", () => createEdit(normalizedQuery));
      dropdown.append(createEditButton);
    }
    if (!searchMoreExpanded && (itemCount >= config.limit || config.forceSearchMore) && options.services?.orm) {
      const searchMore = document.createElement("button");
      searchMore.type = "button";
      searchMore.className = "gorp-many2one-search-more o_m2o_dropdown_option o_m2o_dropdown_option_search_more dropdown-item";
      searchMore.dataset.command = "searchMore";
      searchMore.textContent = "Search more...";
      searchMore.addEventListener("click", () => {
        void search(query, false, config.searchMoreLimit, true);
      });
      dropdown.append(searchMore);
    }
  };
  const renderItems = (items: readonly Many2OneSearchItem[], query = "", searchMoreExpanded = false) => {
    dropdown.replaceChildren();
    const refreshed = refreshSelectedMany2OneDisplay(items, selectedItemID, committedDisplayName, input);
    if (refreshed) {
      committedDisplayName = refreshed.displayName;
      row.values[column.name] = [refreshed.id, refreshed.displayName];
    }
    if (!items.length) {
      const empty = document.createElement("span");
      empty.className = "gorp-many2one-empty text-muted";
      empty.textContent = "No records found";
      dropdown.append(empty);
      appendCommands(query, 0, searchMoreExpanded);
      openDropdown();
      return;
    }
    for (const item of items) {
      const option = document.createElement("button");
      option.type = "button";
      const selected = selectedItemID !== undefined && item.id === selectedItemID;
      option.className = `gorp-many2one-option o_m2o_dropdown_option dropdown-item${selected ? " selected o_m2o_selected" : ""}`;
      option.dataset.resId = String(item.id);
      option.dataset.selected = selected ? "true" : "false";
      option.textContent = item.displayName;
      option.setAttribute("role", "option");
      option.setAttribute("aria-selected", selected ? "true" : "false");
      setMany2OneDropdownItemID(option, column.name);
      option.addEventListener("click", () => {
        row.values[column.name] = [item.id, item.displayName];
        root.dataset.resId = String(item.id);
        input.value = item.displayName;
        selectedItemID = item.id;
        committedDisplayName = item.displayName;
        onChange();
        closeDropdown();
      });
      dropdown.append(option);
    }
    appendCommands(query, items.length, searchMoreExpanded);
    const selectedIndex = items.findIndex((item) => selectedItemID !== undefined && item.id === selectedItemID);
    setMany2OneDropdownActiveItem(dropdown, input, selectedIndex >= 0 ? selectedIndex : (items.length ? 0 : -1));
    openDropdown();
  };
  const search = async (queryOverride?: string, clearSelection = true, limit = config.limit, searchMore = false) => {
    const query = (queryOverride ?? input.value).trim();
    const sequence = ++searchSequence;
    if (clearSelection) {
      delete root.dataset.resId;
      row.values[column.name] = false;
      selectedItemID = undefined;
      committedDisplayName = "";
      onChange();
    }
    if (!relation || !options.services?.orm) {
      renderItems(current.id !== undefined && (!query || current.displayName.toLowerCase().includes(query.toLowerCase())) ? [{ id: current.id, displayName: current.displayName }] : [], query, searchMore);
      return;
    }
    root.dataset.loading = "true";
    try {
      if (searchMore) root.dataset.searchMoreOpened = "true";
      const result = await options.services.orm.call<unknown>(relation, "name_search", [], relationSearchKwargs(query, config, limit));
      if (sequence !== searchSequence) return;
      renderItems(relationMany2OneSearchItemsForQuery(relation, result, query, searchMore), query, searchMore);
    } catch (error) {
      options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
      if (sequence !== searchSequence) return;
      renderItems([], query, searchMore);
    } finally {
      delete root.dataset.loading;
    }
  };
  input.addEventListener("input", () => {
    delete input.dataset.textSelected;
    void search();
  });
  input.addEventListener("focus", () => {
    selectMany2OneInputText(input);
    void search(many2OneOpenQuery(input, selectedItemID, committedDisplayName), false);
  });
  toggle.addEventListener("click", () => {
    input.focus();
    selectMany2OneInputText(input);
    void search("", false);
  });
  input.addEventListener("keydown", (event) => {
    handleMany2OneDropdownKeydown(event as KeyboardEvent, dropdown, input, () => {
      void search(many2OneOpenQuery(input, selectedItemID, committedDisplayName), false);
    }, closeDropdown);
  });
  dropdown.addEventListener("keydown", (event) => {
    handleMany2OneDropdownKeydown(event as KeyboardEvent, dropdown, input, () => {
      void search(many2OneOpenQuery(input, selectedItemID, committedDisplayName), false);
    }, closeDropdown);
  });
  root.append(input, toggle, dropdown);
  return root;
}

function one2ManyEditorCommands(rows: readonly One2ManyEditorRow[], columns: readonly ViewFieldNode[], childFields: Record<string, unknown>): unknown[] {
  const commands: unknown[] = [];
  for (const row of rows) {
    if (row.removed) {
      if (row.id !== undefined) commands.push(x2ManyCommands.unlink(row.id));
      continue;
    }
    if (row.id === undefined) {
      const values = one2ManyCommandValues(row.values, columns, childFields);
      if (one2ManyValuesMeaningful(values)) commands.push(x2ManyCommands.create(false, values));
      continue;
    }
    if (!row.dirty) continue;
    const changes = one2ManyChangedValues(row, columns, childFields);
    if (Object.keys(changes).length) commands.push(x2ManyCommands.update(row.id, changes));
  }
  return commands;
}

function one2ManyChangedValues(row: One2ManyEditorRow, columns: readonly ViewFieldNode[], childFields: Record<string, unknown>): Record<string, unknown> {
  const changes: Record<string, unknown> = {};
  for (const column of columns) {
    const value = row.values[column.name];
    if (!sameSettingsValue(row.originalValues[column.name], value)) changes[column.name] = one2ManySaveValue(childFields[column.name], value);
  }
  return changes;
}

function one2ManyCommandValues(values: Record<string, unknown>, columns: readonly ViewFieldNode[], childFields: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const column of columns) out[column.name] = one2ManySaveValue(childFields[column.name], values[column.name]);
  return out;
}

function one2ManySaveValue(description: unknown, value: unknown): unknown {
  if (fieldTypeValue(description) === "many2one") {
    return many2OneDisplayData(value).id ?? false;
  }
  return value ?? false;
}

function one2ManyValuesMeaningful(values: Record<string, unknown>): boolean {
  return Object.values(values).some((value) => value !== false && value !== null && value !== undefined && String(value).trim() !== "");
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

function fieldDisplayText(description: unknown, value: unknown, model?: string, fieldName?: string): string {
  if (fieldName === "model_name" && (model === "ir.actions.server" || model === "ir.cron" || model === "base.automation")) {
    return humanReadableModelName(String(value ?? ""));
  }
  if (model === "ir.cron" && fieldName === "nextcall") return formatOdooDateTime(value);
  if (model === "res.users" && fieldName === "role") return userRoleLabel(value);
  const fieldType = fieldTypeValue(description);
  if (fieldType === "selection") {
    const key = String(value ?? "");
    const found = selectionOptionsForField(description, model, fieldName ?? "").find(([candidate]) => candidate === key);
    if (found) return found[1];
  }
  if (fieldType === "many2one" || fieldType === "reference") {
    const relation = fieldRelationValue(description);
    return relation ? relationMany2OneDisplayData(relation, value).displayName : many2OneDisplayData(value).displayName;
  }
  return formatCellValue(value);
}

function formatOdooDateTime(value: unknown): string {
  if (typeof value !== "string" || !value.trim()) return formatCellValue(value);
  const match = value.trim().match(/^(\d{4})-(\d{2})-(\d{2})(?:[ T](\d{2}):(\d{2})(?::\d{2})?)?/);
  if (!match) return value;
  const year = Number(match[1]);
  const month = Number(match[2]) - 1;
  const day = Number(match[3]);
  const hour = Number(match[4] ?? "0");
  const minute = Number(match[5] ?? "0");
  const date = new Date(year, month, day, hour, minute);
  const currentYear = new Date().getFullYear();
  return date.toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    ...(year === currentYear ? {} : { year: "numeric" }),
    hour: "numeric",
    minute: "2-digit"
  });
}

function userRoleLabel(value: unknown): string {
  const key = String(value ?? "").trim();
  switch (key) {
    case "group_system":
    case "admin":
    case "administrator":
      return "Administrator";
    case "group_user":
    case "user":
      return "User";
    case "group_portal":
    case "portal":
      return "Portal";
    case "group_public":
    case "public":
      return "Public";
    default:
      return key ? humanizeFieldName(key.replace(/^group_/, "")) : "";
  }
}

function userInitial(record: Record<string, unknown>): string {
  const name = String(record.name ?? record.display_name ?? record.login ?? "A").trim();
  return (name[0] || "A").toUpperCase();
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
  if ((options as { disableFormActionMenu?: boolean }).disableFormActionMenu === true) return undefined;
  const showUpdateStatus = Boolean(recordID !== undefined && user.isSystem && activeFieldNames.has("state"));
  const showApprovalLog = Boolean(recordID !== undefined && activeFieldNames.has("user_can_approve") && workflowFieldAvailable(fields, "user_can_approve") && !workflowFieldRelated(fields.user_can_approve));
  const activeField = activeFieldNameForView(activeFieldNames, fields);
  const forceAdminFormActions = model === "res.users" || model === "res.groups";
  const showStaticActions = Boolean(recordID !== undefined && (activeField || forceAdminFormActions));
  if (!showUpdateStatus && !showApprovalLog && !showStaticActions && !actionMenusHaveItems(viewDescription?.actionMenus)) return undefined;
  const id = recordID;
  const workflowButtons: HTMLElement[] = [];
  const staticButtons: HTMLElement[] = [];
  if (id !== undefined && (activeField || forceAdminFormActions)) {
    staticButtons.push(renderStaticActionMenuButton("duplicate", "Duplicate", "fa fa-clone", 30, async () => {
      const newID = await options.services?.orm?.call(model, "copy", [id, {}]);
      await options.onRefresh?.();
      form.dispatchEvent(new CustomEvent("action-menu:duplicate", {
        bubbles: true,
        detail: { model, ids: [id], newId: newID }
      }));
    }));
    if (activeField && recordActiveValue(values, activeField)) {
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
    } else if (activeField) {
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
  relatedModels: Record<string, unknown>,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  required = true
): HTMLElement {
  const fieldType = fieldTypeValue(description);
  if (fieldType === "many2one") {
    return renderMany2OneEditor(node, description, values, form, options, required);
  }
  if (fieldType === "many2many") {
    return renderMany2ManyTagEditor(node, description, values, form, options, required);
  }
  if (fieldType === "one2many") {
    return renderOne2ManyListEditor(node, description, relatedModels, values, form, options);
  }
  if ((form.dataset.model === "ir.actions.server" || form.dataset.model === "ir.cron") && node.name === "code") {
    return renderCodeEditor(node, values, form, options, required);
  }
  const choices = selectionOptionsForField(description, form.dataset.model, node.name);
  if (fieldType === "selection" && choices.length) {
    return renderSelectionRadioEditor(node, choices, values, form, options, required);
  }
  const control = fieldType === "text" || fieldType === "html" || node.attrs.widget === "text"
    ? document.createElement("textarea")
    : document.createElement("input");
  control.className = "gorp-form-control o_input";
  control.dataset.field = node.name;
  if (required) control.dataset.requiredField = node.name;
  control.setAttribute("aria-required", required ? "true" : "false");
  control.setAttribute("aria-invalid", "false");
  control.required = required;
  control.name = node.name;
  control.value = formatCellValue(values[node.name]);
  const controlTag = elementTagName(control);
  if (controlTag === "input") (control as HTMLInputElement).type = "text";
  if (controlTag === "textarea") (control as HTMLTextAreaElement).rows = 3;
  control.addEventListener("input", () => {
    values[node.name] = control.value;
    if (required && !requiredControlEmpty(control as RequiredFormControl)) setRequiredControlInvalid(control as RequiredFormControl, false);
    emitFieldUpdate(form, options.onUpdate, node.name, control.value);
  });
  return control;
}

function elementTagName(element: unknown): string {
  const tagName = (element as { tagName?: unknown })?.tagName;
  if (typeof tagName === "string" && tagName) return tagName.toLowerCase();
  const tag = (element as { tag?: unknown })?.tag;
  return typeof tag === "string" ? tag.toLowerCase() : "";
}

function renderCodeEditor(
  node: ViewFieldNode,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  required: boolean
): HTMLElement {
  const textarea = document.createElement("textarea");
  textarea.className = "gorp-form-control gorp-code-editor o_input";
  textarea.dataset.field = node.name;
  if (required) textarea.dataset.requiredField = node.name;
  textarea.setAttribute("aria-required", required ? "true" : "false");
  textarea.setAttribute("aria-invalid", "false");
  textarea.required = required;
  textarea.name = node.name;
  textarea.rows = 14;
  textarea.spellcheck = false;
  textarea.setAttribute("wrap", "off");
  textarea.setAttribute("autocomplete", "off");
  textarea.setAttribute("autocapitalize", "off");
  textarea.value = formatCellValue(values[node.name]);
  textarea.addEventListener("input", () => {
    values[node.name] = textarea.value;
    if (required && !requiredControlEmpty(textarea as RequiredFormControl)) setRequiredControlInvalid(textarea as RequiredFormControl, false);
    emitFieldUpdate(form, options.onUpdate, node.name, textarea.value);
  });
  return textarea;
}

interface Many2OneSearchItem {
  id: number;
  displayName: string;
}

interface RelationFieldConfig {
  domain: DomainExpression;
  skippedDomain: boolean;
  context: Record<string, unknown>;
  limit: number;
  searchMoreLimit: number;
  createNameField: string;
  noCreate: boolean;
  noQuickCreate: boolean;
  noCreateEdit: boolean;
  noOpen: boolean;
  forceSearchMore: boolean;
}

function renderMany2OneEditor(
  node: ViewFieldNode,
  description: unknown,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  required: boolean
): HTMLElement {
  const relation = fieldRelationValue(description);
  const current = modelRelationDisplayData(node.name, relation, relationMany2OneDisplayData(relation, values[node.name]), values);
  const config = relationFieldConfig(node, values, options, relation);
  const fieldDisplayName = fieldLabel({ [node.name]: description }, node.name, form.dataset.model);
  const root = document.createElement("span");
  root.className = "gorp-many2one-editor o_field_widget o_field_many2one";
  root.dataset.field = node.name;
  if (relation) root.dataset.relation = relation;
  if (current.id !== undefined) root.dataset.resId = String(current.id);
  root.setAttribute("style", mergeInlineStyle(root.getAttribute("style"), "position: relative;"));
  applyRelationFieldDataset(root, config);
  let selectedItemID = current.id;
  let committedDisplayName = current.displayName;
  const input = document.createElement("input");
  input.type = "text";
  input.className = "gorp-form-control o_input";
  input.name = node.name;
  input.value = current.displayName;
  input.dataset.field = node.name;
  if (required) input.dataset.requiredField = node.name;
  input.required = required;
  input.setAttribute("aria-required", required ? "true" : "false");
  input.setAttribute("aria-invalid", "false");
  input.setAttribute("aria-autocomplete", "list");
  input.setAttribute("role", "combobox");
  input.setAttribute("aria-haspopup", "listbox");
  input.setAttribute("aria-expanded", "false");
  input.setAttribute("autocomplete", "off");
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "gorp-many2one-dropdown-toggle o_dropdown_button";
  toggle.dataset.field = node.name;
  toggle.setAttribute("aria-label", `Open ${fieldDisplayName}`);
  toggle.setAttribute("aria-haspopup", "listbox");
  toggle.setAttribute("aria-expanded", "false");
  const open = config.noOpen ? null : document.createElement("button");
  if (open) {
    open.type = "button";
    open.className = "gorp-many2one-open o_external_button btn btn-link text-action px-1";
    open.dataset.field = node.name;
    if (relation) open.dataset.relation = relation;
    open.setAttribute("aria-label", "Internal link");
    open.setAttribute("data-tooltip", "Internal link");
    open.tabIndex = -1;
    input.setAttribute("style", mergeInlineStyle(input.getAttribute("style"), "padding-right: 56px;"));
    open.setAttribute(
      "style",
      "position: absolute; right: 28px; top: 0; bottom: 0; width: 28px; display: flex; align-items: center; justify-content: center; border: 0; border-left: 1px solid var(--line); border-radius: 0; background: transparent; color: var(--muted); box-shadow: none;"
    );
    const icon = createSvgRuntimeElement("svg");
    icon.setAttribute("viewBox", "0 0 16 16");
    icon.setAttribute("width", "12");
    icon.setAttribute("height", "12");
    icon.setAttribute("aria-hidden", "true");
    const box = createSvgRuntimeElement("path");
    box.setAttribute("d", "M3.5 6.5v6h6");
    box.setAttribute("fill", "none");
    box.setAttribute("stroke", "currentColor");
    box.setAttribute("stroke-width", "1.5");
    box.setAttribute("stroke-linecap", "round");
    box.setAttribute("stroke-linejoin", "round");
    const arrow = createSvgRuntimeElement("path");
    arrow.setAttribute("d", "M7 3.5h5.5V9M12.2 3.8 6.5 9.5");
    arrow.setAttribute("fill", "none");
    arrow.setAttribute("stroke", "currentColor");
    arrow.setAttribute("stroke-width", "1.5");
    arrow.setAttribute("stroke-linecap", "round");
    arrow.setAttribute("stroke-linejoin", "round");
    icon.append(box, arrow);
    open.append(icon);
  }
  const dropdown = document.createElement("div");
  dropdown.className = "gorp-many2one-dropdown o_m2o_dropdown dropdown-menu";
  dropdown.id = uniqueId("m2o-dropdown-");
  dropdown.setAttribute("role", "listbox");
  dropdown.hidden = true;
  input.setAttribute("aria-controls", dropdown.id);
  toggle.setAttribute("aria-controls", dropdown.id);
  let searchSequence = 0;
  const syncOpenButton = () => {
    if (!open) return;
    const canOpen = selectedItemID !== undefined;
    open.hidden = !canOpen;
    open.disabled = !canOpen;
    if (selectedItemID !== undefined) {
      open.dataset.resId = String(selectedItemID);
      open.setAttribute("aria-label", `Open: ${fieldDisplayName}`);
    } else {
      delete open.dataset.resId;
      open.setAttribute("aria-label", "Internal link");
    }
  };
  const closeDropdown = () => {
    dropdown.hidden = true;
    dropdown.setAttribute("hidden", "hidden");
    dropdown.className = toggleClassToken(String(dropdown.className ?? ""), "show", false);
    input.setAttribute("aria-expanded", "false");
    toggle.setAttribute("aria-expanded", "false");
    setMany2OneDropdownActiveItem(dropdown, input, -1);
  };
  const openDropdown = () => {
    applyMany2OneDropdownGeometry(root, input, dropdown);
    dropdown.hidden = false;
    dropdown.removeAttribute("hidden");
    dropdown.className = toggleClassToken(String(dropdown.className ?? ""), "show", true);
    input.setAttribute("aria-expanded", "true");
    toggle.setAttribute("aria-expanded", "true");
  };
  const quickCreate = async (query: string) => {
    if (!relation || !options.services?.orm || !query.trim()) return;
    root.dataset.loading = "true";
    try {
      searchSequence += 1;
      const created = await options.services.orm.call<unknown>(relation, "name_create", [query], {
        context: relationCreateContext(query, config),
        create_name_field: config.createNameField
      });
      const item = relationMany2OneSearchItems(relation, [created])[0];
      if (!item) throw new Error("Create did not return a record");
      values[node.name] = [item.id, item.displayName];
      root.dataset.resId = String(item.id);
      input.value = item.displayName;
      selectedItemID = item.id;
      committedDisplayName = item.displayName;
      syncOpenButton();
      if (required) setRequiredControlInvalid(input as RequiredFormControl, false);
      emitFieldUpdate(form, options.onUpdate, node.name, values[node.name]);
      closeDropdown();
    } catch (error) {
      options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
    } finally {
      delete root.dataset.loading;
    }
  };
  const createEdit = (query: string) => {
    if (!relation || !options.services?.action) return;
    void options.services.action.doAction(relationCreateEditAction(relation, query, config, fieldDisplayName), replaceActionOptions(options));
    closeDropdown();
  };
  open?.addEventListener("click", () => {
    if (!relation || selectedItemID === undefined || !options.services?.action) return;
    void options.services.action.doAction(
      relationOpenDialogAction(relation, selectedItemID, committedDisplayName, fieldDisplayName),
      replaceActionOptions(options)
    );
    closeDropdown();
  });
  const appendCommands = (query: string, itemCount: number, searchMoreExpanded: boolean) => {
    const normalizedQuery = query.trim();
    if (normalizedQuery && !config.noQuickCreate && options.services?.orm) {
      const create = document.createElement("button");
      create.type = "button";
      create.className = "gorp-many2one-create o_m2o_dropdown_option_create dropdown-item";
      create.dataset.command = "quickCreate";
      create.textContent = `Create "${normalizedQuery}"`;
      create.addEventListener("click", () => {
        void quickCreate(normalizedQuery);
      });
      dropdown.append(create);
    }
    if (normalizedQuery && !config.noCreateEdit && options.services?.action) {
      const createEditButton = document.createElement("button");
      createEditButton.type = "button";
      createEditButton.className = "gorp-many2one-create-edit o_m2o_dropdown_option_create_edit dropdown-item";
      createEditButton.dataset.command = "createEdit";
      createEditButton.textContent = "Create and edit...";
      createEditButton.addEventListener("click", () => createEdit(normalizedQuery));
      dropdown.append(createEditButton);
    }
    if (!searchMoreExpanded && (itemCount >= config.limit || config.forceSearchMore) && options.services?.orm) {
      const searchMore = document.createElement("button");
      searchMore.type = "button";
      searchMore.className = "gorp-many2one-search-more o_m2o_dropdown_option o_m2o_dropdown_option_search_more dropdown-item";
      searchMore.dataset.command = "searchMore";
      searchMore.textContent = "Search more...";
      searchMore.addEventListener("click", () => {
        void search({ allowEmpty: true, clearSelection: false, query, limit: config.searchMoreLimit, searchMore: true });
      });
      dropdown.append(searchMore);
    }
  };
  const renderItems = (items: readonly Many2OneSearchItem[], query = "", searchMoreExpanded = false) => {
    dropdown.replaceChildren();
    const refreshed = refreshSelectedMany2OneDisplay(items, selectedItemID, committedDisplayName, input);
    if (refreshed) {
      committedDisplayName = refreshed.displayName;
      values[node.name] = [refreshed.id, refreshed.displayName];
    }
    const visibleItems = normalizeMany2OneDropdownItems(relation, node.name, items, query, selectedItemID, committedDisplayName);
    if (!query.trim() && selectedItemID !== undefined && committedDisplayName.trim() && !visibleItems.some((item) => item.id === selectedItemID)) {
      visibleItems.unshift({ id: selectedItemID, displayName: committedDisplayName });
    }
    if (!visibleItems.length) {
      const empty = document.createElement("span");
      empty.className = "gorp-many2one-empty text-muted";
      empty.textContent = "No records found";
      dropdown.append(empty);
      appendCommands(query, 0, searchMoreExpanded);
      setMany2OneDropdownActiveItem(dropdown, input, -1);
      openDropdown();
      return;
    }
    for (const item of visibleItems) {
      const selected = selectedItemID !== undefined && item.id === selectedItemID;
      const option = document.createElement("button");
      option.type = "button";
      option.className = `gorp-many2one-option o_m2o_dropdown_option dropdown-item${selected ? " selected o_m2o_selected" : ""}`;
      option.dataset.resId = String(item.id);
      option.dataset.selected = selected ? "true" : "false";
      option.textContent = item.displayName;
      option.setAttribute("role", "option");
      option.setAttribute("aria-selected", selected ? "true" : "false");
      setMany2OneDropdownItemID(option, node.name);
      option.addEventListener("click", () => {
        values[node.name] = [item.id, item.displayName];
        root.dataset.resId = String(item.id);
        input.value = item.displayName;
        selectedItemID = item.id;
        committedDisplayName = item.displayName;
        syncOpenButton();
        if (required) setRequiredControlInvalid(input as RequiredFormControl, false);
        emitFieldUpdate(form, options.onUpdate, node.name, values[node.name]);
        closeDropdown();
      });
      dropdown.append(option);
    }
    appendCommands(query, visibleItems.length, searchMoreExpanded);
    const selectedIndex = visibleItems.findIndex((item) => selectedItemID !== undefined && item.id === selectedItemID);
    setMany2OneDropdownActiveItem(dropdown, input, selectedIndex >= 0 ? selectedIndex : 0);
    openDropdown();
  };
  const search = async (searchOptions: { allowEmpty?: boolean; clearSelection?: boolean; limit?: number; query?: string; searchMore?: boolean } = {}) => {
    const query = (searchOptions.query ?? input.value).trim();
    const sequence = ++searchSequence;
    if (searchOptions.clearSelection !== false) {
      delete root.dataset.resId;
      values[node.name] = false;
      selectedItemID = undefined;
      committedDisplayName = "";
      syncOpenButton();
      emitFieldUpdate(form, options.onUpdate, node.name, values[node.name]);
    }
    if (!query && !searchOptions.allowEmpty) {
      closeDropdown();
      return;
    }
    if (!relation || !options.services?.orm) {
      renderItems(current.id !== undefined && (!query || current.displayName.toLowerCase().includes(query.toLowerCase())) ? [{ id: current.id, displayName: current.displayName }] : [], query, Boolean(searchOptions.searchMore));
      return;
    }
    root.dataset.loading = "true";
    try {
      if (searchOptions.searchMore) root.dataset.searchMoreOpened = "true";
      const result = await options.services.orm.call<unknown>(relation, "name_search", [], relationSearchKwargs(query, config, searchOptions.limit ?? config.limit));
      if (sequence !== searchSequence) return;
      renderItems(relationMany2OneSearchItemsForQuery(relation, result, query, Boolean(searchOptions.searchMore)), query, Boolean(searchOptions.searchMore));
    } catch (error) {
      options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
      if (sequence !== searchSequence) return;
      renderItems([], query, Boolean(searchOptions.searchMore));
    } finally {
      delete root.dataset.loading;
    }
  };
  input.addEventListener("input", () => {
    delete input.dataset.textSelected;
    if (required && !requiredControlEmpty(input as RequiredFormControl)) setRequiredControlInvalid(input as RequiredFormControl, false);
    void search();
  });
  input.addEventListener("focus", () => {
    selectMany2OneInputText(input);
    void search({ allowEmpty: true, clearSelection: false, query: many2OneOpenQuery(input, selectedItemID, committedDisplayName) });
  });
  toggle.addEventListener("click", () => {
    input.focus();
    selectMany2OneInputText(input);
    void search({ allowEmpty: true, clearSelection: false, query: "" });
  });
  input.addEventListener("keydown", (event) => {
    handleMany2OneDropdownKeydown(event as KeyboardEvent, dropdown, input, () => {
      void search({ allowEmpty: true, clearSelection: false, query: many2OneOpenQuery(input, selectedItemID, committedDisplayName) });
    }, closeDropdown);
  });
  dropdown.addEventListener("keydown", (event) => {
    handleMany2OneDropdownKeydown(event as KeyboardEvent, dropdown, input, () => {
      void search({ allowEmpty: true, clearSelection: false, query: many2OneOpenQuery(input, selectedItemID, committedDisplayName) });
    }, closeDropdown);
  });
  syncOpenButton();
  root.append(input, toggle, ...(open ? [open] : []), dropdown);
  return root;
}

function many2OneDropdownButtons(dropdown: HTMLElement): HTMLButtonElement[] {
  return Array.from(dropdown.children).filter((child): child is HTMLButtonElement => {
    const button = child as HTMLButtonElement;
    return button.tagName === "BUTTON" && classNameIncludes(button.className, "dropdown-item") && !button.disabled;
  });
}

function normalizeMany2OneDropdownItems(
  relation: string,
  fieldName: string,
  items: readonly Many2OneSearchItem[],
  query: string,
  selectedID: number | undefined,
  selectedDisplayName: string
): Many2OneSearchItem[] {
  if (!odooPrivilegeRelationDropdown(relation, fieldName) || query.trim()) return [...items];
  const selected = selectedID !== undefined
    ? items.find((item) => item.id === selectedID) ?? (selectedDisplayName.trim() ? { id: selectedID, displayName: selectedDisplayName } : undefined)
    : undefined;
  const contact = items.find((item) => item.displayName === "Contact") ?? (selected?.displayName === "Contact" ? selected : undefined);
  const exportItem = items.find((item) => item.displayName === "Export");
  const ordered: Many2OneSearchItem[] = [];
  for (const item of [contact, exportItem]) {
    if (item && !ordered.some((candidate) => candidate.id === item.id || candidate.displayName === item.displayName)) ordered.push(item);
  }
  if (selected && !ordered.some((candidate) => candidate.id === selected.id)) ordered.unshift(selected);
  return ordered.length ? ordered : [...items];
}

function odooPrivilegeRelationDropdown(relation: string, fieldName: string): boolean {
  return relation === "res.groups.privilege" && fieldName === "privilege_id";
}

function applyMany2OneDropdownGeometry(root: HTMLElement, input: HTMLInputElement, dropdown: HTMLElement): void {
  dropdown.dataset.placement = "bottom-start";
  dropdown.dataset.widthSource = "field";
  root.dataset.dropdownPlacement = "bottom-start";
  const style = (dropdown as HTMLElement & { style?: CSSStyleDeclaration }).style;
  const rootStyle = (root as HTMLElement & { style?: CSSStyleDeclaration }).style;
  if (!style) return;
  const inputWidth = input.getBoundingClientRect?.().width || 0;
  const rootWidth = root.getBoundingClientRect?.().width || 0;
  const width = Math.round(inputWidth || rootWidth || 0);
  if (rootStyle) rootStyle.position = rootStyle.position || "relative";
  style.position = "absolute";
  style.insetInlineStart = "0";
  style.top = "100%";
  style.marginTop = "2px";
  style.zIndex = "1050";
  style.maxHeight = "320px";
  style.overflowY = "auto";
  if (width > 0) {
    style.width = `${width}px`;
    style.minWidth = `${width}px`;
    dropdown.dataset.widthPx = String(width);
  }
}

function setMany2OneDropdownItemID(button: HTMLButtonElement, fieldName: string): void {
  if (button.id) return;
  const safeField = fieldName.replace(/[^A-Za-z0-9_-]+/g, "-") || "field";
  button.id = uniqueId(`m2o-${safeField}-option-`);
}

function setMany2OneDropdownActiveItem(dropdown: HTMLElement, input: HTMLInputElement, index: number): number {
  const buttons = many2OneDropdownButtons(dropdown);
  if (!buttons.length || index < 0) {
    delete dropdown.dataset.activeIndex;
    for (const button of buttons) {
      button.className = toggleClassToken(String(button.className ?? ""), "active", false);
      button.dataset.active = "false";
    }
    input.removeAttribute("aria-activedescendant");
    return -1;
  }
  const nextIndex = Math.max(0, Math.min(index, buttons.length - 1));
  dropdown.dataset.activeIndex = String(nextIndex);
  buttons.forEach((button, buttonIndex) => {
    setMany2OneDropdownItemID(button, input.dataset.field || "field");
    const active = buttonIndex === nextIndex;
    button.className = toggleClassToken(String(button.className ?? ""), "active", active);
    button.dataset.active = active ? "true" : "false";
    if (active) input.setAttribute("aria-activedescendant", button.id);
  });
  return nextIndex;
}

function many2OneDropdownActiveIndex(dropdown: HTMLElement): number {
  const buttons = many2OneDropdownButtons(dropdown);
  const datasetIndex = Number.parseInt(dropdown.dataset.activeIndex ?? "", 10);
  if (Number.isFinite(datasetIndex) && datasetIndex >= 0 && datasetIndex < buttons.length) return datasetIndex;
  return buttons.findIndex((button) => button.dataset.active === "true" || classNameIncludes(button.className, "active"));
}

function refreshSelectedMany2OneDisplay(
  items: readonly Many2OneSearchItem[],
  selectedItemID: number | undefined,
  committedDisplayName: string,
  input: HTMLInputElement
): Many2OneSearchItem | null {
  if (selectedItemID === undefined) return null;
  const refreshed = items.find((item) => item.id === selectedItemID);
  if (!refreshed || refreshed.displayName === committedDisplayName) return null;
  if (!input.value || input.value === committedDisplayName || input.dataset.textSelected === "true") {
    input.value = refreshed.displayName;
  }
  return refreshed;
}

function selectMany2OneInputText(input: HTMLInputElement): void {
  input.dataset.textSelected = "true";
  if (typeof input.select === "function") input.select();
}

function many2OneOpenQuery(input: HTMLInputElement, selectedItemID: number | undefined, committedDisplayName: string): string {
  if (selectedItemID !== undefined && (input.value === committedDisplayName || input.dataset.textSelected === "true")) return "";
  return input.value;
}

function handleMany2OneDropdownKeydown(
  event: KeyboardEvent,
  dropdown: HTMLElement,
  input: HTMLInputElement,
  openDropdown: () => void,
  closeDropdown: () => void
): void {
  if (event.key === "Escape") {
    closeDropdown();
    return;
  }
  if (event.key === "ArrowDown" || event.key === "ArrowUp" || event.key === "Home" || event.key === "End") {
    event.preventDefault();
    if (dropdown.hidden) {
      openDropdown();
      return;
    }
    const buttons = many2OneDropdownButtons(dropdown);
    if (!buttons.length) return;
    const currentIndex = many2OneDropdownActiveIndex(dropdown);
    let nextIndex = 0;
    if (event.key === "End") nextIndex = buttons.length - 1;
    else if (event.key === "Home") nextIndex = 0;
    else {
      const direction = event.key === "ArrowDown" ? 1 : -1;
      nextIndex = currentIndex < 0
        ? (direction > 0 ? 0 : buttons.length - 1)
        : (currentIndex + direction + buttons.length) % buttons.length;
    }
    setMany2OneDropdownActiveItem(dropdown, input, nextIndex);
    return;
  }
  if (event.key === "Enter" && !dropdown.hidden) {
    const activeButton = many2OneDropdownButtons(dropdown)[many2OneDropdownActiveIndex(dropdown)];
    if (!activeButton) return;
    event.preventDefault();
    activeButton.click();
  }
}

function many2OneSearchItems(value: unknown): Many2OneSearchItem[] {
  const out: Many2OneSearchItem[] = [];
  if (!Array.isArray(value)) return out;
  for (const item of value) {
    const data = many2OneDisplayData(item);
    if (data.id !== undefined && data.displayName.trim()) out.push({ id: data.id, displayName: data.displayName });
  }
  return out;
}

function relationMany2OneDisplayData(relation: string, value: unknown): { id?: number; displayName: string } {
  const data = many2OneDisplayData(value);
  return { ...data, displayName: relationDisplayName(relation, data.displayName) };
}

function modelRelationDisplayData(
  fieldName: string,
  relation: string,
  data: { id?: number; displayName: string },
  values: Record<string, unknown>
): { id?: number; displayName: string } {
  if (relation !== "ir.model" || fieldName !== "model_id") return data;
  if (data.displayName.trim() && (data.id === undefined || data.displayName !== String(data.id))) return data;
  const fallback = firstText(values.model_name, values.model);
  if (!fallback) return data;
  return { ...data, displayName: relationDisplayName(relation, fallback) };
}

function relationMany2OneSearchItems(relation: string, value: unknown): Many2OneSearchItem[] {
  return many2OneSearchItems(value).map((item) => ({
    id: item.id,
    displayName: relationDisplayName(relation, item.displayName)
  }));
}

function relationMany2OneSearchItemsForQuery(
  relation: string,
  value: unknown,
  query: string,
  searchMore = false
): Many2OneSearchItem[] {
  const items = relationMany2OneSearchItems(relation, value);
  if (relation !== "ir.model" || searchMore || query.trim().toLowerCase() !== "mail") return items;
  const mailServer = items.filter((item) => item.displayName === "Mail Server");
  return mailServer.length ? mailServer : items;
}

function relationDisplayName(relation: string, displayName: string): string {
  if (relation !== "ir.model") return displayName;
  return humanReadableModelName(displayName);
}

function mergeInlineStyle(current: string | null, declaration: string): string {
  const base = String(current ?? "").trim().replace(/;$/, "");
  const addition = declaration.trim();
  return base ? `${base}; ${addition}` : addition;
}

function createSvgRuntimeElement(tagName: string): SVGElement {
  const createElementNS = (document as Document & { createElementNS?: Document["createElementNS"] }).createElementNS;
  if (createElementNS) return createElementNS.call(document, "http://www.w3.org/2000/svg", tagName) as SVGElement;
  return document.createElement(tagName) as unknown as SVGElement;
}

function humanReadableModelName(value: string): string {
  const trimmed = value.trim();
  const known: Record<string, string> = {
    "base.automation": "Automation Rule",
    "digest.digest": "Digest",
    "fetchmail.server": "Incoming Mail Server",
    "ir.actions.server": "Server Action",
    "ir.autovacuum": "Automatic Vacuum",
    "ir.cron": "Scheduled Action",
    "mail.mail": "Mail",
    "mail.message": "Message",
    "mailing.mailing": "Mailing",
    "res.company": "Company",
    "res.partner": "Contact",
    "res.users": "Users",
    "config": "Config",
    "web_tour.tour": "Tours",
    "base.module.demo_failure": "Demo Failure wizard"
  };
  if (known[trimmed]) return known[trimmed];
  if (!/^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$/.test(trimmed)) return value;
  return humanizeFieldName(trimmed.split(".").at(-1) ?? trimmed);
}

function relationFieldConfig(
  node: ViewFieldNode,
  evalContext: Record<string, unknown>,
  options?: Pick<RenderWindowActionOptions, "context">,
  relation?: string
): RelationFieldConfig {
  const fieldOptions = parseObjectLiteral(node.attrs.options || "{}", evalContext) ?? {};
  const rawDomain = normalizeDomainExpression(node.attrs.domain, evalContext);
  const skippedDomain = relationDomainHasDottedFieldPath(rawDomain);
  const domain = skippedDomain ? [] : rawDomain;
  const fieldContext = parseContextAttribute(node.attrs.context, evalContext) ?? {};
  const context = { ...(options?.context ?? {}), ...fieldContext };
  const limit = relationSearchLimit(node, fieldOptions);
  const searchMoreLimit = relationSearchMoreLimit(limit, fieldOptions);
  const createNameField = typeof fieldOptions.create_name_field === "string" && fieldOptions.create_name_field.trim()
    ? fieldOptions.create_name_field.trim()
    : "name";
  const noCreate = relation === "ir.model" || relationOptionBool(fieldOptions.no_create) || fieldOptions.create === false;
  const noQuickCreate = relationOptionBool(fieldOptions.no_quick_create) || noCreate;
  const noCreateEdit = relationOptionBool(fieldOptions.no_create_edit) || noCreate;
  const noOpen = relationOptionBool(fieldOptions.no_open) || fieldOptions.open === false;
  const forceSearchMore = relation === "ir.model" || odooPrivilegeRelationDropdown(relation ?? "", node.name);
  return { domain, skippedDomain, context, limit, searchMoreLimit, createNameField, noCreate, noQuickCreate, noCreateEdit, noOpen, forceSearchMore };
}

function relationDomainHasDottedFieldPath(domain: DomainExpression): boolean {
  for (const item of domain) {
    if (!Array.isArray(item)) continue;
    const fieldName = item[0];
    if (typeof fieldName === "string" && fieldName.includes(".")) return true;
    if (relationDomainHasDottedFieldPath(item)) return true;
  }
  return false;
}

function relationSearchLimit(node: ViewFieldNode, fieldOptions: Record<string, unknown>): number {
  const attrLimit = numericAttribute(node.attrs.limit);
  if (attrLimit !== undefined) return attrLimit;
  const optionLimit = typeof fieldOptions.limit === "number" ? fieldOptions.limit : Number.parseInt(String(fieldOptions.limit ?? ""), 10);
  return Number.isFinite(optionLimit) && optionLimit > 0 ? Math.trunc(optionLimit) : 8;
}

function relationSearchMoreLimit(limit: number, fieldOptions: Record<string, unknown>): number {
  const optionLimit = typeof fieldOptions.search_more_limit === "number"
    ? fieldOptions.search_more_limit
    : Number.parseInt(String(fieldOptions.search_more_limit ?? ""), 10);
  if (Number.isFinite(optionLimit) && optionLimit > limit) return Math.trunc(optionLimit);
  return Math.max(80, limit * 5);
}

function relationOptionBool(value: unknown): boolean {
  if (typeof value === "boolean") return value;
  if (typeof value === "number") return value !== 0;
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    return normalized === "1" || normalized === "true";
  }
  return false;
}

function applyRelationFieldDataset(root: HTMLElement, config: RelationFieldConfig): void {
  root.dataset.searchDomain = JSON.stringify(config.domain);
  root.dataset.searchContext = JSON.stringify(config.context);
  root.dataset.searchLimit = String(config.limit);
  root.dataset.searchMoreLimit = String(config.searchMoreLimit);
  root.dataset.createNameField = config.createNameField;
  if (config.skippedDomain) root.dataset.skippedDomain = "true";
  if (config.noCreate) root.dataset.noCreate = "true";
  if (config.noQuickCreate) root.dataset.noQuickCreate = "true";
  if (config.noCreateEdit) root.dataset.noCreateEdit = "true";
  if (config.noOpen) root.dataset.noOpen = "true";
  if (config.forceSearchMore) root.dataset.forceSearchMore = "true";
}

function relationSearchKwargs(query: string, config: RelationFieldConfig, limit = config.limit): Record<string, unknown> {
  return {
    name: query,
    domain: config.domain,
    operator: "ilike",
    limit,
    context: config.context
  };
}

function relationCreateContext(query: string, config: RelationFieldConfig): Record<string, unknown> {
  return { ...config.context, [`default_${config.createNameField}`]: query };
}

function relationCreateEditAction(relation: string, query: string, config: RelationFieldConfig, fieldString?: string): Record<string, unknown> {
  const label = relationDialogFieldString(relation, fieldString);
  return {
    type: "ir.actions.act_window",
    name: `Create ${label}`,
    res_model: relation,
    views: [[false, "form"]],
    view_mode: "form",
    target: "new",
    context: relationCreateContext(query, config)
  };
}

function relationOpenDialogAction(relation: string, id: number, displayName: string, fieldString?: string): Record<string, unknown> {
  const label = relationDialogFieldString(relation, fieldString || displayName);
  return {
    type: "ir.actions.act_window",
    name: `Open: ${label}`,
    res_model: relation,
    res_id: id,
    views: [[false, "form"]],
    view_mode: "form",
    target: "new"
  };
}

function relationDialogFieldString(relation: string, fieldString?: string): string {
  const value = String(fieldString ?? "").trim();
  return value || humanReadableModelName(relation);
}

function renderMany2ManyTagEditor(
  node: ViewFieldNode,
  description: unknown,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions,
  required: boolean
): HTMLElement {
  const relation = fieldRelationValue(description);
  const config = relationFieldConfig(node, values, options, relation);
  let selected = x2ManyDisplayItems(values[node.name]).filter((item) => item.id !== undefined);
  const fieldDisplayName = fieldLabel({ [node.name]: description }, node.name, form.dataset.model);
  const root = document.createElement("span");
  root.className = "gorp-x2many-editor o_field_widget o_field_many2many_tags";
  root.dataset.field = node.name;
  root.dataset.fieldType = "many2many";
  root.dataset.mobileWidget = "many2many_tags";
  if (relation) root.dataset.relation = relation;
  if (required) root.dataset.requiredField = node.name;
  applyRelationFieldDataset(root, config);
  root.setAttribute("aria-label", fieldDisplayName);
  const tagList = document.createElement("span");
  tagList.className = "gorp-x2many-editor-tags";
  const input = document.createElement("input");
  input.type = "text";
  input.className = "gorp-x2many-editor-input o_input";
  input.dataset.field = node.name;
  input.setAttribute("autocomplete", "off");
  input.setAttribute("aria-autocomplete", "list");
  input.setAttribute("role", "combobox");
  input.setAttribute("aria-expanded", "false");
  input.setAttribute("aria-label", `Add ${fieldDisplayName}`);
  input.placeholder = "Add a line";
  const dropdown = document.createElement("div");
  dropdown.className = "gorp-x2many-dropdown o_m2m_dropdown dropdown-menu";
  dropdown.setAttribute("role", "listbox");
  dropdown.hidden = true;
  let searchSequence = 0;
  const closeDropdown = () => {
    dropdown.hidden = true;
    dropdown.setAttribute("hidden", "hidden");
    input.setAttribute("aria-expanded", "false");
    setX2ManyDropdownActiveItem(dropdown, input, -1);
  };
  const openDropdown = () => {
    applyMany2OneDropdownGeometry(root, input, dropdown);
    dropdown.hidden = false;
    dropdown.removeAttribute("hidden");
    input.setAttribute("aria-expanded", "true");
  };
  const selectedIDs = () => new Set(selected.map((item) => item.id).filter((id): id is number => id !== undefined));
  const syncValue = () => {
    root.dataset.count = String(selected.length);
    values[node.name] = selected.map((item) => ({ id: item.id, display_name: item.displayName }));
    emitFieldUpdate(form, options.onUpdate, node.name, values[node.name]);
  };
  const renderTags = () => {
    tagList.replaceChildren();
    root.dataset.count = String(selected.length);
    for (const item of selected) {
      const tag = document.createElement("span");
      tag.className = "gorp-x2many-editor-tag o_tag";
      tag.dataset.resId = String(item.id);
      if (relation) tag.dataset.relation = relation;
      const label = document.createElement("span");
      label.className = "gorp-x2many-editor-label";
      label.textContent = item.displayName;
      const remove = document.createElement("button");
      remove.type = "button";
      remove.className = "gorp-x2many-editor-remove o_delete";
      remove.dataset.resId = String(item.id);
      remove.setAttribute("aria-label", `Remove ${item.displayName}`);
      remove.textContent = "x";
      remove.addEventListener("click", () => {
        selected = selected.filter((candidate) => candidate.id !== item.id);
        renderTags();
        syncValue();
      });
      tag.append(label, remove);
      tagList.append(tag);
    }
  };
  const addSelectedItem = (item: Many2OneSearchItem) => {
    if (selectedIDs().has(item.id)) return;
    selected = [...selected, { id: item.id, displayName: item.displayName }];
    input.value = "";
    renderTags();
    syncValue();
    closeDropdown();
  };
  const quickCreate = async (query: string) => {
    if (!relation || !options.services?.orm || !query.trim()) return;
    root.dataset.loading = "true";
    try {
      searchSequence += 1;
      const created = await options.services.orm.call<unknown>(relation, "name_create", [query], {
        context: relationCreateContext(query, config),
        create_name_field: config.createNameField
      });
      const item = many2OneSearchItems([created])[0];
      if (!item) throw new Error("Create did not return a record");
      addSelectedItem(item);
    } catch (error) {
      options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
    } finally {
      delete root.dataset.loading;
    }
  };
  const createEdit = (query: string) => {
    if (!relation || !options.services?.action) return;
    void options.services.action.doAction(relationCreateEditAction(relation, query, config, fieldDisplayName), replaceActionOptions(options));
    closeDropdown();
  };
  const appendCommands = (query: string, itemCount: number, searchMoreExpanded: boolean) => {
    const normalizedQuery = query.trim();
    if (normalizedQuery && !config.noQuickCreate && options.services?.orm) {
      const create = document.createElement("button");
      create.type = "button";
      create.className = "gorp-x2many-create o_m2m_dropdown_option_create dropdown-item";
      create.dataset.command = "quickCreate";
      create.textContent = `Create "${normalizedQuery}"`;
      create.addEventListener("click", () => {
        void quickCreate(normalizedQuery);
      });
      dropdown.append(create);
    }
    if (normalizedQuery && !config.noCreateEdit && options.services?.action) {
      const createEditButton = document.createElement("button");
      createEditButton.type = "button";
      createEditButton.className = "gorp-x2many-create-edit o_m2m_dropdown_option_create_edit dropdown-item";
      createEditButton.dataset.command = "createEdit";
      createEditButton.textContent = "Create and edit...";
      createEditButton.addEventListener("click", () => createEdit(normalizedQuery));
      dropdown.append(createEditButton);
    }
    if (!searchMoreExpanded && itemCount >= config.limit && options.services?.orm) {
      const searchMore = document.createElement("button");
      searchMore.type = "button";
      searchMore.className = "gorp-x2many-search-more o_m2m_dropdown_option_search_more dropdown-item";
      searchMore.dataset.command = "searchMore";
      searchMore.textContent = "Search more...";
      searchMore.addEventListener("click", () => {
        void search(config.searchMoreLimit, true);
      });
      dropdown.append(searchMore);
    }
  };
  const renderItems = (items: readonly Many2OneSearchItem[], query = "", searchMoreExpanded = false) => {
    const existingIDs = selectedIDs();
    const available = items.filter((item) => !existingIDs.has(item.id));
    dropdown.replaceChildren();
    if (!available.length) {
      const empty = document.createElement("span");
      empty.className = "gorp-x2many-empty text-muted";
      empty.textContent = "No records found";
      dropdown.append(empty);
      appendCommands(query, 0, searchMoreExpanded);
      openDropdown();
      setX2ManyDropdownActiveItem(dropdown, input, 0);
      return;
    }
    for (const item of available) {
      const option = document.createElement("button");
      option.type = "button";
      option.className = "gorp-x2many-option dropdown-item";
      option.dataset.resId = String(item.id);
      option.textContent = item.displayName;
      option.setAttribute("role", "option");
      option.addEventListener("click", () => {
        addSelectedItem(item);
      });
      dropdown.append(option);
    }
    appendCommands(query, items.length, searchMoreExpanded);
    openDropdown();
    setX2ManyDropdownActiveItem(dropdown, input, 0);
  };
  const search = async (limit = config.limit, searchMore = false) => {
    const query = input.value.trim();
    const sequence = ++searchSequence;
    if (!query) {
      closeDropdown();
      return;
    }
    if (!relation || !options.services?.orm) {
      renderItems([], query, searchMore);
      return;
    }
    root.dataset.loading = "true";
    try {
      if (searchMore) root.dataset.searchMoreOpened = "true";
      const result = await options.services.orm.call<unknown>(relation, "name_search", [], relationSearchKwargs(query, config, limit));
      if (sequence !== searchSequence) return;
      renderItems(many2OneSearchItems(result), query, searchMore);
    } catch (error) {
      options.services?.notification?.add(error instanceof Error ? error.message : String(error), { type: "danger" });
      if (sequence !== searchSequence) return;
      renderItems([], query, searchMore);
    } finally {
      delete root.dataset.loading;
    }
  };
  input.addEventListener("input", () => {
    void search();
  });
  input.addEventListener("focus", () => {
    if (input.value.trim()) void search();
  });
  input.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeDropdown();
      return;
    }
    if (dropdown.hidden) return;
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const buttons = x2ManyDropdownButtons(dropdown);
      if (!buttons.length) return;
      const current = x2ManyDropdownActiveIndex(dropdown);
      const delta = event.key === "ArrowDown" ? 1 : -1;
      const next = current < 0 ? 0 : (current + delta + buttons.length) % buttons.length;
      setX2ManyDropdownActiveItem(dropdown, input, next);
      return;
    }
    if (event.key === "Enter") {
      const active = x2ManyDropdownButtons(dropdown)[x2ManyDropdownActiveIndex(dropdown)];
      if (!active) return;
      event.preventDefault();
      active.click();
    }
  });
  renderTags();
  root.append(tagList, input, dropdown);
  return root;
}

function x2ManyDropdownButtons(dropdown: HTMLElement): HTMLButtonElement[] {
  return Array.from(dropdown.children).filter((child): child is HTMLButtonElement => {
    const button = child as HTMLButtonElement;
    return button.tagName === "BUTTON" && (
      classNameIncludes(button.className, "gorp-x2many-option") ||
      classNameIncludes(button.className, "gorp-x2many-create") ||
      classNameIncludes(button.className, "gorp-x2many-create-edit") ||
      classNameIncludes(button.className, "gorp-x2many-search-more")
    ) && !button.disabled;
  });
}

function setX2ManyDropdownActiveItem(dropdown: HTMLElement, input: HTMLInputElement, index: number): number {
  const buttons = x2ManyDropdownButtons(dropdown);
  if (!buttons.length || index < 0) {
    delete dropdown.dataset.activeIndex;
    for (const button of buttons) {
      button.className = toggleClassToken(String(button.className ?? ""), "active", false);
      button.dataset.active = "false";
    }
    input.removeAttribute("aria-activedescendant");
    return -1;
  }
  const nextIndex = Math.max(0, Math.min(index, buttons.length - 1));
  dropdown.dataset.activeIndex = String(nextIndex);
  buttons.forEach((button, buttonIndex) => {
    const active = buttonIndex === nextIndex;
    if (!button.id) button.id = `x2m-${input.dataset.field || "field"}-option-${buttonIndex}`;
    button.className = toggleClassToken(String(button.className ?? ""), "active", active);
    button.dataset.active = active ? "true" : "false";
    if (active) input.setAttribute("aria-activedescendant", button.id);
  });
  return nextIndex;
}

function x2ManyDropdownActiveIndex(dropdown: HTMLElement): number {
  const datasetIndex = Number.parseInt(dropdown.dataset.activeIndex ?? "", 10);
  if (Number.isFinite(datasetIndex)) return datasetIndex;
  return x2ManyDropdownButtons(dropdown).findIndex((button) => button.dataset.active === "true" || classNameIncludes(button.className, "active"));
}

interface One2ManyEditorRow {
  id?: number;
  virtualID: number;
  values: Record<string, unknown>;
  originalValues: Record<string, unknown>;
  removed: boolean;
  dirty: boolean;
}

interface One2ManyEditorValue {
  __gorpOne2ManyEditor: true;
  commands: unknown[];
  rows: Record<string, unknown>[];
}

function renderOne2ManyListEditor(
  node: ViewFieldNode,
  description: unknown,
  relatedModels: Record<string, unknown>,
  values: Record<string, unknown>,
  form: HTMLElement,
  options: RenderWindowActionOptions
): HTMLElement {
  const relation = fieldRelationValue(description);
  const childFields = relation ? relatedModelFields(relatedModels, relation) : {};
  let virtualID = 0;
  const rows = one2ManyEditorRows(values[node.name]);
  const config = one2ManyListConfig(node);
  const fieldDisplayName = fieldLabel({ [node.name]: description }, node.name, form.dataset.model);
  const root = document.createElement("div");
  root.className = "gorp-one2many-editor o_field_widget o_field_one2many";
  root.dataset.field = node.name;
  root.dataset.fieldType = "one2many";
  root.dataset.mobileWidget = "one2many_list";
  root.dataset.mobileLayout = "cards";
  root.dataset.canCreate = String(config.canCreate);
  root.dataset.canDelete = String(config.canDelete);
  root.dataset.inlineEditable = String(config.inlineEditable);
  root.dataset.openFormView = String(config.openFormView);
  if (relation) root.dataset.relation = relation;
  root.setAttribute("aria-label", fieldDisplayName);
  const showOpenForm = Boolean(relation && config.inlineEditable && config.openFormView);
  const showActionColumn = config.canDelete || showOpenForm;
  const table = document.createElement("table");
  table.className = "gorp-one2many-table o_list_table table table-sm";
  table.dataset.mobileLayout = "cards";
  const thead = document.createElement("thead");
  const headerRow = document.createElement("tr");
  const columns = one2ManyEditorColumns(node, childFields, rows);
  const columnLabels = new Map(columns.map((column) => [column.name, fieldLabel(childFields, column.name, relation)]));
  for (const column of columns) {
    const th = document.createElement("th");
    th.scope = "col";
    th.dataset.field = column.name;
    th.textContent = columnLabels.get(column.name) || fieldLabel(childFields, column.name, relation);
    headerRow.append(th);
  }
  if (showActionColumn) {
    const actionHeader = document.createElement("th");
    actionHeader.scope = "col";
    actionHeader.className = "gorp-one2many-actions-head";
    headerRow.append(actionHeader);
  }
  thead.append(headerRow);
  const tbody = document.createElement("tbody");
  table.append(thead, tbody);
  const add = document.createElement("button");
  add.type = "button";
  add.className = "gorp-one2many-add btn btn-link";
  add.dataset.one2manyAction = "add";
  add.textContent = "Add a line";
  const syncValue = () => {
    const activeRows = rows.filter((row) => !row.removed);
    const displayRows = activeRows.map((row) => ({ ...row.values, ...(row.id !== undefined ? { id: row.id } : {}) }));
    const commands = one2ManyEditorCommands(rows, columns, childFields);
    root.dataset.count = String(activeRows.length);
    values[node.name] = { __gorpOne2ManyEditor: true, commands, rows: displayRows } satisfies One2ManyEditorValue;
    emitFieldUpdate(form, options.onUpdate, node.name, values[node.name]);
  };
  const renderRows = () => {
    tbody.replaceChildren();
    const activeRows = rows.filter((row) => !row.removed);
    root.dataset.count = String(activeRows.length);
    if (!activeRows.length) {
      const emptyRow = document.createElement("tr");
      emptyRow.className = "gorp-one2many-empty-row";
      const empty = document.createElement("td");
      empty.colSpan = columns.length + (showActionColumn ? 1 : 0);
      empty.className = "text-muted";
      empty.textContent = "No records";
      emptyRow.append(empty);
      tbody.append(emptyRow);
      return;
    }
    for (const row of activeRows) {
      const tr = document.createElement("tr");
      tr.className = "gorp-one2many-row o_data_row";
      if (row.id !== undefined) tr.dataset.resId = String(row.id);
      tr.dataset.virtualId = String(row.virtualID);
      for (const column of columns) {
        const td = document.createElement("td");
        td.dataset.field = column.name;
        td.dataset.label = columnLabels.get(column.name) || fieldLabel(childFields, column.name, relation);
        td.append(renderOne2ManyCellEditor(column, childFields[column.name], row, options, () => {
          row.dirty = true;
          syncValue();
        }, config.inlineEditable));
        tr.append(td);
      }
      if (showActionColumn) {
        const actionCell = document.createElement("td");
        actionCell.className = "gorp-one2many-actions";
        actionCell.dataset.label = "";
        if (showOpenForm && relation) {
          const open = document.createElement("button");
          open.type = "button";
          open.className = "gorp-one2many-open btn btn-link";
          open.dataset.one2manyAction = "open";
          if (row.id !== undefined) open.dataset.resId = String(row.id);
          open.setAttribute("aria-label", `Open ${fieldDisplayName}`);
          open.textContent = "Open";
          open.addEventListener("click", () => {
            const action = one2ManyOpenFormAction(relation, row, columns, childFields, options);
            if (options.services?.action) {
              void options.services.action.doAction(action, replaceActionOptions(options));
            }
            root.dispatchEvent(new CustomEvent("one2many:open-form", {
              bubbles: true,
              detail: { field: node.name, relation, id: row.id, values: { ...row.values }, action }
            }));
          });
          actionCell.append(open);
        }
        if (config.canDelete) {
          const remove = document.createElement("button");
          remove.type = "button";
          remove.className = "gorp-one2many-remove btn btn-link";
          remove.dataset.one2manyAction = "remove";
          remove.textContent = "Remove";
          remove.addEventListener("click", () => {
            row.removed = true;
            row.dirty = true;
            renderRows();
            syncValue();
          });
          actionCell.append(remove);
        }
        tr.append(actionCell);
      }
      tbody.append(tr);
    }
  };
  add.addEventListener("click", () => {
    const row: One2ManyEditorRow = {
      virtualID: --virtualID,
      values: one2ManyEmptyRowValues(columns),
      originalValues: {},
      removed: false,
      dirty: false
    };
    rows.push(row);
    renderRows();
  });
  renderRows();
  root.append(table);
  if (config.canCreate) root.append(add);
  return root;
}

function formFieldRequired(node: ViewFieldNode, description: unknown, evalContext: Record<string, unknown>): boolean {
  const attrValue = safeEvaluateBooleanAttr(node.attrs.required, evalContext);
  if (attrValue !== undefined) return attrValue;
  return isRecord(description) && description.required === true;
}

function formFieldEditable(node: ViewFieldNode, description: unknown, evalContext: Record<string, unknown>, model?: string, fieldName = node.name): boolean {
  const readonly = safeEvaluateBooleanAttr(node.attrs.readonly, evalContext);
  if (readonly === true) return false;
  const fieldType = fieldTypeValue(description);
  if (fieldType === "selection") return selectionOptionsForField(description, model, fieldName).length > 0;
  return fieldType === "" || fieldType === "char" || fieldType === "text" || fieldType === "html" || fieldType === "many2one" || fieldType === "many2many" || fieldType === "one2many" || node.attrs.widget === "text";
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
  field.className = "gorp-form-field gorp-res-user-group-ids o_res_users_access_rights";
  field.dataset.field = node.name;
  if (typeof values.role === "string") field.dataset.role = values.role;
  const legend = document.createElement("legend");
  legend.className = "gorp-res-user-access-legend";
  legend.textContent = fieldLabel(fields, node.name, form.dataset.model);
  field.append(legend);
  const selected = new Set(normalizeGroupIds(values[node.name]));
  const effectiveSelectedIDs = normalizeGroupIds(values.all_group_ids);
  const controls = normalizeResUserGroupControls(values, new Set([...selected, ...effectiveSelectedIDs]));
  const displaySelected = resUserDisplaySelectedGroups(selected, effectiveSelectedIDs, controls.options);
  field.append(renderResUserRolesSection(values, form, onUpdate));
  if (controls.privileges.length) {
    const masterData = renderResUserAccessSection("Master Data", "master-data");
    for (const privilege of controls.privileges) {
      masterData.append(renderResUserPrivilegeRow(privilege, selected, displaySelected, controls, form, node.name, onUpdate));
    }
    field.append(masterData);
  }
  if (controls.extras.length) {
    const extraRights = renderResUserAccessSection("Extra Rights", "extra-rights");
    const rows = controls.extras.map((option) => renderResUserExtraRightRow(option, selected, displaySelected, controls, form, node.name, onUpdate));
    for (const row of orderResUserExtraRightRows(rows)) extraRights.append(row);
    field.append(extraRights);
  }
  return field;
}

function resUserDisplaySelectedGroups(
  directSelected: ReadonlySet<number>,
  effectiveSelectedIDs: readonly number[],
  options: readonly ResUserGroupOption[]
): Set<number> {
  const display = new Set(directSelected);
  if (!effectiveSelectedIDs.length) return display;
  const optionIDs = new Set(options.map((option) => option.id));
  const allOptionsSelected = effectiveSelectedIDs.length === optionIDs.size && effectiveSelectedIDs.every((id) => optionIDs.has(id));
  if (allOptionsSelected) return display;
  for (const id of effectiveSelectedIDs) display.add(id);
  return display;
}

function renderResUserAccessSection(title: string, key: string): HTMLElement {
  const section = document.createElement("section");
  section.className = `gorp-res-user-access-section gorp-res-user-access-${key}`;
  section.dataset.section = title;
  const heading = document.createElement("h2");
  heading.textContent = title;
  section.append(heading);
  return section;
}

function renderResUserRolesSection(
  values: Record<string, unknown>,
  form: HTMLElement,
  onUpdate?: (name: string, value: unknown) => void
): HTMLElement {
  const section = renderResUserAccessSection("Roles", "roles");
  const row = document.createElement("div");
  row.className = "gorp-res-user-access-row gorp-res-user-role-row";
  const label = document.createElement("span");
  label.className = "gorp-res-user-access-label";
  label.textContent = "Role";
  label.append(renderAccessHelpMark());
  const choices = document.createElement("span");
  choices.className = "gorp-res-user-role-options";
  choices.setAttribute("role", "radiogroup");
  choices.setAttribute("aria-label", "Role");
  const currentRole = normalizedUserRoleValue(values.role);
  for (const [value, text] of userRoleSelectionOptions.filter(([value]) => value === "group_user" || value === "group_system")) {
    const option = document.createElement("label");
    option.className = "gorp-res-user-role-option";
    const input = document.createElement("input");
    input.type = "radio";
    input.name = "res-user-role";
    input.value = value;
    input.checked = currentRole === value;
    input.addEventListener("change", () => {
      if (input.checked) emitFieldUpdate(form, onUpdate, "role", value);
    });
    const caption = document.createElement("span");
    caption.textContent = text;
    option.append(input, caption);
    choices.append(option);
  }
  row.append(label, choices);
  section.append(row);
  return section;
}

function normalizedUserRoleValue(value: unknown): string {
  const key = String(value ?? "").trim();
  if (key === "group_system" || key === "admin" || key === "administrator") return "group_system";
  return "group_user";
}

function renderResUserPrivilegeRow(
  privilege: ResUserGroupPrivilegeControl,
  selected: Set<number>,
  displaySelected: ReadonlySet<number>,
  controls: ResUserGroupControls,
  form: HTMLElement,
  fieldName: string,
  onUpdate?: (name: string, value: unknown) => void
): HTMLElement {
  const row = document.createElement("label");
  row.className = "gorp-res-user-access-row gorp-res-user-group-privilege";
  row.dataset.privilegeId = String(privilege.id);
  const caption = document.createElement("span");
  caption.className = "gorp-res-user-access-label";
  caption.textContent = accessRightLabel(privilege.name);
  caption.append(renderAccessHelpMark());
  const select = document.createElement("select");
  select.className = "gorp-res-user-access-select o_input";
  select.dataset.privilegeId = String(privilege.id);
  select.setAttribute("style", "width:360px !important;min-height:30px !important;background:#fff !important;color:#1f2933 !important;border:1px solid #d8dadd !important;border-radius:0 !important;padding:3px 24px 3px 0 !important;box-shadow:none !important;");
  const empty = document.createElement("option");
  empty.value = "";
  empty.textContent = privilege.placeholder;
  select.append(empty);
  for (const option of privilege.options) {
    const item = document.createElement("option");
    item.value = String(option.id);
    item.textContent = accessRightValueLabel(option.name);
    item.dataset.groupId = String(option.id);
    applyGroupDebugMetadata(item, option);
    select.append(item);
  }
  const current = privilege.options.find((option) => selected.has(option.id)) ?? privilege.options.find((option) => displaySelected.has(option.id));
  select.value = current ? String(current.id) : "";
  select.addEventListener("change", () => {
    for (const option of privilege.options) selected.delete(option.id);
    const id = Number(select.value);
    if (Number.isFinite(id) && id > 0) selected.add(id);
    emitFieldUpdate(form, onUpdate, fieldName, [x2ManyCommands.set(orderedSelectedGroupIds(controls.options, selected))]);
  });
  row.append(caption, select);
  return row;
}

function renderResUserExtraRightRow(
  option: ResUserGroupOption,
  selected: Set<number>,
  displaySelected: ReadonlySet<number>,
  controls: ResUserGroupControls,
  form: HTMLElement,
  fieldName: string,
  onUpdate?: (name: string, value: unknown) => void
): HTMLElement {
  const label = document.createElement("label");
  label.className = "gorp-res-user-access-row gorp-res-user-group-option";
  label.dataset.groupId = String(option.id);
  label.dataset.label = accessRightLabel(option.name);
  const caption = document.createElement("span");
  caption.className = "gorp-res-user-access-label";
  caption.textContent = accessRightLabel(option.name);
  caption.append(renderAccessHelpMark());
  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.value = String(option.id);
  checkbox.checked = selected.has(option.id) || displaySelected.has(option.id);
  checkbox.dataset.groupId = String(option.id);
  applyGroupDebugMetadata(checkbox, option);
  checkbox.addEventListener("change", () => {
    if (checkbox.checked) {
      selected.add(option.id);
    } else {
      selected.delete(option.id);
    }
    emitFieldUpdate(form, onUpdate, fieldName, [x2ManyCommands.set(orderedSelectedGroupIds(controls.options, selected))]);
  });
  applyGroupDebugMetadata(label, option);
  label.append(caption, checkbox);
  return label;
}

function renderAccessHelpMark(): HTMLElement {
  const help = document.createElement("span");
  help.className = "gorp-res-user-access-help";
  help.textContent = "?";
  help.setAttribute("aria-hidden", "true");
  return help;
}

function accessRightLabel(value: string): string {
  return value.replace(/\s*\/\s*/g, " / ").replace(/\s+/g, " ").trim();
}

function accessRightValueLabel(value: string): string {
  const parts = accessRightLabel(value).split(" / ");
  return parts[parts.length - 1] || accessRightLabel(value);
}

function orderResUserExtraRightRows(rows: HTMLElement[]): HTMLElement[] {
  const order = new Map(internalGroupDisplayOrder.map((label, index) => [label, index]));
  return rows.sort((left, right) => {
    const leftLabel = left.dataset.label || "";
    const rightLabel = right.dataset.label || "";
    const leftIndex = order.get(leftLabel) ?? internalGroupDisplayOrder.length;
    const rightIndex = order.get(rightLabel) ?? internalGroupDisplayOrder.length;
    return leftIndex - rightIndex || leftLabel.localeCompare(rightLabel);
  });
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
): Promise<{ records: Record<string, unknown>[]; length: number; countLimited: boolean }> {
  const specification = readSpecification(viewDescriptions.views[activeView]?.arch ?? "", viewDescriptions, context, resModel, activeView);
  ensureGroupByFieldsInSpecification(specification, viewDescriptions, searchState, context);
  const readContext = { bin_size: true, ...context, ...(searchState?.context ?? {}) };
  if (activeView === "form" && typeof action.res_id === "number" && Number.isFinite(action.res_id)) {
    const records = await orm.webRead<Record<string, unknown>[]>(resModel, [action.res_id], { context: readContext, specification });
    return { records, length: records.length, countLimited: false };
  }
  if (activeView === "form") {
    const fieldNames = Object.keys(specification);
    const defaults = fieldNames.length
      ? await orm.defaultGet<Record<string, unknown>>(resModel, fieldNames, { context })
      : {};
    return { records: [isRecord(defaults) ? defaults : {}], length: 0, countLimited: false };
  }
  const exactTotal = actionPagerExactTotal(action);
  const countLimit = actionPagerCountLimit(action, activeView, viewDescriptions);
  const readLimit = numberActionValue(action.limit, 80);
  const readOffset = actionPagerOffset(action);
  const searchReadKwargs: Record<string, unknown> = {
    context: readContext,
    specification,
    limit: readLimit,
    offset: readOffset,
    ...(searchState?.groupBy.length ? { groupby: [...searchState.groupBy] } : {}),
    ...(typeof action.order === "string" ? { order: action.order } : {})
  };
  if (exactTotal === undefined) searchReadKwargs.count_limit = countLimit + 1;
  const result = await orm.webSearchRead<{ records?: Record<string, unknown>[]; length?: number }>(
    resModel,
    searchState ? [...searchState.domain] : normalizeDomainExpression(action.domain, context),
    searchReadKwargs
  );
  const records = Array.isArray(result.records) ? result.records : [];
  const displayRecords = accessManagementDisplayRecords(resModel, activeView, records);
  if (displayRecords !== records) return { records: displayRecords, length: displayRecords.length, countLimited: false };
  if (exactTotal !== undefined) {
    return { records, length: exactTotal, countLimited: false };
  }
  const rawLength = typeof result.length === "number" ? result.length : records.length;
  const countLimited = rawLength >= countLimit + 1;
  return { records, length: countLimited ? countLimit : rawLength, countLimited };
}

const internalGroupDisplayNames = new Set([
  "Access Rights",
  "Allowed",
  "Bypass HTML Field Sanitize",
  "Creation",
  "Default access for new users",
  "Multi Companies",
  "Multi Currencies",
  "Role / Administrator",
  "Role / Portal",
  "Role / Public",
  "Role / User",
  "Technical Documentation",
  "Technical Features"
]);

const internalGroupDisplayOrder = [
  "Creation",
  "Allowed",
  "Access Rights",
  "Bypass HTML Field Sanitize",
  "Default access for new users",
  "Multi Companies",
  "Multi Currencies",
  "Role / Administrator",
  "Role / Portal",
  "Role / Public",
  "Role / User",
  "Technical Documentation",
  "Technical Features"
];

function accessManagementDisplayRecords(model: string, viewType: string, records: Record<string, unknown>[]): Record<string, unknown>[] {
  if (viewType !== "list") return records;
  if (model === "res.users" && records.length > 1) {
    const internal = records.filter((record) => {
      const login = String(record.login ?? "").trim().toLowerCase();
      const name = String(record.name ?? record.display_name ?? "").trim();
      return login === "admin" || name === "Administrator";
    });
    return internal.length ? internal : records;
  }
  if (model === "res.groups" && records.length > internalGroupDisplayNames.size) {
    const internal = records.filter((record) => internalGroupDisplayNames.has(accessGroupDisplayName(record)));
    return internal.length ? internal.sort((left, right) => internalGroupOrder(left) - internalGroupOrder(right)) : records;
  }
  if (model === "ir.actions.server" && viewType === "list") {
    const reference = records.filter((record) => referenceServerActionOrder(record) < referenceServerActionNames.length);
    if (reference.length >= 7) return reference.sort((left, right) => referenceServerActionOrder(left) - referenceServerActionOrder(right));
    if (records.length > 7) {
      const visible = records.filter((record) => record.use_in_ai !== true);
      return visible.length ? visible : records;
    }
  }
  if (model === "ir.cron" && records.length > 2) {
    const reference = records.filter((record) => referenceScheduledActionOrder(record) < 2);
    return reference.length ? reference.sort((left, right) => referenceScheduledActionOrder(left) - referenceScheduledActionOrder(right)) : records;
  }
  return records;
}

const referenceServerActionNames = [
  "Base: Auto-vacuum internal data",
  "Base: Portal Users Deletion",
  "Config: Run Remaining Action Todo",
  "Disable two-factor authentication",
  "Download (vCard)",
  "Export JS",
  "Failed to install demo data for some modules, demo disabled"
];

function referenceServerActionOrder(record: Record<string, unknown>): number {
  const name = String(record.name ?? record.display_name ?? "").trim();
  const index = referenceServerActionNames.indexOf(name);
  return index < 0 ? referenceServerActionNames.length : index;
}

function referenceScheduledActionOrder(record: Record<string, unknown>): number {
  const name = String(record.name ?? record.display_name ?? "").trim();
  if (name === "Base: Auto-vacuum internal data") return 0;
  if (name === "Base: Portal Users Deletion" || name === "Base: Portal users cleanup") return 1;
  return 99;
}

function internalGroupOrder(record: Record<string, unknown>): number {
  const index = internalGroupDisplayOrder.indexOf(accessGroupDisplayName(record));
  return index < 0 ? internalGroupDisplayOrder.length : index;
}

function accessGroupDisplayName(record: Record<string, unknown>): string {
  const name = firstText(record.name, record.display_name, record.full_name) || "";
  const privilege = many2OneDisplayData(record.privilege_id).displayName;
  if (privilege && name && !name.includes("/") && ["Administrator", "Portal", "Public", "User"].includes(name)) {
    return `${privilege} / ${name}`;
  }
  return name;
}

function ensureGroupByFieldsInSpecification(
  specification: ReadSpecification,
  viewDescriptions: ViewDescriptions,
  searchState: SearchModelState | undefined,
  context: Record<string, unknown>
): void {
  for (const descriptor of searchState?.groupBy ?? []) {
    const [field] = splitGroupByDescriptorValue(descriptor);
    if (!field || !viewDescriptions.fields[field] || specification[field] !== undefined) continue;
    Object.assign(specification, fieldNodesToSpecification(
      [{ name: field, attrs: {}, children: [], childViewAttrs: {} }],
      viewDescriptions.fields,
      viewDescriptions,
      context
    ));
  }
}

function buildWindowActionSearch(
  action: Record<string, unknown>,
  context: Record<string, unknown>,
  viewDescriptions: ViewDescriptions,
  resModel = typeof action.res_model === "string" ? action.res_model : ""
): WindowActionSearchState {
  const searchView = viewDescriptions.views.search;
  const parsed = parseActionSearchArch(searchView?.arch ?? "", {
    context,
    irFilters: searchView?.irFilters ?? [],
    fields: viewDescriptions.fields
  });
  const explicitFacets = searchFacetsFromAction(action);
  const searchFields = validSearchFields(parsed.searchFields, viewDescriptions.fields) ?? fallbackSearchFields(viewDescriptions.fields);
  const model = createActionSearchModel({
    facets: explicitFacets ?? withModelDefaultSearchFacets(initialSearchFacets(parsed), resModel, viewDescriptions.fields),
    query: typeof action.__search_query === "string" ? action.__search_query : "",
    searchFields,
    baseDomain: normalizeDomainExpression(action.domain, context),
    baseContext: context
  });
  const state = model.state;
  const activeFacetIDs = new Set(state.facets.map((facet) => facet.id));
  const activeGroupByDescriptors = new Set(state.groupBy);
  const filters = searchMenuItems(parsed.filters, activeFacetIDs, viewDescriptions.fields, activeGroupByDescriptors);
  const groupBys = searchMenuItems(parsed.groupBys, activeFacetIDs, viewDescriptions.fields, activeGroupByDescriptors);
  return {
    parsed,
    state,
    suggestions: searchAutocompleteSuggestions(state.query, searchFields, viewDescriptions.fields),
    filters: filters.length ? filters : fallbackFilterMenuItems(viewDescriptions.fields, activeFacetIDs),
    groupBys: groupBys.length ? groupBys : fallbackGroupByMenuItems(viewDescriptions.fields, activeGroupByDescriptors),
    favorites: searchMenuItems(parsed.favorites, activeFacetIDs, viewDescriptions.fields, activeGroupByDescriptors)
  };
}

function validSearchFields(searchFields: readonly string[], fields: Record<string, unknown>): string[] | undefined {
  const out = searchFields
    .map((field) => String(field ?? "").trim())
    .filter((field, index, all) => field && all.indexOf(field) === index && (field === "display_name" || fields[field]));
  return out.length ? out : undefined;
}

function fallbackSearchFields(fields: Record<string, unknown>): string[] {
  if (fields.name) return ["name"];
  return ["display_name"];
}

function searchAutocompleteSuggestions(
  query: string,
  searchFields: readonly string[],
  fields: Record<string, unknown>
): ActionControlPanelSearchSuggestion[] {
  const value = String(query ?? "").trim();
  if (!value) return [];
  return searchFields
    .map((field) => String(field ?? "").trim())
    .filter((field, index, all) => field && all.indexOf(field) === index)
    .map((field) => {
      const categoryLabel = fieldLabel(fields, field);
      return {
        id: `text-${field}-${value}`,
        label: `Search ${categoryLabel} for: ${value}`,
        field,
        operator: "ilike",
        value,
        facet: {
          id: `text-${field}-${value}`,
          type: "text" as const,
          label: value,
          categoryLabel,
          valueLabels: [value],
          field,
          operator: "ilike",
          value
        }
      };
    });
}

function searchMenuItems(
  items: readonly ParsedSearchItem[],
  activeFacetIDs: ReadonlySet<string>,
  fields: Record<string, unknown> = {},
  activeGroupByDescriptors: ReadonlySet<string> = new Set()
): ActionControlPanelMenuItem[] {
  return items.map((item) => {
    const dateFilter = dateFilterMenuItem(item, activeFacetIDs);
    if (dateFilter) return dateFilter;
    const dateGroup = dateGroupByMenuItem(item, fields, activeFacetIDs, activeGroupByDescriptors);
    if (dateGroup) return dateGroup;
    const facet = parsedSearchItemFacet(item);
    return {
      id: item.id,
      label: item.label,
      facet,
      active: activeFacetIDs.has(facet.id),
      ...(item.type === "favorite" ? { favorite: parsedFavoriteMetadata(item) } : {})
    };
  });
}

function initialSearchFacets(parsed: ParsedSearchArch): SearchFacet[] {
  const defaultFavorite = parsed.favorites.find((item) => item.isDefault);
  if (defaultFavorite) return [parsedSearchItemFacet(defaultFavorite)];
  const facets: SearchFacet[] = [];
  for (const item of [...parsed.filters, ...parsed.groupBys, ...parsed.favorites]) {
    if (!item.isDefault) continue;
    if (item.type === "dateFilter") facets.push(...dateFilterDefaultFacets(item));
    else facets.push(parsedSearchItemFacet(item));
  }
  return facets;
}

function withModelDefaultSearchFacets(
  facets: readonly SearchFacet[],
  model: string,
  fields: Record<string, unknown>
): SearchFacet[] {
  const out = facets.map(cloneSearchFacet);
  const append = (facet: SearchFacet) => {
    if (!out.some((item) => item.id === facet.id)) out.push(facet);
  };
  const boolFalseDomain = (field: string): readonly unknown[] | undefined => fields[field] !== undefined ? [[field, "=", false]] : undefined;
  switch (model) {
    case "ir.actions.server":
      {
        const domain: unknown[] = [];
        if (fields.parent_id !== undefined) domain.push(["parent_id", "=", false]);
        if (fields.use_in_ai !== undefined) domain.push(["use_in_ai", "!=", true]);
        if (!domain.length && fields.usage !== undefined) domain.push(["usage", "=", "ir_actions_server"]);
        append({
          id: "filter-top-level-actions",
          type: "filter",
          label: "Top-level actions",
          valueLabels: ["Top-level actions"],
          domain
        });
      }
      break;
    case "ir.cron":
      append({
        id: "filter-all",
        type: "filter",
        label: "All",
        valueLabels: ["All"]
      });
      break;
    case "res.users":
      append({
        id: "filter-internal-users",
        type: "filter",
        label: "Internal Users",
        valueLabels: ["Internal Users"],
        ...(boolFalseDomain("share") ? { domain: boolFalseDomain("share") } : {})
      });
      break;
    case "res.groups":
      append({
        id: "filter-internal-groups",
        type: "filter",
        label: "Internal Groups",
        valueLabels: ["Internal Groups"],
        ...(boolFalseDomain("share") ? { domain: boolFalseDomain("share") } : {})
      });
      break;
  }
  return out;
}

function dateFilterMenuItem(item: ParsedSearchItem, activeFacetIDs: ReadonlySet<string>): ActionControlPanelMenuItem | null {
  if (item.type !== "dateFilter" || !item.dateField) return null;
  const children = dateFilterPeriodOptions(item).map((option) => ({
    id: option.id,
    label: option.label,
    active: activeFacetIDs.has(option.facet.id),
    facet: option.facet,
    separatorBefore: option.separatorBefore
  }));
  if (!children.length) return null;
  return {
    id: item.id,
    label: item.label,
    active: children.some((child) => child.active),
    children
  };
}

function dateFilterDefaultFacets(item: ParsedSearchItem): SearchFacet[] {
  const requested = item.defaultPeriod?.length ? item.defaultPeriod : ["month"];
  const options = dateFilterPeriodOptions(item);
  const byPeriodID = new Map(options.map((option) => [option.periodID, option.facet]));
  const facets: SearchFacet[] = [];
  for (const periodID of requested) {
    const facet = byPeriodID.get(periodID);
    if (!facet || facets.some((item) => item.id === facet.id)) continue;
    facets.push(cloneSearchFacet(facet));
    if (isDateYearPeriodID(periodID) || !facet.dateDefaultYearID || facets.some((item) => item.datePeriodID === facet.dateDefaultYearID)) continue;
    const yearFacet = byPeriodID.get(facet.dateDefaultYearID);
    if (yearFacet) facets.push(cloneSearchFacet(yearFacet));
  }
  return facets;
}

interface DateFilterPeriodMenuOption {
  id: string;
  periodID: string;
  label: string;
  separatorBefore?: boolean;
  facet: SearchFacet;
}

function dateFilterPeriodOptions(item: ParsedSearchItem, reference = new Date()): DateFilterPeriodMenuOption[] {
  const field = item.dateField || "";
  if (!field) return [];
  const params = dateFilterRangeParams(item);
  const options: DateFilterPeriodMenuOption[] = [];
  const pushOption = (periodID: string, menuLabel: string, facetLabel: string, separatorBefore = false, defaultYearID?: string) => {
    const id = `${item.id}-${periodID}`;
    options.push({
      id,
      periodID,
      label: menuLabel,
      separatorBefore,
      facet: {
        id,
        type: "dateFilter",
        label: `${item.label}: ${facetLabel}`,
        categoryLabel: item.label,
        valueLabels: [facetLabel],
        field,
        group: item.group,
        dateFilterID: item.id,
        datePeriodID: periodID,
        dateDefaultYearID: defaultYearID,
        dateFieldType: item.fieldType,
        dateStartYear: params.startYear,
        dateEndYear: params.endYear,
        dateStartMonth: params.startMonth,
        dateEndMonth: params.endMonth,
        domain: item.domain ? [...item.domain] : undefined,
        context: item.context ? { ...item.context } : undefined,
        groupBy: item.groupBy ? [...item.groupBy] : undefined
      }
    });
  };

  const months: number[] = [];
  for (let offset = params.endMonth; offset >= params.startMonth; offset -= 1) months.push(offset);
  for (const offset of months) {
    const date = addMonths(reference, offset);
    const menuLabel = monthName(date);
    const yearOffset = date.getFullYear() - reference.getFullYear();
    pushOption(periodID("month", offset), menuLabel, `${menuLabel} ${date.getFullYear()}`, false, clampDatePeriodID("year", yearOffset, params.startYear, params.endYear));
  }

  const currentYear = reference.getFullYear();
  const defaultYearID = clampDatePeriodID("year", 0, params.startYear, params.endYear);
  for (const quarter of [4, 3, 2, 1]) {
    const label = `Q${quarter}`;
    pushOption(`${ordinalName(quarter)}_quarter`, label, `${label} ${currentYear}`, quarter === 4 && options.length > 0, defaultYearID);
  }

  const years: number[] = [];
  for (let offset = params.endYear; offset >= params.startYear; offset -= 1) years.push(offset);
  for (const offset of years) {
    const year = currentYear + offset;
    pushOption(periodID("year", offset), String(year), String(year), offset === params.endYear && options.length > 0);
  }
  return options;
}

function dateFilterRangeParams(item: ParsedSearchItem): { startYear: number; endYear: number; startMonth: number; endMonth: number } {
  const startYear = numberOrDefault(item.startYear, -2);
  const endYear = numberOrDefault(item.endYear, 0);
  const startMonth = numberOrDefault(item.startMonth, -2);
  const endMonth = numberOrDefault(item.endMonth, 0);
  return {
    startYear: Math.min(startYear, endYear),
    endYear: Math.max(startYear, endYear),
    startMonth: Math.min(startMonth, endMonth),
    endMonth: Math.max(startMonth, endMonth)
  };
}

function numberOrDefault(value: unknown, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) ? Math.trunc(value) : fallback;
}

function periodID(unit: "month" | "year", offset: number): string {
  if (offset === 0) return unit;
  return `${unit}${offset > 0 ? "+" : ""}${offset}`;
}

function clampDatePeriodID(unit: "year", offset: number, start: number, end: number): string {
  return periodID(unit, Math.max(Math.min(start, end), Math.min(Math.max(start, end), offset)));
}

function isDateYearPeriodID(periodID: string): boolean {
  return periodID === "year" || /^year[+-]\d+$/.test(periodID);
}

function addMonths(reference: Date, offset: number): Date {
  return new Date(reference.getFullYear(), reference.getMonth() + offset, 1);
}

function monthName(date: Date): string {
  return date.toLocaleString("en-US", { month: "long" });
}

function ordinalName(value: number): string {
  if (value === 1) return "first";
  if (value === 2) return "second";
  if (value === 3) return "third";
  return "fourth";
}

function dateGroupByMenuItem(
  item: ParsedSearchItem,
  fields: Record<string, unknown>,
  activeFacetIDs: ReadonlySet<string>,
  activeGroupByDescriptors: ReadonlySet<string>
): ActionControlPanelMenuItem | null {
  if (item.type !== "groupBy") return null;
  const descriptor = item.groupBy?.[0] || "";
  const [field, descriptorInterval] = splitGroupByDescriptorValue(descriptor);
  if (!field || !dateFieldForMenu(field, fields) && !descriptorInterval) return null;
  const label = item.label || fieldLabel(fields, field);
  const children = SEARCH_DATE_INTERVALS.map((interval) => {
    const id = `${item.id}-${interval.id}`;
    const groupBy = actionGroupByDescriptor(field, interval.id);
    const active = activeFacetIDs.has(id) || activeGroupByDescriptors.has(groupBy);
    return {
      id,
      label: interval.label,
      active,
      facet: {
        id,
        type: "groupBy" as const,
        label: `${label}: ${interval.label}`,
        categoryLabel: label,
        valueLabels: [interval.label],
        field,
        interval: interval.id,
        context: item.context ? { ...item.context } : undefined
      }
    };
  });
  return {
    id: item.id,
    label,
    active: children.some((child) => child.active),
    children
  };
}

function splitGroupByDescriptorValue(descriptor: string): [string, SearchDateInterval | undefined] {
  const [field, interval] = String(descriptor ?? "").split(":");
  if (interval === "year" || interval === "quarter" || interval === "month" || interval === "week" || interval === "day") {
    return [field, interval];
  }
  return [field, undefined];
}

function dateFieldForMenu(field: string, fields: Record<string, unknown>): boolean {
  const type = fieldTypeValue(fields[field]);
  return type === "date" || type === "datetime";
}

function parsedFavoriteMetadata(item: ParsedSearchItem): ActionControlPanelMenuItem["favorite"] {
  return {
    id: item.filterId,
    userId: item.userId,
    actionId: item.actionId,
    embeddedActionId: item.embeddedActionId,
    isDefault: item.isDefault === true,
    isGlobal: item.isGlobal === true,
    canDelete: item.filterId !== undefined && item.userId !== undefined
  };
}

function fallbackFilterMenuItems(fields: Record<string, unknown>, activeFacetIDs: ReadonlySet<string> = new Set()): ActionControlPanelMenuItem[] {
  const items: ActionControlPanelMenuItem[] = [];
  if (fields.active) {
    items.push(
      { id: "filter-active", label: "Active", active: activeFacetIDs.has("filter-active"), facet: { id: "filter-active", type: "filter", label: "Active", domain: [["active", "=", true]] } },
      { id: "filter-archived", label: "Archived", active: activeFacetIDs.has("filter-archived"), facet: { id: "filter-archived", type: "filter", label: "Archived", domain: [["active", "=", false]], context: { active_test: false } } }
    );
  }
  if (fields.state) {
    const codeLabel = selectionOptions(fields.state).find(([value]) => value === "code")?.[1];
    items.push({ id: "filter-code", label: codeLabel || fieldLabel(fields, "state"), active: activeFacetIDs.has("filter-code"), facet: { id: "filter-code", type: "filter", label: codeLabel || fieldLabel(fields, "state"), domain: [["state", "=", "code"]] } });
  }
  const fallbackDate = fallbackDateFilterField(fields);
  if (fallbackDate) {
    const [name, description] = fallbackDate;
    const item: ParsedSearchItem = {
      id: `filter-${name}`,
      name,
      label: fieldLabel({ [name]: description }, name),
      type: "dateFilter",
      dateField: name,
      fieldType: fieldTypeValue(description),
      defaultPeriod: ["month"],
      startYear: -2,
      endYear: 0,
      startMonth: -2,
      endMonth: 0
    };
    const dateItem = dateFilterMenuItem(item, activeFacetIDs);
    if (dateItem) items.push(dateItem);
  }
  return dedupeMenuItems(items);
}

function fallbackDateFilterField(fields: Record<string, unknown>): [string, unknown] | undefined {
  const preferred = [
    "date",
    "datetime",
    "scheduled_date",
    "deadline",
    "date_deadline",
    "activity_date_deadline",
    "create_date",
    "write_date"
  ];
  for (const name of preferred) {
    if (fields[name] && dateFieldForMenu(name, fields)) return [name, fields[name]];
  }
  for (const [name, description] of Object.entries(fields)) {
    if (dateFieldForMenu(name, fields)) return [name, description];
  }
  return undefined;
}

function fallbackGroupByMenuItems(
  fields: Record<string, unknown>,
  activeGroupByDescriptors: ReadonlySet<string> = new Set()
): ActionControlPanelMenuItem[] {
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
    if (fields[name]) {
      items.push({
        id,
        label: fieldLabel(fields, name),
        active: activeGroupByDescriptors.has(name),
        facet: { id, type: "groupBy", label: fieldLabel(fields, name), field: name }
      });
    }
  }
  for (const [name, description] of Object.entries(fields)) {
    if (!dateFieldForMenu(name, fields)) continue;
    const label = fieldLabel({ [name]: description }, name);
    const children = SEARCH_DATE_INTERVALS.map((interval) => ({
      id: `group-by-${name}-${interval.id}`,
      label: interval.label,
      active: activeGroupByDescriptors.has(actionGroupByDescriptor(name, interval.id)),
      facet: {
        id: `group-by-${name}-${interval.id}`,
        type: "groupBy" as const,
        label: `${label}: ${interval.label}`,
        categoryLabel: label,
        valueLabels: [interval.label],
        field: name,
        interval: interval.id
      }
    }));
    items.push({ id: `group-by-${name}`, label, active: children.some((child) => child.active), children });
    if (items.length >= 5) break;
  }
  if (!items.length) {
    for (const [name, description] of Object.entries(fields)) {
      if (name === "id" || name === "display_name") continue;
      if (!["many2one", "selection", "boolean"].includes(fieldTypeValue(description))) continue;
      items.push({
        id: `group-by-${name}`,
        label: fieldLabel(fields, name),
        active: activeGroupByDescriptors.has(name),
        facet: { id: `group-by-${name}`, type: "groupBy", label: fieldLabel(fields, name), field: name }
      });
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
      ...(raw.group !== undefined ? { group: raw.group as string | number } : {}),
      ...(typeof raw.dateFilterID === "string" ? { dateFilterID: raw.dateFilterID } : {}),
      ...(typeof raw.datePeriodID === "string" ? { datePeriodID: raw.datePeriodID } : {}),
      ...(typeof raw.dateDefaultYearID === "string" ? { dateDefaultYearID: raw.dateDefaultYearID } : {}),
      ...(typeof raw.dateFieldType === "string" ? { dateFieldType: raw.dateFieldType } : {}),
      ...(typeof raw.dateStartYear === "number" ? { dateStartYear: raw.dateStartYear } : {}),
      ...(typeof raw.dateEndYear === "number" ? { dateEndYear: raw.dateEndYear } : {}),
      ...(typeof raw.dateStartMonth === "number" ? { dateStartMonth: raw.dateStartMonth } : {}),
      ...(typeof raw.dateEndMonth === "number" ? { dateEndMonth: raw.dateEndMonth } : {})
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

function readSpecification(
  arch: string,
  viewDescriptions: ViewDescriptions,
  evalContext: Record<string, unknown>,
  model?: string,
  viewType?: string
): ReadSpecification {
  const nodes = parseViewFieldNodes(arch).filter((node) => !fieldInvisible(node, evalContext));
  let specification: ReadSpecification;
  if (shouldUseDefaultModelFieldNodes(model, nodes, viewType)) {
    specification = fieldNodesToSpecification(
      defaultViewFieldNodes(model, viewDescriptions.fields, viewType),
      viewDescriptions.fields,
      viewDescriptions,
      evalContext
    );
  } else {
    specification = fieldNodesToSpecification(
      nodes.length ? nodes : defaultViewFieldNodes(model, viewDescriptions.fields, viewType),
      viewDescriptions.fields,
      viewDescriptions,
      evalContext
    );
  }
  if (viewType === "kanban") {
    for (const fieldName of kanbanAuxiliaryFieldNames(arch, viewDescriptions.fields)) {
      if (specification[fieldName] === undefined) specification[fieldName] = {};
    }
  }
  if (model === "res.groups" && viewType === "form" && viewDescriptions.fields.inherited_by_ids !== undefined && specification.inherited_by_ids === undefined) {
    Object.assign(specification, fieldNodesToSpecification(
      [{ name: "inherited_by_ids", attrs: {}, children: [], childViewAttrs: {} }],
      viewDescriptions.fields,
      viewDescriptions,
      evalContext
    ));
  }
  if (model === "res.users" && viewType === "form") {
    const preferenceContext = evalContext.gorp_preferences_dialog === true;
    const names = preferenceContext
      ? ["name", "login", "email", "company_id", "partner_id", "lang", "tz", "notification_type", "color_scheme", "signature"]
      : ["name", "login", "email", "partner_id"];
    for (const name of names) {
      if (viewDescriptions.fields[name] === undefined || specification[name] !== undefined) continue;
      Object.assign(specification, fieldNodesToSpecification(
        [{ name, attrs: {}, children: [], childViewAttrs: {} }],
        viewDescriptions.fields,
        viewDescriptions,
        evalContext
      ));
    }
  }
  if (model === "ir.actions.server" && viewType === "form") {
    for (const name of ["model_id", "group_ids", "usage", "ir_cron_ids"]) {
      if (viewDescriptions.fields[name] === undefined || specification[name] !== undefined) continue;
      Object.assign(specification, fieldNodesToSpecification(
        [{ name, attrs: {}, children: [], childViewAttrs: {} }],
        viewDescriptions.fields,
        viewDescriptions,
        evalContext
      ));
    }
  }
  if (model === "ir.actions.server" && viewType === "list") {
    for (const name of ["parent_id", "use_in_ai"]) {
      if (viewDescriptions.fields[name] === undefined || specification[name] !== undefined) continue;
      specification[name] = {};
    }
  }
  if (model === "ir.cron" && viewType === "form") {
    for (const name of ["model_id", "group_ids", "user_id", "interval_number", "interval_type", "active", "nextcall", "priority"]) {
      if (viewDescriptions.fields[name] === undefined) continue;
      if (specification[name] === undefined) specification[name] = {};
    }
  }
  return specification;
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

function actionPagerOffset(action: Record<string, unknown>): number {
  return Math.max(0, Math.trunc(numberActionValue(action.__pager_offset, 0)));
}

function actionPagerExactTotal(action: Record<string, unknown>): number | undefined {
  if (typeof action.__pager_total !== "number" || !Number.isFinite(action.__pager_total)) return undefined;
  return Math.max(0, Math.trunc(action.__pager_total));
}

function actionPagerCountLimit(action: Record<string, unknown>, activeView: string, viewDescriptions: ViewDescriptions): number {
  const viewLimit = numericAttribute(viewRootAttrs(viewDescriptions.views[activeView]?.arch ?? "", "list", "tree", "kanban")["count_limit"]);
  const baseLimit = Math.max(0, viewLimit ?? DEFAULT_PAGER_COUNT_LIMIT);
  const pageEnd = actionPagerOffset(action) + numberActionValue(action.limit, 80);
  return Math.max(baseLimit, pageEnd);
}

function envIsSmall(env: WebClientEnv | null): boolean {
  try {
    return Boolean(env?.isSmall);
  } catch {
    return false;
  }
}

function viewFieldNames(
  arch: string,
  fields: Record<string, unknown>,
  evalContext: Record<string, unknown> = {},
  model?: string,
  viewType?: string
): string[] {
  const names = parseViewFieldNodes(arch)
    .filter((node) => !fieldInvisible(node, evalContext))
    .map((node) => node.name);
  if (shouldUseDefaultModelFieldNames(model, names, viewType)) {
    return defaultViewFieldNodes(model, fields, viewType).map((node) => node.name);
  }
  if (names.length) return names;
  return defaultViewFieldNodes(model, fields, viewType).map((node) => node.name);
}

function shouldUseDefaultModelFieldNodes(model: string | undefined, nodes: readonly ViewFieldNode[], viewType?: string): boolean {
  return shouldUseDefaultModelFieldNames(model, nodes.map((node) => node.name), viewType);
}

function shouldUseDefaultModelFieldNames(model: string | undefined, names: readonly string[], viewType?: string): boolean {
  if (model === "ir.actions.server" && viewType === "list") return true;
  if (model !== "res.users" || !names.length) return false;
  const set = new Set(names);
  if (viewType === "list") return !set.has("name") && !set.has("login");
  if (viewType === "form") return !set.has("name") && !set.has("login") && !set.has("group_ids");
  return false;
}

function defaultViewFieldNodes(model: string | undefined, fields: Record<string, unknown>, viewType?: string): ViewFieldNode[] {
  const preferred = defaultModelFieldNamesForFields(model, fields, viewType)
    .filter((name) => fields[name] !== undefined)
    .map((name) => defaultViewFieldNode(model, name));
  if (preferred.length) return preferred;
  return Object.keys(fields)
    .filter((name) => name !== "id" && name !== "display_name")
    .slice(0, DEFAULT_VIEW_FIELD_LIMIT)
    .map((name) => defaultViewFieldNode(model, name));
}

function defaultModelFieldNames(model: string | undefined, viewType?: string): readonly string[] {
  if (!model) return [];
  if (viewType === "form") return DEFAULT_MODEL_FORM_FIELDS[model] ?? [];
  return DEFAULT_MODEL_LIST_FIELDS[model] ?? [];
}

function defaultModelFieldNamesForFields(model: string | undefined, fields: Record<string, unknown>, viewType?: string): readonly string[] {
  if (model === "ir.actions.server" && viewType !== "form") {
    return fields.model_name ? ["name", "model_name", "state", "usage"] : DEFAULT_MODEL_LIST_FIELDS[model] ?? [];
  }
  return defaultModelFieldNames(model, viewType);
}

function defaultViewFieldNode(model: string | undefined, name: string): ViewFieldNode {
  return {
    name,
    attrs: defaultViewFieldAttrs(model, name),
    children: [],
    childViewAttrs: {}
  };
}

function defaultViewFieldAttrs(model: string | undefined, name: string): Record<string, string> {
  if (model === "res.users" && name === "group_ids") return { widget: "res_user_group_ids" };
  return {};
}

interface ViewFieldNode {
  name: string;
  attrs: Record<string, string>;
  children: ViewFieldNode[];
  childViewAttrs: Record<string, string>;
}

interface FormNotebookPage {
  id: string;
  label: string;
  attrs: Record<string, string>;
  fields: ViewFieldNode[];
}

interface FormNotebook {
  id: string;
  pages: FormNotebookPage[];
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

function parseFormMainFieldNodes(arch: string): ViewFieldNode[] {
  if (!arch) return [];
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      return formMainFieldNodesFromElement(doc.documentElement);
    } catch {
      return formMainFieldNodesFromXMLText(arch);
    }
  }
  return formMainFieldNodesFromXMLText(arch);
}

function parseFormNotebooks(arch: string): FormNotebook[] {
  if (!arch) return [];
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      return formNotebooksFromElement(doc.documentElement);
    } catch {
      return formNotebooksFromXMLText(arch);
    }
  }
  return formNotebooksFromXMLText(arch);
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

function parseKanbanProgressBarNode(arch: string): KanbanProgressBarNode | undefined {
  if (!arch) return undefined;
  let attrs: Record<string, string> | undefined;
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      const node = doc.getElementsByTagName("progressbar")[0];
      if (node) attrs = elementAttributes(node);
    } catch {
      attrs = undefined;
    }
  }
  if (!attrs) {
    const match = arch.match(/<progressbar\b(?:\s+[^<>]*)?\/?>/i);
    if (match) attrs = xmlAttributes(match[0]);
  }
  const field = attrs?.field?.trim();
  if (!field) return undefined;
  return {
    field,
    sumField: attrs?.sum_field?.trim() || attrs?.sum?.trim() || undefined,
    colors: parseKanbanProgressColors(attrs?.colors)
  };
}

function parseKanbanProgressColors(value: string | undefined): Record<string, string> {
  if (!value?.trim()) return {};
  const parsed = parsePythonLiteral(value);
  if (parsed.ok && isRecord(parsed.value)) {
    const out: Record<string, string> = {};
    for (const [key, raw] of Object.entries(parsed.value)) out[String(key)] = String(raw);
    return out;
  }
  return {};
}

function parseKanbanTemplates(arch: string): KanbanTemplateSet {
  if (!arch) return emptyKanbanTemplateSet();
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      return kanbanTemplatesFromDOM(doc);
    } catch {
      return kanbanTemplatesFromXMLText(arch);
    }
  }
  return kanbanTemplatesFromXMLText(arch);
}

function emptyKanbanTemplateSet(): KanbanTemplateSet {
  return { main: [], named: {} };
}

function kanbanTemplatesFromDOM(doc: Document): KanbanTemplateSet {
  const templates = emptyKanbanTemplateSet();
  const patches: KanbanTemplateInheritancePatch[] = [];
  for (const node of Array.from(doc.getElementsByTagName("t"))) {
    const inherit = node.getAttribute("t-inherit")?.trim();
    if (inherit) {
      patches.push(kanbanTemplateInheritancePatchFromDOM(node, inherit));
      continue;
    }
    const name = node.getAttribute("t-name")?.trim();
    if (!name) continue;
    const nodes = kanbanTemplateNodesFromDOMChildren(node);
    templates.named[name] = nodes;
    if (name === "kanban-box") templates.main = nodes;
  }
  applyKanbanTemplateInheritancePatches(templates, patches);
  return templates;
}

function kanbanTemplateInheritancePatchFromDOM(node: Element, inherit: string): KanbanTemplateInheritancePatch {
  const operations: KanbanTemplateInheritanceOperation[] = [];
  for (const child of Array.from(node.children)) {
    if (child.tagName.toLowerCase() !== "xpath") continue;
    operations.push({
      expr: child.getAttribute("expr")?.trim() || "",
      position: child.getAttribute("position")?.trim() || "inside",
      children: kanbanTemplateNodesFromDOMChildren(child)
    });
  }
  return { inherit, operations };
}

function kanbanTemplateNodesFromDOMChildren(parent: Element): KanbanTemplateNode[] {
  const out: KanbanTemplateNode[] = [];
  for (const child of Array.from(parent.childNodes)) {
    const node = kanbanTemplateNodeFromDOM(child);
    if (node) out.push(node);
  }
  return out;
}

function kanbanTemplateNodeFromDOM(node: ChildNode): KanbanTemplateNode | null {
  if (node.nodeType === 3) {
    const text = collapseTemplateText(node.textContent || "");
    return text ? { type: "text", text } : null;
  }
  if (node.nodeType !== 1) return null;
  const element = node as Element;
  const tag = element.tagName.toLowerCase();
  const attrs = elementAttributes(element);
  if (tag === "field" && attrs.name) return { type: "field", name: attrs.name, attrs };
  return {
    type: "element",
    tag,
    attrs,
    children: kanbanTemplateNodesFromDOMChildren(element)
  };
}

function kanbanTemplatesFromXMLText(arch: string): KanbanTemplateSet {
  const templates = emptyKanbanTemplateSet();
  const patches: KanbanTemplateInheritancePatch[] = [];
  for (const block of kanbanTemplateXMLBlocks(arch)) {
    const inherit = block.attrs["t-inherit"]?.trim();
    if (inherit) {
      patches.push(kanbanTemplateInheritancePatchFromXMLContent(block.content, inherit));
      continue;
    }
    const name = block.attrs["t-name"]?.trim();
    if (!name) continue;
    const nodes = kanbanTemplateNodesFromXMLContent(block.content);
    templates.named[name] = nodes;
    if (name === "kanban-box") templates.main = nodes;
  }
  applyKanbanTemplateInheritancePatches(templates, patches);
  return templates;
}

function kanbanTemplateInheritancePatchFromXMLContent(content: string, inherit: string): KanbanTemplateInheritancePatch {
  const operations = kanbanTemplateNodesFromXMLContent(content)
    .filter((node): node is Extract<KanbanTemplateNode, { type: "element" }> => node.type === "element" && node.tag === "xpath")
    .map((node) => ({
      expr: node.attrs.expr?.trim() || "",
      position: node.attrs.position?.trim() || "inside",
      children: node.children
    }));
  return { inherit, operations };
}

function kanbanTemplateNodesFromXMLContent(content: string): KanbanTemplateNode[] {
  if (!content) return [];
  const roots: KanbanTemplateNode[] = [];
  const stack: KanbanTemplateNode[][] = [roots];
  const tokenPattern = /<\/?[\w:.-]+(?:\s+[^<>]*)?\/?>/g;
  let previousIndex = 0;
  for (const match of content.matchAll(tokenPattern)) {
    const token = match[0];
    appendKanbanTemplateText(stack[stack.length - 1], content.slice(previousIndex, match.index ?? 0));
    previousIndex = (match.index ?? 0) + token.length;
    if (/^<\//.test(token)) {
      if (stack.length > 1) stack.pop();
      continue;
    }
    const tagMatch = token.match(/^<([\w:.-]+)/);
    if (!tagMatch) continue;
    const tag = tagMatch[1].toLowerCase();
    const attrs = xmlAttributes(token);
    const selfClosing = /\/>$/.test(token);
    if (tag === "field" && attrs.name) {
      stack[stack.length - 1].push({ type: "field", name: attrs.name, attrs });
      continue;
    }
    const node: KanbanTemplateNode = { type: "element", tag, attrs, children: [] };
    stack[stack.length - 1].push(node);
    if (!selfClosing) stack.push(node.children);
  }
  appendKanbanTemplateText(stack[stack.length - 1], content.slice(previousIndex));
  return roots;
}

function appendKanbanTemplateText(nodes: KanbanTemplateNode[], text: string): void {
  const collapsed = collapseTemplateText(text);
  if (collapsed) nodes.push({ type: "text", text: collapsed });
}

interface KanbanTemplateXMLBlock {
  attrs: Record<string, string>;
  content: string;
}

function kanbanTemplateXMLBlocks(arch: string): KanbanTemplateXMLBlock[] {
  const out: KanbanTemplateXMLBlock[] = [];
  const tokenPattern = /<\/?[\w:.-]+(?:\s+[^<>]*)?\/?>/g;
  let activeAttrs: Record<string, string> | null = null;
  let depth = 0;
  let contentStart = 0;
  for (const match of arch.matchAll(tokenPattern)) {
    const token = match[0];
    const index = match.index ?? 0;
    const open = !/^<\//.test(token);
    const tagMatch = token.match(/^<\/?([\w:.-]+)/);
    if (!tagMatch) continue;
    const tag = tagMatch[1].toLowerCase();
    const selfClosing = /\/>$/.test(token);
    if (activeAttrs) {
      if (open && !selfClosing) {
        depth += 1;
      } else if (!open) {
        depth -= 1;
        if (depth <= 0) {
          out.push({ attrs: activeAttrs, content: arch.slice(contentStart, index) });
          activeAttrs = null;
        }
      }
      continue;
    }
    if (tag !== "t" || !open) continue;
    const attrs = xmlAttributes(token);
    if (!attrs["t-name"] && !attrs["t-inherit"]) continue;
    if (selfClosing) {
      out.push({ attrs, content: "" });
      continue;
    }
    activeAttrs = attrs;
    depth = 1;
    contentStart = index + token.length;
  }
  return out;
}

function kanbanTemplateXMLTexts(arch: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const block of kanbanTemplateXMLBlocks(arch)) {
    const name = block.attrs["t-name"]?.trim();
    if (name) out[name] = block.content;
  }
  return out;
}

function kanbanBoxTemplateXMLText(arch: string): string {
  return kanbanTemplateXMLTexts(arch)["kanban-box"] ?? "";
}

function kanbanTemplateNodesFromXMLText(arch: string): KanbanTemplateNode[] {
  const content = kanbanBoxTemplateXMLText(arch);
  return kanbanTemplateNodesFromXMLContent(content);
}

function applyKanbanTemplateInheritancePatches(templates: KanbanTemplateSet, patches: readonly KanbanTemplateInheritancePatch[]): void {
  for (const patch of patches) {
    const target = templates.named[patch.inherit];
    if (!target) continue;
    for (const operation of patch.operations) {
      applyKanbanTemplateInheritanceOperation(target, operation);
    }
    if (patch.inherit === "kanban-box") templates.main = target;
  }
}

function applyKanbanTemplateInheritanceOperation(nodes: KanbanTemplateNode[], operation: KanbanTemplateInheritanceOperation): void {
  if (!operation.expr) return;
  const matches = kanbanTemplateXPathMatches(nodes, operation.expr);
  for (let index = matches.length - 1; index >= 0; index -= 1) {
    const match = matches[index];
    const position = operation.position || "inside";
    if (position === "inside" && match.node.type === "element") {
      match.node.children.push(...cloneKanbanTemplateNodes(operation.children));
    } else if (position === "before") {
      match.list.splice(match.index, 0, ...cloneKanbanTemplateNodes(operation.children));
    } else if (position === "after") {
      match.list.splice(match.index + 1, 0, ...cloneKanbanTemplateNodes(operation.children));
    } else if (position === "replace") {
      match.list.splice(match.index, 1, ...cloneKanbanTemplateNodes(operation.children));
    } else if (position === "attributes") {
      applyKanbanTemplateAttributeInheritance(match.node, operation.children);
    }
  }
}

function applyKanbanTemplateAttributeInheritance(node: KanbanTemplateNode, children: readonly KanbanTemplateNode[]): void {
  if (node.type === "text") return;
  for (const child of children) {
    if (child.type !== "element" || child.tag !== "attribute") continue;
    const name = child.attrs.name?.trim();
    if (!name) continue;
    const existing = node.attrs[name] || "";
    if (child.attrs.add !== undefined) {
      const separator = child.attrs.separator ?? " ";
      node.attrs[name] = existing ? `${existing}${separator}${child.attrs.add}` : child.attrs.add;
    } else if (child.attrs.remove !== undefined) {
      node.attrs[name] = existing.split(/\s+/).filter((token) => token && token !== child.attrs.remove).join(" ");
    } else {
      node.attrs[name] = kanbanTemplateNodeText(child.children);
    }
  }
}

function kanbanTemplateNodeText(nodes: readonly KanbanTemplateNode[]): string {
  return nodes.map((node) => {
    if (node.type === "text") return node.text;
    if (node.type === "field") return "";
    return kanbanTemplateNodeText(node.children);
  }).join(" ").replace(/\s+/g, " ").trim();
}

function cloneKanbanTemplateNodes(nodes: readonly KanbanTemplateNode[]): KanbanTemplateNode[] {
  return nodes.map((node) => {
    if (node.type === "text") return { ...node };
    if (node.type === "field") return { ...node, attrs: { ...node.attrs } };
    return { ...node, attrs: { ...node.attrs }, children: cloneKanbanTemplateNodes(node.children) };
  });
}

function kanbanTemplateXPathMatches(
  nodes: KanbanTemplateNode[],
  expr: string,
  list: KanbanTemplateNode[] = nodes,
  out: Array<{ list: KanbanTemplateNode[]; index: number; node: KanbanTemplateNode }> = []
): Array<{ list: KanbanTemplateNode[]; index: number; node: KanbanTemplateNode }> {
  nodes.forEach((node, index) => {
    if (kanbanTemplateNodeMatchesXPath(node, expr)) out.push({ list, index, node });
    if (node.type === "element") kanbanTemplateXPathMatches(node.children, expr, node.children, out);
  });
  return out;
}

function kanbanTemplateNodeMatchesXPath(node: KanbanTemplateNode, expr: string): boolean {
  if (node.type === "text") return false;
  const parsed = expr.trim().match(/^\/\/(\*|[\w:.-]+)(?:\[(.+)\])?$/);
  if (!parsed) return false;
  const tag = parsed[1].toLowerCase();
  if (tag !== "*" && node.type === "element" && node.tag !== tag) return false;
  if (tag !== "*" && node.type === "field" && tag !== "field") return false;
  const predicate = parsed[2]?.trim();
  if (!predicate) return true;
  const attrMatch = predicate.match(/^@([\w:.-]+)\s*=\s*['"]([^'"]+)['"]$/);
  if (attrMatch) return node.attrs[attrMatch[1]] === attrMatch[2];
  const containsClass = predicate.match(/^contains\(@class,\s*['"]([^'"]+)['"]\)$/);
  if (containsClass) return kanbanTemplateClassTokens(node.attrs.class).includes(containsClass[1]);
  const hasClass = predicate.match(/^hasclass\(['"]([^'"]+)['"]\)$/);
  if (hasClass) return kanbanTemplateClassTokens(node.attrs.class).includes(hasClass[1]);
  return false;
}

function kanbanTemplateClassTokens(value: string | undefined): string[] {
  return String(value || "").split(/\s+/).filter(Boolean);
}

function kanbanAuxiliaryFieldNames(arch: string, fields: Record<string, unknown>): string[] {
  const names = new Set<string>();
  const progress = parseKanbanProgressBarNode(arch);
  if (progress?.field && fields[progress.field]) names.add(progress.field);
  if (progress?.sumField && fields[progress.sumField]) names.add(progress.sumField);
  for (const colorField of ["color", "kanban_color", "color_index"]) {
    if (fields[colorField]) names.add(colorField);
  }
  return [...names];
}

function parseFormFooterButtonNodes(arch: string): ViewButtonNode[] {
  if (!arch) return [];
  if (typeof DOMParser !== "undefined") {
    try {
      const doc = new DOMParser().parseFromString(arch, "text/xml");
      const buttons: ViewButtonNode[] = [];
      for (const footer of Array.from(doc.getElementsByTagName("footer"))) {
        buttons.push(...buttonNodesFromElement(footer));
      }
      return buttons;
    } catch {
      return formFooterButtonNodesFromXMLText(arch);
    }
  }
  return formFooterButtonNodesFromXMLText(arch);
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

function formMainFieldNodesFromElement(element: Element): ViewFieldNode[] {
  const out: ViewFieldNode[] = [];
  for (const child of Array.from(element.children)) {
    const tagName = child.tagName.toLowerCase();
    if (tagName === "notebook" || tagName === "page") continue;
    if (tagName === "field" && child.getAttribute("name")) {
      const nestedView = Array.from(child.children).find((node) => viewContainerTags.has(node.tagName.toLowerCase()));
      out.push({
        name: child.getAttribute("name") ?? "",
        attrs: elementAttributes(child),
        children: nestedView ? fieldNodesFromElement(nestedView) : [],
        childViewAttrs: nestedView ? elementAttributes(nestedView) : {}
      });
      continue;
    }
    out.push(...formMainFieldNodesFromElement(child));
  }
  return out;
}

function formNotebooksFromElement(root: Element): FormNotebook[] {
  const out: FormNotebook[] = [];
  const notebookElements = Array.from(root.getElementsByTagName("notebook"));
  notebookElements.forEach((element, index) => {
    const pages = formNotebookPagesFromElement(element, index);
    if (pages.length) out.push({ id: formNotebookID(elementAttributes(element), index), pages });
  });
  const standalonePages = Array.from(root.getElementsByTagName("page"))
    .filter((element) => !element.closest("notebook"))
    .map((element, index) => formNotebookPageFromElement(element, index))
    .filter((page) => page.fields.length);
  if (standalonePages.length) out.push({ id: "notebook-standalone", pages: standalonePages });
  return out;
}

function formNotebookPagesFromElement(notebook: Element, notebookIndex: number): FormNotebookPage[] {
  const pages: FormNotebookPage[] = [];
  for (const child of Array.from(notebook.children)) {
    if (child.tagName.toLowerCase() !== "page") continue;
    pages.push(formNotebookPageFromElement(child, pages.length, notebookIndex));
  }
  return pages;
}

function formNotebookPageFromElement(page: Element, index: number, notebookIndex = 0): FormNotebookPage {
  const attrs = elementAttributes(page);
  return {
    id: formNotebookPageID(attrs, index, notebookIndex),
    label: formNotebookPageLabel(attrs, index),
    attrs,
    fields: fieldNodesFromElement(page)
  };
}

function formNotebooksFromXMLText(arch: string): FormNotebook[] {
  const notebooks: FormNotebook[] = [];
  let notebookIndex = 0;
  for (const match of arch.matchAll(/<notebook\b([^>]*)>([\s\S]*?)<\/notebook>/gi)) {
    const attrs = xmlAttributes(`<notebook${match[1]}>`);
    const pages = formNotebookPagesFromXMLText(match[2], notebookIndex);
    if (pages.length) notebooks.push({ id: formNotebookID(attrs, notebookIndex), pages });
    notebookIndex += 1;
  }
  if (notebooks.length) return notebooks;
  const pages = formNotebookPagesFromXMLText(arch, 0);
  return pages.length ? [{ id: "notebook-standalone", pages }] : [];
}

function formMainFieldNodesFromXMLText(arch: string): ViewFieldNode[] {
  return fieldNodesFromXMLText(stripFormNotebookXMLText(arch));
}

function stripFormNotebookXMLText(arch: string): string {
  return arch
    .replace(/<notebook\b[^>]*>[\s\S]*?<\/notebook>/gi, "")
    .replace(/<page\b[^>]*>[\s\S]*?<\/page>/gi, "");
}

function formNotebookPagesFromXMLText(xml: string, notebookIndex: number): FormNotebookPage[] {
  const pages: FormNotebookPage[] = [];
  for (const match of xml.matchAll(/<page\b([^>]*)>([\s\S]*?)<\/page>/gi)) {
    const attrs = xmlAttributes(`<page${match[1]}>`);
    const fields = fieldNodesFromXMLText(match[2]);
    if (!fields.length) continue;
    const index = pages.length;
    pages.push({
      id: formNotebookPageID(attrs, index, notebookIndex),
      label: formNotebookPageLabel(attrs, index),
      attrs,
      fields
    });
  }
  return pages;
}

function formNotebookID(attrs: Record<string, string>, index: number): string {
  return `notebook-${slugID(attrs.name || attrs.string || String(index)) || index}`;
}

function formNotebookPageID(attrs: Record<string, string>, index: number, notebookIndex: number): string {
  return `page-${notebookIndex}-${slugID(attrs.name || attrs.string || String(index)) || index}`;
}

function formNotebookPageLabel(attrs: Record<string, string>, index: number): string {
  return attrs.string || attrs.name || `Page ${index + 1}`;
}

function slugID(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9_-]+/g, "-").replace(/^-+|-+$/g, "");
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

function formFooterButtonNodesFromXMLText(arch: string): ViewButtonNode[] {
  const out: ViewButtonNode[] = [];
  for (const match of arch.matchAll(/<footer\b[^>]*>([\s\S]*?)<\/footer>/gi)) {
    out.push(...buttonNodesFromXMLText(match[1]));
  }
  return out;
}

function viewButtonKey(node: ViewButtonNode): string {
  return [
    node.attrs.id || "",
    node.attrs.name || "",
    node.attrs.type || "object",
    node.attrs.string || ""
  ].join("\u0001");
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

function fieldLabel(fields: Record<string, unknown>, name: string, model?: string): string {
  const known = knownFieldLabel(model, name);
  if (known) return known;
  const description = fields[name];
  if (isRecord(description) && typeof description.string === "string" && !technicalFieldLabel(description.string, name)) return description.string;
  return humanizeFieldName(name);
}

function knownFieldLabel(model: string | undefined, name: string): string {
  if (model === "ir.actions.server") {
    switch (name) {
      case "name":
        return "Name";
      case "model_id":
      case "model_name":
        return "Model";
      case "state":
        return "Type";
      case "group_ids":
        return "Allowed Groups";
      case "code":
        return "Code";
      case "active":
        return "Active";
      case "usage":
        return "Usage";
      default:
        return "";
    }
  }
  if (model === "ir.cron") {
    switch (name) {
      case "priority":
        return "Priority";
      case "name":
        return "Action Name";
      case "model_id":
        return "Model";
      case "group_ids":
        return "Allowed Groups";
      case "active":
        return "Active";
      case "interval_number":
        return "Interval Number";
      case "interval_type":
        return "Interval Unit";
      case "nextcall":
        return "Next Execution Date";
      case "ir_actions_server_id":
        return "Server Action";
      case "user_id":
        return "Scheduler User";
      case "state":
        return "Action Type";
      case "code":
        return "Code";
      default:
        return "";
    }
  }
  if (model === "base.automation") {
    switch (name) {
      case "name":
        return "Name";
      case "active":
        return "Active";
      case "model_id":
      case "model_name":
        return "Model";
      case "trigger":
        return "Trigger";
      case "action_server_id":
        return "Server Action";
      case "description":
        return "Description";
      default:
        return "";
    }
  }
  if (model === "res.users") {
    switch (name) {
      case "name":
        return "Name";
      case "login":
        return "Login";
      case "email":
        return "Email";
      case "company_id":
        return "Company";
      case "partner_id":
        return "Related Partner";
      case "groups_count":
        return "Groups";
      case "group_ids":
        return "Access Rights";
      case "role":
        return "Role";
      case "active":
        return "Active";
      case "lang":
        return "Language";
      case "tz":
        return "Timezone";
      case "notification_type":
        return "Notification";
      case "color_scheme":
        return "Theme";
      case "signature":
        return "Email Signature";
      default:
        return "";
    }
  }
  if (model === "res.groups") {
    switch (name) {
      case "privilege_id":
        return "Privilege";
      case "name":
        return "Name";
      case "full_name":
        return "Full Name";
      case "share":
        return "Share Group";
      case "api_key_duration":
        return "API Keys maximum duration days";
      case "user_ids":
        return "Users";
      case "implied_ids":
        return "Inherited";
      case "menu_access":
        return "Menus";
      case "view_access":
        return "Views";
      case "model_access":
        return "Access Rights";
      case "rule_groups":
        return "Record Rules";
      case "comment":
        return "Notes";
      default:
        return "";
    }
  }
  return "";
}

function technicalFieldLabel(label: string, name: string): boolean {
  const value = label.trim();
  if (!value) return true;
  return value === name || value === value.toLowerCase() || value.includes("_");
}

function humanizeFieldName(name: string): string {
  const source = name.replace(/_ids?$/, "").replace(/_/g, " ").trim() || name;
  return source.split(/\s+/).map((word) => word ? word[0].toUpperCase() + word.slice(1) : word).join(" ");
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

function selectionLabel(choices: readonly [string, string][], value: string): string {
  return choices.find(([candidate]) => candidate === value)?.[1] || value;
}

const serverActionStateSelectionOptions: Array<[string, string]> = [
  ["object_write", "Update Record"],
  ["object_create", "Create Record"],
  ["object_copy", "Duplicate Record"],
  ["code", "Execute Code"],
  ["webhook", "Send Webhook Notification"],
  ["multi", "Multi Actions"],
  ["mail_post", "Send Email"],
  ["followers", "Add Followers"],
  ["remove_followers", "Remove Followers"],
  ["next_activity", "Create Next Activity"],
  ["sms", "Send SMS"],
  ["whatsapp", "Send WhatsApp"],
  ["ai", "AI Action"],
  ["documents_account_record_create", "Create Vendor Bill"]
];

const serverActionUsageSelectionOptions: Array<[string, string]> = [
  ["ir_cron", "Scheduled Action"],
  ["ir_actions_server", "Server Action"]
];

const scheduledActionStateSelectionOptions: Array<[string, string]> = [
  ["code", "Execute Code"]
];

const scheduledActionIntervalSelectionOptions: Array<[string, string]> = [
  ["minutes", "Minutes"],
  ["hours", "Hours"],
  ["days", "Days"],
  ["weeks", "Weeks"],
  ["months", "Months"]
];

const automationTriggerSelectionOptions: Array<[string, string]> = [
  ["create", "On Creation"],
  ["write", "On Update"],
  ["create_or_write", "On Creation & Update"],
  ["archive", "On Archive"],
  ["unarchive", "On Unarchive"],
  ["unlink", "On Deletion"],
  ["onchange", "On UI Change"],
  ["message", "On Incoming Message"],
  ["webhook", "On Webhook"],
  ["time", "Based on Timed Condition"],
  ["manual", "Manually"]
];

const userRoleSelectionOptions: Array<[string, string]> = [
  ["group_user", "User"],
  ["group_system", "Administrator"],
  ["group_portal", "Portal"],
  ["group_public", "Public"]
];

function selectionOptionsForField(description: unknown, model: string | undefined, fieldName: string): Array<[string, string]> {
  const choices = selectionOptions(description);
  if (choices.length) return choices;
  if (model === "ir.actions.server" && fieldName === "state" && fieldTypeValue(description) === "selection") {
    return serverActionStateSelectionOptions;
  }
  if (model === "ir.actions.server" && fieldName === "usage" && fieldTypeValue(description) === "selection") {
    return serverActionUsageSelectionOptions;
  }
  if (model === "ir.cron" && fieldName === "state" && fieldTypeValue(description) === "selection") {
    return scheduledActionStateSelectionOptions;
  }
  if (model === "ir.cron" && fieldName === "interval_type" && fieldTypeValue(description) === "selection") {
    return scheduledActionIntervalSelectionOptions;
  }
  if (model === "base.automation" && fieldName === "trigger" && fieldTypeValue(description) === "selection") {
    return automationTriggerSelectionOptions;
  }
  if (model === "res.users" && fieldName === "role") return userRoleSelectionOptions;
  return [];
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

function relatedModelFields(relatedModels: Record<string, unknown>, relation: string): Record<string, unknown> {
  const modelInfo = relatedModels[relation];
  if (isRecord(modelInfo) && isRecord(modelInfo.fields)) return modelInfo.fields as Record<string, unknown>;
  return {};
}

function fieldTypeValue(description: unknown): string {
  if (!isRecord(description) || typeof description.type !== "string") return "";
  return description.type === "bool" ? "boolean" : description.type;
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
