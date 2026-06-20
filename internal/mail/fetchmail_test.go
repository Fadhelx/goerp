package mail

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

type fakeFetchmailConnector struct {
	conns  map[int64]*fakeFetchmailConnection
	errors map[int64]error
	seen   []int64
}

func (f *fakeFetchmailConnector) Connect(_ context.Context, server FetchmailServerConfig) (FetchmailConnection, error) {
	f.seen = append(f.seen, server.ID)
	if f.errors != nil {
		if err := f.errors[server.ID]; err != nil {
			return nil, err
		}
	}
	if f.conns != nil {
		if conn := f.conns[server.ID]; conn != nil {
			return conn, nil
		}
	}
	return &fakeFetchmailConnection{}, nil
}

type fakeFetchmailConnection struct {
	messages     []FetchedMessage
	marked       []string
	checkErr     error
	retrieveErr  error
	closeCount   int
	checkStarted chan struct{}
	checkRelease chan struct{}
	unreadCount  int
}

func (c *fakeFetchmailConnection) CheckUnreadMessages(context.Context) (int, error) {
	if c.checkStarted != nil {
		close(c.checkStarted)
		c.checkStarted = nil
	}
	if c.checkRelease != nil {
		<-c.checkRelease
	}
	if c.checkErr != nil {
		return 0, c.checkErr
	}
	if c.unreadCount > 0 {
		return c.unreadCount, nil
	}
	return len(c.messages), nil
}

func (c *fakeFetchmailConnection) RetrieveUnreadMessages(_ context.Context, limit int) ([]FetchedMessage, error) {
	if c.retrieveErr != nil {
		return nil, c.retrieveErr
	}
	if limit > 0 && len(c.messages) > limit {
		return append([]FetchedMessage(nil), c.messages[:limit]...), nil
	}
	return append([]FetchedMessage(nil), c.messages...), nil
}

func (c *fakeFetchmailConnection) MarkHandled(_ context.Context, message FetchedMessage) error {
	c.marked = append(c.marked, message.Num)
	return nil
}

func (c *fakeFetchmailConnection) Close() error {
	c.closeCount++
	return nil
}

func TestProcessFetchmailServersProcessesMessagesWithServerOptions(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{
		"object_id": modelID,
		"attach":    false,
		"original":  true,
	})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundMultipartMessage("<fetchmail-ok@remote>", "Fetchmail OK", true)),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" || conn.closeCount != 1 {
		t.Fatalf("connection marked=%+v close=%d", conn.marked, conn.closeCount)
	}
	serverRows, err := env.Model("fetchmail.server").Browse(serverID).Read("date", "error_date", "error_message")
	if err != nil {
		t.Fatal(err)
	}
	if !timeValue(serverRows[0]["date"]).Equal(now) || serverRows[0]["error_date"] != nil || serverRows[0]["error_message"] != "" {
		t.Fatalf("server rows = %+v", serverRows)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-ok@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := found.Read("model", "res_id", "attachment_ids", "incoming_email_to", "email_from")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 || messageRows[0]["model"] != "res.partner" || int64FromAny(messageRows[0]["res_id"]) == 0 {
		t.Fatalf("message rows = %+v", messageRows)
	}
	if got := messageRows[0]["attachment_ids"].([]int64); len(got) != 0 {
		t.Fatalf("attachment ids = %#v", got)
	}
	if messageRows[0]["incoming_email_to"] != "catch@example.com" || messageRows[0]["email_from"] != "Attach Sender <attach.sender@example.com>" {
		t.Fatalf("message metadata = %+v", messageRows)
	}
}

