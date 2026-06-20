export type AiButtonStatus = "idle" | "ready" | "loading" | "disabled" | "error";
export type AiChatRole = "user" | "assistant" | "system";
export type AiErrorCode = "auth" | "network" | "rate_limit" | "server" | "validation" | "unknown";
export type AiProvider = "openai" | "bedrock" | "custom" | "disabled";

export const aiRoutes = {
  generateResponse: "/ai/generate_response",
  closeAIChat: "/ai/close_ai_chat",
  transcriptionSession: "/ai/transcription/session"
} as const;

export const agentChatActionTag = "agent_chat_action" as const;
export const aiPromptButtonStoragePrefix = "ai.thread.prompt_buttons." as const;
export const defaultTranscriptionTokenLifespanSeconds = 7200;

export interface AiButtonOptions {
  status?: AiButtonStatus;
  label?: string;
  panelOpen?: boolean;
  unreadCount?: number;
  disabledReason?: string;
  error?: AiErrorState | null;
}

export interface AiButtonState {
  status: AiButtonStatus;
  label: string;
  disabled: boolean;
  busy: boolean;
  expanded: boolean;
  badgeCount: number;
  ariaLabel: string;
  reason?: string;
}

export interface SourceCitationInput {
  id?: string;
  title?: string;
  uri?: string;
  excerpt?: string;
  line?: number;
}

export interface SourceCitation {
  id: string;
  title: string;
  uri?: string;
  excerpt?: string;
  line?: number;
}

export interface AiChatMessageInput {
  id: string;
  role: AiChatRole;
  content: string;
  createdAt?: string;
  citations?: readonly SourceCitationInput[];
  error?: AiErrorState;
}

export interface AiChatMessage {
  id: string;
  role: AiChatRole;
  content: string;
  createdAt?: string;
  citations: SourceCitation[];
  error?: AiErrorState;
}

export interface AiChatPanelOptions {
  open?: boolean;
  title?: string;
  input?: string;
  pending?: boolean;
  selectedPromptId?: string;
  messages?: readonly AiChatMessageInput[];
  error?: AiErrorState | null;
}

export interface AiChatPanelState {
  open: boolean;
  title: string;
  input: string;
  pending: boolean;
  selectedPromptId?: string;
  messages: AiChatMessage[];
  error?: AiErrorState;
}

export interface PromptButtonInput {
  id: string;
  label: string;
  prompt: string;
  disabled?: boolean;
  group?: string;
}

export interface PromptButton {
  id: string;
  label: string;
  prompt: string;
  disabled: boolean;
  group?: string;
}

export interface PromptButtonSelection {
  button: PromptButton;
  prompt: string;
}

export interface AiAdminSettingsInput {
  enabled?: boolean;
  provider?: AiProvider;
  model?: string;
  endpoint?: string;
  maxPromptButtons?: number;
  maxContextSources?: number;
  timeoutMs?: number;
  citationsRequired?: boolean;
}

export interface AiAdminSettings {
  enabled: boolean;
  provider: AiProvider;
  model: string;
  endpoint?: string;
  maxPromptButtons: number;
  maxContextSources: number;
  timeoutMs: number;
  citationsRequired: boolean;
}

export interface AiValidationIssue {
  field: string;
  code: string;
  message: string;
}

export interface AiAdminSettingsValidation {
  valid: boolean;
  settings: AiAdminSettings;
  issues: AiValidationIssue[];
}

export interface AiErrorStateOptions {
  code?: AiErrorCode;
  message?: string;
  status?: number;
  retryable?: boolean;
  details?: string;
}

export interface AiErrorState {
  code: AiErrorCode;
  message: string;
  retryable: boolean;
  status?: number;
  details?: string;
}

export interface AiCurrentViewInfo {
  action_id?: number;
  view_id?: number;
  model?: string;
  view_type?: string;
  available_view_types?: readonly string[];
  facets?: readonly Record<string, unknown>[];
  [key: string]: unknown;
}

export interface AiJsonRpcRequest<TParams extends object> {
  route: string;
  params: TParams;
}

export interface AiGenerateResponseInput {
  mailMessageId: number;
  channelId: number;
  currentViewInfo?: AiCurrentViewInfo;
  sessionIdentifier?: string;
}

export interface AiGenerateResponseParams {
  mail_message_id: number;
  channel_id: number;
  current_view_info?: AiCurrentViewInfo;
  ai_session_identifier?: string;
}

