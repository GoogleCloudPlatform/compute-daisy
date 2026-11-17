//  Copyright 2026 Google Inc. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package daisy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

// bulkOp returns a successful bulk insert operation that created one VM in
// zone.
func bulkOp(zone string) *compute.Operation {
	return &compute.Operation{
		Name:   "op",
		Status: "DONE",
		InstancesBulkInsertOperationMetadata: &compute.InstancesBulkInsertOperationMetadata{
			PerLocationStatus: map[string]compute.BulkInsertOperationStatus{
				"zones/" + zone: {CreatedVmCount: 1, TargetVmCount: 1, Status: "DONE"},
			},
		},
	}
}

// bulkUnsupportedClient hides BulkInsertInstances.
type bulkUnsupportedClient struct {
	daisyCompute.Client
}

func TestIsLocalSSDDiskType(t *testing.T) {
	tests := []struct {
		diskType string
		want     bool
	}{
		{diskType: "local-ssd", want: true},
		{diskType: "projects/test-project/zones/test-zone/diskTypes/local-ssd", want: true},
		{diskType: "zones/test-zone/diskTypes/local-ssd", want: true},
		{diskType: "pd-balanced", want: false},
		{diskType: "projects/test-project/zones/test-zone/diskTypes/pd-standard", want: false},
		{diskType: "", want: false},
	}
	for _, tt := range tests {
		if got := isLocalSSDDiskType(tt.diskType); got != tt.want {
			t.Errorf("isLocalSSDDiskType(%q) = %v, want %v", tt.diskType, got, tt.want)
		}
	}
}

// The Instance to InstanceProperties conversion keeps every field and rejects
// fields InstanceProperties doesn't have.
func TestInstancePropertiesConversion(t *testing.T) {
	const (
		machineType = "n2-standard-2"
		diskType    = "pd-balanced"
		sourceImage = "projects/test-project/global/images/test-image"
	)
	sub := func(n string) string {
		return fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/regions/%s/subnetworks/%s", testProject, testRegion, n)
	}
	val := "v"
	in := &compute.Instance{
		Name:        "vm-name",
		Zone:        "some-zone",
		Description: "description",
		Labels:      map[string]string{"workflow": "abc"},
		MachineType: machineType,
		Metadata:    &compute.Metadata{Items: []*compute.MetadataItems{{Key: "k", Value: &val}}},
		Disks: []*compute.AttachedDisk{{
			Boot:       true,
			AutoDelete: true,
			DeviceName: "vm-name",
			Mode:       "READ_WRITE",
			InitializeParams: &compute.AttachedDiskInitializeParams{
				DiskName:    "vm-name",
				DiskType:    diskType,
				DiskSizeGb:  20,
				SourceImage: sourceImage,
			},
		}},
		NetworkInterfaces: []*compute.NetworkInterface{
			{Subnetwork: sub("s1"), NicType: "GVNIC", AccessConfigs: []*compute.AccessConfig{{Type: "ONE_TO_ONE_NAT"}}},
			{Subnetwork: sub("s2"), NicType: "GVNIC", StackType: "IPV4_IPV6"},
		},
		ServiceAccounts:         []*compute.ServiceAccount{{Email: "default", Scopes: []string{"https://www.googleapis.com/auth/cloud-platform"}}},
		Tags:                    &compute.Tags{Items: []string{"t"}},
		Scheduling:              &compute.Scheduling{OnHostMaintenance: "TERMINATE"},
		GuestAccelerators:       []*compute.AcceleratorConfig{{AcceleratorType: "nvidia-l4", AcceleratorCount: 1}},
		MinCpuPlatform:          "Intel Ice Lake",
		AdvancedMachineFeatures: &compute.AdvancedMachineFeatures{ThreadsPerCore: 1},
	}
	props, err := instanceProperties(in)
	if err != nil {
		t.Fatalf("instanceProperties: %v", err)
	}
	if in.Name != "vm-name" || in.Zone != "some-zone" {
		t.Errorf("instanceProperties modified its input: Name=%q Zone=%q", in.Name, in.Zone)
	}

	// Every field except Name, Zone and the disks' InitializeParams.DiskName
	// round-trips unchanged.
	if in.Disks[0].InitializeParams.DiskName != "vm-name" {
		t.Errorf("instanceProperties modified its input: DiskName=%q", in.Disks[0].InitializeParams.DiskName)
	}
	if got := props.Disks[0].InitializeParams.DiskName; got != "" {
		t.Errorf("InitializeParams.DiskName = %q, want empty (bulk insert rejects disk names)", got)
	}
	want := *in
	want.Name, want.Zone = "", ""
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	gotJSON, err := json.Marshal(props)
	if err != nil {
		t.Fatal(err)
	}
	var wantM, gotM map[string]any
	if err := json.Unmarshal(wantJSON, &wantM); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(gotJSON, &gotM); err != nil {
		t.Fatal(err)
	}
	delete(wantM["disks"].([]any)[0].(map[string]any)["initializeParams"].(map[string]any), "diskName")
	if d := diff(wantM, gotM, 0); d != "" {
		t.Errorf("instanceProperties mismatch (-want +got):\n%s", d)
	}
	if props.Description != "description" || props.Labels["workflow"] != "abc" {
		t.Errorf("Description/Labels not carried over: %q, %v", props.Description, props.Labels)
	}

	// A field InstanceProperties doesn't have is an error naming it.
	bad := *in
	bad.Hostname = "host.example.com"
	if _, err := instanceProperties(&bad); err == nil || !strings.Contains(err.Error(), "hostname") {
		t.Errorf("instanceProperties with Hostname: error = %v, want one naming \"hostname\"", err)
	}
}