func TestProcessFetchmailServersUsesGatewayUserForFallbackTarget(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 10, 0, 0, time.UTC)
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{
		"name":             "Fetch Gateway User",
		"email":            "fetch.gateway@example.com",
		"email_normalized": "fetch.gateway@example.com",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{
		"login":      "fetch-gateway",
		"name":       "Fetch Gateway",
		"partner_id": authorPartnerID,
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	modelID := fetchmailTestModelID(t, env, "gateway.thread")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundNewMessage("<fetch-gateway@remote>", "Fetch Gateway <fetch.gateway@example.com>", "Fetch Gateway Creates", "<p>Fetch</p>")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 || result.Skipped != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" {
		t.Fatalf("connection marked=%+v", conn.marked)
	}
	foundTargets, err := env.Model("gateway.thread").Search(domain.Cond("name", "=", "Fetch Gateway Creates"))
	if err != nil {
		t.Fatal(err)
	}
	targetRows, err := foundTargets.Read("id", "create_uid", "write_uid", "email")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 || targetRows[0]["create_uid"] != authorUserID || targetRows[0]["write_uid"] != authorUserID || targetRows[0]["email"] != "fetch.gateway@example.com" {
		t.Fatalf("target rows = %+v user=%d", targetRows, authorUserID)
	}
	foundMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetch-gateway@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	messageRows, err := foundMessages.Read("create_uid", "write_uid", "author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(messageRows) != 1 ||
		messageRows[0]["create_uid"] != int64(1) ||
		messageRows[0]["write_uid"] != int64(1) ||
		messageRows[0]["author_id"] != authorPartnerID ||
		messageRows[0]["model"] != "gateway.thread" ||
		messageRows[0]["res_id"] != targetRows[0]["id"] {
		t.Fatalf("message rows = %+v target=%+v", messageRows, targetRows)
	}
}

func TestProcessFetchmailServersPropagatesMailingTraceUTM(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 15, 0, 0, time.UTC)
	campaignID, err := env.Model("utm.campaign").Create(map[string]any{"name": "Fetch Campaign"})
	if err != nil {
		t.Fatal(err)
	}
	sourceID, err := env.Model("utm.source").Create(map[string]any{"name": "Fetch Source"})
	if err != nil {
		t.Fatal(err)
	}
	mediumID, err := env.Model("utm.medium").Create(map[string]any{"name": "Fetch Email"})
	if err != nil {
		t.Fatal(err)
	}
	mailingID, err := env.Model("mailing.mailing").Create(map[string]any{
		"name":        "Fetch Mailing",
		"campaign_id": campaignID,
		"source_id":   sourceID,
		"medium_id":   mediumID,
	})
	if err != nil {
		t.Fatal(err)
	}
	partnerID, err := env.Model("res.partner").Create(map[string]any{"name": "Fetch Recipient", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("mailing.trace").Create(map[string]any{
		"message_id":      "<fetch-utm-parent@local>",
		"email":           "fetch.recipient@example.com",
		"model":           "res.partner",
		"res_id":          partnerID,
		"mass_mailing_id": mailingID,
	}); err != nil {
		t.Fatal(err)
	}
	modelID := fetchmailTestModelID(t, env, "gateway.thread")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(strings.Join([]string{
			"Message-Id: <fetch-utm-reply@remote>",
			"From: Fetch Prospect <fetch.prospect@example.com>",
			"To: catch@example.com",
			"Subject: Fetch UTM Reply",
			"References: <fetch-utm-parent@local>",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Fetch UTM body",
			"",
		}, "\r\n")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	foundTargets, err := env.Model("gateway.thread").Search(domain.Cond("name", "=", "Fetch UTM Reply"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := foundTargets.Read("campaign_id", "source_id", "medium_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["campaign_id"] != campaignID || rows[0]["source_id"] != sourceID || rows[0]["medium_id"] != mediumID {
		t.Fatalf("gateway UTM rows = %+v", rows)
	}
}

func TestProcessFetchmailServersRunsInboundMessageNewAndUpdateHooks(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 20, 0, 0, time.UTC)
	authorPartnerID, err := env.Model("res.partner").Create(map[string]any{
		"name":             "Fetch Hook User",
		"email":            "fetch.hook@example.com",
		"email_normalized": "fetch.hook@example.com",
		"active":           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	authorUserID, err := env.Model("res.users").Create(map[string]any{
		"login":      "fetch-hook",
		"name":       "Fetch Hook",
		"partner_id": authorPartnerID,
		"active":     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	unregister := RegisterInboundMessageHandler("gateway.thread", InboundMessageHandler{
		MessageNew: func(hookEnv *record.Env, req InboundMessageNewRequest) (int64, error) {
			if hookEnv.Context().UserID != authorUserID || req.Message.AuthorID != authorPartnerID || req.Message.MessageID != "<fetch-hook-new@remote>" {
				t.Fatalf("new hook env=%+v req=%+v", hookEnv.Context(), req)
			}
			calls = append(calls, "new")
			return hookEnv.Model(req.Model).Create(map[string]any{
				"name":            req.Message.Subject + " handled",
				"description":     req.Message.BodyHTML,
				"gateway_user_id": hookEnv.Context().UserID,
				"active":          true,
			})
		},
		MessageUpdate: func(hookEnv *record.Env, req InboundMessageUpdateRequest) error {
			if hookEnv.Context().UserID != authorUserID || req.Message.AuthorID != authorPartnerID || req.Message.MessageID != "<fetch-hook-update@remote>" || req.ResID == 0 || req.Message.ParentID == 0 {
				t.Fatalf("update hook env=%+v req=%+v", hookEnv.Context(), req)
			}
			calls = append(calls, "update")
			return hookEnv.Model(req.Model).Browse(req.ResID).Write(map[string]any{
				"description":     req.Message.Subject + "|" + req.Message.BodyHTML,
				"message_count":   int64(1),
				"gateway_user_id": hookEnv.Context().UserID,
			})
		},
	})
	t.Cleanup(unregister)
	modelID := fetchmailTestModelID(t, env, "gateway.thread")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundNewMessage("<fetch-hook-new@remote>", "Fetch Hook <fetch.hook@example.com>", "Fetch Hook New", "<p>Fetch hook body</p>")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 {
		t.Fatalf("new result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" {
		t.Fatalf("new marked = %+v", conn.marked)
	}
	foundTargets, err := env.Model("gateway.thread").Search(domain.Cond("name", "=", "Fetch Hook New handled"))
	if err != nil {
		t.Fatal(err)
	}
	targetRows, err := foundTargets.Read("id", "description", "gateway_user_id", "create_uid", "write_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetRows) != 1 ||
		targetRows[0]["description"] != "<p>Fetch hook body</p>" ||
		targetRows[0]["gateway_user_id"] != authorUserID ||
		targetRows[0]["create_uid"] != authorUserID ||
		targetRows[0]["write_uid"] != authorUserID {
		t.Fatalf("new target rows = %+v user=%d", targetRows, authorUserID)
	}
	threadID := int64FromAny(targetRows[0]["id"])
	newMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetch-hook-new@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	newRows, err := newMessages.Read("create_uid", "write_uid", "author_id", "model", "res_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(newRows) != 1 || newRows[0]["create_uid"] != int64(1) || newRows[0]["write_uid"] != int64(1) || newRows[0]["author_id"] != authorPartnerID || newRows[0]["model"] != "gateway.thread" || newRows[0]["res_id"] != threadID {
		t.Fatalf("new message rows = %+v", newRows)
	}
	parentID, err := env.Model("mail.message").Create(map[string]any{
		"subject":      "Fetch parent",
		"body":         "<p>Parent</p>",
		"message_type": "email",
		"model":        "gateway.thread",
		"res_id":       threadID,
		"message_id":   "<fetch-hook-parent@local>",
	})
	if err != nil {
		t.Fatal(err)
	}
	conn.messages = []FetchedMessage{{
		Num: "2",
		Raw: []byte(strings.Join([]string{
			"Message-Id: <fetch-hook-update@remote>",
			"From: Fetch Hook <fetch.hook@example.com>",
			"To: catch@example.com",
			"Subject: Fetch Hook Update",
			"In-Reply-To: <fetch-hook-parent@local>",
			"References: <fetch-hook-parent@local>",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Fetch update body",
			"",
		}, "\r\n")),
	}}
	conn.marked = nil
	result, err = ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 {
		t.Fatalf("update result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "2" {
		t.Fatalf("update marked = %+v", conn.marked)
	}
	updateTargetRows, err := env.Model("gateway.thread").Browse(threadID).Read("description", "message_count", "gateway_user_id", "write_uid")
	if err != nil {
		t.Fatal(err)
	}
	if len(updateTargetRows) != 1 ||
		!strings.Contains(stringAny(updateTargetRows[0]["description"]), "Fetch Hook Update|<pre>Fetch update body</pre>") ||
		updateTargetRows[0]["message_count"] != int64(1) ||
		updateTargetRows[0]["gateway_user_id"] != authorUserID ||
		updateTargetRows[0]["write_uid"] != authorUserID {
		t.Fatalf("update target rows = %+v user=%d", updateTargetRows, authorUserID)
	}
	updateMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetch-hook-update@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	updateRows, err := updateMessages.Read("create_uid", "write_uid", "author_id", "model", "res_id", "parent_id")
	if err != nil {
		t.Fatal(err)
	}
	if len(updateRows) != 1 ||
		updateRows[0]["create_uid"] != int64(1) ||
		updateRows[0]["write_uid"] != int64(1) ||
		updateRows[0]["author_id"] != authorPartnerID ||
		updateRows[0]["model"] != "gateway.thread" ||
		updateRows[0]["res_id"] != threadID ||
		updateRows[0]["parent_id"] != parentID {
		t.Fatalf("update message rows = %+v parent=%d", updateRows, parentID)
	}
	if strings.Join(calls, ",") != "new,update" {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestProcessFetchmailServersMarksLoopDetectionRepliesHandled(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(strings.Join([]string{
			"Message-Id: <fetchmail-loop@remote>",
			"From: Auto Reply <auto@example.com>",
			"To: catch@example.com",
			"In-Reply-To: <20260619-loop-detection-bounce-email@example.com>",
			"Subject: Loop reply",
			"Content-Type: text/plain; charset=utf-8",
			"",
			"Loop reply body",
			"",
		}, "\r\n")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" {
		t.Fatalf("connection marked=%+v", conn.marked)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-loop@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("loop reply should not create mail.message, count = %d", found.Len())
	}
}

func TestProcessFetchmailServersMarksDuplicateMessagesHandled(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{
		{
			Num: "1",
			Raw: []byte(inboundNewMessage("<fetchmail-duplicate@remote>", "Duplicate <duplicate@example.com>", "Duplicate", "<p>Duplicate</p>")),
		},
		{
			Num: "2",
			Raw: []byte(inboundNewMessage("<fetchmail-duplicate@remote>", "Duplicate <duplicate@example.com>", "Duplicate again", "<p>Duplicate again</p>")),
		},
	}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 2 || result.Processed != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 2 || conn.marked[0] != "1" || conn.marked[1] != "2" {
		t.Fatalf("connection marked=%+v", conn.marked)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-duplicate@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("duplicate message count = %d", found.Len())
	}
}

func TestProcessFetchmailServersConcurrentDuplicateAcrossWorkersMarksBothHandled(t *testing.T) {
	env, _ := threadEnv(t)
	otherWorker := env.WithSequenceNamespace("fetchmail-worker-2")
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	raw := inboundNewMessage("<fetchmail-cross-worker@remote>", "Duplicate <duplicate@example.com>", "Duplicate", "<p>Duplicate</p>")
	firstConn := &fakeFetchmailConnection{messages: []FetchedMessage{{Num: "1", Raw: []byte(raw)}}}
	secondConn := &fakeFetchmailConnection{messages: []FetchedMessage{{Num: "1", Raw: []byte(raw)}}}
	firstConnector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: firstConn}}
	secondConnector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: secondConn}}
	openServerLocker := FetchmailServerLockFunc(func(int64) (func(), bool, error) {
		return func() {}, true, nil
	})
	locked := make(chan struct{})
	release := make(chan struct{})
	restore := setInboundAfterMessageIDLockHook(func(messageID string) {
		if messageID != "<fetchmail-cross-worker@remote>" {
			return
		}
		select {
		case <-locked:
		default:
			close(locked)
			<-release
		}
	})
	defer restore()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	type fetchResult struct {
		result FetchmailResult
		err    error
	}
	firstDone := make(chan fetchResult, 1)
	go func() {
		result, err := ProcessFetchmailServers(context.Background(), env, firstConnector, FetchmailOptions{Now: now, ServerLocker: openServerLocker})
		firstDone <- fetchResult{result: result, err: err}
	}()
	select {
	case <-locked:
	case <-time.After(2 * time.Second):
		t.Fatal("first fetchmail message did not acquire duplicate lock")
	}
	secondResult, err := ProcessFetchmailServers(context.Background(), otherWorker, secondConnector, FetchmailOptions{Now: now, ServerLocker: openServerLocker})
	if err != nil {
		t.Fatal(err)
	}
	if secondResult.Servers != 1 || secondResult.Fetched != 1 || secondResult.Processed != 1 || secondResult.Failed != 0 {
		t.Fatalf("second result = %+v", secondResult)
	}
	if len(secondConn.marked) != 1 || secondConn.marked[0] != "1" {
		t.Fatalf("second connection marked = %+v", secondConn.marked)
	}
	close(release)
	var first fetchResult
	select {
	case first = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first fetchmail run did not finish")
	}
	if first.err != nil {
		t.Fatal(first.err)
	}
	if first.result.Servers != 1 || first.result.Fetched != 1 || first.result.Processed != 1 || first.result.Failed != 0 {
		t.Fatalf("first result = %+v", first.result)
	}
	if len(firstConn.marked) != 1 || firstConn.marked[0] != "1" {
		t.Fatalf("first connection marked = %+v", firstConn.marked)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-cross-worker@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("cross-worker duplicate message count = %d", found.Len())
	}
}

func TestProcessFetchmailServersSkipsLockedServer(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundNewMessage("<fetchmail-locked@remote>", "Locked <locked@example.com>", "Locked", "<p>Locked</p>")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}
	busyLocker := FetchmailServerLockFunc(func(int64) (func(), bool, error) {
		return func() {}, false, nil
	})

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now, ServerLocker: busyLocker})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 0 || result.Processed != 0 || result.Failed != 0 || result.Skipped != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(connector.seen) != 0 || len(conn.marked) != 0 || conn.closeCount != 0 {
		t.Fatalf("connector seen=%+v marked=%+v close=%d", connector.seen, conn.marked, conn.closeCount)
	}
}

func TestProcessFetchmailServersConcurrentLockedServerDoesNotDoubleFetch(t *testing.T) {
	env, _ := threadEnv(t)
	otherWorker := env.WithSequenceNamespace("fetchmail-server-worker-2")
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	raw := inboundNewMessage("<fetchmail-server-lock@remote>", "Locked <locked@example.com>", "Server lock", "<p>Server lock</p>")
	checkStarted := make(chan struct{})
	checkRelease := make(chan struct{})
	firstConn := &fakeFetchmailConnection{
		messages:     []FetchedMessage{{Num: "1", Raw: []byte(raw)}},
		checkStarted: checkStarted,
		checkRelease: checkRelease,
	}
	secondConn := &fakeFetchmailConnection{messages: []FetchedMessage{{Num: "1", Raw: []byte(raw)}}}
	firstConnector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: firstConn}}
	secondConnector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: secondConn}}
	defer func() {
		select {
		case <-checkRelease:
		default:
			close(checkRelease)
		}
	}()

	type fetchResult struct {
		result FetchmailResult
		err    error
	}
	firstDone := make(chan fetchResult, 1)
	go func() {
		result, err := ProcessFetchmailServers(context.Background(), env, firstConnector, FetchmailOptions{Now: now})
		firstDone <- fetchResult{result: result, err: err}
	}()
	select {
	case <-checkStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first fetchmail server did not enter check")
	}
	secondResult, err := ProcessFetchmailServers(context.Background(), otherWorker, secondConnector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if secondResult.Servers != 1 || secondResult.Fetched != 0 || secondResult.Processed != 0 || secondResult.Failed != 0 || secondResult.Skipped != 1 {
		t.Fatalf("second result = %+v", secondResult)
	}
	if len(secondConnector.seen) != 0 || len(secondConn.marked) != 0 || secondConn.closeCount != 0 {
		t.Fatalf("second connector seen=%+v marked=%+v close=%d", secondConnector.seen, secondConn.marked, secondConn.closeCount)
	}
	close(checkRelease)
	var first fetchResult
	select {
	case first = <-firstDone:
	case <-time.After(2 * time.Second):
		t.Fatal("first fetchmail run did not finish")
	}
	if first.err != nil {
		t.Fatal(first.err)
	}
	if first.result.Servers != 1 || first.result.Fetched != 1 || first.result.Processed != 1 || first.result.Failed != 0 || first.result.Skipped != 0 {
		t.Fatalf("first result = %+v", first.result)
	}
	if len(firstConn.marked) != 1 || firstConn.marked[0] != "1" || firstConn.closeCount != 1 {
		t.Fatalf("first connection marked=%+v close=%d", firstConn.marked, firstConn.closeCount)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-server-lock@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 1 {
		t.Fatalf("server-lock message count = %d", found.Len())
	}
}

func TestProcessFetchmailServersGeneratesDistinctFallbackMessageIDs(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	firstRaw := strings.Replace(inboundNewMessage("<fetchmail-missing-one@remote>", "Missing One <missing.one@example.com>", "Missing one", "<p>One</p>"), "Message-Id: <fetchmail-missing-one@remote>\r\n", "", 1)
	secondRaw := strings.Replace(inboundNewMessage("<fetchmail-missing-two@remote>", "Missing Two <missing.two@example.com>", "Missing two", "<p>Two</p>"), "Message-Id: <fetchmail-missing-two@remote>\r\n", "", 1)
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{
		{Num: "1", Raw: []byte(firstRaw)},
		{Num: "2", Raw: []byte(secondRaw)},
	}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 2 || result.Processed != 2 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 2 {
		t.Fatalf("connection marked=%+v", conn.marked)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("subject", "in", []any{"Missing one", "Missing two"}))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("message_id")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		seen[stringAny(row["message_id"])] = true
	}
	if len(rows) != 2 || len(seen) != 2 {
		t.Fatalf("message rows = %+v", rows)
	}
}

func TestProcessFetchmailServersMarksSenderLoopsHandled(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	if _, err := env.Model("ir.config_parameter").Create(map[string]any{"key": "mail.gateway.loop.threshold", "value": "1"}); err != nil {
		t.Fatal(err)
	}
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	if _, err := env.Model("res.partner").Create(map[string]any{
		"name":             "Existing Loop",
		"email":            "fetch.loop@example.com",
		"email_normalized": "fetch.loop@example.com",
		"active":           true,
	}); err != nil {
		t.Fatal(err)
	}
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundNewMessage("<fetchmail-sender-loop@remote>", "Fetch Loop <fetch.loop@example.com>", "Fetch loop", "<p>Loop</p>")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" {
		t.Fatalf("connection marked=%+v", conn.marked)
	}
	found, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-sender-loop@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if found.Len() != 0 {
		t.Fatalf("looping inbound message count = %d", found.Len())
	}
}

func TestProcessFetchmailServersHonorsOrderingAndPerServerBatchLimit(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	laterID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID, "priority": 20})
	firstID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID, "priority": 1})
	laterConn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundNewMessage("<later@remote>", "Later <later@example.com>", "Later", "<p>Later</p>")),
	}}}
	firstConn := &fakeFetchmailConnection{messages: []FetchedMessage{
		{Num: "1", Raw: []byte(inboundNewMessage("<first@remote>", "First <first@example.com>", "First", "<p>First</p>"))},
		{Num: "2", Raw: []byte(inboundNewMessage("<second@remote>", "Second <second@example.com>", "Second", "<p>Second</p>"))},
	}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{
		laterID: laterConn,
		firstID: firstConn,
	}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{BatchLimit: 1, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 2 || result.Processed != 2 || result.Failed != 0 || result.Remaining != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(connector.seen) != 2 || connector.seen[0] != firstID || connector.seen[1] != laterID {
		t.Fatalf("connect order = %+v", connector.seen)
	}
	if len(firstConn.marked) != 1 || firstConn.marked[0] != "1" || len(laterConn.marked) != 1 || laterConn.marked[0] != "1" {
		t.Fatalf("marked first=%+v later=%+v", firstConn.marked, laterConn.marked)
	}
}

func TestProcessFetchmailServersReportsRemainingUnreadMessages(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{
		unreadCount: 3,
		messages: []FetchedMessage{
			{Num: "1", Raw: []byte(inboundNewMessage("<remaining-one@remote>", "One <one@example.com>", "One", "<p>One</p>"))},
			{Num: "2", Raw: []byte(inboundNewMessage("<remaining-two@remote>", "Two <two@example.com>", "Two", "<p>Two</p>"))},
			{Num: "3", Raw: []byte(inboundNewMessage("<remaining-three@remote>", "Three <three@example.com>", "Three", "<p>Three</p>"))},
		},
	}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{BatchLimit: 1, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Checked != 1 || result.Fetched != 1 || result.Processed != 1 || result.Failed != 0 || result.Remaining != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" {
		t.Fatalf("marked = %+v", conn.marked)
	}
}

func TestProcessFetchmailServersCommitsSourceProgress(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{
		unreadCount: 3,
		messages: []FetchedMessage{
			{Num: "1", Raw: []byte(inboundNewMessage("<progress-one@remote>", "One <one@example.com>", "Progress one", "<p>One</p>"))},
			{Num: "2", Raw: []byte(inboundNewMessage("<progress-two@remote>", "Two <two@example.com>", "Progress two", "<p>Two</p>"))},
			{Num: "3", Raw: []byte(inboundNewMessage("<progress-three@remote>", "Three <three@example.com>", "Progress three", "<p>Three</p>"))},
		},
	}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}
	type progressCall struct {
		processed  int
		remaining  *int
		deactivate bool
	}
	var calls []progressCall
	progress := func(processed int, remaining *int, deactivate bool) bool {
		var copied *int
		if remaining != nil {
			value := *remaining
			copied = &value
		}
		calls = append(calls, progressCall{processed: processed, remaining: copied, deactivate: deactivate})
		return true
	}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{BatchLimit: 2, Now: now, Progress: progress})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Checked != 1 || result.Fetched != 2 || result.Processed != 2 || result.Failed != 0 || result.Remaining != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(calls) != 4 {
		t.Fatalf("progress calls = %+v", calls)
	}
	if calls[0].processed != 0 || calls[0].remaining == nil || *calls[0].remaining != 1 || calls[0].deactivate {
		t.Fatalf("initial progress = %+v", calls[0])
	}
	if calls[1].processed != 1 || calls[1].remaining != nil || calls[1].deactivate ||
		calls[2].processed != 1 || calls[2].remaining != nil || calls[2].deactivate {
		t.Fatalf("message progress = %+v", calls[1:3])
	}
	if calls[3].processed != 1 || calls[3].remaining == nil || *calls[3].remaining != 1 || calls[3].deactivate {
		t.Fatalf("server progress = %+v", calls[3])
	}
}

