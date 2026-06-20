package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorp/internal/domain"
	"gorp/internal/record"
)

const (
	DefaultFetchmailBatchLimit = 50
	fetchmailFailureWindow     = 5 * 24 * time.Hour
)

type FetchmailServerConfig struct {
	ID         int64
	Name       string
	Active     bool
	State      string
	Server     string
	Port       int
	ServerType string
	IsSSL      bool
	User       string
	Password   string
	ObjectID   int64
	Attach     bool
	Original   bool
	Priority   int
	Date       time.Time
	ErrorDate  time.Time
}

type FetchedMessage struct {
	Num string
	Raw []byte
}

type FetchmailConnection interface {
	CheckUnreadMessages(context.Context) (int, error)
	RetrieveUnreadMessages(context.Context, int) ([]FetchedMessage, error)
	MarkHandled(context.Context, FetchedMessage) error
	Close() error
}

type FetchmailConnector interface {
	Connect(context.Context, FetchmailServerConfig) (FetchmailConnection, error)
}

type FetchmailServerLocker interface {
	TryLockFetchmailServer(int64) (func(), bool, error)
}

type FetchmailProgressFunc func(processed int, remaining *int, deactivate bool) bool

type FetchmailAdminNotifyFunc func(message string) error

type FetchmailServerLockFunc func(int64) (func(), bool, error)

func (f FetchmailServerLockFunc) TryLockFetchmailServer(serverID int64) (func(), bool, error) {
	if f == nil {
		return func() {}, true, nil
	}
	return f(serverID)
}

type FetchmailOptions struct {
	ServerIDs       []int64
	BatchLimit      int
	Now             time.Time
	MessageIDLocker InboundMessageIDLocker
	ServerLocker    FetchmailServerLocker
	Progress        FetchmailProgressFunc
	NotifyAdmin     FetchmailAdminNotifyFunc
	CronEndTime     time.Time
	TimeBudget      time.Duration
	Clock           func() time.Time
}

type FetchmailResult struct {
	Servers   int
	Checked   int
	Fetched   int
	Processed int
	Failed    int
	Skipped   int
	Remaining int
}

func ConfirmFetchmailServers(ctx context.Context, env *record.Env, connector FetchmailConnector, ids []int64, now time.Time) error {
	if env == nil {
		return fmt.Errorf("fetchmail confirm requires env")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	servers, err := fetchmailServersByID(env, ids)
	if err != nil {
		return err
	}
	for _, server := range servers {
		if err := verifyFetchmailConnection(ctx, connector, server); err != nil {
			return err
		}
		if err := env.Model("fetchmail.server").Browse(server.ID).Write(map[string]any{
			"state": "done",
		}); err != nil {
			return err
		}
	}
	return nil
}

func ProcessFetchmailServers(ctx context.Context, env *record.Env, connector FetchmailConnector, options FetchmailOptions) (FetchmailResult, error) {
	if env == nil {
		return FetchmailResult{}, fmt.Errorf("fetchmail processing requires env")
	}
	env = fetchmailCronEnv(env)
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	}
	if options.BatchLimit <= 0 {
		options.BatchLimit = DefaultFetchmailBatchLimit
	}
	servers, err := fetchmailServers(env, options.ServerIDs)
	if err != nil {
		return FetchmailResult{}, err
	}
	result := FetchmailResult{}
	var firstErr error
	clock := fetchmailClock(options.Clock, options.Now)
	progress := newFetchmailProgress(servers, options.Progress, fetchmailDeadline(options, servers, clock), clock)
	for _, server := range servers {
		if progress.stopped() {
			break
		}
		result.Servers++
		if !server.Active || server.State != "done" || server.ServerType == "local" {
			result.Skipped++
			continue
		}
		progress.checkingServer()
		unlock, locked, err := lockFetchmailServer(options.ServerLocker, server.ID)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			result.Skipped++
			continue
		}
		if !locked {
			result.Skipped++
			continue
		}
		serverResult, err := func() (FetchmailResult, error) {
			defer unlock()
			return processFetchmailServer(ctx, env, connector, server, options.Now, options.BatchLimit, options.MessageIDLocker, progress, options.NotifyAdmin)
		}()
		if err != nil && firstErr == nil {
			firstErr = err
		}
		result.Checked += serverResult.Checked
		result.Fetched += serverResult.Fetched
		result.Processed += serverResult.Processed
		result.Failed += serverResult.Failed
		result.Skipped += serverResult.Skipped
		result.Remaining += serverResult.Remaining
	}
	return result, firstErr
}

