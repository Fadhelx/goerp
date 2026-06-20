export type WorkflowRecordId = number | string;
export type ApprovalButtonKind =
  | "approve"
  | "reject"
  | "return"
  | "forward"
  | "transfer"
  | "cancel"
  | "reset"
  | "update_status"
  | "approval_log"
  | "custom";
export type WorkflowWizardAction =
  | "approve"
  | "reject"
  | "return"
  | "forward"
  | "transfer"
  | "cancel"
  | "cancel_workflow"
  | "reset_to_draft"
  | "update_status";
export type WorkflowViewType = "form" | "list";

export interface ApprovalRecordState {
  id?: WorkflowRecordId;
  model?: string;
  resModel?: string;
  userCanApprove?: boolean;
  approvalState?: string;
  approvedButtonClicked?: unknown;
  selectedIds?: readonly WorkflowRecordId[];
  workflowStates?: readonly string[];
  activeFields?: readonly string[] | Record<string, unknown>;
  fields?: Record<string, { related?: boolean } | undefined>;
}

export interface ApprovalButtonInput {
  name: string;
  label?: string;
  kind?: ApprovalButtonKind;
  buttonId?: WorkflowRecordId;
  transitionId?: WorkflowRecordId;
  visible?: boolean;
  disabled?: boolean;
  requiresApproval?: boolean;
  requiresSelection?: boolean;
  selectedIds?: readonly WorkflowRecordId[];
  args?: string;
  approvedButtonClicked?: unknown;
  confirm?: string;
  context?: Record<string, unknown>;
}

export interface WorkflowActionDescriptor {
  type: "object-method" | "window" | "wizard";
  name: string;
  model?: string;
  resModel?: string;
  recordIds: WorkflowRecordId[];
  params: Record<string, unknown>;
  confirm?: string;
}

export interface WorkflowCallButtonRequest {
  model: string;
  method: string;
  args: unknown[];
  kwargs: Record<string, unknown>;
}

export interface WorkflowDatasetService {
  callButton<T = unknown>(
    model: string,
    method: string,
    args?: readonly unknown[],
    kwargs?: Record<string, unknown>
  ): Promise<T>;
}

export interface WorkflowActionService {
  doAction<T = unknown>(action: Record<string, unknown>, options?: Record<string, unknown>): Promise<T>;
}

export interface WorkflowExecutionServices {
  dataset: WorkflowDatasetService;
  action: WorkflowActionService;
}

export interface WorkflowExecutionOptions {
  actionOptions?: Record<string, unknown>;
  refresh?: () => unknown | Promise<unknown>;
}

export interface ApprovalButtonState {
  id: string;
  name: string;
  kind: ApprovalButtonKind;
  label: string;
  visible: boolean;
  enabled: boolean;
  disabled: boolean;
  reason?: string;
  clickedValue?: unknown;
  ariaLabel: string;
  action: WorkflowActionDescriptor | null;
}

export interface StatusSelectionInput {
  value: string;
  label: string;
  visible?: boolean;
}

export interface StatusTag {
  value: string;
  label: string;
  active: boolean;
  visible: boolean;
  source: "current" | "workflow" | "selection";
}

export interface ApprovalLogEntryInput {
  id: WorkflowRecordId;
  action?: string;
  status?: string;
  state?: string;
  stateLabel?: string;
  actorId?: WorkflowRecordId;
  actorName?: string;
  date?: string;
  comment?: string;
  sequence?: number;
}

export interface ApprovalLogEntryView {
  id: WorkflowRecordId;
  action: string;
  label: string;
  status: string;
  state?: string;
  stateLabel?: string;
  actorName: string;
  date?: string;
  comment: string;
  sequence: number;
  isCurrentUser: boolean;
}

export interface ApprovalLogViewModel {
  entries: ApprovalLogEntryView[];
  empty: boolean;
  columns: string[];
  summary: Record<string, number>;
}

