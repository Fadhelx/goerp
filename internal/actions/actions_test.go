package actions

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRegistryRunsRegisteredGoAction(t *testing.T) {
	registry := NewRegistry(Hooks{})
	err := registry.RegisterGo("mark.done", func(_ context.Context, action ServerAction, exec ExecutionContext) (Result, error) {
		if action.Name != "Mark Done" {
			t.Fatalf("action name = %q", action.Name)
		}
		if exec.RecordID != 42 {
			t.Fatalf("record id = %d", exec.RecordID)
		}
		return Result{Metadata: map[string]any{"done": true}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	id, err := registry.Register(ServerAction{Name: "Mark Done", Kind: KindGo, GoActionName: "mark.done"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), id, ExecutionContext{Model: "res.partner", RecordID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActionID != id || result.Kind != KindGo || result.GoActionName != "mark.done" {
		t.Fatalf("result = %+v", result)
	}
	if result.Metadata["done"] != true {
		t.Fatalf("metadata = %+v", result.Metadata)
	}
}

func TestRegistryRunsNamedGoAction(t *testing.T) {
	registry := NewRegistry(Hooks{})
	err := registry.RegisterGo("base.autovacuum", func(_ context.Context, action ServerAction, exec ExecutionContext) (Result, error) {
		if action.Kind != KindGo || action.GoActionName != "base.autovacuum" {
			t.Fatalf("action = %+v", action)
		}
		if exec.Trigger != "cron" || exec.Metadata["cron_name"] != "Auto Vacuum" {
			t.Fatalf("exec = %+v", exec)
		}
		return Result{Metadata: map[string]any{"ok": true}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.RunNamed(context.Background(), "base.autovacuum", ExecutionContext{
		Trigger:  "cron",
		Metadata: map[string]any{"cron_name": "Auto Vacuum"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != KindGo || result.GoActionName != "base.autovacuum" || result.Metadata["ok"] != true {
		t.Fatalf("result = %+v", result)
	}
}

func TestRegistryRunsCreateAndWriteHooks(t *testing.T) {
	creator := &captureCreator{id: 7}
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Creator: creator, Writer: writer})
	createID, err := registry.Register(ServerAction{
		Name:   "Create Partner",
		Model:  "res.partner",
		Kind:   KindCreate,
		Values: map[string]any{"name": "Ada"},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeID, err := registry.Register(ServerAction{
		Name:   "Write Partner",
		Model:  "res.partner",
		Kind:   KindWrite,
		Values: map[string]any{"active": false},
	})
	if err != nil {
		t.Fatal(err)
	}

	createResult, err := registry.Run(context.Background(), createID, ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if createResult.CreatedID != 7 {
		t.Fatalf("created id = %d", createResult.CreatedID)
	}
	if creator.model != "res.partner" || !reflect.DeepEqual(creator.values, map[string]any{"name": "Ada"}) {
		t.Fatalf("create call = %s %+v", creator.model, creator.values)
	}

	writeResult, err := registry.Run(context.Background(), writeID, ExecutionContext{RecordIDs: []int64{3, 4}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(writeResult.WrittenIDs, []int64{4}) {
		t.Fatalf("written ids = %+v", writeResult.WrittenIDs)
	}
	if len(writer.calls) != 2 || !reflect.DeepEqual(writer.calls[0].ids, []int64{3}) || !reflect.DeepEqual(writer.calls[1].ids, []int64{4}) {
		t.Fatalf("write calls = %+v", writer.calls)
	}
	if writer.model != "res.partner" || !reflect.DeepEqual(writer.ids, []int64{4}) {
		t.Fatalf("last write call = %s %+v", writer.model, writer.ids)
	}
	if !reflect.DeepEqual(writer.values, map[string]any{"active": false}) {
		t.Fatalf("write values = %+v", writer.values)
	}
}

func TestRegistrySplitsSingletonWritePerActiveRecord(t *testing.T) {
	operator := &captureObjectOperator{}
	registry := NewRegistry(Hooks{ObjectOperator: operator})
	id, err := registry.Register(ServerAction{
		Name:   "Write Partner",
		Model:  "res.partner",
		Kind:   KindWrite,
		Values: map[string]any{"active": false},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), id, ExecutionContext{
		Model:     "res.partner",
		RecordID:  99,
		RecordIDs: []int64{1, 2},
		Values:    map[string]any{"active_id": int64(99), "active_ids": []int64{1, 2}},
		Metadata:  map[string]any{"active_id": int64(99), "active_ids": []int64{1, 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.WrittenIDs, []int64{2}) {
		t.Fatalf("written ids = %+v", result.WrittenIDs)
	}
	if len(operator.writeCalls) != 2 {
		t.Fatalf("write calls = %+v", operator.writeCalls)
	}
	for index, wantID := range []int64{1, 2} {
		call := operator.writeCalls[index]
		if call.exec.RecordID != wantID || !reflect.DeepEqual(call.exec.RecordIDs, []int64{wantID}) {
			t.Fatalf("call %d exec = %+v", index, call.exec)
		}
		if !reflect.DeepEqual(call.action.RecordIDs, []int64{wantID}) {
			t.Fatalf("call %d action record ids = %+v", index, call.action.RecordIDs)
		}
		if call.exec.Values["active_id"] != wantID || !reflect.DeepEqual(call.exec.Values["active_ids"], []int64{wantID}) {
			t.Fatalf("call %d values = %+v", index, call.exec.Values)
		}
		if call.exec.Metadata["active_id"] != wantID || !reflect.DeepEqual(call.exec.Metadata["active_ids"], []int64{wantID}) {
			t.Fatalf("call %d metadata = %+v", index, call.exec.Metadata)
		}
		if call.exec.Values["active_model"] != "res.partner" || call.exec.Metadata["active_model"] != "res.partner" {
			t.Fatalf("call %d active model values=%+v metadata=%+v", index, call.exec.Values, call.exec.Metadata)
		}
	}
}

func TestRegistrySplitsSingletonCreateAndCopyPerActiveRecord(t *testing.T) {
	operator := &captureObjectOperator{created: 21, copiedID: 22}
	registry := NewRegistry(Hooks{ObjectOperator: operator})
	createID, err := registry.Register(ServerAction{
		Name:   "Create Child",
		Model:  "res.partner",
		Kind:   KindCreate,
		Values: map[string]any{"name": "Child"},
	})
	if err != nil {
		t.Fatal(err)
	}
	copyID, err := registry.Register(ServerAction{
		Name:        "Copy Child",
		Model:       "res.partner",
		Kind:        KindCopy,
		ResourceRef: "res.partner,7",
	})
	if err != nil {
		t.Fatal(err)
	}

	createResult, err := registry.Run(context.Background(), createID, ExecutionContext{Model: "res.partner", RecordIDs: []int64{4, 5}})
	if err != nil {
		t.Fatal(err)
	}
	if createResult.CreatedID != 21 || len(operator.createCalls) != 2 {
		t.Fatalf("create result=%+v calls=%+v", createResult, operator.createCalls)
	}
	if operator.createCalls[0].exec.RecordID != 4 || operator.createCalls[1].exec.RecordID != 5 {
		t.Fatalf("create calls = %+v", operator.createCalls)
	}
	for index, call := range operator.createCalls {
		wantID := int64(index + 4)
		if !reflect.DeepEqual(call.exec.RecordIDs, []int64{wantID}) || !reflect.DeepEqual(call.action.RecordIDs, []int64{wantID}) {
			t.Fatalf("create call %d = %+v", index, call)
		}
		if call.exec.Values["active_id"] != wantID || !reflect.DeepEqual(call.exec.Values["active_ids"], []int64{wantID}) {
			t.Fatalf("create call %d values = %+v", index, call.exec.Values)
		}
	}

	copyResult, err := registry.Run(context.Background(), copyID, ExecutionContext{Model: "res.partner", RecordIDs: []int64{4, 5}})
	if err != nil {
		t.Fatal(err)
	}
	if copyResult.CreatedID != 22 || len(operator.copyCalls) != 2 {
		t.Fatalf("copy result=%+v calls=%+v", copyResult, operator.copyCalls)
	}
	if operator.copyCalls[0].exec.RecordID != 4 || operator.copyCalls[1].exec.RecordID != 5 {
		t.Fatalf("copy calls = %+v", operator.copyCalls)
	}
	for index, call := range operator.copyCalls {
		wantID := int64(index + 4)
		if !reflect.DeepEqual(call.exec.RecordIDs, []int64{wantID}) || !reflect.DeepEqual(call.action.RecordIDs, []int64{wantID}) {
			t.Fatalf("copy call %d = %+v", index, call)
		}
		if call.exec.Values["active_id"] != wantID || !reflect.DeepEqual(call.exec.Values["active_ids"], []int64{wantID}) {
			t.Fatalf("copy call %d values = %+v", index, call.exec.Values)
		}
	}
}

func TestRegistryRunsObjectWriteValueConversions(t *testing.T) {
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Writer: writer})
	id, err := registry.Register(ServerAction{
		Name:               "Set Active",
		Model:              "res.partner",
		Kind:               KindWrite,
		UpdatePath:         "active",
		UpdateFieldType:    "boolean",
		UpdateBooleanValue: "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 9}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(writer.values, map[string]any{"active": false}) {
		t.Fatalf("bool values = %+v", writer.values)
	}

	id, err = registry.Register(ServerAction{
		Name:            "Set Company",
		Model:           "res.partner",
		Kind:            KindWrite,
		UpdatePath:      "company_id",
		UpdateFieldType: "many2one",
		ResourceRef:     "res.company,4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 9}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(writer.values, map[string]any{"company_id": int64(4)}) {
		t.Fatalf("many2one values = %+v", writer.values)
	}

	id, err = registry.Register(ServerAction{
		Name:               "Set Groups",
		Model:              "res.users",
		Kind:               KindWrite,
		UpdatePath:         "groups_id",
		UpdateFieldType:    "many2many",
		UpdateM2MOperation: "set",
		Value:              "5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 9}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(writer.values, map[string]any{"groups_id": RelationCommand{Operation: RelationSet, IDs: []int64{5}}}) {
		t.Fatalf("many2many values = %+v", writer.values)
	}
}

func TestRegistryRunsObjectWriteSequenceHook(t *testing.T) {
	writer := &captureWriter{}
	sequencer := &captureSequencer{values: map[int64]string{8: "SO/0007"}}
	registry := NewRegistry(Hooks{Writer: writer, Sequencer: sequencer})
	id, err := registry.Register(ServerAction{
		Name:            "Set Name",
		Model:           "res.partner",
		Kind:            KindWrite,
		UpdatePath:      "name",
		UpdateFieldType: "char",
		EvaluationType:  "sequence",
		SequenceID:      8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 9}); err != nil {
		t.Fatal(err)
	}
	if sequencer.id != 8 {
		t.Fatalf("sequence id = %d", sequencer.id)
	}
	if !reflect.DeepEqual(writer.values, map[string]any{"name": "SO/0007"}) {
		t.Fatalf("sequence values = %+v", writer.values)
	}
}

func TestRegistryRunsObjectCopyHook(t *testing.T) {
	operator := &captureObjectOperator{copiedID: 12}
	registry := NewRegistry(Hooks{ObjectOperator: operator})
	id, err := registry.Register(ServerAction{
		Name:        "Copy Partner",
		Model:       "res.partner",
		Kind:        KindCopy,
		ResourceRef: "res.partner,7",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 9})
	if err != nil {
		t.Fatal(err)
	}
	if result.CreatedID != 12 || operator.action.ResourceRef != "res.partner,7" || operator.exec.RecordID != 9 {
		t.Fatalf("copy result=%+v operator=%+v", result, operator)
	}
}

func TestRegistryRunsEquationEvaluatorHook(t *testing.T) {
	writer := &captureWriter{}
	evaluator := &captureEvaluator{value: "record-9"}
	registry := NewRegistry(Hooks{Writer: writer, Evaluator: evaluator})
	id, err := registry.Register(ServerAction{
		Name:            "Set Name",
		Model:           "res.partner",
		Kind:            KindWrite,
		UpdatePath:      "name",
		UpdateFieldType: "char",
		EvaluationType:  "equation",
		Value:           "record.id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 9}); err != nil {
		t.Fatal(err)
	}
	if evaluator.action.Value != "record.id" || evaluator.exec.RecordID != 9 {
		t.Fatalf("evaluator = %+v", evaluator)
	}
	if !reflect.DeepEqual(writer.values, map[string]any{"name": "record-9"}) {
		t.Fatalf("writer values = %+v", writer.values)
	}
}

func TestRegistryEquationEvaluatorErrorStopsWrite(t *testing.T) {
	errBoom := errors.New("boom")
	writer := &captureWriter{}
	evaluator := &captureEvaluator{err: errBoom}
	registry := NewRegistry(Hooks{Writer: writer, Evaluator: evaluator})
	id, err := registry.Register(ServerAction{
		Name:            "Set Name",
		Model:           "res.partner",
		Kind:            KindWrite,
		UpdatePath:      "name",
		UpdateFieldType: "char",
		EvaluationType:  "equation",
		Value:           "record.id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 9}); !errors.Is(err, errBoom) {
		t.Fatalf("error = %v", err)
	}
	if writer.values != nil {
		t.Fatalf("writer values = %+v", writer.values)
	}
}

func TestRegistryBlocksWarningActions(t *testing.T) {
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Writer: writer})
	id, err := registry.Register(ServerAction{
		Name:    "Warned",
		Model:   "res.partner",
		Kind:    KindWrite,
		Warning: "bad configuration",
		Values:  map[string]any{"name": "x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 1})
	if !errors.Is(err, ErrActionWarning) {
		t.Fatalf("error = %v", err)
	}
	if result.DisabledReason != "bad configuration" || writer.values != nil {
		t.Fatalf("result=%+v writer=%+v", result, writer)
	}
}

func TestRegistryBlocksActionsWithoutRequiredGroup(t *testing.T) {
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Writer: writer})
	id, err := registry.Register(ServerAction{
		Name:     "Restricted",
		Model:    "res.partner",
		Kind:     KindWrite,
		GroupIDs: []int64{10},
		Values:   map[string]any{"name": "Ada"},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 1, UserID: 20, UserGroupIDs: []int64{30}})
	if !errors.Is(err, ErrActionForbidden) || result.DisabledReason != "forbidden server action" {
		t.Fatalf("error = %v result=%+v", err, result)
	}
	if writer.values != nil {
		t.Fatalf("writer = %+v", writer)
	}
}

func TestRegistryAllowsActionsWithRequiredGroupFromContext(t *testing.T) {
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Writer: writer})
	id, err := registry.Register(ServerAction{
		Name:     "Restricted",
		Model:    "res.partner",
		Kind:     KindWrite,
		GroupIDs: []int64{10},
		Values:   map[string]any{"name": "Ada"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := registry.Run(context.Background(), id, ExecutionContext{RecordID: 1, UserID: 20, Values: map[string]any{"group_ids": []int64{10}}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(writer.values, map[string]any{"name": "Ada"}) {
		t.Fatalf("writer = %+v", writer)
	}
}

func TestRegistryRunsSendMailAndEnqueueHooks(t *testing.T) {
	mailer := &captureMailer{}
	enqueuer := &captureEnqueuer{}
	registry := NewRegistry(Hooks{Mailer: mailer, Enqueuer: enqueuer})
	mailID, err := registry.Register(ServerAction{
		Name:           "Send Welcome",
		Model:          "res.partner",
		Kind:           KindSendMail,
		MailTemplateID: 11,
		Values:         map[string]any{"lang": "en_US"},
	})
	if err != nil {
		t.Fatal(err)
	}
	queueID, err := registry.Register(ServerAction{
		Name:      "Queue Sync",
		Model:     "res.partner",
		Kind:      KindEnqueue,
		QueueKey:  "partner:5",
		QueueName: "partner.sync",
		Payload:   map[string]any{"priority": "normal"},
	})
	if err != nil {
		t.Fatal(err)
	}

	mailResult, err := registry.Run(context.Background(), mailID, ExecutionContext{RecordID: 5, Values: map[string]any{"company_id": int64(1)}})
	if err != nil {
		t.Fatal(err)
	}
	if !mailResult.MailSent {
		t.Fatal("expected mail sent result")
	}
	if mailer.request.TemplateID != 11 || !reflect.DeepEqual(mailer.request.RecordIDs, []int64{5}) {
		t.Fatalf("mail request = %+v", mailer.request)
	}
	if !reflect.DeepEqual(mailer.request.Values, map[string]any{"company_id": int64(1), "lang": "en_US"}) {
		t.Fatalf("mail values = %+v", mailer.request.Values)
	}

	queueResult, err := registry.Run(context.Background(), queueID, ExecutionContext{RecordID: 5, Values: map[string]any{"source": "automation"}})
	if err != nil {
		t.Fatal(err)
	}
	if !queueResult.Enqueued {
		t.Fatal("expected enqueued result")
	}
	if enqueuer.job.Key != "partner:5" || enqueuer.job.Name != "partner.sync" {
		t.Fatalf("queue job = %+v", enqueuer.job)
	}
	if !reflect.DeepEqual(enqueuer.job.Payload, map[string]any{"priority": "normal", "source": "automation"}) {
		t.Fatalf("queue payload = %+v", enqueuer.job.Payload)
	}
}

func TestRegistryRunsMailThreadActionHooks(t *testing.T) {
	mailer := &captureMailer{}
	followers := &captureFollowerUpdater{}
	activities := &captureActivityScheduler{}
	registry := NewRegistry(Hooks{Mailer: mailer, FollowerUpdater: followers, ActivityScheduler: activities})
	mailID, err := registry.Register(ServerAction{
		Name:               "Post",
		Model:              "res.partner",
		Kind:               KindMailPost,
		TemplateID:         10,
		MailPostMethod:     "comment",
		MailPostAutoFollow: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	followID, err := registry.Register(ServerAction{
		Name:       "Follow",
		Model:      "res.partner",
		Kind:       KindFollowers,
		PartnerIDs: []int64{4, 5},
	})
	if err != nil {
		t.Fatal(err)
	}
	activityID, err := registry.Register(ServerAction{
		Name:                          "Activity",
		Model:                         "res.partner",
		Kind:                          KindNextActivity,
		ActivityTypeID:                3,
		ActivitySummary:               "Call",
		ActivityDateDeadlineRange:     2,
		ActivityDateDeadlineRangeType: "days",
		ActivityUserType:              "specific",
		ActivityUserID:                7,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = registry.Run(context.Background(), mailID, ExecutionContext{RecordID: 8, Values: map[string]any{"body": "Hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if mailer.request.TemplateID != 10 || mailer.request.Metadata["mail_post_method"] != "comment" || mailer.request.Metadata["mail_post_autofollow"] != true {
		t.Fatalf("mail request = %+v", mailer.request)
	}
	_, err = registry.Run(context.Background(), followID, ExecutionContext{RecordIDs: []int64{8, 9}})
	if err != nil {
		t.Fatal(err)
	}
	if followers.request.Model != "res.partner" || !reflect.DeepEqual(followers.request.RecordIDs, []int64{8, 9}) || !reflect.DeepEqual(followers.request.PartnerIDs, []int64{4, 5}) || followers.request.Remove {
		t.Fatalf("followers request = %+v", followers.request)
	}
	_, err = registry.Run(context.Background(), activityID, ExecutionContext{RecordID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if activities.request.ActivityTypeID != 3 || activities.request.UserID != 7 || !reflect.DeepEqual(activities.request.RecordIDs, []int64{8}) {
		t.Fatalf("activity request = %+v", activities.request)
	}
}

func TestRegistryRunsMultiActionChildrenInOrder(t *testing.T) {
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Writer: writer})
	firstID, err := registry.Register(ServerAction{
		Name:     "First",
		Model:    "res.partner",
		Kind:     KindWrite,
		Sequence: 1,
		Values:   map[string]any{"name": "First"},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := registry.Register(ServerAction{
		Name:     "Second",
		Model:    "res.partner",
		Kind:     KindWrite,
		Sequence: 2,
		Values:   map[string]any{"name": "Second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	multiID, err := registry.Register(ServerAction{
		Name:     "Multi",
		Model:    "res.partner",
		Kind:     KindMulti,
		ChildIDs: []int64{secondID, firstID},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), multiID, ExecutionContext{RecordID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActionID != secondID || result.Kind != KindWrite {
		t.Fatalf("result = %+v", result)
	}
	if !reflect.DeepEqual(writer.ids, []int64{8}) || !reflect.DeepEqual(writer.values, map[string]any{"name": "Second"}) {
		t.Fatalf("last write = ids %+v values %+v", writer.ids, writer.values)
	}
}

func TestRegistrySplitsMultiActionPerActiveRecord(t *testing.T) {
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Writer: writer})
	firstID, err := registry.Register(ServerAction{
		Name:     "First",
		Model:    "res.partner",
		Kind:     KindWrite,
		Sequence: 1,
		Values:   map[string]any{"name": "First"},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := registry.Register(ServerAction{
		Name:     "Second",
		Model:    "res.partner",
		Kind:     KindWrite,
		Sequence: 2,
		Values:   map[string]any{"name": "Second"},
	})
	if err != nil {
		t.Fatal(err)
	}
	multiID, err := registry.Register(ServerAction{
		Name:     "Multi",
		Model:    "res.partner",
		Kind:     KindMulti,
		ChildIDs: []int64{secondID, firstID},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), multiID, ExecutionContext{Model: "res.partner", RecordID: 99, RecordIDs: []int64{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActionID != secondID || result.Kind != KindWrite || !reflect.DeepEqual(result.WrittenIDs, []int64{2}) {
		t.Fatalf("result = %+v", result)
	}
	if len(writer.calls) != 4 {
		t.Fatalf("write calls = %+v", writer.calls)
	}
	want := []struct {
		ids    []int64
		values map[string]any
	}{
		{[]int64{1}, map[string]any{"name": "First"}},
		{[]int64{1}, map[string]any{"name": "Second"}},
		{[]int64{2}, map[string]any{"name": "First"}},
		{[]int64{2}, map[string]any{"name": "Second"}},
	}
	for index, expected := range want {
		if !reflect.DeepEqual(writer.calls[index].ids, expected.ids) || !reflect.DeepEqual(writer.calls[index].values, expected.values) {
			t.Fatalf("call %d = %+v", index, writer.calls[index])
		}
	}
}

func TestRegistryMultiActionStopsOnChildWarning(t *testing.T) {
	writer := &captureWriter{}
	registry := NewRegistry(Hooks{Writer: writer})
	warnedID, err := registry.Register(ServerAction{
		Name:     "Warned",
		Model:    "res.partner",
		Kind:     KindWrite,
		Sequence: 1,
		Warning:  "bad configuration",
		Values:   map[string]any{"name": "Warned"},
	})
	if err != nil {
		t.Fatal(err)
	}
	laterID, err := registry.Register(ServerAction{
		Name:     "Later",
		Model:    "res.partner",
		Kind:     KindWrite,
		Sequence: 2,
		Values:   map[string]any{"name": "Later"},
	})
	if err != nil {
		t.Fatal(err)
	}
	multiID, err := registry.Register(ServerAction{
		Name:     "Multi",
		Model:    "res.partner",
		Kind:     KindMulti,
		ChildIDs: []int64{warnedID, laterID},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), multiID, ExecutionContext{RecordID: 8})
	if !errors.Is(err, ErrActionWarning) {
		t.Fatalf("error = %v result=%+v", err, result)
	}
	if result.ActionID != warnedID || result.DisabledReason != "bad configuration" {
		t.Fatalf("result = %+v", result)
	}
	if writer.values != nil {
		t.Fatalf("later child ran: ids=%+v values=%+v", writer.ids, writer.values)
	}
}

func TestRegistryMultiActionStopsRecursiveCycle(t *testing.T) {
	registry := NewRegistry(Hooks{Writer: &captureWriter{}})
	firstID, err := registry.Register(ServerAction{
		ID:       100,
		Name:     "First",
		Model:    "res.partner",
		Kind:     KindMulti,
		Sequence: 1,
		ChildIDs: []int64{200},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := registry.Register(ServerAction{
		ID:       200,
		Name:     "Second",
		Model:    "res.partner",
		Kind:     KindMulti,
		Sequence: 1,
		ChildIDs: []int64{100},
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstID != 100 || secondID != 200 {
		t.Fatalf("ids = %d %d", firstID, secondID)
	}

	result, err := registry.Run(context.Background(), firstID, ExecutionContext{RecordID: 8})
	if !errors.Is(err, ErrActionRecursion) || result.ActionID != firstID {
		t.Fatalf("error = %v result=%+v", err, result)
	}
}

func TestRegistryRunsAIActionWithSelectedTools(t *testing.T) {
	aiRunner := &captureAIRunner{}
	registry := NewRegistry(Hooks{AIRunner: aiRunner})
	toolID, err := registry.Register(ServerAction{
		Name:      "Write Name",
		Model:     "res.partner",
		Kind:      KindWrite,
		UseInAI:   true,
		Values:    map[string]any{"name": "Ada"},
		Metadata:  map[string]any{"xml_id": "base.action_write_name"},
		AIToolIDs: []int64{99},
	})
	if err != nil {
		t.Fatal(err)
	}
	hiddenToolID, err := registry.Register(ServerAction{
		Name:  "Hidden",
		Kind:  KindWrite,
		Model: "res.partner",
	})
	if err != nil {
		t.Fatal(err)
	}
	aiID, err := registry.Register(ServerAction{
		Name:           "AI Update",
		Model:          "res.partner",
		Kind:           KindAI,
		AIActionPrompt: "Update the current partner.",
		AIToolIDs:      []int64{toolID, hiddenToolID, 12345},
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := registry.Run(context.Background(), aiID, ExecutionContext{RecordID: 42, RecordIDs: []int64{42}})
	if err != nil {
		t.Fatal(err)
	}
	if result.ActionID != aiID || result.Kind != KindAI {
		t.Fatalf("result = %+v", result)
	}
	if aiRunner.request.Action.ID != aiID || aiRunner.request.Action.AIActionPrompt != "Update the current partner." {
		t.Fatalf("ai action = %+v", aiRunner.request.Action)
	}
	if len(aiRunner.request.Tools) != 1 || aiRunner.request.Tools[0].ID != toolID {
		t.Fatalf("tools = %+v", aiRunner.request.Tools)
	}
	if !reflect.DeepEqual(aiRunner.request.Execution.RecordIDs, []int64{42}) {
		t.Fatalf("execution = %+v", aiRunner.request.Execution)
	}
}

func TestRegistryRejectsAIActionWithoutRunner(t *testing.T) {
	registry := NewRegistry(Hooks{})
	id, err := registry.Register(ServerAction{Name: "AI", Kind: KindAI})
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Run(context.Background(), id, ExecutionContext{})
	if !errors.Is(err, ErrAIRunnerMissing) {
		t.Fatalf("expected missing AI runner, got %v", err)
	}
}

func TestRegistryRejectsDisabledExecutionKinds(t *testing.T) {
	registry := NewRegistry(Hooks{})
	webhookID, err := registry.Register(ServerAction{Name: "Call Webhook", Kind: KindWebhook, WebhookURL: "https://example.invalid/hook"})
	if err != nil {
		t.Fatal(err)
	}
	codeID, err := registry.Register(ServerAction{Name: "Run Code", Kind: KindCode, Code: "record.name = 'x'"})
	if err != nil {
		t.Fatal(err)
	}

	webhookResult, err := registry.Run(context.Background(), webhookID, ExecutionContext{})
	if !errors.Is(err, ErrWebhookDisabled) {
		t.Fatalf("webhook error = %v", err)
	}
	if webhookResult.DisabledReason == "" {
		t.Fatalf("webhook result = %+v", webhookResult)
	}

	codeResult, err := registry.Run(context.Background(), codeID, ExecutionContext{})
	if !errors.Is(err, ErrCodeExecutionDisabled) {
		t.Fatalf("code error = %v", err)
	}
	if codeResult.DisabledReason == "" {
		t.Fatalf("code result = %+v", codeResult)
	}
}

func TestRegistryRunsEnterpriseServerActionStates(t *testing.T) {
	sms := &captureSMSSender{}
	whatsapp := &captureWhatsAppSender{}
	documents := &captureDocumentCreator{action: map[string]any{"type": "ir.actions.act_window"}}
	registry := NewRegistry(Hooks{SMSSender: sms, WhatsAppSender: whatsapp, DocumentCreator: documents})
	smsID, err := registry.Register(ServerAction{Name: "SMS", Model: "res.partner", Kind: KindSMS, SMSTemplateID: 11, SMSMethod: "comment", Metadata: map[string]any{"campaign_id": int64(8)}})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := registry.Register(ServerAction{Name: "WhatsApp", Model: "res.partner", Kind: KindWhatsApp, WhatsAppTemplateID: 12})
	if err != nil {
		t.Fatal(err)
	}
	documentID, err := registry.Register(ServerAction{Name: "Create Vendor Bill", Model: "documents.document", Kind: KindDocumentAccount, DocumentsAccountCreateModel: "account.move.in_invoice", DocumentsAccountMoveType: "in_invoice", DocumentsAccountJournalID: 7})
	if err != nil {
		t.Fatal(err)
	}

	smsResult, err := registry.Run(context.Background(), smsID, ExecutionContext{RecordIDs: []int64{1, 2}, Metadata: map[string]any{"mass_mailing_id": int64(7)}})
	if err != nil {
		t.Fatal(err)
	}
	if !smsResult.SMSSent || sms.request.TemplateID != 11 || sms.request.Method != "comment" || sms.request.Model != "res.partner" || !reflect.DeepEqual(sms.request.RecordIDs, []int64{1, 2}) {
		t.Fatalf("sms result=%+v request=%+v", smsResult, sms.request)
	}
	if sms.request.Metadata["mass_mailing_id"] != int64(7) || sms.request.Metadata["campaign_id"] != int64(8) {
		t.Fatalf("sms metadata = %+v", sms.request.Metadata)
	}
	whatsappResult, err := registry.Run(context.Background(), whatsappID, ExecutionContext{RecordID: 3, Metadata: map[string]any{"whatsapp_message_id": int64(99)}})
	if err != nil {
		t.Fatal(err)
	}
	if !whatsappResult.WhatsAppSent || whatsapp.request.TemplateID != 12 || whatsapp.request.Model != "res.partner" || !reflect.DeepEqual(whatsapp.request.RecordIDs, []int64{3}) || whatsapp.request.Metadata["whatsapp_message_id"] != int64(99) {
		t.Fatalf("whatsapp result=%+v request=%+v", whatsappResult, whatsapp.request)
	}
	documentResult, err := registry.Run(context.Background(), documentID, ExecutionContext{RecordIDs: []int64{4, 5}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(documentResult.Action, map[string]any{"type": "ir.actions.act_window"}) ||
		documents.request.Model != "documents.document" ||
		!reflect.DeepEqual(documents.request.RecordIDs, []int64{4, 5}) ||
		documents.request.CreateModel != "account.move.in_invoice" ||
		documents.request.MoveType != "in_invoice" ||
		documents.request.JournalID != 7 {
		t.Fatalf("document result=%+v request=%+v", documentResult, documents.request)
	}
}

func TestRegistryEnterpriseServerActionStatesNoopWhenUnconfigured(t *testing.T) {
	registry := NewRegistry(Hooks{})
	smsID, err := registry.Register(ServerAction{Name: "SMS", Model: "res.partner", Kind: KindSMS})
	if err != nil {
		t.Fatal(err)
	}
	whatsappID, err := registry.Register(ServerAction{Name: "WhatsApp", Model: "res.partner", Kind: KindWhatsApp})
	if err != nil {
		t.Fatal(err)
	}
	documentID, err := registry.Register(ServerAction{Name: "Create Entry", Model: "documents.document", Kind: KindDocumentAccount, DocumentsAccountCreateModel: "account.move.entry"})
	if err != nil {
		t.Fatal(err)
	}

	if result, err := registry.Run(context.Background(), smsID, ExecutionContext{RecordID: 1}); err != nil || result.SMSSent {
		t.Fatalf("sms result=%+v err=%v", result, err)
	}
	if result, err := registry.Run(context.Background(), whatsappID, ExecutionContext{RecordID: 1}); err != nil || result.WhatsAppSent {
		t.Fatalf("whatsapp result=%+v err=%v", result, err)
	}
	if result, err := registry.Run(context.Background(), documentID, ExecutionContext{}); err != nil || result.Action != nil {
		t.Fatalf("document result=%+v err=%v", result, err)
	}
}

func TestRegistryReturnsMissingHookErrors(t *testing.T) {
	registry := NewRegistry(Hooks{})
	id, err := registry.Register(ServerAction{Name: "Write", Kind: KindWrite, Model: "res.partner", Values: map[string]any{"name": "x"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = registry.Run(context.Background(), id, ExecutionContext{RecordID: 1})
	if !errors.Is(err, ErrWriteHookMissing) {
		t.Fatalf("error = %v", err)
	}
}

type captureCreator struct {
	id     int64
	model  string
	values map[string]any
	calls  []captureCreateCall
}

type captureCreateCall struct {
	model  string
	values map[string]any
}

func (c *captureCreator) Create(_ context.Context, model string, values map[string]any) (int64, error) {
	c.model = model
	c.values = values
	c.calls = append(c.calls, captureCreateCall{model: model, values: cloneMap(values)})
	return c.id, nil
}

type captureWriter struct {
	model  string
	ids    []int64
	values map[string]any
	calls  []captureWriteCall
}

type captureWriteCall struct {
	model  string
	ids    []int64
	values map[string]any
}

func (w *captureWriter) Write(_ context.Context, model string, ids []int64, values map[string]any) error {
	w.model = model
	w.ids = ids
	w.values = values
	w.calls = append(w.calls, captureWriteCall{model: model, ids: cloneIDs(ids), values: cloneMap(values)})
	return nil
}

type captureSequencer struct {
	id     int64
	values map[int64]string
}

func (s *captureSequencer) NextSequence(_ context.Context, id int64) (string, error) {
	s.id = id
	return s.values[id], nil
}

type captureObjectOperator struct {
	action      ServerAction
	exec        ExecutionContext
	values      map[string]any
	created     int64
	written     []int64
	copiedID    int64
	createCalls []captureObjectCall
	writeCalls  []captureObjectCall
	copyCalls   []captureObjectCall
}

type captureObjectCall struct {
	action ServerAction
	exec   ExecutionContext
	values map[string]any
}

func (o *captureObjectOperator) CreateObject(_ context.Context, action ServerAction, exec ExecutionContext, values map[string]any) (int64, error) {
	o.action = action
	o.exec = exec
	o.values = values
	o.createCalls = append(o.createCalls, captureObjectCall{action: cloneAction(action), exec: cloneExecution(exec), values: cloneMap(values)})
	if o.created != 0 {
		return o.created, nil
	}
	return 1, nil
}

func (o *captureObjectOperator) WriteObject(_ context.Context, action ServerAction, exec ExecutionContext, values map[string]any) ([]int64, error) {
	o.action = action
	o.exec = exec
	o.values = values
	o.writeCalls = append(o.writeCalls, captureObjectCall{action: cloneAction(action), exec: cloneExecution(exec), values: cloneMap(values)})
	if len(o.written) > 0 {
		return o.written, nil
	}
	return []int64{exec.RecordID}, nil
}

func (o *captureObjectOperator) CopyObject(_ context.Context, action ServerAction, exec ExecutionContext) (int64, error) {
	o.action = action
	o.exec = exec
	o.copyCalls = append(o.copyCalls, captureObjectCall{action: cloneAction(action), exec: cloneExecution(exec)})
	return o.copiedID, nil
}

type captureEvaluator struct {
	action ServerAction
	exec   ExecutionContext
	value  any
	err    error
}

func (e *captureEvaluator) EvaluateActionValue(_ context.Context, action ServerAction, exec ExecutionContext) (any, error) {
	e.action = action
	e.exec = exec
	if e.err != nil {
		return nil, e.err
	}
	return e.value, nil
}

type captureMailer struct {
	request MailRequest
}

func (m *captureMailer) SendMail(_ context.Context, request MailRequest) error {
	m.request = request
	return nil
}

type captureEnqueuer struct {
	job QueueJob
}

func (e *captureEnqueuer) Enqueue(_ context.Context, job QueueJob) error {
	e.job = job
	return nil
}

type captureFollowerUpdater struct {
	request FollowersRequest
}

func (f *captureFollowerUpdater) UpdateFollowers(_ context.Context, request FollowersRequest) error {
	f.request = request
	return nil
}

type captureActivityScheduler struct {
	request ActivityRequest
}

func (a *captureActivityScheduler) ScheduleActivity(_ context.Context, request ActivityRequest) error {
	a.request = request
	return nil
}

type captureSMSSender struct {
	request SMSRequest
}

func (s *captureSMSSender) SendSMS(_ context.Context, request SMSRequest) error {
	s.request = request
	return nil
}

type captureWhatsAppSender struct {
	request WhatsAppRequest
}

func (w *captureWhatsAppSender) SendWhatsApp(_ context.Context, request WhatsAppRequest) error {
	w.request = request
	return nil
}

type captureDocumentCreator struct {
	request DocumentAccountRecordRequest
	action  any
}

func (d *captureDocumentCreator) CreateDocumentAccountRecord(_ context.Context, request DocumentAccountRecordRequest) (any, error) {
	d.request = request
	return d.action, nil
}

type captureAIRunner struct {
	request AIActionRequest
}

func (r *captureAIRunner) RunAI(_ context.Context, request AIActionRequest) (Result, error) {
	r.request = request
	return Result{Metadata: map[string]any{"ok": true}}, nil
}
