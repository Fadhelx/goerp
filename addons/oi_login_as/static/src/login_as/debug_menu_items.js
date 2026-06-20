/** @odoo-module **/

import { browser } from "@web/core/browser/browser";
import { _t } from "@web/core/l10n/translation";
import { registry } from "@web/core/registry";
import { user } from "@web/core/user";

function openLoginAsWizard(env) {
    env.services.action.doAction({
        type: "ir.actions.act_window",
        name: "Login As",
        res_model: "login.as",
        views: [[false, "form"]],
        target: "new",
    });
}

export function becomeSuperuser() {
    const redirect = browser.location.pathname + browser.location.search + browser.location.hash;
    const becomeSuperuserURL = `/web/become/debug?redirect=${encodeURIComponent(redirect)}`;
    return {
        type: "item",
        description: _t("Become Superuser"),
        hide: !user.isAdmin,
        href: becomeSuperuserURL,
        callback: () => browser.open(becomeSuperuserURL, "_self"),
        sequence: 560,
        section: "tools",
    };
}

delete registry.category("debug").category("default").content.becomeSuperuser;
registry.category("debug").category("default").add("becomeSuperuser", becomeSuperuser);

registry.category("debug").category("default").add(
    "oi_login_as.open_login_as_wizard",
    (env) => ({
        type: "item",
        description: "Login As",
        callback: () => openLoginAsWizard(env),
        sequence: 10,
    }),
    { sequence: 10 },
);
