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
	"fmt"
	"testing"

	"ascend-common/devmanager"
	"ascend-common/devmanager/common"
	"ascend-common/devmanager/dcmi"

	"github.com/Project-HAMi/ascend-device-plugin/internal"
)

// slotFakeDevManager implements devmanager.DeviceInterface by embedding the
// interface and overriding only the methods UpdateDevice relies on.
type slotFakeDevManager struct {
	devmanager.DeviceInterface
	boardInfo common.BoardInfo
	boardErr  error
	pcieBus   string
	pcieErr   error
}

func (f *slotFakeDevManager) GetDeviceList() (int32, []int32, error) {
	return 2, []int32{0, 1}, nil
}

func (f *slotFakeDevManager) GetPhysicIDFromLogicID(logicID int32) (int32, error) {
	return logicID, nil
}

func (f *slotFakeDevManager) GetCardIDDeviceID(logicID int32) (int32, int32, error) {
	return logicID / 2, logicID % 2, nil
}

func (f *slotFakeDevManager) GetDieID(logicID int32, _ dcmi.DieType) (string, error) {
	return fmt.Sprintf("uuid-%d", logicID), nil
}

func (f *slotFakeDevManager) GetDeviceHealth(int32) (uint32, error) {
	return 0, nil
}

func (f *slotFakeDevManager) GetBoardInfo(int32) (common.BoardInfo, error) {
	return f.boardInfo, f.boardErr
}

func (f *slotFakeDevManager) GetPCIeBusInfo(int32) (string, error) {
	return f.pcieBus, f.pcieErr
}

// TestVDeviceCount verifies that VDeviceCount honors a per-node override
// (nodeConfig.VDeviceCount) and otherwise falls back to the global default.
func TestVDeviceCount(t *testing.T) {
	// A representative Ascend910B4 config: 32768 MiB allocatable, smallest
	// template 8192 MiB -> global default = 32768/8192 = 4 vDevices/card.
	cfg910B4 := internal.VNPUConfig{
		CommonWord:        "Ascend910B4",
		MemoryAllocatable: 32768,
		Templates: []internal.Template{
			{Name: "vir03_1c_8g", Memory: 8192, AICore: 5},
			{Name: "vir06_1c_16g", Memory: 16384, AICore: 10},
			{Name: "vir12_3c_32g", Memory: 32768, AICore: 20},
		},
	}

	tests := []struct {
		name       string
		config     internal.VNPUConfig
		nodeConfig *internal.NodeConfig
		want       int
	}{
		{
			name:       "no node config -> global default (32768/8192=4)",
			config:     cfg910B4,
			nodeConfig: nil,
			want:       4,
		},
		{
			name:       "node config VDeviceCount=0 -> falls back to global default",
			config:     cfg910B4,
			nodeConfig: &internal.NodeConfig{Name: "node-001", VDeviceCount: 0},
			want:       4,
		},
		{
			name:       "node config VDeviceCount=8 -> honored (the fix)",
			config:     cfg910B4,
			nodeConfig: &internal.NodeConfig{Name: "node-001", VDeviceCount: 8},
			want:       8,
		},
		{
			name:       "node config VDeviceCount=2 -> honored, tracks the value",
			config:     cfg910B4,
			nodeConfig: &internal.NodeConfig{Name: "node-001", VDeviceCount: 2},
			want:       2,
		},
		{
			name:       "no templates, no node config -> 1",
			config:     internal.VNPUConfig{CommonWord: "AscendX", MemoryAllocatable: 32768},
			nodeConfig: nil,
			want:       1,
		},
		{
			name:       "no templates but node override present -> node override wins",
			config:     internal.VNPUConfig{CommonWord: "AscendX", MemoryAllocatable: 32768},
			nodeConfig: &internal.NodeConfig{Name: "node-001", VDeviceCount: 5},
			want:       5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			am := &AscendManager{
				config:     tt.config,
				nodeConfig: tt.nodeConfig,
			}
			if got := am.VDeviceCount(); got != tt.want {
				t.Fatalf("VDeviceCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestUpdateDeviceSlotInfo verifies that UpdateDevice collects slot metadata
// from board/PCIe queries and degrades to zero values when those queries fail.
func TestUpdateDeviceSlotInfo(t *testing.T) {
	tests := []struct {
		name          string
		fake          *slotFakeDevManager
		wantErr       bool
		wantSlotID    uint32
		wantPCIeBus   string
		wantCardIDs   []int32
		wantDeviceIDs []int32
	}{
		{
			name: "collects slot info per device",
			fake: &slotFakeDevManager{
				boardInfo: common.BoardInfo{SlotId: 3},
				pcieBus:   "0000:C1:00.0",
			},
			wantErr:       false,
			wantSlotID:    3,
			wantPCIeBus:   "0000:C1:00.0",
			wantCardIDs:   []int32{0, 0},
			wantDeviceIDs: []int32{0, 1},
		},
		{
			name: "degrades to zero values when queries fail",
			fake: &slotFakeDevManager{
				boardErr: fmt.Errorf("board info unsupported"),
				pcieErr:  fmt.Errorf("pcie info unsupported"),
			},
			wantErr:       false,
			wantSlotID:    0,
			wantPCIeBus:   "",
			wantCardIDs:   []int32{0, 0},
			wantDeviceIDs: []int32{0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			am := &AscendManager{
				mgr:    tt.fake,
				config: internal.VNPUConfig{MemoryAllocatable: 43008, AICore: 8},
			}
			err := am.UpdateDevice()
			if (err != nil) != tt.wantErr {
				t.Fatalf("UpdateDevice() error = %v, wantErr %v", err, tt.wantErr)
			}
			devs := am.GetDevices()
			if len(devs) != 2 {
				t.Fatalf("expected 2 devices, got %d", len(devs))
			}
			for i, dev := range devs {
				if dev.SlotID != tt.wantSlotID {
					t.Fatalf("device[%d] SlotID = %d, want %d", i, dev.SlotID, tt.wantSlotID)
				}
				if dev.PCIeBus != tt.wantPCIeBus {
					t.Fatalf("device[%d] PCIeBus = %q, want %q", i, dev.PCIeBus, tt.wantPCIeBus)
				}
				if dev.CardID != tt.wantCardIDs[i] {
					t.Fatalf("device[%d] CardID = %d, want %d", i, dev.CardID, tt.wantCardIDs[i])
				}
				if dev.DeviceID != tt.wantDeviceIDs[i] {
					t.Fatalf("device[%d] DeviceID = %d, want %d", i, dev.DeviceID, tt.wantDeviceIDs[i])
				}
			}
		})
	}
}