export interface AiCloseChatParams {
  channel_id: number;
}

export interface AiTranscriptionSessionParams {
  language: string;
  prompt: string;
}

export interface AiOrmCallRequest<TArgs extends readonly unknown[] = readonly unknown[]> {
  model: string;
  method: string;
  args: TArgs;
  kwargs: Record<string, unknown>;
}

export interface AiDraftChannelInput {
  callerComponentName: string;
  channelTitle?: string | null;
  recordModel?: string | null;
  recordId?: number | null;
  frontEndRecordInfo?: unknown;
  textSelection?: string | null;
}

export type AiDraftChannelArgs = [
  callerComponentName: string,
  channelTitle: string | null,
  recordModel: string | null,
  recordId: number | null,
  frontEndRecordInfo: unknown,
  textSelection: string | null
];

export interface AiDraftChannelResult {
  aiChannelId: number;
  data: unknown;
  prompts: string[];
  modelHasThread: boolean;
}

export interface AiRecordFieldInfo {
  type?: string;
  [key: string]: unknown;
}

export type AiRecordFieldsInfo = Record<string, AiRecordFieldInfo>;

export interface AiAgentChatActionParams {
  channelId: number | string;
  user_prompt?: string;
  [key: string]: unknown;
}

export interface AiAgentChatAction {
  type: "ir.actions.client";
  tag: typeof agentChatActionTag;
  params: AiAgentChatActionParams;
  [key: string]: unknown;
}

export interface AiAgentChatActionState {
  tag: typeof agentChatActionTag;
  threadModel: "discuss.channel";
  channelId: number;
  userPrompt: string;
  shouldPost: boolean;
  focus: true;
}

export interface AiAgentChatThread {
  status?: string;
  isLoadedDeferred?: PromiseLike<unknown>;
  open?(options: { focus: true }): unknown | Promise<unknown>;
  openChatWindow?(): unknown | Promise<unknown>;
  post?(message: string): unknown | Promise<unknown>;
}

export interface AiAgentChatThreadLookup {
  model: "discuss.channel";
  id: number;
}

export interface AiAgentChatActionHandlerOptions {
  getThread(thread: AiAgentChatThreadLookup): AiAgentChatThread | null | Promise<AiAgentChatThread | null>;
}

export interface AiActionInvocationLike {
  action: Record<string, unknown>;
  options?: Record<string, unknown>;
}

export interface AiAgentChatActionHandler {
  (invocation: AiActionInvocationLike): Promise<AiAgentChatActionState>;
  (env: unknown, action: Record<string, unknown>, options?: Record<string, unknown>): Promise<AiAgentChatActionState>;
}

export type AiOpenMenuEventName =
  | "AI_OPEN_MENU_LIST"
  | "AI_OPEN_MENU_KANBAN"
  | "AI_OPEN_MENU_PIVOT"
  | "AI_OPEN_MENU_GRAPH";

export type AiNaturalLanguageEventName = AiOpenMenuEventName | "AI_ADJUST_SEARCH";

export interface AiMenuRecord {
  id: number;
  actionID?: number;
  actionId?: number;
  [key: string]: unknown;
}

export interface AiOpenMenuPayload {
  menuID: number;
  aiSessionIdentifier?: string;
  selectedFilters?: readonly string[];
  selectedGroupBys?: readonly string[];
  rowGroupBys?: readonly string[];
  colGroupBys?: readonly string[];
  groupBys?: readonly string[];
  measures?: readonly string[];
  measure?: string;
  mode?: string;
  order?: string;
  stacked?: boolean;
  cumulated?: boolean;
  search?: readonly string[];
  sortedColumn?: Record<string, unknown>;
  customDomain?: unknown;
  [key: string]: unknown;
}

export interface AiOpenMenuInvocation {
  action: Record<string, unknown>;
  options: {
    viewType: "list" | "kanban" | "pivot" | "graph";
    clearBreadcrumbs: true;
    props: {
      ai: Record<string, unknown>;
    };
    menu: AiMenuRecord;
  };
}

export interface AiAdjustSearchPayload {
  aiSessionIdentifier?: string;
  removeFacets?: readonly string[];
  toggleFilters?: readonly string[];
  toggleGroupBys?: readonly string[];
  applySearches?: readonly string[];
  measures?: readonly string[];
  mode?: string | null;
  order?: string;
  stacked?: boolean;
  cumulated?: boolean;
  customDomain?: unknown;
  switchViewType?: string | false;
  [key: string]: unknown;
}