export interface WorkflowWizardOptions {
  action: WorkflowWizardAction;
  selectedIds?: readonly WorkflowRecordId[];
  comment?: string;
  requireComment?: boolean;
  targetStateId?: WorkflowRecordId;
  targetUserId?: WorkflowRecordId;
  targetUserIds?: readonly WorkflowRecordId[];
  loading?: boolean;
  context?: Record<string, unknown>;
}

export interface WorkflowWizardState {
  action: WorkflowWizardAction;
  selectedIds: WorkflowRecordId[];
  comment: string;
  requireComment: boolean;
  targetStateId?: WorkflowRecordId;
  targetUserId?: WorkflowRecordId;
  targetUserIds: WorkflowRecordId[];
  loading: boolean;
  context: Record<string, unknown>;
  issues: string[];
  canSubmit: boolean;
}

export interface WorkflowFieldDescriptor {
  name: string;
  type: "boolean" | "char" | "many2many" | "many2one" | "selection";
  readonly?: boolean;
  relation?: string;
  fields?: Record<string, unknown>;
}

export interface WorkflowToolbarActionDescriptor {
  id: string;
  label: string;
  kind: ApprovalButtonKind;
  viewTypes: WorkflowViewType[];
  requiresSelection: boolean;
}

export interface WorkflowMetadataInjectionDescriptor {
  viewType: WorkflowViewType;
  fields: WorkflowFieldDescriptor[];
  buttonClickParams: string[];
  toolbarActions: WorkflowToolbarActionDescriptor[];
  loadRelations: Record<string, { fields: Record<string, unknown> }>;
}

export const workflowButtonClickParams = ["approved_button_clicked"] as const;

export const workflowInjectedFields: WorkflowFieldDescriptor[] = [
  { name: "user_can_approve", type: "boolean", readonly: true },
  { name: "approved_button_clicked", type: "boolean", readonly: false },
  { name: "workflow_states", type: "selection", readonly: true },
  { name: "approval_user_ids", type: "many2many", relation: "res.users", fields: { display_name: {} } },
  { name: "approval_done_user_ids", type: "many2many", relation: "res.users", fields: { display_name: {} } },
  { name: "workflow_view_id", type: "many2one", relation: "ir.ui.view", readonly: true }
];

export function createApprovalButtonState(
  record: ApprovalRecordState,
  input: ApprovalButtonInput
): ApprovalButtonState {
  const name = requireText(input.name, "approval button name");
  const kind = input.kind ?? inferApprovalButtonKind(name);
  const label = nonEmpty(input.label) ?? titleize(kind);
  const selectedIds = [...(input.selectedIds ?? record.selectedIds ?? recordIdList(record.id))];
  const requiresApproval = input.requiresApproval ?? isApprovalTransition(kind, name);
  const requiresSelection = input.requiresSelection ?? false;
  const clickedValue = resolveClickedValue(input);
  const model = record.resModel ?? record.model;

  let visible = input.visible !== false;
  let reason = "";
  if (requiresApproval && record.userCanApprove === false) {
    visible = false;
    reason = "user cannot approve";
  }
  if (requiresSelection && selectedIds.length === 0) {
    reason = "no records selected";
  }
  if (input.disabled) {
    reason = "disabled";
  }

  const disabled = Boolean(input.disabled) || !visible || (requiresSelection && selectedIds.length === 0);
  const action = visible
    ? createApprovalActionDescriptor(kind, name, selectedIds, model, clickedValue, input)
    : null;
  const ariaLabel = [label, visible ? undefined : "hidden", disabled ? "disabled" : "enabled", reason]
    .filter(Boolean)
    .join(", ");

  return {
    id: name,
    name,
    kind,
    label,
    visible,
    enabled: !disabled,
    disabled,
    ...(reason ? { reason } : {}),
    ...(clickedValue !== undefined ? { clickedValue } : {}),
    ariaLabel,
    action
  };
}

export function createApprovalButtonStates(
  record: ApprovalRecordState,
  buttons: readonly ApprovalButtonInput[]
): ApprovalButtonState[] {
  return buttons.map((button) => createApprovalButtonState(record, button));
}

