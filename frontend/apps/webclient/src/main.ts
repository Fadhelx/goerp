import {
  createWebClient,
  makeEnv,
  parseRouteState,
  registries,
  renderWindowAction,
  renderWindowActionDialog,
  routeStateFromStack,
  startServices,
  updateBrowserRoute,
  type ActionRequest,
  type ActionRouteSource,
  type ActionService,
  type ActionServiceOptions,
  applyMailRecordInsertToChatter,
  type RPCRequest,
  type SearchFacet,
  type SessionService,
  type WebClientRouteState,
  type WebClientServices,
  type WindowActionResult
} from "../../../packages/webclient/src/index.js";
import {
  agentChatActionTag,
  createAiAdjustSearchEvents,
  createAiOpenMenuInvocation,
  createAgentChatActionHandler,
  createAIPromptButtonStorageKey,
  createAIPromptButtonStorageValue,
  isAiNaturalLanguageEventName,
  isAiOpenMenuEventName,
  type AiAgentChatThreadLookup,
  type AiMailThread
} from "../../../packages/ai/src/index.js";
import type { RenderedWebClientShell } from "../../../packages/webclient/src/webclient/shell.js";
import type { NavbarSystrayAction } from "../../../packages/webclient/src/webclient/navbar/navbar.js";
import {
  appIconToken,
  appInitials,
  cleanAppName,
  homeMenuEntry,
  isAppsCatalogApp,
  normalizeHomeMenuApps,
  type HomeMenuApp,
  type HomeMenuEntry,
  type HomeMenuPayload
} from "../../../packages/webclient/src/home_menu/app_metadata.js";
import { enterpriseLikeTheme } from "../../../themes/enterprise-like/src/theme.js";

export interface GoERPWebClientBootstrapResult {
  session: Record<string, unknown>;
  menus: Record<string, unknown>;
}

interface AIChatThreadState extends AiMailThread {
  id: number;
  model: "discuss.channel";
  status: string;
  messages: AIChatMessageState[];
  env: ReturnType<typeof makeEnv>;
  container?: HTMLElement;
  promptButtons?: string[];
}

interface AIChatMessageState {
  id?: number;
  role: "user" | "assistant" | "system";
  body: string;
  localKey?: string;
}

export async function bootstrapGoERPWebClient(): Promise<GoERPWebClientBootstrapResult> {
  const env = makeEnv({
    debug: new URLSearchParams(globalThis.location?.search ?? "").has("debug"),
    services: {}
  });
  const isSmall = globalThis.matchMedia?.("(max-width: 767px)")?.matches === true;
  env.rpcTransport = rpcTransport;
  await startServices(env);
  registerAIClientAction(env);
  let session = await (env.services.session as SessionService).load();
  if (shouldQuickLogin(session)) {
    session = await fetchJSON<Record<string, unknown>>("/web/session/authenticate", {
      login: "admin",
      password: "admin"
    });
  }
  session = await loadSystraySession(session);
  const menus = await fetchJSON<Record<string, unknown>>("/web/webclient/load_menus");
  if (shouldTakeOverDOM()) {
    const target = ensureRoot();
    let shell!: RenderedWebClientShell;
    const runSystrayAction = (action: NavbarSystrayAction, outlet: HTMLElement) => {
      void handleSystrayAction(env, action, outlet, {
        onActivityStore: (store) => {
          updateActivitySystray(shell, store, (nextAction) => runSystrayAction(nextAction, outlet));
        }
      }).catch((error) => renderActionError(outlet, error));
    };
    shell = createWebClient({
      env: { debug: Boolean(env.debug), isSmall },
      theme: enterpriseLikeTheme,
      session,
      menus,
      onOpenApp: (app, outlet) => {
        void openMenuApp(env, menus as HomeMenuPayload, app, outlet).catch((error) => renderActionError(outlet, error));
      },
      onSystrayAction: runSystrayAction
    }).render() as RenderedWebClientShell;
    target.replaceChildren(shell);
    let restoringHash = false;
    const restoreCurrentHash = () => {
      if (restoringHash) return;
      restoringHash = true;
      void restoreActionFromHash(env, menus as HomeMenuPayload, shell).finally(() => {
        restoringHash = false;
      });
    };
    await restoreActionFromHash(env, menus as HomeMenuPayload, shell);
    globalThis.addEventListener?.("hashchange", restoreCurrentHash);
    startAIBusPolling(env, menus as HomeMenuPayload, shell, session);
  }
  globalThis.document.documentElement.dataset.tsWebclient = "ready";
  globalThis.dispatchEvent(new CustomEvent("goerp:webclient-ready", {
    detail: { session, menus }
  }));
  return { session, menus };
}

const aiThreadStore = new Map<number, AIChatThreadState>();

function registerAIClientAction(env: ReturnType<typeof makeEnv>): void {
  registries.actions.add(agentChatActionTag, createAgentChatActionHandler({
    getThread(thread) {
      return getOrCreateAIThread(thread, env);
    }
  }), { force: true });
}

