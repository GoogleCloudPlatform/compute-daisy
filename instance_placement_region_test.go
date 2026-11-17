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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const (
	prChosenZone   = "test-region-z3"
	prMachineType  = "n2-standard-2"
	prDiskType     = "pd-balanced"
	prSourceImage  = "projects/test-project/global/images/test-image"
	prNetwork      = "global/networks/test-network"
	prInstanceName = "vm"
)

// prCalls records the compute API calls made by the fakes in a
// PlacementRegion test.
type prCalls struct {
	mu          sync.Mutex
	bulk        []*compute.BulkInsertInstanceResource
	bulkRegions []string
	zonalCreate []string
	get         []string
	serial      []string
	del         []string
	zoneScoped  []string
}

func (c *prCalls) add(list *[]string, v string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	*list = append(*list, v)
}

func (c *prCalls) snapshot(list *[]string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), *list...)
}

// placementRegionWorkflow returns a test workflow whose fakes know testRegion,
// fail any zone-scoped existence lookup, and simulate a bulk insert that
// lands in prChosenZone.
func placementRegionWorkflow(t *testing.T) (*Workflow, *daisyCompute.TestClient, *prCalls) {
	t.Helper()
	w := testWorkflow()
	base := w.ComputeClient.(*daisyCompute.TestClient)
	// TestClient has no ListRegions override, so serve regions over HTTP and
	// fail any other request that reaches the real client.
	ts, tc, err := daisyCompute.NewTestClient(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/regions") {
			fmt.Fprintf(rw, `{"items":[{"name":%q}]}`, testRegion)
			return
		}
		http.Error(rw, "unexpected request: "+r.URL.String(), http.StatusBadRequest)
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ts.Close)
	tc.GetProjectFn = base.GetProjectFn
	tc.GetZoneFn = base.GetZoneFn
	tc.ListZonesFn = base.ListZonesFn
	tc.ListImagesFn = base.ListImagesFn
	tc.ListNetworksFn = base.ListNetworksFn
	w.ComputeClient = tc
	calls := &prCalls{}

	tc.ListMachineTypesFn = func(p, z string, _ ...daisyCompute.ListCallOption) ([]*compute.MachineType, error) {
		calls.add(&calls.zoneScoped, "ListMachineTypes "+z)
		return nil, errors.New("unexpected zone-scoped lookup")
	}
	tc.GetMachineTypeFn = func(_, z, _ string) (*compute.MachineType, error) {
		calls.add(&calls.zoneScoped, "GetMachineType "+z)
		return nil, errors.New("unexpected zone-scoped lookup")
	}
	tc.ListDisksFn = func(_, z string, _ ...daisyCompute.ListCallOption) ([]*compute.Disk, error) {
		calls.add(&calls.zoneScoped, "ListDisks "+z)
		return nil, errors.New("unexpected zone-scoped lookup")
	}
	tc.ListInstancesFn = func(_, z string, _ ...daisyCompute.ListCallOption) ([]*compute.Instance, error) {
		calls.add(&calls.zoneScoped, "ListInstances "+z)
		return nil, errors.New("unexpected zone-scoped lookup")
	}
	tc.CreateInstanceFn = func(_, z string, _ *compute.Instance) error {
		calls.add(&calls.zonalCreate, z)
		return nil
	}
	tc.BulkInsertInstancesFn = func(_, region string, r *compute.BulkInsertInstanceResource) (*compute.Operation, error) {
		calls.mu.Lock()
		calls.bulk = append(calls.bulk, r)
		calls.bulkRegions = append(calls.bulkRegions, region)
		calls.mu.Unlock()
		return bulkOp(prChosenZone), nil
	}
	tc.GetInstanceFn = func(p, z, name string) (*compute.Instance, error) {
		calls.add(&calls.get, z)
		return &compute.Instance{
			Name: name,
			Zone: "https://www.googleapis.com/compute/v1/projects/" + p + "/zones/" + z,
			Disks: []*compute.AttachedDisk{
				{DeviceName: name, Source: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", p, z, name)},
				{DeviceName: "data", Source: fmt.Sprintf("https://www.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s-data", p, z, name)},
				{DeviceName: "local-ssd-0", Type: "SCRATCH"},
			},
		}, nil
	}
	tc.GetSerialPortOutputFn = func(_, z, _ string, _, _ int64) (*compute.SerialPortOutput, error) {
		calls.add(&calls.serial, z)
		return nil, errors.New("no serial output")
	}
	// A stopped instance ends serial-port logging after its first failed poll,
	// so that no logging goroutine outlives the test.
	tc.InstanceStatusFn = func(_, _, _ string) (string, error) {
		return "TERMINATED", nil
	}
	tc.InstanceStoppedFn = func(_, _, _ string) (bool, error) {
		return true, nil
	}
	tc.DeleteInstanceFn = func(_, z, _ string) error {
		calls.add(&calls.del, z)
		return nil
	}
	return w, tc, calls
}

