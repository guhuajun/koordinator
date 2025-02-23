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
	"errors"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	quotav1 "k8s.io/apiserver/pkg/quota/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/scheduler/framework"

	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/topologymanager"
	"github.com/koordinator-sh/koordinator/pkg/util/bitmask"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

type ResourceManager interface {
	GetTopologyHints(node *corev1.Node, pod *corev1.Pod, options *ResourceOptions) (map[string][]topologymanager.NUMATopologyHint, error)
	Allocate(node *corev1.Node, pod *corev1.Pod, options *ResourceOptions) (*PodAllocation, error)

	Update(nodeName string, allocation *PodAllocation)
	Release(nodeName string, podUID types.UID)

	GetNodeAllocation(nodeName string) *NodeAllocation
	GetAllocatedCPUSet(nodeName string, podUID types.UID) (cpuset.CPUSet, bool)
	GetAvailableCPUs(nodeName string, preferredCPUs cpuset.CPUSet) (availableCPUs cpuset.CPUSet, allocated CPUDetails, err error)
}

type ResourceOptions struct {
	numCPUsNeeded         int
	requestCPUBind        bool
	requests              corev1.ResourceList
	originalRequests      corev1.ResourceList
	requiredCPUBindPolicy bool
	cpuBindPolicy         schedulingconfig.CPUBindPolicy
	cpuExclusivePolicy    schedulingconfig.CPUExclusivePolicy
	preferredCPUs         cpuset.CPUSet
	reusableResources     map[int]corev1.ResourceList
	hint                  topologymanager.NUMATopologyHint
	topologyOptions       TopologyOptions
}

type resourceManager struct {
	numaAllocateStrategy   schedulingconfig.NUMAAllocateStrategy
	topologyOptionsManager TopologyOptionsManager
	lock                   sync.Mutex
	nodeAllocations        map[string]*NodeAllocation
}

func NewResourceManager(
	handle framework.Handle,
	defaultNUMAAllocateStrategy schedulingconfig.NUMAAllocateStrategy,
	topologyOptionsManager TopologyOptionsManager,
) ResourceManager {
	manager := &resourceManager{
		numaAllocateStrategy:   defaultNUMAAllocateStrategy,
		topologyOptionsManager: topologyOptionsManager,
		nodeAllocations:        map[string]*NodeAllocation{},
	}
	handle.SharedInformerFactory().Core().V1().Nodes().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{DeleteFunc: manager.onNodeDelete})
	return manager
}