export interface AiBusApplicationEvent {
  name: "APPLY_AI_ADJUST_SEARCH";
  detail: Record<string, unknown>;
}

export interface RealtimeParameters {
  expires_after: {
    anchor: "created_at";
    seconds: number;
  };
  session: {
    type: "transcription";
    audio: {
      input: {
        transcription: {
          language: string;
          model: "whisper-1";
          prompt: string;
        };
        turn_detection: {
          type: "server_vad";
        };
        noise_reduction: {
          type: "far_field";
        };
      };
    };
  };
}

export const defaultAiAdminSettings: AiAdminSettings = {
  enabled: false,
  provider: "disabled",
  model: "",
  maxPromptButtons: 6,
  maxContextSources: 8,
  timeoutMs: 30000,
  citationsRequired: true
};

export function createAiButtonState(options: AiButtonOptions = {}): AiButtonState {
  const status = options.status ?? (options.error ? "error" : "idle");
  const label = nonEmpty(options.label) ?? "AI";
  const badgeCount = Math.max(0, Math.floor(options.unreadCount ?? 0));
  const reason = options.disabledReason ?? options.error?.message;
  const ariaLabel = [label, status, badgeCount ? `${badgeCount} unread` : undefined, reason]
    .filter(Boolean)
    .join(", ");

  return {
    status,
    label,
    disabled: status === "disabled" || status === "loading",
    busy: status === "loading",
    expanded: Boolean(options.panelOpen),
    badgeCount,
    ariaLabel,
    ...(reason ? { reason } : {})
  };
}

export function createChatPanelState(options: AiChatPanelOptions = {}): AiChatPanelState {
  return {
    open: Boolean(options.open),
    title: nonEmpty(options.title) ?? "AI Assistant",
    input: options.input ?? "",
    pending: Boolean(options.pending),
    ...(options.selectedPromptId ? { selectedPromptId: options.selectedPromptId } : {}),
    messages: (options.messages ?? []).map(createChatMessage),
    ...(options.error ? { error: options.error } : {})
  };
}

export function setChatPanelOpen(state: AiChatPanelState, open: boolean): AiChatPanelState {
  return { ...state, open };
}

export function setChatPanelInput(state: AiChatPanelState, input: string): AiChatPanelState {
  return { ...state, input };
}

export function setChatPanelPending(state: AiChatPanelState, pending: boolean): AiChatPanelState {
  return { ...state, pending };
}

export function addChatMessage(state: AiChatPanelState, message: AiChatMessageInput): AiChatPanelState {
  return {
    ...state,
    messages: [...state.messages, createChatMessage(message)],
    error: message.error ?? state.error
  };
}

export function createPromptButtons(buttons: readonly PromptButtonInput[]): PromptButton[] {
  const seen = new Set<string>();
  return buttons.map((button) => {
    const id = requireText(button.id, "prompt button id");
    const group = nonEmpty(button.group);
    if (seen.has(id)) throw new Error(`duplicate prompt button id: ${id}`);
    seen.add(id);
    return {
      id,
      label: requireText(button.label, "prompt button label"),
      prompt: requireText(button.prompt, "prompt button prompt"),
      disabled: Boolean(button.disabled),
      ...(group ? { group } : {})
    };
  });
}

export function selectPromptButton(buttons: readonly PromptButton[], id: string): PromptButtonSelection {
  const button = buttons.find((candidate) => candidate.id === id);
  if (!button) throw new Error(`prompt button not found: ${id}`);
  if (button.disabled) throw new Error(`prompt button disabled: ${id}`);
  return { button, prompt: button.prompt };
}

export function normalizeSourceCitations(citations: readonly SourceCitationInput[]): SourceCitation[] {
  const seen = new Set<string>();
  const normalized: SourceCitation[] = [];

  citations.forEach((citation, index) => {
    const uri = nonEmpty(citation.uri);
    const title = nonEmpty(citation.title) ?? uri ?? `Source ${index + 1}`;
    const id = nonEmpty(citation.id) ?? citationKey(title, uri, citation.line);
    const excerpt = nonEmpty(citation.excerpt);
    if (seen.has(id)) return;
    seen.add(id);
    normalized.push({
      id,
      title,
      ...(uri ? { uri } : {}),
      ...(excerpt ? { excerpt } : {}),
      ...(citation.line !== undefined ? { line: citation.line } : {})
    });
  });

  return normalized;
}