// regionalInstance returns a networkconfig-shaped PlacementRegion instance:
// short machine and disk types, an InitializeParams boot disk, a second
// persistent disk that isn't auto-deleted, and a local SSD.
func regionalInstance(name string) *Instance {
	return &Instance{
		InstanceBase: InstanceBase{PlacementRegion: testRegion},
		Instance: compute.Instance{
			Name:        name,
			MachineType: prMachineType,
			Description: "test description",
			Labels:      map[string]string{"daisy-test": "true"},
			Disks: []*compute.AttachedDisk{
				{AutoDelete: true, InitializeParams: &compute.AttachedDiskInitializeParams{SourceImage: prSourceImage, DiskType: prDiskType, DiskSizeGb: 20}},
				{DeviceName: "data", InitializeParams: &compute.AttachedDiskInitializeParams{DiskName: name + "-data", SourceImage: prSourceImage, DiskType: prDiskType}},
				{DeviceName: "local-ssd-0", InitializeParams: &compute.AttachedDiskInitializeParams{DiskType: "local-ssd"}},
			},
			NetworkInterfaces: []*compute.NetworkInterface{{Network: prNetwork}},
		},
	}
}

func mustNewStep(t *testing.T, w *Workflow, name string) *Step {
	t.Helper()
	s, err := w.NewStep(name)
	if err != nil {
		t.Fatalf("NewStep(%q): %v", name, err)
	}
	return s
}

func populateAndValidate(ctx context.Context, s *Step) DError {
	if err := s.CreateInstances.populate(ctx, s); err != nil {
		return err
	}
	return s.CreateInstances.validate(ctx, s)
}

// PlacementRegion populate keeps the zone and link empty and the types short;
// the zonal path is unchanged.
func TestPlacementRegionPopulate(t *testing.T) {
	ctx := t.Context()
	w, _, _ := placementRegionWorkflow(t)
	s := mustNewStep(t, w, "s")
	i := regionalInstance(prInstanceName)
	s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
	if err := s.CreateInstances.populate(ctx, s); err != nil {
		t.Fatalf("populate: %v", err)
	}

	realName := w.genName(prInstanceName)
	if i.Zone != "" {
		t.Errorf("Zone = %q, want empty (workflow zone %q must not be inherited)", i.Zone, w.Zone)
	}
	if i.link != "" {
		t.Errorf("link = %q, want empty", i.link)
	}
	if !i.zonePending() {
		t.Error("zonePending() = false, want true")
	}
	if i.Name != realName {
		t.Errorf("Name = %q, want %q", i.Name, realName)
	}
	if i.MachineType != prMachineType {
		t.Errorf("MachineType = %q, want short name %q", i.MachineType, prMachineType)
	}
	boot, data, ssd := i.Disks[0], i.Disks[1], i.Disks[2]
	if got := boot.InitializeParams.DiskType; got != prDiskType {
		t.Errorf("boot DiskType = %q, want short name %q", got, prDiskType)
	}
	if got := boot.InitializeParams.DiskName; got != realName {
		t.Errorf("boot DiskName = %q, want %q", got, realName)
	}
	if got := data.InitializeParams.DiskType; got != prDiskType {
		t.Errorf("data DiskType = %q, want short name %q", got, prDiskType)
	}
	if ssd.Type != "SCRATCH" || !ssd.AutoDelete || ssd.InitializeParams.DiskName != "" {
		t.Errorf("local-ssd disk = {Type: %q, AutoDelete: %v, DiskName: %q}, want {SCRATCH, true, \"\"}", ssd.Type, ssd.AutoDelete, ssd.InitializeParams.DiskName)
	}

	// The zonal path is unchanged: same input without PlacementRegion inherits
	// the workflow zone and gets zonal URLs.
	s2 := mustNewStep(t, w, "s2")
	z := regionalInstance("zonal")
	z.PlacementRegion = ""
	s2.CreateInstances = &CreateInstances{Instances: []*Instance{z}}
	if err := s2.CreateInstances.populate(ctx, s2); err != nil {
		t.Fatalf("zonal populate: %v", err)
	}
	if z.Zone != testZone {
		t.Errorf("zonal Zone = %q, want %q", z.Zone, testZone)
	}
	if want := fmt.Sprintf("projects/%s/zones/%s/instances/%s", testProject, testZone, z.Name); z.link != want {
		t.Errorf("zonal link = %q, want %q", z.link, want)
	}
	if want := fmt.Sprintf("projects/%s/zones/%s/machineTypes/%s", testProject, testZone, prMachineType); z.MachineType != want {
		t.Errorf("zonal MachineType = %q, want %q", z.MachineType, want)
	}
	if want := fmt.Sprintf("projects/%s/zones/%s/diskTypes/%s", testProject, testZone, prDiskType); z.Disks[0].InitializeParams.DiskType != want {
		t.Errorf("zonal DiskType = %q, want %q", z.Disks[0].InitializeParams.DiskType, want)
	}
}

