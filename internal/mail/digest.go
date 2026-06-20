package mail

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type DigestSendResult struct {
	DigestIDs  []int64
	MailIDs    []int64
	Sent       int
	Skipped    int
	SlowedDown int
}

type digestUserInfo struct {
	ID             int64
	Name           string
	Email          string
	EmailFormatted string
	PartnerID      int64
	CompanyID      int64
	GroupIDs       []int64
}

type digestCompanyInfo struct {
	ID             int64
	Name           string
	EmailFormatted string
}

type digestKPIInfo struct {
	Name     string
	FullName string
	Action   string
	Cols     []digestKPIColumn
}

type digestKPIColumn struct {
	Value    string
	Margin   float64
	Subtitle string
}

type digestTimeframe struct {
	Name          string
	Start         time.Time
	End           time.Time
	PreviousStart time.Time
	PreviousEnd   time.Time
}

func SendDigests(env *record.Env, ids []int64, now time.Time, updatePeriodicity bool) (DigestSendResult, error) {
	if env == nil {
		return DigestSendResult{}, fmt.Errorf("digest send requires env")
	}
	now = queueNow(now)
	rows, err := digestRows(env, ids)
	if err != nil {
		return DigestSendResult{}, err
	}
	result := DigestSendResult{}
	for _, row := range rows {
		digestID := int64FromAny(row["id"])
		if digestID == 0 {
			result.Skipped++
			continue
		}
		userIDs := uniqueIDs(int64s(row["user_ids"]))
		slowDown := updatePeriodicity && digestShouldSlowDown(env, userIDs, strings.TrimSpace(stringAny(row["periodicity"])), now)
		for _, userID := range userIDs {
			mailID, sent, err := sendDigestToUser(env, row, userID, now, slowDown)
			if err != nil {
				return result, err
			}
			if sent {
				result.MailIDs = append(result.MailIDs, mailID)
				result.Sent++
			}
		}
		values := map[string]any{"next_run_date": digestNextRunDate(strings.TrimSpace(stringAny(row["periodicity"])), now)}
		if slowDown {
			values["periodicity"] = digestNextPeriodicity(strings.TrimSpace(stringAny(row["periodicity"])))
			values["next_run_date"] = digestNextRunDate(stringAny(values["periodicity"]), now)
			result.SlowedDown++
		}
		if err := messageSystemEnv(env).Model("digest.digest").Browse(digestID).Write(values); err != nil {
			return result, err
		}
		result.DigestIDs = append(result.DigestIDs, digestID)
	}
	return result, nil
}

func SendDueDigests(env *record.Env, now time.Time) (DigestSendResult, error) {
	if env == nil {
		return DigestSendResult{}, fmt.Errorf("digest cron requires env")
	}
	now = queueNow(now)
	found, err := messageSystemEnv(env).Model("digest.digest").Search(domain.And())
	if err != nil {
		return DigestSendResult{}, err
	}
	rows, err := found.Read("id", "state", "active", "next_run_date")
	if err != nil {
		return DigestSendResult{}, err
	}
	today := digestDateOnly(now)
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row["active"] == false || strings.TrimSpace(stringAny(row["state"])) != "activated" {
			continue
		}
		nextRun := timeValue(row["next_run_date"])
		if !nextRun.IsZero() && !digestDateOnly(nextRun).After(today) {
			ids = append(ids, int64FromAny(row["id"]))
		}
	}
	if len(ids) == 0 {
		return DigestSendResult{}, nil
	}
	return SendDigests(env, ids, now, true)
}

func digestRows(env *record.Env, ids []int64) ([]map[string]any, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	systemEnv := messageSystemEnv(env)
	set := systemEnv.Model("digest.digest").Browse(uniqueIDs(ids)...)
	return set.Read("id", "name", "company_id", "periodicity", "next_run_date", "user_ids", "state", "kpi_res_users_connected", "kpi_mail_message_total", "kpi_account_total_revenue")
}