export function formatCitationLabel(citation: SourceCitation, index = 0): string {
  const marker = `[${index + 1}]`;
  if (citation.line !== undefined) return `${marker} ${citation.title}:${citation.line}`;
  return `${marker} ${citation.title}`;
}

export function validateAiAdminSettings(input: AiAdminSettingsInput = {}): AiAdminSettingsValidation {
  const settings: AiAdminSettings = {
    ...defaultAiAdminSettings,
    ...input,
    provider: input.provider ?? (input.enabled ? "openai" : defaultAiAdminSettings.provider),
    model: input.model?.trim() ?? defaultAiAdminSettings.model,
    endpoint: input.endpoint?.trim() || undefined
  };
  const issues: AiValidationIssue[] = [];

  if (!["openai", "bedrock", "custom", "disabled"].includes(settings.provider)) {
    issues.push(issue("provider", "invalid_provider", "Provider is not supported."));
  }
  if (settings.enabled && settings.provider === "disabled") {
    issues.push(issue("provider", "provider_disabled", "Enabled AI requires an active provider."));
  }
  if (settings.enabled && !settings.model) {
    issues.push(issue("model", "required", "Enabled AI requires a model."));
  }
  if (settings.provider === "custom" && settings.enabled && !settings.endpoint) {
    issues.push(issue("endpoint", "required", "Custom AI provider requires an endpoint."));
  }
  if (settings.endpoint && !isHttpUrl(settings.endpoint)) {
    issues.push(issue("endpoint", "invalid_url", "Endpoint must be an http or https URL."));
  }
  if (!isIntegerInRange(settings.maxPromptButtons, 0, 24)) {
    issues.push(issue("maxPromptButtons", "out_of_range", "Prompt button count must be between 0 and 24."));
  }
  if (!isIntegerInRange(settings.maxContextSources, 0, 64)) {
    issues.push(issue("maxContextSources", "out_of_range", "Context source count must be between 0 and 64."));
  }
  if (!isIntegerInRange(settings.timeoutMs, 1000, 120000)) {
    issues.push(issue("timeoutMs", "out_of_range", "Timeout must be between 1000 and 120000 ms."));
  }

  return {
    valid: issues.length === 0,
    settings,
    issues
  };
}

export function createAiErrorState(error: unknown, options: AiErrorStateOptions = {}): AiErrorState {
  const status = options.status ?? readStatus(error);
  const code = options.code ?? classifyError(status);
  const message = nonEmpty(options.message) ?? readMessage(error) ?? defaultErrorMessage(code);
  const retryable = options.retryable ?? defaultRetryable(code);
  const details = nonEmpty(options.details);

  return {
    code,
    message,
    retryable,
    ...(status !== undefined ? { status } : {}),
    ...(details ? { details } : {})
  };
}

export function createGenerateResponseRequest(
  input: AiGenerateResponseInput
): AiJsonRpcRequest<AiGenerateResponseParams> {
  const params: AiGenerateResponseParams = {
    mail_message_id: requirePositiveInteger(input.mailMessageId, "mail message id"),
    channel_id: requirePositiveInteger(input.channelId, "channel id")
  };
  if (input.currentViewInfo) params.current_view_info = input.currentViewInfo;
  const sessionIdentifier = nonEmpty(input.sessionIdentifier);
  if (sessionIdentifier) params.ai_session_identifier = sessionIdentifier;
  return { route: aiRoutes.generateResponse, params };
}

export function createCloseAIChatRequest(channelId: number): AiJsonRpcRequest<AiCloseChatParams> {
  return {
    route: aiRoutes.closeAIChat,
    params: { channel_id: requirePositiveInteger(channelId, "channel id") }
  };
}

export function createTranscriptionSessionRequest(
  language: string,
  prompt: string
): AiJsonRpcRequest<AiTranscriptionSessionParams> {
  return {
    route: aiRoutes.transcriptionSession,
    params: {
      language: requireText(language, "transcription language"),
      prompt: prompt ?? ""
    }
  };
}