// A valid PlacementRegion instance validates without zone-scoped lookups and
// registers itself and its disks with empty links.
func TestPlacementRegionValidate(t *testing.T) {
	ctx := t.Context()
	w, _, calls := placementRegionWorkflow(t)
	s := mustNewStep(t, w, "s")
	i := regionalInstance(prInstanceName)
	s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
	if err := populateAndValidate(ctx, s); err != nil {
		t.Fatalf("validate of a valid PlacementRegion instance: %v", err)
	}
	if got := calls.snapshot(&calls.zoneScoped); len(got) != 0 {
		t.Errorf("validate made zone-scoped lookups: %q", got)
	}

	ir, ok := w.instances.get(prInstanceName)
	if !ok {
		t.Fatalf("instance %q not registered", prInstanceName)
	}
	if ir.link != "" || ir.creator != s {
		t.Errorf("instance registry entry = {link: %q, creator: %v}, want {\"\", %v}", ir.link, ir.creator, s)
	}
	realName := w.genName(prInstanceName)
	for _, name := range []string{realName, prInstanceName + "-data"} {
		dr, ok := w.disks.get(name)
		if !ok {
			t.Errorf("disk %q not registered", name)
			continue
		}
		if dr.link != "" || dr.creator != s {
			t.Errorf("disk %q registry entry = {link: %q, creator: %v}, want {\"\", %v}", name, dr.link, dr.creator, s)
		}
	}
	if len(w.disks.m) != 2 {
		t.Errorf("registered disks = %v, want only the two persistent disks (no local SSD)", w.disks.m)
	}
	// The instance is registered as attached to its persistent disks, keyed by
	// device name.
	for _, device := range []string{realName, "data"} {
		if _, ok := w.disks.attachments[device][prInstanceName]; !ok {
			t.Errorf("device %q has no attachment to instance %q: %v", device, prInstanceName, w.disks.attachments[device])
		}
	}
}

