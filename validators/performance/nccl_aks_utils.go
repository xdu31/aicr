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
	"context"
	"fmt"
	"log/slog"

	"github.com/NVIDIA/aicr/pkg/defaults"
	aicrErrors "github.com/NVIDIA/aicr/pkg/errors"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ncclTopoConfigMapName is the name used for the NCCL topology ConfigMap
// created by the AKS NCCL validator. Cleaned up in cleanupNCCLResources.
const ncclTopoConfigMapName = "nccl-all-reduce-topo"

// mlnxNICResource is the Kubernetes extended resource name for Mellanox
// InfiniBand NICs exposed by the NVIDIA Network Operator device plugin.
const mlnxNICResource = v1.ResourceName("nvidia.com/mlnxnics")

// discoverAKSNodeConfig reads the Mellanox NIC count from a GPU node's
// allocatable resources. A count of 0 is valid — the Network Operator may
// not be deployed, but NCCL still uses IB via OFED kernel drivers.
func discoverAKSNodeConfig(node v1.Node) int {
	quantity := node.Status.Allocatable[mlnxNICResource]
	return int(quantity.Value())
}

// buildMLNXResourceLine returns the YAML line for nvidia.com/mlnxnics
// resource requests/limits at the correct indentation, or an empty string
// if count is 0 (same graceful-degradation pattern as buildEFAResourceLine).
func buildMLNXResourceLine(count int, indent string) string {
	if count == 0 {
		return ""
	}
	return fmt.Sprintf("%snvidia.com/mlnxnics: \"%d\"", indent, count)
}

// ndv5TopoXML is the NCCL topology XML for Azure ND H100 v5 / ND H200 v5
// VMs. Describes the PCIe Gen5 topology: 2 NUMA nodes (Intel Sapphire
// Rapids), 4 GPU+NIC pairs per NUMA. Each GPU (class 0x030200) is paired
// with a ConnectX-7 NIC (class 0x020700) under a PCIe bridge at 32 GT/s x16.
//
// Source: excalibur (azure-h100.xml) and nccl-doctor (ndv5-topo.xml) —
// both describe the identical ND H100 v5 hardware topology.
const ndv5TopoXML = `<system version="1">
  <cpu numaid="0" affinity="ffffffff,ffff0000,00000000" arch="x86_64" vendor="GenuineIntel" familyid="6" modelid="143">
    <pci busid="ffff:ff:01.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="0001:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0101:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
    <pci busid="ffff:ff:02.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="0002:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0102:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
    <pci busid="ffff:ff:03.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="0003:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0103:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
    <pci busid="ffff:ff:04.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="0008:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0104:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
  </cpu>
  <cpu numaid="1" affinity="00000000,0000ffff,ffffffff" arch="x86_64" vendor="GenuineIntel" familyid="6" modelid="143">
    <pci busid="ffff:ff:05.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="0009:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0105:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
    <pci busid="ffff:ff:06.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="000a:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0106:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
    <pci busid="ffff:ff:07.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="000b:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0107:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
    <pci busid="ffff:ff:08.0" class="0x060400" link_speed="32.0 GT/s PCIe" link_width="16">
      <pci busid="000c:00:00.0" class="0x030200" link_speed="32.0 GT/s PCIe" link_width="16"/>
      <pci busid="0108:00:00.0" class="0x020700" link_speed="32.0 GT/s PCIe" link_width="16"/>
    </pci>
  </cpu>
</system>`

// createTopoConfigMap creates a ConfigMap containing the ND H100 v5 NCCL
// topology XML. The ConfigMap is mounted into worker pods so NCCL reads
// the topology at /etc/nccl/topo.xml instead of auto-discovering it.
// Uses create-or-update semantics per CLAUDE.md Kubernetes patterns.
func createTopoConfigMap(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	slog.Info("Creating NCCL topology ConfigMap", "name", ncclTopoConfigMapName, "namespace", namespace)

	createCtx, cancel := context.WithTimeout(ctx, defaults.DiagnosticTimeout)
	defer cancel()

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ncclTopoConfigMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			"topo.xml": ndv5TopoXML,
		},
	}

	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(createCtx, cm, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return aicrErrors.Wrap(aicrErrors.ErrCodeInternal, "failed to create NCCL topology ConfigMap", err)
	}

	// AlreadyExists: update in place (prior run may have left a stale CM).
	_, err = clientset.CoreV1().ConfigMaps(namespace).Update(createCtx, cm, metav1.UpdateOptions{})
	if err != nil {
		return aicrErrors.Wrap(aicrErrors.ErrCodeInternal, "failed to update NCCL topology ConfigMap", err)
	}
	slog.Info("Updated existing NCCL topology ConfigMap", "name", ncclTopoConfigMapName)
	return nil
}

// deleteTopoConfigMap removes the NCCL topology ConfigMap. NotFound is
// expected for non-AKS platforms and is logged at debug.
func deleteTopoConfigMap(clientset kubernetes.Interface, namespace string) {
	deleteCtx, cancel := context.WithTimeout(context.Background(), defaults.DiagnosticTimeout)
	defer cancel()

	err := clientset.CoreV1().ConfigMaps(namespace).Delete(deleteCtx, ncclTopoConfigMapName, metav1.DeleteOptions{})
	switch {
	case err == nil:
		slog.Info("Deleted NCCL topology ConfigMap", "name", ncclTopoConfigMapName)
	case apierrors.IsNotFound(err):
		slog.Debug("NCCL topology ConfigMap not present (non-AKS platform), skipping", "name", ncclTopoConfigMapName)
	default:
		slog.Error("Warning: Failed to delete NCCL topology ConfigMap", "error", err, "name", ncclTopoConfigMapName)
	}
}