export function resolveApprovedButtonUpdate(
  record: ApprovalRecordState,
  input: ApprovalButtonInput
): { approved_button_clicked: unknown } | null {
  if (!hasField(record.activeFields, "approved_button_clicked")) return null;
  const clickedValue = resolveClickedValue(input);
  if (clickedValue === undefined) return null;
  return { approved_button_clicked: clickedValue };
}

export function createStatusTags(
  selection: readonly (readonly [string, string] | StatusSelectionInput)[],
  currentValue: string,
  options: { visibleValues?: readonly string[]; workflowStates?: readonly string[] } = {}
): StatusTag[] {
  const visibleValues = new Set(options.visibleValues ?? []);
  const workflowStates = new Set(options.workflowStates ?? []);
  return selection.map((entry) => {
    let tag: { value: string; label: string; visible: boolean };
    if (Array.isArray(entry)) {
      tag = tupleStatusTag(entry);
    } else {
      const status = entry as StatusSelectionInput;
      tag = {
        value: requireText(status.value, "status value"),
        label: requireText(status.label, "status label"),
        visible: Boolean(status.visible)
      };
    }
    const active = tag.value === currentValue;
    const visible = active || tag.visible || visibleValues.has(tag.value) || workflowStates.has(tag.value);
    const source = active ? "current" : workflowStates.has(tag.value) ? "workflow" : "selection";
    return { value: tag.value, label: tag.label, active, visible, source };
  });
}

function tupleStatusTag(entry: readonly unknown[]): { value: string; label: string; visible: boolean } {
  return { value: String(entry[0]), label: String(entry[1]), visible: false };
}

export function createApprovalLogViewModel(
  entries: readonly ApprovalLogEntryInput[],
  options: { currentUserId?: WorkflowRecordId; columns?: readonly string[] } = {}
): ApprovalLogViewModel {
  const normalized = entries
    .map((entry, index) => normalizeApprovalLogEntry(entry, index, options.currentUserId))
    .sort((a, b) => a.sequence - b.sequence || String(a.date ?? "").localeCompare(String(b.date ?? "")));
  const summary: Record<string, number> = {};
  for (const entry of normalized) summary[entry.status] = (summary[entry.status] ?? 0) + 1;
  return {
    entries: normalized,
    empty: normalized.length === 0,
    columns: [...(options.columns ?? ["date", "actor", "action", "state", "comment"])],
    summary
  };
}

export function createWorkflowWizardState(options: WorkflowWizardOptions): WorkflowWizardState {
  const selectedIds = [...(options.selectedIds ?? [])];
  const targetUserIds = [...(options.targetUserIds ?? [])];
  const comment = options.comment ?? "";
  const requireComment = Boolean(options.requireComment);
  const loading = Boolean(options.loading);
  const issues: string[] = [];

  if (selectedIds.length === 0) issues.push("select at least one record");
  if (requireComment && !nonEmpty(comment)) issues.push("comment is required");
  if (options.action === "update_status" && options.targetStateId === undefined) {
    issues.push("target state is required");
  }
  if ((options.action === "forward" || options.action === "transfer") && options.targetUserId === undefined && targetUserIds.length === 0) {
    issues.push("target user is required");
  }
  if (loading) issues.push("request is pending");

  return {
    action: options.action,
    selectedIds,
    comment,
    requireComment,
    ...(options.targetStateId !== undefined ? { targetStateId: options.targetStateId } : {}),
    ...(options.targetUserId !== undefined ? { targetUserId: options.targetUserId } : {}),
    targetUserIds,
    loading,
    context: { ...(options.context ?? {}) },
    issues,
    canSubmit: issues.length === 0
  };
}

export function updateWorkflowWizardState(
  state: WorkflowWizardState,
  patch: Partial<WorkflowWizardOptions>
): WorkflowWizardState {
  return createWorkflowWizardState({
    action: patch.action ?? state.action,
    selectedIds: patch.selectedIds ?? state.selectedIds,
    comment: patch.comment ?? state.comment,
    requireComment: patch.requireComment ?? state.requireComment,
    targetStateId: patch.targetStateId ?? state.targetStateId,
    targetUserId: patch.targetUserId ?? state.targetUserId,
    targetUserIds: patch.targetUserIds ?? state.targetUserIds,
    loading: patch.loading ?? state.loading,
    context: patch.context ?? state.context
  });
}

