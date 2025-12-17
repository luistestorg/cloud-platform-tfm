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

// IntegrationTestConfig holds configuration for integration tests
type IntegrationTestConfig struct {
	ProjectName       string
	StackName         string
	Region            string
	SkipCleanup       bool
	DeploymentTimeout time.Duration
}

// TestStackReferenceIntegration validates that infra-kube stack correctly
// consumes outputs from infra-aws stack
func TestStackReferenceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	stackName := "dev"
	infraAwsStackRef := fmt.Sprintf("organization/eks-infra-aws/%s", stackName)

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Logf("Warning: Could not create workspace: %v", err)
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	infraAwsStack, err := auto.SelectStack(ctx, infraAwsStackRef, ws)
	if err != nil {
		t.Logf("Warning: Could not access infra-aws stack: %v", err)
		t.Skip("Skipping test - infra-aws stack not available")
		return
	}

	outputs, err := infraAwsStack.Outputs(ctx)
	if err != nil {
		t.Logf("Warning: Could not get outputs: %v", err)
		t.Skip("Skipping test - cannot retrieve stack outputs")
		return
	}

	// Verify required outputs exist
	requiredOutputs := []string{"vpcId", "privateSubnetIds", "publicSubnetIds"}
	for _, output := range requiredOutputs {
		_, exists := outputs[output]
		assert.True(t, exists, fmt.Sprintf("Output %s should exist in infra-aws stack", output))
	}

	t.Logf("✓ Stack reference integration validated")
}

// TestInfraKubeStackOutputs validates that infra-kube exports all required outputs
func TestInfraKubeStackOutputs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	stackRef := "organization/eks-infra-kube/dev"

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	stack, err := auto.SelectStack(ctx, stackRef, ws)
	if err != nil {
		t.Logf("Warning: Could not select stack %s: %v", stackRef, err)
		t.Skip("Skipping test - stack not available")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot retrieve outputs")
		return
	}

	// Verify required outputs
	requiredOutputs := []string{"clusterName", "clusterEndpoint", "kubeconfig"}
	for _, outputName := range requiredOutputs {
		output, exists := outputs[outputName]
		assert.True(t, exists, fmt.Sprintf("Output %s should exist", outputName))
		if exists {
			assert.NotNil(t, output.Value, fmt.Sprintf("Output %s should have a value", outputName))
		}
	}

	t.Logf("✓ All required outputs are present")
}

// TestStackDeploymentSuccess validates stack has been successfully deployed
func TestStackDeploymentSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	stackRef := "organization/eks-infra-kube/dev"
	stack, err := auto.SelectStack(ctx, stackRef, ws)
	if err != nil {
		t.Skip("Skipping test - stack not available")
		return
	}

	// Get stack info
	info, err := stack.Info(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get stack info")
		return
	}

	// Verify stack has resources
	assert.NotNil(t, info, "Stack info should exist")

	if info.ResourceCount != nil {
		t.Logf("Stack has %d resources deployed", *info.ResourceCount)
		assert.Greater(t, *info.ResourceCount, 0, "Stack should have resources deployed")
	}

	t.Logf("✓ Stack deployment validated")
}

// TestStackConfigurationPresent validates stack has required configuration
func TestStackConfigurationPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	stackRef := "organization/eks-infra-kube/dev"
	stack, err := auto.SelectStack(ctx, stackRef, ws)
	if err != nil {
		t.Skip("Skipping test - stack not available")
		return
	}

	// Get all config
	config, err := stack.GetAllConfig(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot retrieve config")
		return
	}

	// Verify critical config exists
	requiredConfig := []string{
		"aws:region",
		"eks-infra-kube:infraAwsStack",
		"eks-infra-kube:clusterName",
	}

	for _, configKey := range requiredConfig {
		_, exists := config[configKey]
		assert.True(t, exists, fmt.Sprintf("Config %s should be set", configKey))
	}

	t.Logf("✓ All required configuration is present")
}

// TestStackDependencyOrder validates deployment happens in correct order
func TestStackDependencyOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	// Check infra-aws exists first
	infraAwsRef := "organization/eks-infra-aws/dev"
	_, err = auto.SelectStack(ctx, infraAwsRef, ws)
	infraAwsExists := err == nil

	// Check infra-kube
	infraKubeRef := "organization/eks-infra-kube/dev"
	infraKubeStack, err := auto.SelectStack(ctx, infraKubeRef, ws)
	infraKubeExists := err == nil

	if !infraKubeExists {
		t.Skip("Skipping test - infra-kube stack not deployed")
		return
	}

	// infra-kube should not exist without infra-aws
	if infraKubeExists {
		assert.True(t, infraAwsExists,
			"infra-kube should not be deployed without infra-aws (dependency violation)")
	}

	// Verify infra-kube references infra-aws
	if infraKubeExists {
		config, _ := infraKubeStack.GetAllConfig(ctx)
		stackRef, hasRef := config["eks-infra-kube:infraAwsStack"]

		assert.True(t, hasRef, "infra-kube should have infraAwsStack configuration")
		if hasRef {
			assert.Contains(t, stackRef.Value, "eks-infra-aws",
				"infraAwsStack should reference the correct stack")
		}
	}

	t.Logf("✓ Stack dependency order is correct")
}

