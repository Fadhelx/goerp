import assert from "node:assert/strict";
import {
  createApprovalButtonState,
  createApprovalLogViewModel,
  createStatusTags,
  createWorkflowWindowAction,
  createWorkflowMetadataInjection,
  createWorkflowWizardAction,
  createWorkflowWizardState,
  executeWorkflowAction,
  isApprovalLogAvailable,
  resolveApprovedButtonUpdate,
  updateWorkflowWizardState,
  workflowActionToCallButtonRequest
} from "../../../dist/packages/oi-workflow/src/index.js";

const record = {
  id: 42,
  model: "purchase.request",
  userCanApprove: true,
  activeFields: ["approved_button_clicked", "user_can_approve"],
  fields: { user_can_approve: { related: false } }
};

const approve = createApprovalButtonState(record, {
  name: "approval_manager",
  args: "[\"manager\"]",
  confirm: "Approve?"
});
assert.equal(approve.visible, true);
assert.equal(approve.enabled, true);
assert.equal(approve.kind, "approve");
assert.equal(approve.clickedValue, "manager");
assert.equal(approve.action?.params.approved_button_clicked, "manager");
assert.deepEqual(resolveApprovedButtonUpdate(record, { name: "approval_manager", args: "[false]" }), {
  approved_button_clicked: false
});

const hidden = createApprovalButtonState({ ...record, userCanApprove: false }, { name: "approval_manager" });
assert.equal(hidden.visible, false);
assert.equal(hidden.action, null);

const approveSelected = createApprovalButtonState(
  { model: "x.model", selectedIds: [] },
  { name: "action_approve_all", kind: "approve", requiresSelection: true }
);
assert.equal(approveSelected.enabled, false);
assert.equal(approveSelected.reason, "no records selected");

const tags = createStatusTags(
  [
    ["draft", "Draft"],
    ["review", "Review"],
    ["approved", "Approved"]
  ],
  "review",
  { workflowStates: ["approved"] }
);
assert.deepEqual(
  tags.filter((tag) => tag.visible).map((tag) => tag.value),
  ["review", "approved"]
);
assert.equal(tags[1].source, "current");
assert.equal(tags[2].source, "workflow");

const log = createApprovalLogViewModel(
  [
    { id: 2, action: "approve", actorId: 7, actorName: "Manager", sequence: 2, comment: "ok" },
    { id: 1, action: "reject", actorId: 8, actorName: "Auditor", sequence: 1 }
  ],
  { currentUserId: 7 }
);
assert.equal(log.entries[0].status, "rejected");
assert.equal(log.entries[1].isCurrentUser, true);
assert.deepEqual(log.summary, { rejected: 1, approved: 1 });

let wizard = createWorkflowWizardState({
  action: "forward",
  selectedIds: [1],
  requireComment: true
});
assert.equal(wizard.canSubmit, false);
assert.match(wizard.issues.join(","), /comment is required/);
assert.match(wizard.issues.join(","), /target user is required/);

wizard = updateWorkflowWizardState(wizard, { comment: "please review", targetUserId: 9 });
const wizardAction = createWorkflowWizardAction(wizard);
assert.equal(wizardAction.type, "wizard");
assert.equal(wizardAction.params.target_user_id, 9);

const formInjection = createWorkflowMetadataInjection("form", { includeAdvanceViewFields: true });
assert.equal(formInjection.buttonClickParams.includes("approved_button_clicked"), true);
assert.equal(formInjection.fields.some((field) => field.name === "workflow_view_id"), true);
assert.equal(formInjection.loadRelations.approval_user_ids.fields.display_name instanceof Object, true);

const listInjection = createWorkflowMetadataInjection("list", { showActionApproveAll: true });
assert.equal(listInjection.toolbarActions[0].id, "approve");
assert.equal(isApprovalLogAvailable(record), true);

const sourceApproval = createApprovalButtonState(record, {
  name: "approval_action_button",
  args: "[77]",
  context: { lang: "en_US" }
});
assert.deepEqual(workflowActionToCallButtonRequest(sourceApproval.action), {
  model: "purchase.request",
  method: "approval_action_button",
  args: [[42], 77],
  kwargs: { lang: "en_US" }
});

const transition = workflowActionToCallButtonRequest({
  type: "object-method",
  name: "approval_transition_button",
  model: "purchase.request",
  recordIds: [42],
  params: { transition_id: 12 }
});
assert.deepEqual(transition, {
  model: "purchase.request",
  method: "approval_transition_button",
  args: [[42], 12],
  kwargs: {}
});

const approveAll = workflowActionToCallButtonRequest({
  type: "object-method",
  name: "action_approve_all",
  model: "purchase.request",
  recordIds: [42, 43],
  params: {}
});
assert.deepEqual(approveAll.args, [[42, 43]]);

const calls = [];
const serviceResult = await executeWorkflowAction(
  {
    type: "object-method",
    name: "approval_action_button",
    model: "purchase.request",
    recordIds: [42],
    params: { button_id: 77 }
  },
  {
    dataset: {
      callButton(model, method, args, kwargs) {
        calls.push({ model, method, args, kwargs });
        return Promise.resolve({ type: "ir.actions.client", tag: "soft_reload" });
      }
    },
    action: {
      doAction(action) {
        calls.push({ action });
        return Promise.resolve(action);
      }
    }
  }
);
assert.equal(serviceResult.tag, "soft_reload");
assert.deepEqual(calls[0], {
  model: "purchase.request",
  method: "approval_action_button",
  args: [[42], 77],
  kwargs: {}
});
assert.equal(calls[1].action.tag, "soft_reload");

const windowAction = createWorkflowWindowAction({
  type: "window",
  name: "approval_log",
  resModel: "approval.log",
  recordIds: [42],
  params: { res_model: "purchase.request" }
});
assert.equal(windowAction.res_model, "approval.log");
assert.deepEqual(windowAction.domain[1], ["record_id", "in", [42]]);