export function createAIDraftChannelCall(input: AiDraftChannelInput): AiOrmCallRequest<AiDraftChannelArgs> {
  const recordId = input.recordId === undefined || input.recordId === null
    ? null
    : requirePositiveInteger(input.recordId, "record id");
  return {
    model: "discuss.channel",
    method: "create_ai_draft_channel",
    args: [
      requireText(input.callerComponentName, "caller component name"),
      nullableText(input.channelTitle),
      nullableText(input.recordModel),
      recordId,
      input.frontEndRecordInfo ?? null,
      nullableText(input.textSelection)
    ],
    kwargs: {}
  };
}

export function createAskAIActionCall(userPrompt = ""): AiOrmCallRequest<[string]> {
  return {
    model: "ai.agent",
    method: "action_ask_ai",
    args: [userPrompt ?? ""],
    kwargs: {}
  };
}

export function createGetAskAIAgentCall(): AiOrmCallRequest<[]> {
  return {
    model: "ai.agent",
    method: "get_ask_ai_agent",
    args: [],
    kwargs: {}
  };
}

export function normalizeAIDraftChannelResult(result: unknown): AiDraftChannelResult {
  const value = requireRecord(result, "AI draft channel result");
  const prompts = value.prompts;
  if (!Array.isArray(prompts)) throw new Error("AI draft channel prompts should be an array");
  return {
    aiChannelId: positiveIntegerFromUnknown(value.ai_channel_id, "AI channel id"),
    data: value.data ?? null,
    prompts: prompts.map((prompt) => requireText(String(prompt), "AI prompt name")),
    modelHasThread: Boolean(value.model_has_thread)
  };
}

export function createAIPromptButtonStorageKey(channelId: number): string {
  return `${aiPromptButtonStoragePrefix}${requirePositiveInteger(channelId, "channel id")}`;
}

export function createAIPromptButtonStorageValue(prompts: readonly string[]): string {
  return JSON.stringify(prompts.map((prompt) => requireText(prompt, "AI prompt name")));
}

export function createAIRecordContext(
  recordData: Record<string, unknown>,
  fieldsInfo: AiRecordFieldsInfo = {}
): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const fieldName of Object.keys(recordData)) {
    const fieldValue = recordData[fieldName];
    const fieldInfo = fieldsInfo[fieldName] ?? {};
    switch (fieldInfo.type) {
      case "binary":
        break;
      case "many2one":
        result[fieldName] = displayName(fieldValue);
        break;
      case "many2many":
      case "one2many": {
        const records = relationalRecords(fieldValue);
        if (records.length > 50) break;
        result[fieldName] = records.map(displayName).filter((name) => name !== null);
        break;
      }
      default:
        result[fieldName] = fieldValue;
    }
  }
  return result;
}

export function createAgentChatActionState(action: Record<string, unknown>): AiAgentChatActionState {
  if (action.type !== "ir.actions.client" || action.tag !== agentChatActionTag) {
    throw new Error(`Expected ${agentChatActionTag} client action`);
  }
  const params = requireRecord(action.params, "AI agent chat action params");
  const userPrompt = typeof params.user_prompt === "string" ? params.user_prompt : "";
  return {
    tag: agentChatActionTag,
    threadModel: "discuss.channel",
    channelId: positiveIntegerFromUnknown(params.channelId, "channel id"),
    userPrompt,
    shouldPost: userPrompt.length > 0,
    focus: true
  };
}

export function createAgentChatActionHandler(
  options: AiAgentChatActionHandlerOptions
): AiAgentChatActionHandler {
  return (async (first: unknown, second?: Record<string, unknown>) => {
    const action = second ?? requireRecord((first as AiActionInvocationLike).action, "AI action invocation action");
    const state = createAgentChatActionState(action);
    const thread = await options.getThread({ model: state.threadModel, id: state.channelId });
    if (!thread) throw new Error("Thread not found");
    await thread.open?.({ focus: true });
    await thread.openChatWindow?.();
    if (thread.isLoadedDeferred) await thread.isLoadedDeferred;
    if (state.shouldPost && thread.status !== "loading") {
      if (!thread.post) throw new Error("Thread cannot post messages");
      await thread.post(state.userPrompt);
    }
    return state;
  }) as AiAgentChatActionHandler;
}