func (c *resourceManager) onNodeDelete(obj interface{}) {
	var node *corev1.Node
	switch t := obj.(type) {
	case *corev1.Node:
		node = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		node, ok = t.Obj.(*corev1.Node)
		if !ok {
			return
		}
	default:
		break
	}

	if node == nil {
		return
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	delete(c.nodeAllocations, node.Name)
}

func (c *resourceManager) getOrCreateNodeAllocation(nodeName string) *NodeAllocation {
	c.lock.Lock()
	defer c.lock.Unlock()
	v := c.nodeAllocations[nodeName]
	if v == nil {
		v = NewNodeAllocation(nodeName)
		c.nodeAllocations[nodeName] = v
	}
	return v
}

func (c *resourceManager) GetTopologyHints(node *corev1.Node, pod *corev1.Pod, options *ResourceOptions) (map[string][]topologymanager.NUMATopologyHint, error) {
	topologyOptions := options.topologyOptions
	if len(topologyOptions.NUMANodeResources) == 0 {
		return nil, fmt.Errorf("insufficient resources on NUMA Node")
	}

	totalAvailable, _, err := c.getAvailableNUMANodeResources(node.Name, topologyOptions, options.reusableResources)
	if err != nil {
		return nil, err
	}

	nodes := make([]int, 0, len(topologyOptions.NUMANodeResources))
	for _, v := range topologyOptions.NUMANodeResources {
		nodes = append(nodes, v.Node)
	}
	result := generateResourceHints(nodes, options.requests, totalAvailable)
	hints := make(map[string][]topologymanager.NUMATopologyHint)
	for k, v := range result {
		hints[k] = v
	}
	return hints, nil
}

func (c *resourceManager) Allocate(node *corev1.Node, pod *corev1.Pod, options *ResourceOptions) (*PodAllocation, error) {
	allocation := &PodAllocation{
		UID:                pod.UID,
		Namespace:          pod.Namespace,
		Name:               pod.Name,
		CPUExclusivePolicy: options.cpuExclusivePolicy,
	}
	if options.hint.NUMANodeAffinity != nil {
		resources, err := c.allocateResourcesByHint(node, pod, options)
		if err != nil {
			return nil, err
		}
		allocation.NUMANodeResources = resources
	}
	if options.requestCPUBind {
		cpus, err := c.allocateCPUSet(node, pod, allocation.NUMANodeResources, options)
		if err != nil {
			return nil, err
		}
		allocation.CPUSet = cpus
	}
	return allocation, nil
}

func (c *resourceManager) allocateResourcesByHint(node *corev1.Node, pod *corev1.Pod, options *ResourceOptions) ([]NUMANodeResource, error) {
	if len(options.topologyOptions.NUMANodeResources) == 0 {
		return nil, fmt.Errorf("insufficient resources on NUMA Node")
	}

	totalAvailable, _, err := c.getAvailableNUMANodeResources(node.Name, options.topologyOptions, options.reusableResources)
	if err != nil {
		return nil, err
	}

	var requests corev1.ResourceList
	if options.requestCPUBind {
		requests = options.originalRequests.DeepCopy()
	} else {
		requests = options.requests.DeepCopy()
	}

	intersectionResources := sets.NewString()
	var result []NUMANodeResource
	for _, numaNodeID := range options.hint.NUMANodeAffinity.GetBits() {
		allocatable := totalAvailable[numaNodeID]
		r := NUMANodeResource{
			Node:      numaNodeID,
			Resources: corev1.ResourceList{},
		}
		for resourceName, quantity := range requests {
			if allocatableQuantity, ok := allocatable[resourceName]; ok {
				intersectionResources.Insert(string(resourceName))
				var allocated resource.Quantity
				allocatable[resourceName], requests[resourceName], allocated = allocateRes(allocatableQuantity, quantity)
				if !allocated.IsZero() {
					r.Resources[resourceName] = allocated
				}
			}
		}
		if !quotav1.IsZero(r.Resources) {
			result = append(result, r)
		}
		if quotav1.IsZero(requests) {
			break
		}
	}

	var reasons []string
	for resourceName, quantity := range requests {
		if intersectionResources.Has(string(resourceName)) {
			if !quantity.IsZero() {
				reasons = append(reasons, fmt.Sprintf("Insufficient NUMA %s", resourceName))
			}
		}
	}
	if len(reasons) > 0 {
		return nil, framework.NewStatus(framework.Unschedulable, reasons...).AsError()
	}
	return result, nil
}

func allocateRes(available, request resource.Quantity) (resource.Quantity, resource.Quantity, resource.Quantity) {
	switch available.Cmp(request) {
	case 1:
		available = available.DeepCopy()
		available.Sub(request)
		allocated := request.DeepCopy()
		request.Sub(request)
		return available, request, allocated
	case -1:
		request = request.DeepCopy()
		request.Sub(available)
		allocated := available.DeepCopy()
		available.Sub(available)
		return available, request, allocated
	default:
		request = request.DeepCopy()
		request.Sub(request)
		return request, request, available.DeepCopy()
	}
}

func (c *resourceManager) allocateCPUSet(node *corev1.Node, pod *corev1.Pod, allocatedNUMANodes []NUMANodeResource, options *ResourceOptions) (cpuset.CPUSet, error) {
	empty := cpuset.CPUSet{}
	availableCPUs, allocatedCPUs, err := c.GetAvailableCPUs(node.Name, options.preferredCPUs)
	if err != nil {
		return empty, err
	}

	topologyOptions := &options.topologyOptions
	if options.requiredCPUBindPolicy {
		cpuDetails := topologyOptions.CPUTopology.CPUDetails.KeepOnly(availableCPUs)
		availableCPUs = filterAvailableCPUsByRequiredCPUBindPolicy(options.cpuBindPolicy, availableCPUs, cpuDetails, topologyOptions.CPUTopology.CPUsPerCore())
	}

	if availableCPUs.Size() < options.numCPUsNeeded {
		return empty, fmt.Errorf("not enough cpus available to satisfy request")
	}

	result := cpuset.CPUSet{}
	numaAllocateStrategy := GetNUMAAllocateStrategy(node, c.numaAllocateStrategy)
	numCPUsNeeded := options.numCPUsNeeded
	if len(allocatedNUMANodes) > 0 {
		for _, numaNode := range allocatedNUMANodes {
			cpusInNUMANode := topologyOptions.CPUTopology.CPUDetails.CPUsInNUMANodes(numaNode.Node)
			availableCPUsInNUMANode := availableCPUs.Intersection(cpusInNUMANode)

			numCPUs := availableCPUsInNUMANode.Size()
			quantity := numaNode.Resources[corev1.ResourceCPU]
			nodeNumCPUsNeeded := int(quantity.MilliValue() / 1000)
			if nodeNumCPUsNeeded < numCPUs {
				numCPUs = nodeNumCPUsNeeded
			}

			cpus, err := takePreferredCPUs(
				topologyOptions.CPUTopology,
				topologyOptions.MaxRefCount,
				availableCPUsInNUMANode,
				options.preferredCPUs,
				allocatedCPUs,
				numCPUs,
				options.cpuBindPolicy,
				options.cpuExclusivePolicy,
				numaAllocateStrategy,
			)
			if err != nil {
				return empty, err
			}

			result = result.Union(cpus)
		}
		numCPUsNeeded -= result.Size()
		if numCPUsNeeded != 0 {
			return empty, fmt.Errorf("not enough cpus available to satisfy request")
		}
	}

	if numCPUsNeeded > 0 {
		availableCPUs = availableCPUs.Difference(result)
		remainingCPUs, err := takePreferredCPUs(
			topologyOptions.CPUTopology,
			topologyOptions.MaxRefCount,
			availableCPUs,
			options.preferredCPUs,
			allocatedCPUs,
			numCPUsNeeded,
			options.cpuBindPolicy,
			options.cpuExclusivePolicy,
			numaAllocateStrategy,
		)
		if err != nil {
			return empty, err
		}
		result = result.Union(remainingCPUs)
	}

	if options.requiredCPUBindPolicy {
		err = satisfiedRequiredCPUBindPolicy(options.cpuBindPolicy, result, topologyOptions.CPUTopology)
		if err != nil {
			return empty, err
		}
	}

	return result, err
}

func (c *resourceManager) Update(nodeName string, allocation *PodAllocation) {
	topologyOptions := c.topologyOptionsManager.GetTopologyOptions(nodeName)
	if topologyOptions.CPUTopology == nil || !topologyOptions.CPUTopology.IsValid() {
		return
	}

	nodeAllocation := c.getOrCreateNodeAllocation(nodeName)
	nodeAllocation.lock.Lock()
	defer nodeAllocation.lock.Unlock()

	nodeAllocation.update(allocation, topologyOptions.CPUTopology)
}

func (c *resourceManager) Release(nodeName string, podUID types.UID) {
	nodeAllocation := c.getOrCreateNodeAllocation(nodeName)
	nodeAllocation.lock.Lock()
	defer nodeAllocation.lock.Unlock()
	nodeAllocation.release(podUID)
}

func (c *resourceManager) GetAllocatedCPUSet(nodeName string, podUID types.UID) (cpuset.CPUSet, bool) {
	nodeAllocation := c.getOrCreateNodeAllocation(nodeName)
	nodeAllocation.lock.RLock()
	defer nodeAllocation.lock.RUnlock()

	return nodeAllocation.getCPUs(podUID)
}

func (c *resourceManager) GetAvailableCPUs(nodeName string, preferredCPUs cpuset.CPUSet) (availableCPUs cpuset.CPUSet, allocated CPUDetails, err error) {
	topologyOptions := c.topologyOptionsManager.GetTopologyOptions(nodeName)
	if topologyOptions.CPUTopology == nil {
		return cpuset.NewCPUSet(), nil, errors.New(ErrNotFoundCPUTopology)
	}
	if !topologyOptions.CPUTopology.IsValid() {
		return cpuset.NewCPUSet(), nil, errors.New(ErrInvalidCPUTopology)
	}

	allocation := c.getOrCreateNodeAllocation(nodeName)
	allocation.lock.RLock()
	defer allocation.lock.RUnlock()
	availableCPUs, allocated = allocation.getAvailableCPUs(topologyOptions.CPUTopology, topologyOptions.MaxRefCount, topologyOptions.ReservedCPUs, preferredCPUs)
	return availableCPUs, allocated, nil
}

func (c *resourceManager) GetNodeAllocation(nodeName string) *NodeAllocation {
	return c.getOrCreateNodeAllocation(nodeName)
}

func (c *resourceManager) getAvailableNUMANodeResources(nodeName string, topologyOptions TopologyOptions, reusableResources map[int]corev1.ResourceList) (totalAvailable, totalAllocated map[int]corev1.ResourceList, err error) {
	nodeAllocation := c.getOrCreateNodeAllocation(nodeName)
	nodeAllocation.lock.RLock()
	defer nodeAllocation.lock.RUnlock()
	totalAvailable, totalAllocated = nodeAllocation.getAvailableNUMANodeResources(topologyOptions, reusableResources)
	return totalAvailable, totalAllocated, nil
}

func generateResourceHints(numaNodes []int, podRequests corev1.ResourceList, totalAvailable map[int]corev1.ResourceList) map[string][]topologymanager.NUMATopologyHint {
	// Initialize minAffinitySize to include all NUMA Cells.
	minAffinitySize := len(numaNodes)

	hints := map[string][]topologymanager.NUMATopologyHint{}
	bitmask.IterateBitMasks(numaNodes, func(mask bitmask.BitMask) {
		maskBits := mask.GetBits()

		available := make(corev1.ResourceList)
		for _, nodeID := range maskBits {
			available = quotav1.Add(available, totalAvailable[nodeID])
		}
		if satisfied, _ := quotav1.LessThanOrEqual(podRequests, available); !satisfied {
			return
		}

		// set the minimum amount of NUMA nodes that can satisfy the resources requests
		if mask.Count() < minAffinitySize {
			minAffinitySize = mask.Count()
		}

		for resourceName := range podRequests {
			if _, ok := available[resourceName]; !ok {
				continue
			}
			if _, ok := hints[string(resourceName)]; !ok {
				hints[string(resourceName)] = []topologymanager.NUMATopologyHint{}
			}
			hints[string(resourceName)] = append(hints[string(resourceName)], topologymanager.NUMATopologyHint{
				NUMANodeAffinity: mask,
				Preferred:        false,
			})
		}
	})

	// update hints preferred according to multiNUMAGroups, in case when it wasn't provided, the default
	// behavior to prefer the minimal amount of NUMA nodes will be used
	for resourceName := range podRequests {
		for i, hint := range hints[string(resourceName)] {
			hints[string(resourceName)][i].Preferred = len(hint.NUMANodeAffinity.GetBits()) == minAffinitySize
		}
	}

	return hints
}

func filterAvailableCPUsByRequiredCPUBindPolicy(policy schedulingconfig.CPUBindPolicy, availableCPUs cpuset.CPUSet, cpuDetails CPUDetails, cpusPerCore int) cpuset.CPUSet {
	if policy == schedulingconfig.CPUBindPolicyFullPCPUs {
		cpuDetails.KeepOnly(availableCPUs)
		cpus := cpuDetails.CPUsInCores(cpuDetails.Cores().ToSliceNoSort()...)
		if cpus.Size()%cpusPerCore != 0 {
			return availableCPUs
		}
		return cpus
	}
	return availableCPUs
}

func satisfiedRequiredCPUBindPolicy(policy schedulingconfig.CPUBindPolicy, cpus cpuset.CPUSet, topology *CPUTopology) error {
	satisfied := true
	if policy == schedulingconfig.CPUBindPolicyFullPCPUs {
		satisfied = determineFullPCPUs(cpus, topology.CPUDetails, topology.CPUsPerCore())
	} else if policy == schedulingconfig.CPUBindPolicySpreadByPCPUs {
		satisfied = determineSpreadByPCPUs(cpus, topology.CPUDetails)
	}
	if !satisfied {
		return fmt.Errorf("insufficient CPUs to satisfy required cpu bind policy %s", policy)
	}
	return nil
}

func determineFullPCPUs(cpus cpuset.CPUSet, details CPUDetails, cpusPerCore int) bool {
	details = details.KeepOnly(cpus)
	return details.Cores().Size()*cpusPerCore == cpus.Size()
}

func determineSpreadByPCPUs(cpus cpuset.CPUSet, details CPUDetails) bool {
	details = details.KeepOnly(cpus)
	return details.Cores().Size() == cpus.Size()
}