func TestProcessFetchmailServersUsesCronEndTimeWithServerBuffer(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	firstID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID, "priority": 1})
	secondID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID, "priority": 2})
	firstConn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundNewMessage("<budget-first@remote>", "First <first@example.com>", "Budget first", "<p>First</p>")),
	}}}
	secondConn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "1",
		Raw: []byte(inboundNewMessage("<budget-second@remote>", "Second <second@example.com>", "Budget second", "<p>Second</p>")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{
		firstID:  firstConn,
		secondID: secondConn,
	}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{
		Now:         now,
		CronEndTime: now,
		Clock:       func() time.Time { return now.Add(5 * time.Second) },
		Progress:    func(int, *int, bool) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 2 || result.Checked != 2 || result.Fetched != 2 || result.Processed != 2 || result.Remaining != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(firstConn.marked) != 1 || len(secondConn.marked) != 1 {
		t.Fatalf("marked first=%+v second=%+v", firstConn.marked, secondConn.marked)
	}
}

func TestProcessFetchmailServersStopsWhenCronBudgetExpires(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{
		unreadCount: 3,
		messages: []FetchedMessage{
			{Num: "1", Raw: []byte(inboundNewMessage("<budget-one@remote>", "One <one@example.com>", "Budget one", "<p>One</p>"))},
			{Num: "2", Raw: []byte(inboundNewMessage("<budget-two@remote>", "Two <two@example.com>", "Budget two", "<p>Two</p>"))},
			{Num: "3", Raw: []byte(inboundNewMessage("<budget-three@remote>", "Three <three@example.com>", "Budget three", "<p>Three</p>"))},
		},
	}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{
		Now:         now,
		CronEndTime: now,
		Clock:       func() time.Time { return now.Add(5 * time.Second) },
		Progress:    func(int, *int, bool) bool { return true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Checked != 1 || result.Fetched != 1 || result.Processed != 1 || result.Remaining != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "1" {
		t.Fatalf("marked = %+v", conn.marked)
	}
}

func TestConfirmFetchmailServersFailureDoesNotWriteErrorFields(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID, "state": "draft", "password": "secret-pass"})
	connector := &fakeFetchmailConnector{errors: map[int64]error{serverID: errors.New("auth failed for secret-pass")}}

	err := ConfirmFetchmailServers(context.Background(), env, connector, []int64{serverID}, now)
	if err == nil {
		t.Fatal("expected confirm error")
	}
	rows, err := env.Model("fetchmail.server").Browse(serverID).Read("state", "error_date", "error_message")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["state"] != "draft" || rows[0]["error_date"] != nil || rows[0]["error_message"] != nil {
		t.Fatalf("confirm failure row = %+v", rows)
	}
}

func TestProcessFetchmailServersMarksHandledAfterMessageFailure(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "missing.model")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{{
		Num: "bad",
		Raw: []byte(inboundNewMessage("<bad@remote>", "Bad <bad@example.com>", "Bad", "<p>Bad</p>")),
	}}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fetched != 1 || result.Processed != 0 || result.Failed != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 1 || conn.marked[0] != "bad" {
		t.Fatalf("marked = %+v", conn.marked)
	}
	serverRows, err := env.Model("fetchmail.server").Browse(serverID).Read("date", "error_date", "error_message")
	if err != nil {
		t.Fatal(err)
	}
	if !timeValue(serverRows[0]["date"]).Equal(now) || serverRows[0]["error_date"] != nil || serverRows[0]["error_message"] != "" {
		t.Fatalf("server rows = %+v", serverRows)
	}
}