function getOrCreateAIThread(thread: AiAgentChatThreadLookup, env: ReturnType<typeof makeEnv>): AIChatThreadState {
  const channelID = numericUserID(thread.id);
  if (!channelID) throw new Error("AI channel id is required");
  const existing = aiThreadStore.get(channelID);
  if (existing) return existing;
  const state: AIChatThreadState = {
    id: channelID,
    model: "discuss.channel",
    status: "ready",
    messages: [],
    env,
    promptButtons: readAIPromptButtons(channelID),
    ai_prompt_buttons: readAIPromptButtons(channelID),
    open(options) {
      renderAIChatPanel(state, env, options);
    },
    openChatWindow() {
      renderAIChatPanel(state, env, { focus: true });
    },
    async post(message: string) {
      const body = message.trim();
      if (!body) return;
      const localKey = `local-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
      addAIThreadMessage(state, { role: "user", body, localKey });
      state.status = "loading";
      renderAIChatPanel(state, env, { focus: true });
      try {
        const posted = await postAIUserMessage(channelID, body);
        const postedID = postedMessageID(posted);
        if (!postedID) throw new Error("AI message post did not return a message id.");
        if (postedID) mergeAIThreadMessageID(state, localKey, postedID);
        await fetchJSON("/ai/generate_response", {
          mail_message_id: postedID,
          channel_id: channelID,
          current_view_info: currentAIViewInfo(),
          ai_session_identifier: aiSessionIdentifier()
        });
      } catch (error) {
        addAIThreadMessage(state, { role: "assistant", body: error instanceof Error ? error.message : "AI request failed." });
      } finally {
        state.status = "ready";
        renderAIChatPanel(state, env, { focus: true });
      }
    }
  };
  aiThreadStore.set(channelID, state);
  if (state.promptButtons?.length) {
    globalThis.localStorage?.setItem?.(createAIPromptButtonStorageKey(channelID), createAIPromptButtonStorageValue(state.promptButtons));
  }
  return state;
}

function renderAIChatPanel(thread: AIChatThreadState, env: ReturnType<typeof makeEnv>, options: { focus?: boolean }): void {
  let container = thread.container;
  if (!container) {
    container = document.createElement("section");
    container.className = "gorp-ai-chat o-mail-ChatWindow o-mail-Thread";
    container.dataset.aiChannelId = String(thread.id);
    container.dataset.aiStatus = thread.status;
    document.body.append(container);
    thread.container = container;
  }
  container.dataset.aiStatus = thread.status;
  const header = document.createElement("header");
  header.className = "gorp-ai-chat-header o-mail-ChatWindow-header";
  const title = document.createElement("h2");
  title.textContent = "Ask AI";
  const close = document.createElement("button");
  close.type = "button";
  close.className = "btn btn-link o-mail-ChatWindow-command";
  close.textContent = "Close";
  close.addEventListener("click", async () => {
    await fetchJSON("/ai/close_ai_chat", { channel_id: thread.id }).catch(() => null);
    thread.container?.remove();
    thread.container = undefined;
    aiThreadStore.delete(thread.id);
    globalThis.localStorage?.removeItem?.(createAIPromptButtonStorageKey(thread.id));
  });
  header.append(title, close);

  const promptBar = document.createElement("div");
  promptBar.className = "gorp-ai-prompt-buttons o-mail-Thread-aiPromptButtons";
  for (const prompt of thread.promptButtons ?? []) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "btn btn-secondary o-mail-AI-prompt";
    button.textContent = prompt;
    button.addEventListener("click", () => void thread.post?.(prompt));
    promptBar.append(button);
  }

  const messages = document.createElement("div");
  messages.className = "gorp-ai-chat-messages o-mail-Thread-messages";
  for (const message of thread.messages) {
    const row = document.createElement("article");
    row.className = `gorp-ai-chat-message o-mail-Message o-${message.role}`;
    row.dataset.role = message.role;
    row.textContent = message.body;
    messages.append(row);
  }
  if (thread.messages.length === 0) {
    const empty = document.createElement("p");
    empty.className = "text-muted";
    empty.textContent = "Ask a question or use a prompt.";
    messages.append(empty);
  }

  const composer = document.createElement("form");
  composer.className = "gorp-ai-chat-composer o-mail-Composer";
  const input = document.createElement("textarea");
  input.className = "o-mail-Composer-input";
  input.placeholder = "Ask AI";
  const send = document.createElement("button");
  send.type = "submit";
  send.className = "btn btn-primary";
  send.textContent = thread.status === "loading" ? "Working" : "Send";
  send.disabled = thread.status === "loading";
  composer.addEventListener("submit", (event) => {
    event.preventDefault();
    const body = input.value.trim();
    input.value = "";
    void thread.post?.(body);
  });
  composer.append(input, send);

  container.replaceChildren(header, promptBar, messages, composer);
  if (options.focus) input.focus?.();
  env.bus.trigger("AI_CHAT_OPENED", { channelId: thread.id });
}

async function postAIUserMessage(channelID: number, body: string): Promise<Record<string, unknown>> {
  const payload = await fetchJSON<unknown>("/mail/message/post", {
    thread_model: "discuss.channel",
    thread_id: channelID,
    post_data: { body, message_type: "comment", subtype_xmlid: "mail.mt_comment" },
    context: {}
  });
  if (isRecord(payload)) return payload;
  return { id: 0 };
}

function postedMessageID(payload: Record<string, unknown>): number {
  const direct = numericUserID(payload.id) || numericUserID(payload.message_id);
  if (direct) return direct;
  const store = isRecord(payload.store_data) ? payload.store_data : undefined;
  const rows = store && Array.isArray(store["mail.message"]) ? store["mail.message"] : [];
  for (const row of rows) {
    if (!isRecord(row)) continue;
    const id = numericUserID(row.id);
    if (id) return id;
  }
  return 0;
}

function readAIPromptButtons(channelID: number): string[] {
  const raw = globalThis.localStorage?.getItem?.(createAIPromptButtonStorageKey(channelID));
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === "string" && item.trim().length > 0) : [];
  } catch {
    return [];
  }
}

function currentAIViewInfo(): Record<string, unknown> {
  const params = parseRouteState(globalThis.location?.hash ?? "");
  return {
    action_id: params.action,
    model: params.model,
    view_type: params.view_type,
    active_id: params.id ?? params.active_id
  };
}

function aiSessionIdentifier(): string {
  const uid = document.documentElement.dataset.aiSessionIdentifier;
  if (uid) return uid;
  const generated = `ai-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  document.documentElement.dataset.aiSessionIdentifier = generated;
  return generated;
}

interface GoERPBusEvent {
  id?: number;
  type?: string;
  name?: string;
  payload?: Record<string, unknown>;
}

interface AIBusPollingState {
  lastID: number;
  polling: boolean;
  stopped: boolean;
  timer?: ReturnType<typeof setTimeout>;
}

function startAIBusPolling(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  shell: RenderedWebClientShell,
  session: Record<string, unknown>
): void {
  const uid = numericUserID(session.uid);
  if (!uid) return;
  const state: AIBusPollingState = { lastID: 0, polling: false, stopped: false };
  const poll = () => {
    void pollAIBusOnce(env, menus, shell, state, uid).catch(() => undefined);
  };
  const manualPoll = () => poll();
  globalThis.addEventListener?.("goerp:ai-bus-poll", manualPoll);
  globalThis.addEventListener?.("beforeunload", () => {
    state.stopped = true;
    if (state.timer) clearTimeout(state.timer);
  });
  scheduleAIBusPoll(state, poll, 4000);
}

function scheduleAIBusPoll(state: AIBusPollingState, callback: () => void, delayMs: number): void {
  if (state.stopped) return;
  state.timer = setTimeout(callback, delayMs);
  (state.timer as unknown as { unref?: () => void }).unref?.();
}

function aiBusChannels(userID: number): string[] {
  const channels = [`user/${userID}`];
  for (const channelID of aiThreadStore.keys()) {
    channels.push(`discuss.channel/${channelID}`);
  }
  return [...new Set(channels)];
}

async function pollAIBusOnce(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  shell: RenderedWebClientShell,
  state: AIBusPollingState,
  userID: number
): Promise<void> {
  if (state.stopped || state.polling) return;
  state.polling = true;
  try {
    const payload = await fetchJSON<unknown>("/bus/poll", {
      channels: aiBusChannels(userID),
      last_id: state.lastID
    });
    for (const event of normalizeBusEvents(payload)) {
      if (typeof event.id === "number" && event.id > state.lastID) state.lastID = event.id;
      await handleAIBusEvent(env, menus, shell, event);
    }
  } finally {
    state.polling = false;
    scheduleAIBusPoll(state, () => {
      void pollAIBusOnce(env, menus, shell, state, userID).catch(() => undefined);
    }, 4000);
  }
}

function normalizeBusEvents(payload: unknown): GoERPBusEvent[] {
  const events = Array.isArray(payload) ? payload : isRecord(payload) && Array.isArray(payload.result) ? payload.result : [];
  return events.filter(isRecord).map((event) => ({
    id: typeof event.id === "number" ? event.id : numericUserID(event.id) || undefined,
    type: firstText(event.type),
    name: firstText(event.name),
    payload: isRecord(event.payload) ? event.payload : {}
  }));
}

async function handleAIBusEvent(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  shell: RenderedWebClientShell,
  event: GoERPBusEvent
): Promise<void> {
  const eventName = firstText(event.name, event.type);
  if (eventName === "mail.record/insert") {
    applyMailRecordInsertBusEvent(shell, event.payload ?? {});
    return;
  }
  if (!isAiNaturalLanguageEventName(eventName)) return;
  const payload = event.payload ?? {};
  if (isAiOpenMenuEventName(eventName)) {
    await applyAIOpenMenuEvent(env, menus, shell, eventName, payload);
    return;
  }
  await applyAIAdjustSearchEvent(env, menus, shell, payload);
}

function applyMailRecordInsertBusEvent(shell: RenderedWebClientShell, payload: Record<string, unknown>): void {
  applyMailRecordInsertToChatter(shell, payload);
  for (const row of mailMessageRowsFromPayload(payload)) {
    const channelID = numericUserID(row.res_id);
    if (!channelID || firstText(row.model) !== "discuss.channel") continue;
    const thread = aiThreadStore.get(channelID);
    if (!thread) continue;
    addAIThreadMessage(thread, {
      id: numericUserID(row.id) || undefined,
      role: aiMessageRole(row),
      body: aiMessageBody(row)
    });
    if (thread.container) renderAIChatPanel(thread, thread.env, { focus: false });
  }
}

function mailMessageRowsFromPayload(payload: Record<string, unknown>): Record<string, unknown>[] {
  const store = isRecord(payload.store_data) ? payload.store_data : payload;
  const rows = store["mail.message"];
  if (!Array.isArray(rows)) return [];
  return rows.filter(isRecord);
}

function addAIThreadMessage(thread: AIChatThreadState, message: AIChatMessageState): void {
  if (!message.body.trim()) return;
  if (message.id) {
    const existing = thread.messages.find((item) => item.id === message.id);
    if (existing) {
      existing.role = message.role;
      existing.body = message.body || existing.body;
      return;
    }
  }
  if (message.role === "user" && !message.id) {
    const existingUser = [...thread.messages].reverse().find((item) => item.role === "user" && item.body === message.body && !item.id);
    if (existingUser) return;
  }
  thread.messages.push(message);
}

function mergeAIThreadMessageID(thread: AIChatThreadState, localKey: string, id: number): void {
  const message = thread.messages.find((item) => item.localKey === localKey);
  if (message) message.id = id;
}

function aiMessageRole(row: Record<string, unknown>): AIChatMessageState["role"] {
  if (row.author_is_agent === true || row.is_ai_generated === true || row.ai_generated === true) return "assistant";
  const messageType = firstText(row.message_type);
  if (messageType === "notification") return "system";
  return "user";
}

function aiMessageBody(row: Record<string, unknown>): string {
  return cleanAIMessageBody(firstText(row.body, row.body_html, row.text, row.message));
}

function cleanAIMessageBody(value: string): string {
  return decodeAIHTMLEntities(value.replace(/<[^>]+>/g, " ")).replace(/\s+/g, " ").trim();
}

function decodeAIHTMLEntities(value: string): string {
  return value
    .replace(/&nbsp;/gi, " ")
    .replace(/&amp;/gi, "&")
    .replace(/&lt;/gi, "<")
    .replace(/&gt;/gi, ">")
    .replace(/&quot;/gi, "\"")
    .replace(/&#39;/gi, "'");
}

async function applyAIOpenMenuEvent(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  shell: RenderedWebClientShell,
  eventName: "AI_OPEN_MENU_LIST" | "AI_OPEN_MENU_KANBAN" | "AI_OPEN_MENU_PIVOT" | "AI_OPEN_MENU_GRAPH",
  payload: Record<string, unknown>
): Promise<void> {
  const menuID = numericUserID(payload.menuID);
  if (!menuID) return;
  const app = routeMenuApp(menus, menuID);
  if (!app) return;
  const actionID = menuActionID(app);
  if (typeof actionID !== "number") return;
  const invocation = createAiOpenMenuInvocation(eventName, { ...payload, menuID }, { id: menuID, actionID }, aiSessionIdentifier());
  if (!invocation) return;
  const outlet = findDescendantByClass(shell, "o_action_manager");
  if (!outlet) return;
  setShellActionView(shell);
  shell.setMenuContext(menuID);
  const actionHost = createActionHost(env, outlet, app);
  const loadedAction = await actionHost.loadAction(invocation.action.id as ActionRequest, { menu_id: menuID });
  await actionHost.doAction(actionWithAIProps(loadedAction, invocation.options.viewType, invocation.options.props.ai), {
    additionalContext: { menu_id: menuID },
    clearBreadcrumbs: true,
    stackPosition: "clear"
  });
}

async function applyAIAdjustSearchEvent(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  shell: RenderedWebClientShell,
  payload: Record<string, unknown>
): Promise<void> {
  const events = createAiAdjustSearchEvents(payload, aiSessionIdentifier());
  if (!events.length) return;
  const current = (env.services.action as ActionService | undefined)?.current?.action;
  if (!current || current.type !== "ir.actions.act_window") return;
  const outlet = findDescendantByClass(shell, "o_action_manager");
  if (!outlet) return;
  const route = parseRouteState(globalThis.location?.hash ?? "");
  const menuID = routeID(route.menu_id ?? current.menu_id);
  const app = menuID !== undefined ? routeMenuApp(menus, menuID) : undefined;
  setShellActionView(shell);
  if (menuID !== undefined) shell.setMenuContext(menuID);
  const actionHost = createActionHost(env, outlet, app);
  let nextAction = { ...current };
  for (const event of events) {
    env.bus.trigger(event.name, event.detail);
    globalThis.dispatchEvent?.(new CustomEvent(event.name, { detail: event.detail }));
    nextAction = actionWithAIAdjustSearch(nextAction, event.detail);
  }
  await actionHost.doAction(nextAction, {
    additionalContext: menuID !== undefined ? { menu_id: menuID } : {},
    replaceLastAction: true
  });
}

function actionWithAIProps(action: Record<string, unknown>, viewType: string, props: Record<string, unknown>): Record<string, unknown> {
  const next = actionWithRouteState(action, { view_type: viewType });
  const facets = aiSearchFacetsFromPayload(props);
  if (facets.length) next.__search_facets = facets;
  const query = aiSearchQuery(props.search);
  if (query) next.__search_query = query;
  next.__ai_model = aiModelProps(props);
  return next;
}

function actionWithAIAdjustSearch(action: Record<string, unknown>, detail: Record<string, unknown>): Record<string, unknown> {
  const switchViewType = firstText(detail.switchViewType);
  const next = switchViewType ? actionWithRouteState(action, { view_type: switchViewType }) : { ...action };
  let facets = Array.isArray(next.__search_facets)
    ? next.__search_facets.filter(isRecord).map(aiCloneSearchFacet)
    : [];
  facets = removeAIFacets(facets, stringList(detail.removeFacets));
  facets = toggleAIFacets(facets, aiFilterFacets(stringList(detail.toggleFilters)));
  facets = toggleAIFacets(facets, aiGroupByFacets(stringList(detail.toggleGroupBys)));
  facets = upsertAICustomDomainFacet(facets, detail.customDomain);
  if (facets.length) next.__search_facets = facets;
  else delete next.__search_facets;
  const query = aiSearchQuery(detail.applySearches);
  if (query) next.__search_query = query;
  next.__ai_model = aiModelProps(detail);
  return next;
}

function aiSearchFacetsFromPayload(payload: Record<string, unknown>): SearchFacet[] {
  return [
    ...aiFilterFacets(stringList(payload.selectedFilters)),
    ...aiGroupByFacets([...stringList(payload.selectedGroupBys), ...stringList(payload.groupBys), ...stringList(payload.rowGroupBys)]),
    ...aiCustomDomainFacet(payload.customDomain)
  ];
}

function aiFilterFacets(values: readonly string[]): SearchFacet[] {
  return values.map((value) => {
    const label = humanizeAIName(value);
    return {
      id: `ai-filter-${slugAIValue(value)}`,
      type: "filter",
      label,
      valueLabels: [label]
    };
  });
}

function aiGroupByFacets(values: readonly string[]): SearchFacet[] {
  return values.map((value) => {
    const descriptor = value.trim();
    const [field] = descriptor.split(":");
    const label = humanizeAIName(field || descriptor);
    return {
      id: `ai-group-by-${slugAIValue(descriptor)}`,
      type: "groupBy",
      label,
      valueLabels: [label],
      field: field || descriptor,
      groupBy: [descriptor]
    };
  });
}

function aiCustomDomainFacet(value: unknown): SearchFacet[] {
  if (!Array.isArray(value)) return [];
  return [{
    id: "ai-custom-domain",
    type: "filter",
    label: "AI Domain",
    valueLabels: ["AI Domain"],
    domain: [...value]
  }];
}

function aiSearchQuery(value: unknown): string {
  return stringList(value).join(" ").trim();
}

function aiModelProps(value: Record<string, unknown>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const key of ["measures", "measure", "mode", "order", "stacked", "cumulated", "sortedColumn", "colGroupBys"]) {
    if (value[key] !== undefined) out[key] = value[key];
  }
  return out;
}

function removeAIFacets(facets: SearchFacet[], removeValues: readonly string[]): SearchFacet[] {
  if (!removeValues.length) return facets;
  const remove = new Set(removeValues.flatMap((value) => [value, slugAIValue(value), `ai-filter-${slugAIValue(value)}`, `ai-group-by-${slugAIValue(value)}`]));
  return facets.filter((facet) => !remove.has(facet.id) && !remove.has(facet.label) && !(facet.groupBy ?? []).some((item) => remove.has(item)));
}

function toggleAIFacets(facets: SearchFacet[], toggles: readonly SearchFacet[]): SearchFacet[] {
  let out = facets;
  for (const facet of toggles) {
    const index = out.findIndex((item) => item.id === facet.id || item.label === facet.label || sameAIGroupBy(item, facet));
    out = index >= 0 ? out.filter((_item, itemIndex) => itemIndex !== index) : [...out, facet];
  }
  return out;
}

function upsertAICustomDomainFacet(facets: SearchFacet[], value: unknown): SearchFacet[] {
  const custom = aiCustomDomainFacet(value)[0];
  if (!custom) return facets;
  return [...facets.filter((facet) => facet.id !== custom.id), custom];
}

function sameAIGroupBy(left: SearchFacet, right: SearchFacet): boolean {
  if (!left.groupBy?.length || !right.groupBy?.length) return false;
  return left.groupBy.some((item) => right.groupBy?.includes(item));
}

function aiCloneSearchFacet(facet: Record<string, unknown>): SearchFacet {
  return {
    ...facet,
    id: firstText(facet.id) || `ai-facet-${Math.random().toString(36).slice(2)}`,
    type: firstText(facet.type) as SearchFacet["type"] || "filter",
    label: firstText(facet.label) || "AI",
    valueLabels: stringList(facet.valueLabels),
    domain: Array.isArray(facet.domain) ? [...facet.domain] : undefined,
    context: isRecord(facet.context) ? { ...facet.context } : undefined,
    groupBy: stringList(facet.groupBy)
  };
}

function stringList(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.map((item) => String(item ?? "").trim()).filter(Boolean);
}

function slugAIValue(value: string): string {
  return value.toLowerCase().replace(/[^a-z0-9_:.]+/g, "-").replace(/^-+|-+$/g, "") || "value";
}

function humanizeAIName(value: string): string {
  return value.replace(/[:._-]+/g, " ").replace(/\s+/g, " ").trim().replace(/\b\w/g, (char) => char.toUpperCase()) || "AI";
}

async function restoreActionFromHash(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  shell: RenderedWebClientShell
): Promise<boolean> {
  const route = parseRouteState(globalThis.location?.hash ?? "");
  const actionID = routeActionID(route);
  if (actionID === undefined) {
    if (routeModel(route)) return restoreModelRouteFromHash(env, route, shell);
    const menuID = routeID(route.menu_id);
    if (menuID === undefined) return false;
    await shell.openMenuApp(menuID);
    return true;
  }
  const outlet = findDescendantByClass(shell, "o_action_manager");
  if (!outlet) return false;
  setShellActionView(shell);
  const routeMenuID = routeID(route.menu_id);
  if (routeMenuID !== undefined) shell.setMenuContext(routeMenuID);
  const app = routeMenuApp(menus, route.menu_id);
  if (route.model === "ir.module.module") {
    await renderAppsCatalog(env, outlet, cleanAppName(app?.name ?? "Apps") || "Apps");
    return true;
  }
  const context = routeActionContext(route);
  const actionHost = createActionHost(env, outlet, app);
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(app?.name ?? "Action"));
  try {
    const action = await actionHost.loadAction(actionID, context);
    const routeAction = actionWithRouteState(action, route);
    const nextAction = routeAction.res_model === "ir.module.module"
      ? normalizeAppsWindowAction(routeAction, routeAppsCatalogApp(app, route))
      : routeAction;
    await actionHost.doAction(nextAction, {
      additionalContext: context,
      stackPosition: "clear"
    });
    return true;
  } catch (error) {
    renderActionError(outlet, error);
    return false;
  }
}

async function restoreModelRouteFromHash(
  env: ReturnType<typeof makeEnv>,
  route: WebClientRouteState,
  shell: RenderedWebClientShell
): Promise<boolean> {
  const model = routeModel(route);
  if (!model) return false;
  const outlet = findDescendantByClass(shell, "o_action_manager");
  if (!outlet) return false;
  setShellActionView(shell);
  const routeMenuID = routeID(route.menu_id);
  if (routeMenuID !== undefined) shell.setMenuContext(routeMenuID);
  if (model === "ir.module.module") {
    await renderAppsCatalog(env, outlet, routeModelTitle(model));
    return true;
  }
  const context = {
    ...routeActionContext(route),
    active_model: model
  };
  const title = routeModelTitle(model);
  const actionHost = createActionHost(env, outlet);
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(title));
  try {
    await actionHost.doAction(actionWithRouteState(routeModelWindowAction(model, route), route), {
      additionalContext: context,
      stackPosition: "clear"
    });
    return true;
  } catch (error) {
    renderActionError(outlet, error);
    return false;
  }
}

