/** @odoo-module **/

import { Component } from "@odoo/owl";
import { browser } from "@web/core/browser/browser";
import { _t } from "@web/core/l10n/translation";
import { registry } from "@web/core/registry";
import { session } from "@web/session";
import { user } from "@web/core/user";

export class LoginAs extends Component {
    static props = {};
    static template = "oi_login_as.LoginAs";

    setup() {
    }

    get impersonate() {
        return session.impersonate;
    }

    _onClick() {
        const redirect = browser.location.pathname + browser.location.search + browser.location.hash;
        if (this.impersonate) {
            return this.env.services.action.doAction({
                type: "ir.actions.act_url",
                url: `/web/login_back?redirect=${redirect}`,
                target: "self",
            });
        }
        return this.env.services.action.doAction({
            type: "ir.actions.act_window",
            name: _t("Login as"),
            res_model: "login.as",
            views: [[false, "form"]],
            target: "new",
        });
    }
}

registry.category("systray").add(
    "LoginAsSystrayItem",
    {
        Component: LoginAs,
        isDisplayed: (env) => (env.debug && user.isSystem) || session.impersonate,
    },
    { sequence: 1 }
);
