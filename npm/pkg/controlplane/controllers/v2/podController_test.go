// Copyright 2018 Microsoft. All rights reserved.
// MIT License
package controllers

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/Azure/azure-container-networking/npm/pkg/dataplane"
	"github.com/Azure/azure-container-networking/npm/pkg/dataplane/ipsets"
	dpmocks "github.com/Azure/azure-container-networking/npm/pkg/dataplane/mocks"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
)

const (
	HostNetwork    = true
	NonHostNetwork = false
)

// To indicate the object is needed to be DeletedFinalStateUnknown Object
type IsDeletedFinalStateUnknownObject bool

const (
	DeletedFinalStateUnknownObject IsDeletedFinalStateUnknownObject = true
	DeletedFinalStateknownObject   IsDeletedFinalStateUnknownObject = false
)

type podFixture struct {
	t *testing.T

	// Objects to put in the store.
	podLister []*corev1.Pod
	// (TODO) Actions expected to happen on the client. Will use this to check action.
	kubeactions []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object

	dp            dataplane.GenericDataplane
	podController *PodController
	kubeInformer  kubeinformers.SharedInformerFactory
}

func newFixture(t *testing.T, dp dataplane.GenericDataplane) *podFixture {
	f := &podFixture{
		t:           t,
		podLister:   []*corev1.Pod{},
		kubeobjects: []runtime.Object{},
		dp:          dp,
	}
	return f
}

func getKey(obj interface{}, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		t.Errorf("Unexpected error getting key for obj %v: %v", obj, err)
		return ""
	}
	return key
}

func (f *podFixture) newPodController(stopCh chan struct{}) {
	kubeclient := k8sfake.NewSimpleClientset(f.kubeobjects...)
	f.kubeInformer = kubeinformers.NewSharedInformerFactory(kubeclient, noResyncPeriodFunc())

	npmNamespaceCache := &NpmNamespaceCache{NsMap: make(map[string]*Namespace)}
	f.podController = NewPodController(f.kubeInformer.Core().V1().Pods(), f.dp, npmNamespaceCache)

	for _, pod := range f.podLister {
		f.kubeInformer.Core().V1().Pods().Informer().GetIndexer().Add(pod)
	}

	// Do not start informer to avoid unnecessary event triggers
	// (TODO): Leave stopCh and below commented code to enhance UTs to even check event triggers as well later if possible
	// f.kubeInformer.Start(stopCh)
}

