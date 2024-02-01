// Copyright 2024 Rubrik, Inc.

package failuregen_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"rubrik/cqlproxy/failuregen"
	"rubrik/cqlproxy/testutil"
)

var knownFailures = []failuregen.FailurePoint{
	failuregen.SChTargetStateP1,
	failuregen.BeforeAdditiveSchemaChange,
	failuregen.AfterAdditiveSchemaChange,
	failuregen.SChTargetStateUR2,
	failuregen.SChTargetStateUR2Q,
	failuregen.SChTargetStateMT3,
	failuregen.SChTargetStateEM4,
	failuregen.BeforeMetadataMigration,
	failuregen.AfterMetadataMigration,
	failuregen.SChTargetStateRR5,
	failuregen.SChTargetStateC6,
	failuregen.BeforeDestructiveSchemaChange,
	failuregen.AfterDestructiveSchemaChange,
	failuregen.SChTargetStateNU0,
}

func TestAssuredFailureGeneratorInjectsDesiredFailures(t *testing.T) {
	afp := testutil.AssureFailuresAt(
		t,
		failuregen.AfterAdditiveSchemaChange,
		failuregen.BeforeMetadataMigration,
		failuregen.SChTargetStateMT3)

	for _, fp := range knownFailures {
		if fp == failuregen.AfterAdditiveSchemaChange ||
			fp == failuregen.BeforeMetadataMigration ||
			fp == failuregen.SChTargetStateMT3 {
			require.Error(t, afp.FailMaybe(fp))
		} else {
			require.NoError(t, afp.FailMaybe(fp))
		}
	}

	require.NoError(t, afp.FailMaybe("no such failure-point"))
}

func TestAssuredFailureGeneratorWithNoPlan(t *testing.T) {
	afp := testutil.AssureFailuresAt(t)

	for _, fp := range knownFailures {
		require.NoError(t, afp.FailMaybe(fp))
	}
	require.NoError(t, afp.FailMaybe("no such failure-point"))

	os.Remove(afp.(*failuregen.AssuredFailurePlanImpl).PlanFilePath)
	for _, fp := range knownFailures {
		require.NoError(t, afp.FailMaybe(fp))
	}
	require.NoError(t, afp.FailMaybe("no such failure-point"))

	os.WriteFile(
		afp.(*failuregen.AssuredFailurePlanImpl).PlanFilePath,
		nil,
		0644)

	for _, fp := range knownFailures {
		require.NoError(t, afp.FailMaybe(fp))
	}
	require.NoError(t, afp.FailMaybe("no such failure-point"))
}

func TestAssuredFailureGeneratorFailsForMalformedPlan(t *testing.T) {
	afp := testutil.AssureFailuresAt(t)

	os.WriteFile(
		afp.(*failuregen.AssuredFailurePlanImpl).PlanFilePath,
		[]byte("malformed"),
		0644)

	for _, fp := range knownFailures {
		require.Error(t, afp.FailMaybe(fp))
	}

	require.Error(t, afp.FailMaybe("no such failure-point"))
}
