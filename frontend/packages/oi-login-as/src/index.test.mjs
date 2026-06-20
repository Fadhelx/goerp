import assert from "node:assert/strict";
import {
  createDebugBecomeSuperuserAction,
  createDebugRouteGate,
  createImpersonationContextFromSession,
  createImpersonationBanner,
  createImpersonationContext,
  createLoginBackAction,
  createSwitchUserAction,
  normalizeLoginAsSessionInfo,
  normalizeRedirect
} from "../../../dist/packages/oi-login-as/src/index.js";

const admin = { id: 1, displayName: "Admin", isSystem: true, active: true };
const employee = { id: 7, displayName: "Employee", active: true };

const context = createImpersonationContext({
  impersonate: true,
  currentUser: employee,
  originalUser: admin,
  effectiveUser: employee,
  redirect: "/web#action=1",
  debug: true,
  userIsSystem: true
});
assert.equal(context.active, true);
assert.equal(context.banner.visible, true);
assert.equal(context.banner.effectiveName, "Employee");
assert.equal(context.backAction.visible, true);
assert.equal(context.backAction.url, "/web/login_back?redirect=%2Fweb%23action%3D1");
assert.equal(context.debugGate.debugMenuVisible, true);
assert.equal(context.debugGate.routeEnabled, false);

assert.deepEqual(createImpersonationBanner({ active: false }), {
  visible: false,
  text: ""
});

const switchAction = createSwitchUserAction(employee, { redirect: "/web", visible: true });
assert.equal(switchAction.enabled, true);
assert.equal(switchAction.url, "/web/login_as/7?redirect=%2Fweb");

const inactive = createSwitchUserAction({ id: 8, displayName: "Inactive", active: false });
assert.equal(inactive.enabled, false);
assert.equal(inactive.reason, "target user is inactive");

const systemTarget = createSwitchUserAction(admin);
assert.equal(systemTarget.enabled, false);
assert.equal(systemTarget.reason, "system user target is blocked");

const back = createLoginBackAction({ active: false });
assert.equal(back.visible, false);
assert.equal(back.enabled, false);

const gate = createDebugRouteGate({
  debug: true,
  userIsSystem: true,
  allowDebugBecomeSuperuser: true
});
assert.equal(gate.routeEnabled, true);
const debugAction = createDebugBecomeSuperuserAction({ redirect: "/web?debug=1", gate });
assert.equal(debugAction.enabled, true);
assert.equal(debugAction.url, "/web/become/debug?redirect=%2Fweb%3Fdebug%3D1");

assert.equal(normalizeRedirect("https://example.com/bad"), "/web");
assert.equal(normalizeRedirect("//evil.test"), "/web");
assert.equal(normalizeRedirect("/web#menu_id=2"), "/web#menu_id=2");

const backendSession = {
  uid: 20,
  is_system: true,
  debug: true,
  login_as: {
    active: true,
    original_uid: 1,
    effective_uid: 20,
    banner: "Impersonating Portal User",
    return_to: "/web#menu_id=1",
    back_route: "/web/login_back"
  },
  user_context: {
    login_as: true,
    login_as_original_uid: 1,
    login_as_back_route: "/web/login_back"
  }
};
const normalized = normalizeLoginAsSessionInfo(backendSession);
assert.equal(normalized.impersonate, true);
assert.equal(normalized.originalUser.id, 1);
assert.equal(normalized.effectiveUser.id, 20);
assert.equal(normalized.effectiveUser.displayName, "Portal User");
assert.equal(normalized.redirect, "/web#menu_id=1");

const sessionContext = createImpersonationContextFromSession(backendSession);
assert.equal(sessionContext.active, true);
assert.equal(sessionContext.backAction.url, "/web/login_back?redirect=%2Fweb%23menu_id%3D1");

const legacySession = normalizeLoginAsSessionInfo({
  uid: 7,
  impersonate: true,
  login_as_original_uid: 1,
  login_as_user_id: 7,
  login_as_return_to: "/web#legacy"
});
assert.equal(legacySession.impersonate, true);
assert.equal(legacySession.effectiveUser.id, 7);
assert.equal(legacySession.redirect, "/web#legacy");