export function createWorkflowWizardAction(state: WorkflowWizardState): WorkflowActionDescriptor {
  if (!state.canSubmit) {
    throw new Error(`workflow wizard invalid: ${state.issues.join(", ")}`);
  }
  return {
    type: "wizard",
    name: state.action,
    recordIds: state.selectedIds,
    params: {
      comment: state.comment,
      ...(state.targetStateId !== undefined ? { target_state_id: state.targetStateId } : {}),
      ...(state.targetUserId !== undefined ? { target_user_id: state.targetUserId } : {}),
      ...(state.targetUserIds.length ? { target_user_ids: state.targetUserIds } : {}),
      context: state.context
    }
  };
}

export function createWorkflowMetadataInjection(
  viewType: WorkflowViewType,
  options: { showActionApproveAll?: boolean; includeAdvanceViewFields?: boolean } = {}
): WorkflowMetadataInjectionDescriptor {
  const toolbarActions: WorkflowToolbarActionDescriptor[] = [
    { id: "approval_log", label: "Approval Log", kind: "approval_log", viewTypes: ["form", "list"], requiresSelection: false },
    { id: "update_status", label: "Update Status", kind: "update_status", viewTypes: ["form", "list"], requiresSelection: true }
  ];
  if (viewType === "list" && options.showActionApproveAll) {
    toolbarActions.unshift({ id: "approve", label: "Approve", kind: "approve", viewTypes: ["list"], requiresSelection: true });
  }

  const fields = options.includeAdvanceViewFields
    ? workflowInjectedFields
    : workflowInjectedFields.filter((field) => field.name !== "workflow_view_id");

  return {
    viewType,
    fields: fields.map((field) => ({ ...field })),
    buttonClickParams: [...workflowButtonClickParams],
    toolbarActions,
    loadRelations: {
      approval_user_ids: { fields: { display_name: {} } },
      approval_done_user_ids: { fields: { display_name: {} } }
    }
  };
}

export function isApprovalLogAvailable(record: ApprovalRecordState): boolean {
  const field = record.fields?.user_can_approve;
  if (!field) return hasField(record.activeFields, "user_can_approve");
  return field.related !== true;
}

export async function executeWorkflowAction<T = unknown>(
  descriptor: WorkflowActionDescriptor,
  services: WorkflowExecutionServices,
  options: WorkflowExecutionOptions = {}
): Promise<T> {
  if (descriptor.type === "window" || descriptor.type === "wizard") {
    return services.action.doAction<T>(createWorkflowWindowAction(descriptor), options.actionOptions);
  }
  const request = workflowActionToCallButtonRequest(descriptor);
  const result = await services.dataset.callButton<T>(
    request.model,
    request.method,
    request.args,
    request.kwargs
  );
  if (isActionResult(result)) {
    await services.action.doAction(result as Record<string, unknown>, options.actionOptions);
  }
  if (options.refresh) await options.refresh();
  return result;
}

export function workflowActionToCallButtonRequest(
  descriptor: WorkflowActionDescriptor
): WorkflowCallButtonRequest {
  if (descriptor.type !== "object-method") {
    throw new Error(`workflow action ${descriptor.name} is not an object method`);
  }
  const model = descriptor.model ?? descriptor.resModel;
  if (!model) throw new Error(`workflow action ${descriptor.name} requires a model`);
  const kwargs = normalizeKwargs(descriptor.params);

  if (descriptor.name === "approval_action_button") {
    return {
      model,
      method: "approval_action_button",
      args: [descriptor.recordIds, requiredRecordId(descriptor, ["button_id", "approval_button_id"])],
      kwargs
    };
  }
  if (descriptor.name === "approval_transition_button") {
    return {
      model,
      method: "approval_transition_button",
      args: [descriptor.recordIds, requiredRecordId(descriptor, ["transition_id", "workflow_transition_id"])],
      kwargs
    };
  }
  if (descriptor.name === "action_approve_all") {
    return {
      model,
      method: "action_approve_all",
      args: [descriptor.recordIds],
      kwargs
    };
  }
  return {
    model,
    method: descriptor.name,
    args: [descriptor.recordIds],
    kwargs
  };
}