type fetchmailProgressState struct {
	commit    FetchmailProgressFunc
	remaining int
	stop      bool
	deadline  time.Time
	clock     func() time.Time
}

func newFetchmailProgress(servers []FetchmailServerConfig, commit FetchmailProgressFunc, deadline time.Time, clock func() time.Time) *fetchmailProgressState {
	state := &fetchmailProgressState{commit: commit, deadline: deadline, clock: clock}
	if commit == nil {
		return state
	}
	for _, server := range servers {
		if server.Active && server.State == "done" && server.ServerType != "local" {
			state.remaining++
		}
	}
	remaining := state.remaining
	if !commit(0, &remaining, false) {
		state.stop = true
	}
	return state
}

func fetchmailClock(clock func() time.Time, base time.Time) func() time.Time {
	if clock != nil {
		return clock
	}
	if !base.IsZero() {
		started := time.Now()
		return func() time.Time { return base.Add(time.Since(started)).UTC() }
	}
	return func() time.Time { return time.Now().UTC() }
}

func fetchmailDeadline(options FetchmailOptions, servers []FetchmailServerConfig, clock func() time.Time) time.Time {
	deadline := options.CronEndTime
	if deadline.IsZero() && options.TimeBudget > 0 {
		base := options.Now
		if base.IsZero() {
			base = clock()
		}
		deadline = base.Add(options.TimeBudget)
	}
	if deadline.IsZero() {
		return time.Time{}
	}
	return deadline.Add(4 * time.Second * time.Duration(fetchmailEligibleServerCount(servers)))
}

func fetchmailEligibleServerCount(servers []FetchmailServerConfig) int {
	count := 0
	for _, server := range servers {
		if server.Active && server.State == "done" && server.ServerType != "local" {
			count++
		}
	}
	return count
}

func (p *fetchmailProgressState) checkingServer() {
	if p == nil || p.commit == nil {
		return
	}
	if p.remaining > 0 {
		p.remaining--
	}
}

func (p *fetchmailProgressState) addUnread(count int) {
	if p == nil || p.commit == nil || count <= 0 {
		return
	}
	p.remaining += count
}

func (p *fetchmailProgressState) messageDone() bool {
	if p == nil {
		return true
	}
	if p.commit == nil {
		p.stop = !p.hasTime()
		return !p.stop
	}
	if p.remaining > 0 {
		p.remaining--
	}
	if !p.commit(1, nil, false) || !p.hasTime() {
		p.stop = true
	}
	return !p.stop
}

func (p *fetchmailProgressState) serverDone() bool {
	if p == nil {
		return true
	}
	if p.commit == nil {
		p.stop = !p.hasTime()
		return !p.stop
	}
	remaining := p.remaining
	if !p.commit(1, &remaining, false) || !p.hasTime() {
		p.stop = true
	}
	return !p.stop
}

func (p *fetchmailProgressState) stopped() bool {
	return p != nil && p.stop
}

func (p *fetchmailProgressState) hasTime() bool {
	if p == nil || p.deadline.IsZero() {
		return true
	}
	return p.clock().Before(p.deadline)
}

func fetchmailCronEnv(env *record.Env) *record.Env {
	context := env.Context()
	values := map[string]any{}
	for key, value := range context.Values {
		values[key] = value
	}
	values["fetchmail_cron_running"] = true
	context.Values = values
	return env.WithContext(context)
}

