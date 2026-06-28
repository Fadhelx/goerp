import assert from "node:assert/strict";
import {
  addChatMessage,
  agentChatActionTag,
  aiRoutes,
  createAgentChatActionHandler,
  createAgentChatActionState,
  createAIChatLauncher,
  createAiAdjustModelEvents,
  createAiAdjustSearchEvents,
  createAiButtonState,
  createAIComposerAction,
  createAiErrorState,
  createAiOpenMenuInvocation,
  createAIDraftChannelCall,
  createAIPromptButtonModels,
  createAIPromptButtonStorageKey,
  createAIPromptButtonStorageValue,
  createAIRecordContext,
  createAskAIActionCall,
  createChatPanelState,
  createCloseAIChatRequest,
  createGenerateResponseRequest,
  createGetAskAIAgentCall,
  createPromptButtons,
  createRealtimeTranscriptionParameters,
  createTranscriptionSessionRequest,
  formatCitationLabel,
  isAiNaturalLanguageEventName,
  launchAIChat,
  normalizeAIDraftChannelResult,
  normalizeSourceCitations,
  resolveAIRecipientPartnerIds,
  selectPromptButton,
  setChatPanelInput,
  setChatPanelOpen,
  validateAiAdminSettings
} from "../../../dist/packages/ai/src/index.js";

const button = createAiButtonState({
  status: "loading",
  label: "Ask AI",
  panelOpen: true,
  unreadCount: 2
});

assert.equal(button.disabled, true);
assert.equal(button.busy, true);
assert.equal(button.expanded, true);
assert.equal(button.badgeCount, 2);
assert.match(button.ariaLabel, /Ask AI/);

const promptButtons = createPromptButtons([
  { id: "summarize", label: "Summarize", prompt: "Summarize this record." },
  { id: "find-risk", label: "Find risk", prompt: "Find operational risk.", group: "analysis" }
]);

assert.equal(promptButtons.length, 2);
assert.equal(promptButtons[1].group, "analysis");
assert.equal(selectPromptButton(promptButtons, "summarize").prompt, "Summarize this record.");
assert.throws(() => createPromptButtons([
  { id: "dup", label: "A", prompt: "A" },
  { id: "dup", label: "B", prompt: "B" }
]), /duplicate prompt button id/);

const citations = normalizeSourceCitations([
  { title: "Runbook", uri: "https://example.test/runbook", line: 14 },
  { title: "Runbook", uri: "https://example.test/runbook", line: 14 },
  { excerpt: "internal note" }
]);

assert.equal(citations.length, 2);
assert.equal(citations[0].id, "https://example.test/runbook#14");
assert.equal(formatCitationLabel(citations[0], 0), "[1] Runbook:14");
assert.equal(citations[1].title, "Source 3");

const panel = createChatPanelState({
  messages: [
    {
      id: "m1",
      role: "assistant",
      content: "Use the runbook.",
      citations
    }
  ]
});
const openPanel = setChatPanelInput(setChatPanelOpen(panel, true), "Next step?");
const updatedPanel = addChatMessage(openPanel, {
  id: "m2",
  role: "user",
  content: "Next step?"
});

assert.equal(updatedPanel.open, true);
assert.equal(updatedPanel.input, "Next step?");
assert.equal(updatedPanel.messages.length, 2);
assert.equal(updatedPanel.messages[0].citations.length, 2);

const validSettings = validateAiAdminSettings({
  enabled: true,
  provider: "openai",
  model: "gpt-5-mini",
  maxPromptButtons: 4,
  maxContextSources: 10,
  timeoutMs: 15000
});

assert.equal(validSettings.valid, true);
assert.equal(validSettings.settings.provider, "openai");

const invalidSettings = validateAiAdminSettings({
  enabled: true,
  provider: "custom",
  model: "",
  endpoint: "ftp://example.test",
  maxPromptButtons: 30,
  maxContextSources: 65,
  timeoutMs: 500
});

