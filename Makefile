.PHONY: ci test runtime-test vet fmt-check frontend-install frontend-typecheck frontend-lint frontend-test frontend-build

.PHONY: frontend-e2e

ci: fmt-check vet test frontend-install frontend-typecheck frontend-lint frontend-test frontend-build frontend-e2e

RUNTIME_TEST_GROUP_1 := ^Test(AIFieldsCSVFormatsRelationSelectionAndDescriptions|AIParseOrderedMeasures|AIRuntimeReadGroupRecordsetAggregateSerializesIDs|AIRuntimeSearchOrdersAndLimits|BootstrapOIInstallsRuntimeModules|BootstrapOIAccountingPhaseGate|CSVModelNameFromTemplatePath|RuntimeFetchmailCronDeactivatesWhenNoEligibleServers|FetchmailCronEndTimeUsesExplicitAndDefaultBudgets|RuntimeFetchmailCronProgressSchedulesImmediateTriggerForRemainingMessages|FetchmailServerLifecycleTogglesGatewayCron|BootstrapOIRegistersWorkflowEscalationAction|BootstrapOIRegistersDelegationClearAccessCacheAction)$$
RUNTIME_TEST_GROUP_2 := ^Test(ViewsFromEnv.*|MenusFromEnv.*|BootstrapOIDelegation.*|OIEnvDelegation.*|BootstrapOIServerActionHooksPreserveObjectEvaluatorAndSequence|ActionsFromEnv.*|ServerActionsFromEnvRunsMailThreadStates|ServerActionsFromEnvRunsEnterpriseTransportAndDocumentStates|ServerActionsFromEnvLoadsInactiveActionsAsDisabled)$$
RUNTIME_TEST_GROUP_3 := ^Test(RuntimeWhatsApp.*|RuntimeSMS.*|EnvActionHooksSend.*|ServerActionsFromEnvRunsObject.*|ServerActionsFromEnvRunsX2ManyObjectWriteOperations|EnvActionHooksNextSequence.*)$$
RUNTIME_TEST_GROUP_4 := ^Test(ServerActionsFromEnvBlocks.*|ServerActionsFromEnvWarnings.*|ServerActionsFromEnvWarns.*|ServerActionsFromEnvPropagatesChildWarnings|ServerActionsFromEnvFilters.*|ServerActionsFromEnvHydrates.*|ServerActionStorage.*|ServerActionsFromEnvEvaluates.*)$$
RUNTIME_TEST_GROUP_5 := ^Test(BootstrapOIAIToolActionsAndTopics|RuntimeAIToolActionsRunFromServerActions|BootstrapOIExposesHTTPModulesAssetsMenusAndViews|BootstrapOIDelegationWebViewsAndActionMetadata|BootstrapWebNormalUserCanOpenRecordsListAndForm|BootstrapOIAIGenerateResponse.*|RuntimeAIDraftChannelCallKWCreatesContext|RuntimeAIAskAIActionAlwaysCreatesNewChannel|AIProviderResolver.*|AISettingsEnvUsesSystemContext|RuntimeAIActionRunsSelectedToolAndLogsSummary|RuntimeSecretRef.*|AssetsFromSources.*)$$

test:
	go test -timeout=20m $$(go list ./... | grep -v '^gorp/internal/runtime$$')
	$(MAKE) runtime-test

runtime-test:
	go test -timeout=10m ./internal/runtime -run '$(RUNTIME_TEST_GROUP_1)'
	go test -timeout=10m ./internal/runtime -run '$(RUNTIME_TEST_GROUP_2)'
	go test -timeout=10m ./internal/runtime -run '$(RUNTIME_TEST_GROUP_3)'
	go test -timeout=10m ./internal/runtime -run '$(RUNTIME_TEST_GROUP_4)'
	go test -timeout=10m ./internal/runtime -run '$(RUNTIME_TEST_GROUP_5)'

vet:
	go vet ./...

fmt-check:
	test -z "$$(gofmt -l .)"

frontend-install:
	pnpm -C frontend install

frontend-typecheck:
	pnpm -C frontend typecheck

frontend-lint:
	pnpm -C frontend lint

frontend-test:
	pnpm -C frontend test

frontend-build:
	pnpm -C frontend build

frontend-e2e:
	pnpm -C frontend test:e2e