func processFetchmailServer(ctx context.Context, env *record.Env, connector FetchmailConnector, server FetchmailServerConfig, now time.Time, remaining int, messageLocker InboundMessageIDLocker, progress *fetchmailProgressState, notifyAdmin FetchmailAdminNotifyFunc) (FetchmailResult, error) {
	refreshed, err := fetchmailServersByID(env, []int64{server.ID})
	if err != nil {
		return FetchmailResult{}, err
	}
	if len(refreshed) == 0 {
		return FetchmailResult{Skipped: 1}, nil
	}
	server = refreshed[0]
	if !server.Active || server.State != "done" || server.ServerType == "local" {
		return FetchmailResult{Skipped: 1}, nil
	}
	if strings.TrimSpace(server.Server) == "" {
		result := FetchmailResult{Checked: 1, Skipped: 1}
		if err := writeFetchmailSuccess(env, server.ID, now); err != nil {
			return result, err
		}
		progress.serverDone()
		return result, nil
	}
	conn, err := fetchmailConnector(connector).Connect(ctx, server)
	if err != nil {
		firstErr := err
		if writeErr := handleFetchmailConnectionError(env, server, now, err, notifyAdmin); writeErr != nil && firstErr == nil {
			firstErr = writeErr
		}
		if writeErr := writeFetchmailDate(env, server.ID, now); writeErr != nil && firstErr == nil {
			firstErr = writeErr
		}
		progress.serverDone()
		return FetchmailResult{Checked: 1}, firstErr
	}
	serverResult, processErr := processFetchmailConnection(ctx, env, conn, server, now, remaining, messageLocker, progress)
	serverResult.Checked = 1
	var firstErr error
	if processErr != nil {
		firstErr = processErr
	}
	if err := conn.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	if processErr != nil {
		if writeErr := handleFetchmailConnectionError(env, server, now, processErr, notifyAdmin); writeErr != nil && firstErr == nil {
			firstErr = writeErr
		}
		if writeErr := writeFetchmailDate(env, server.ID, now); writeErr != nil && firstErr == nil {
			firstErr = writeErr
		}
		progress.serverDone()
		return serverResult, firstErr
	}
	if err := writeFetchmailSuccess(env, server.ID, now); err != nil && firstErr == nil {
		firstErr = err
	}
	progress.serverDone()
	return serverResult, firstErr
}

func verifyFetchmailConnection(ctx context.Context, connector FetchmailConnector, server FetchmailServerConfig) error {
	if server.ServerType == "local" || strings.TrimSpace(server.Server) == "" {
		return nil
	}
	conn, err := fetchmailConnector(connector).Connect(ctx, server)
	if err != nil {
		return err
	}
	return conn.Close()
}

func processFetchmailConnection(ctx context.Context, env *record.Env, conn FetchmailConnection, server FetchmailServerConfig, now time.Time, limit int, locker InboundMessageIDLocker, progress *fetchmailProgressState) (FetchmailResult, error) {
	result := FetchmailResult{}
	if limit <= 0 {
		return result, nil
	}
	unreadCount, err := conn.CheckUnreadMessages(ctx)
	if err != nil {
		return result, err
	}
	progress.addUnread(unreadCount)
	messages, err := conn.RetrieveUnreadMessages(ctx, limit)
	if err != nil {
		return result, err
	}
	modelName, err := fetchmailObjectModel(env, server.ObjectID)
	if err != nil {
		return result, err
	}
	for _, message := range messages {
		if len(message.Raw) == 0 {
			continue
		}
		result.Fetched++
		snapshot := env.Snapshot()
		_, err := ProcessInboundEmailWithOptions(env, message.Raw, InboundProcessOptions{
			FallbackModel:    modelName,
			SaveOriginal:     server.Original,
			StripAttachments: !server.Attach,
			Now:              now,
			MessageIDLocker:  locker,
		})
		if err != nil {
			env.Restore(snapshot)
			result.Failed++
		} else {
			result.Processed++
		}
		continueRun := progress.messageDone()
		if err := conn.MarkHandled(ctx, message); err != nil {
			return result, err
		}
		if !continueRun {
			break
		}
	}
	if unreadCount > result.Fetched {
		result.Remaining = unreadCount - result.Fetched
	}
	return result, nil
}