// Invalid PlacementRegion configurations are rejected at validation.
func TestPlacementRegionValidateRejects(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(i *Instance)
		wantErr string
	}{
		{name: "unknown_region", modify: func(i *Instance) { i.PlacementRegion = "no-such-region" }, wantErr: "region does not exist"},
		{name: "zone_and_region", modify: func(i *Instance) { i.Zone = testZone }, wantErr: "mutually exclusive"},
		{name: "overwrite", modify: func(i *Instance) { i.OverWrite = true }, wantErr: "OverWrite is not supported"},
		{name: "full_machine_type_url", modify: func(i *Instance) {
			i.MachineType = fmt.Sprintf("projects/%s/zones/%s/machineTypes/%s", testProject, testZone, prMachineType)
		}, wantErr: "short machine type name"},
		{name: "full_disk_type_url", modify: func(i *Instance) {
			i.Disks[0].InitializeParams.DiskType = fmt.Sprintf("projects/%s/zones/%s/diskTypes/%s", testProject, testZone, prDiskType)
		}, wantErr: "short disk type name"},
		{name: "full_local_ssd_disk_type_url", modify: func(i *Instance) {
			i.Disks[2].InitializeParams.DiskType = fmt.Sprintf("zones/%s/diskTypes/local-ssd", testZone)
		}, wantErr: "short disk type name"},
		{name: "full_accelerator_type_url", modify: func(i *Instance) {
			i.GuestAccelerators = []*compute.AcceleratorConfig{{AcceleratorType: fmt.Sprintf("projects/%s/zones/%s/acceleratorTypes/nvidia-l4", testProject, testZone), AcceleratorCount: 1}}
		}, wantErr: "short accelerator type name"},
		{name: "source_disk", modify: func(i *Instance) {
			i.Disks = []*compute.AttachedDisk{{Source: fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, testDisk)}}
		}, wantErr: "requires disks to use InitializeParams"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, _, _ := placementRegionWorkflow(t)
			s := mustNewStep(t, w, "s")
			i := regionalInstance(prInstanceName)
			tt.modify(i)
			s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
			err := populateAndValidate(t.Context(), s)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s: populateAndValidate() error = %v, want substring %q", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestPlacementRegionValidateRejectsBeta(t *testing.T) {
	w, _, _ := placementRegionWorkflow(t)
	s := mustNewStep(t, w, "s")
	ib := &InstanceBeta{
		InstanceBase: InstanceBase{PlacementRegion: testRegion},
		Instance: computeBeta.Instance{
			Name:        prInstanceName,
			MachineType: prMachineType,
			Disks:       []*computeBeta.AttachedDisk{{InitializeParams: &computeBeta.AttachedDiskInitializeParams{SourceImage: prSourceImage, DiskType: prDiskType}}},
		},
	}
	s.CreateInstances = &CreateInstances{InstancesBeta: []*InstanceBeta{ib}}
	if err := populateAndValidate(t.Context(), s); err == nil || !strings.Contains(err.Error(), "not supported for InstanceBeta") {
		t.Errorf("populateAndValidate(beta) error = %v, want substring %q", err, "not supported for InstanceBeta")
	}
}

// Duplicate names are still caught by the registry's by-name check.
func TestPlacementRegionValidateDuplicates(t *testing.T) {
	ctx := t.Context()
	w, _, _ := placementRegionWorkflow(t)
	s1 := mustNewStep(t, w, "s1")
	s1.CreateInstances = &CreateInstances{Instances: []*Instance{regionalInstance(prInstanceName)}}
	if err := populateAndValidate(ctx, s1); err != nil {
		t.Fatalf("first step: %v", err)
	}

	s2 := mustNewStep(t, w, "s2")
	w.AddDependency(s2, s1)
	dupInstance := regionalInstance(prInstanceName)
	dupInstance.Disks[1].InitializeParams.DiskName = "other-data"
	s2.CreateInstances = &CreateInstances{Instances: []*Instance{dupInstance}}
	if err := populateAndValidate(ctx, s2); err == nil || !strings.Contains(err.Error(), "already created by step") {
		t.Errorf("duplicate instance: error = %v, want 'already created by step'", err)
	}

	s3 := mustNewStep(t, w, "s3")
	w.AddDependency(s3, s1)
	dupDisk := regionalInstance("vm2")
	dupDisk.Disks[1].InitializeParams.DiskName = prInstanceName + "-data"
	s3.CreateInstances = &CreateInstances{Instances: []*Instance{dupDisk}}
	if err := populateAndValidate(ctx, s3); err == nil || !strings.Contains(err.Error(), "already created by step") {
		t.Errorf("duplicate disk: error = %v, want 'already created by step'", err)
	}
}