export function createWorkflowWindowAction(descriptor: WorkflowActionDescriptor): Record<string, unknown> {
  if (descriptor.type === "window" && descriptor.name === "approval_log") {
    return {
      name: "Approval Log",
      res_model: descriptor.resModel ?? "approval.log",
      type: "ir.actions.act_window",
      views: [[false, "list"]],
      view_mode: "list",
      domain: [
        ["model", "=", descriptor.params.res_model],
        ["record_id", "in", descriptor.recordIds]
      ],
      context: {
        hide_record: descriptor.recordIds.length === 1,
        hide_model: true
      }
    };
  }
  if (descriptor.type === "wizard" && descriptor.name === "update_status") {
    return {
      name: "Change Document Status",
      res_model: descriptor.resModel ?? "approval.state.update",
      type: "ir.actions.act_window",
      views: [[false, "form"]],
      view_mode: "form",
      target: "new",
      context: {
        default_res_model: descriptor.params.active_model,
        default_res_ids: descriptor.recordIds
      }
    };
  }
  return {
    type: "ir.actions.act_window",
    name: titleize(descriptor.name),
    ...(descriptor.resModel ? { res_model: descriptor.resModel } : {}),
    target: descriptor.type === "wizard" ? "new" : "current",
    context: { ...descriptor.params }
  };
}

function createApprovalActionDescriptor(
  kind: ApprovalButtonKind,
  name: string,
  selectedIds: WorkflowRecordId[],
  model: string | undefined,
  clickedValue: unknown,
  input: ApprovalButtonInput
): WorkflowActionDescriptor {
  if (kind === "approval_log") {
    return {
      type: "window",
      name: "approval_log",
      resModel: "approval.log",
      recordIds: selectedIds,
      params: { res_model: model, res_ids: selectedIds }
    };
  }
  if (kind === "update_status") {
    return {
      type: "wizard",
      name: "update_status",
      resModel: "approval.state.update",
      recordIds: selectedIds,
      params: { active_model: model, active_ids: selectedIds }
    };
  }
  const parsedArgs = parseJsonArray(input.args);
  const buttonId = input.buttonId ?? (name === "approval_action_button" ? parsedArgs[0] : undefined);
  const transitionId = input.transitionId ?? (name === "approval_transition_button" ? parsedArgs[0] : undefined);
  return {
    type: "object-method",
    name,
    model,
    recordIds: selectedIds,
    params: {
      ...(clickedValue !== undefined ? { approved_button_clicked: clickedValue } : {}),
      ...(buttonId !== undefined ? { button_id: buttonId } : {}),
      ...(transitionId !== undefined ? { transition_id: transitionId } : {}),
      ...(input.context ? { context: { ...input.context } } : {})
    },
    ...(input.confirm ? { confirm: input.confirm } : {})
  };
}

function normalizeApprovalLogEntry(
  entry: ApprovalLogEntryInput,
  index: number,
  currentUserId: WorkflowRecordId | undefined
): ApprovalLogEntryView {
  const action = nonEmpty(entry.action) ?? "pending";
  const status = nonEmpty(entry.status) ?? statusFromAction(action);
  const comment = entry.comment ?? "";
  const actorName = nonEmpty(entry.actorName) ?? "Unknown";
  return {
    id: entry.id,
    action,
    label: titleize(action),
    status,
    ...(entry.state ? { state: entry.state } : {}),
    ...(entry.stateLabel ? { stateLabel: entry.stateLabel } : {}),
    actorName,
    ...(entry.date ? { date: entry.date } : {}),
    comment,
    sequence: entry.sequence ?? index,
    isCurrentUser: currentUserId !== undefined && entry.actorId === currentUserId
  };
}

