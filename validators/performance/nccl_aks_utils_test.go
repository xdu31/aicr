// Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/xml"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDiscoverAKSNodeConfig(t *testing.T) {
	tests := []struct {
		name     string
		node     v1.Node
		wantMLNX int
	}{
		{
			name: "ND H100 v5 with 8 Mellanox NICs",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nvidia.com/gpu.product": "NVIDIA-H100-80GB-HBM3",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceName("nvidia.com/gpu"):      resource.MustParse("8"),
						v1.ResourceName("nvidia.com/mlnxnics"): resource.MustParse("8"),
					},
				},
			},
			wantMLNX: 8,
		},
		{
			name: "no mlnxnics (Network Operator not deployed)",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nvidia.com/gpu.product": "NVIDIA-H100-80GB-HBM3",
					},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{
						v1.ResourceName("nvidia.com/gpu"): resource.MustParse("8"),
					},
				},
			},
			wantMLNX: 0,
		},
		{
			name: "empty allocatable",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{},
				},
				Status: v1.NodeStatus{
					Allocatable: v1.ResourceList{},
				},
			},
			wantMLNX: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := discoverAKSNodeConfig(tt.node)
			if got != tt.wantMLNX {
				t.Errorf("discoverAKSNodeConfig() = %d, want %d", got, tt.wantMLNX)
			}
		})
	}
}

func TestBuildMLNXResourceLine(t *testing.T) {
	tests := []struct {
		name   string
		count  int
		indent string
		want   string
	}{
		{
			name:   "8 Mellanox NICs",
			count:  8,
			indent: "                      ",
			want:   `                      nvidia.com/mlnxnics: "8"`,
		},
		{
			name:   "4 Mellanox NICs",
			count:  4,
			indent: "                      ",
			want:   `                      nvidia.com/mlnxnics: "4"`,
		},
		{
			name:   "no NICs — empty string",
			count:  0,
			indent: "                      ",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMLNXResourceLine(tt.count, tt.indent)
			if got != tt.want {
				t.Errorf("buildMLNXResourceLine(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}

// xmlSystem is a minimal struct for validating the NCCL topology XML constant.
type xmlSystem struct {
	XMLName xml.Name `xml:"system"`
	Version string   `xml:"version,attr"`
	CPUs    []xmlCPU `xml:"cpu"`
}

type xmlCPU struct {
	NumaID string   `xml:"numaid,attr"`
	PCIs   []xmlPCI `xml:"pci"`
}

type xmlPCI struct {
	BusID    string   `xml:"busid,attr"`
	Class    string   `xml:"class,attr"`
	Children []xmlPCI `xml:"pci"`
}

func TestNdv5TopoXML(t *testing.T) {
	var sys xmlSystem
	if err := xml.Unmarshal([]byte(ndv5TopoXML), &sys); err != nil {
		t.Fatalf("ndv5TopoXML is not valid XML: %v", err)
	}

	if sys.Version != "1" {
		t.Errorf("system version = %q, want %q", sys.Version, "1")
	}

	if len(sys.CPUs) != 2 {
		t.Fatalf("expected 2 NUMA CPUs, got %d", len(sys.CPUs))
	}

	// Each NUMA node has 4 PCIe bridges, each with 1 GPU + 1 NIC = 8 GPUs total.
	totalGPUs := 0
	totalNICs := 0
	for _, cpu := range sys.CPUs {
		if len(cpu.PCIs) != 4 {
			t.Errorf("NUMA %s: expected 4 PCIe bridges, got %d", cpu.NumaID, len(cpu.PCIs))
		}
		for _, bridge := range cpu.PCIs {
			// Each bridge should be class 0x060400 (PCI-to-PCI bridge)
			if bridge.Class != "0x060400" {
				t.Errorf("bridge %s: class = %q, want 0x060400", bridge.BusID, bridge.Class)
			}
			for _, child := range bridge.Children {
				switch {
				case strings.HasPrefix(child.Class, "0x0302"):
					totalGPUs++
				case strings.HasPrefix(child.Class, "0x0207"):
					totalNICs++
				}
			}
		}
	}

	if totalGPUs != 8 {
		t.Errorf("expected 8 GPUs in topology, got %d", totalGPUs)
	}
	if totalNICs != 8 {
		t.Errorf("expected 8 NICs in topology, got %d", totalNICs)
	}
}