// Steps that need the instance's zone at validation time fail clearly for a
// zone-pending instance.
func TestPlacementRegionZonePendingStepValidators(t *testing.T) {
	zonalDisk := fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, testZone, testDisk)
	tests := []struct {
		name  string
		setup func(s *Step)
	}{
		{name: "AttachDisks", setup: func(s *Step) {
			s.AttachDisks = &AttachDisks{{Instance: prInstanceName, AttachedDisk: compute.AttachedDisk{Source: zonalDisk}}}
		}},
		{name: "DetachDisks", setup: func(s *Step) {
			s.DetachDisks = &DetachDisks{{Instance: prInstanceName, DeviceName: "data"}}
		}},
		{name: "UpdateInstancesMetadata", setup: func(s *Step) {
			s.UpdateInstancesMetadata = &UpdateInstancesMetadata{{Instance: prInstanceName, Metadata: map[string]string{"k": "v"}}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			w, tc, calls := placementRegionWorkflow(t)
			// The zonal disk used by AttachDisks exists.
			tc.ListDisksFn = func(p, z string, _ ...daisyCompute.ListCallOption) ([]*compute.Disk, error) {
				if z == "" {
					calls.add(&calls.zoneScoped, "ListDisks with empty zone")
				}
				return []*compute.Disk{{Name: testDisk}}, nil
			}
			tc.AttachDiskFn = func(_, z, _ string, _ *compute.AttachedDisk) error {
				calls.add(&calls.zoneScoped, "AttachDisk "+z)
				return nil
			}
			tc.DetachDiskFn = func(_, z, _, _ string) error {
				calls.add(&calls.zoneScoped, "DetachDisk "+z)
				return nil
			}
			create := mustNewStep(t, w, "create")
			create.CreateInstances = &CreateInstances{Instances: []*Instance{regionalInstance(prInstanceName)}}
			if err := populateAndValidate(ctx, create); err != nil {
				t.Fatalf("%s: populateAndValidate(create) error = %v, want nil", tt.name, err)
			}
			s := mustNewStep(t, w, "use")
			w.AddDependency(s, create)
			tt.setup(s)
			impl, err := s.stepImpl()
			if err != nil {
				t.Fatalf("%s: stepImpl() error = %v, want nil", tt.name, err)
			}
			if err := impl.populate(ctx, s); err != nil {
				t.Fatalf("%s: impl.populate() error = %v, want nil", tt.name, err)
			}
			err = impl.validate(ctx, s)
			if err == nil || !strings.Contains(err.Error(), "has no zone yet (PlacementRegion)") {
				t.Errorf("%s: impl.validate() error = %v, want substring %q", tt.name, err, "has no zone yet (PlacementRegion)")
			}
			if got := calls.snapshot(&calls.zoneScoped); len(got) != 0 {
				t.Errorf("%s: impl.validate() zone-scoped calls = %q, want none", tt.name, got)
			}
		})
	}
}

// A zonal instance never uses bulk insert.
func TestCreateInstancesRunZonalDoesNotBulkInsert(t *testing.T) {
	ctx := t.Context()
	w, _, calls := placementRegionWorkflow(t)
	s := mustNewStep(t, w, "s")
	i := &Instance{InstanceBase: InstanceBase{Resource: Resource{daisyName: "z", Project: testProject}}, Instance: compute.Instance{Name: "z", Zone: testZone, MachineType: "mt", Disks: []*compute.AttachedDisk{{Source: "d"}}}}
	i.link = instanceLink(testProject, testZone, "z")
	s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
	if err := s.CreateInstances.run(ctx, s); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := calls.snapshot(&calls.zonalCreate); len(got) != 1 || got[0] != testZone {
		t.Errorf("zonal creates = %q, want [%q]", got, testZone)
	}
	calls.mu.Lock()
	defer calls.mu.Unlock()
	if len(calls.bulk) != 0 {
		t.Errorf("BulkInsertInstances called %d times for a zonal instance", len(calls.bulk))
	}
}

