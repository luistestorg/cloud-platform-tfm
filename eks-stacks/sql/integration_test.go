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

// TestSQLStackReference validates sql references infra-aws correctly
func TestSQLStackReference(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	// Check infra-aws stack exists
	infraAwsRef := "organization/eks-infra-aws/dev"
	_, err = auto.SelectStack(ctx, infraAwsRef, ws)
	if err != nil {
		t.Skip("Skipping test - infra-aws stack not available")
		return
	}

	// Check sql stack
	sqlRef := "organization/eks-sql/dev"
	sqlStack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
		return
	}

	// Verify sql has reference to infra-aws
	config, err := sqlStack.GetAllConfig(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get config")
		return
	}

	infraStackRef, hasRef := config["eks-sql:infraStackRef"]
	assert.True(t, hasRef, "sql should have infraStackRef configuration")

	if hasRef {
		assert.Contains(t, infraStackRef.Value, "eks-infra-aws",
			"infraStackRef should reference eks-infra-aws stack")
	}

	t.Logf("✓ Stack reference validation passed")
}

// TestRDSEndpointExported validates RDS endpoint is exported
func TestRDSEndpointExported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify RDS endpoint output
	rdsEndpoint, exists := outputs["rdsEndpoint"]
	assert.True(t, exists, "rdsEndpoint output should exist")

	if exists {
		endpointStr := fmt.Sprintf("%v", rdsEndpoint.Value)
		assert.Contains(t, endpointStr, ".rds.amazonaws.com",
			"RDS endpoint should be an RDS hostname")
		assert.Contains(t, endpointStr, ":5432",
			"RDS endpoint should include PostgreSQL port")
	}

	t.Logf("✓ RDS endpoint validated")
}

// TestRDSAddressExported validates RDS address is exported
func TestRDSAddressExported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify RDS address output
	rdsAddress, exists := outputs["rdsAddress"]
	assert.True(t, exists, "rdsAddress output should exist")

	if exists {
		addressStr := fmt.Sprintf("%v", rdsAddress.Value)
		assert.Contains(t, addressStr, ".rds.amazonaws.com",
			"RDS address should be an RDS hostname")
	}

	t.Logf("✓ RDS address validated")
}

// TestRDSPortExported validates RDS port is exported
func TestRDSPortExported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify RDS port output
	rdsPort, exists := outputs["rdsPort"]
	assert.True(t, exists, "rdsPort output should exist")

	if exists {
		portValue := fmt.Sprintf("%v", rdsPort.Value)
		assert.Equal(t, "5432", portValue, "RDS port should be 5432 for PostgreSQL")
	}

	t.Logf("✓ RDS port validated")
}

// TestRDSArnExported validates RDS ARN is exported
func TestRDSArnExported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify RDS ARN output
	rdsArn, exists := outputs["rdsArn"]
	assert.True(t, exists, "rdsArn output should exist")

	if exists {
		arnStr := fmt.Sprintf("%v", rdsArn.Value)
		assert.Contains(t, arnStr, "arn:aws:rds:",
			"RDS ARN should have correct format")
	}

	t.Logf("✓ RDS ARN validated")
}

// TestSQLStackDependencyOrder validates deployment order
func TestSQLStackDependencyOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	// Verify infra-aws exists
	infraAwsRef := "organization/eks-infra-aws/dev"
	_, err = auto.SelectStack(ctx, infraAwsRef, ws)
	infraAwsExists := err == nil

	// Verify sql
	sqlRef := "organization/eks-sql/dev"
	_, err = auto.SelectStack(ctx, sqlRef, ws)
	sqlExists := err == nil

	if sqlExists {
		assert.True(t, infraAwsExists,
			"sql should not exist without infra-aws (dependency violation)")
	}

	t.Logf("✓ Stack dependency order validated")
}

