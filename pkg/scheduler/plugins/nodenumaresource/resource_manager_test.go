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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apiext "github.com/koordinator-sh/koordinator/apis/extension"
	schedulingconfig "github.com/koordinator-sh/koordinator/pkg/scheduler/apis/config"
	"github.com/koordinator-sh/koordinator/pkg/scheduler/frameworkext/topologymanager"
	"github.com/koordinator-sh/koordinator/pkg/util/bitmask"
	"github.com/koordinator-sh/koordinator/pkg/util/cpuset"
)

func TestResourceManagerAllocate(t *testing.T) {
	tests := []struct {
		name                string
		pod                 *corev1.Pod
		options             *ResourceOptions
		amplificationRatios map[corev1.ResourceName]apiext.Ratio
		allocated           *PodAllocation
		want                *PodAllocation
		wantErr             bool
	}{
		{
			name: "allocate with non-existing resources in NUMA",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:  4,
				requestCPUBind: false,
				requests: corev1.ResourceList{
					corev1.ResourceCPU:       resource.MustParse("4"),
					apiext.ResourceGPUMemory: resource.MustParse("10Gi"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			want: &PodAllocation{
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("4"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allocate with insufficient resources",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:  4,
				requestCPUBind: false,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("54"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "allocate with required CPUBindPolicyFullPCPUs",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicyFullPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			want: &PodAllocation{
				CPUSet: cpuset.MustParse("0-3"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("4"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allocate with required CPUBindPolicyFullPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicyFullPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("4-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want: &PodAllocation{
				CPUSet: cpuset.MustParse("0-3"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewQuantity(4, resource.DecimalSI),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "failed to allocate with required CPUBindPolicyFullPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicyFullPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("1,3,5,7-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "allocate with required CPUBindPolicySpreadByPCPUs",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			want: &PodAllocation{
				CPUSet: cpuset.MustParse("0,2,4,6"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("4"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allocate with required CPUBindPolicySpreadByPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("1,3,5,7-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want: &PodAllocation{
				CPUSet: cpuset.MustParse("0,2,4,6"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewQuantity(4, resource.DecimalSI),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "failed to allocate with required CPUBindPolicySpreadByPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("4-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "allocate with required CPUBindPolicySpreadByPCPUs and amplified requests",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("6"),
				},
				originalRequests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			amplificationRatios: map[corev1.ResourceName]apiext.Ratio{
				corev1.ResourceCPU: 1.5,
			},
			want: &PodAllocation{
				CPUSet: cpuset.MustParse("0,2,4,6"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("4"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allocate with required CPUBindPolicySpreadByPCPUs and allocated and amplified requests",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("6"),
				},
				originalRequests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			amplificationRatios: map[corev1.ResourceName]apiext.Ratio{
				corev1.ResourceCPU: 1.5,
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("1,3,5,7-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want: &PodAllocation{
				CPUSet: cpuset.MustParse("0,2,4,6"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("4"),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "failed to allocate with CPU Share and allocated and amplified ratios",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        false,
				requiredCPUBindPolicy: false,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				originalRequests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0)
						return mask
					}(),
				},
			},
			amplificationRatios: map[corev1.ResourceName]apiext.Ratio{
				corev1.ResourceCPU: 1.5,
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("0-49,52-101"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("50"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("50"),
						},
					},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "allocate by numa hint on mixed cpuset/share node",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         8,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicyFullPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("8"),
				},
				hint: topologymanager.NUMATopologyHint{
					NUMANodeAffinity: func() bitmask.BitMask {
						mask, _ := bitmask.NewBitMask(0, 1)
						return mask
					}(),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("0-43,53-96"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
				},
			},
			want: &PodAllocation{
				CPUSet: cpuset.MustParse("44-47,98-101"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewQuantity(4, resource.DecimalSI),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: *resource.NewQuantity(4, resource.DecimalSI),
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil, nil)
			tom := NewTopologyOptionsManager()
			tom.UpdateTopologyOptions("test-node", func(options *TopologyOptions) {
				options.CPUTopology = buildCPUTopologyForTest(2, 1, 26, 2)
				options.NUMANodeResources = []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("52"),
							corev1.ResourceMemory: resource.MustParse("128Gi"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("52"),
							corev1.ResourceMemory: resource.MustParse("128Gi"),
						},
					},
				}
			})
			tt.options.topologyOptions = tom.GetTopologyOptions("test-node")

			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("104"),
						corev1.ResourceMemory: resource.MustParse("256Gi"),
					},
				},
			}
			apiext.SetNodeResourceAmplificationRatios(node, tt.amplificationRatios)
			resourceManager := NewResourceManager(suit.Handle, schedulingconfig.NUMALeastAllocated, tom)
			if tt.allocated != nil {
				resourceManager.Update(node.Name, tt.allocated)
			}
			if tt.options.originalRequests == nil {
				tt.options.originalRequests = tt.options.requests.DeepCopy()
			}
			assert.NoError(t, amplifyNUMANodeResources(node, &tt.options.topologyOptions))

			got, err := resourceManager.Allocate(node, tt.pod, tt.options)
			if tt.wantErr != (err != nil) {
				t.Errorf("wantErr %v but got %v", tt.wantErr, err != nil)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResourceManagerGetTopologyHint(t *testing.T) {
	tests := []struct {
		name                string
		pod                 *corev1.Pod
		options             *ResourceOptions
		amplificationRatios map[corev1.ResourceName]apiext.Ratio
		allocated           *PodAllocation
		want                map[string][]topologymanager.NUMATopologyHint
		wantErr             bool
	}{
		{
			name: "allocate with required CPUBindPolicyFullPCPUs",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicyFullPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
			want: map[string][]topologymanager.NUMATopologyHint{
				string(corev1.ResourceCPU): {
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(1)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0, 1)
							return mask
						}(),
						Preferred: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allocate with required CPUBindPolicyFullPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicyFullPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("4-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want: map[string][]topologymanager.NUMATopologyHint{
				string(corev1.ResourceCPU): {
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0, 1)
							return mask
						}(),
						Preferred: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "failed to allocate with required CPUBindPolicyFullPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicyFullPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("1,3,5,7-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want: map[string][]topologymanager.NUMATopologyHint{
				string(corev1.ResourceCPU): {
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0, 1)
							return mask
						}(),
						Preferred: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allocate with required CPUBindPolicySpreadByPCPUs",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
			want: map[string][]topologymanager.NUMATopologyHint{
				string(corev1.ResourceCPU): {
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(1)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0, 1)
							return mask
						}(),
						Preferred: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "allocate with required CPUBindPolicySpreadByPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("1,3,5,7-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want: map[string][]topologymanager.NUMATopologyHint{
				string(corev1.ResourceCPU): {
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0, 1)
							return mask
						}(),
						Preferred: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "failed to allocate with required CPUBindPolicySpreadByPCPUs and allocated",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        true,
				requiredCPUBindPolicy: true,
				cpuBindPolicy:         schedulingconfig.CPUBindPolicySpreadByPCPUs,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("4-104"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("48"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("52"),
						},
					},
				},
			},
			want: map[string][]topologymanager.NUMATopologyHint{
				string(corev1.ResourceCPU): {
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0)
							return mask
						}(),
						Preferred: true,
					},
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0, 1)
							return mask
						}(),
						Preferred: false,
					},
				},
			},
			wantErr: false,
		},
		{
			name: "failed to allocate with CPU Share and allocated and amplified ratios",
			pod:  &corev1.Pod{},
			options: &ResourceOptions{
				numCPUsNeeded:         4,
				requestCPUBind:        false,
				requiredCPUBindPolicy: false,
				requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
				originalRequests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			},
			amplificationRatios: map[corev1.ResourceName]apiext.Ratio{
				corev1.ResourceCPU: 1.5,
			},
			allocated: &PodAllocation{
				UID:       "123456",
				Name:      "test-xxx",
				Namespace: "default",
				CPUSet:    cpuset.MustParse("0-49,52-101"),
				NUMANodeResources: []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("50"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("50"),
						},
					},
				},
			},
			want: map[string][]topologymanager.NUMATopologyHint{
				string(corev1.ResourceCPU): {
					{
						NUMANodeAffinity: func() bitmask.BitMask {
							mask, _ := bitmask.NewBitMask(0, 1)
							return mask
						}(),
						Preferred: true,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suit := newPluginTestSuit(t, nil, nil)
			tom := NewTopologyOptionsManager()
			tom.UpdateTopologyOptions("test-node", func(options *TopologyOptions) {
				options.CPUTopology = buildCPUTopologyForTest(2, 1, 26, 2)
				options.NUMANodeResources = []NUMANodeResource{
					{
						Node: 0,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("52"),
							corev1.ResourceMemory: resource.MustParse("128Gi"),
						},
					},
					{
						Node: 1,
						Resources: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("52"),
							corev1.ResourceMemory: resource.MustParse("128Gi"),
						},
					},
				}
			})
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-node",
				},
				Status: corev1.NodeStatus{
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("104"),
						corev1.ResourceMemory: resource.MustParse("256Gi"),
					},
				},
			}
			apiext.SetNodeResourceAmplificationRatios(node, tt.amplificationRatios)

			resourceManager := NewResourceManager(suit.Handle, schedulingconfig.NUMALeastAllocated, tom)
			if tt.allocated != nil {
				resourceManager.Update(node.Name, tt.allocated)
			}
			tt.options.topologyOptions = tom.GetTopologyOptions(node.Name)

			if tt.options.originalRequests == nil {
				tt.options.originalRequests = tt.options.requests.DeepCopy()
			}
			assert.NoError(t, amplifyNUMANodeResources(node, &tt.options.topologyOptions))

			got, err := resourceManager.GetTopologyHints(node, tt.pod, tt.options)
			if tt.wantErr != (err != nil) {
				t.Errorf("wantErr %v but got %v", tt.wantErr, err != nil)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}