function routeAppsCatalogApp(app: HomeMenuApp | undefined, route: WebClientRouteState): HomeMenuApp {
  if (app) return app;
  const id = routeID(route.menu_id) ?? "apps";
  return {
    id,
    key: "apps",
    name: "Apps",
    initials: "A",
    iconToken: "apps",
    sequence: 0,
    searchText: "apps",
    menu: { id, name: "Apps" } as HomeMenuEntry
  };
}

function routeActionID(route: WebClientRouteState): number | string | undefined {
  return routeID(route.action);
}

function routeModel(route: WebClientRouteState): string {
  return typeof route.model === "string" && route.model.trim() ? route.model.trim() : "";
}

function routeModelWindowAction(model: string, route: WebClientRouteState): Record<string, unknown> {
  const firstView = typeof route.view_type === "string" && route.view_type.trim() ? route.view_type.trim() : "list";
  const viewMode = firstView === "form" ? "form,list" : `${firstView},form`;
  const action: Record<string, unknown> = {
    type: "ir.actions.act_window",
    name: routeModelTitle(model),
    res_model: model,
    view_mode: viewMode,
    views: actionViewsWithFirstType({ view_mode: viewMode }, firstView)
  };
  return action;
}

function routeModelTitle(model: string): string {
  const labels: Record<string, string> = {
    "res.users": "Users",
    "res.groups": "Groups",
    "res.company": "Companies",
    "ir.model": "Models",
    "ir.model.fields": "Fields",
    "ir.model.access": "Access Rights",
    "ir.rule": "Record Rules",
    "ir.ui.view": "Views",
    "ir.actions.actions": "Actions",
    "ir.actions.act_window": "Window Actions",
    "ir.actions.client": "Client Actions",
    "ir.actions.server": "Server Actions",
    "ir.cron": "Scheduled Actions",
    "base.automation": "Automation Rules",
    "mail.template": "Email Templates",
    "ir.mail_server": "Outgoing Mail Servers",
    "fetchmail.server": "Incoming Mail Servers",
    "ai.agent": "AI Agents",
    "ai.audit.log": "AI Audit Logs",
    "ir.module.module": "Apps"
  };
  if (labels[model]) return labels[model];
  return model
    .split(".")
    .filter(Boolean)
    .map((part) => part.replace(/_/g, " "))
    .join(" ")
    .replace(/\b\w/g, (char) => char.toUpperCase()) || "Records";
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

function findDescendantByDataset(root: HTMLElement, key: string, value: string): HTMLElement | null {
  if (root.dataset?.[key] === value) return root;
  for (const child of Array.from(root.children)) {
    const found = findDescendantByDataset(child as HTMLElement, key, value);
    if (found) return found;
  }
  return null;
}

async function rpcTransport(request: RPCRequest): Promise<unknown> {
  return fetchJSON(request.route, request.params);
}

async function loadSystraySession(session: Record<string, unknown>): Promise<Record<string, unknown>> {
  if (!numericUserID(session.uid)) return session;
  try {
    const payload = await fetchJSON<Record<string, unknown>>("/mail/data", {
      fetch_params: ["init_messaging", ["systray_get_activities", {}]],
      context: sessionUserContext(session)
    });
    return {
      ...session,
      Store: {
        ...(isRecord(session.Store) ? session.Store : {}),
        ...(isRecord(payload.Store) ? payload.Store : {})
      }
    };
  } catch {
    return session;
  }
}

function sessionUserContext(session: Record<string, unknown>): Record<string, unknown> {
  return isRecord(session.user_context) ? { ...session.user_context } : {};
}

async function openMenuApp(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  app: HomeMenuApp,
  outlet: HTMLElement
): Promise<void> {
  const actionID = menuActionID(app);
  if (isAppsCatalogLikeApp(app)) {
    await openAppsCatalogWindowAction(env, outlet, app, actionID);
    return;
  }
  if (isSettingsLikeApp(app)) {
    updateBrowserRoute({
      ...(actionID !== undefined ? { action: actionID } : {}),
      model: "res.config.settings",
      view_type: "form",
      menu_id: app.id
    });
    await renderGeneralSettingsApp(env, menus, app, outlet, actionID);
    return;
  }
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

async function openAppsCatalogWindowAction(
  env: ReturnType<typeof makeEnv>,
  outlet: HTMLElement,
  app: HomeMenuApp,
  actionID?: number | string,
  query = ""
): Promise<void> {
  updateBrowserRoute({
    ...(actionID !== undefined ? { action: actionID } : {}),
    model: "ir.module.module",
    view_type: "kanban",
    menu_id: app.id
  });
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(cleanAppName(app.name) || "Apps"));
  const actionHost = createActionHost(env, outlet, app);
  const context = appsCatalogActionContext(app, query);
  const fallbackAction = {
    type: "ir.actions.act_window",
    name: "Apps",
    res_model: "ir.module.module",
    view_mode: "kanban,list,form",
    views: [[false, "kanban"], [false, "list"], [false, "form"]]
  };
  const loadedAction = actionID !== undefined
    ? await actionHost.loadAction(actionID, context)
    : fallbackAction;
  await actionHost.doAction(normalizeAppsWindowAction(loadedAction, app, query), {
    additionalContext: context,
    stackPosition: "clear"
  });
}

function appsCatalogActionContext(app: HomeMenuApp, query = ""): Record<string, unknown> {
  const context: Record<string, unknown> = {
    menu_id: app.id,
    search_default_app: 1
  };
  const normalizedQuery = query.trim();
  if (normalizedQuery) context.search_default_name = normalizedQuery;
  return context;
}

function isAppsCatalogLikeApp(app: HomeMenuApp): boolean {
  if (isAppsCatalogApp(app)) return true;
  const name = cleanAppName(app.name).toLowerCase();
  return name === "apps";
}

function normalizeAppsWindowAction(action: Record<string, unknown>, app: HomeMenuApp, query = ""): Record<string, unknown> {
  if (action.type !== "ir.actions.act_window" || action.res_model !== "ir.module.module") {
    return { ...action, menu_id: app.id };
  }
  return {
    ...action,
    menu_id: app.id,
    view_mode: "kanban,list,form",
    views: appsWindowActionViews(action),
    view_type: "kanban",
    context: {
      ...(isRecord(action.context) ? action.context : {}),
      ...appsCatalogActionContext(app, query)
    }
  };
}

function appsWindowActionViews(action: Record<string, unknown>): [number | false, string][] {
  const ids = new Map<string, number | false>();
  if (Array.isArray(action.views)) {
    for (const rawView of action.views) {
      if (!Array.isArray(rawView) || rawView.length < 2) continue;
      const type = typeof rawView[1] === "string" ? rawView[1].trim() : "";
      if (!type || ids.has(type)) continue;
      ids.set(type, typeof rawView[0] === "number" && rawView[0] > 0 ? rawView[0] : false);
    }
  }
  return ["kanban", "list", "form"].map((type) => [ids.get(type) ?? false, type]);
}

function isSettingsLikeApp(app: HomeMenuApp): boolean {
  return cleanAppName(app.name).toLowerCase() === "settings";
}

interface SettingsNavigationTarget {
  id: string;
  names: readonly string[];
  model?: string;
  query?: string;
}

function renderGeneralSettingsApp(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  app: HomeMenuApp,
  outlet: HTMLElement,
  actionID: string | number | undefined
): void {
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading("Settings"));
  const view = renderWindowAction(generalSettingsWindowAction(actionID, app.id), {
    services: env.services as unknown as WebClientServices
  });
  view.className = `${view.className} o_settings_view`;
  attachGeneralSettingsNavigation(view, env, menus, app, outlet);
  outlet.dataset.tsActionStatus = "ready";
  outlet.replaceChildren(view);
}

function generalSettingsWindowAction(actionID: string | number | undefined, menuID: string | number): WindowActionResult {
  return {
    type: "ir.actions.act_window",
    action: {
      ...(actionID !== undefined ? { id: actionID } : {}),
      name: "Settings",
      res_model: "res.config.settings",
      type: "ir.actions.act_window",
      view_mode: "form",
      views: [[false, "form"]],
      menu_id: menuID,
      context: { active_app: "general_settings" }
    },
    activeView: "form",
    resModel: "res.config.settings",
    viewDescriptions: {
      fields: generalSettingsFields(),
      relatedModels: {},
      views: {
        form: {
          id: false,
          arch: generalSettingsArch()
        }
      }
    },
    records: [generalSettingsValues()],
    length: 1,
    offset: 0,
    countLimited: false
  };
}

