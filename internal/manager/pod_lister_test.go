/*
 * Copyright 2026 The HAMi Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package manager

import (
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testAnnoKey = "huawei.com/Ascend910C"

func makePod(name, namespace, nodeName string, phase corev1.PodPhase, annoKey, annoVal string) *corev1.Pod {
	annotations := map[string]string{}
	if annoKey != "" {
		annotations[annoKey] = annoVal
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}

func makeAnnotation(infos []vnpuAnnotationInfo) string {
	b, _ := json.Marshal(infos)
	return string(b)
}

func makeDeviceByUUID(devs []*Device) func(string) *Device {
	return func(uuid string) *Device {
		for _, d := range devs {
			if d.UUID == uuid {
				return d
			}
		}
		return nil
	}
}

func TestActiveVnpuKeys_RunningPodMatchingTemplate(t *testing.T) {
	devs := []*Device{
		{UUID: "uuid-0", LogicID: 0},
	}
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-running", "default", "node1", corev1.PodRunning,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-0", Temp: "vir01"},
			})),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} to be active, got %v", active)
	}
}

func TestActiveVnpuKeys_SucceededPodSkipped(t *testing.T) {
	devs := []*Device{
		{UUID: "uuid-0", LogicID: 0},
	}
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-done", "default", "node1", corev1.PodSucceeded,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-0", Temp: "vir01"},
			})),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if len(active) != 0 {
		t.Errorf("expected no active keys for Succeeded pod, got %v", active)
	}
}

func TestActiveVnpuKeys_FailedPodSkipped(t *testing.T) {
	devs := []*Device{
		{UUID: "uuid-0", LogicID: 0},
	}
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-failed", "default", "node1", corev1.PodFailed,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-0", Temp: "vir01"},
			})),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if len(active) != 0 {
		t.Errorf("expected no active keys for Failed pod, got %v", active)
	}
}

func TestActiveVnpuKeys_DifferentTemplateNotBlocking(t *testing.T) {
	devs := []*Device{
		{UUID: "uuid-0", LogicID: 0},
	}
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-running", "default", "node1", corev1.PodRunning,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-0", Temp: "vir01"},
			})),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir02"}]; ok {
		t.Errorf("did not expect key {0, vir02} to be active, got %v", active)
	}
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} to be active, got %v", active)
	}
}

func TestActiveVnpuKeys_DifferentDeviceNotBlocking(t *testing.T) {
	devs := []*Device{
		{UUID: "uuid-0", LogicID: 0},
		{UUID: "uuid-1", LogicID: 1},
	}
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-running", "default", "node1", corev1.PodRunning,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-0", Temp: "vir01"},
			})),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 1, Template: "vir01"}]; ok {
		t.Errorf("did not expect key {1, vir01} to be active, got %v", active)
	}
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} to be active, got %v", active)
	}
}

func TestActiveVnpuKeys_NoPods(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(nil))
	if len(active) != 0 {
		t.Errorf("expected no active keys with no pods, got %v", active)
	}
}

func TestActiveVnpuKeys_NoAnnotation(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-no-anno", "default", "node1", corev1.PodRunning, "", ""),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(nil))
	if len(active) != 0 {
		t.Errorf("expected no active keys for pod without annotation, got %v", active)
	}
}

func TestActiveVnpuKeys_MultiplePodsMixedPhases(t *testing.T) {
	devs := []*Device{
		{UUID: "uuid-0", LogicID: 0},
		{UUID: "uuid-1", LogicID: 1},
	}
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-running", "default", "node1", corev1.PodRunning,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-0", Temp: "vir01"},
			})),
		makePod("pod-succeeded", "default", "node1", corev1.PodSucceeded,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-1", Temp: "vir02"},
			})),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} from running pod to be active, got %v", active)
	}
	if _, ok := active[VnpuKey{LogicID: 1, Template: "vir02"}]; ok {
		t.Errorf("did not expect key {1, vir02} from succeeded pod to be active, got %v", active)
	}
}

// makeStatusfulPod builds a pod on node1 with the given container statuses and
// vnpu annotation claiming (uuid-0, vir01).
func makeStatusfulPod(name string, phase corev1.PodPhase, statuses []corev1.ContainerStatus) *corev1.Pod {
	pod := makePod(name, "default", "node1", phase, testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
		{UUID: "uuid-0", Temp: "vir01"},
	}))
	pod.Status.ContainerStatuses = statuses
	return pod
}

func waitingStatus(reason string) corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:  "main",
		State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: reason}},
	}
}

func runningStatus() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:  "main",
		State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
	}
}

func terminatedStatus() corev1.ContainerStatus {
	return corev1.ContainerStatus{
		Name:  "main",
		State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}},
	}
}

// unknownStatus builds a ContainerStatus whose state kubelet could not
// determine (all of Running/Waiting/Terminated nil).
func unknownStatus() corev1.ContainerStatus {
	return corev1.ContainerStatus{Name: "main"}
}

func TestActiveVnpuKeys_CrashLoopPodExcluded(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-zombie", corev1.PodRunning,
			[]corev1.ContainerStatus{waitingStatus("CrashLoopBackOff")}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if len(active) != 0 {
		t.Errorf("expected no active keys for crash-looping pod, got %v", active)
	}
}

func TestActiveVnpuKeys_AllTerminatedPodExcluded(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-exited", corev1.PodRunning,
			[]corev1.ContainerStatus{terminatedStatus()}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if len(active) != 0 {
		t.Errorf("expected no active keys for all-terminated pod, got %v", active)
	}
}

func TestActiveVnpuKeys_ContainerCreatingProtected(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-starting", corev1.PodRunning,
			[]corev1.ContainerStatus{waitingStatus("ContainerCreating")}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} for ContainerCreating pod, got %v", active)
	}
}

func TestActiveVnpuKeys_ImagePullBackOffProtected(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-pulling", corev1.PodRunning,
			[]corev1.ContainerStatus{waitingStatus("ImagePullBackOff")}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} for ImagePullBackOff pod, got %v", active)
	}
}

func TestActiveVnpuKeys_MixedCrashLoopAndRunningProtected(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-mixed", corev1.PodRunning,
			[]corev1.ContainerStatus{waitingStatus("CrashLoopBackOff"), runningStatus()}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} for mixed pod with a running container, got %v", active)
	}
}

func TestActiveVnpuKeys_PendingNoStatusesProtected(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makePod("pod-fresh", "default", "node1", corev1.PodPending,
			testAnnoKey, makeAnnotation([]vnpuAnnotationInfo{
				{UUID: "uuid-0", Temp: "vir01"},
			})),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} for freshly scheduled pod without container statuses, got %v", active)
	}
}

// TestPodActivelyUsesVnpu directly covers the pure classification function;
// the ActiveVnpuKeys tests above cover it indirectly through the full pipeline.
func TestPodActivelyUsesVnpu(t *testing.T) {
	tests := []struct {
		name     string
		phase    corev1.PodPhase
		init     bool // place statuses on init containers instead of regular ones
		statuses []corev1.ContainerStatus
		want     bool
	}{
		{name: "no statuses (fresh pod race window)", phase: corev1.PodRunning, want: true},
		{name: "running", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{runningStatus()}, want: true},
		{name: "all terminated", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{terminatedStatus()}, want: false},
		{name: "all crashloop", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{waitingStatus("CrashLoopBackOff")}, want: false},
		{name: "container creating", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{waitingStatus("ContainerCreating")}, want: true},
		{name: "waiting with empty reason", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{waitingStatus("")}, want: true},
		{name: "undetermined on pending pod (pre-start)", phase: corev1.PodPending, statuses: []corev1.ContainerStatus{unknownStatus()}, want: true},
		{name: "undetermined on running pod (status lost)", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{unknownStatus()}, want: false},
		{name: "undetermined init container on running pod", phase: corev1.PodRunning, init: true, statuses: []corev1.ContainerStatus{unknownStatus()}, want: false},
		{name: "undetermined + terminated", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{unknownStatus(), terminatedStatus()}, want: false},
		{name: "undetermined + running sidecar", phase: corev1.PodRunning, statuses: []corev1.ContainerStatus{unknownStatus(), runningStatus()}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{Status: corev1.PodStatus{Phase: tt.phase}}
			if tt.init {
				pod.Status.InitContainerStatuses = tt.statuses
			} else {
				pod.Status.ContainerStatuses = tt.statuses
			}
			if got := podActivelyUsesVnpu(pod); got != tt.want {
				t.Errorf("podActivelyUsesVnpu() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestActiveVnpuKeys_UnknownStateRunningPodExcluded pins the field scenario:
// a non-Pending pod whose container state is undeterminable (all states nil,
// the API form of containerStatusUnknown) must not protect its vnpu slots.
func TestActiveVnpuKeys_UnknownStateRunningPodExcluded(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-status-lost", corev1.PodRunning,
			[]corev1.ContainerStatus{unknownStatus()}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if len(active) != 0 {
		t.Errorf("expected no active keys for pod with undeterminable container state, got %v", active)
	}
}

// TestActiveVnpuKeys_UnknownStatePendingPodProtected keeps the pre-start
// window protective: a state-less container entry on a Pending pod is normal
// (container not yet created), not a lost status.
func TestActiveVnpuKeys_UnknownStatePendingPodProtected(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-prestart", corev1.PodPending,
			[]corev1.ContainerStatus{unknownStatus()}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} for Pending pod in pre-start window, got %v", active)
	}
}

// TestActiveVnpuKeys_UnknownPhasePodExcluded: a pod on an unreachable node
// (phase Unknown) is skipped at the phase level even when its stale container
// snapshot still shows Running — the hardware query stays authoritative.
func TestActiveVnpuKeys_UnknownPhasePodExcluded(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-node-lost", corev1.PodUnknown,
			[]corev1.ContainerStatus{runningStatus()}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if len(active) != 0 {
		t.Errorf("expected no active keys for phase-Unknown pod, got %v", active)
	}
}

// TestActiveVnpuKeys_MixedUnknownAndRunningProtected: a pod with one
// undeterminable container but another still Running stays protective.
func TestActiveVnpuKeys_MixedUnknownAndRunningProtected(t *testing.T) {
	devs := []*Device{{UUID: "uuid-0", LogicID: 0}}
	fakeClient := fake.NewSimpleClientset(
		makeStatusfulPod("pod-sidecar-running", corev1.PodRunning,
			[]corev1.ContainerStatus{unknownStatus(), runningStatus()}),
	)

	pl, err := newPodLister("node1", fakeClient)
	if err != nil {
		t.Fatalf("newPodLister: %v", err)
	}
	defer pl.Stop()

	active := pl.ActiveVnpuKeys(testAnnoKey, makeDeviceByUUID(devs))
	if _, ok := active[VnpuKey{LogicID: 0, Template: "vir01"}]; !ok {
		t.Errorf("expected key {0, vir01} for pod with a running sidecar, got %v", active)
	}
}
