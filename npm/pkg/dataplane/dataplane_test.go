package dataplane

import (
	"fmt"
	"testing"

	"github.com/Azure/azure-container-networking/common"
	"github.com/Azure/azure-container-networking/npm/metrics"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/policies"
	"github.com/Azure/azure-container-networking/npm/util"
	testutils "github.com/Azure/azure-container-networking/test/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	nodeName                = "testnode"
	fakeIPSetRestoreSuccess = testutils.TestCmd{
		Cmd:      []string{util.Ipset, util.IpsetRestoreFlag},
		ExitCode: 0,
	}

	setPodKey1 = &ipsets.TranslatedIPSet{
		Metadata: ipsets.NewIPSetMetadata("setpodkey1", ipsets.KeyLabelOfPod),
	}
	testPolicyobj = policies.NPMNetworkPolicy{
		Name: "ns1/testpolicy",
		PodSelectorIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: ipsets.NewIPSetMetadata("setns1", ipsets.Namespace),
			},
			setPodKey1,
			{
				Metadata: ipsets.NewIPSetMetadata("nestedset1", ipsets.NestedLabelOfPod),
				Members: []string{
					"setpodkey1",
				},
			},
		},
		RuleIPSets: []*ipsets.TranslatedIPSet{
			{
				Metadata: ipsets.NewIPSetMetadata("setns2", ipsets.Namespace),
			},
			{
				Metadata: ipsets.NewIPSetMetadata("setpodkey2", ipsets.KeyLabelOfPod),
			},
			{
				Metadata: ipsets.NewIPSetMetadata("setpodkeyval2", ipsets.KeyValueLabelOfPod),
			},
			{
				Metadata: ipsets.NewIPSetMetadata("testcidr1", ipsets.CIDRBlocks),
				Members: []string{
					"10.0.0.0/8",
				},
			},
		},
		ACLs: []*policies.ACLPolicy{
			{
				PolicyID:  "testpol1",
				Target:    policies.Dropped,
				Direction: policies.Egress,
			},
		},
	}
)

func TestNewDataPlane(t *testing.T) {
	metrics.InitializeAll()

	calls := getNewDataplaneTestCalls()
	dp, err := NewDataPlane("testnode", common.NewMockIOShim(calls))
	require.NoError(t, err)

	if dp == nil {
		t.Error("NewDataPlane() returned nil")
	}

	setMetadata := ipsets.NewIPSetMetadata("test", ipsets.Namespace)
	dp.CreateIPSets([]*ipsets.IPSetMetadata{setMetadata})
}

func TestInitializeDataPlane(t *testing.T) {
	metrics.InitializeAll()

	calls := append(getNewDataplaneTestCalls(), policies.GetInitializeTestCalls()...)
	dp, err := NewDataPlane("testnode", common.NewMockIOShim(calls))
	require.NoError(t, err)

	assert.NotNil(t, dp)
	err = dp.InitializeDataPlane()
	require.NoError(t, err)
}

func TestResetDataPlane(t *testing.T) {
	metrics.InitializeAll()

	calls := append(getNewDataplaneTestCalls(), getInitializeTestCalls()...)
	calls = append(calls, getResetTestCalls()...)
	dp, err := NewDataPlane("testnode", common.NewMockIOShim(calls))
	require.NoError(t, err)

	assert.NotNil(t, dp)
	err = dp.InitializeDataPlane()
	require.NoError(t, err)
	err = dp.ResetDataPlane()
	require.NoError(t, err)
}

func TestCreateAndDeleteIpSets(t *testing.T) {
	metrics.InitializeAll()

	calls := getNewDataplaneTestCalls()
	dp, err := NewDataPlane("testnode", common.NewMockIOShim(calls))
	require.NoError(t, err)
	assert.NotNil(t, dp)
	setsTocreate := []*ipsets.IPSetMetadata{
		{
			Name: "test",
			Type: ipsets.Namespace,
		},
		{
			Name: "test1",
			Type: ipsets.Namespace,
		},
	}

	dp.CreateIPSets(setsTocreate)

	// Creating again to see if duplicates get created
	dp.CreateIPSets(setsTocreate)

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.Nil(t, set)
	}
}

