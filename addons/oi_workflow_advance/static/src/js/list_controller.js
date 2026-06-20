/** @odoo-module **/

// Advanced workflow flowchart view routing.
import { ListController } from "@web/views/list/list_controller";
import { patch } from "@web/core/utils/patch";
import { ConfirmationDialog } from "@web/core/confirmation_dialog/confirmation_dialog";
import { _t } from "@web/core/l10n/translation";
import { uniqueId } from "@web/core/utils/functions";

export class SelectViewDialog extends ConfirmationDialog {
    static template = "oi_workflow_advance.confirm_view";
    static props = {
        ...ConfirmationDialog.props,
        multi_workflow_view: Object,
        input_name: String,
    };
    static defaultProps = {
        ...ConfirmationDialog.defaultProps,
        title: _t("Select View"),
    };
}

patch(ListController.prototype, {
    async createRecord({ group } = {}) {
        if (!this.editable) {
            const currentAction = this.actionService.currentController.action;
            if (currentAction.multi_workflow_view) {
                const multiWorkflowView = JSON.parse(currentAction.multi_workflow_view);
                const inputName = uniqueId("workflow-name");
                await this.dialogService.add(SelectViewDialog, {
                    multi_workflow_view: multiWorkflowView,
                    input_name: inputName,
                    body: "body",
                    confirm: () => {
                        const value = document.querySelector(`input[name="${inputName}"]:checked`)?.dataset.value;
                        const workflowView = multiWorkflowView.find((view) => String(view.id) === String(value));
                        if (!workflowView) return;
                        this.actionService.doAction(
                            {
                                type: "ir.actions.act_window",
                                res_model: currentAction.res_model,
                                res_id: undefined,
                                target: "current",
                                views: [[workflowView.view_id, "form"]],
                            },
                            { additional_context: { ...workflowView.create_context } }
                        );
                    },
                    cancel: () => {},
                });
                return;
            }
        }
        return super.createRecord(...arguments);
    },

    async openRecord(record) {
        const workflowViewId = record.data.workflow_view_id;
        if (workflowViewId) {
            await this.actionService.doAction({
                type: "ir.actions.act_window",
                res_model: record.resModel,
                res_id: record.resId,
                target: "current",
                views: [[workflowViewId[0], "form"]],
            });
            return;
        }
        return super.openRecord(...arguments);
    },
});