func fetchmailServers(env *record.Env, ids []int64) ([]FetchmailServerConfig, error) {
	if len(ids) > 0 {
		return fetchmailServersByID(env, ids)
	}
	found, err := env.Model("fetchmail.server").SearchWithOptions(
		domain.And(
			domain.Cond("state", "=", "done"),
			domain.Cond("server_type", "!=", "local"),
		),
		record.SearchOptions{Order: "priority asc, date asc nulls first, id asc"},
	)
	if err != nil {
		return nil, err
	}
	rows, err := found.Read("id")
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, int64FromAny(row["id"]))
	}
	return fetchmailServersByID(env, out)
}

func fetchmailServersByID(env *record.Env, ids []int64) ([]FetchmailServerConfig, error) {
	ids = uniqueIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := env.Model("fetchmail.server").Browse(ids...).Read("id", "name", "active", "state", "server", "port", "server_type", "is_ssl", "user", "password", "object_id", "attach", "original", "priority", "date", "error_date")
	if err != nil {
		return nil, err
	}
	servers := make([]FetchmailServerConfig, 0, len(rows))
	for _, row := range rows {
		serverType := strings.TrimSpace(stringAny(row["server_type"]))
		if serverType == "" {
			serverType = "imap"
		}
		active := true
		if value, ok := row["active"]; ok && value != nil {
			active = boolAny(value)
		}
		attach := true
		if value, ok := row["attach"]; ok && value != nil {
			attach = boolAny(value)
		}
		servers = append(servers, FetchmailServerConfig{
			ID:         int64FromAny(row["id"]),
			Name:       stringAny(row["name"]),
			Active:     active,
			State:      stringAny(row["state"]),
			Server:     stringAny(row["server"]),
			Port:       int(int64FromAny(row["port"])),
			ServerType: serverType,
			IsSSL:      boolAny(row["is_ssl"]),
			User:       stringAny(row["user"]),
			Password:   stringAny(row["password"]),
			ObjectID:   int64FromAny(row["object_id"]),
			Attach:     attach,
			Original:   boolAny(row["original"]),
			Priority:   int(int64FromAny(row["priority"])),
			Date:       timeValue(row["date"]),
			ErrorDate:  timeValue(row["error_date"]),
		})
	}
	sort.SliceStable(servers, func(i, j int) bool {
		if servers[i].Priority != servers[j].Priority {
			return servers[i].Priority < servers[j].Priority
		}
		if servers[i].Date.IsZero() != servers[j].Date.IsZero() {
			return servers[i].Date.IsZero()
		}
		if !servers[i].Date.Equal(servers[j].Date) {
			return servers[i].Date.Before(servers[j].Date)
		}
		return servers[i].ID < servers[j].ID
	})
	return servers, nil
}

func writeFetchmailSuccess(env *record.Env, id int64, now time.Time) error {
	if id == 0 {
		return nil
	}
	return env.Model("fetchmail.server").Browse(id).Write(map[string]any{
		"date":          now.UTC(),
		"error_date":    nil,
		"error_message": "",
	})
}

func writeFetchmailDate(env *record.Env, id int64, now time.Time) error {
	if id == 0 {
		return nil
	}
	return env.Model("fetchmail.server").Browse(id).Write(map[string]any{"date": now.UTC()})
}

func writeFetchmailError(env *record.Env, id int64, now time.Time, err error, secrets ...string) error {
	if id == 0 {
		return nil
	}
	return env.Model("fetchmail.server").Browse(id).Write(map[string]any{
		"error_date":    now.UTC(),
		"error_message": safeFetchmailError(err, secrets...),
	})
}

func handleFetchmailConnectionError(env *record.Env, server FetchmailServerConfig, now time.Time, err error, notifyAdmin FetchmailAdminNotifyFunc) error {
	if server.ErrorDate.IsZero() {
		return writeFetchmailError(env, server.ID, now, err, server.Password)
	}
	if server.ErrorDate.Before(now.Add(-fetchmailFailureWindow)) {
		if writeErr := env.Model("fetchmail.server").Browse(server.ID).Write(map[string]any{
			"state": "draft",
		}); writeErr != nil {
			return writeErr
		}
		if notifyAdmin != nil {
			return notifyAdmin(fmt.Sprintf("Deactivating fetchmail %s server %s (too many failures)", strings.TrimSpace(server.ServerType), strings.TrimSpace(server.Name)))
		}
	}
	return nil
}