func TestBulkInsertZone(t *testing.T) {
	const chosenZone = "test-region-z3"
	status := func(m map[string]int64) *compute.Operation {
		op := &compute.Operation{
			Name: "op",
			InstancesBulkInsertOperationMetadata: &compute.InstancesBulkInsertOperationMetadata{
				PerLocationStatus: map[string]compute.BulkInsertOperationStatus{},
			},
		}
		for k, v := range m {
			op.InstancesBulkInsertOperationMetadata.PerLocationStatus[k] = compute.BulkInsertOperationStatus{CreatedVmCount: v}
		}
		return op
	}

	opWithZeroAndOne := status(map[string]int64{
		"zones/test-region-z1": 0,
		"zones/" + chosenZone:  1,
	})
	if got, err := bulkInsertZone(opWithZeroAndOne); err != nil || got != chosenZone {
		t.Fatalf("bulkInsertZone(valid) = %q, %v; want %q, nil", got, err, chosenZone)
	}

	opWithErr := bulkOp(chosenZone)
	opWithErr.Error = &compute.OperationError{Errors: []*compute.OperationErrorErrors{{Code: "ZONE_RESOURCE_POOL_EXHAUSTED", Message: "out of capacity"}}}

	tests := []struct {
		name    string
		op      *compute.Operation
		wantErr string
	}{
		{name: "nil_op", op: nil, wantErr: "no operation"},
		{name: "op_error", op: opWithErr, wantErr: "ZONE_RESOURCE_POOL_EXHAUSTED"},
		{name: "no_metadata", op: &compute.Operation{Name: "op"}, wantErr: "no instancesBulkInsertOperationMetadata"},
		{name: "no_created_vm", op: status(map[string]int64{"zones/" + chosenZone: 0}), wantErr: "want exactly one location"},
		{name: "empty_status", op: status(nil), wantErr: "want exactly one location"},
		{name: "two_zones", op: status(map[string]int64{"zones/a": 1, "zones/b": 1}), wantErr: "want exactly one location"},
		{name: "two_vms_in_one_zone", op: status(map[string]int64{"zones/a": 2}), wantErr: "want 1 created VM"},
		{name: "missing_zones_prefix", op: status(map[string]int64{chosenZone: 1}), wantErr: "unexpected location"},
		{name: "regions_key", op: status(map[string]int64{"regions/" + testRegion: 1}), wantErr: "unexpected location"},
		{name: "invalid_zone_name", op: status(map[string]int64{"zones/BAD_ZONE": 1}), wantErr: "unexpected location"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone, err := bulkInsertZone(tt.op)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("bulkInsertZone() = %q, %v; want error containing %q", zone, err, tt.wantErr)
			}
		})
	}
}

