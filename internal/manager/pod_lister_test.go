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
	if !active[VnpuKey{LogicID: 0, Template: "vir01"}] {
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
	if active[VnpuKey{LogicID: 0, Template: "vir02"}] {
		t.Errorf("did not expect key {0, vir02} to be active, got %v", active)
	}
	if !active[VnpuKey{LogicID: 0, Template: "vir01"}] {
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
	if active[VnpuKey{LogicID: 1, Template: "vir01"}] {
		t.Errorf("did not expect key {1, vir01} to be active, got %v", active)
	}
	if !active[VnpuKey{LogicID: 0, Template: "vir01"}] {
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
	if !active[VnpuKey{LogicID: 0, Template: "vir01"}] {
		t.Errorf("expected key {0, vir01} from running pod to be active, got %v", active)
	}
	if active[VnpuKey{LogicID: 1, Template: "vir02"}] {
		t.Errorf("did not expect key {1, vir02} from succeeded pod to be active, got %v", active)
	}
}