func createPod(name, ns, rv, podIP string, labels map[string]string, isHostNewtwork bool, podPhase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			Labels:          labels,
			ResourceVersion: rv,
		},
		Spec: corev1.PodSpec{
			HostNetwork: isHostNewtwork,
			Containers: []corev1.Container{
				{
					Ports: []corev1.ContainerPort{
						{
							Name:          fmt.Sprintf("app:%s", name),
							ContainerPort: 8080,
							// Protocol:      "TCP",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: podPhase,
			PodIP: podIP,
		},
	}
}

func addPod(t *testing.T, f *podFixture, podObj *corev1.Pod) {
	// simulate pod add event and add pod object to sharedInformer cache
	f.podController.addPod(podObj)

	if f.podController.workqueue.Len() == 0 {
		t.Logf("Add Pod: worker queue length is 0 ")
		return
	}

	f.podController.processNextWorkItem()
}

func deletePod(t *testing.T, f *podFixture, podObj *corev1.Pod, isDeletedFinalStateUnknownObject IsDeletedFinalStateUnknownObject) {
	addPod(t, f, podObj)
	t.Logf("Complete add pod event")

	// simulate pod delete event and delete pod object from sharedInformer cache
	f.kubeInformer.Core().V1().Pods().Informer().GetIndexer().Delete(podObj)

	if isDeletedFinalStateUnknownObject {
		podKey := getKey(podObj, t)
		tombstone := cache.DeletedFinalStateUnknown{
			Key: podKey,
			Obj: podObj,
		}
		f.podController.deletePod(tombstone)
	} else {
		f.podController.deletePod(podObj)
	}

	if f.podController.workqueue.Len() == 0 {
		t.Logf("Delete Pod: worker queue length is 0 ")
		return
	}

	f.podController.processNextWorkItem()
}

// Need to make more cases - interestings..
func updatePod(t *testing.T, f *podFixture, oldPodObj, newPodObj *corev1.Pod) {
	addPod(t, f, oldPodObj)
	t.Logf("Complete add pod event")

	// simulate pod update event and update the pod to shared informer's cache
	f.kubeInformer.Core().V1().Pods().Informer().GetIndexer().Update(newPodObj)
	f.podController.updatePod(oldPodObj, newPodObj)

	if f.podController.workqueue.Len() == 0 {
		t.Logf("Update Pod: worker queue length is 0 ")
		return
	}

	f.podController.processNextWorkItem()
}

type expectedValues struct {
	expectedLenOfPodMap    int
	expectedLenOfNsMap     int
	expectedLenOfWorkQueue int
}

func checkPodTestResult(testName string, f *podFixture, testCases []expectedValues) {
	for _, test := range testCases {
		if got := len(f.podController.podMap); got != test.expectedLenOfPodMap {
			f.t.Errorf("%s failed @ PodMap length = %d, want %d", testName, got, test.expectedLenOfPodMap)
		}
		if got := len(f.podController.npmNamespaceCache.NsMap); got != test.expectedLenOfNsMap {
			f.t.Errorf("%s failed @ NsMap length = %d, want %d", testName, got, test.expectedLenOfNsMap)
		}
		if got := f.podController.workqueue.Len(); got != test.expectedLenOfWorkQueue {
			f.t.Errorf("%s failed @ Workqueue length = %d, want %d", testName, got, test.expectedLenOfWorkQueue)
		}
	}
}

func checkNpmPodWithInput(testName string, f *podFixture, inputPodObj *corev1.Pod) {
	podKey := getKey(inputPodObj, f.t)
	cachedNpmPodObj := f.podController.podMap[podKey]

	if cachedNpmPodObj.PodIP != inputPodObj.Status.PodIP {
		f.t.Errorf("%s failed @ PodIp check got = %s, want %s", testName, cachedNpmPodObj.PodIP, inputPodObj.Status.PodIP)
	}

	if !reflect.DeepEqual(cachedNpmPodObj.Labels, inputPodObj.Labels) {
		f.t.Errorf("%s failed @ Labels check got = %v, want %v", testName, cachedNpmPodObj.Labels, inputPodObj.Labels)
	}

	inputPortList := getContainerPortList(inputPodObj)
	if !reflect.DeepEqual(cachedNpmPodObj.ContainerPorts, inputPortList) {
		f.t.Errorf("%s failed @ Container port check got = %v, want %v", testName, cachedNpmPodObj.PodIP, inputPortList)
	}
}

func TestAddMultiplePods(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj1 := createPod("test-pod-1", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)
	podObj2 := createPod("test-pod-2", "test-namespace", "0", "1.2.3.5", labels, NonHostNetwork, corev1.PodRunning)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)

	f.podLister = append(f.podLister, podObj1, podObj2)
	f.kubeobjects = append(f.kubeobjects, podObj1, podObj2)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	mockIPSets := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfPod),
		ipsets.NewIPSetMetadata("app:test-pod", ipsets.KeyValueLabelOfPod),
	}
	podMetadata1 := dataplane.NewPodMetadata("test-namespace/test-pod-1", "1.2.3.4", "")
	podMetadata2 := dataplane.NewPodMetadata("test-namespace/test-pod-2", "1.2.3.5", "")

	dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, mockIPSets[:1]).Return(nil).Times(1)
	for _, metaData := range []*dataplane.PodMetadata{podMetadata1, podMetadata2} {
		dp.EXPECT().AddToSets(mockIPSets[:1], metaData).Return(nil).Times(1)
		dp.EXPECT().AddToSets(mockIPSets[1:], metaData).Return(nil).Times(1)
	}
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod-1", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod-1", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod-2", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod-2", "1.2.3.5,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)

	addPod(t, f, podObj1)
	addPod(t, f, podObj2)

	testCases := []expectedValues{
		{2, 1, 0},
	}
	checkPodTestResult("TestAddMultiplePods", f, testCases)
	checkNpmPodWithInput("TestAddMultiplePods", f, podObj1)
	checkNpmPodWithInput("TestAddMultiplePods", f, podObj2)
}

