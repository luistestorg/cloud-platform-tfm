package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMonLogStackReference validates mon-log references infra-kube correctly
func TestMonLogStackReference(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	// Check infra-kube stack exists
	infraKubeRef := "organization/eks-infra-kube/dev"
	_, err = auto.SelectStack(ctx, infraKubeRef, ws)
	if err != nil {
		t.Skip("Skipping test - infra-kube stack not available")
		return
	}

	// Check mon-log stack
	monLogRef := "organization/eks-mon-log/dev"
	monLogStack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	// Verify mon-log has reference to infra-kube
	config, err := monLogStack.GetAllConfig(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get config")
		return
	}

	infraKubeStackRef, hasRef := config["eks-mon-log:infraKubeStackRef"]
	assert.True(t, hasRef, "mon-log should have infraKubeStackRef configuration")

	if hasRef {
		assert.Contains(t, infraKubeStackRef.Value, "eks-infra-kube",
			"infraKubeStackRef should reference eks-infra-kube stack")
	}

	t.Logf("✓ Stack reference validation passed")
}

// TestMonitoringNamespaceExists validates monitoring namespace is created
func TestMonitoringNamespaceExists(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	monLogRef := "organization/eks-mon-log/dev"
	stack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify monitoring namespace output exists
	nsOutput, exists := outputs["monitoringNamespace"]
	assert.True(t, exists, "monitoringNamespace output should exist")

	if exists {
		assert.Equal(t, "monitoring", nsOutput.Value,
			"Monitoring namespace should be named 'monitoring'")
	}

	t.Logf("✓ Monitoring namespace validated")
}

// TestPrometheusServiceExported validates Prometheus service is exported
func TestPrometheusServiceExported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	monLogRef := "organization/eks-mon-log/dev"
	stack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify Prometheus service output
	promService, exists := outputs["prometheusService"]
	assert.True(t, exists, "prometheusService output should exist")

	if exists {
		assert.Contains(t, promService.Value, "prometheus",
			"Prometheus service should contain 'prometheus' in name")
	}

	t.Logf("✓ Prometheus service output validated")
}

// TestGrafanaServiceExported validates Grafana service is exported
func TestGrafanaServiceExported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	monLogRef := "organization/eks-mon-log/dev"
	stack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify Grafana service output
	grafanaService, exists := outputs["grafanaService"]
	assert.True(t, exists, "grafanaService output should exist")

	if exists {
		assert.Contains(t, grafanaService.Value, "grafana",
			"Grafana service should contain 'grafana' in name")
	}

	t.Logf("✓ Grafana service output validated")
}

// TestLokiStackOutputs validates Loki-related outputs when enabled
func TestLokiStackOutputs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	monLogRef := "organization/eks-mon-log/dev"
	stack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	// Check if Loki is enabled in config
	config, err := stack.GetAllConfig(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get config")
		return
	}

	enableLoki, hasLokiConfig := config["eks-mon-log:enableLoki"]
	if !hasLokiConfig || enableLoki.Value != "true" {
		t.Skip("Skipping test - Loki not enabled")
		return
	}

	// Verify Loki outputs exist
	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	lokiService, hasLoki := outputs["lokiService"]
	assert.True(t, hasLoki, "lokiService output should exist when Loki is enabled")

	if hasLoki {
		assert.Equal(t, "loki", lokiService.Value, "Loki service name should be 'loki'")
	}

	_, hasPromtail := outputs["promtailDaemonSet"]
	assert.True(t, hasPromtail, "promtailDaemonSet output should exist when Loki is enabled")

	t.Logf("✓ Loki stack outputs validated")
}

// TestAlertManagerOutput validates AlertManager output when enabled
func TestAlertManagerOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	monLogRef := "organization/eks-mon-log/dev"
	stack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	// Check if AlertManager is enabled
	config, err := stack.GetAllConfig(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get config")
		return
	}

	enableAM, hasAMConfig := config["eks-mon-log:enableAlertManager"]
	if !hasAMConfig || enableAM.Value != "true" {
		t.Skip("Skipping test - AlertManager not enabled")
		return
	}

	// Verify AlertManager output
	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	amService, hasAM := outputs["alertmanagerService"]
	assert.True(t, hasAM, "alertmanagerService output should exist when enabled")

	if hasAM {
		assert.Contains(t, amService.Value, "alertmanager",
			"AlertManager service should contain 'alertmanager'")
	}

	t.Logf("✓ AlertManager output validated")
}

// TestMonLogStackDependencyOrder validates deployment order
func TestMonLogStackDependencyOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	// Verify infra-kube exists
	infraKubeRef := "organization/eks-infra-kube/dev"
	_, err = auto.SelectStack(ctx, infraKubeRef, ws)
	infraKubeExists := err == nil

	// Verify mon-log
	monLogRef := "organization/eks-mon-log/dev"
	_, err = auto.SelectStack(ctx, monLogRef, ws)
	monLogExists := err == nil

	if monLogExists {
		assert.True(t, infraKubeExists,
			"mon-log should not exist without infra-kube (dependency violation)")
	}

	t.Logf("✓ Stack dependency order validated")
}