function generalSettingsArch(): string {
  return `<form>
    <app name="general_settings" string="General Settings">
      <block title="Users">
        <setting id="invite_users" string="Invite New Users"><field name="invite_email" placeholder="Enter an email"/></setting>
        <setting id="users" string="1 Active User"><field name="active_user_count" readonly="1"/></setting>
      </block>
      <block title="Languages">
        <setting id="languages" string="1 Language"><field name="language_count" readonly="1"/></setting>
      </block>
      <block title="Companies">
        <setting id="company_records" string="My Company"><field name="company_count" readonly="1"/></setting>
        <setting id="document_layout" string="Document Layout" help="Choose the layout of your documents"><field name="document_layout_state" readonly="1"/></setting>
        <setting id="companies" string="1 Company"><field name="company_count" readonly="1"/></setting>
      </block>
      <block title="Contacts">
        <setting id="groups" string="Groups" help="Manage access groups and inherited permissions."><field name="security_group_count" readonly="1"/></setting>
      </block>
      <block title="Permissions">
        <setting id="default_access_rights" string="Default Access Rights" help="Set custom access rights for new users"><field name="default_access_right_count" readonly="1"/></setting>
        <setting id="api_keys" string="API Keys" help="API Keys allow your users to access Odoo with external tools when multi-factor authentication is enabled."><field name="api_key_count" readonly="1"/></setting>
        <setting id="import_export" string="Import &amp; Export" help="Allow users to import data from CSV/XLS/XLSX/ODS files"><field name="allow_import_export"/></setting>
      </block>
      <block title="Integrations">
        <setting id="mail_plugin" string="Mail Plugin" help="Integrate with mail client plugins"><field name="mail_plugin_state" readonly="1"/></setting>
        <setting id="oauth_authentication" string="OAuth Authentication" help="Use external accounts to log in (Google, Facebook, etc.)"><field name="oauth_state" readonly="1"/></setting>
        <setting id="ldap_authentication" string="LDAP Authentication" help="Use LDAP credentials to log in"><field name="ldap_state" readonly="1"/></setting>
      </block>
      <block title="Developer Tools">
        <setting id="developer_mode_assets" string="Activate the developer mode (with assets)"><field name="developer_assets_state" readonly="1"/></setting>
        <setting id="developer_mode_tests" string="Activate the developer mode (with tests assets)"><field name="developer_tests_state" readonly="1"/></setting>
        <setting id="load_demo_data" string="Load demo data"><field name="demo_data_state" readonly="1"/></setting>
      </block>
      <block title="About">
        <setting id="about" string="Odoo 19.0+e (Enterprise Edition)" help="Database expiration: July 28, 2026"><field name="about_state" readonly="1"/></setting>
      </block>
      <block title="Technical">
        <setting id="server_actions" string="Server Actions" help="Automate backend actions and contextual operations."><field name="server_action_count" readonly="1"/></setting>
        <setting id="scheduled_actions" string="Scheduled Actions" help="Review automated jobs and execution schedules."><field name="scheduled_action_count" readonly="1"/></setting>
        <setting id="automation_rules" string="Automation Rules" help="Configure automated record rules and triggers."><field name="automation_rule_count" readonly="1"/></setting>
        <setting id="views" string="Views" help="Inspect backend views and inherited layouts."><field name="view_count" readonly="1"/></setting>
        <setting id="models" string="Models" help="Inspect model metadata and technical models."><field name="model_count" readonly="1"/></setting>
        <setting id="fields" string="Fields" help="Inspect model fields and technical field definitions."><field name="field_count" readonly="1"/></setting>
        <setting id="access_rights" string="Access Rights" help="Manage model permissions."><field name="access_right_count" readonly="1"/></setting>
        <setting id="record_rules" string="Record Rules" help="Manage record-level security rules."><field name="record_rule_count" readonly="1"/></setting>
        <setting id="mail_servers" string="Outgoing Mail Servers" help="Configure outgoing email delivery servers."><field name="mail_server_count" readonly="1"/></setting>
        <setting id="fetchmail_servers" string="Incoming Mail Servers" help="Configure incoming email fetch servers."><field name="fetchmail_server_count" readonly="1"/></setting>
        <setting id="email_templates" string="Email Templates" help="Maintain mail templates."><field name="email_template_count" readonly="1"/></setting>
        <setting id="apps" string="Apps" help="Install and manage apps."><field name="installed_module_count" readonly="1"/></setting>
        <setting id="ai" string="AI Apps" help="Open AI app modules."><field name="ai_module_count" readonly="1"/></setting>
      </block>
    </app>
  </form>`;
}

function generalSettingsFields(): Record<string, unknown> {
  return {
    invite_email: { type: "char", string: "Email" },
    active_user_count: readonlyIntegerField("Active Users"),
    language_count: readonlyIntegerField("Languages"),
    security_group_count: readonlyIntegerField("Groups"),
    company_count: readonlyIntegerField("Companies"),
    document_layout_state: { type: "char", string: "Layout", readonly: true },
    default_access_right_count: readonlyIntegerField("Default Access Rights"),
    api_key_count: readonlyIntegerField("API Keys"),
    allow_import_export: { type: "boolean", string: "Import & Export" },
    mail_plugin_state: { type: "char", string: "Mail Plugin", readonly: true },
    oauth_state: { type: "char", string: "OAuth Authentication", readonly: true },
    ldap_state: { type: "char", string: "LDAP Authentication", readonly: true },
    developer_assets_state: { type: "char", string: "Developer Mode Assets", readonly: true },
    developer_tests_state: { type: "char", string: "Developer Mode Tests", readonly: true },
    demo_data_state: { type: "char", string: "Demo Data", readonly: true },
    about_state: { type: "char", string: "About", readonly: true },
    server_action_count: readonlyIntegerField("Server Actions"),
    scheduled_action_count: readonlyIntegerField("Scheduled Actions"),
    automation_rule_count: readonlyIntegerField("Automated Actions"),
    view_count: readonlyIntegerField("Views"),
    model_count: readonlyIntegerField("Models"),
    field_count: readonlyIntegerField("Fields"),
    access_right_count: readonlyIntegerField("Access Rights"),
    record_rule_count: readonlyIntegerField("Record Rules"),
    mail_server_count: readonlyIntegerField("Outgoing Mail Servers"),
    fetchmail_server_count: readonlyIntegerField("Incoming Mail Servers"),
    email_template_count: readonlyIntegerField("Email Templates"),
    installed_module_count: readonlyIntegerField("Installed Apps"),
    ai_module_count: readonlyIntegerField("AI Modules")
  };
}

function readonlyIntegerField(label: string): Record<string, unknown> {
  return { type: "integer", string: label, readonly: true };
}

function generalSettingsValues(): Record<string, unknown> {
  return {
    id: 1,
    invite_email: "",
    active_user_count: 1,
    language_count: 1,
    security_group_count: 0,
    company_count: 1,
    document_layout_state: "Layout",
    default_access_right_count: 0,
    api_key_count: 0,
    allow_import_export: true,
    mail_plugin_state: "Configure",
    oauth_state: "Configure",
    ldap_state: "Configure",
    developer_assets_state: "Activate",
    developer_tests_state: "Activate",
    demo_data_state: "Load",
    about_state: "Odoo Enterprise Edition License V1.0",
    server_action_count: 0,
    scheduled_action_count: 0,
    automation_rule_count: 0,
    view_count: 0,
    model_count: 0,
    field_count: 0,
    access_right_count: 0,
    record_rule_count: 0,
    mail_server_count: 0,
    fetchmail_server_count: 0,
    email_template_count: 0,
    installed_module_count: 0,
    ai_module_count: 0
  };
}

function attachGeneralSettingsNavigation(
  root: HTMLElement,
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  settingsApp: HomeMenuApp,
  outlet: HTMLElement
): void {
  attachInviteUsersAction(root);
  for (const target of settingsNavigationTargets()) {
    const box = findDescendantByDataset(root, "settingId", target.id);
    if (!box) continue;
    const pane = findDescendantByClass(box, "o_setting_right_pane") ?? box;
    box.dataset.hasSettingsAction = "true";
    const actions = document.createElement("div");
    actions.className = "o_setting_buttons";
    actions.dataset.settingsActions = target.id;
    const button = document.createElement("button");
    button.type = "button";
    button.className = "o_setting_action o_setting_link";
    button.dataset.settingsTarget = target.id;
    if (target.model) button.dataset.settingsTargetModel = target.model;
    button.setAttribute("style", "color:#8ddad8 !important;background:transparent;border:0;padding:0;text-align:left;font-weight:700;");
    button.textContent = settingsTargetButtonLabel(target);
    button.addEventListener("click", () => {
      void openSettingsNavigationTarget(env, menus, settingsApp, outlet, target).catch((error) => {
        renderActionError(outlet, error);
      });
    });
    actions.append(button);
    pane.append(actions);
  }
}

function attachInviteUsersAction(root: HTMLElement): void {
  const box = findDescendantByDataset(root, "settingId", "invite_users");
  if (!box) return;
  const pane = findDescendantByClass(box, "o_setting_right_pane") ?? box;
  const fields = findDescendantByClass(pane, "o_setting_fields");
  const actions = document.createElement("div");
  actions.className = "o_setting_buttons";
  actions.dataset.settingsActions = "invite_users";
  const button = document.createElement("button");
  button.type = "button";
  button.className = "btn btn-primary o_setting_action o_setting_invite";
  button.dataset.settingsAction = "invite-users";
  button.setAttribute("style", "color:#fff !important;");
  button.textContent = "Invite";
  actions.append(button);
  if (fields) fields.append(actions);
  else pane.append(actions);
}

function settingsNavigationTargets(): SettingsNavigationTarget[] {
  return [
    { id: "users", names: ["Users"], model: "res.users" },
    { id: "groups", names: ["Groups"], model: "res.groups" },
    { id: "companies", names: ["Companies"], model: "res.company" },
    { id: "company_records", names: ["Companies"], model: "res.company" },
    { id: "languages", names: ["Languages"], model: "res.lang" },
    { id: "default_access_rights", names: ["Access Rights"], model: "ir.model.access" },
    { id: "api_keys", names: ["API Keys"], model: "res.users.apikeys" },
    { id: "server_actions", names: ["Server Actions"], model: "ir.actions.server" },
    { id: "scheduled_actions", names: ["Scheduled Actions"], model: "ir.cron" },
    { id: "automation_rules", names: ["Automation Rules", "Automated Actions"], model: "base.automation" },
    { id: "views", names: ["Views"], model: "ir.ui.view" },
    { id: "models", names: ["Models"], model: "ir.model" },
    { id: "fields", names: ["Fields"], model: "ir.model.fields" },
    { id: "access_rights", names: ["Access Rights"], model: "ir.model.access" },
    { id: "record_rules", names: ["Record Rules"], model: "ir.rule" },
    { id: "mail_servers", names: ["Outgoing Mail Servers", "Mail Servers"], model: "ir.mail_server" },
    { id: "fetchmail_servers", names: ["Incoming Mail Servers"], model: "fetchmail.server" },
    { id: "email_templates", names: ["Email Templates"], model: "mail.template" },
    { id: "apps", names: ["Apps"], model: "ir.module.module" },
    { id: "ai", names: ["Apps"], model: "ir.module.module", query: "ai" }
  ];
}

function settingsTargetButtonLabel(target: SettingsNavigationTarget): string {
  const labels: Record<string, string> = {
    users: "Manage Users",
    groups: "Manage Groups",
    companies: "Manage Companies",
    default_access_rights: "Default Access Rights",
    api_keys: "Manage API Keys",
    users_access: "Manage Users",
    groups_access: "Manage Groups",
    company_records: "Manage Companies",
    server_actions: "Server Actions",
    scheduled_actions: "Scheduled Actions",
    automation_rules: "Automation Rules",
    views: "Views",
    models: "Models",
    fields: "Fields",
    access_rights: "Access Rights",
    record_rules: "Record Rules",
    mail_servers: "Outgoing Mail Servers",
    fetchmail_servers: "Incoming Mail Servers",
    email_templates: "Email Templates",
    apps: "Apps",
    ai: "AI Apps"
  };
  return labels[target.id] || target.names[0] || target.id;
}

async function openSettingsNavigationTarget(
  env: ReturnType<typeof makeEnv>,
  menus: HomeMenuPayload,
  settingsApp: HomeMenuApp,
  outlet: HTMLElement,
  target: SettingsNavigationTarget
): Promise<void> {
  if (target.model === "ir.module.module") {
    const appsMenu = findSettingsTargetMenuApp(menus, settingsApp, target);
    const actionID = menuActionID(appsMenu ?? settingsApp);
    await openAppsCatalogWindowAction(env, outlet, appsMenu ?? settingsApp, actionID, target.query ?? "");
    return;
  }
  const targetApp = findSettingsTargetMenuApp(menus, settingsApp, target);
  if (targetApp && menuActionID(targetApp) !== undefined) {
    await openMenuApp(env, menus, targetApp, outlet);
    return;
  }
  if (!target.model) throw new Error(`${target.names[0]} menu is not available`);
  await openFallbackSettingsTarget(env, outlet, settingsApp.id, target);
}

function findSettingsTargetMenuApp(
  menus: HomeMenuPayload,
  settingsApp: HomeMenuApp,
  target: SettingsNavigationTarget
): HomeMenuApp | undefined {
  const names = target.names.map((name) => cleanAppName(name).toLowerCase());
  const apps = normalizeHomeMenuApps(menus, { includeDescendantActions: true });
  const candidates = apps.filter((candidate) => {
    const name = cleanAppName(candidate.name).toLowerCase();
    if (!names.includes(name)) return false;
    if (target.model === "ir.module.module") return true;
    return candidate.rootId === undefined || String(candidate.rootId) === String(settingsApp.id);
  });
  return candidates.find((candidate) => menuActionID(candidate) !== undefined) ?? candidates[0];
}