func NotifyAdminChannel(env *record.Env, message string) error {
	if env == nil || strings.TrimSpace(message) == "" {
		return nil
	}
	channelID := adminChannelID(env)
	if channelID == 0 {
		return nil
	}
	_, err := PostMessage(messageSystemEnv(env), PostRequest{
		Model:       "discuss.channel",
		ResID:       channelID,
		Body:        message,
		MessageType: "comment",
	})
	return err
}

func adminChannelID(env *record.Env) int64 {
	if env == nil {
		return 0
	}
	if _, ok := env.ModelMetadata("ir.model.data"); !ok {
		return 0
	}
	found, err := env.Model("ir.model.data").SearchWithOptions(domain.Cond("complete_name", "=", "mail.channel_admin"), record.SearchOptions{Limit: 1})
	if err != nil || found.Len() == 0 {
		return 0
	}
	rows, err := found.Read("model", "res_id")
	if err != nil || len(rows) == 0 || strings.TrimSpace(stringAny(rows[0]["model"])) != "discuss.channel" {
		return 0
	}
	return int64FromAny(rows[0]["res_id"])
}

func safeFetchmailError(err error, secrets ...string) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	if len(text) > 1000 {
		return text[:1000]
	}
	return text
}

func fetchmailObjectModel(env *record.Env, objectID int64) (string, error) {
	if env == nil || objectID == 0 {
		return "", nil
	}
	rows, err := env.Model("ir.model").Browse(objectID).Read("model")
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return strings.TrimSpace(stringAny(rows[0]["model"])), nil
}

func fetchmailConnector(connector FetchmailConnector) FetchmailConnector {
	if connector != nil {
		return connector
	}
	return NetworkFetchmailConnector{}
}

func ActiveFetchmailServerCount(env *record.Env) (int, error) {
	if env == nil {
		return 0, nil
	}
	found, err := env.Model("fetchmail.server").Search(domain.And(
		domain.Cond("active", "=", true),
		domain.Cond("state", "=", "done"),
		domain.Cond("server_type", "!=", "local"),
	))
	if err != nil {
		return 0, err
	}
	return found.Len(), nil
}

var localFetchmailServerLocks sync.Map

type LocalFetchmailServerLocker struct{}

func (LocalFetchmailServerLocker) TryLockFetchmailServer(serverID int64) (func(), bool, error) {
	if serverID == 0 {
		return func() {}, true, nil
	}
	if _, loaded := localFetchmailServerLocks.LoadOrStore(serverID, struct{}{}); loaded {
		return func() {}, false, nil
	}
	return func() { localFetchmailServerLocks.Delete(serverID) }, true, nil
}

func lockFetchmailServer(locker FetchmailServerLocker, serverID int64) (func(), bool, error) {
	if locker == nil {
		locker = LocalFetchmailServerLocker{}
	}
	unlock, locked, err := locker.TryLockFetchmailServer(serverID)
	if unlock == nil {
		unlock = func() {}
	}
	return unlock, locked, err
}

type NetworkFetchmailConnector struct{}

func (NetworkFetchmailConnector) Connect(ctx context.Context, server FetchmailServerConfig) (FetchmailConnection, error) {
	switch strings.ToLower(strings.TrimSpace(server.ServerType)) {
	case "pop":
		return connectPOP3(ctx, server)
	case "imap", "gmail", "outlook", "":
		return connectIMAP(ctx, server)
	default:
		return nil, fmt.Errorf("unsupported fetchmail server type %q", server.ServerType)
	}
}