assert.equal(invalidSettings.valid, false);
assert.deepEqual(
  invalidSettings.issues.map((issue) => issue.field),
  ["model", "endpoint", "maxPromptButtons", "maxContextSources", "timeoutMs"]
);

const rateLimit = createAiErrorState({ status: 429, message: "Too many requests" });
assert.equal(rateLimit.code, "rate_limit");
assert.equal(rateLimit.retryable, true);
assert.equal(rateLimit.message, "Too many requests");

const validationError = createAiErrorState("Bad prompt", { status: 400 });
assert.equal(validationError.code, "validation");
assert.equal(validationError.retryable, false);

const generateRequest = createGenerateResponseRequest({
  mailMessageId: 5,
  channelId: 9,
  currentViewInfo: { model: "res.partner", view_type: "form", available_view_types: ["list", "form"] },
  sessionIdentifier: "abc"
});
assert.equal(generateRequest.route, aiRoutes.generateResponse);
assert.equal(generateRequest.params.mail_message_id, 5);
assert.equal(generateRequest.params.channel_id, 9);
assert.equal(generateRequest.params.ai_session_identifier, "abc");
assert.equal(generateRequest.params.current_view_info.model, "res.partner");
assert.throws(() => createGenerateResponseRequest({ mailMessageId: 0, channelId: 9 }), /positive integer/);

const closeRequest = createCloseAIChatRequest(9);
assert.equal(closeRequest.route, aiRoutes.closeAIChat);
assert.deepEqual(closeRequest.params, { channel_id: 9 });

const transcriptionRequest = createTranscriptionSessionRequest("en", "Meeting notes");
assert.equal(transcriptionRequest.route, aiRoutes.transcriptionSession);
assert.deepEqual(transcriptionRequest.params, { language: "en", prompt: "Meeting notes" });

const realtime = createRealtimeTranscriptionParameters("en", "Meeting notes");
assert.equal(realtime.expires_after.anchor, "created_at");
assert.equal(realtime.expires_after.seconds, 7200);
assert.equal(realtime.session.audio.input.transcription.model, "whisper-1");
assert.equal(realtime.session.audio.input.turn_detection.type, "server_vad");
assert.equal(realtime.session.audio.input.noise_reduction.type, "far_field");

const draftCall = createAIDraftChannelCall({
  callerComponentName: "chatter_ai_button",
  channelTitle: "Partner reply",
  recordModel: "res.partner",
  recordId: 42,
  frontEndRecordInfo: { name: "Ada" },
  textSelection: "Selected text"
});
assert.equal(draftCall.model, "discuss.channel");
assert.equal(draftCall.method, "create_ai_draft_channel");
assert.deepEqual(draftCall.args, [
  "chatter_ai_button",
  "Partner reply",
  "res.partner",
  42,
  { name: "Ada" },
  "Selected text"
]);
assert.deepEqual(draftCall.kwargs, {});
assert.throws(() => createAIDraftChannelCall({ callerComponentName: "", recordId: 1 }), /caller component name/);
assert.throws(() => createAIDraftChannelCall({ callerComponentName: "mail_composer", recordId: 0 }), /record id/);

assert.deepEqual(createAskAIActionCall("Show partners"), {
  model: "ai.agent",
  method: "action_ask_ai",
  args: ["Show partners"],
  kwargs: {}
});
assert.deepEqual(createGetAskAIAgentCall(), {
  model: "ai.agent",
  method: "get_ask_ai_agent",
  args: [],
  kwargs: {}
});