function statusFromAction(action: string): string {
  const normalized = action.toLowerCase();
  if (normalized.includes("reject")) return "rejected";
  if (normalized.includes("return")) return "returned";
  if (normalized.includes("forward")) return "forwarded";
  if (normalized.includes("approve")) return "approved";
  if (normalized.includes("cancel")) return "cancelled";
  return "pending";
}

function inferApprovalButtonKind(name: string): ApprovalButtonKind {
  const normalized = name.toLowerCase();
  if (normalized.includes("approval_log")) return "approval_log";
  if (normalized.includes("update_status")) return "update_status";
  if (normalized.includes("reject")) return "reject";
  if (normalized.includes("return")) return "return";
  if (normalized.includes("forward")) return "forward";
  if (normalized.includes("transfer")) return "transfer";
  if (normalized.includes("cancel")) return "cancel";
  if (normalized.includes("reset") || normalized.includes("draft")) return "reset";
  if (normalized.includes("approve") || normalized.startsWith("approval")) return "approve";
  return "custom";
}

function isApprovalTransition(kind: ApprovalButtonKind, name: string): boolean {
  return kind !== "approval_log" && kind !== "update_status" && (kind !== "custom" || name.startsWith("approval"));
}

function resolveClickedValue(input: ApprovalButtonInput): unknown {
  if (input.approvedButtonClicked !== undefined) return input.approvedButtonClicked;
  if (!input.name.startsWith("approval")) return undefined;
  const parsed = parseJsonArray(input.args);
  return parsed.length ? parsed[0] : true;
}

function parseJsonArray(value: string | undefined): unknown[] {
  if (!value) return [];
  try {
    const parsed = JSON.parse(value);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

function normalizeKwargs(params: Record<string, unknown>): Record<string, unknown> {
  const kwargs = { ...(isRecord(params.context) ? params.context : {}) };
  for (const [key, value] of Object.entries(params)) {
    if (key === "context" || key === "button_id" || key === "approval_button_id" || key === "transition_id" || key === "workflow_transition_id" || key === "approved_button_clicked") continue;
    kwargs[key] = value;
  }
  return kwargs;
}

function requiredRecordId(
  descriptor: WorkflowActionDescriptor,
  keys: readonly string[]
): WorkflowRecordId {
  for (const key of keys) {
    const value = descriptor.params[key];
    if (isWorkflowRecordId(value)) return value;
  }
  if (descriptor.name === "approval_action_button" && isWorkflowRecordId(descriptor.params.approved_button_clicked)) {
    return descriptor.params.approved_button_clicked;
  }
  throw new Error(`${descriptor.name} requires ${keys[0]}`);
}

function isWorkflowRecordId(value: unknown): value is WorkflowRecordId {
  return (typeof value === "number" && Number.isFinite(value)) || (typeof value === "string" && value.trim() !== "");
}

function isActionResult(value: unknown): value is Record<string, unknown> {
  return isRecord(value) && typeof value.type === "string";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function recordIdList(id: WorkflowRecordId | undefined): WorkflowRecordId[] {
  return id === undefined ? [] : [id];
}

function hasField(fields: ApprovalRecordState["activeFields"], name: string): boolean {
  if (!fields) return false;
  return Array.isArray(fields) ? fields.includes(name) : Object.prototype.hasOwnProperty.call(fields, name);
}

function requireText(value: string, label: string): string {
  const text = nonEmpty(value);
  if (!text) throw new Error(`${label} is required`);
  return text;
}

function nonEmpty(value: unknown): string | undefined {
  const text = typeof value === "string" ? value.trim() : "";
  return text ? text : undefined;
}

function titleize(value: string): string {
  return value
    .split(/[_\s-]+/)
    .filter(Boolean)
    .map((part) => part.slice(0, 1).toUpperCase() + part.slice(1).toLowerCase())
    .join(" ");
}