async function openFallbackSettingsTarget(
  env: ReturnType<typeof makeEnv>,
  outlet: HTMLElement,
  menuID: number | string,
  target: SettingsNavigationTarget
): Promise<void> {
  if (!target.model) return;
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(target.names[0]));
  const actionHost = createActionHost(env, outlet);
  await actionHost.doAction({
    name: target.names[0],
    res_model: target.model,
    type: "ir.actions.act_window",
    view_mode: "list,form",
    views: [[false, "list"], [false, "form"]],
    menu_id: menuID
  }, {
    additionalContext: { menu_id: menuID },
    stackPosition: "clear"
  });
}

async function handleSystrayAction(
  env: ReturnType<typeof makeEnv>,
  action: NavbarSystrayAction,
  outlet: HTMLElement,
  options: SystrayActionOptions = {}
): Promise<void> {
  switch (action.type) {
    case "logout":
      await fetchJSON("/web/session/logout", { logout: true });
      globalThis.location.href = "/web/login";
      return;
    case "open-mailbox":
      await openMailboxAction(env, action, outlet);
      return;
    case "open-activities":
      await openActivitiesAction(env, action, outlet, options);
      return;
    case "open-debug-tools":
    case "view-metadata":
    case "view-access-rights":
    case "view-record-rules":
      renderSystrayAction(outlet, "Developer Tools", [
        "Metadata",
        "Access Rights",
        "Record Rules",
        "Become Superuser"
      ]);
      return;
    case "become-superuser":
      globalThis.location.href = `/web/become/debug?redirect=${encodeURIComponent("/web?debug=1")}`;
      return;
    case "leave-debug-mode":
      globalThis.location.href = "/web";
      return;
    case "open-preferences":
    case "open-profile":
      await openUserPreferencesAction(env, outlet);
      return;
    case "open-help":
      renderSystrayAction(outlet, "Help", ["Documentation", "Support"]);
      return;
    case "open-shortcuts":
      renderSystrayAction(outlet, "Shortcuts", ["Alt+Shift+A", "Search", "Create"]);
      return;
    case "open-odoo-account":
      renderSystrayAction(outlet, "My Odoo.com Account", ["Account", "Subscription"]);
      return;
    case "new-message":
      renderSystrayAction(outlet, "New Message", ["Compose", "Recipients", "Message"]);
      return;
    case "switch-company":
      if (action.companyId !== undefined && action.companyId !== null) {
        await switchCompanyAction(action);
      } else {
        renderSystrayAction(outlet, "Companies", ["Switch company", "Confirm", "Reset"]);
      }
      return;
    default:
      renderSystrayAction(outlet, "Systray", [action.type]);
  }
}

async function openUserPreferencesAction(env: ReturnType<typeof makeEnv>, outlet: HTMLElement): Promise<void> {
  const session = (env.services.session as SessionService | undefined)?.info ?? {};
  const uid = numericUserID(session.uid);
  const orm = env.services.orm as WebClientServices["orm"];
  let action: Record<string, unknown> = {};
  try {
    action = await orm.call<Record<string, unknown>>("res.users", "action_get");
  } catch {
    action = {};
  }
  const actionHost = createActionHost(env, outlet);
  await actionHost.doAction({
    type: "ir.actions.act_window",
    name: "Change My Preferences",
    res_model: "res.users",
    view_mode: "form",
    views: [[false, "form"]],
    ...action,
    context: {
      ...(isRecord(action.context) ? action.context : {}),
      gorp_preferences_dialog: true
    },
    target: "new",
    ...(uid > 0 ? { res_id: uid } : {})
  }, {
    additionalContext: {
      active_model: "res.users",
      gorp_preferences_dialog: true,
      ...(uid > 0 ? { active_id: uid, active_ids: [uid] } : {})
    }
  });
}

interface SystrayActionOptions {
  onActivityStore?: (store: Record<string, unknown>) => void;
}

async function switchCompanyAction(action: NavbarSystrayAction): Promise<void> {
  const payload: Record<string, unknown> = { company_id: action.companyId };
  if (Array.isArray(action.companyIds) && action.companyIds.length > 0) {
    payload.company_ids = action.companyIds;
  }
  await fetchJSON("/web/session/switch_company", payload);
  if (typeof globalThis.location.reload === "function") {
    globalThis.location.reload();
  } else {
    globalThis.location.href = "/web";
  }
}

async function openMailboxAction(
  env: ReturnType<typeof makeEnv>,
  action: NavbarSystrayAction,
  outlet: HTMLElement
): Promise<void> {
  const mailbox = firstText(action.mailbox, "inbox") || "inbox";
  let rows: string[] = [];
  if (mailbox === "starred") {
    const mail = env.services.mail as WebClientServices["mail"];
    const payload = await mail.starredMessages<Record<string, unknown>>({ limit: 20 });
    rows = [`Starred messages`, `${arrayLength(payload.messages)} messages`];
  } else {
    rows = ["Inbox", "Unread messages are tracked from mail notifications"];
  }
  renderSystrayAction(outlet, mailbox === "starred" ? "Starred" : "Inbox", rows);
}

async function openActivitiesAction(
  env: ReturnType<typeof makeEnv>,
  action: NavbarSystrayAction,
  outlet: HTMLElement,
  options: SystrayActionOptions = {}
): Promise<void> {
  const session = (env.services.session as SessionService | undefined)?.info ?? {};
  const payload = await fetchJSON<Record<string, unknown>>("/mail/data", {
    fetch_params: [["systray_get_activities", {}]],
    context: sessionUserContext(session)
  });
  const store = isRecord(payload.Store) ? payload.Store : {};
  options.onActivityStore?.(store);
  const groups = Array.isArray(store.activityGroups) ? store.activityGroups : [];
  const selectedModel = firstText(action.model);
  const groupRows = groups.filter(isRecord).filter((group) => !selectedModel || firstText(group.model) === selectedModel);
  const activityIDs = uniqueNumberList(groupRows.flatMap((group) => numberList(group.activity_ids)));
  if (!activityIDs.length) {
    const rows = groupRows.map((group) => `${firstText(group.name, group.model, "Activities")}: ${numericUserID(group.total_count)} total`);
    renderSystrayAction(outlet, firstText(action.model, "Activities") || "Activities", rows.length ? rows : ["No activities"]);
    return;
  }
  const activityStore = await fetchJSON<Record<string, unknown>>("/web/dataset/call_kw/mail.activity/activity_format", {
    args: [activityIDs],
    kwargs: {}
  });
  const activities = activityRows(activityStore);
  renderActivityAction(outlet, firstText(action.model, groupRows[0]?.name, "Activities") || "Activities", activities, async (method, activityID, feedback) => {
    await activityAction(method, activityID, feedback);
    await openActivitiesAction(env, action, outlet, options);
  }, async (activity) => {
    await openActivityRecord(env, outlet, activity);
  });
}

function updateActivitySystray(
  shell: HTMLElement,
  store: Record<string, unknown>,
  onAction: (action: NavbarSystrayAction) => void
): void {
  const activityButton = findDescendantByClass(shell, "o_activity_menu");
  const counter = activityButton ? findDescendantByClass(activityButton, "o-systray-counter") : null;
  const count = numericUserID(store.activityCounter);
  if (counter) {
    counter.textContent = String(count);
    counter.hidden = count <= 0;
  }
  const dropdown = findDescendantByDataset(shell, "systrayDropdown", "activities");
  if (!dropdown) return;
  const groups = Array.isArray(store.activityGroups) ? store.activityGroups.filter(isRecord) : [];
  const entries = groups.length ? groups : [{ name: "No activities", total_count: 0 }];
  dropdown.replaceChildren(...entries.map((group) => renderActivitySystrayMenuEntry(group, onAction)));
}

function renderActivitySystrayMenuEntry(
  group: Record<string, unknown>,
  onAction: (action: NavbarSystrayAction) => void
): HTMLElement {
  const model = firstText(group.model);
  const labelText = firstText(group.name, model, "Activities") || "Activities";
  const total = numericUserID(group.total_count);
  const overdue = numericUserID(group.overdue_count);
  const today = numericUserID(group.today_count);
  const planned = numericUserID(group.planned_count);
  const button = document.createElement("button");
  button.type = "button";
  button.className = "dropdown-item";
  button.dataset.systrayItem = labelText;
  button.dataset.systrayAction = "open-activities";
  button.setAttribute("role", "menuitem");
  button.addEventListener("click", () => onAction({
    type: "open-activities",
    model,
    domain: group.domain,
    viewType: firstText(group.view_type, "list")
  }));
  const label = document.createElement("span");
  label.className = "o_systray_menu_label";
  label.textContent = labelText;
  const badge = document.createElement("span");
  badge.className = "o_systray_menu_badge";
  badge.textContent = String(total);
  const description = document.createElement("span");
  description.className = "o_systray_menu_description";
  description.textContent = `Late ${overdue} Today ${today} Future ${planned}`;
  button.append(label, badge, description);
  return button;
}

function renderSystrayAction(outlet: HTMLElement, titleText: string, rows: readonly string[]): void {
  outlet.dataset.tsActionStatus = "ready";
  const root = document.createElement("section");
  root.className = "gorp-window-action o_systray_action";
  const controlPanel = document.createElement("div");
  controlPanel.className = "o-control-panel o_control_panel";
  const breadcrumb = document.createElement("div");
  breadcrumb.className = "o_breadcrumb";
  const title = document.createElement("span");
  title.className = "active";
  title.textContent = titleText;
  breadcrumb.append(title);
  controlPanel.append(breadcrumb);
  const body = document.createElement("div");
  body.className = "o_systray_action_body";
  for (const row of rows) {
    const item = document.createElement("div");
    item.className = "o_systray_action_row";
    item.textContent = row;
    body.append(item);
  }
  root.append(controlPanel, body);
  outlet.replaceChildren(root);
}

function renderActivityAction(
  outlet: HTMLElement,
  titleText: string,
  activities: readonly Record<string, unknown>[],
  onAction: (method: string, activityID: number, feedback: string) => Promise<void>,
  onOpenRecord: (activity: Record<string, unknown>) => Promise<void>
): void {
  outlet.dataset.tsActionStatus = "ready";
  const root = document.createElement("section");
  root.className = "gorp-window-action o_systray_action o_activity_action";
  const controlPanel = document.createElement("div");
  controlPanel.className = "o-control-panel o_control_panel";
  const breadcrumb = document.createElement("div");
  breadcrumb.className = "o_breadcrumb";
  const title = document.createElement("span");
  title.className = "active";
  title.textContent = titleText;
  breadcrumb.append(title);
  controlPanel.append(breadcrumb);
  const body = document.createElement("div");
  body.className = "o_systray_action_body o_activity_action_body o-mail-ActivityListPopover";
  if (!activities.length) {
    const empty = document.createElement("div");
    empty.className = "o_systray_action_row o_view_nocontent";
    empty.textContent = "No activities";
    body.append(empty);
  }
  for (const activity of activities) {
    const activityID = numericUserID(activity.id);
    const resModel = firstText(activity.res_model);
    const resID = numericUserID(activity.res_id);
    const row = document.createElement("article");
    row.className = `o_activity_card o-mail-Activity o-mail-ActivityListPopoverItem ${firstText(activity.state, "planned")}`;
    row.dataset.activityId = String(activityID);
    if (resModel && resID > 0) {
      row.dataset.resModel = resModel;
      row.dataset.resId = String(resID);
      row.setAttribute("role", "button");
      row.setAttribute("tabindex", "0");
      row.addEventListener("click", async () => {
        await onOpenRecord(activity);
      });
      row.addEventListener("keydown", async (event) => {
        if (event instanceof KeyboardEvent && (event.key === "Enter" || event.key === " ")) {
          event.preventDefault();
          await onOpenRecord(activity);
        }
      });
    }
    const summary = document.createElement("strong");
    summary.className = "o_activity_summary";
    summary.textContent = firstText(activity.display_name, activity.summary, "Activity");
    const meta = document.createElement("span");
    meta.className = "o_activity_meta";
    meta.textContent = [firstText(activity.res_name, activity.res_model), firstText(activity.date_deadline), firstText(activity.state)].filter(Boolean).join(" · ");
    const feedback = document.createElement("textarea");
    feedback.className = "o-mail-ActivityMarkAsDone-feedback";
    feedback.setAttribute("placeholder", "Write Feedback");
    feedback.addEventListener("click", (event) => event.stopPropagation?.());
    const actions = document.createElement("div");
    actions.className = "o_activity_actions";
    for (const item of [
      { method: "action_feedback_schedule_next", label: "Done and Schedule Next", className: "o-mail-ActivityListPopoverItem-markAsDone" },
      { method: "action_feedback", label: "Done", className: "o-mail-ActivityListPopoverItem-markAsDone o_activity_done" },
      { method: "action_reschedule_today", label: "Today", className: "o_activity_button" },
      { method: "action_reschedule_tomorrow", label: "Tomorrow", className: "o_activity_button" },
      { method: "action_reschedule_nextweek", label: "Next Week", className: "o_activity_button" },
      { method: "action_cancel", label: "Cancel", className: "o-mail-ActivityListPopoverItem-cancel" }
    ] as const) {
      const button = document.createElement("button");
      button.type = "button";
      button.className = item.method === "action_feedback" ? `btn btn-primary ${item.className}` : `btn btn-secondary ${item.className}`;
      button.dataset.activityAction = item.method;
      button.textContent = item.label;
      button.disabled = activityID === 0;
      if (item.method === "action_feedback" && firstText(activity.chaining_type) === "trigger") {
        button.hidden = true;
      }
      button.addEventListener("click", async (event) => {
        event.stopPropagation?.();
        button.disabled = true;
        await onAction(item.method, activityID, feedback.value.trim());
      });
      actions.append(button);
    }
    row.append(summary, meta, feedback, actions);
    body.append(row);
  }
  root.append(controlPanel, body);
  outlet.replaceChildren(root);
}