export function createRealtimeTranscriptionParameters(language: string, prompt: string): RealtimeParameters {
  return {
    expires_after: {
      anchor: "created_at",
      seconds: defaultTranscriptionTokenLifespanSeconds
    },
    session: {
      type: "transcription",
      audio: {
        input: {
          transcription: {
            language: requireText(language, "transcription language"),
            model: "whisper-1",
            prompt: prompt ?? ""
          },
          turn_detection: { type: "server_vad" },
          noise_reduction: { type: "far_field" }
        }
      }
    }
  };
}

export function isAiNaturalLanguageEventName(value: string): value is AiNaturalLanguageEventName {
  return value === "AI_ADJUST_SEARCH" || isAiOpenMenuEventName(value);
}

export function isAiOpenMenuEventName(value: string): value is AiOpenMenuEventName {
  return (
    value === "AI_OPEN_MENU_LIST" ||
    value === "AI_OPEN_MENU_KANBAN" ||
    value === "AI_OPEN_MENU_PIVOT" ||
    value === "AI_OPEN_MENU_GRAPH"
  );
}

export function createAiOpenMenuInvocation(
  eventName: AiOpenMenuEventName,
  payload: AiOpenMenuPayload,
  menu: AiMenuRecord,
  sessionIdentifier?: string
): AiOpenMenuInvocation | null {
  if (!aiSessionMatches(payload.aiSessionIdentifier, sessionIdentifier)) return null;
  const actionID = menu.actionID ?? menu.actionId;
  if (!Number.isInteger(actionID) || Number(actionID) <= 0) return null;
  const viewType = aiOpenMenuViewType(eventName);
  return {
    action: { id: actionID, type: "ir.actions.act_window" },
    options: {
      viewType,
      clearBreadcrumbs: true,
      props: { ai: aiOpenMenuProps(eventName, payload) },
      menu: { ...menu }
    }
  };
}

export function createAiAdjustSearchEvents(
  payload: AiAdjustSearchPayload,
  sessionIdentifier?: string
): AiBusApplicationEvent[] {
  if (!aiSessionMatches(payload.aiSessionIdentifier, sessionIdentifier)) return [];
  const detail: Record<string, unknown> = {
    removeFacets: [...(payload.removeFacets ?? [])],
    toggleFilters: [...(payload.toggleFilters ?? [])],
    toggleGroupBys: [...(payload.toggleGroupBys ?? [])],
    applySearches: [...(payload.applySearches ?? [])],
    measures: [...(payload.measures ?? [])],
    mode: payload.mode ?? null,
    order: payload.order ?? "ASC",
    stacked: Boolean(payload.stacked),
    cumulated: Boolean(payload.cumulated)
  };
  if (payload.switchViewType) detail.switchViewType = payload.switchViewType;
  if (payload.customDomain !== undefined) detail.customDomain = payload.customDomain;
  return [{ name: "APPLY_AI_ADJUST_SEARCH", detail }];
}

function createChatMessage(message: AiChatMessageInput): AiChatMessage {
  return {
    id: requireText(message.id, "chat message id"),
    role: message.role,
    content: message.content,
    ...(message.createdAt ? { createdAt: message.createdAt } : {}),
    citations: normalizeSourceCitations(message.citations ?? []),
    ...(message.error ? { error: message.error } : {})
  };
}

function citationKey(title: string, uri: string | undefined, line: number | undefined): string {
  return [uri ?? title, line ?? ""].join("#");
}

function classifyError(status: number | undefined): AiErrorCode {
  if (status === 401 || status === 403) return "auth";
  if (status === 429) return "rate_limit";
  if (status !== undefined && status >= 500) return "server";
  if (status !== undefined && status >= 400) return "validation";
  if (status === 0) return "network";
  return "unknown";
}

function defaultErrorMessage(code: AiErrorCode): string {
  switch (code) {
    case "auth":
      return "AI request is not authorized.";
    case "network":
      return "AI request could not reach the service.";
    case "rate_limit":
      return "AI request was rate limited.";
    case "server":
      return "AI service returned an error.";
    case "validation":
      return "AI request is invalid.";
    case "unknown":
      return "AI request failed.";
  }
}

function defaultRetryable(code: AiErrorCode): boolean {
  return code === "network" || code === "rate_limit" || code === "server";
}

function isHttpUrl(value: string): boolean {
  try {
    const url = new URL(value);
    return url.protocol === "http:" || url.protocol === "https:";
  } catch {
    return false;
  }
}

function isIntegerInRange(value: number, min: number, max: number): boolean {
  return Number.isInteger(value) && value >= min && value <= max;
}

