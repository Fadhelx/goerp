/** @odoo-module **/

import { StatusBarField } from "@web/views/fields/statusbar/statusbar_field";
import { patch } from "@web/core/utils/patch";

patch(StatusBarField.prototype, {
    getAllItems() {
        const { visibleSelection, name, record } = this.props;
        if (visibleSelection?.includes("WORKFLOW") && name === "state") {
            const currentValue = record.data[name];
            const workflowStates = record.data.workflow_states;
            if (workflowStates) {
                return this.field.selection
                    .filter(
                        ([value]) =>
                            value === currentValue ||
                            visibleSelection.includes(value) ||
                            workflowStates.includes(value)
                    )
                    .map(([value, label]) => ({
                        value,
                        label,
                        isFolded: false,
                        isSelected: value === currentValue,
                    }));
            }
        }
        return super.getAllItems();
    },
});
