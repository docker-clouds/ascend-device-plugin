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
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// VnpuKey identifies a vnpu slot by its physical device logicID and template name.
type VnpuKey struct {
	LogicID  int32
	Template string
}

// vnpuAnnotationInfo is a subset of the pod's runtime-info annotation that we
// need to correlate vnpus with pods.
type vnpuAnnotationInfo struct {
	UUID string `json:"UUID,omitempty"`
	Temp string `json:"temp,omitempty"`
}

// PodLister maintains a node-scoped cache of pods for pod-aware vnpu cleanup.
// It is optional: when nil or not synced, CleanupIdleVNPUs skips cleanup
// entirely as a safe default.
type PodLister struct {
	nodeName        string
	informerFactory informers.SharedInformerFactory
	podLister       corelisters.PodLister
	podListerSynced cache.InformerSynced
	stopCh          chan struct{}
}

// NewPodLister creates and starts a node-scoped Pod informer.
func NewPodLister(nodeName string) (*PodLister, error) {
	if nodeName == "" {
		return nil, fmt.Errorf("nodeName is empty")
	}

	config, err := clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))
	if err != nil {
		return nil, fmt.Errorf("build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build clientset: %w", err)
	}

	return newPodLister(nodeName, clientset)
}

// newPodLister creates a PodLister from the given clientset. Shared by
// NewPodLister (production) and tests (fake clientset).
func newPodLister(nodeName string, clientset kubernetes.Interface) (*PodLister, error) {
	pl := &PodLister{
		nodeName: nodeName,
		stopCh:   make(chan struct{}),
	}

	pl.informerFactory = informers.NewSharedInformerFactoryWithOptions(
		clientset,
		0,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fmt.Sprintf("spec.nodeName=%s", nodeName)
		}),
	)

	podInformer := pl.informerFactory.Core().V1().Pods()
	pl.podLister = podInformer.Lister()
	pl.podListerSynced = podInformer.Informer().HasSynced

	pl.informerFactory.Start(pl.stopCh)
	if !cache.WaitForCacheSync(pl.stopCh, pl.podListerSynced) {
		return nil, fmt.Errorf("failed to sync pod informer cache")
	}

	klog.Info("PodLister informer synced for pod-aware vnpu cleanup")
	return pl, nil
}

// Stop shuts down the informer factory. Safe to call multiple times.
func (pl *PodLister) Stop() {
	select {
	case <-pl.stopCh:
	default:
		close(pl.stopCh)
	}
}

// IsSynced returns true when the pod cache has completed its initial sync.
func (pl *PodLister) IsSynced() bool {
	return pl.podListerSynced()
}

// ActiveVnpuKeys returns the (logicID, template) pairs that are referenced by
// pods on this node still actively using (or about to use) their vnpu slots,
// mapped to the claiming pod names. annoKey is the pod annotation key (e.g.
// "huawei.com/Ascend910C") and deviceByUUID resolves a device UUID to its
// Device so the logicID can be obtained.
//
// Pods in a terminal phase (Succeeded/Failed) and non-terminal pods whose
// containers are all crash-looping or terminated (zombies of failed
// workloads) do not contribute keys, so their idle vnpus stay eligible for
// cleanup.
func (pl *PodLister) ActiveVnpuKeys(annoKey string, deviceByUUID func(string) *Device) map[VnpuKey][]string {
	pods, err := pl.podLister.List(labels.Everything())
	if err != nil {
		klog.Warningf("PodLister: failed to list pods: %v", err)
		return nil
	}

	active := make(map[VnpuKey][]string)
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		if !podActivelyUsesVnpu(pod) {
			klog.Infof("PodLister: pod %s/%s holds vnpu annotation but all containers are in CrashLoopBackOff/terminated state; treating as zombie, not protecting slots",
				pod.Namespace, pod.Name)
			continue
		}

		anno, ok := pod.Annotations[annoKey]
		if !ok || anno == "" {
			continue
		}

		var infos []vnpuAnnotationInfo
		if err := json.Unmarshal([]byte(anno), &infos); err != nil {
			klog.V(4).Infof("PodLister: failed to unmarshal annotation %s for pod %s/%s: %v",
				annoKey, pod.Namespace, pod.Name, err)
			continue
		}

		for _, info := range infos {
			if info.UUID == "" {
				continue
			}
			dev := deviceByUUID(info.UUID)
			if dev == nil {
				continue
			}
			key := VnpuKey{LogicID: dev.LogicID, Template: info.Temp}
			active[key] = append(active[key], pod.Namespace+"/"+pod.Name)
			klog.V(4).Infof("PodLister: active vnpu key %+v from pod %s/%s (phase=%s)",
				key, pod.Namespace, pod.Name, pod.Status.Phase)
		}
	}
	return active
}

// podActivelyUsesVnpu reports whether the pod may still make use of its
// claimed vnpus and therefore needs its slots protected from idle cleanup.
//
// Protective: any container Running, waiting in a non-backoff phase
// (ContainerCreating/PodInitializing/ImagePullBackOff/...), or no container
// status yet (freshly scheduled pod, allocation-to-start race window).
// Not protective: every container is either Waiting with reason
// CrashLoopBackOff (kubelet backoff after repeated crashes) or Terminated
// awaiting restart — the pod is a zombie whose containers get vnpus
// re-created on demand if they ever start again.
func podActivelyUsesVnpu(pod *corev1.Pod) bool {
	statuses := make([]corev1.ContainerStatus, 0,
		len(pod.Status.InitContainerStatuses)+len(pod.Status.ContainerStatuses))
	statuses = append(statuses, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	if len(statuses) == 0 {
		return true
	}
	for _, cs := range statuses {
		switch {
		case cs.State.Running != nil:
			return true
		case cs.State.Terminated != nil:
			continue
		case cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff":
			continue
		default:
			return true
		}
	}
	return false
}