const draftResult = normalizeAIDraftChannelResult({
  ai_channel_id: "77",
  data: { "discuss.channel": [{ id: 77, name: "AI" }] },
  prompts: ["Summarize", "Reply"],
  model_has_thread: true
});
assert.equal(draftResult.aiChannelId, 77);
assert.equal(draftResult.modelHasThread, true);
assert.deepEqual(draftResult.prompts, ["Summarize", "Reply"]);
assert.equal(createAIPromptButtonStorageKey(77), "ai.thread.prompt_buttons.77");
assert.equal(createAIPromptButtonStorageValue(["Summarize", "Reply"]), "[\"Summarize\",\"Reply\"]");
assert.throws(() => normalizeAIDraftChannelResult({ ai_channel_id: 77, prompts: "bad" }), /prompts should be an array/);

const recordContext = createAIRecordContext(
  {
    name: "Ada",
    avatar: "base64",
    partner_id: { display_name: "Partner A" },
    tag_ids: {
      records: [
        { data: { display_name: "VIP" } },
        { data: { name: "Prospect" } }
      ]
    }
  },
  {
    avatar: { type: "binary" },
    partner_id: { type: "many2one" },
    tag_ids: { type: "many2many" }
  }
);
assert.deepEqual(recordContext, {
  name: "Ada",
  partner_id: "Partner A",
  tag_ids: ["VIP", "Prospect"]
});

const agentAction = {
  type: "ir.actions.client",
  tag: agentChatActionTag,
  params: { channelId: "81", user_prompt: "Summarize" }
};
const actionState = createAgentChatActionState(agentAction);
assert.equal(actionState.channelId, 81);
assert.equal(actionState.threadModel, "discuss.channel");
assert.equal(actionState.shouldPost, true);

const threadCalls = [];
const handler = createAgentChatActionHandler({
  getThread(thread) {
    threadCalls.push(["getThread", thread]);
    return {
      status: "ready",
      isLoadedDeferred: Promise.resolve(),
      open(options) {
        threadCalls.push(["open", options]);
      },
      openChatWindow() {
        threadCalls.push(["openChatWindow"]);
      },
      post(message) {
        threadCalls.push(["post", message]);
      }
    };
  }
});
const handledState = await handler({ action: agentAction, options: {} });
assert.equal(handledState.channelId, 81);
assert.deepEqual(threadCalls, [
  ["getThread", { model: "discuss.channel", id: 81 }],
  ["open", { focus: true }],
  ["openChatWindow"],
  ["post", "Summarize"]
]);
const registryHandledState = await handler(null, { ...agentAction, params: { channelId: 82 } }, {});
assert.equal(registryHandledState.channelId, 82);
await assert.rejects(
  () => createAgentChatActionHandler({ getThread: () => null })({ action: agentAction }),
  /Thread not found/
);