func TestAddPod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, podObj)
	f.kubeobjects = append(f.kubeobjects, podObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	mockIPSets := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfPod),
		ipsets.NewIPSetMetadata("app:test-pod", ipsets.KeyValueLabelOfPod),
	}
	podMetadata1 := dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4", "")

	dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, mockIPSets[:1]).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(1)

	addPod(t, f, podObj)
	testCases := []expectedValues{
		{1, 1, 0},
	}
	checkPodTestResult("TestAddPod", f, testCases)
	checkNpmPodWithInput("TestAddPod", f, podObj)
}

func TestAddHostNetworkPod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, HostNetwork, corev1.PodRunning)
	podKey := getKey(podObj, t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, podObj)
	f.kubeobjects = append(f.kubeobjects, podObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	addPod(t, f, podObj)
	testCases := []expectedValues{
		{0, 0, 0},
	}
	checkPodTestResult("TestAddHostNetworkPod", f, testCases)

	if _, exists := f.podController.podMap[podKey]; exists {
		t.Error("TestAddHostNetworkPod failed @ cached pod obj exists check")
	}
}

func TestDeletePod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)
	podKey := getKey(podObj, t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, podObj)
	f.kubeobjects = append(f.kubeobjects, podObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	// Add pod section
	mockIPSets := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfPod),
		ipsets.NewIPSetMetadata("app:test-pod", ipsets.KeyValueLabelOfPod),
	}
	podMetadata1 := dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4", "")

	dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, mockIPSets[:1]).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	// Delete pod section
	dp.EXPECT().RemoveFromSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().RemoveFromSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		RemoveFromSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)

	deletePod(t, f, podObj, DeletedFinalStateknownObject)
	testCases := []expectedValues{
		{0, 1, 0},
	}

	checkPodTestResult("TestDeletePod", f, testCases)
	if _, exists := f.podController.podMap[podKey]; exists {
		t.Error("TestDeletePod failed @ cached pod obj exists check")
	}
}

func TestDeleteHostNetworkPod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, HostNetwork, corev1.PodRunning)
	podKey := getKey(podObj, t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, podObj)
	f.kubeobjects = append(f.kubeobjects, podObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	deletePod(t, f, podObj, DeletedFinalStateknownObject)
	testCases := []expectedValues{
		{0, 0, 0},
	}
	checkPodTestResult("TestDeleteHostNetworkPod", f, testCases)
	if _, exists := f.podController.podMap[podKey]; exists {
		t.Error("TestDeleteHostNetworkPod failed @ cached pod obj exists check")
	}
}

// this UT only tests deletePod event handler function in podController
func TestDeletePodWithTombstone(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	podKey := getKey(podObj, t)
	tombstone := cache.DeletedFinalStateUnknown{
		Key: podKey,
		Obj: podObj,
	}

	f.podController.deletePod(tombstone)
	testCases := []expectedValues{
		{0, 0, 1},
	}
	checkPodTestResult("TestDeletePodWithTombstone", f, testCases)
}

