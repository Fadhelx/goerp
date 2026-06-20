/** @odoo-module **/

import { Component } from "@odoo/owl";
import { browser } from "@web/core/browser/browser";
import { _t } from "@web/core/l10n/translation";
import { registry } from "@web/core/registry";
import { session } from "@web/session";
import { user } from "@web/core/user";

export class LoginAsSystray extends Component {
    static template = "oi_login_as.LoginAs";

    get loginAs() {
        return session.login_as || {};
    }

    get isImpersonating() {
        return Boolean(
            this.loginAs.active ||
                session.impersonate ||
                session.login_as_user_id ||
                session.login_as_original_uid
        );
    }

    get redirect() {
        return browser.location.pathname + browser.location.search + browser.location.hash;
    }

    onClick() {
        if (this.isImpersonating) {
            return this.env.services.action.doAction({
                type: "ir.actions.act_url",
                url: `/web/login_back?redirect=${encodeURIComponent(this.redirect)}`,
                target: "self",
            });
        }
        return this.env.services.action.doAction({
            type: "ir.actions.act_window",
            name: _t("Login As"),
            res_model: "login.as",
            views: [[false, "form"]],
            target: "new",
        });
    }
}

registry.category("systray").add(
    "oi_login_as.LoginAsSystray",
    {
        Component: LoginAsSystray,
        isDisplayed: (env) => (env.debug && user.isSystem) || Boolean(session.login_as?.active || session.impersonate),
    },
    { sequence: 10 }
);