const launcherCalls = [];
const launcherThread = {
  id: 90,
  model: "discuss.channel",
  status: "ready",
  suggestedRecipients: [
    { email: "new@example.test" },
    { email: "existing@example.test", partner_id: 12 }
  ],
  additionalRecipients: [{ email: "other@example.test", partnerId: 13 }],
  open(options) {
    launcherCalls.push(["thread.open", options]);
  },
  openChatWindow() {
    launcherCalls.push(["thread.openChatWindow"]);
  },
  fetchNewMessages() {
    launcherCalls.push(["thread.fetchNewMessages"]);
  }
};
const storageCalls = [];
const partnerRequests = [];
const insertedPartners = [];
const actionRequests = [];
const launcherOrm = {
  async call(model, method, args, kwargs) {
    launcherCalls.push(["orm.call", model, method, args, kwargs]);
    assert.equal(model, "discuss.channel");
    assert.equal(method, "create_ai_draft_channel");
    assert.equal(args[0], "chatter_ai_button");
    assert.equal(args[1], "Partner reply");
    assert.equal(args[2], "res.partner");
    assert.equal(args[3], 42);
    assert.deepEqual(args[4], { name: "Ada", partner_id: "Partner A" });
    assert.equal(args[5], "Selected text");
    return {
      ai_channel_id: "90",
      data: { "discuss.channel": [{ id: 90, name: "AI" }] },
      prompts: ["Summarize", "Reply"],
      model_has_thread: true
    };
  }
};
const mailStore = {
  aiInsertButtonTarget: false,
  insert(data) {
    launcherCalls.push(["store.insert", data]);
  },
  Thread: {
    getOrFetch(ref) {
      launcherCalls.push(["Thread.getOrFetch", ref]);
      return launcherThread;
    }
  },
  "res.partner": {
    insert(data) {
      insertedPartners.push(data);
      return { id: data.id };
    }
  }
};
const launcher = createAIChatLauncher({
  orm: launcherOrm,
  mailStore,
  action: {
    async doAction(action, options) {
      actionRequests.push({ action, options });
      await options.onClose();
    }
  },
  storage: {
    setItem(key, value) {
      storageCalls.push(["setItem", key, value]);
    },
    removeItem(key) {
      storageCalls.push(["removeItem", key]);
    },
    getItem() {
      return null;
    }
  },
  async partnerFromEmail(request) {
    partnerRequests.push(request);
    return [{ id: 44, display_name: "New Partner" }];
  }
});
const launchResult = await launcher.launchAIChat({
  callerComponentName: "chatter_ai_button",
  recordModel: "res.partner",
  recordId: 42,
  channelTitle: "Partner reply",
  aiChatSourceId: "editor-1",
  originalRecordData: {
    name: "Ada",
    avatar: "base64",
    partner_id: { display_name: "Partner A" }
  },
  originalRecordFields: {
    avatar: { type: "binary" },
    partner_id: { type: "many2one" }
  },
  textSelection: "Selected text"
});
assert.equal(launchResult.channelId, 90);
assert.equal(mailStore.aiInsertButtonTarget, "editor-1");
assert.deepEqual(launcherThread.ai_prompt_buttons, ["Summarize", "Reply"]);
assert.deepEqual(launcherThread.aiPromptButtons, ["Summarize", "Reply"]);
assert.equal(launcherThread.aiChatSource, "editor-1");
assert.equal(typeof launcherThread.aiSpecialActions.sendMessage, "function");
assert.equal(typeof launcherThread.aiSpecialActions.logNote, "function");
assert.deepEqual(storageCalls[0], ["setItem", "ai.thread.prompt_buttons.90", "[\"Summarize\",\"Reply\"]"]);
assert.deepEqual(launcherCalls.map((call) => call[0]), [
  "orm.call",
  "store.insert",
  "Thread.getOrFetch",
  "thread.open",
  "thread.openChatWindow"
]);
await launcherThread.aiSpecialActions.sendMessage("Hello");
assert.deepEqual(partnerRequests, [{
  threadModel: "res.partner",
  threadId: 42,
  emails: ["new@example.test"]
}]);
assert.deepEqual(insertedPartners, [{ id: 44, display_name: "New Partner" }]);
assert.deepEqual(actionRequests[0].action.context.default_partner_ids, [44, 12, 13]);
assert.equal(actionRequests[0].action.context.default_subtype_xmlid, "mail.mt_comment");
assert.equal(actionRequests[0].action.context.default_body, "Hello");
assert.deepEqual(actionRequests[0].action.context.default_res_ids, [42]);
assert.equal(launcherCalls.at(-1)[0], "thread.fetchNewMessages");
await launcherThread.aiSpecialActions.logNote("Internal");
assert.deepEqual(actionRequests[1].action.context.default_partner_ids, []);
assert.equal(actionRequests[1].action.context.default_subtype_xmlid, "mail.mt_note");
assert.deepEqual(createAIComposerAction("message", "res.partner", 7, "Body", [1]).context.default_partner_ids, [1]);
assert.deepEqual(createAIPromptButtonModels(["Summarize", "Reply"], 90), [
  { id: "90-1", label: "Summarize", prompt: "Summarize" },
  { id: "90-2", label: "Reply", prompt: "Reply" }
]);

