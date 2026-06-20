/** @odoo-module **/

import { FormController } from "@web/views/form/form_controller";
import { patch } from "@web/core/utils/patch";
import { _t } from "@web/core/l10n/translation";
import { evaluateBooleanExpr, evaluateExpr } from "@web/core/py_js/py";
import { user } from "@web/core/user";

patch(FormController.prototype, {
    async beforeExecuteActionButton(clickParams) {
        const validateForm = clickParams.validate_form && evaluateBooleanExpr(clickParams.validate_form);
        if (validateForm === false) return true;

        const record = this.model.root;
        if (clickParams.name?.startsWith("approval") && "approved_button_clicked" in record.activeFields) {
            await record.update({ approved_button_clicked: JSON.parse(clickParams.args || "[true]")[0] });
        }
        if (clickParams.approved_button_clicked !== undefined) {
            await record.update({ approved_button_clicked: evaluateExpr(clickParams.approved_button_clicked) });
        }
        if (validateForm) {
            const valid = await record.checkValidity({ displayNotification: true });
            if (!valid) return false;
        }
        return super.beforeExecuteActionButton(clickParams);
    },

    getStaticActionMenuItems() {
        const items = super.getStaticActionMenuItems();
        Object.assign(items, {
            approval_log: {
                isAvailable: () =>
                    "user_can_approve" in this.model.root.activeFields &&
                    !this.model.root.activeFields.user_can_approve.related,
                sequence: 100,
                icon: "fa fa-arrows-h",
                description: _t("Approval Log"),
                callback: () => {
                    const { resModel, resId } = this.model.root;
                    return this.env.services.action.doAction({
                        name: _t("Approval Log"),
                        res_model: "approval.log",
                        type: "ir.actions.act_window",
                        views: [[false, "list"]],
                        view_mode: "list",
                        domain: [
                            ["model", "=", resModel],
                            ["record_id", "=", resId],
                        ],
                        context: { hide_record: true, hide_model: true },
                    });
                },
            },
            update_status: {
                isAvailable: () => "state" in this.model.root.activeFields && user.isSystem,
                sequence: 100,
                icon: "fa fa-code",
                description: _t("Update Status"),
                callback: async () => {
                    await this.env.services.action.doAction(
                        {
                            name: _t("Change Document Status"),
                            res_model: "approval.state.update",
                            type: "ir.actions.act_window",
                            views: [[false, "form"]],
                            view_mode: "form",
                            target: "new",
                            context: {
                                default_res_model: this.model.root.resModel,
                                default_res_ids: [this.model.root.resId],
                            },
                        },
                        { onClose: () => this.model.root.load() }
                    );
                },
            },
        });
        return items;
    },
});