// TestSQLStackResourceCount validates resource count
func TestSQLStackResourceCount(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
		return
	}

	info, err := stack.Info(ctx)
	if err != nil || info.ResourceCount == nil {
		t.Skip("Skipping test - cannot get resource count")
		return
	}

	resourceCount := *info.ResourceCount

	t.Logf("sql stack has %d resources", resourceCount)

	// SQL stack typically has 5-15 resources (RDS, SG, subnet group, params, alarms)
	assert.Greater(t, resourceCount, 3, "Stack should have meaningful resources")
	assert.Less(t, resourceCount, 50, "Stack should follow micro-stack principles")

	t.Logf("✓ Resource count validated")
}

// TestSQLStackConfiguration validates required configuration
func TestSQLStackConfiguration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
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
		"eks-sql:infraStackRef",
		"eks-sql:dbName",
		"eks-sql:dbInstanceType",
		"eks-sql:zone",
	}

	for _, key := range requiredConfig {
		_, exists := config[key]
		assert.True(t, exists, fmt.Sprintf("Config %s should be set", key))
	}

	t.Logf("✓ Stack configuration validated")
}

// TestSQLBlastRadius validates micro-stack isolation
func TestSQLBlastRadius(t *testing.T) {
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
		"organization/eks-sql/dev",
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

	// Calculate sql blast radius
	sqlCount, hasSQL := stackResourceCounts["organization/eks-sql/dev"]
	if !hasSQL {
		t.Skip("Skipping test - sql resource count not available")
		return
	}

	blastRadius := float64(sqlCount) / float64(totalResources) * 100

	t.Logf("sql blast radius: %.1f%% (%d/%d resources)",
		blastRadius, sqlCount, totalResources)

	// sql should represent < 30% of total infrastructure
	assert.Less(t, blastRadius, 30.0,
		"sql blast radius should be < 30% of total infrastructure")

	t.Logf("✓ Blast radius validated")
}

// TestDatabaseNameExported validates database name is exported
func TestDatabaseNameExported(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	if err != nil {
		t.Skip("Skipping test - no Pulumi workspace available")
		return
	}

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)
	if err != nil {
		t.Skip("Skipping test - sql stack not deployed")
		return
	}

	outputs, err := stack.Outputs(ctx)
	if err != nil {
		t.Skip("Skipping test - cannot get outputs")
		return
	}

	// Verify database name output
	dbName, exists := outputs["rdsDbName"]
	assert.True(t, exists, "rdsDbName output should exist")

	if exists {
		nameStr := fmt.Sprintf("%v", dbName.Value)
		assert.NotEmpty(t, nameStr, "Database name should not be empty")
		assert.Equal(t, "tfmdb", nameStr, "Database name should be 'tfmdb'")
	}

	t.Logf("✓ Database name validated")
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

	sqlRef := "organization/eks-sql/dev"
	stack, err := auto.SelectStack(ctx, sqlRef, ws)

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

// TestFullSQLDeployment performs end-to-end deployment test
func TestFullSQLDeployment(t *testing.T) {
	if os.Getenv("RUN_E2E_TESTS") != "true" {
		t.Skip("Skipping E2E test (set RUN_E2E_TESTS=true to run)")
	}

	ctx := context.Background()

	ws, err := auto.NewLocalWorkspace(ctx)
	require.NoError(t, err)

	stackName := "sql-e2e-test"
	stack, err := auto.UpsertStack(ctx, stackName, ws)
	require.NoError(t, err)

	// Configure stack
	err = stack.SetConfig(ctx, "aws:region", auto.ConfigValue{Value: "us-east-1"})
	require.NoError(t, err)

	err = stack.SetConfig(ctx, "eks-sql:dbName", auto.ConfigValue{Value: "testdb"})
	require.NoError(t, err)

	err = stack.SetConfig(ctx, "eks-sql:dbInstanceType", auto.ConfigValue{Value: "db.t4g.micro"})
	require.NoError(t, err)

	// Deploy with timeout
	deployCtx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	t.Log("Starting E2E deployment (this will take 15-20 minutes)...")

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