func defaultFetchmailPort(server FetchmailServerConfig) int {
	if server.Port > 0 {
		return server.Port
	}
	switch strings.ToLower(strings.TrimSpace(server.ServerType)) {
	case "pop":
		if server.IsSSL {
			return 995
		}
		return 110
	default:
		if server.IsSSL {
			return 993
		}
		return 143
	}
}

func dialFetchmail(ctx context.Context, server FetchmailServerConfig) (net.Conn, error) {
	addr := net.JoinHostPort(strings.TrimSpace(server.Server), strconv.Itoa(defaultFetchmailPort(server)))
	dialer := net.Dialer{Timeout: 60 * time.Second}
	if server.IsSSL {
		return tls.DialWithDialer(&dialer, "tcp", addr, &tls.Config{ServerName: strings.TrimSpace(server.Server), MinVersion: tls.VersionTLS12})
	}
	return dialer.DialContext(ctx, "tcp", addr)
}

type pop3Connection struct {
	conn *textproto.Conn
	nums []int
}

func connectPOP3(ctx context.Context, server FetchmailServerConfig) (*pop3Connection, error) {
	raw, err := dialFetchmail(ctx, server)
	if err != nil {
		return nil, err
	}
	conn := textproto.NewConn(raw)
	if line, err := conn.ReadLine(); err != nil || !strings.HasPrefix(line, "+OK") {
		_ = conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("pop3 greeting failed")
	}
	p := &pop3Connection{conn: conn}
	if _, err := p.command("USER " + server.User); err != nil {
		_ = p.Close()
		return nil, err
	}
	if _, err := p.command("PASS " + server.Password); err != nil {
		_ = p.Close()
		return nil, err
	}
	return p, nil
}

func (p *pop3Connection) CheckUnreadMessages(context.Context) (int, error) {
	line, err := p.command("STAT")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("invalid pop3 STAT response")
	}
	count, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, err
	}
	if _, err := p.command("LIST"); err != nil {
		return 0, err
	}
	lines, err := p.conn.ReadDotLines()
	if err != nil {
		return 0, err
	}
	p.nums = p.nums[:0]
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		num, err := strconv.Atoi(fields[0])
		if err != nil {
			return 0, err
		}
		p.nums = append(p.nums, num)
	}
	if len(p.nums) == 0 && count > 0 {
		p.nums = make([]int, count)
		for i := 1; i <= count; i++ {
			p.nums[i-1] = i
		}
	}
	return len(p.nums), nil
}

func (p *pop3Connection) RetrieveUnreadMessages(_ context.Context, limit int) ([]FetchedMessage, error) {
	out := []FetchedMessage{}
	for _, num := range p.nums {
		if limit > 0 && len(out) >= limit {
			break
		}
		if _, err := p.command("RETR " + strconv.Itoa(num)); err != nil {
			return out, err
		}
		data, err := p.conn.ReadDotBytes()
		if err != nil {
			return out, err
		}
		out = append(out, FetchedMessage{Num: strconv.Itoa(num), Raw: data})
	}
	return out, nil
}

func (p *pop3Connection) MarkHandled(_ context.Context, message FetchedMessage) error {
	_, err := p.command("DELE " + message.Num)
	return err
}

func (p *pop3Connection) Close() error {
	if p == nil || p.conn == nil {
		return nil
	}
	_, _ = p.command("QUIT")
	return p.conn.Close()
}

func (p *pop3Connection) command(command string) (string, error) {
	if err := p.conn.PrintfLine(command); err != nil {
		return "", err
	}
	line, err := p.conn.ReadLine()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(line, "+OK") {
		return "", fmt.Errorf("pop3 command failed: %s", line)
	}
	return line, nil
}

type imapConnection struct {
	conn *bufio.ReadWriter
	raw  net.Conn
	tag  atomic.Int64
	nums []string
}

func connectIMAP(ctx context.Context, server FetchmailServerConfig) (*imapConnection, error) {
	raw, err := dialFetchmail(ctx, server)
	if err != nil {
		return nil, err
	}
	conn := bufio.NewReadWriter(bufio.NewReader(raw), bufio.NewWriter(raw))
	i := &imapConnection{conn: conn, raw: raw}
	if _, err := i.readLine(); err != nil {
		_ = i.Close()
		return nil, err
	}
	if _, err := i.command("LOGIN %s %s", imapQuote(server.User), imapQuote(server.Password)); err != nil {
		_ = i.Close()
		return nil, err
	}
	return i, nil
}