func TestDeletePodWithTombstoneAfterAddingPod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, podObj)
	f.kubeobjects = append(f.kubeobjects, podObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	// Add pod section
	mockIPSets := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfPod),
		ipsets.NewIPSetMetadata("app:test-pod", ipsets.KeyValueLabelOfPod),
	}
	podMetadata1 := dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4", "")

	dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, mockIPSets[:1]).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	// Delete pod section
	dp.EXPECT().RemoveFromSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().RemoveFromSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		RemoveFromSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)

	deletePod(t, f, podObj, DeletedFinalStateUnknownObject)
	testCases := []expectedValues{
		{0, 1, 0},
	}
	checkPodTestResult("TestDeletePodWithTombstoneAfterAddingPod", f, testCases)
}

func TestLabelUpdatePod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	oldPodObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, oldPodObj)
	f.kubeobjects = append(f.kubeobjects, oldPodObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	newPodObj := oldPodObj.DeepCopy()
	newPodObj.Labels = map[string]string{
		"app": "new-test-pod",
	}
	// oldPodObj.ResourceVersion value is "0"
	newRV, _ := strconv.Atoi(oldPodObj.ResourceVersion)
	newPodObj.ResourceVersion = fmt.Sprintf("%d", newRV+1)

	// Add pod section
	mockIPSets := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfPod),
		ipsets.NewIPSetMetadata("app:test-pod", ipsets.KeyValueLabelOfPod),
	}
	podMetadata1 := dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4", "")

	dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, mockIPSets[:1]).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	// Update section
	dp.EXPECT().RemoveFromSets(mockIPSets[2:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().AddToSets([]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:new-test-pod", ipsets.KeyValueLabelOfPod)}, podMetadata1).Return(nil).Times(1)

	updatePod(t, f, oldPodObj, newPodObj)

	testCases := []expectedValues{
		{1, 1, 0},
	}
	checkPodTestResult("TestLabelUpdatePod", f, testCases)
	checkNpmPodWithInput("TestLabelUpdatePod", f, newPodObj)
}

func TestIPAddressUpdatePod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	oldPodObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, oldPodObj)
	f.kubeobjects = append(f.kubeobjects, oldPodObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	newPodObj := oldPodObj.DeepCopy()
	// oldPodObj.ResourceVersion value is "0"
	newRV, _ := strconv.Atoi(oldPodObj.ResourceVersion)
	newPodObj.ResourceVersion = fmt.Sprintf("%d", newRV+1)
	// oldPodObj PodIP is "1.2.3.4"
	newPodObj.Status.PodIP = "4.3.2.1"
	// Add pod section
	mockIPSets := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfPod),
		ipsets.NewIPSetMetadata("app:test-pod", ipsets.KeyValueLabelOfPod),
	}
	podMetadata1 := dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4", "")

	dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, mockIPSets[:1]).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	// Delete pod section
	dp.EXPECT().RemoveFromSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().RemoveFromSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		RemoveFromSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	// New IP Pod add
	podMetadata2 := dataplane.NewPodMetadata("test-namespace/test-pod", "4.3.2.1", "")
	dp.EXPECT().AddToSets(mockIPSets[:1], podMetadata2).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[1:], podMetadata2).Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "4.3.2.1,8080", ""),
		).
		Return(nil).Times(1)

	updatePod(t, f, oldPodObj, newPodObj)

	testCases := []expectedValues{
		{1, 1, 0},
	}
	checkPodTestResult("TestIPAddressUpdatePod", f, testCases)
	checkNpmPodWithInput("TestIPAddressUpdatePod", f, newPodObj)
}