function issue(field: string, code: string, message: string): AiValidationIssue {
  return { field, code, message };
}

function nonEmpty(value: string | null | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

function nullableText(value: string | null | undefined): string | null {
  return nonEmpty(value) ?? null;
}

function requireText(value: string, field: string): string {
  const text = nonEmpty(value);
  if (!text) throw new Error(`${field} is required`);
  return text;
}

function requirePositiveInteger(value: number, field: string): number {
  if (!Number.isInteger(value) || value <= 0) throw new Error(`${field} must be a positive integer`);
  return value;
}

function positiveIntegerFromUnknown(value: unknown, field: string): number {
  const numeric = typeof value === "number" ? value : typeof value === "string" ? Number(value) : NaN;
  return requirePositiveInteger(numeric, field);
}

function requireRecord(value: unknown, field: string): Record<string, unknown> {
  if (!isRecord(value)) throw new Error(`${field} should be an object`);
  return value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function relationalRecords(value: unknown): unknown[] {
  if (!isRecord(value) || !Array.isArray(value.records)) return [];
  return value.records;
}

function displayName(value: unknown): string | null {
  if (!isRecord(value)) return null;
  const data = isRecord(value.data) ? value.data : value;
  const display = data.display_name ?? data.name;
  return typeof display === "string" && display.length > 0 ? display : null;
}

function aiSessionMatches(eventSession: string | undefined, currentSession: string | undefined): boolean {
  if (!eventSession) return true;
  return eventSession === currentSession;
}

function aiOpenMenuViewType(eventName: AiOpenMenuEventName): "list" | "kanban" | "pivot" | "graph" {
  switch (eventName) {
    case "AI_OPEN_MENU_KANBAN":
      return "kanban";
    case "AI_OPEN_MENU_PIVOT":
      return "pivot";
    case "AI_OPEN_MENU_GRAPH":
      return "graph";
    case "AI_OPEN_MENU_LIST":
      return "list";
  }
}

function aiOpenMenuProps(eventName: AiOpenMenuEventName, payload: AiOpenMenuPayload): Record<string, unknown> {
  switch (eventName) {
    case "AI_OPEN_MENU_LIST":
    case "AI_OPEN_MENU_KANBAN":
      return withCustomDomain(payload, {
        selectedFilters: [...(payload.selectedFilters ?? [])],
        selectedGroupBys: [...(payload.selectedGroupBys ?? [])],
        search: [...(payload.search ?? [])]
      });
    case "AI_OPEN_MENU_PIVOT":
      return withCustomDomain(payload, {
        selectedFilters: [...(payload.selectedFilters ?? [])],
        selectedGroupBys: [...(payload.rowGroupBys ?? [])],
        colGroupBys: [...(payload.colGroupBys ?? [])],
        measures: [...(payload.measures ?? [])],
        search: [...(payload.search ?? [])],
        ...(payload.sortedColumn ? { sortedColumn: { ...payload.sortedColumn } } : {})
      });
    case "AI_OPEN_MENU_GRAPH":
      return withCustomDomain(payload, {
        selectedFilters: [...(payload.selectedFilters ?? [])],
        groupBys: [...(payload.groupBys ?? [])],
        measure: payload.measure ?? "",
        mode: payload.mode ?? "",
        order: payload.order ?? "ASC",
        stacked: Boolean(payload.stacked),
        cumulated: Boolean(payload.cumulated),
        search: [...(payload.search ?? [])]
      });
  }
}

function withCustomDomain(payload: { customDomain?: unknown }, props: Record<string, unknown>): Record<string, unknown> {
  if (payload.customDomain !== undefined) props.customDomain = payload.customDomain;
  return props;
}

function readMessage(error: unknown): string | undefined {
  if (error instanceof Error) return nonEmpty(error.message);
  if (typeof error === "string") return nonEmpty(error);
  if (error && typeof error === "object") {
    const message = (error as { message?: unknown }).message;
    return typeof message === "string" ? nonEmpty(message) : undefined;
  }
  return undefined;
}

function readStatus(error: unknown): number | undefined {
  if (!error || typeof error !== "object") return undefined;
  const status = (error as { status?: unknown; statusCode?: unknown }).status;
  const statusCode = (error as { statusCode?: unknown }).statusCode;
  if (typeof status === "number") return status;
  if (typeof statusCode === "number") return statusCode;
  return undefined;
}