func (i *imapConnection) CheckUnreadMessages(context.Context) (int, error) {
	if _, err := i.command("SELECT INBOX"); err != nil {
		return 0, err
	}
	i.nums = nil
	lines, err := i.command("SEARCH UNSEEN")
	if err != nil {
		return 0, err
	}
	for _, line := range lines {
		if strings.HasPrefix(strings.ToUpper(line), "* SEARCH") {
			fields := strings.Fields(line)
			if len(fields) > 2 {
				i.nums = append([]string(nil), fields[2:]...)
			}
		}
	}
	return len(i.nums), nil
}

func (i *imapConnection) RetrieveUnreadMessages(_ context.Context, limit int) ([]FetchedMessage, error) {
	out := []FetchedMessage{}
	for _, num := range i.nums {
		if limit > 0 && len(out) >= limit {
			break
		}
		raw, err := i.fetchRFC822(num)
		if err != nil {
			return out, err
		}
		if _, err := i.command("STORE %s -FLAGS (\\Seen)", num); err != nil {
			return out, err
		}
		out = append(out, FetchedMessage{Num: num, Raw: raw})
	}
	return out, nil
}

func (i *imapConnection) MarkHandled(_ context.Context, message FetchedMessage) error {
	_, err := i.command("STORE %s +FLAGS (\\Seen)", message.Num)
	return err
}

func (i *imapConnection) Close() error {
	if i == nil || i.raw == nil {
		return nil
	}
	_, _ = i.command("CLOSE")
	_, _ = i.command("LOGOUT")
	return i.raw.Close()
}

func (i *imapConnection) command(format string, args ...any) ([]string, error) {
	tag := fmt.Sprintf("A%04d", i.tag.Add(1))
	if _, err := fmt.Fprintf(i.conn, tag+" "+format+"\r\n", args...); err != nil {
		return nil, err
	}
	if err := i.conn.Flush(); err != nil {
		return nil, err
	}
	lines := []string{}
	for {
		line, err := i.readLine()
		if err != nil {
			return lines, err
		}
		lines = append(lines, line)
		if strings.HasPrefix(line, tag+" ") {
			if strings.Contains(strings.ToUpper(line), " OK") {
				return lines, nil
			}
			return lines, fmt.Errorf("imap command failed: %s", line)
		}
	}
}

func (i *imapConnection) fetchRFC822(num string) ([]byte, error) {
	tag := fmt.Sprintf("A%04d", i.tag.Add(1))
	if _, err := fmt.Fprintf(i.conn, "%s FETCH %s (RFC822)\r\n", tag, num); err != nil {
		return nil, err
	}
	if err := i.conn.Flush(); err != nil {
		return nil, err
	}
	var payload []byte
	for {
		line, err := i.readLine()
		if err != nil {
			return payload, err
		}
		if size, ok := imapLiteralSize(line); ok {
			payload = make([]byte, size)
			if _, err := ioReadFull(i.conn.Reader, payload); err != nil {
				return payload, err
			}
			continue
		}
		if strings.HasPrefix(line, tag+" ") {
			if strings.Contains(strings.ToUpper(line), " OK") {
				return payload, nil
			}
			return payload, fmt.Errorf("imap fetch failed: %s", line)
		}
	}
}

func (i *imapConnection) readLine() (string, error) {
	line, err := i.conn.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func imapLiteralSize(line string) (int, bool) {
	start := strings.LastIndex(line, "{")
	end := strings.LastIndex(line, "}")
	if start == -1 || end == -1 || end <= start+1 {
		return 0, false
	}
	size, err := strconv.Atoi(line[start+1 : end])
	return size, err == nil
}

func imapQuote(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func ioReadFull(reader *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := reader.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	if total != len(buf) {
		return total, errors.New("short read")
	}
	return total, nil
}