func TestProcessFetchmailServersContinuesAfterMessageFailureInSameBatch(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	unregister := RegisterInboundMessageHandler("gateway.thread", InboundMessageHandler{
		MessageNew: func(hookEnv *record.Env, req InboundMessageNewRequest) (int64, error) {
			id, err := hookEnv.Model(req.Model).Create(map[string]any{
				"name":        req.Message.Subject,
				"description": req.Message.BodyHTML,
				"active":      true,
			})
			if err != nil {
				return 0, err
			}
			if req.Message.MessageID == "<fetchmail-rollback-bad@remote>" {
				return 0, errors.New("synthetic fetchmail failure")
			}
			return id, nil
		},
	})
	t.Cleanup(unregister)
	modelID := fetchmailTestModelID(t, env, "gateway.thread")
	serverID := createFetchmailTestServer(t, env, map[string]any{"object_id": modelID})
	conn := &fakeFetchmailConnection{messages: []FetchedMessage{
		{Num: "bad", Raw: []byte(inboundNewMessage("<fetchmail-rollback-bad@remote>", "Bad <bad@example.com>", "Fetch Rollback Bad", "<p>Bad</p>"))},
		{Num: "good", Raw: []byte(inboundNewMessage("<fetchmail-rollback-good@remote>", "Good <good@example.com>", "Fetch Rollback Good", "<p>Good</p>"))},
	}}
	connector := &fakeFetchmailConnector{conns: map[int64]*fakeFetchmailConnection{serverID: conn}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if result.Servers != 1 || result.Fetched != 2 || result.Processed != 1 || result.Failed != 1 || result.Skipped != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(conn.marked) != 2 || conn.marked[0] != "bad" || conn.marked[1] != "good" {
		t.Fatalf("marked = %+v", conn.marked)
	}
	badTargets, err := env.Model("gateway.thread").Search(domain.Cond("name", "=", "Fetch Rollback Bad"))
	if err != nil {
		t.Fatal(err)
	}
	if badTargets.Len() != 0 {
		t.Fatalf("failed target count = %d", badTargets.Len())
	}
	goodTargets, err := env.Model("gateway.thread").Search(domain.Cond("name", "=", "Fetch Rollback Good"))
	if err != nil {
		t.Fatal(err)
	}
	if goodTargets.Len() != 1 {
		t.Fatalf("good target count = %d", goodTargets.Len())
	}
	badMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-rollback-bad@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if badMessages.Len() != 0 {
		t.Fatalf("failed message count = %d", badMessages.Len())
	}
	goodMessages, err := env.Model("mail.message").Search(domain.Cond("message_id", "=", "<fetchmail-rollback-good@remote>"))
	if err != nil {
		t.Fatal(err)
	}
	if goodMessages.Len() != 1 {
		t.Fatalf("good message count = %d", goodMessages.Len())
	}
}

func TestIMAPConnectionClearsUnreadFlagAfterFetchUntilHandled(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	commands := make(chan string, 16)
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		writer := bufio.NewWriter(server)
		if _, err := fmt.Fprint(writer, "* OK ready\r\n"); err != nil {
			done <- err
			return
		}
		if err := writer.Flush(); err != nil {
			done <- err
			return
		}
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			line = strings.TrimRight(line, "\r\n")
			commands <- line
			fields := strings.Fields(line)
			if len(fields) == 0 {
				done <- fmt.Errorf("empty imap command")
				return
			}
			tag := fields[0]
			upper := strings.ToUpper(line)
			switch {
			case strings.Contains(upper, "SELECT INBOX"):
				fmt.Fprintf(writer, "%s OK SELECT\r\n", tag)
			case strings.Contains(upper, "SEARCH UNSEEN"):
				fmt.Fprintf(writer, "* SEARCH 2\r\n%s OK SEARCH\r\n", tag)
			case strings.Contains(upper, "FETCH 2"):
				fmt.Fprintf(writer, "* 2 FETCH (RFC822 {%d}\r\n%s)\r\n%s OK FETCH\r\n", len("Subject: Hi\r\n\r\nBody"), "Subject: Hi\r\n\r\nBody", tag)
			case strings.Contains(upper, "STORE 2 -FLAGS"):
				fmt.Fprintf(writer, "%s OK STORE\r\n", tag)
			case strings.Contains(upper, "STORE 2 +FLAGS"):
				fmt.Fprintf(writer, "%s OK STORE\r\n", tag)
			case strings.Contains(upper, "CLOSE"):
				fmt.Fprintf(writer, "%s OK CLOSE\r\n", tag)
			case strings.Contains(upper, "LOGOUT"):
				fmt.Fprintf(writer, "%s OK LOGOUT\r\n", tag)
				if err := writer.Flush(); err != nil {
					done <- err
					return
				}
				close(commands)
				done <- nil
				return
			default:
				done <- fmt.Errorf("unexpected imap command %q", line)
				return
			}
			if err := writer.Flush(); err != nil {
				done <- err
				return
			}
		}
	}()
	conn := &imapConnection{conn: bufio.NewReadWriter(bufio.NewReader(client), bufio.NewWriter(client)), raw: client}
	if _, err := conn.readLine(); err != nil {
		t.Fatal(err)
	}
	count, err := conn.CheckUnreadMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unread count = %d", count)
	}
	messages, err := conn.RetrieveUnreadMessages(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Num != "2" || !strings.Contains(string(messages[0].Raw), "Subject: Hi") {
		t.Fatalf("messages = %+v", messages)
	}
	if err := conn.MarkHandled(context.Background(), messages[0]); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for command := range commands {
		got = append(got, command)
	}
	if len(got) != 7 ||
		!strings.Contains(got[0], "SELECT INBOX") ||
		!strings.Contains(got[1], "SEARCH UNSEEN") ||
		!strings.Contains(got[2], "FETCH 2") ||
		!strings.Contains(got[3], "STORE 2 -FLAGS") ||
		!strings.Contains(got[4], "STORE 2 +FLAGS") ||
		!strings.Contains(got[5], "CLOSE") ||
		!strings.Contains(got[6], "LOGOUT") {
		t.Fatalf("commands = %+v", got)
	}
}

