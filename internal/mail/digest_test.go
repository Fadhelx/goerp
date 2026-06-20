package mail

import (
	"strings"
	"testing"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

func TestSendDigestsCreatesMailConsumesTipAndUpdatesNextRun(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	companyPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Digest Co", "email": "digest.co@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Digest Co", "partner_id": companyPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	groupID, err := env.Model("res.groups").Create(map[string]any{"name": "Digest Users"})
	if err != nil {
		t.Fatal(err)
	}
	userPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Digest User", "email": "digest.user@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{
		"name":          "Digest User",
		"login":         "digest.user@example.com",
		"email":         "digest.user@example.com",
		"partner_id":    userPartnerID,
		"company_id":    companyID,
		"groups_id":     []int64{groupID},
		"all_group_ids": []int64{groupID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "digest-secret"}); err != nil {
		t.Fatal(err)
	}
	tipID, err := env.Model("digest.tip").Create(map[string]any{"name": "First Tip", "sequence": int64(1), "group_id": groupID, "tip_description": `<p>First Tip</p>`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mail.message").Create(map[string]any{"message_type": "email", "create_date": now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	digestID, err := env.Model("digest.digest").Create(map[string]any{
		"name":                   "Daily Digest",
		"active":                 true,
		"state":                  "activated",
		"periodicity":            "daily",
		"next_run_date":          "2026-06-20",
		"user_ids":               []int64{userID},
		"company_id":             companyID,
		"kpi_mail_message_total": true,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := SendDigests(env, []int64{digestID}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || result.SlowedDown != 0 || len(result.MailIDs) != 1 || len(result.DigestIDs) != 1 || result.DigestIDs[0] != digestID {
		t.Fatalf("digest result = %+v", result)
	}
	mailRows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("state", "auto_delete", "email_from", "email_to", "subject", "body_html", "headers")
	if err != nil {
		t.Fatal(err)
	}
	if len(mailRows) != 1 {
		t.Fatalf("mail rows = %+v", mailRows)
	}
	if mailRows[0]["state"] != "outgoing" || mailRows[0]["auto_delete"] != true || mailRows[0]["email_from"] != "Digest Co <digest.co@example.com>" || mailRows[0]["email_to"] != "Digest User <digest.user@example.com>" || mailRows[0]["subject"] != "Digest Co: Daily Digest" {
		t.Fatalf("mail row = %+v", mailRows[0])
	}
	body := stringAny(mailRows[0]["body_html"])
	for _, fragment := range []string{"o_digest_mail_main", "Daily Digest", "First Tip", "Messages Sent", "Unsubscribe"} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("digest body missing %q: %s", fragment, body)
		}
	}
	headers := stringAny(mailRows[0]["headers"])
	for _, fragment := range []string{"List-Unsubscribe", "unsubscribe_oneclik", "List-Unsubscribe=One-Click", "X-Auto-Response-Suppress"} {
		if !strings.Contains(headers, fragment) {
			t.Fatalf("digest headers missing %q: %s", fragment, headers)
		}
	}
	digestRows, err := env.Model("digest.digest").Browse(digestID).Read("periodicity", "next_run_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(digestRows) != 1 || digestRows[0]["periodicity"] != "daily" || timeValue(digestRows[0]["next_run_date"]).Format("2006-01-02") != "2026-06-21" {
		t.Fatalf("digest rows = %+v", digestRows)
	}
	tipRows, err := env.Model("digest.tip").Browse(tipID).Read("user_ids")
	if err != nil {
		t.Fatal(err)
	}
	if len(tipRows) != 1 || !containsInt64(int64s(tipRows[0]["user_ids"]), userID) {
		t.Fatalf("tip rows = %+v", tipRows)
	}
}

func TestSendDigestsRendersEnabledKPIsMarginsAndConnectedUsers(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	companyID, userID := digestTestCompanyAndUser(t, env)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "digest-secret"}); err != nil {
		t.Fatal(err)
	}
	if err := env.Model("res.users").Browse(userID).Write(map[string]any{
		"login_date":  now.Add(-time.Hour),
		"company_ids": []int64{companyID},
	}); err != nil {
		t.Fatal(err)
	}
	currentPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Current User", "email": "current.user@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{
		"name":        "Current User",
		"login":       "current.user@example.com",
		"email":       "current.user@example.com",
		"partner_id":  currentPartnerID,
		"company_id":  companyID,
		"company_ids": []int64{companyID},
		"login_date":  now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	previousPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Previous User", "email": "previous.user@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("res.users").Create(map[string]any{
		"name":        "Previous User",
		"login":       "previous.user@example.com",
		"email":       "previous.user@example.com",
		"partner_id":  previousPartnerID,
		"company_id":  companyID,
		"company_ids": []int64{companyID},
		"login_date":  now.Add(-25 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := env.Model("mail.message").Create(map[string]any{"message_type": "email"}); err != nil {
			t.Fatal(err)
		}
	}
	digestID := digestTestRow(t, env, "KPI Digest", companyID, userID, map[string]any{
		"kpi_mail_message_total":  true,
		"kpi_res_users_connected": true,
	})

	result, err := SendDigests(env, []int64{digestID}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("body_html")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("mail rows = %+v", rows)
	}
	body := stringAny(rows[0]["body_html"])
	for _, fragment := range []string{
		`data-field="kpi_mail_message_total"`,
		`data-field="kpi_res_users_connected"`,
		"Messages Sent",
		"Connected Users",
		">2</span>",
		"100.00 %",
		"Last 24 hours",
		"Last 7 Days",
		"Last 30 Days",
		"Odoo Mobile",
		"Powered by",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("digest KPI body missing %q: %s", fragment, body)
		}
	}
}

func TestSendDigestsRendersTipTemplateBeforeConsuming(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	companyID, userID := digestTestCompanyAndUser(t, env)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "database.secret", "value": "digest-secret"}); err != nil {
		t.Fatal(err)
	}
	tipID, err := env.Model("digest.tip").Create(map[string]any{
		"name": "Rendered Tip",
		"tip_description": `<t t-set="record_exists" t-value="True" />
                <t t-if="record_exists">
                    <p class="rendered">Record exists.</p>
                </t>
                <t t-else="">
                    <p class="not-rendered">Record does not exist.</p>
                </t>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	digestID := digestTestRow(t, env, "Tip Digest", companyID, userID, nil)

	result, err := SendDigests(env, []int64{digestID}, now, false)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := env.Model("mail.mail").Browse(result.MailIDs...).Read("body_html")
	if err != nil {
		t.Fatal(err)
	}
	body := stringAny(rows[0]["body_html"])
	if !strings.Contains(body, `<p class="rendered">Record exists.</p>`) || strings.Contains(body, "not-rendered") || strings.Contains(body, "t-set") {
		t.Fatalf("digest rendered tip body = %s", body)
	}
	tipRows, err := env.Model("digest.tip").Browse(tipID).Read("user_ids")
	if err != nil {
		t.Fatal(err)
	}
	if !containsInt64(int64s(tipRows[0]["user_ids"]), userID) {
		t.Fatalf("tip not consumed = %+v", tipRows)
	}
}

func TestSendDueDigestsFiltersActivatedDueRowsAndSlowsInactiveUsers(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	companyID, userID := digestTestCompanyAndUser(t, env)
	dueID := digestTestRow(t, env, "Due", companyID, userID, map[string]any{"state": "activated", "active": true, "periodicity": "daily", "next_run_date": "2026-06-20"})
	futureID := digestTestRow(t, env, "Future", companyID, userID, map[string]any{"state": "activated", "active": true, "periodicity": "daily", "next_run_date": "2026-06-21"})
	deactivatedID := digestTestRow(t, env, "Deactivated", companyID, userID, map[string]any{"state": "deactivated", "active": true, "periodicity": "daily", "next_run_date": "2026-06-20"})
	inactiveID := digestTestRow(t, env, "Inactive", companyID, userID, map[string]any{"state": "activated", "active": false, "periodicity": "daily", "next_run_date": "2026-06-20"})
	emptyDateID := digestTestRow(t, env, "No Date", companyID, userID, map[string]any{"state": "activated", "active": true, "periodicity": "daily"})

	result, err := SendDueDigests(env, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || result.SlowedDown != 1 || len(result.DigestIDs) != 1 || result.DigestIDs[0] != dueID {
		t.Fatalf("due digest result = %+v", result)
	}
	rows, err := env.Model("digest.digest").Browse(dueID, futureID, deactivatedID, inactiveID, emptyDateID).Read("id", "periodicity", "next_run_date")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]map[string]any{}
	for _, row := range rows {
		byID[int64FromAny(row["id"])] = row
	}
	if byID[dueID]["periodicity"] != "weekly" || timeValue(byID[dueID]["next_run_date"]).Format("2006-01-02") != "2026-06-27" {
		t.Fatalf("due row = %+v", byID[dueID])
	}
	if byID[futureID]["periodicity"] != "daily" || timeValue(byID[futureID]["next_run_date"]).Format("2006-01-02") != "2026-06-21" {
		t.Fatalf("future row = %+v", byID[futureID])
	}
	if byID[deactivatedID]["periodicity"] != "daily" || byID[inactiveID]["periodicity"] != "daily" || byID[emptyDateID]["periodicity"] != "daily" {
		t.Fatalf("skipped rows = %+v", byID)
	}
	mails, err := env.Model("mail.mail").Search(domain.And())
	if err != nil {
		t.Fatal(err)
	}
	if mails.Len() != 1 {
		t.Fatalf("mail count = %d", mails.Len())
	}
}

func TestSendDueDigestsKeepsPeriodicityWithRecentUserLog(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	companyID, userID := digestTestCompanyAndUser(t, env)
	if _, err := env.Model("res.users.log").Create(map[string]any{"create_uid": userID, "create_date": now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	digestID := digestTestRow(t, env, "Recent", companyID, userID, map[string]any{"state": "activated", "active": true, "periodicity": "daily", "next_run_date": "2026-06-20"})

	result, err := SendDueDigests(env, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sent != 1 || result.SlowedDown != 0 || len(result.DigestIDs) != 1 || result.DigestIDs[0] != digestID {
		t.Fatalf("recent digest result = %+v", result)
	}
	rows, err := env.Model("digest.digest").Browse(digestID).Read("periodicity", "next_run_date")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["periodicity"] != "daily" || timeValue(rows[0]["next_run_date"]).Format("2006-01-02") != "2026-06-21" {
		t.Fatalf("recent digest rows = %+v", rows)
	}
}

func digestTestCompanyAndUser(t *testing.T, env *record.Env) (int64, int64) {
	t.Helper()
	companyPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Digest Test Co", "email": "digest.test@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	companyID, err := env.Model("res.company").Create(map[string]any{"name": "Digest Test Co", "partner_id": companyPartnerID})
	if err != nil {
		t.Fatal(err)
	}
	userPartnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Digest Test User", "email": "digest.test.user@example.com", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	userID, err := env.Model("res.users").Create(map[string]any{
		"name":       "Digest Test User",
		"login":      "digest.test.user@example.com",
		"email":      "digest.test.user@example.com",
		"partner_id": userPartnerID,
		"company_id": companyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return companyID, userID
}

func digestTestRow(t *testing.T, env *record.Env, name string, companyID int64, userID int64, overrides map[string]any) int64 {
	t.Helper()
	values := map[string]any{
		"name":        name,
		"active":      true,
		"state":       "activated",
		"periodicity": "daily",
		"user_ids":    []int64{userID},
		"company_id":  companyID,
	}
	for key, value := range overrides {
		values[key] = value
	}
	id, err := env.Model("digest.digest").Create(values)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