// TestMonLogStackResourceCount validates resource count
func TestMonLogStackResourceCount(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	monLogRef := "organization/eks-mon-log/dev"
	stack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	info, err := stack.Info(ctx)
	if err != nil || info.ResourceCount == nil {
		t.Skip("Skipping test - cannot get resource count")
		return
	}

	resourceCount := *info.ResourceCount

	t.Logf("mon-log stack has %d resources", resourceCount)

	// Monitoring stack typically has 10-40 resources
	assert.Greater(t, resourceCount, 5, "Stack should have meaningful resources")
	assert.Less(t, resourceCount, 50, "Stack should follow micro-stack principles")

	t.Logf("✓ Resource count validated")
}

// TestMonLogStackConfiguration validates required configuration
func TestMonLogStackConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	monLogRef := "organization/eks-mon-log/dev"
	stack, err := auto.SelectStack(ctx, monLogRef, ws)
	if err != nil {
		t.Skip("Skipping test - mon-log stack not deployed")
		return
	}

	config, err := stack.GetAllConfig(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get config")
		return
	}

	// Verify required configuration exists
	requiredConfig := []string{
		"aws:region",
		"eks-mon-log:environment",
		"eks-mon-log:infraKubeStackRef",
		"eks-mon-log:prometheusStorage",
		"eks-mon-log:grafanaStorage",
	}

	for _, key := range requiredConfig {
		_, exists := config[key]
		assert.True(t, exists, fmt.Sprintf("Config %s should be set", key))
	}

	t.Logf("✓ Stack configuration validated")
}

// TestMonLogBlastRadius validates micro-stack isolation
func TestMonLogBlastRadius(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	stacks := []string{
		"organization/eks-infra-aws/dev",
		"organization/eks-infra-kube/dev",
		"organization/eks-mon-log/dev",
	}

	stackResourceCounts := make(map[string]int)
	totalResources := 0

	for _, stackRef := range stacks {
		stack, err := auto.SelectStack(ctx, stackRef, ws)
		if err != nil {
			continue
		}

		info, err := stack.Info(ctx)
		if err != nil {
			continue
		}

		if info.ResourceCount != nil {
			count := *info.ResourceCount
			stackResourceCounts[stackRef] = count
			totalResources += count
		}
	}

	if totalResources == 0 {
		t.Skip("Skipping test - no resource counts available")
		return
	}

	// Calculate mon-log blast radius
	monLogCount, hasMonLog := stackResourceCounts["organization/eks-mon-log/dev"]
	if !hasMonLog {
		t.Skip("Skipping test - mon-log resource count not available")
		return
	}

	blastRadius := float64(monLogCount) / float64(totalResources) * 100

	t.Logf("mon-log blast radius: %.1f%% (%d/%d resources)",
		blastRadius, monLogCount, totalResources)

	// mon-log should represent < 40% of total infrastructure
	assert.Less(t, blastRadius, 40.0,
		"mon-log blast radius should be < 40% of total infrastructure")

	t.Logf("✓ Blast radius validated")
}

// TestFullMonLogDeployment performs end-to-end deployment test
func TestFullMonLogDeployment(t *testing.T) {
	if os.Getenv("RUN_E2E_TESTS") != "true" {
		t.Skip("Skipping E2E test (set RUN_E2E_TESTS=true to run)")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	require.NoError(t, err)

	stackName := "mon-log-e2e-test"
	stack, err := auto.UpsertStack(ctx, stackName, ws)
	require.NoError(t, err)

	// Configure stack
	err = stack.SetConfig(ctx, "aws:region", auto.ConfigValue{Value: "us-east-1"})
	require.NoError(t, err)

	err = stack.SetConfig(ctx, "eks-mon-log:environment", auto.ConfigValue{Value: "test"})
	require.NoError(t, err)

	// Deploy with timeout
	deployCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	t.Log("Starting E2E deployment (this will take 10-15 minutes)...")

	upRes, err := stack.Up(deployCtx, optup.ProgressStreams(os.Stdout))
	require.NoError(t, err, "Stack deployment should succeed")

	t.Logf("✓ Stack deployed successfully")
	t.Logf("Summary: %+v", upRes.Summary)

	// Verify outputs
	outputs, err := stack.Outputs(ctx)
	require.NoError(t, err)

	assert.NotEmpty(t, outputs, "Stack should export outputs")

	// Cleanup
	if !t.Failed() {
		t.Log("Cleaning up resources...")
		_, err = stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
		assert.NoError(t, err, "Stack cleanup should succeed")
	}
}