func TestPOPConnectionDeletesHandledMessagesAndQuits(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	commands := make(chan string, 8)
	done := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		writer := bufio.NewWriter(server)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- err
				return
			}
			line = strings.TrimRight(line, "\r\n")
			commands <- line
			switch {
			case line == "STAT":
				fmt.Fprint(writer, "+OK 2 42\r\n")
			case line == "LIST":
				fmt.Fprint(writer, "+OK scan\r\n1 21\r\n2 21\r\n.\r\n")
			case line == "RETR 1":
				fmt.Fprint(writer, "+OK message\r\nSubject: POP\r\n\r\nBody\r\n.\r\n")
			case line == "DELE 1":
				fmt.Fprint(writer, "+OK deleted\r\n")
			case line == "QUIT":
				fmt.Fprint(writer, "+OK bye\r\n")
				if err := writer.Flush(); err != nil {
					done <- err
					return
				}
				close(commands)
				done <- nil
				return
			default:
				done <- fmt.Errorf("unexpected pop3 command %q", line)
				return
			}
			if err := writer.Flush(); err != nil {
				done <- err
				return
			}
		}
	}()
	conn := &pop3Connection{conn: textproto.NewConn(client)}
	count, err := conn.CheckUnreadMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("unread count = %d", count)
	}
	messages, err := conn.RetrieveUnreadMessages(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Num != "1" || !strings.Contains(string(messages[0].Raw), "Subject: POP") {
		t.Fatalf("messages = %+v", messages)
	}
	if err := conn.MarkHandled(context.Background(), messages[0]); err != nil {
		t.Fatal(err)
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for command := range commands {
		got = append(got, command)
	}
	want := []string{"STAT", "LIST", "RETR 1", "DELE 1", "QUIT"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %+v", got)
	}
}

func TestProcessFetchmailServersDeactivatesAfterRepeatedConnectionFailure(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{
		"object_id":     modelID,
		"password":      "secret-pass",
		"error_date":    now.Add(-fetchmailFailureWindow - time.Second),
		"error_message": "previous connection failure",
	})
	connector := &fakeFetchmailConnector{errors: map[int64]error{
		serverID: errors.New("auth failed for secret-pass"),
	}}

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{Now: now})
	if err == nil {
		t.Fatal("expected connection error")
	}
	if result.Fetched != 0 || result.Processed != 0 {
		t.Fatalf("result = %+v", result)
	}
	serverRows, err := env.Model("fetchmail.server").Browse(serverID).Read("state", "error_date", "error_message")
	if err != nil {
		t.Fatal(err)
	}
	if serverRows[0]["state"] != "draft" || !timeValue(serverRows[0]["error_date"]).Equal(now.Add(-fetchmailFailureWindow-time.Second)) || serverRows[0]["error_message"] != "previous connection failure" {
		t.Fatalf("server rows = %+v", serverRows)
	}
}