func TestAddToSet(t *testing.T) {
	metrics.InitializeAll()

	calls := getNewDataplaneTestCalls()
	dp, err := NewDataPlane("testnode", common.NewMockIOShim(calls))
	require.NoError(t, err)

	setsTocreate := []*ipsets.IPSetMetadata{
		{
			Name: "test",
			Type: ipsets.Namespace,
		},
		{
			Name: "test1",
			Type: ipsets.Namespace,
		},
	}

	dp.CreateIPSets(setsTocreate)

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	podMetadata := NewPodMetadata("testns/a", "10.0.0.1", nodeName)
	err = dp.AddToSets(setsTocreate, podMetadata)
	require.NoError(t, err)

	v6PodMetadata := NewPodMetadata("testns/a", "2001:db8:0:0:0:0:2:1", nodeName)
	// Test IPV6 addess it should error out
	err = dp.AddToSets(setsTocreate, v6PodMetadata)
	require.NoError(t, err)

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.NotNil(t, set)
	}

	err = dp.RemoveFromSets(setsTocreate, podMetadata)
	require.NoError(t, err)

	err = dp.RemoveFromSets(setsTocreate, v6PodMetadata)
	require.NoError(t, err)

	for _, v := range setsTocreate {
		dp.DeleteIPSet(v)
	}

	for _, v := range setsTocreate {
		prefixedName := v.GetPrefixName()
		set := dp.ipsetMgr.GetIPSet(prefixedName)
		assert.Nil(t, set)
	}
}

func TestApplyPolicy(t *testing.T) {
	metrics.InitializeAll()

	calls := append(getNewDataplaneTestCalls(), getAddPolicyTestCallsForDP(&testPolicyobj)...)
	ioShim := common.NewMockIOShim(calls)
	dp, err := NewDataPlane("testnode", ioShim)
	require.NoError(t, err)

	err = dp.AddPolicy(&testPolicyobj)
	require.NoError(t, err)
}

func TestRemovePolicy(t *testing.T) {
	metrics.InitializeAll()

	calls := append(getNewDataplaneTestCalls(), getAddPolicyTestCallsForDP(&testPolicyobj)...)
	calls = append(calls, getRemovePolicyTestCallsForDP(&testPolicyobj)...)
	ioShim := common.NewMockIOShim(calls)
	dp, err := NewDataPlane("testnode", ioShim)
	require.NoError(t, err)

	err = dp.AddPolicy(&testPolicyobj)
	require.NoError(t, err)

	err = dp.RemovePolicy(testPolicyobj.Name)
	require.NoError(t, err)
}

func TestUpdatePolicy(t *testing.T) {
	metrics.InitializeAll()

	updatedTestPolicyobj := testPolicyobj
	updatedTestPolicyobj.ACLs = []*policies.ACLPolicy{
		{
			PolicyID:  "testpol1",
			Target:    policies.Dropped,
			Direction: policies.Ingress,
		},
	}

	calls := append(getNewDataplaneTestCalls(), getAddPolicyTestCallsForDP(&testPolicyobj)...)
	calls = append(calls, getRemovePolicyTestCallsForDP(&testPolicyobj)...)
	calls = append(calls, getAddPolicyTestCallsForDP(&updatedTestPolicyobj)...)
	for _, call := range calls {
		fmt.Println(call)
	}
	ioShim := common.NewMockIOShim(calls)
	dp, err := NewDataPlane("testnode", ioShim)
	require.NoError(t, err)

	err = dp.AddPolicy(&testPolicyobj)
	require.NoError(t, err)

	err = dp.UpdatePolicy(&updatedTestPolicyobj)
	require.NoError(t, err)
}

func getNewDataplaneTestCalls() []testutils.TestCmd {
	return append(getResetTestCalls(), getInitializeTestCalls()...)
}

func getInitializeTestCalls() []testutils.TestCmd {
	return []testutils.TestCmd{}
	// TODO update when piped error is fixed in fexec
	// return policies.GetInitializeTestCalls()
}

func getResetTestCalls() []testutils.TestCmd {
	return ipsets.GetResetTestCalls()
	// TODO update when piped error is fixed in fexec
	// return append(ipsets.GetResetTestCalls(), policies.GetResetTestCalls()...)
}

func getAddPolicyTestCallsForDP(networkPolicy *policies.NPMNetworkPolicy) []testutils.TestCmd {
	toAddOrUpdateSets := getAffectedIPSets(networkPolicy)
	calls := ipsets.GetApplyIPSetsTestCalls(toAddOrUpdateSets, nil)
	calls = append(calls, policies.GetAddPolicyTestCalls(networkPolicy)...)
	return calls
}

func getRemovePolicyTestCallsForDP(networkPolicy *policies.NPMNetworkPolicy) []testutils.TestCmd {
	// NOTE toDeleteSets is only correct if these ipsets are referenced by no other policy in iMgr
	toDeleteSets := getAffectedIPSets(networkPolicy)
	calls := policies.GetRemovePolicyTestCalls(networkPolicy)
	calls = append(calls, ipsets.GetApplyIPSetsTestCalls(nil, toDeleteSets)...)
	return calls
}

func getAffectedIPSets(networkPolicy *policies.NPMNetworkPolicy) []*ipsets.IPSetMetadata {
	sets := make([]*ipsets.IPSetMetadata, 0)
	for _, translatedIPSet := range networkPolicy.PodSelectorIPSets {
		sets = append(sets, translatedIPSet.Metadata)
	}
	for _, translatedIPSet := range networkPolicy.RuleIPSets {
		sets = append(sets, translatedIPSet.Metadata)
	}
	return sets
}