func TestPodStatusUpdatePod(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	oldPodObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)
	podKey := getKey(oldPodObj, t)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	f.podLister = append(f.podLister, oldPodObj)
	f.kubeobjects = append(f.kubeobjects, oldPodObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	newPodObj := oldPodObj.DeepCopy()
	// oldPodObj.ResourceVersion value is "0"
	newRV, _ := strconv.Atoi(oldPodObj.ResourceVersion)
	newPodObj.ResourceVersion = fmt.Sprintf("%d", newRV+1)

	// oldPodObj PodIP is "1.2.3.4"
	newPodObj.Status.Phase = corev1.PodSucceeded

	mockIPSets := []*ipsets.IPSetMetadata{
		ipsets.NewIPSetMetadata("test-namespace", ipsets.Namespace),
		ipsets.NewIPSetMetadata("app", ipsets.KeyLabelOfPod),
		ipsets.NewIPSetMetadata("app:test-pod", ipsets.KeyValueLabelOfPod),
	}
	podMetadata1 := dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4", "")

	dp.EXPECT().AddToLists([]*ipsets.IPSetMetadata{kubeAllNamespaces}, mockIPSets[:1]).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().AddToSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		AddToSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)
	dp.EXPECT().ApplyDataPlane().Return(nil).Times(2)
	// Delete pod section
	dp.EXPECT().RemoveFromSets(mockIPSets[:1], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().RemoveFromSets(mockIPSets[1:], podMetadata1).Return(nil).Times(1)
	dp.EXPECT().
		RemoveFromSets(
			[]*ipsets.IPSetMetadata{ipsets.NewIPSetMetadata("app:test-pod", ipsets.NamedPorts)},
			dataplane.NewPodMetadata("test-namespace/test-pod", "1.2.3.4,8080", ""),
		).
		Return(nil).Times(1)

	updatePod(t, f, oldPodObj, newPodObj)

	testCases := []expectedValues{
		{0, 1, 0},
	}
	checkPodTestResult("TestPodStatusUpdatePod", f, testCases)
	if _, exists := f.podController.podMap[podKey]; exists {
		t.Error("TestPodStatusUpdatePod failed @ cached pod obj exists check")
	}
}

func TestPodMapMarshalJSON(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	labels := map[string]string{
		"app": "test-pod",
	}
	pod := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)
	podKey, err := cache.MetaNamespaceKeyFunc(pod)
	assert.NoError(t, err)

	npmPod := newNpmPod(pod)
	f.podController.podMap[podKey] = npmPod

	npMapRaw, err := f.podController.MarshalJSON()
	assert.NoError(t, err)

	expect := []byte(`{"test-namespace/test-pod":{"Name":"test-pod","Namespace":"test-namespace","PodIP":"1.2.3.4","Labels":{},"ContainerPorts":[],"Phase":"Running"}}`)
	fmt.Printf("%s\n", string(npMapRaw))
	assert.ElementsMatch(t, expect, npMapRaw)
}

func TestHasValidPodIP(t *testing.T) {
	podObj := &corev1.Pod{
		Status: corev1.PodStatus{
			Phase: "Running",
			PodIP: "1.2.3.4",
		},
	}
	if ok := hasValidPodIP(podObj); !ok {
		t.Errorf("TestisValidPod failed @ isValidPod")
	}
}

// Extra unit test which is not quite related to PodController,
// but help to understand how workqueue works to make event handler logic lock-free.
// If the same key are queued into workqueue in multiple times,
// they are combined into one item (accurately, if the item is not processed).
func TestWorkQueue(t *testing.T) {
	labels := map[string]string{
		"app": "test-pod",
	}
	podObj := createPod("test-pod", "test-namespace", "0", "1.2.3.4", labels, NonHostNetwork, corev1.PodRunning)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dp := dpmocks.NewMockGenericDataplane(ctrl)
	f := newFixture(t, dp)

	f.podLister = append(f.podLister, podObj)
	f.kubeobjects = append(f.kubeobjects, podObj)
	stopCh := make(chan struct{})
	defer close(stopCh)
	f.newPodController(stopCh)

	podKeys := []string{"test-pod", "test-pod", "test-pod1"}
	expectedWorkQueueLength := []int{1, 1, 2}

	for idx, podKey := range podKeys {
		f.podController.workqueue.Add(podKey)
		workQueueLength := f.podController.workqueue.Len()
		if workQueueLength != expectedWorkQueueLength[idx] {
			t.Errorf("TestWorkQueue failed due to returned workqueue length = %d, want %d",
				workQueueLength, expectedWorkQueueLength)
		}
	}
}