func sendDigestToUser(env *record.Env, digest map[string]any, userID int64, now time.Time, slowDown bool) (int64, bool, error) {
	user, err := digestUser(env, userID)
	if err != nil {
		return 0, false, err
	}
	if user.ID == 0 {
		return 0, false, nil
	}
	company := digestCompany(env, firstNonZero(user.CompanyID, int64FromAny(digest["company_id"]), env.Context().CompanyID))
	tip := digestNextTipHTML(env, user)
	body := digestMailBody(env, digest, user, company, tip, now, slowDown)
	headers, _ := json.Marshal(digestMailHeaders(env, int64FromAny(digest["id"]), userID))
	mailID, err := messageSystemEnv(env).Model("mail.mail").Create(map[string]any{
		"auto_delete": true,
		"author_id":   firstNonZero(currentUserPartnerID(env, env.Context().UserID), user.PartnerID),
		"body_html":   body,
		"email_from":  firstText(company.EmailFormatted, digestCurrentUserEmail(env), user.EmailFormatted),
		"email_to":    user.EmailFormatted,
		"headers":     string(headers),
		"state":       "outgoing",
		"subject":     fmt.Sprintf("%s: %s", firstText(company.Name, "Company"), firstText(digest["name"], "Digest")),
	})
	if err != nil {
		return 0, false, err
	}
	return mailID, true, nil
}