// TestBlastRadiusReduction validates micro-stack isolation
func TestBlastRadiusReduction(t *testing.T) {
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
	}

	stackResourceCounts := make(map[string]int)
	totalResources := 0

	for _, stackRef := range stacks {
		stack, err := auto.SelectStack(ctx, stackRef, ws)
		if err != nil {
			continue
		}

		// Get stack summary
		summary, err := stack.Info(ctx)
		if err != nil {
			continue
		}

		if summary.ResourceCount != nil {
			count := *summary.ResourceCount
			stackResourceCounts[stackRef] = count
			totalResources += count

			// Each micro-stack should have < 50 resources (per design goal)
			assert.Less(t, count, 50,
				fmt.Sprintf("Stack %s should have < 50 resources for maintainability", stackRef))
		}
	}

	if totalResources > 0 {
		for stackRef, count := range stackResourceCounts {
			blastRadius := float64(count) / float64(totalResources) * 100
			t.Logf("Stack %s: %d resources (%.1f%% of total)", stackRef, count, blastRadius)

			// Each stack should represent < 50% of total (good separation)
			assert.Less(t, blastRadius, 50.0,
				"Individual stack blast radius should be < 50% of total infrastructure")
		}
	}

	t.Logf("✓ Blast radius is properly contained across micro-stacks")
}

// TestStackResourceCount validates resource count per stack
func TestStackResourceCount(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	stackRef := "organization/eks-infra-kube/dev"
	stack, err := auto.SelectStack(ctx, stackRef, ws)
	if err != nil {
		t.Skip("Skipping test - stack not available")
		return
	}

	summary, err := stack.Info(ctx)
	if err != nil || summary.ResourceCount == nil {
		t.Skip("Skipping test - cannot get resource count")
		return
	}

	resourceCount := *summary.ResourceCount

	t.Logf("Stack has %d resources", resourceCount)

	// Verify it follows micro-stack principles
	assert.Less(t, resourceCount, 50, "Micro-stack should have fewer than 50 resources")
	assert.Greater(t, resourceCount, 5, "Stack should have meaningful resource count")
}

// TestStackExists validates basic stack accessibility
func TestStackExists(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	stackRef := "organization/eks-infra-kube/dev"
	stack, err := auto.SelectStack(ctx, stackRef, ws)

	if err != nil {
		t.Skip("Skipping test - stack not deployed yet")
		return
	}

	// Get basic info
	info, err := stack.Info(ctx)
	require.NoError(t, err, "Should be able to get stack info")

	assert.NotNil(t, info, "Stack info should not be nil")
	t.Logf("✓ Stack exists and is accessible")
}

// TestStackOutputsAccessibility validates outputs can be retrieved
func TestStackOutputsAccessibility(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	stackRef := "organization/eks-infra-kube/dev"
	stack, err := auto.SelectStack(ctx, stackRef, ws)
	if err != nil {
		t.Skip("Skipping test - stack not available")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot retrieve outputs")
		return
	}

	// Verify we can access outputs
	assert.NotNil(t, outputs, "Outputs should not be nil")

	if len(outputs) > 0 {
		t.Logf("Stack has %d outputs", len(outputs))

		// Log output names for debugging
		for name := range outputs {
			t.Logf("  - %s", name)
		}
	}

	t.Logf("✓ Stack outputs are accessible")
}

// TestFullStackDeployment performs end-to-end deployment test
func TestFullStackDeployment(t *testing.T) {
	if os.Getenv("RUN_E2E_TESTS") != "true" {
		t.Skip("Skipping E2E test (set RUN_E2E_TESTS=true to run)")
	}

	ctx := context.Background()

	cfg := IntegrationTestConfig{
		ProjectName:       "eks-infra-kube",
		StackName:         "integration-test",
		Region:            "us-east-1",
		SkipCleanup:       false,
		DeploymentTimeout: 20 * time.Minute,
	}

	// Create workspace
	ws, err := auto.NewLocalWorkspace(ctx)
	require.NoError(t, err)

	stack, err := auto.UpsertStack(ctx, cfg.StackName, ws)
	require.NoError(t, err)

	// Configure stack
	err = stack.SetConfig(ctx, "aws:region", auto.ConfigValue{Value: cfg.Region})
	require.NoError(t, err)

	// Deploy with timeout
	deployCtx, cancel := context.WithTimeout(ctx, cfg.DeploymentTimeout)
	defer cancel()

	t.Log("Starting E2E deployment (this will take 15-20 minutes)...")

	upRes, err := stack.Up(deployCtx, optup.ProgressStreams(os.Stdout))
	require.NoError(t, err, "Stack deployment should succeed")

	t.Logf("✓ Stack deployed successfully")
	t.Logf("Summary: %+v", upRes.Summary)

	// Verify outputs exist
	outputs, err := stack.Outputs(ctx)
	require.NoError(t, err)

	assert.NotEmpty(t, outputs, "Stack should export outputs")

	// Cleanup
	if !cfg.SkipCleanup && !t.Failed() {
		t.Log("Cleaning up resources...")
		_, err = stack.Destroy(ctx, optdestroy.ProgressStreams(os.Stdout))
		assert.NoError(t, err, "Stack cleanup should succeed")
	}
}
