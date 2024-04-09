// Copyright 2024 Rubrik, Inc.

package failuregen_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/rubrikinc/failure-test-utils/failuregen"
	"github.com/stretchr/testify/require"
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
	afp := AssureFailuresAt(
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
	afp := AssureFailuresAt(t)

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
	afp := AssureFailuresAt(t)

	os.WriteFile(
		afp.(*failuregen.AssuredFailurePlanImpl).PlanFilePath,
		[]byte("malformed"),
		0644)

	for _, fp := range knownFailures {
		require.Error(t, afp.FailMaybe(fp))
	}

	require.Error(t, afp.FailMaybe("no such failure-point"))
}

// AssureFailuresAt creates an assured failure plan with given failure points
func AssureFailuresAt(
	t *testing.T,
	fp ...failuregen.FailurePoint,
) failuregen.AssuredFailurePlan {
	f, err := os.CreateTemp("", "callisto.assured_failure.json.*")
	require.NoError(t, err)
	defer f.Close()
	bytes, err := json.Marshal(fp)
	require.NoError(t, err)
	_, err = f.Write(bytes)
	require.NoError(t, err)
	path := f.Name()
	t.Cleanup(func() {
		err := os.Remove(path)
		if !os.IsNotExist(err) {
			require.NoError(t, err)
		}
	})
	afp := failuregen.NewAssuredFailurePlan()
	afp.(*failuregen.AssuredFailurePlanImpl).PlanFilePath = path
	return afp
}