func digestMailBody(env *record.Env, digest map[string]any, user digestUserInfo, company digestCompanyInfo, tip string, now time.Time, slowDown bool) string {
	var b strings.Builder
	digestID := int64FromAny(digest["id"])
	title := firstText(digest["name"], "Digest")
	kpis := digestKPIData(env, digest, user, company, now)
	preferences := digestPreferences(digest, user, slowDown)
	b.WriteString(`<!DOCTYPE html><html><head><meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/><meta name="viewport" content="width=device-width, initial-scale=1.0"/>`)
	b.WriteString(`<style type="text/css">.global_layout{width:588px;margin:0 auto;background-color:#fff;border-left:1px solid #d8dadd;border-right:1px solid #d8dadd}.company_name{display:inline;color:#878d97;font-weight:bold;font-size:24px}.header_title{color:#374151;font-size:18px}.header_date{color:#878d97;font-size:14px}.td_button_connect{background-color:#714B67;border-radius:3px;white-space:nowrap}.button{color:#fff;font-size:16px}.kpi_header{font-size:14px;font-weight:bold;color:#374151}.kpi_cell{width:33%;text-align:center;padding-top:10px}.kpi_value{color:#374151;font-weight:bold;text-decoration:none;font-size:28px}.kpi_value_label{display:inline-block;margin-bottom:10px;color:#878d97;font-size:14px}.positive_kpi_margin{padding:3px 10px;font-size:12px;border-radius:5px;background-color:#c4ecd7;color:#17613a}.negative_kpi_margin{padding:3px 10px;font-size:12px;border-radius:5px;background-color:#f7dddc;color:#712b29}.preference{margin-bottom:15px;color:#374151;font-size:14px}.by_odoo{color:#878d97;font-size:12px}.odoo_link_text{font-weight:bold;color:#714B67}.run_business{color:#374151;margin:15px auto;font-size:18px}#footer{background-color:#F9FAFB;color:#878d97;text-align:center;font-size:20px;border:1px solid #F9FAFB;border-top:0}</style>`)
	b.WriteString(`</head><body><div class="o_mail_wrapper"><div class="o_mail_content o_digest_mail_main">`)
	b.WriteString(`<table cellspacing="0" cellpadding="0" style="width:100%;background-color:#F9FAFB;" align="center"><tbody><tr><td id="header_background" align="center">`)
	b.WriteString(`<table cellspacing="0" cellpadding="0" border="0" id="header" class="global_layout"><tbody>`)
	b.WriteString(`<tr><td style="padding:20px 20px 5px 20px;"><p class="company_name">`)
	b.WriteString(html.EscapeString(firstText(company.Name, "Company")))
	b.WriteString(`</p></td><td align="right" style="padding:20px 20px 5px 0px;"><table><tbody><tr><td class="td_button td_button_connect" style="height:29px;padding:3px 10px;"><a href="/" target="_blank"><span class="button" id="button_connect">Connect</span></a></td></tr></tbody></table></td></tr>`)
	b.WriteString(`<tr><td style="padding-left:20px" colspan="2"><div class="header_title"><p>`)
	b.WriteString(html.EscapeString(title))
	b.WriteString(`</p></div></td></tr><tr><td style="padding:10px 0 20px 20px;" colspan="2"><span class="header_date">`)
	b.WriteString(html.EscapeString(now.Format("January 02, 2006")))
	b.WriteString(`</span></td></tr></tbody></table></td></tr></tbody></table>`)
	b.WriteString(`<table cellspacing="0" cellpadding="0" border="0" style="width:100%;background-color:#F9FAFB;"><tbody><tr><td align="center"><table cellspacing="0" cellpadding="0" border="0" class="global_layout"><tbody>`)
	if strings.TrimSpace(tip) != "" {
		b.WriteString(`<tr><td colspan="3" style="width:100%;padding:20px;border:1px solid #F9FAFB;"><table><tbody><tr><td class="digest_tip">`)
		b.WriteString(tip)
		b.WriteString(`</td></tr></tbody></table></td></tr>`)
	}
	if len(kpis) > 0 {
		b.WriteString(`<tr><td style="padding:20px 20px 0 20px;" class="global_td">`)
		for _, kpi := range kpis {
			b.WriteString(`<table data-field="`)
			b.WriteString(html.EscapeString(kpi.Name))
			b.WriteString(`" cellspacing="0" cellpadding="10" style="width:100%;margin-bottom:5px;"><tbody>`)
			b.WriteString(`<tr class="kpi_header"><td colspan="2" style="padding:0 0 5px 0;"><span style="text-transform:uppercase;">`)
			b.WriteString(html.EscapeString(kpi.FullName))
			b.WriteString(`</span></td>`)
			if strings.TrimSpace(kpi.Action) != "" {
				b.WriteString(`<td align="right" style="padding:0 0 5px 0;"><a href="/odoo/action-`)
				b.WriteString(html.EscapeString(kpi.Action))
				b.WriteString(`"><span id="button_open_report">➔ Open Report</span></a></td>`)
			}
			b.WriteString(`</tr><tr style="vertical-align:top;">`)
			for _, col := range kpi.Cols {
				digestWriteKPICell(&b, col)
			}
			b.WriteString(`</tr></tbody></table>`)
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody><tfoot><tr><td style="padding:20px;border-bottom:1px solid #d8dadd;"><table border="0" width="100%"><tbody><tr style="background-color:#F9FAFB;"><td align="center" colspan="3" valign="center" style="padding:15px;">`)
	for _, preference := range preferences {
		b.WriteString(`<div class="preference">`)
		b.WriteString(preference)
		b.WriteString(`</div>`)
	}
	b.WriteString(`<div class="by_odoo" style="margin-bottom:15px;">Sent by <a href="https://www.odoo.com" target="_blank"><span class="odoo_link_text">Odoo</span></a>`)
	if token := DigestUnsubscribeToken(env, digestID, user.ID); token != "" {
		b.WriteString(` – <a href="/digest/`)
		b.WriteString(fmt.Sprint(digestID))
		b.WriteString(`/unsubscribe?token=`)
		b.WriteString(html.EscapeString(token))
		b.WriteString(`&amp;user_id=`)
		b.WriteString(fmt.Sprint(user.ID))
		b.WriteString(`" target="_blank" style="text-decoration:none;"><span style="color:#878d97;">Unsubscribe</span></a>`)
	}
	b.WriteString(`</div></td></tr></tbody></table></td></tr>`)
	b.WriteString(`<tr><td colspan="3" style="padding:20px;width:100%;border-bottom:1px solid #d8dadd;"><table><tbody><tr><td align="right" style="width:33%;"><img src="https://www.odoo.com/web/image/38874595-16ef5349/odoo-mobile.png" alt="Odoo Mobile" /></td><td align="left" style="width:66%;"><p class="run_business">Run your business from anywhere with <b>Odoo Mobile</b>.</p><a href="https://play.google.com/store/apps/details?id=com.odoo.mobile" target="_blank"><img class="download_app" height="40" width="135" src="https://download.odoocdn.com/digests/digest/static/src/img/google_play.png" /></a><br/><a href="https://itunes.apple.com/us/app/odoo/id1272543640" target="_blank"><img class="download_app" height="40" width="135" src="https://download.odoocdn.com/digests/digest/static/src/img/app_store.png" /></a></td></tr></tbody></table></td></tr>`)
	b.WriteString(`</tfoot></table></td></tr></tbody><tfoot><tr><td align="center" style="padding:20px 0 0 0;"><table align="center"><tbody><tr><td><div id="footer"><p style="font-weight:bold;">`)
	b.WriteString(html.EscapeString(firstText(company.Name, "Company")))
	b.WriteString(`</p><p class="by_odoo" id="powered">Powered by <a href="https://www.odoo.com" target="_blank" class="odoo_link"><span class="odoo_link_text">Odoo</span></a></p></div></td></tr></tbody></table></td></tr></tfoot></table>`)
	b.WriteString(`</div></div></body></html>`)
	return b.String()
}

func digestWriteKPICell(b *strings.Builder, col digestKPIColumn) {
	b.WriteString(`<td class="kpi_cell" style="padding-top:10px;border-top:1px solid #e6e6e6;"><div><span class="kpi_value kpi_border_col">`)
	b.WriteString(html.EscapeString(col.Value))
	b.WriteString(`</span><br/><span class="kpi_value_label">`)
	b.WriteString(html.EscapeString(col.Subtitle))
	b.WriteString(`</span>`)
	if col.Margin > 0 {
		b.WriteString(`<table class="kpi_margin_margin" align="center" border="0" cellspacing="0" cellpadding="0"><tbody><tr><td class="kpi_margin positive_kpi_margin">⬆ `)
		b.WriteString(fmt.Sprintf("%.2f", col.Margin))
		b.WriteString(` %</td></tr></tbody></table>`)
	} else if col.Margin < 0 {
		b.WriteString(`<table class="kpi_margin_margin" align="center" border="0" cellspacing="0" cellpadding="0"><tbody><tr><td class="kpi_margin negative_kpi_margin">⬇ `)
		b.WriteString(fmt.Sprintf("%.2f", col.Margin))
		b.WriteString(` %</td></tr></tbody></table>`)
	}
	b.WriteString(`</div></td>`)
}

func digestKPIData(env *record.Env, digest map[string]any, user digestUserInfo, company digestCompanyInfo, now time.Time) []digestKPIInfo {
	fields := digestEnabledKPIFields(digest)
	if len(fields) == 0 {
		return nil
	}
	frames := digestTimeframes(now)
	out := make([]digestKPIInfo, 0, len(fields))
	for _, fieldName := range fields {
		info := digestKPIInfo{Name: fieldName, FullName: digestKPIName(fieldName), Action: digestKPIAction(fieldName), Cols: make([]digestKPIColumn, 0, len(frames))}
		for _, frame := range frames {
			current := digestKPIValue(env, fieldName, company, frame.Start, frame.End)
			previous := digestKPIValue(env, fieldName, company, frame.PreviousStart, frame.PreviousEnd)
			info.Cols = append(info.Cols, digestKPIColumn{
				Value:    digestFormatKPIValue(fieldName, current),
				Margin:   digestMarginValue(current, previous),
				Subtitle: frame.Name,
			})
		}
		out = append(out, info)
	}
	return out
}

func digestEnabledKPIFields(digest map[string]any) []string {
	fields := []string{}
	for _, fieldName := range []string{"kpi_res_users_connected", "kpi_mail_message_total", "kpi_account_total_revenue"} {
		if boolAny(digest[fieldName]) {
			fields = append(fields, fieldName)
		}
	}
	return fields
}

func digestKPIName(fieldName string) string {
	switch fieldName {
	case "kpi_res_users_connected":
		return "Connected Users"
	case "kpi_mail_message_total":
		return "Messages Sent"
	case "kpi_account_total_revenue":
		return "Total Revenue"
	default:
		return strings.TrimPrefix(fieldName, "kpi_")
	}
}

func digestKPIAction(fieldName string) string {
	switch fieldName {
	case "kpi_mail_message_total":
		return "mail.action_discuss"
	default:
		return ""
	}
}

func digestTimeframes(now time.Time) []digestTimeframe {
	now = queueNow(now)
	return []digestTimeframe{
		{Name: "Last 24 hours", Start: now.AddDate(0, 0, -1), End: now, PreviousStart: now.AddDate(0, 0, -2), PreviousEnd: now.AddDate(0, 0, -1)},
		{Name: "Last 7 Days", Start: now.AddDate(0, 0, -7), End: now, PreviousStart: now.AddDate(0, 0, -14), PreviousEnd: now.AddDate(0, 0, -7)},
		{Name: "Last 30 Days", Start: now.AddDate(0, -1, 0), End: now, PreviousStart: now.AddDate(0, -2, 0), PreviousEnd: now.AddDate(0, -1, 0)},
	}
}

func digestKPIValue(env *record.Env, fieldName string, company digestCompanyInfo, start time.Time, end time.Time) float64 {
	switch fieldName {
	case "kpi_res_users_connected":
		return float64(digestConnectedUserCount(env, company.ID, start, end))
	case "kpi_mail_message_total":
		return float64(digestMailMessageCount(env, start, end))
	default:
		return 0
	}
}

func digestFormatKPIValue(fieldName string, value float64) string {
	switch fieldName {
	default:
		return fmt.Sprintf("%d", int64(value))
	}
}

func digestMarginValue(value float64, previousValue float64) float64 {
	if value == previousValue || value == 0 || previousValue == 0 {
		return 0
	}
	return (value - previousValue) / previousValue * 100
}

func digestPreferences(digest map[string]any, user digestUserInfo, slowDown bool) []string {
	preferences := []string{}
	if slowDown {
		preferences = append(preferences, "We have noticed you did not connect these last few days. We have automatically switched your preference to "+html.EscapeString(digestNextPeriodicity(strings.TrimSpace(stringAny(digest["periodicity"]))))+" Digests.")
	} else if strings.TrimSpace(stringAny(digest["periodicity"])) == "daily" && containsInt64(user.GroupIDs, 1) {
		preferences = append(preferences, `<p>Prefer a broader overview?<br /><a href="/digest/`+fmt.Sprint(int64FromAny(digest["id"]))+`/set_periodicity?periodicity=weekly" target="_blank" style="color:#017e84; font-weight: bold;">Switch to weekly Digests</a></p>`)
	}
	if containsInt64(user.GroupIDs, 1) {
		preferences = append(preferences, `<p>Want to customize this email?<br /><a href="/odoo/digest.digest/`+fmt.Sprint(int64FromAny(digest["id"]))+`" target="_blank" style="color:#017e84; font-weight: bold;">Choose the metrics you care about</a></p>`)
	}
	return preferences
}

func digestMailHeaders(env *record.Env, digestID int64, userID int64) map[string]string {
	headers := map[string]string{"X-Auto-Response-Suppress": "OOF"}
	if token := DigestUnsubscribeToken(env, digestID, userID); token != "" {
		headers["List-Unsubscribe"] = fmt.Sprintf("</digest/%d/unsubscribe_oneclik?token=%s&user_id=%d>", digestID, token, userID)
		headers["List-Unsubscribe-Post"] = "List-Unsubscribe=One-Click"
	}
	return headers
}

func DigestUnsubscribeToken(env *record.Env, digestID int64, userID int64) string {
	if env == nil || digestID == 0 || userID == 0 {
		return ""
	}
	secret := configParameter(messageSystemEnv(env), "database.secret")
	if secret == "" {
		return ""
	}
	payload := fmt.Sprintf("(%s, (%d, %d))", pythonReprString("digest-unsubscribe"), digestID, userID)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func digestNextTipHTML(env *record.Env, user digestUserInfo) string {
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("digest.tip"); !ok {
		return ""
	}
	found, err := systemEnv.Model("digest.tip").SearchWithOptions(domain.And(), record.SearchOptions{Order: "sequence, id"})
	if err != nil || found.Len() == 0 {
		return ""
	}
	rows, err := found.Read("id", "group_id", "user_ids", "tip_description")
	if err != nil {
		return ""
	}
	userGroups := map[int64]bool{}
	for _, groupID := range user.GroupIDs {
		userGroups[groupID] = true
	}
	for _, row := range rows {
		if containsInt64(int64s(row["user_ids"]), user.ID) {
			continue
		}
		groupID := int64FromAny(row["group_id"])
		if groupID != 0 && !userGroups[groupID] {
			continue
		}
		tip := strings.TrimSpace(stringAny(row["tip_description"]))
		if tip == "" {
			continue
		}
		tip = digestRenderTipHTML(tip)
		tipID := int64FromAny(row["id"])
		if tipID != 0 {
			consumed := uniqueIDs(append(int64s(row["user_ids"]), user.ID))
			_ = systemEnv.Model("digest.tip").Browse(tipID).Write(map[string]any{"user_ids": consumed})
		}
		return tip
	}
	return ""
}

func digestRenderTipHTML(tip string) string {
	tip = strings.TrimSpace(tip)
	if tip == "" {
		return ""
	}
	tip = strings.ReplaceAll(tip, `<t t-set="record_exists" t-value="True" />`, "")
	tip = strings.ReplaceAll(tip, `<t t-set="record_exists" t-value="True"/>`, "")
	tip = strings.ReplaceAll(tip, `<t t-if="record_exists">`, "")
	tip = strings.ReplaceAll(tip, `</t>
                <t t-else="">`, `<digest-else>`)
	if index := strings.Index(tip, `<digest-else>`); index >= 0 {
		keep := strings.TrimSpace(tip[:index])
		if end := strings.LastIndex(keep, `</t>`); end >= 0 {
			keep = strings.TrimSpace(keep[:end])
		}
		tip = keep
	}
	tip = strings.ReplaceAll(tip, `</t>`, "")
	return strings.TrimSpace(tip)
}

func digestShouldSlowDown(env *record.Env, userIDs []int64, periodicity string, now time.Time) bool {
	userIDs = uniqueIDs(userIDs)
	if len(userIDs) == 0 {
		return true
	}
	limit := digestLogLimit(periodicity, now)
	found, err := messageSystemEnv(env).Model("res.users.log").Search(domain.Cond("create_uid", "in", int64sToAny(userIDs)))
	if err != nil || found.Len() == 0 {
		return true
	}
	rows, err := found.Read("create_uid", "create_date")
	if err != nil {
		return true
	}
	for _, row := range rows {
		createDate := timeValue(row["create_date"])
		if !createDate.IsZero() && !createDate.Before(limit) {
			return false
		}
	}
	return true
}

func digestLogLimit(periodicity string, now time.Time) time.Time {
	switch periodicity {
	case "weekly":
		return now.AddDate(0, 0, -7)
	case "monthly":
		return now.AddDate(0, -1, 0)
	case "quarterly":
		return now.AddDate(0, -3, 0)
	default:
		return now.AddDate(0, 0, -2)
	}
}

func digestNextPeriodicity(periodicity string) string {
	switch periodicity {
	case "daily":
		return "weekly"
	case "weekly":
		return "monthly"
	default:
		return "quarterly"
	}
}

func digestNextRunDate(periodicity string, now time.Time) string {
	switch periodicity {
	case "weekly":
		return digestDateOnly(now.AddDate(0, 0, 7)).Format("2006-01-02")
	case "monthly":
		return digestDateOnly(now.AddDate(0, 1, 0)).Format("2006-01-02")
	case "quarterly":
		return digestDateOnly(now.AddDate(0, 3, 0)).Format("2006-01-02")
	default:
		return digestDateOnly(now.AddDate(0, 0, 1)).Format("2006-01-02")
	}
}

func digestDateOnly(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
}

func digestMailMessageCount(env *record.Env, start time.Time, end time.Time) int {
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("mail.message"); !ok {
		return 0
	}
	found, err := systemEnv.Model("mail.message").Search(domain.And())
	if err != nil || found.Len() == 0 {
		return 0
	}
	rows, err := found.Read("create_date", "message_type", "subtype_id")
	if err != nil {
		return 0
	}
	count := 0
	for _, row := range rows {
		messageType := strings.TrimSpace(stringAny(row["message_type"]))
		if messageType != "comment" && messageType != "email" && messageType != "email_outgoing" {
			continue
		}
		if subtypeID := int64FromAny(row["subtype_id"]); subtypeID != 0 && messageType == "comment" && !digestMessageSubtypeAllowed(env, subtypeID) {
			continue
		}
		createDate := timeValue(row["create_date"])
		if !createDate.IsZero() && !createDate.Before(start) && createDate.Before(end) {
			count++
		}
	}
	return count
}

func digestMessageSubtypeAllowed(env *record.Env, subtypeID int64) bool {
	if env == nil || subtypeID == 0 {
		return true
	}
	if _, ok := messageSystemEnv(env).ModelMetadata("mail.message.subtype"); !ok {
		return true
	}
	rows, err := messageSystemEnv(env).Model("mail.message.subtype").Browse(subtypeID).Read("name")
	if err != nil || len(rows) == 0 {
		return true
	}
	name := strings.ToLower(strings.TrimSpace(stringAny(rows[0]["name"])))
	return name == "" || strings.Contains(name, "comment") || strings.Contains(name, "note")
}

func digestConnectedUserCount(env *record.Env, companyID int64, start time.Time, end time.Time) int {
	systemEnv := messageSystemEnv(env)
	if _, ok := systemEnv.ModelMetadata("res.users"); !ok {
		return 0
	}
	found, err := systemEnv.Model("res.users").Search(domain.And())
	if err != nil || found.Len() == 0 {
		return 0
	}
	rows, err := found.Read("login_date", "company_id", "company_ids", "active")
	if err != nil {
		return 0
	}
	count := 0
	for _, row := range rows {
		if row["active"] == false {
			continue
		}
		loginDate := timeValue(row["login_date"])
		if loginDate.IsZero() || loginDate.Before(start) || !loginDate.Before(end) {
			continue
		}
		if companyID != 0 && int64FromAny(row["company_id"]) != companyID && !containsInt64(int64s(row["company_ids"]), companyID) {
			continue
		}
		count++
	}
	return count
}

func digestUser(env *record.Env, userID int64) (digestUserInfo, error) {
	if env == nil || userID == 0 {
		return digestUserInfo{}, nil
	}
	rows, err := messageSystemEnv(env).Model("res.users").Browse(userID).Read("id", "name", "email", "partner_id", "company_id", "groups_id", "all_group_ids")
	if err != nil || len(rows) == 0 {
		return digestUserInfo{}, err
	}
	row := rows[0]
	info := digestUserInfo{
		ID:        int64FromAny(row["id"]),
		Name:      stringAny(row["name"]),
		Email:     strings.TrimSpace(stringAny(row["email"])),
		PartnerID: int64FromAny(row["partner_id"]),
		CompanyID: int64FromAny(row["company_id"]),
		GroupIDs:  uniqueIDs(append(int64s(row["groups_id"]), int64s(row["all_group_ids"])...)),
	}
	if info.Email == "" && info.PartnerID != 0 {
		partnerRows, err := messageSystemEnv(env).Model("res.partner").Browse(info.PartnerID).Read("email")
		if err != nil {
			return info, err
		}
		if len(partnerRows) > 0 {
			info.Email = strings.TrimSpace(stringAny(partnerRows[0]["email"]))
		}
	}
	info.EmailFormatted = formatMailingReportEmail(info.Name, info.Email)
	return info, nil
}

func digestCompany(env *record.Env, companyID int64) digestCompanyInfo {
	info := digestCompanyInfo{ID: companyID, Name: "Company"}
	if env == nil || companyID == 0 {
		return info
	}
	rows, err := messageSystemEnv(env).Model("res.company").Browse(companyID).Read("name", "partner_id")
	if err != nil || len(rows) == 0 {
		return info
	}
	info.Name = firstText(rows[0]["name"], info.Name)
	partnerID := int64FromAny(rows[0]["partner_id"])
	if partnerID != 0 {
		partnerRows, err := messageSystemEnv(env).Model("res.partner").Browse(partnerID).Read("email")
		if err == nil && len(partnerRows) > 0 {
			info.EmailFormatted = formatMailingReportEmail(info.Name, strings.TrimSpace(stringAny(partnerRows[0]["email"])))
		}
	}
	return info
}

func digestCurrentUserEmail(env *record.Env) string {
	if env == nil || env.Context().UserID == 0 {
		return ""
	}
	user, err := digestUser(env, env.Context().UserID)
	if err != nil {
		return ""
	}
	return user.EmailFormatted
}