func TestBulkCreate(t *testing.T) {
	const chosenZone = "test-region-z3"
	newInst := func() *Instance {
		return &Instance{
			InstanceBase: InstanceBase{Resource: Resource{daisyName: "vm", RealName: "vm-real", Project: testProject}},
			Instance: compute.Instance{
				Name:        "vm-real",
				MachineType: "n2-standard-2",
				Description: "test description",
				Labels:      map[string]string{"daisy-test": "true"},
				Disks: []*compute.AttachedDisk{
					{Boot: true, AutoDelete: true, DeviceName: "vm-real", InitializeParams: &compute.AttachedDiskInitializeParams{DiskName: "vm-real", DiskType: "pd-balanced"}},
					{DeviceName: "data", InitializeParams: &compute.AttachedDiskInitializeParams{DiskName: "vm-data", DiskType: "pd-balanced"}},
					{DeviceName: "local-ssd-0", Type: "SCRATCH", InitializeParams: &compute.AttachedDiskInitializeParams{DiskType: "local-ssd"}},
				},
				NetworkInterfaces: []*compute.NetworkInterface{
					{Network: "projects/test-project/global/networks/test-network", AccessConfigs: []*compute.AccessConfig{{Type: "ONE_TO_ONE_NAT"}}},
				},
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		var gotProject, gotRegion, gotGetZone string
		var gotReq *compute.BulkInsertInstanceResource
		tc := &daisyCompute.TestClient{
			BulkInsertInstancesFn: func(p, r string, req *compute.BulkInsertInstanceResource) (*compute.Operation, error) {
				gotProject, gotRegion, gotReq = p, r, req
				return bulkOp(chosenZone), nil
			},
			GetInstanceFn: func(p, z, name string) (*compute.Instance, error) {
				gotGetZone = z
				return &compute.Instance{
					Name: name,
					Zone: "https://www.googleapis.com/compute/v1/projects/" + p + "/zones/" + z,
					Disks: []*compute.AttachedDisk{
						{DeviceName: name, Source: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", p, z, name)},
						{DeviceName: "data", Source: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s-data", p, z, name)},
						{DeviceName: "local-ssd-0", Type: "SCRATCH"},
					},
				}, nil
			},
		}

		i := newInst()
		if err := i.bulkCreate(tc, testRegion); err != nil {
			t.Fatalf("bulkCreate: %v", err)
		}
		if gotProject != testProject || gotRegion != testRegion {
			t.Errorf("BulkInsertInstances project/region = %q/%q, want %q/%q", gotProject, gotRegion, testProject, testRegion)
		}
		if gotReq.Count != 1 || gotReq.MinCount != 1 {
			t.Errorf("Count, MinCount = %d, %d; want 1, 1", gotReq.Count, gotReq.MinCount)
		}
		if _, ok := gotReq.PerInstanceProperties["vm-real"]; !ok || len(gotReq.PerInstanceProperties) != 1 {
			t.Errorf("PerInstanceProperties = %v, want only key %q", gotReq.PerInstanceProperties, "vm-real")
		}
		if gotReq.LocationPolicy == nil || gotReq.LocationPolicy.TargetShape != "ANY_SINGLE_ZONE" || len(gotReq.LocationPolicy.Locations) != 0 {
			t.Errorf("LocationPolicy = %+v, want TargetShape ANY_SINGLE_ZONE with no Locations", gotReq.LocationPolicy)
		}
		if gotReq.NamePattern != "" || gotReq.SourceInstanceTemplate != "" {
			t.Errorf("NamePattern/SourceInstanceTemplate = %q/%q, want empty", gotReq.NamePattern, gotReq.SourceInstanceTemplate)
		}
		for idx, d := range gotReq.InstanceProperties.Disks {
			if d.InitializeParams != nil && d.InitializeParams.DiskName != "" {
				t.Errorf("InstanceProperties.Disks[%d].InitializeParams.DiskName = %q, want empty", idx, d.InitializeParams.DiskName)
			}
		}
		wantDiskNames := map[string]string{"vm-real": "vm-real", "data": "vm-data"}
		if !reflect.DeepEqual(i.bulkDiskNames, wantDiskNames) {
			t.Errorf("bulkDiskNames = %v, want %v", i.bulkDiskNames, wantDiskNames)
		}
		if gotGetZone != chosenZone || i.Zone != chosenZone {
			t.Errorf("GetInstance zone / i.Zone = %q / %q, want %q", gotGetZone, i.Zone, chosenZone)
		}
		if len(i.Disks) != 3 || i.Disks[0].InitializeParams != nil {
			t.Errorf("Instance not refreshed from GetInstance: Disks = %+v", i.Disks)
		}
	})

	t.Run("rebuilds_request_on_retry", func(t *testing.T) {
		var requests []*compute.BulkInsertInstanceResource
		tc := &daisyCompute.TestClient{
			BulkInsertInstancesFn: func(_, _ string, req *compute.BulkInsertInstanceResource) (*compute.Operation, error) {
				requests = append(requests, req)
				if len(requests) == 1 {
					return nil, &googleapi.Error{Code: http.StatusPreconditionFailed}
				}
				return bulkOp(chosenZone), nil
			},
			GetInstanceFn: func(_, _, name string) (*compute.Instance, error) {
				return &compute.Instance{Name: name}, nil
			},
		}
		i := newInst()
		if err := i.bulkCreate(tc, testRegion); err == nil {
			t.Fatal("first bulkCreate succeeded, want error")
		}
		i.NetworkInterfaces[0].AccessConfigs = nil
		if err := i.bulkCreate(tc, testRegion); err != nil {
			t.Fatalf("second bulkCreate: %v", err)
		}
		if len(requests) != 2 {
			t.Fatalf("attempts = %d, want 2", len(requests))
		}
		if len(requests[0].InstanceProperties.NetworkInterfaces[0].AccessConfigs) != 1 {
			t.Errorf("first request AccessConfigs = %+v, want 1", requests[0].InstanceProperties.NetworkInterfaces[0].AccessConfigs)
		}
		if len(requests[1].InstanceProperties.NetworkInterfaces[0].AccessConfigs) != 0 {
			t.Errorf("second request AccessConfigs = %+v, want none", requests[1].InstanceProperties.NetworkInterfaces[0].AccessConfigs)
		}
	})

	t.Run("unsupported_client", func(t *testing.T) {
		i := newInst()
		err := i.bulkCreate(bulkUnsupportedClient{&daisyCompute.TestClient{}}, testRegion)
		if err == nil || !strings.Contains(err.Error(), "does not support regional bulk insert") {
			t.Errorf("bulkCreate error = %v, want 'does not support regional bulk insert'", err)
		}
	})

	t.Run("api_error_leaves_zone_empty", func(t *testing.T) {
		tc := &daisyCompute.TestClient{
			BulkInsertInstancesFn: func(_, _ string, _ *compute.BulkInsertInstanceResource) (*compute.Operation, error) {
				return nil, errors.New("boom")
			},
		}
		i := newInst()
		if err := i.bulkCreate(tc, testRegion); err == nil {
			t.Fatal("bulkCreate succeeded, want error")
		}
		if i.Zone != "" {
			t.Errorf("Zone = %q, want empty", i.Zone)
		}
	})

	t.Run("refresh_error_leaves_zone_empty", func(t *testing.T) {
		tc := &daisyCompute.TestClient{
			BulkInsertInstancesFn: func(_, _ string, _ *compute.BulkInsertInstanceResource) (*compute.Operation, error) {
				return bulkOp(chosenZone), nil
			},
			GetInstanceFn: func(_, _, _ string) (*compute.Instance, error) {
				return nil, errors.New("refresh failed")
			},
		}
		i := newInst()
		if err := i.bulkCreate(tc, testRegion); err == nil {
			t.Fatal("bulkCreate succeeded, want error")
		}
		if i.Zone != "" {
			t.Errorf("Zone = %q, want empty", i.Zone)
		}
	})
}

func TestFinalizeLocation(t *testing.T) {
	const chosenZone = "test-region-z3"
	bootSrc := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/vm-real", testProject, chosenZone)
	dataSrc := fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/vm-data-gce", testProject, chosenZone)

	setup := func() (*Workflow, *Instance, *Resource, *Resource) {
		w := testWorkflow()
		bootRes := &Resource{RealName: "vm-real"}
		dataRes := &Resource{RealName: "vm-data"}
		w.disks.m["vm-real"] = bootRes
		w.disks.m["vm-data"] = dataRes
		i := &Instance{
			InstanceBase: InstanceBase{Resource: Resource{daisyName: "vm", RealName: "vm-real", Project: testProject}},
			Instance: compute.Instance{
				Name: "vm-real",
				Zone: chosenZone,
				Disks: []*compute.AttachedDisk{
					{DeviceName: "vm-real", Source: bootSrc},
					{DeviceName: "data", Source: dataSrc},
					{DeviceName: "local-ssd-0", Type: "SCRATCH"},
				},
			},
			bulkDiskNames: map[string]string{
				"vm-real": "vm-real",
				"data":    "vm-data",
			},
		}
		return w, i, bootRes, dataRes
	}

	t.Run("success", func(t *testing.T) {
		w, i, bootRes, dataRes := setup()
		if err := i.finalizeLocation(w); err != nil {
			t.Fatalf("finalizeLocation: %v", err)
		}
		if want := fmt.Sprintf("projects/%s/zones/%s/instances/vm-real", testProject, chosenZone); i.link != want {
			t.Errorf("instance link = %q, want %q", i.link, want)
		}
		if want := fmt.Sprintf("projects/%s/zones/%s/disks/vm-real", testProject, chosenZone); bootRes.link != want {
			t.Errorf("boot disk link = %q, want %q", bootRes.link, want)
		}
		if want := fmt.Sprintf("projects/%s/zones/%s/disks/vm-data-gce", testProject, chosenZone); dataRes.link != want {
			t.Errorf("data disk link = %q, want %q", dataRes.link, want)
		}
	})

	errTests := []struct {
		name    string
		mutate  func(w *Workflow, i *Instance)
		wantErr string
	}{
		{
			name:    "missing_device",
			mutate:  func(_ *Workflow, i *Instance) { i.Disks = i.Disks[:1] },
			wantErr: "no disk with device name",
		},
		{
			name:    "empty_source",
			mutate:  func(_ *Workflow, i *Instance) { i.Disks[1].Source = "" },
			wantErr: "no disk with device name",
		},
		{
			name:    "malformed_source",
			mutate:  func(_ *Workflow, i *Instance) { i.Disks[1].Source = "garbage" },
			wantErr: "unexpected source",
		},
		{
			name:    "missing_registry_entry",
			mutate:  func(w *Workflow, _ *Instance) { delete(w.disks.m, "vm-data") },
			wantErr: "not found in registry",
		},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			w, i, bootRes, dataRes := setup()
			tt.mutate(w, i)
			err := i.finalizeLocation(w)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("finalizeLocation error = %v, want one containing %q", err, tt.wantErr)
			}
			// Atomic on error: neither instance link nor any disk link is modified.
			if i.link != "" || bootRes.link != "" || dataRes.link != "" {
				t.Errorf("links mutated on error: instance=%q boot=%q data=%q", i.link, bootRes.link, dataRes.link)
			}
		})
	}
}
