/*
Copyright 2022 The Koordinator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodenumaresource

import (
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"

	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

type NodeAllocation struct {
	lock               sync.RWMutex
	nodeName           string
	allocatedPods      map[types.UID]PodAllocation
	allocatedCPUs      CPUDetails
	allocatedResources map[int]*NUMANodeResource
}

type PodAllocation struct {
	UID                types.UID                           `json:"uid,omitempty"`
	Namespace          string                              `json:"namespace,omitempty"`
	Name               string                              `json:"name,omitempty"`
	CPUSet             cpuset.CPUSet                       `json:"cpuset,omitempty"`
	CPUExclusivePolicy schedulingconfig.CPUExclusivePolicy `json:"cpuExclusivePolicy,omitempty"`
	NUMANodeResources  []NUMANodeResource                  `json:"numaNodeResources,omitempty"`
}

func NewNodeAllocation(nodeName string) *NodeAllocation {
	return &NodeAllocation{
		nodeName:           nodeName,
		allocatedPods:      map[types.UID]PodAllocation{},
		allocatedCPUs:      NewCPUDetails(),
		allocatedResources: map[int]*NUMANodeResource{},
	}
}

func (n *NodeAllocation) update(allocation *PodAllocation, cpuTopology *CPUTopology) {
	n.release(allocation.UID)
	n.addPodAllocation(allocation, cpuTopology)
}

func (n *NodeAllocation) getCPUs(podUID types.UID) (cpuset.CPUSet, bool) {
	request, ok := n.allocatedPods[podUID]
	return request.CPUSet, ok
}

func (n *NodeAllocation) addCPUs(cpuTopology *CPUTopology, podUID types.UID, cpuset cpuset.CPUSet, exclusivePolicy schedulingconfig.CPUExclusivePolicy) {
	n.addPodAllocation(&PodAllocation{
		UID:                podUID,
		CPUSet:             cpuset,
		CPUExclusivePolicy: exclusivePolicy,
	}, cpuTopology)
}

func (n *NodeAllocation) addPodAllocation(request *PodAllocation, cpuTopology *CPUTopology) {
	if _, ok := n.allocatedPods[request.UID]; ok {
		return
	}
	n.allocatedPods[request.UID] = *request

	for _, cpuID := range request.CPUSet.ToSliceNoSort() {
		cpuInfo, ok := n.allocatedCPUs[cpuID]
		if !ok {
			cpuInfo = cpuTopology.CPUDetails[cpuID]
		}
		cpuInfo.ExclusivePolicy = request.CPUExclusivePolicy
		cpuInfo.RefCount++
		n.allocatedCPUs[cpuID] = cpuInfo
	}

	for nodeID, numaNodeRes := range request.NUMANodeResources {
		res := n.allocatedResources[numaNodeRes.Node]
		if res == nil {
			res = &NUMANodeResource{
				Node:      nodeID,
				Resources: make(corev1.ResourceList),
			}
			n.allocatedResources[numaNodeRes.Node] = res
		}
		res.Resources = quotav1.Add(res.Resources, numaNodeRes.Resources)
	}
}

func (n *NodeAllocation) release(podUID types.UID) {
	request, ok := n.allocatedPods[podUID]
	if !ok {
		return
	}
	delete(n.allocatedPods, podUID)

	for _, cpuID := range request.CPUSet.ToSliceNoSort() {
		cpuInfo, ok := n.allocatedCPUs[cpuID]
		if !ok {
			continue
		}
		cpuInfo.RefCount--
		if cpuInfo.RefCount == 0 {
			delete(n.allocatedCPUs, cpuID)
		} else {
			n.allocatedCPUs[cpuID] = cpuInfo
		}
	}

	for _, numaNodeRes := range request.NUMANodeResources {
		res := n.allocatedResources[numaNodeRes.Node]
		if res != nil {
			res.Resources = quotav1.SubtractWithNonNegativeResult(res.Resources, numaNodeRes.Resources)
		}
	}
}

func (n *NodeAllocation) getAvailableCPUs(cpuTopology *CPUTopology, maxRefCount int, reservedCPUs, preferredCPUs cpuset.CPUSet) (availableCPUs cpuset.CPUSet, allocateInfo CPUDetails) {
	allocateInfo = n.allocatedCPUs.Clone()
	if !preferredCPUs.IsEmpty() {
		for _, cpuID := range preferredCPUs.ToSliceNoSort() {
			cpuInfo, ok := allocateInfo[cpuID]
			if ok {
				cpuInfo.RefCount--
				if cpuInfo.RefCount == 0 {
					delete(allocateInfo, cpuID)
				} else {
					allocateInfo[cpuID] = cpuInfo
				}
			}
		}
	}
	allocated := allocateInfo.CPUs().Filter(func(cpuID int) bool {
		return allocateInfo[cpuID].RefCount >= maxRefCount
	})
	availableCPUs = cpuTopology.CPUDetails.CPUs().Difference(allocated).Difference(reservedCPUs)
	return
}

func (n *NodeAllocation) getAvailableNUMANodeResources(topologyOptions TopologyOptions) (totalAvailable, totalAllocated map[int]corev1.ResourceList) {
	totalAvailable = make(map[int]corev1.ResourceList)
	totalAllocated = make(map[int]corev1.ResourceList)
	for _, numaNodeRes := range topologyOptions.NUMANodeResources {
		var allocatedRes corev1.ResourceList
		allocated := n.allocatedResources[numaNodeRes.Node]
		if allocated != nil {
			allocatedRes = allocated.Resources
			totalAllocated[numaNodeRes.Node] = allocatedRes.DeepCopy()
		}
		totalAvailable[numaNodeRes.Node] = quotav1.SubtractWithNonNegativeResult(numaNodeRes.Resources, allocatedRes)
	}
	return totalAvailable, totalAllocated
}