// Full workflow lifecycle (populate, validate, run, cleanup) of a
// PlacementRegion instance.
func TestPlacementRegionWorkflowRun(t *testing.T) {
	ctx := t.Context()
	w, _, calls := placementRegionWorkflow(t)
	s := mustNewStep(t, w, "create")
	i := regionalInstance(prInstanceName)
	s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
	// Serial-port logging runs in a goroutine; hold the workflow open until it
	// has polled once, so that cleanup doesn't race it.
	wait := mustNewStep(t, w, "wait-for-serial")
	wait.testType = &mockStep{runImpl: func(context.Context, *Step) DError {
		deadline := time.Now().Add(10 * time.Second)
		for len(calls.snapshot(&calls.serial)) == 0 {
			if time.Now().After(deadline) {
				t.Errorf("serial port output was never polled")
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}}
	w.AddDependency(wait, s)

	if err := w.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	realName := w.genName(prInstanceName)

	// Bulk insert request shape.
	calls.mu.Lock()
	bulk, regions := calls.bulk, calls.bulkRegions
	calls.mu.Unlock()
	if len(bulk) != 1 {
		t.Fatalf("BulkInsertInstances called %d times, want 1", len(bulk))
	}
	if regions[0] != testRegion {
		t.Errorf("bulk insert region = %q, want %q", regions[0], testRegion)
	}
	req := bulk[0]
	if req.Count != 1 || req.MinCount != 1 {
		t.Errorf("Count, MinCount = %d, %d; want 1, 1", req.Count, req.MinCount)
	}
	if _, ok := req.PerInstanceProperties[realName]; !ok || len(req.PerInstanceProperties) != 1 {
		t.Errorf("PerInstanceProperties = %v, want exactly key %q", req.PerInstanceProperties, realName)
	}
	if req.LocationPolicy == nil || req.LocationPolicy.TargetShape != "ANY_SINGLE_ZONE" {
		t.Errorf("LocationPolicy = %+v, want TargetShape ANY_SINGLE_ZONE", req.LocationPolicy)
	} else if len(req.LocationPolicy.Locations) != 0 {
		t.Errorf("LocationPolicy.Locations = %v, want none", req.LocationPolicy.Locations)
	}
	if req.NamePattern != "" || req.SourceInstanceTemplate != "" {
		t.Errorf("NamePattern/SourceInstanceTemplate = %q/%q, want empty", req.NamePattern, req.SourceInstanceTemplate)
	}
	p := req.InstanceProperties
	if p == nil {
		t.Fatal("InstanceProperties is nil")
	}
	if p.MachineType != prMachineType {
		t.Errorf("InstanceProperties.MachineType = %q, want %q", p.MachineType, prMachineType)
	}
	if p.Description != "test description" || p.Labels["daisy-test"] != "true" {
		t.Errorf("InstanceProperties Description/Labels = %q/%v, want them carried over", p.Description, p.Labels)
	}
	if len(p.Disks) != 3 || p.Disks[0].DeviceName != realName || p.Disks[0].InitializeParams.DiskName != "" || p.Disks[0].InitializeParams.DiskType != prDiskType || !p.Disks[0].Boot {
		t.Errorf("InstanceProperties.Disks[0] = %+v, want unnamed boot disk with device %q of type %q", p.Disks[0], realName, prDiskType)
	}
	for i, d := range p.Disks {
		if d.InitializeParams != nil && d.InitializeParams.DiskName != "" {
			t.Errorf("InstanceProperties.Disks[%d].InitializeParams.DiskName = %q, want empty (bulk insert rejects disk names)", i, d.InitializeParams.DiskName)
		}
	}
	if want := fmt.Sprintf("projects/%s/global/networks/test-network", testProject); len(p.NetworkInterfaces) != 1 || p.NetworkInterfaces[0].Network != want {
		t.Errorf("InstanceProperties.NetworkInterfaces = %+v, want network %q", p.NetworkInterfaces, want)
	}
	if got := calls.snapshot(&calls.zonalCreate); len(got) != 0 {
		t.Errorf("zonal CreateInstance called: %q", got)
	}

	// The chosen zone is read back into the instance, its link and its disks.
	if i.Zone != prChosenZone {
		t.Errorf("Zone = %q, want %q", i.Zone, prChosenZone)
	}
	if want := instanceLink(testProject, prChosenZone, realName); i.link != want {
		t.Errorf("link = %q, want %q", i.link, want)
	}
	if !i.createdInWorkflow {
		t.Error("createdInWorkflow = false, want true")
	}
	for name, want := range map[string]string{
		realName:                 fmt.Sprintf("projects/%s/zones/%s/disks/%s", testProject, prChosenZone, realName),
		prInstanceName + "-data": fmt.Sprintf("projects/%s/zones/%s/disks/%s-data", testProject, prChosenZone, realName),
	} {
		dr, ok := w.disks.get(name)
		if !ok {
			t.Errorf("disk %q not registered", name)
		} else if dr.link != want {
			t.Errorf("disk %q link = %q, want %q (from refreshed Disks[].Source)", name, dr.link, want)
		}
	}
	for what, zones := range map[string][]string{
		"GetInstance":         calls.snapshot(&calls.get),
		"GetSerialPortOutput": calls.snapshot(&calls.serial),
		"DeleteInstance":      calls.snapshot(&calls.del),
	} {
		if len(zones) == 0 {
			t.Errorf("%s never called", what)
		}
		for _, z := range zones {
			if z != prChosenZone {
				t.Errorf("%s called in zone %q, want %q", what, z, prChosenZone)
			}
		}
	}
	if got := calls.snapshot(&calls.del); len(got) != 1 {
		t.Errorf("DeleteInstance called %d times during cleanup, want 1", len(got))
	}
	if got := calls.snapshot(&calls.zoneScoped); len(got) != 0 {
		t.Errorf("unexpected zone-scoped lookups: %q", got)
	}
}

// Failed bulk insert results leave the instance zone-pending and not created,
// and cleanup makes no API calls for it.
func TestPlacementRegionRunErrors(t *testing.T) {
	tests := []struct {
		name string
		op   *compute.Operation
		err  error
	}{
		{name: "api_error", err: &googleapi.Error{Code: http.StatusServiceUnavailable, Message: "ZONE_RESOURCE_POOL_EXHAUSTED"}},
		{name: "op_error", op: func() *compute.Operation {
			op := bulkOp(prChosenZone)
			op.Error = &compute.OperationError{Errors: []*compute.OperationErrorErrors{{Code: "ZONE_RESOURCE_POOL_EXHAUSTED"}}}
			return op
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			w, tc, calls := placementRegionWorkflow(t)
			tc.BulkInsertInstancesFn = func(_, _ string, _ *compute.BulkInsertInstanceResource) (*compute.Operation, error) {
				return tt.op, tt.err
			}
			s := mustNewStep(t, w, "s")
			i := regionalInstance(prInstanceName)
			s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
			if err := populateAndValidate(ctx, s); err != nil {
				t.Fatalf("%s: populateAndValidate() error = %v, want nil", tt.name, err)
			}
			if err := s.CreateInstances.run(ctx, s); err == nil {
				t.Fatalf("%s: CreateInstances.run() = nil, want error", tt.name)
			}
			if i.link != "" || i.createdInWorkflow || !i.zonePending() {
				t.Errorf("%s: CreateInstances.run() left link=%q, createdInWorkflow=%v, zonePending=%v; want \"\", false, true", tt.name, i.link, i.createdInWorkflow, i.zonePending())
			}
			if i.Zone != "" {
				t.Errorf("%s: CreateInstances.run() left Zone = %q, want empty", tt.name, i.Zone)
			}
			for name := range w.disks.m {
				if dr, _ := w.disks.get(name); dr.link != "" {
					t.Errorf("%s: CreateInstances.run() left disk %q link = %q, want empty", tt.name, name, dr.link)
				}
			}

			getsBefore := len(calls.snapshot(&calls.get))
			w.instances.cleanup()
			w.disks.cleanup()
			if got := calls.snapshot(&calls.del); len(got) != 0 {
				t.Errorf("%s: cleanup() DeleteInstance zones = %q, want none", tt.name, got)
			}
			if got := len(calls.snapshot(&calls.get)) - getsBefore; got != 0 {
				t.Errorf("%s: cleanup() GetInstance calls = %d, want 0", tt.name, got)
			}
		})
	}
}

// If creation fails after GCE created the VM (the refresh or finalizeLocation
// fails), the instance stays zone-pending and is not marked created, so daisy
// cleanup doesn't touch it. Callers sweep such VMs by label, as documented on
// PlacementRegion.
func TestPlacementRegionRunErrorsAfterBulkInsert(t *testing.T) {
	SleepFn = func(time.Duration) {}
	defer func() { SleepFn = time.Sleep }()
	tests := []struct {
		name string
		get  func(name string) (*compute.Instance, error)
	}{
		{name: "refresh_fails", get: func(string) (*compute.Instance, error) {
			return nil, &googleapi.Error{Code: http.StatusInternalServerError}
		}},
		{name: "missing_device", get: func(name string) (*compute.Instance, error) {
			return &compute.Instance{Name: name}, nil
		}},
		{name: "malformed_disk_source", get: func(name string) (*compute.Instance, error) {
			return &compute.Instance{Name: name, Disks: []*compute.AttachedDisk{{DeviceName: name, Source: "garbage"}, {DeviceName: "data", Source: "garbage"}}}, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			w, tc, calls := placementRegionWorkflow(t)
			tc.GetInstanceFn = func(_, z, name string) (*compute.Instance, error) {
				calls.add(&calls.get, z)
				return tt.get(name)
			}
			s := mustNewStep(t, w, "s")
			i := regionalInstance(prInstanceName)
			s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
			if err := populateAndValidate(ctx, s); err != nil {
				t.Fatalf("%s: populateAndValidate() error = %v, want nil", tt.name, err)
			}
			if err := s.CreateInstances.run(ctx, s); err == nil {
				t.Fatalf("%s: CreateInstances.run() = nil, want error", tt.name)
			}
			if i.link != "" || i.createdInWorkflow || !i.zonePending() {
				t.Errorf("%s: CreateInstances.run() left link=%q, createdInWorkflow=%v, zonePending=%v; want \"\", false, true", tt.name, i.link, i.createdInWorkflow, i.zonePending())
			}
			w.instances.cleanup()
			if got := calls.snapshot(&calls.del); len(got) != 0 {
				t.Errorf("%s: instances.cleanup() DeleteInstance zones = %q, want none", tt.name, got)
			}
		})
	}
}

// deleteFn of a zone-pending instance fails without calling the API.
func TestPlacementRegionDeleteZonePending(t *testing.T) {
	w, _, calls := placementRegionWorkflow(t)
	SleepFn = func(time.Duration) {}
	defer func() { SleepFn = time.Sleep }()
	err := w.instances.deleteFn(&Resource{daisyName: prInstanceName, RealName: "real"})
	if err == nil || !strings.Contains(err.Error(), "has no zone yet (PlacementRegion)") {
		t.Errorf("deleteFn error = %v, want 'has no zone yet (PlacementRegion)'", err)
	}
	if got := append(calls.snapshot(&calls.get), calls.snapshot(&calls.del)...); len(got) != 0 {
		t.Errorf("deleteFn made API calls in zones %q, want none", got)
	}
}

func TestPlacementRegionClientWithoutBulkInsert(t *testing.T) {
	w, tc, _ := placementRegionWorkflow(t)
	s := mustNewStep(t, w, "s")
	i := regionalInstance(prInstanceName)
	s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
	if err := populateAndValidate(t.Context(), s); err != nil {
		t.Fatalf("populate/validate: %v", err)
	}
	err := i.create(bulkUnsupportedClient{tc})
	if err == nil || !strings.Contains(err.Error(), "does not support regional bulk insert") {
		t.Errorf("create error = %v, want 'does not support regional bulk insert'", err)
	}
}

// A retry after an external IP is denied sends no access configs.
func TestPlacementRegionRetryWhenExternalIPDenied(t *testing.T) {
	ctx := t.Context()
	w, tc, _ := placementRegionWorkflow(t)
	var attempts []*compute.BulkInsertInstanceResource
	tc.BulkInsertInstancesFn = func(_, _ string, r *compute.BulkInsertInstanceResource) (*compute.Operation, error) {
		// Snapshot the request: daisy mutates the instance between attempts.
		b, err := json.Marshal(r)
		if err != nil {
			t.Errorf("marshaling bulk insert request: %v", err)
		}
		c := &compute.BulkInsertInstanceResource{}
		if err := json.Unmarshal(b, c); err != nil {
			t.Errorf("unmarshaling bulk insert request: %v", err)
		}
		attempts = append(attempts, c)
		if len(attempts) == 1 {
			return nil, &googleapi.Error{Code: http.StatusPreconditionFailed, Message: "Constraint constraints/compute.vmExternalIpAccess violated"}
		}
		return bulkOp(prChosenZone), nil
	}
	s := mustNewStep(t, w, "s")
	i := regionalInstance(prInstanceName)
	i.RetryWhenExternalIPDenied = true
	// No serial-port logging goroutine that could outlive the test.
	i.SerialPortsToLog = []int64{}
	i.NetworkInterfaces = []*compute.NetworkInterface{{Network: prNetwork}, {Network: prNetwork}}
	s.CreateInstances = &CreateInstances{Instances: []*Instance{i}}
	if err := populateAndValidate(ctx, s); err != nil {
		t.Fatalf("populate/validate: %v", err)
	}
	if err := s.CreateInstances.run(ctx, s); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(attempts) != 2 {
		t.Fatalf("bulk insert attempts = %d, want 2", len(attempts))
	}
	for n, nic := range attempts[0].InstanceProperties.NetworkInterfaces {
		if len(nic.AccessConfigs) == 0 {
			t.Errorf("first attempt NIC %d has no access configs, want the default external IP", n)
		}
	}
	for n, nic := range attempts[1].InstanceProperties.NetworkInterfaces {
		if len(nic.AccessConfigs) != 0 {
			t.Errorf("retry NIC %d AccessConfigs = %+v, want none", n, nic.AccessConfigs)
		}
	}
	if i.Zone != prChosenZone {
		t.Errorf("Zone = %q, want %q", i.Zone, prChosenZone)
	}
}