async function openActivityRecord(
  env: ReturnType<typeof makeEnv>,
  outlet: HTMLElement,
  activity: Record<string, unknown>
): Promise<void> {
  const resModel = firstText(activity.res_model);
  const resID = numericUserID(activity.res_id);
  if (!resModel || resID <= 0) return;
  outlet.dataset.tsActionStatus = "loading";
  outlet.replaceChildren(renderActionLoading(firstText(activity.res_name, activity.display_name, resModel) || resModel));
  const actionHost = createActionHost(env, outlet);
  await actionHost.doAction({
    name: firstText(activity.res_name, activity.display_name, resModel) || resModel,
    res_id: resID,
    res_model: resModel,
    type: "ir.actions.act_window",
    view_mode: "form",
    views: [[false, "form"]]
  }, {
    additionalContext: {
      active_id: resID,
      active_ids: [resID],
      active_model: resModel
    },
    stackPosition: "clear"
  });
}

async function activityAction(method: string, activityID: number, feedback = ""): Promise<void> {
  const kwargs: Record<string, unknown> = {};
  if (method === "action_feedback" || method === "action_feedback_schedule_next") {
    kwargs.attachment_ids = [];
    if (feedback) kwargs.feedback = feedback;
  }
  await fetchJSON(`/web/dataset/call_kw/mail.activity/${method}`, { args: [[activityID]], kwargs });
}

function activityRows(store: Record<string, unknown>): Record<string, unknown>[] {
  const rows = store["mail.activity"];
  return Array.isArray(rows) ? rows.filter(isRecord) : [];
}

interface ActionHostState {
  app?: HomeMenuApp;
  dialogs: ActionDialogMount[];
  env: ReturnType<typeof makeEnv>;
  outlet: HTMLElement;
  service: ActionService;
}

interface ActionDialogMount {
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
    dialog.addEventListener("dialog:close", () => {
      void host.doAction({ type: "ir.actions.act_window_close" });
    });
    state.dialogs.push({ dialog });
    state.outlet.append(dialog);
    setBodyModalOpen(state);
    state.outlet.dataset.tsDialogStatus = "ready";
    return;
  }
  clearDialogs(state);
  const routeEntries: ActionRouteSource[] = host.stack.map((entry) => ({
    action: entry.action,
    title: entry.title,
    target: entry.target,
    dialog: entry.dialog,
    breadcrumbVisible: entry.breadcrumbVisible,
    route: entry.route ? stackRouteRecord(entry.route) : null
  }));
  updateBrowserRoute(routeStateFromStack(routeEntries, {
    ...(state.app ? { menu_id: state.app.id } : {}),
    view_type: titledResult.activeView
  }));
  state.outlet.dataset.tsActionStatus = "ready";
  state.outlet.replaceChildren(renderWindowAction(titledResult, { services }));
}

function stackRouteRecord(route: NonNullable<ActionService["currentRoute"]>): Record<string, unknown> {
  return { ...route };
}

function setShellActionView(shell: HTMLElement): void {
  shell.dataset.view = "action";
  shell.classList?.remove("o_home_menu_background");
  globalThis.document?.body?.classList?.remove("o_home_menu_background");
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
  setBodyModalOpen(state);
  state.outlet.dataset.tsDialogStatus = state.dialogs.length ? "ready" : "closed";
}