func TestProcessFetchmailServersNotifiesAdminWhenStaleFailureDeactivates(t *testing.T) {
	env, _ := threadEnv(t)
	now := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	modelID := fetchmailTestModelID(t, env, "res.partner")
	serverID := createFetchmailTestServer(t, env, map[string]any{
		"name":          "Failing Inbox",
		"object_id":     modelID,
		"error_date":    now.Add(-fetchmailFailureWindow - time.Second),
		"error_message": "previous connection failure",
	})
	connector := &fakeFetchmailConnector{errors: map[int64]error{serverID: errors.New("network down")}}
	var notifications []string

	result, err := ProcessFetchmailServers(context.Background(), env, connector, FetchmailOptions{
		Now: now,
		NotifyAdmin: func(message string) error {
			notifications = append(notifications, message)
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected connection error")
	}
	if result.Checked != 1 || result.Fetched != 0 || result.Processed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(notifications) != 1 || notifications[0] != "Deactivating fetchmail imap server Failing Inbox (too many failures)" {
		t.Fatalf("notifications = %+v", notifications)
	}
	serverRows, err := env.Model("fetchmail.server").Browse(serverID).Read("state", "error_date", "error_message")
	if err != nil {
		t.Fatal(err)
	}
	if serverRows[0]["state"] != "draft" || !timeValue(serverRows[0]["error_date"]).Equal(now.Add(-fetchmailFailureWindow-time.Second)) || serverRows[0]["error_message"] != "previous connection failure" {
		t.Fatalf("server rows = %+v", serverRows)
	}
}

func TestNotifyAdminChannelPostsToConfiguredAdminChannel(t *testing.T) {
	env, _ := threadEnv(t)
	channelID, err := env.Model("discuss.channel").Create(map[string]any{"name": "Administrators", "channel_type": "channel", "active": true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Model("ir.model.data").Create(map[string]any{
		"module":        "mail",
		"name":          "channel_admin",
		"complete_name": "mail.channel_admin",
		"model":         "discuss.channel",
		"res_id":        channelID,
	}); err != nil {
		t.Fatal(err)
	}

	if err := NotifyAdminChannel(env, "Deactivating fetchmail imap server Administrators (too many failures)"); err != nil {
		t.Fatal(err)
	}
	found, err := env.Model("mail.message").Search(domain.And(
		domain.Cond("model", "=", "discuss.channel"),
		domain.Cond("res_id", "=", channelID),
	))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := found.Read("body", "message_type")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0]["message_type"] != "comment" || !strings.Contains(stringAny(rows[0]["body"]), "Deactivating fetchmail imap server Administrators") {
		t.Fatalf("admin channel messages = %+v", rows)
	}
}

func fetchmailTestModelID(t *testing.T, env *record.Env, modelName string) int64 {
	t.Helper()
	id, err := env.Model("ir.model").Create(map[string]any{"model": modelName, "name": modelName})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func createFetchmailTestServer(t *testing.T, env *record.Env, vals map[string]any) int64 {
	t.Helper()
	base := map[string]any{
		"name":        "Inbound",
		"active":      true,
		"state":       "done",
		"server":      "mock",
		"server_type": "imap",
		"priority":    5,
		"attach":      true,
	}
	for key, value := range vals {
		base[key] = value
	}
	id, err := env.Model("fetchmail.server").Create(base)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