const noPartnerThread = { suggestedRecipients: [{ email: "missing@example.test" }], additionalRecipients: [] };
assert.deepEqual(await resolveAIRecipientPartnerIds({ orm: launcherOrm, mailStore: { ...mailStore, Thread: mailStore.Thread } }, noPartnerThread, "res.partner", 1), []);
await assert.rejects(
  () => launchAIChat({ orm: launcherOrm, mailStore }, { callerComponentName: "chatter_ai_button", recordId: 0 }),
  /record id/
);

assert.equal(isAiNaturalLanguageEventName("AI_OPEN_MENU_LIST"), true);
assert.equal(isAiNaturalLanguageEventName("mail.message"), false);

const listInvocation = createAiOpenMenuInvocation(
  "AI_OPEN_MENU_LIST",
  {
    menuID: 7,
    selectedFilters: ["late"],
    selectedGroupBys: ["partner_id"],
    search: ["Ada"],
    customDomain: [["active", "=", true]],
    aiSessionIdentifier: "sid"
  },
  { id: 7, actionID: 99, name: "Partners" },
  "sid"
);
assert.equal(listInvocation.action.id, 99);
assert.equal(listInvocation.options.viewType, "list");
assert.equal(listInvocation.options.clearBreadcrumbs, true);
assert.deepEqual(listInvocation.options.props.ai.selectedFilters, ["late"]);
assert.deepEqual(listInvocation.options.props.ai.customDomain, [["active", "=", true]]);
assert.equal(
  createAiOpenMenuInvocation("AI_OPEN_MENU_LIST", { menuID: 7, aiSessionIdentifier: "other" }, { id: 7, actionID: 99 }, "sid"),
  null
);
assert.equal(createAiOpenMenuInvocation("AI_OPEN_MENU_LIST", { menuID: 7 }, { id: 7 }, "sid"), null);

const pivotInvocation = createAiOpenMenuInvocation(
  "AI_OPEN_MENU_PIVOT",
  {
    menuID: 8,
    selectedFilters: ["confirmed"],
    rowGroupBys: ["partner_id"],
    colGroupBys: ["date:month"],
    measures: ["amount_total"],
    search: [],
    sortedColumn: { measure: "amount_total", order: "desc" }
  },
  { id: 8, actionId: 101 }
);
assert.equal(pivotInvocation.options.viewType, "pivot");
assert.deepEqual(pivotInvocation.options.props.ai.selectedGroupBys, ["partner_id"]);
assert.deepEqual(pivotInvocation.options.props.ai.sortedColumn, { measure: "amount_total", order: "desc" });

const adjustEvents = createAiAdjustSearchEvents({
  removeFacets: ["old"],
  toggleFilters: ["draft"],
  toggleGroupBys: ["stage_id"],
  applySearches: ["name=Ada"],
  measures: ["__count"],
  switchViewType: "kanban",
  customDomain: [["amount_total", ">=", 500]],
  aiSessionIdentifier: "sid"
}, "sid");
assert.equal(adjustEvents.length, 1);
assert.equal(adjustEvents[0].name, "APPLY_AI_ADJUST_SEARCH");
assert.equal(adjustEvents[0].detail.switchViewType, "kanban");
assert.equal(adjustEvents[0].detail.order, "ASC");
assert.deepEqual(adjustEvents[0].detail.customDomain, [["amount_total", ">=", 500]]);
assert.deepEqual(createAiAdjustSearchEvents({ aiSessionIdentifier: "other" }, "sid"), []);
const modelEvents = createAiAdjustModelEvents({
  measures: ["amount_total"],
  mode: "bar",
  order: "DESC",
  stacked: true,
  cumulated: false
});
assert.equal(modelEvents[0].name, "APPLY_AI_ADJUST_MODEL");
assert.deepEqual(modelEvents[0].detail.measures, ["amount_total"]);
assert.equal(modelEvents[0].detail.mode, "bar");
assert.equal(modelEvents[0].detail.order, "DESC");