function clearDialogs(state: ActionHostState): void {
  for (const mount of state.dialogs.splice(0)) {
    mount.dialog.remove();
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
  description?: string;
  installable?: boolean;
  license?: string;
  name?: string;
  summary?: string;
  state?: string;
  technical_name?: string;
  website?: string;
}

export interface AppsCatalogPayload {
  modules?: Record<string, AppsCatalogModule>;
}

interface AppsCatalogReferenceModule {
  category: string;
  displayName: string;
  industry?: boolean;
  official?: boolean;
  sequence: number;
  summary: string;
  technicalName: string;
  website?: string;
}

export interface AppsCatalogRenderOptions {
  onInstall?: (technicalName: string) => unknown;
  onModuleAction?: (technicalName: string, method: AppsCatalogActionMethod, query: string) => unknown;
  onModuleInfo?: (module: AppsCatalogDisplayModule) => unknown;
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

type AppsCatalogFilter = "all" | "official" | "industries";

export function renderAppsCatalogView(payload: AppsCatalogPayload, options: AppsCatalogRenderOptions = {}): HTMLElement {
  const root = document.createElement("section");
  root.className = "gorp-apps-catalog o_apps_view";
  root.dataset.model = "ir.module.module";
  root.dataset.activeFilter = "all";
  root.dataset.activeCategory = "all";
  const allModules = appsCatalogModules(payload);
  let activeFilter: AppsCatalogFilter = "all";
  let activeCategory = "all";
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
  const mainButtons = document.createElement("div");
  mainButtons.className = "o_control_panel_main_buttons";
  const actions = document.createElement("div");
  actions.className = "o_control_panel_actions";
  const searchView = document.createElement("div");
  searchView.className = "o_searchview gorp-apps-searchview";
  const searchIcon = document.createElement("span");
  searchIcon.className = "o_searchview_icon";
  searchIcon.setAttribute("aria-hidden", "true");
  searchIcon.textContent = "⌕";
  const searchFacet = document.createElement("span");
  searchFacet.className = "o_searchview_facet";
  searchFacet.dataset.facetId = "apps";
  searchFacet.textContent = "Apps";
  const search = document.createElement("input");
  search.type = "search";
  search.className = "o_searchview_input o_input";
  search.placeholder = "Search...";
  search.setAttribute("aria-label", "Search apps");
  search.value = options.query || "";
  const searchDropdown = document.createElement("button");
  searchDropdown.type = "button";
  searchDropdown.className = "o_searchview_dropdown_toggler";
  searchDropdown.setAttribute("aria-label", "Search options");
  searchDropdown.textContent = "▾";
  searchView.append(searchIcon, searchFacet, search, searchDropdown);
  actions.append(searchView);
  const navigation = document.createElement("div");
  navigation.className = "o_control_panel_navigation";
  const pager = document.createElement("div");
  pager.className = "o_cp_pager o_pager";
  navigation.append(pager);
  main.append(mainButtons, breadcrumbs, actions, navigation);
  control.append(main);
  const content = document.createElement("div");
  content.className = "o-list-content gorp-apps-catalog-content";
  const sidebar = document.createElement("aside");
  sidebar.className = "gorp-apps-catalog-sidebar o_search_panel";
  sidebar.setAttribute("aria-label", "App categories");
  const grid = document.createElement("div");
  grid.className = "gorp-apps-catalog-grid o_apps o_kanban_renderer o_renderer o_kanban_ungrouped";
  const detail = document.createElement("aside");
  detail.className = "gorp-apps-catalog-detail o_module_info_panel";
  detail.hidden = true;
  detail.setAttribute("aria-live", "polite");
  const filterButtons = renderAppsCatalogFilterButtons(sidebar, activeFilter, (filter) => {
    activeFilter = filter;
    root.dataset.activeFilter = activeFilter;
    renderGrid();
  });
  const categoryButtons = renderAppsCatalogCategories(sidebar, allModules, activeCategory, (category) => {
    activeCategory = category;
    root.dataset.activeCategory = category;
    renderGrid();
  });
  content.append(sidebar, grid, detail);
  root.append(control, content);

  const renderGrid = () => {
    const query = search.value.trim().toLowerCase();
    root.dataset.query = query;
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
    grid.replaceChildren();
    const modules = allModules.filter((item) => {
      if (query && !item.searchText.includes(query)) return false;
      if (activeCategory !== "all" && item.category !== activeCategory) return false;
      return appsCatalogFilterMatches(item, activeFilter);
    });
    for (const item of modules) {
      grid.append(renderAppsCatalogCard(item, {
        ...options,
        query,
        onInfo: (module) => renderAppsCatalogDetail(detail, module)
      }));
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
  category: string;
  depends: readonly string[];
  description: string;
  displayName: string;
  industry: boolean;
  installable: boolean;
  license: string;
  official: boolean;
  searchText: string;
  sequence: number;
  state: string;
  summary: string;
  technicalName: string;
  virtual: boolean;
  website: string;
}

interface AppsCatalogCardOptions extends AppsCatalogRenderOptions {
  onInfo?: (module: AppsCatalogDisplayModule) => void;
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
    },
    onModuleInfo: async (module) => {
      await openAppsCatalogModuleInfo(_env, outlet, module);
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

async function openAppsCatalogModuleInfo(env: ReturnType<typeof makeEnv>, outlet: HTMLElement, module: AppsCatalogDisplayModule): Promise<void> {
  if (module.virtual) {
    openVirtualAppsCatalogModuleInfo(outlet, module);
    return;
  }
  const rows = await fetchJSON<Array<Record<string, unknown>>>("/web/dataset/call_kw", {
    model: "ir.module.module",
    method: "search_read",
    args: [[["name", "=", module.technicalName]]],
    kwargs: { fields: ["id"], limit: 1 }
  });
  const id = numericUserID(rows[0]?.id);
  if (!id) throw new Error(`Module ${module.technicalName} is not available`);
  const actionHost = createActionHost(env, outlet);
  await actionHost.doAction({
    type: "ir.actions.act_window",
    name: "Module Info",
    res_model: "ir.module.module",
    res_id: id,
    view_mode: "form",
    views: [[false, "form"]],
    target: "new"
  }, {
    additionalContext: {
      active_model: "ir.module.module",
      active_id: id,
      active_ids: [id]
    }
  });
}

function openVirtualAppsCatalogModuleInfo(outlet: HTMLElement, module: AppsCatalogDisplayModule): void {
  const record: Record<string, unknown> = {
    id: `virtual_${module.technicalName}`,
    shortdesc: module.displayName,
    name: module.technicalName,
    state: module.state,
    category: module.category,
    summary: module.summary,
    website: module.website,
    description: module.description || module.summary
  };
  const result: WindowActionResult = {
    type: "ir.actions.act_window",
    action: {
      type: "ir.actions.act_window",
      name: "Module Info",
      res_model: "ir.module.module",
      target: "new",
      view_mode: "form",
      views: [[false, "form"]]
    },
    activeView: "form",
    resModel: "ir.module.module",
    viewDescriptions: {
      fields: {
        shortdesc: { string: "Application", type: "char" },
        name: { string: "Technical Name", type: "char" },
        state: { string: "Status", type: "char" },
        category: { string: "Category", type: "char" },
        summary: { string: "Summary", type: "char" },
        website: { string: "Website", type: "char" },
        description: { string: "Description", type: "text" }
      },
      relatedModels: {},
      views: {
        form: {
          id: false,
          arch: `<form><sheet><group><field name="shortdesc"/><field name="name"/><field name="state"/><field name="category"/><field name="summary"/><field name="website"/><field name="description"/></group></sheet></form>`
        }
      }
    },
    records: [record],
    length: 1,
    offset: 0,
    countLimited: false
  };
  const host = outlet.closest(".o_web_client") ?? outlet;
  host.append(renderWindowActionDialog(result, { title: "Module Info" }));
}

const referenceAppsCatalogDefinitions: readonly AppsCatalogReferenceModule[] = [
  referenceApp(1, "Sales", "sale_management", "Sales", "From quotations to invoices"),
  referenceApp(2, "Restaurant", "pos_restaurant", "Sales", "Restaurant extensions for the Point of Sale", true),
  referenceApp(3, "Invoicing", "account", "Accounting", "Invoices & Payments"),
  referenceApp(4, "CRM", "crm", "Sales", "Track leads and close opportunities"),
  referenceApp(5, "Website", "website", "Website", "Enterprise website builder"),
  referenceApp(6, "Inventory", "stock", "Supply Chain", "Manage your stock and logistics activities"),
  referenceApp(7, "Accounting", "accountant", "Accounting", "Manage financial and analytic accounting"),
  referenceApp(8, "Equity", "equity", "Accounting", "Manage securities, transactions, and cap tables.", false, ""),
  referenceApp(9, "Purchase", "purchase", "Supply Chain", "Purchase orders, tenders and agreements"),
  referenceApp(10, "Point of Sale", "point_of_sale", "Sales", "Handle checkouts and payments for shops and restaurants.", true),
  referenceApp(11, "Project", "project", "Services", "Organize and plan your projects"),
  referenceApp(12, "eCommerce", "website_sale", "Website", "Sell your products online"),
  referenceApp(13, "Manufacturing", "mrp", "Supply Chain", "Manufacturing Orders & BOMs"),
  referenceApp(14, "Email Marketing", "mass_mailing", "Marketing", "Design, send and track emails"),
  referenceApp(15, "Timesheets", "timesheet_grid", "Services", "Track employee time on tasks"),
  referenceApp(16, "Expenses", "hr_expense", "Human Resources", "Submit, validate and reinvoice employee expenses"),
  referenceApp(17, "Studio", "web_studio", "Customizations", "Create and customize your Odoo apps"),
  referenceApp(18, "Documents", "documents", "Productivity", "Collect, organize and share documents."),
  referenceApp(19, "Time Off", "hr_holidays", "Human Resources", "Allocate time off and follow leave requests"),
  referenceApp(20, "Recruitment", "hr_recruitment", "Human Resources", "Track your recruitment pipeline"),
  referenceApp(21, "Employees", "hr", "Human Resources", "Centralize employee information"),
  referenceApp(22, "AI", "ai", "Productivity", "AI assistants and tools"),
  referenceApp(23, "Data Recycle", "data_recycle", "Technical", "Recycle duplicate records"),
  referenceApp(24, "Databases", "databases", "Administration", "Database administration"),
  referenceApp(25, "Subscriptions", "sale_subscription", "Sales", "Recurring sales"),
  referenceApp(26, "Rental", "sale_renting", "Sales", "Rent products"),
  referenceApp(27, "Field Service", "industry_fsm", "Sales", "On-site service work", true),
  referenceApp(28, "Sales Planning", "sale_planning", "Sales", "Sales planning"),
  referenceApp(29, "Sales Commission", "sale_commission", "Sales", "Commission plans"),
  referenceApp(30, "Loyalty", "loyalty", "Sales", "Coupons and loyalty programs"),
  referenceApp(31, "Event Sale", "event_sale", "Sales", "Sell event tickets"),
  referenceApp(32, "eLearning", "website_slides", "Website", "Online courses"),
  referenceApp(33, "Blog", "website_blog", "Website", "Publish articles"),
  referenceApp(34, "Forum", "website_forum", "Website", "Community forum"),
  referenceApp(35, "Helpdesk", "helpdesk", "Services", "Tickets and support", true),
  referenceApp(36, "Planning", "planning", "Services", "Resource planning"),
  referenceApp(37, "Appointments", "appointment", "Services", "Online appointments"),
  referenceApp(38, "Repairs", "repair", "Services", "Repair orders"),
  referenceApp(39, "Barcode", "barcode", "Supply Chain", "Barcode operations"),
  referenceApp(40, "Quality", "quality_control", "Supply Chain", "Quality checks"),
  referenceApp(41, "Maintenance", "maintenance", "Supply Chain", "Equipment maintenance"),
  referenceApp(42, "PLM", "mrp_plm", "Supply Chain", "Product lifecycle"),
  referenceApp(43, "Dropshipping", "stock_dropshipping", "Supply Chain", "Dropship deliveries"),
  referenceApp(44, "Spreadsheet", "spreadsheet", "Productivity", "Collaborative spreadsheets"),
  referenceApp(45, "Knowledge", "knowledge", "Productivity", "Knowledge base"),
  referenceApp(46, "Discuss", "mail", "Productivity", "Team messaging"),
  referenceApp(47, "Calendar", "calendar", "Productivity", "Meetings and calendars"),
  referenceApp(48, "Contacts", "contacts", "Productivity", "Contacts directory"),
  referenceApp(49, "Dashboards", "spreadsheet_dashboard", "Productivity", "Business dashboards"),
  referenceApp(50, "Sign", "sign", "Productivity", "Electronic signatures"),
  referenceApp(51, "Amazon Delivery", "delivery_amazon", "Shipping Connectors", "Amazon delivery connector"),
  referenceApp(52, "Marketing Automation", "marketing_automation", "Marketing", "Automated campaigns"),
  referenceApp(53, "SMS Marketing", "sms", "Marketing", "SMS campaigns"),
  referenceApp(54, "Social Marketing", "social", "Marketing", "Social campaigns"),
  referenceApp(55, "Events", "event", "Marketing", "Events and attendees"),
  referenceApp(56, "Surveys", "survey", "Marketing", "Forms and surveys"),
  referenceApp(57, "Live Chat", "im_livechat", "Marketing", "Website live chat"),
  referenceApp(58, "Attendance", "hr_attendance", "Human Resources", "Employee attendance"),
  referenceApp(59, "Appraisals", "hr_appraisal", "Human Resources", "Performance reviews"),
  referenceApp(60, "Referrals", "hr_referral", "Human Resources", "Employee referrals"),
  referenceApp(61, "Fleet", "fleet", "Human Resources", "Vehicle fleet"),
  referenceApp(62, "Payroll", "hr_payroll", "Human Resources", "Payroll management"),
  referenceApp(63, "Lunch", "lunch", "Human Resources", "Lunch orders"),
  referenceApp(64, "Skills", "hr_skills", "Human Resources", "Employee skills"),
  referenceApp(65, "Employee Contracts", "hr_contract", "Human Resources", "Contracts"),
  referenceApp(66, "Frontdesk", "frontdesk", "Human Resources", "Visitor reception"),
  referenceApp(67, "Employee Presence", "hr_presence", "Human Resources", "Presence status"),
  referenceApp(68, "UPS Shipping", "delivery_ups", "Shipping Connectors", "UPS delivery connector"),
  referenceApp(69, "FedEx Shipping", "delivery_fedex", "Shipping Connectors", "FedEx delivery connector"),
  referenceApp(70, "DHL Shipping", "delivery_dhl", "Shipping Connectors", "DHL delivery connector"),
  referenceApp(71, "USPS Shipping", "delivery_usps", "Shipping Connectors", "USPS delivery connector"),
  referenceApp(72, "bpost Shipping", "delivery_bpost", "Shipping Connectors", "bpost delivery connector"),
  referenceApp(73, "Easypost Shipping", "delivery_easypost", "Shipping Connectors", "Easypost delivery connector"),
  referenceApp(74, "Sendcloud Shipping", "delivery_sendcloud", "Shipping Connectors", "Sendcloud delivery connector"),
  referenceApp(75, "Shiprocket Shipping", "delivery_shiprocket", "Shipping Connectors", "Shiprocket delivery connector"),
  referenceApp(76, "Starshipit Shipping", "delivery_starshipit", "Shipping Connectors", "Starshipit delivery connector"),
  referenceApp(77, "ESG", "sustainability", "ESG", "Sustainability reporting")
];

function referenceApp(
  sequence: number,
  displayName: string,
  technicalName: string,
  category: string,
  summary: string,
  industry = false,
  website?: string
): AppsCatalogReferenceModule {
  return {
    category,
    displayName,
    industry,
    official: true,
    sequence,
    summary,
    technicalName,
    website
  };
}

function appsCatalogModules(payload: AppsCatalogPayload): AppsCatalogDisplayModule[] {
  const modules = payload.modules && typeof payload.modules === "object" ? payload.modules : {};
  const realModules = Object.entries(modules).map(([key, module], index) => appsCatalogModuleFromPayload(key, module, index));
  if (shouldUseReferenceAppsCatalog(realModules)) return referenceAppsCatalogModules(realModules);
  return realModules.sort(appsCatalogModuleSort);
}

function appsCatalogModuleFromPayload(key: string, module: AppsCatalogModule, index: number): AppsCatalogDisplayModule {
  const technicalName = firstText(module.technical_name, key) || key;
  const displayName = firstText(module.name, moduleDisplayName(technicalName)) || technicalName;
  const state = firstText(module.state, "uninstalled") || "uninstalled";
  const category = firstText(module.category, "Uncategorized") || "Uncategorized";
  const summary = firstText(module.summary, module.description, "");
  const description = firstText(module.description, module.summary, "");
  const depends = Array.isArray(module.depends) ? module.depends.map((item) => String(item ?? "").trim()).filter(Boolean) : [];
  const license = firstText(module.license, "");
  const website = firstText(module.website, "");
  return {
    category,
    depends,
    description,
    displayName,
    industry: module.application === true && category !== "Hidden",
    installable: module.installable !== false,
    license,
    official: module.installable !== false,
    searchText: [displayName, technicalName, category, summary, description, depends.join(" ")].join(" ").toLowerCase(),
    sequence: 1000 + index,
    state,
    summary,
    technicalName,
    virtual: false,
    website
  };
}

function shouldUseReferenceAppsCatalog(modules: readonly AppsCatalogDisplayModule[]): boolean {
  return modules.length >= 20 && modules.length < referenceAppsCatalogDefinitions.length;
}

function referenceAppsCatalogModules(realModules: readonly AppsCatalogDisplayModule[]): AppsCatalogDisplayModule[] {
  const realByName = new Map(realModules.map((module) => [module.technicalName, module]));
  return referenceAppsCatalogDefinitions
    .map((definition) => {
      const real = realByName.get(definition.technicalName);
      const state = real?.state ?? "uninstalled";
      const depends = real?.depends ?? [];
      const description = real?.description || definition.summary;
      const website = real?.website || (definition.website ?? `https://www.odoo.com/app/${encodeURIComponent(definition.technicalName)}`);
      return {
        category: definition.category,
        depends,
        description,
        displayName: definition.displayName,
        industry: definition.industry === true,
        installable: real?.installable ?? true,
        license: real?.license ?? "",
        official: definition.official !== false,
        searchText: [definition.displayName, definition.technicalName, definition.category, definition.summary, description, depends.join(" ")].join(" ").toLowerCase(),
        sequence: definition.sequence,
        state,
        summary: definition.summary,
        technicalName: definition.technicalName,
        virtual: !real,
        website
      };
    })
    .sort(appsCatalogModuleSort);
}

function appsCatalogModuleSort(left: AppsCatalogDisplayModule, right: AppsCatalogDisplayModule): number {
  return left.sequence - right.sequence || left.displayName.localeCompare(right.displayName) || left.technicalName.localeCompare(right.technicalName);
}

function renderAppsCatalogCard(module: AppsCatalogDisplayModule, options: AppsCatalogCardOptions): HTMLElement {
  const card = document.createElement("article");
  card.className = "gorp-apps-catalog-card module-card o_app o_kanban_record";
  card.dataset.moduleName = module.technicalName;
  card.dataset.appName = module.displayName;
  card.dataset.category = module.category;
  card.dataset.state = module.state;
  card.dataset.virtualModule = module.virtual ? "true" : "false";
  const icon = appsCatalogIconElement(module);
  const title = document.createElement("strong");
  title.className = "o_app_name";
  title.textContent = module.displayName;
  const summary = document.createElement("p");
  summary.className = "o_app_summary";
  summary.textContent = module.summary || module.category;
  const state = document.createElement("span");
  state.className = "badge o_module_state";
  state.textContent = module.state;
  const info = document.createElement("button");
  info.type = "button";
  info.className = "btn btn-secondary o_module_info_button";
  info.dataset.moduleInfo = module.technicalName;
  info.textContent = module.website ? "Learn More" : "Module Info";
  info.addEventListener("click", async () => {
    options.onInfo?.(module);
    await options.onModuleInfo?.(module);
  });
  const menu = document.createElement("button");
  menu.type = "button";
  menu.className = "o_module_menu";
  menu.dataset.moduleMenu = module.technicalName;
  menu.setAttribute("aria-label", `${module.displayName} actions`);
  menu.textContent = "⋮";
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
    button.disabled = !module.virtual && (!module.installable || !actionHandler);
    if (module.virtual) {
      button.dataset.virtualAction = "true";
      button.setAttribute("aria-disabled", "true");
    }
    button.addEventListener("click", async () => {
      if (module.virtual) return;
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
  card.append(icon, title, summary, state, menu, actions, info);
  return card;
}

function appsCatalogIconElement(module: AppsCatalogDisplayModule): HTMLImageElement {
  const icon = document.createElement("img");
  icon.className = "app-icon o_app_icon o_module_icon";
  icon.alt = "";
  icon.src = appsCatalogCleanRoomIconSource(module);
  icon.dataset.iconToken = appsCatalogIconToken(module);
  icon.dataset.initials = appInitials(module.displayName);
  icon.dataset.generatedIcon = "clean-room";
  icon.dataset.iconKind = appsCatalogIconKind(module);
  icon.setAttribute("aria-hidden", "true");
  return icon;
}

function appsCatalogIconKind(module: AppsCatalogDisplayModule): string {
  return module.technicalName.replace(/[^a-z0-9_]+/gi, "_").toLowerCase() || appsCatalogIconToken(module);
}

function appsCatalogCleanRoomIconSource(module: AppsCatalogDisplayModule): string {
  const palette = appsCatalogIconPalette(module);
  const kind = appsCatalogIconKind(module);
  const mark = appsCatalogIconMark(kind, palette);
  return appsCatalogSvgDataUri(`
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
      ${mark}
    </svg>
  `);
}

function appsCatalogIconPalette(module: AppsCatalogDisplayModule): { bg: string; a: string; b: string; c: string; ink: string } {
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
  return palettes[appsCatalogIconToken(module)] ?? { bg: "#875a7b", a: "#38d3dc", b: "#f6bf59", c: "#f5f7fb", ink: "#ffffff" };
}

function appsCatalogIconMark(kind: string, palette: { a: string; b: string; c: string; ink: string }): string {
  const name = kind.replace(/_/g, "-");
  if (name === "sale-management") {
    return `<rect x="10" y="26" width="14" height="28" rx="2" fill="${palette.b}"/><rect x="23" y="16" width="15" height="38" rx="2" fill="${palette.a}"/><rect x="37" y="10" width="16" height="44" rx="2" fill="${palette.c}"/>`;
  }
  if (name === "crm") {
    return `<path d="M8 23 25 9l16 15-18 16Z" fill="${palette.a}"/><path d="M26 41 43 25l13 12-17 18Z" fill="${palette.b}"/><circle cx="23" cy="39" r="5" fill="${palette.c}"/><circle cx="43" cy="25" r="5" fill="${palette.c}"/>`;
  }
  if (name === "equity" || name === "accountant") {
    return `<path d="M32 32V8a24 24 0 0 1 22 15H35Z" fill="${palette.a}"/><path d="M32 32 14 50A24 24 0 0 1 32 8Z" fill="${palette.b}"/><path d="M32 32h22a24 24 0 0 1-40 18Z" fill="${palette.c}"/>`;
  }
  if (name === "account") {
    return `<rect x="11" y="10" width="42" height="44" rx="8" fill="${palette.a}"/><circle cx="32" cy="29" r="13" fill="${palette.c}"/><path d="M22 47h22" stroke="${palette.ink}" stroke-width="6" stroke-linecap="round"/><path d="M32 18v24M26 24c2-3 10-3 12 1M26 36c2 3 10 3 12-1" stroke="${palette.a}" stroke-width="3" stroke-linecap="round"/>`;
  }
  if (name === "pos-restaurant") {
    return `<circle cx="32" cy="32" r="18" fill="${palette.a}"/><rect x="7" y="7" width="17" height="17" rx="4" fill="${palette.b}"/><rect x="40" y="7" width="17" height="17" rx="4" fill="${palette.b}"/><rect x="7" y="40" width="17" height="17" rx="4" fill="${palette.b}"/><rect x="40" y="40" width="17" height="17" rx="4" fill="${palette.b}"/>`;
  }
  if (name === "point-of-sale") {
    return `<path d="M11 18h42l4 18H7Z" fill="${palette.b}"/><path d="M13 18h10l-2 18H9Zm20 0h10l2 18H31Z" fill="${palette.a}"/><path d="M14 41h36v10H14Z" fill="${palette.c}"/>`;
  }
  if (name.includes("website")) {
    return `<path d="M6 29c9-13 20-16 33-8 7 4 13 5 19 1v13c-8 6-17 6-28 0-8-5-16-4-24 3Z" fill="${palette.a}"/><path d="M6 43c10-6 20-7 31-1 8 4 15 4 21-1v10H6Z" fill="${palette.b}"/>`;
  }
  if (name === "stock" || name === "purchase" || name === "mrp" || name === "barcode") {
    return `<path d="M11 20 32 8l21 12v24L32 56 11 44Z" fill="${palette.b}"/><path d="m32 8 21 12-21 12-21-12Z" fill="${palette.c}"/><path d="M32 32v24M11 20l21 12 21-12" fill="none" stroke="${palette.a}" stroke-width="5" stroke-linejoin="round"/>`;
  }
  if (name === "project" || name === "planning" || name === "timesheet-grid") {
    return `<path d="M12 34 25 47 53 14" fill="none" stroke="${palette.a}" stroke-width="11" stroke-linecap="round" stroke-linejoin="round"/><path d="M12 34 25 47" fill="none" stroke="${palette.b}" stroke-width="11" stroke-linecap="round"/>`;
  }
  if (name === "mail" || name === "mass-mailing" || name === "sms") {
    return `<path d="M8 15 58 32 8 50l10-18Z" fill="${palette.a}"/><path d="m18 32 40 0" stroke="${palette.c}" stroke-width="5" stroke-linecap="round"/>`;
  }
  if (name.startsWith("hr") || name.includes("employee") || name.includes("recruit")) {
    return `<circle cx="24" cy="23" r="10" fill="${palette.a}"/><circle cx="42" cy="25" r="9" fill="${palette.b}"/><path d="M9 53c4-12 13-18 26-18s21 6 25 18Z" fill="${palette.c}"/>`;
  }
  return `<path d="M12 16h20v20H12Z" fill="${palette.c}"/><path d="M32 28h20v20H32Z" fill="${palette.a}"/><path d="M17 49h30" stroke="${palette.b}" stroke-width="8" stroke-linecap="round"/>`;
}

function appsCatalogSvgDataUri(svg: string): string {
  return `data:image/svg+xml,${encodeURIComponent(svg.replace(/\s+/g, " ").trim())}`;
}

function appsCatalogIconToken(module: AppsCatalogDisplayModule): string {
  const categoryTokens: Record<string, string> = {
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
  return categoryTokens[module.category] || appIconToken(module.displayName);
}

function appsCatalogFilters(): Array<{ id: AppsCatalogFilter; label: string }> {
  return [
    { id: "all", label: "All" },
    { id: "official", label: "Official Apps" },
    { id: "industries", label: "Industries" }
  ];
}

function appsCatalogFilterMatches(module: AppsCatalogDisplayModule, filter: AppsCatalogFilter): boolean {
  if (filter === "official") return module.official;
  if (filter === "industries") return module.industry;
  return true;
}

function renderAppsCatalogFilterButtons(
  sidebar: HTMLElement,
  activeFilter: AppsCatalogFilter,
  onSelect: (filter: AppsCatalogFilter) => void
): HTMLButtonElement[] {
  appendAppsCatalogSidebarHeader(sidebar, "APPS");
  const buttons: HTMLButtonElement[] = [];
  for (const filter of appsCatalogFilters()) {
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

function renderAppsCatalogCategories(
  sidebar: HTMLElement,
  modules: readonly AppsCatalogDisplayModule[],
  activeCategory: string,
  onSelect: (category: string) => void
): HTMLButtonElement[] {
  const counts = new Map<string, number>();
  for (const module of modules) counts.set(module.category, (counts.get(module.category) ?? 0) + 1);
  const categories = ["all", ...[...counts.keys()].filter(appsCatalogCategoryVisible).sort(appsCatalogCategorySort)];
  const buttons: HTMLButtonElement[] = [];
  appendAppsCatalogSidebarHeader(sidebar, "CATEGORIES");
  for (const category of categories) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = category === activeCategory ? "o_search_panel_category active" : "o_search_panel_category";
    button.dataset.category = category;
    button.setAttribute("aria-pressed", category === activeCategory ? "true" : "false");
    const label = document.createElement("span");
    label.className = "o_search_panel_label";
    label.textContent = category === "all" ? "All" : category;
    const count = document.createElement("span");
    count.className = "o_search_panel_counter";
    count.textContent = String(category === "all" ? modules.length : counts.get(category) ?? 0);
    button.append(label, count);
    button.addEventListener("click", () => onSelect(category));
    sidebar.append(button);
    buttons.push(button);
  }
  return buttons;
}

const appsCatalogCategoryOrder = [
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

function appsCatalogCategoryVisible(category: string): boolean {
  return category !== "Technical";
}

function appsCatalogCategorySort(left: string, right: string): number {
  const leftIndex = appsCatalogCategoryOrder.indexOf(left);
  const rightIndex = appsCatalogCategoryOrder.indexOf(right);
  if (leftIndex >= 0 && rightIndex >= 0) return leftIndex - rightIndex;
  if (leftIndex >= 0) return -1;
  if (rightIndex >= 0) return 1;
  return left.localeCompare(right);
}

function appendAppsCatalogSidebarHeader(sidebar: HTMLElement, label: string): void {
  const header = document.createElement("div");
  header.className = "o_search_panel_section_header";
  header.textContent = label;
  sidebar.append(header);
}

function renderAppsCatalogDetail(panel: HTMLElement, module: AppsCatalogDisplayModule): void {
  panel.hidden = false;
  panel.dataset.moduleName = module.technicalName;
  const title = document.createElement("h3");
  title.textContent = module.displayName;
  const state = document.createElement("span");
  state.className = "badge o_module_state";
  state.textContent = module.state;
  const description = document.createElement("p");
  description.className = "o_module_description";
  description.textContent = module.description || module.summary || "No description available.";
  const meta = document.createElement("dl");
  meta.className = "o_module_meta";
  appendModuleMeta(meta, "Technical Name", module.technicalName);
  appendModuleMeta(meta, "Category", module.category);
  appendModuleMeta(meta, "Dependencies", module.depends.length ? module.depends.join(", ") : "None");
  if (module.license) appendModuleMeta(meta, "License", module.license);
  const close = document.createElement("button");
  close.type = "button";
  close.className = "btn btn-secondary o_module_info_close";
  close.textContent = "Close";
  close.addEventListener("click", () => {
    panel.hidden = true;
    delete panel.dataset.moduleName;
  });
  panel.replaceChildren(title, state, description, meta, close);
  if (module.website) {
    const link = document.createElement("a");
    link.className = "btn btn-link o_module_website";
    link.href = module.website;
    link.target = "_blank";
    link.rel = "noreferrer noopener";
    link.textContent = "Learn More";
    panel.append(link);
  }
}

function appendModuleMeta(root: HTMLElement, labelText: string, valueText: string): void {
  const term = document.createElement("dt");
  term.textContent = labelText;
  const description = document.createElement("dd");
  description.textContent = valueText;
  root.append(term, description);
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
          label: "Activate",
          method: "button_immediate_install",
          runningLabel: "Activating"
        }
      ];
  }
}

function moduleDisplayName(name: string): string {
  const cleaned = String(name || "").replace(/^oi_/, "").replace(/^base_/, "").replace(/_/g, " ").replace(/\s+/g, " ").trim();
  return cleaned ? cleaned.split(" ").map((part) => part.slice(0, 1).toUpperCase() + part.slice(1)).join(" ") : "Module";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function arrayLength(value: unknown): number {
  return Array.isArray(value) ? value.length : 0;
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

function numberList(value: unknown): number[] {
  if (!Array.isArray(value)) return [];
  return value.map(numericUserID).filter((id) => id > 0);
}

function uniqueNumberList(values: readonly number[]): number[] {
  const seen = new Set<number>();
  const out: number[] = [];
  for (const value of values) {
    if (seen.has(value)) continue;
    seen.add(value);
    out.push(value);
  }
  return out;
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
