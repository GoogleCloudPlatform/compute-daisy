//  Copyright 2017 Google Inc. All Rights Reserved.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const (
	defaultAccessConfigType = "ONE_TO_ONE_NAT"
	defaultDiskMode         = diskModeRW
	defaultDiskType         = "pd-standard"
	diskModeRO              = "READ_ONLY"
	diskModeRW              = "READ_WRITE"
	localSSDDiskType        = "local-ssd"
)

var (
	instanceURLRgx = regexp.MustCompile(fmt.Sprintf(`^(projects/(?P<project>%[1]s)/)?zones/(?P<zone>%[2]s)/instances/(?P<instance>%[2]s)$`, projectRgxStr, rfc1035))
	validDiskModes = []string{diskModeRO, diskModeRW}
)

func checkDiskMode(m string) bool {
	parts := strings.Split(m, "/")
	m = parts[len(parts)-1]
	return strIn(m, validDiskModes)
}

// instanceExists should only be used during validation for existing GCE instances
// and should not be relied or populated for daisy created resources.
func (w *Workflow) instanceExists(project, zone, instance string) (bool, DError) {
	return w.instanceCache.resourceExists(func(project, zone string, opts ...daisyCompute.ListCallOption) (interface{}, error) {
		return w.ComputeClient.ListInstances(project, zone)
	}, project, zone, instance)
}

// MarshalJSON is a workaround to prevent Instance from using compute.Instance's implementation.
func (i *Instance) MarshalJSON() ([]byte, error) {
	return json.Marshal(*i)
}

// InstanceInterface represent abstract Instance across different API stages (Alpha, Beta, API)
type InstanceInterface interface {
	getName() string
	setName(name string)
	getDescription() string
	setDescription(description string)
	getZone() string
	setZone(zone string)
	getMachineType() string
	setMachineType(machineType string)
	populateDisks(w *Workflow) DError
	populateNetworks() DError
	populateScopes() DError
	initializeComputeMetadata()
	appendComputeMetadata(key string, value *string)
	validateNetworks(s *Step) (errs DError)
	getComputeDisks() []*computeDisk
	create(cc daisyCompute.Client) error
	delete(cc daisyCompute.Client, deleteDisk bool) error
	updateDisksAndNetworksBeforeCreate(w *Workflow)
	getMetadata() map[string]string
	setMetadata(md map[string]string)
	getSourceMachineImage() string
	setSourceMachineImage(machineImage string)
}

// InstanceBase is a base struct for GA/Beta instances.
// It holds the shared properties between the two.
type InstanceBase struct {
	Resource

	// OAuth2 scopes to give the instance. If left unset
	// https://www.googleapis.com/auth/devstorage.read_only will be added.
	Scopes []string `json:",omitempty"`
	// StartupScript is the Sources path to a startup script to use in this step.
	// This will be automatically mapped to the appropriate metadata key.
	StartupScript string `json:",omitempty"`
	// RetryWhenExternalIPDenied indicates whether to retry CreateInstances when
	// it fails due to external IP denied by organization IP.
	RetryWhenExternalIPDenied bool `json:",omitempty"`
	// Should an existing instance of the same name be deleted, defaults to false
	// which will fail validation.
	OverWrite bool `json:",omitempty"`
	// Serial port to log to GCS bucket, defaults to 1
	SerialPortsToLog []int64 `json:",omitempty"`
	// AutoSelectDiskType prevents Daisy from defaulting empty InitializeParams.DiskType to pd-standard.
	AutoSelectDiskType bool `json:",omitempty"`
	// PlacementRegion, if set (e.g. "us-central1"), creates the instance with
	// regionInstances.bulkInsert and lets GCE pick any zone in the region that
	// offers the machine type and has capacity. Zone must be empty; it holds
	// the chosen zone once the instance is created. Disks must use
	// InitializeParams, and MachineType, disk types and accelerator types must
	// be short names. GA Instance only.
	//
	// Not supported: AttachDisks, DetachDisks and UpdateInstancesMetadata on
	// the instance, and Source, AttachDisks and ResizeDisks references to its
	// InitializeParams disks.
	//
	// Limitation: if any error occurs after the bulk insert succeeded (e.g. a
	// timeout, the refresh of the created instance, or recording its location),
	// daisy doesn't mark the VM created and won't delete it. Callers that need
	// guaranteed cleanup should also sweep by label.
	PlacementRegion string `json:",omitempty"`
}

// instanceLink returns the partial URL of a zonal instance.
func instanceLink(project, zone, name string) string {
	return fmt.Sprintf("projects/%s/zones/%s/instances/%s", project, zone, name)
}

// isLocalSSDDiskType reports whether diskType, a disk type URL or a short
// disk type name, is local SSD.
func isLocalSSDDiskType(diskType string) bool {
	return diskType == localSSDDiskType || NamedSubexp(diskTypeURLRgx, diskType)["disktype"] == localSSDDiskType
}

// zonePending reports whether the instance uses PlacementRegion and has an
// empty link, i.e. its zone isn't known yet. Until creation succeeds, such an
// instance has an empty Zone and link, and its InitializeParams disks are
// registered with empty links.
func (ib *InstanceBase) zonePending() bool {
	return ib.PlacementRegion != "" && ib.link == ""
}

// zonalInstanceLink parses the link of an instance resource. It returns an
// error if the instance has no zone yet, i.e. it uses PlacementRegion and
// hasn't been created.
func zonalInstanceLink(res *Resource) (map[string]string, DError) {
	if res.link == "" {
		return nil, Errf("instance %q has no zone yet (PlacementRegion)", res.daisyName)
	}
	return NamedSubexp(instanceURLRgx, res.link), nil
}

// Instance is used to create a GCE instance using GA API.
// By default, output of serial port 1 will be streamed to the daisy logs directory.
type Instance struct {
	InstanceBase
	compute.Instance

	// Additional metadata to set for the instance.
	Metadata map[string]string `json:"metadata,omitempty"`

	// bulkDiskNames maps the device name of each InitializeParams disk sent in
	// a regional bulk insert to the disk's registry name. It's recorded before
	// the insert because the refreshed instance no longer has InitializeParams,
	// only each disk's DeviceName and Source.
	bulkDiskNames map[string]string
}

// InstanceBeta is used to create a GCE instance using Beta API.
// By default, output of serial port 1 will be streamed to the daisy logs directory.
type InstanceBeta struct {
	InstanceBase
	computeBeta.Instance

	// Additional metadata to set for the instance.
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (i *Instance) getMachineType() string {
	return i.MachineType
}

func (i *Instance) setMachineType(machineType string) {
	i.MachineType = machineType
}

func (i *Instance) getDescription() string {
	return i.Description
}
func (i *Instance) setDescription(description string) {
	i.Description = description
}

func (i *Instance) getName() string {
	return i.Name
}
func (i *Instance) setName(name string) {
	i.Name = name
}

func (i *Instance) getZone() string {
	return i.Zone
}

func (i *Instance) setZone(zone string) {
	i.Zone = zone
}

func (i *Instance) initializeComputeMetadata() {
	if i.Instance.Metadata == nil {
		i.Instance.Metadata = &compute.Metadata{}
	}
}

func (i *Instance) appendComputeMetadata(key string, value *string) {
	i.Instance.Metadata.Items = append(i.Instance.Metadata.Items, &compute.MetadataItems{Key: key, Value: value})
}

func (i *Instance) create(cc daisyCompute.Client) error {
	if i.zonePending() {
		return i.bulkCreate(cc, i.PlacementRegion)
	}
	return cc.CreateInstance(i.Project, i.Zone, &i.Instance)
}

// bulkInstanceInserter is implemented by compute clients that support
// regionInstances.bulkInsert. It's not part of daisyCompute.Client so that
// existing Client implementations keep compiling.
type bulkInstanceInserter interface {
	BulkInsertInstances(project, region string, r *compute.BulkInsertInstanceResource) (*compute.Operation, error)
}

// bulkCreate creates the instance in region with regionInstances.bulkInsert
// and lets GCE pick its zone. On success it sets Zone to the chosen zone and
// refreshes Instance from GCE. The request is rebuilt on every call so that
// retries pick up changes to Instance, such as removed access configs.
func (i *Instance) bulkCreate(cc daisyCompute.Client, region string) error {
	// BulkInsertInstances is optional on daisyCompute.Client. Verify support
	// before converting the instance spec.
	bi, ok := cc.(bulkInstanceInserter)
	if !ok {
		return fmt.Errorf("instance %q: compute client %T does not support regional bulk insert", i.Name, cc)
	}

	// Bulk insert takes InstanceProperties rather than Instance (stripping
	// Name, Zone, and InitializeParams.DiskName). Converting here also catches
	// fields regional bulk insert does not support before mutating i.
	props, err := instanceProperties(&i.Instance)
	if err != nil {
		return err
	}

	// Map DeviceName to Daisy's registry disk name before GetInstance replaces
	// i.Instance and drops InitializeParams. Bulk insert names disks itself, so
	// finalizeLocation uses this map to link GCE's disk URLs back to the disk
	// registry. Skip local SSDs because scratch disks are not registered.
	diskNames := map[string]string{}
	for _, d := range i.Disks {
		if d.InitializeParams != nil && !isLocalSSDDiskType(d.InitializeParams.DiskType) {
			diskNames[d.DeviceName] = d.InitializeParams.DiskName
		}
	}
	i.bulkDiskNames = diskNames

	req := &compute.BulkInsertInstanceResource{
		// Request a single VM and require it to be created (all-or-nothing).
		Count:              1,
		MinCount:           1,
		InstanceProperties: props,
		// PerInstanceProperties keys assign i.Name verbatim instead of using
		// NamePattern. The value is empty because all settings are in
		// InstanceProperties.
		PerInstanceProperties: map[string]compute.BulkInsertInstanceResourcePerInstanceProperties{
			i.Name: {},
		},
		// Omit Locations so GCE considers every zone in the region.
		// ANY_SINGLE_ZONE picks one zone based on capacity, quotas, and
		// reservations.
		LocationPolicy: &compute.LocationPolicy{TargetShape: "ANY_SINGLE_ZONE"},
	}

	// Submit the regional bulk insert and wait for completion. The returned
	// operation's per-location metadata is how GCE reports the chosen zone.
	op, err := bi.BulkInsertInstances(i.Project, region, req)
	if err != nil {
		return err
	}
	zone, err := bulkInsertZone(op)
	if err != nil {
		return fmt.Errorf("instance %q: %v", i.Name, err)
	}

	// Fetch the created instance to match CreateInstance's refresh behavior
	// and obtain each disk's GCE-assigned Source URL for finalizeLocation.
	createdInstance, err := cc.GetInstance(i.Project, zone, i.Name)
	if err != nil {
		return err
	}

	// Set i.Zone after copying createdInstance. GetInstance returns Zone as a
	// full URL, whereas Daisy expects the short zone name (for example, in
	// finalizeLocation). Deferring both writes until after GetInstance also
	// leaves i.Zone empty on error.
	i.Instance = *createdInstance
	i.Zone = zone
	return nil
}

// instanceProperties converts ci, without its Name and Zone, to the
// InstanceProperties of a bulk insert request. A set field that
// InstanceProperties doesn't have is an error naming the field. The JSON round
// trip doesn't carry ForceSendFields or NullFields over.
//
// InitializeParams.DiskName is cleared: bulk insert rejects named disks and
// names them itself. finalizeLocation takes the real disk links from the
// created instance, matched by DeviceName.
func instanceProperties(ci *compute.Instance) (*compute.InstanceProperties, error) {
	c := *ci
	c.Name = ""
	c.Zone = ""
	b, err := json.Marshal(c)
	if err != nil {
		return nil, fmt.Errorf("cannot convert instance %q for bulk insert: %v", ci.Name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	props := &compute.InstanceProperties{}
	if err := dec.Decode(props); err != nil {
		return nil, fmt.Errorf("cannot convert instance %q for bulk insert, regional bulk insert doesn't support this field: %v", ci.Name, err)
	}
	// props was decoded fresh, so this doesn't modify ci.
	for _, d := range props.Disks {
		if d.InitializeParams != nil {
			d.InitializeParams.DiskName = ""
		}
	}
	return props, nil
}

// bulkInsertZone returns the zone a single-instance bulk insert created its
// instance in.
func bulkInsertZone(op *compute.Operation) (string, error) {
	if op == nil {
		return "", fmt.Errorf("bulk insert returned no operation")
	}
	if op.Error != nil {
		var operrs string
		for _, operr := range op.Error.Errors {
			operrs += fmt.Sprintf("\nCode: %s\nMessage: %s", operr.Code, operr.Message)
		}
		return "", fmt.Errorf("bulk insert operation %q failed:%s", op.Name, operrs)
	}
	if op.InstancesBulkInsertOperationMetadata == nil {
		return "", fmt.Errorf("bulk insert operation %q has no instancesBulkInsertOperationMetadata", op.Name)
	}
	var created []string
	for loc, st := range op.InstancesBulkInsertOperationMetadata.PerLocationStatus {
		if st.CreatedVmCount != 0 {
			created = append(created, loc)
		}
	}
	if len(created) != 1 {
		sort.Strings(created)
		return "", fmt.Errorf("bulk insert operation %q: want exactly one location with a created VM, got %q", op.Name, created)
	}
	loc := created[0]
	if n := op.InstancesBulkInsertOperationMetadata.PerLocationStatus[loc].CreatedVmCount; n != 1 {
		return "", fmt.Errorf("bulk insert operation %q: want 1 created VM in %q, got %d", op.Name, loc, n)
	}
	zone := strings.TrimPrefix(loc, "zones/")
	if zone == loc || !rfc1035Rgx.MatchString(zone) {
		return "", fmt.Errorf("bulk insert operation %q: unexpected location %q, want zones/<zone>", op.Name, loc)
	}
	return zone, nil
}

// finalizeLocation is called once a bulk-inserted instance has been created in
// its chosen zone. It sets the instance link and the registry links of its
// InitializeParams disks. On error nothing is changed.
func (i *Instance) finalizeLocation(w *Workflow) DError {
	sources := map[string]string{}
	for _, d := range i.Disks {
		sources[d.DeviceName] = d.Source
	}
	type diskLink struct {
		res  *Resource
		link string
	}
	var diskLinks []diskLink
	for deviceName, diskName := range i.bulkDiskNames {
		src, ok := sources[deviceName]
		if !ok || src == "" {
			return Errf("instance %q (zone %q): created instance has no disk with device name %q", i.daisyName, i.Zone, deviceName)
		}
		idx := strings.Index(src, "projects/")
		if idx < 0 || !diskURLRgx.MatchString(src[idx:]) {
			return Errf("instance %q (zone %q): unexpected source %q for disk with device name %q", i.daisyName, i.Zone, src, deviceName)
		}
		res, ok := w.disks.get(diskName)
		if !ok {
			return Errf("instance %q (zone %q): disk %q not found in registry", i.daisyName, i.Zone, diskName)
		}
		diskLinks = append(diskLinks, diskLink{res: res, link: src[idx:]})
	}

	for _, dl := range diskLinks {
		dl.res.link = dl.link
	}
	i.link = instanceLink(i.Project, i.Zone, i.RealName)
	return nil
}

func (i *Instance) delete(cc daisyCompute.Client, deleteDisk bool) error {
	return deleteInstance(deleteDisk, cc, i.Project, i.Zone, i.Name)
}

func (i *Instance) updateDisksAndNetworksBeforeCreate(w *Workflow) {
	for _, d := range i.Disks {
		if diskRes, ok := w.disks.get(d.Source); ok {
			d.Source = diskRes.link
		}
		if d.InitializeParams != nil && d.InitializeParams.SourceImage != "" {
			if image, ok := w.images.get(d.InitializeParams.SourceImage); ok {
				d.InitializeParams.SourceImage = image.link
			}
		}
	}

	for _, n := range i.NetworkInterfaces {
		if netRes, ok := w.networks.get(n.Network); ok {
			n.Network = netRes.link
		}
		if subnetRes, ok := w.subnetworks.get(n.Subnetwork); ok {
			n.Subnetwork = subnetRes.link
		}
	}
}

func (i *Instance) getMetadata() map[string]string {
	return i.Metadata
}

func (i *Instance) setMetadata(md map[string]string) {
	i.Metadata = md
}

func (i *Instance) getSourceMachineImage() string {
	return i.Instance.SourceMachineImage
}

func (i *Instance) setSourceMachineImage(machineImage string) {
	i.SourceMachineImage = machineImage
}

func (i *Instance) register(name string, s *Step, ir *instanceRegistry, errs DError) {
	// Register disk attachments.
	for _, d := range i.Disks {
		diskName := d.Source
		if d.InitializeParams != nil {
			if isLocalSSDDiskType(d.InitializeParams.DiskType) {
				continue
			}
			diskName = d.InitializeParams.DiskName
		}

		errs = addErrs(errs, ir.w.disks.regAttach(d.DeviceName, diskName, name, d.Mode, s))
	}

	// Register network connections.
	for _, n := range i.NetworkInterfaces {
		nName := n.Network
		errs = addErrs(errs, ir.w.networks.regConnect(nName, name, s))
	}
}

func (i *InstanceBeta) getMachineType() string {
	return i.MachineType
}

func (i *InstanceBeta) setMachineType(machineType string) {
	i.MachineType = machineType
}

func (i *InstanceBeta) getDescription() string {
	return i.Description
}
func (i *InstanceBeta) setDescription(description string) {
	i.Description = description
}

func (i *InstanceBeta) getName() string {
	return i.Name
}
func (i *InstanceBeta) setName(name string) {
	i.Name = name
}

func (i *InstanceBeta) getZone() string {
	return i.Zone
}

func (i *InstanceBeta) setZone(zone string) {
	i.Zone = zone
}

func (i *InstanceBeta) appendComputeMetadata(key string, value *string) {
	i.Instance.Metadata.Items = append(i.Instance.Metadata.Items, &computeBeta.MetadataItems{Key: key, Value: value})
}

func (i *InstanceBeta) initializeComputeMetadata() {
	if i.Instance.Metadata == nil {
		i.Instance.Metadata = &computeBeta.Metadata{}
	}
}

func (i *InstanceBeta) create(cc daisyCompute.Client) error {
	return cc.CreateInstanceBeta(i.Project, i.Zone, &i.Instance)
}

func (i *InstanceBeta) delete(cc daisyCompute.Client, deleteDisk bool) error {
	return deleteInstance(deleteDisk, cc, i.Project, i.Zone, i.Name)
}

func (i *InstanceBeta) updateDisksAndNetworksBeforeCreate(w *Workflow) {
	for _, d := range i.Disks {
		if diskRes, ok := w.disks.get(d.Source); ok {
			d.Source = diskRes.link
		}
		if d.InitializeParams != nil && d.InitializeParams.SourceImage != "" {
			if image, ok := w.images.get(d.InitializeParams.SourceImage); ok {
				d.InitializeParams.SourceImage = image.link
			}
		}
	}

	for _, n := range i.NetworkInterfaces {
		if netRes, ok := w.networks.get(n.Network); ok {
			n.Network = netRes.link
		}
		if subnetRes, ok := w.subnetworks.get(n.Subnetwork); ok {
			n.Subnetwork = subnetRes.link
		}
	}
}

func (i *InstanceBeta) getMetadata() map[string]string {
	return i.Metadata
}

func (i *InstanceBeta) setMetadata(md map[string]string) {
	i.Metadata = md
}

func (i *InstanceBeta) getSourceMachineImage() string {
	return i.Instance.SourceMachineImage
}

func (i *InstanceBeta) setSourceMachineImage(machineImage string) {
	i.SourceMachineImage = machineImage
}

func (i *InstanceBeta) register(name string, s *Step, ir *instanceRegistry, errs DError) {
	// Register disk attachments.
	for _, d := range i.Disks {
		diskName := d.Source
		if d.InitializeParams != nil {
			parts := NamedSubexp(diskTypeURLRgx, d.InitializeParams.DiskType)
			if parts["disktype"] == "local-ssd" {
				continue
			}
			diskName = d.InitializeParams.DiskName
		}
		errs = addErrs(errs, ir.w.disks.regAttach(d.DeviceName, diskName, name, d.Mode, s))
	}

	// Register network connections.
	for _, n := range i.NetworkInterfaces {
		nName := n.Network
		errs = addErrs(errs, ir.w.networks.regConnect(nName, name, s))
	}
}

func (ib *InstanceBase) populate(ctx context.Context, ii InstanceInterface, s *Step) DError {
	var errs DError
	if ib.zonePending() {
		// The zone is chosen by GCE at creation; don't inherit the workflow zone.
		errs = ib.Resource.populateHelper(ctx, s, ii.getName())
		ii.setName(ib.RealName)
	} else {
		var name, zone string
		name, zone, errs = ib.Resource.populateWithZone(ctx, s, ii.getName(), ii.getZone())
		ii.setName(name)
		ii.setZone(zone)
	}

	ii.setDescription(strOr(ii.getDescription(), fmt.Sprintf("Instance created by Daisy in workflow %q on behalf of %s.", s.w.Name, s.w.username)))
	errs = addErrs(errs, ib.populateSerialPortsToLog())
	errs = addErrs(errs, ii.populateDisks(s.w))
	errs = addErrs(errs, ib.populateMachineType(ii))
	errs = addErrs(errs, ib.populateMetadata(ii, s.w))
	errs = addErrs(errs, ii.populateNetworks())
	errs = addErrs(errs, ii.populateScopes())
	if !ib.zonePending() {
		ib.link = instanceLink(ib.Project, ii.getZone(), ii.getName())
	}

	if machineImageURLRgx.MatchString(ii.getSourceMachineImage()) {
		ii.setSourceMachineImage(extendPartialURL(ii.getSourceMachineImage(), ib.Project))
	}
	return errs
}

func (i *Instance) populateDisks(w *Workflow) DError {
	autonameIdx := 1
	for di, d := range i.Disks {
		d.Boot = di == 0
		d.Mode = strOr(d.Mode, defaultDiskMode)
		if diskURLRgx.MatchString(d.Source) {
			d.Source = extendPartialURL(d.Source, i.Project)
		}
		p := d.InitializeParams
		if p != nil {
			// If name isn't set, set name to "instance-name", "instance-name-2", etc.
			if p.DiskName == "" {
				p.DiskName = i.Name
				if autonameIdx > 1 {
					p.DiskName = fmt.Sprintf("%s-%d", i.Name, autonameIdx)
				}
				autonameIdx++
			}
			if d.DeviceName == "" {
				d.DeviceName = p.DiskName
			}

			// Extend SourceImage if short URL.
			if imageURLRgx.MatchString(p.SourceImage) {
				p.SourceImage = extendPartialURL(p.SourceImage, i.Project)
			}

			// Extend DiskType if short URL, or create extended URL.
			if !i.AutoSelectDiskType {
				p.DiskType = strOr(p.DiskType, defaultDiskType)
			}
			// A PlacementRegion instance keeps the short name: its zone isn't known yet.
			if p.DiskType != "" && !i.zonePending() {
				if diskTypeURLRgx.MatchString(p.DiskType) {
					p.DiskType = extendPartialURL(p.DiskType, i.Project)
				} else {
					p.DiskType = fmt.Sprintf("projects/%s/zones/%s/diskTypes/%s", i.Project, i.Zone, p.DiskType)
				}
			}
			if isLocalSSDDiskType(p.DiskType) {
				d.AutoDelete = true
				d.Type = "SCRATCH"
				p.DiskName = ""
			}
		} else if d.DeviceName == "" {
			d.DeviceName = path.Base(d.Source)
		}
	}
	return nil
}

func (i *InstanceBeta) populateDisks(w *Workflow) DError {
	autonameIdx := 1
	for di, d := range i.Disks {
		d.Boot = di == 0
		d.Mode = strOr(d.Mode, defaultDiskMode)
		if diskURLRgx.MatchString(d.Source) {
			d.Source = extendPartialURL(d.Source, i.Project)
		}
		p := d.InitializeParams
		if p != nil {
			// If name isn't set, set name to "instance-name", "instance-name-2", etc.
			if p.DiskName == "" {
				p.DiskName = i.Name
				if autonameIdx > 1 {
					p.DiskName = fmt.Sprintf("%s-%d", i.Name, autonameIdx)
				}
				autonameIdx++
			}
			if d.DeviceName == "" {
				d.DeviceName = p.DiskName
			}

			// Extend SourceImage if short URL.
			if imageURLRgx.MatchString(p.SourceImage) {
				p.SourceImage = extendPartialURL(p.SourceImage, i.Project)
			}

			// Extend DiskType if short URL, or create extended URL.
			if !i.AutoSelectDiskType {
				p.DiskType = strOr(p.DiskType, defaultDiskType)
			}
			if p.DiskType != "" {
				if diskTypeURLRgx.MatchString(p.DiskType) {
					p.DiskType = extendPartialURL(p.DiskType, i.Project)
				} else {
					p.DiskType = fmt.Sprintf("projects/%s/zones/%s/diskTypes/%s", i.Project, i.Zone, p.DiskType)
				}
			}
			parts := NamedSubexp(diskTypeURLRgx, p.DiskType)
			if parts["disktype"] == "local-ssd" {
				d.AutoDelete = true
				d.Type = "SCRATCH"
				p.DiskName = ""
			}
		} else if d.DeviceName == "" {
			d.DeviceName = path.Base(d.Source)
		}
	}
	return nil
}

func uniqueSerialPortsToLog(slice []int64) []int64 {
	keys := make(map[int64]bool)
	list := []int64{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func (ib *InstanceBase) populateSerialPortsToLog() DError {
	if ib.SerialPortsToLog == nil {
		ib.SerialPortsToLog = append(ib.SerialPortsToLog, 1)
	} else {
		ib.SerialPortsToLog = uniqueSerialPortsToLog(ib.SerialPortsToLog)
	}
	return nil
}

func (ib *InstanceBase) populateMachineType(ii InstanceInterface) DError {
	// when creating instance from a machine image, don't set default machine type
	if ii.getSourceMachineImage() != "" && ii.getMachineType() == "" {
		return nil
	}

	ii.setMachineType(strOr(ii.getMachineType(), "n1-standard-1"))
	if ib.zonePending() {
		// Bulk insert takes a short machine type name; the zone isn't known yet.
		return nil
	}
	if machineTypeURLRegex.MatchString(ii.getMachineType()) {
		ii.setMachineType(extendPartialURL(ii.getMachineType(), ib.Project))
	} else {
		ii.setMachineType(fmt.Sprintf("projects/%s/zones/%s/machineTypes/%s", ib.Project, ii.getZone(), ii.getMachineType()))
	}
	return nil
}

func (ib *InstanceBase) populateMetadata(ii InstanceInterface, w *Workflow) DError {
	if ii.getMetadata() == nil {
		ii.setMetadata(map[string]string{})
	}
	ii.initializeComputeMetadata()

	ii.getMetadata()["daisy-sources-path"] = "gs://" + path.Join(w.bucket, w.sourcesPath)
	ii.getMetadata()["daisy-logs-path"] = "gs://" + path.Join(w.bucket, w.logsPath)
	ii.getMetadata()["daisy-outs-path"] = "gs://" + path.Join(w.bucket, w.outsPath)
	if ib.StartupScript != "" {
		if !w.sourceExists(ib.StartupScript) {
			return Errf("bad value for StartupScript, source not found: %s", ib.StartupScript)
		}
		ib.StartupScript = "gs://" + path.Join(w.bucket, w.sourcesPath, ib.StartupScript)
		ii.getMetadata()["startup-script-url"] = ib.StartupScript
		ii.getMetadata()["windows-startup-script-url"] = ib.StartupScript
	}
	for k, v := range ii.getMetadata() {
		vCopy := v
		ii.appendComputeMetadata(k, &vCopy)
	}
	return nil
}

func (i *Instance) populateNetworks() DError {
	defaultAcs := []*compute.AccessConfig{{Type: defaultAccessConfigType}}
	defaultIpv6Acs := []*compute.AccessConfig{{Type: "DIRECT_IPV6", NetworkTier: "PREMIUM"}}

	if i.NetworkInterfaces == nil {
		i.NetworkInterfaces = []*compute.NetworkInterface{{}}
	}
	for _, n := range i.NetworkInterfaces {
		if strings.Contains(n.StackType, "IPV6") && n.Ipv6AccessConfigs == nil && n.Ipv6AccessType != "INTERNAL" {
			n.Ipv6AccessConfigs = defaultIpv6Acs
		}
		if (n.StackType == "" || strings.Contains(n.StackType, "IPV4")) && n.AccessConfigs == nil {
			n.AccessConfigs = defaultAcs
		}

		// Only set default if no subnetwork or network set.
		if n.Subnetwork == "" {
			n.Network = strOr(n.Network, "global/networks/default")
		}

		if networkURLRegex.MatchString(n.Network) {
			n.Network = extendPartialURL(n.Network, i.Project)
		}

		if subnetworkURLRegex.MatchString(n.Subnetwork) {
			n.Subnetwork = extendPartialURL(n.Subnetwork, i.Project)
		}
	}

	return nil
}

func (i *InstanceBeta) populateNetworks() DError {
	defaultAcs := []*computeBeta.AccessConfig{{Type: defaultAccessConfigType}}
	defaultIpv6Acs := []*computeBeta.AccessConfig{{Type: "DIRECT_IPV6", NetworkTier: "PREMIUM"}}

	if i.NetworkInterfaces == nil {
		i.NetworkInterfaces = []*computeBeta.NetworkInterface{{}}
	}
	for _, n := range i.NetworkInterfaces {
		if strings.Contains(n.StackType, "IPV6") && n.Ipv6AccessConfigs == nil && n.Ipv6AccessType != "INTERNAL" {
			n.Ipv6AccessConfigs = defaultIpv6Acs
		}
		if (n.StackType == "" || strings.Contains(n.StackType, "IPV4")) && n.AccessConfigs == nil {
			n.AccessConfigs = defaultAcs
		}

		// Only set default if no subnetwork or network set.
		if n.Subnetwork == "" {
			n.Network = strOr(n.Network, "global/networks/default")
		}

		if networkURLRegex.MatchString(n.Network) {
			n.Network = extendPartialURL(n.Network, i.Project)
		}

		if subnetworkURLRegex.MatchString(n.Subnetwork) {
			n.Subnetwork = extendPartialURL(n.Subnetwork, i.Project)
		}
	}

	return nil
}

func (i *Instance) populateScopes() DError {
	if i.Scopes == nil {
		i.Scopes = append(i.Scopes, "https://www.googleapis.com/auth/devstorage.read_only")
	}
	if i.ServiceAccounts == nil {
		i.ServiceAccounts = []*compute.ServiceAccount{{Email: "default", Scopes: i.Scopes}}
	}
	return nil
}

func (i *InstanceBeta) populateScopes() DError {
	if i.Scopes == nil {
		i.Scopes = append(i.Scopes, "https://www.googleapis.com/auth/devstorage.read_only")
	}
	if i.ServiceAccounts == nil {
		i.ServiceAccounts = []*computeBeta.ServiceAccount{{Email: "default", Scopes: i.Scopes}}
	}
	return nil
}

func (ib *InstanceBase) validate(ctx context.Context, ii InstanceInterface, s *Step) DError {
	pre := fmt.Sprintf("cannot create instance %q", ib.daisyName)
	var errs DError
	if ib.zonePending() {
		errs = ib.validatePlacementRegion(ctx, ii, s, pre)
	} else {
		errs = ib.Resource.validateWithZone(ctx, s, ii.getZone(), pre)
	}
	errs = addErrs(errs, ib.validateSerialPortsToLog())
	errs = addErrs(errs, ib.validateDisks(ii, s))
	errs = addErrs(errs, ib.validateMachineType(ii, s.w))
	errs = addErrs(errs, ii.validateNetworks(s))
	errs = addErrs(errs, ib.validateSourceMachineImage(ii, s))

	// Register creation. A zone-pending instance has no link to look up yet;
	// its name already carries the workflow ID.
	errs = addErrs(errs, s.w.instances.regCreate(ib.daisyName, &ib.Resource, ib.OverWrite || ib.zonePending(), s))
	return errs
}

// validatePlacementRegion replaces the zone checks for a PlacementRegion instance.
func (ib *InstanceBase) validatePlacementRegion(ctx context.Context, ii InstanceInterface, s *Step, pre string) DError {
	errs := ib.Resource.validateWithRegion(ctx, s, ib.PlacementRegion, pre)
	if ii.getZone() != "" {
		errs = addErrs(errs, Errf("%s: Zone and PlacementRegion are mutually exclusive", pre))
	}
	if i, ok := ii.(*Instance); !ok {
		errs = addErrs(errs, Errf("%s: PlacementRegion is not supported for InstanceBeta", pre))
	} else {
		for _, a := range i.GuestAccelerators {
			if a != nil && !rfc1035Rgx.MatchString(a.AcceleratorType) {
				errs = addErrs(errs, Errf("%s: bad GuestAccelerators.AcceleratorType: %q, PlacementRegion requires a short accelerator type name", pre, a.AcceleratorType))
			}
		}
	}
	if ib.OverWrite {
		errs = addErrs(errs, Errf("%s: OverWrite is not supported with PlacementRegion", pre))
	}
	return errs
}

func (ib *InstanceBase) validateSourceMachineImage(ii InstanceInterface, s *Step) DError {
	// regUse needs the partal url of a non daisy resource.
	lookup := ii.getSourceMachineImage()
	if lookup == "" {
		return nil
	}
	if _, err := s.w.machineImages.regUse(lookup, s); err != nil {
		return newErr("failed to register use of machine image when creating an instance", err)
	}
	return nil
}

type computeDisk struct {
	mode                string
	source              string
	hasInitializeParams bool
	diskName            string
	sourceImage         string
	autoDelete          bool
	diskType            string
}

func (i *Instance) getComputeDisks() []*computeDisk {
	var computeDisks []*computeDisk
	for _, d := range i.Disks {
		computeDisk := computeDisk{mode: d.Mode, source: d.Source, hasInitializeParams: d.InitializeParams != nil, autoDelete: d.AutoDelete}
		if computeDisk.hasInitializeParams {
			computeDisk.diskName = d.InitializeParams.DiskName
			computeDisk.sourceImage = d.InitializeParams.SourceImage
			computeDisk.diskType = d.InitializeParams.DiskType
		}
		computeDisks = append(computeDisks, &computeDisk)
	}
	return computeDisks
}

func (i *InstanceBeta) getComputeDisks() []*computeDisk {
	var computeDisks []*computeDisk
	for _, d := range i.Disks {
		computeDisk := computeDisk{mode: d.Mode, source: d.Source, hasInitializeParams: d.InitializeParams != nil, autoDelete: d.AutoDelete}
		if computeDisk.hasInitializeParams {
			computeDisk.diskName = d.InitializeParams.DiskName
			computeDisk.sourceImage = d.InitializeParams.SourceImage
			computeDisk.diskType = d.InitializeParams.DiskType
		}
		computeDisks = append(computeDisks, &computeDisk)
	}
	return computeDisks
}

func (ib *InstanceBase) validateSerialPortsToLog() (errs DError) {
	for _, port := range ib.SerialPortsToLog {
		if port < 0 || port > 4 {
			errs = addErrs(errs, Errf("cannot create instance: SerialPortsToLog must be between 1-4, inclusive"))
		}
	}
	return
}

func (ib *InstanceBase) validateDisks(ii InstanceInterface, s *Step) (errs DError) {
	computeDisks := ii.getComputeDisks()
	if len(computeDisks) == 0 && ii.getSourceMachineImage() == "" {
		errs = addErrs(errs, Errf("cannot create instance: no disks nor source machine image provided"))
	}
	if len(computeDisks) > 0 && ii.getSourceMachineImage() != "" {
		errs = addErrs(errs, Errf("cannot create instance: can't provide disks when SourceMachineImage provided"))
	}
	for _, d := range computeDisks {
		if !checkDiskMode(d.mode) {
			errs = addErrs(errs, Errf("cannot create instance: bad disk mode: %q", d.mode))
		}
		if d.source != "" && d.hasInitializeParams {
			errs = addErrs(errs, Errf("cannot create instance: disk.source and disk.initializeParams are mutually exclusive"))
		}
		if d.hasInitializeParams {
			errs = addErrs(errs, ib.validateDiskInitializeParams(d, ii, s))
		} else if ib.zonePending() {
			errs = addErrs(errs, Errf("cannot create instance %q: PlacementRegion requires disks to use InitializeParams, got disk.source %q", ib.daisyName, d.source))
		} else {
			errs = addErrs(errs, ib.validateDiskSource(d.source, ii, s))
		}
	}
	return
}

func (ib *InstanceBase) validateDiskInitializeParams(d *computeDisk, ii InstanceInterface, s *Step) (errs DError) {
	if ib.zonePending() {
		// Bulk insert takes a short disk type name; populate already detected
		// local SSD.
		if d.diskType == localSSDDiskType {
			return
		}
		if d.diskType != "" && !rfc1035Rgx.MatchString(d.diskType) {
			errs = addErrs(errs, Errf("cannot create instance: bad InitializeParams.DiskType: %q, PlacementRegion requires a short disk type name", d.diskType))
		}
	} else if d.diskType != "" {
		parts := NamedSubexp(diskTypeURLRgx, d.diskType)
		if parts["project"] != ib.Project {
			errs = addErrs(errs, Errf("cannot create instance in project %q with InitializeParams.DiskType in project %q", ib.Project, parts["project"]))
		}
		if parts["zone"] != ii.getZone() {
			errs = addErrs(errs, Errf("cannot create instance in zone %q with InitializeParams.DiskType in zone %q", ii.getZone(), parts["zone"]))
		}
		if parts["disktype"] == "local-ssd" {
			return
		}
	}

	if _, err := s.w.images.regUse(d.sourceImage, s); err != nil {
		errs = addErrs(errs, Errf("cannot create instance: can't use InitializeParams.SourceImage %q: %v", d.sourceImage, err))
	}
	if !rfc1035Rgx.MatchString(d.diskName) {
		errs = addErrs(errs, Errf("cannot create instance: bad InitializeParams.DiskName: %q", d.diskName))
	}
	link := fmt.Sprintf("projects/%s/zones/%s/disks/%s", ib.Project, ii.getZone(), d.diskName)
	if ib.zonePending() {
		// The link is set once the instance is created in its chosen zone.
		link = ""
	}
	// Set cleanup if not being autodeleted.
	r := &Resource{RealName: d.diskName, link: link, NoCleanup: d.autoDelete}
	errs = addErrs(errs, s.w.disks.regCreate(d.diskName, r, s, ib.zonePending()))

	return
}

func (ib *InstanceBase) validateDiskSource(diskSource string, ii InstanceInterface, s *Step) DError {
	dr, errs := s.w.disks.regUse(diskSource, s)
	if dr == nil {
		// Return now, the rest of this function can't be run without dr.
		return addErrs(errs, Errf("cannot create instance: disk %q not found in registry", diskSource))
	}

	// Ensure disk is in the same project and zone.
	result := NamedSubexp(diskURLRgx, dr.link)
	if result["project"] != ib.Project {
		errs = addErrs(errs, Errf("cannot create instance in project %q with disk in project %q: %q", ib.Project, result["project"], diskSource))
	}
	if result["zone"] != ii.getZone() {
		errs = addErrs(errs, Errf("cannot create instance in project %q with disk in zone %q: %q", ii.getZone(), result["zone"], diskSource))
	}
	return errs
}

func (ib *InstanceBase) validateMachineType(ii InstanceInterface, w *Workflow) (errs DError) {
	if ii.getSourceMachineImage() != "" && ii.getMachineType() == "" {
		return
	}

	if ib.zonePending() {
		// Bulk insert takes a short name; GCE checks that some zone offers it.
		if !rfc1035Rgx.MatchString(ii.getMachineType()) {
			errs = addErrs(errs, Errf("can't create instance: bad MachineType: %q, PlacementRegion requires a short machine type name", ii.getMachineType()))
		}
		return
	}

	if !machineTypeURLRegex.MatchString(ii.getMachineType()) {
		errs = addErrs(errs, Errf("can't create instance: bad MachineType: %q", ii.getMachineType()))
		return
	}

	result := NamedSubexp(machineTypeURLRegex, ii.getMachineType())
	if result["project"] != ib.Project {
		errs = addErrs(errs, Errf("cannot create instance in project %q with MachineType in project %q: %q", ib.Project, result["project"], ii.getMachineType()))
	}
	if result["zone"] != ii.getZone() {
		errs = addErrs(errs, Errf("cannot create instance in zone %q with MachineType in zone %q: %q", ii.getZone(), result["zone"], ii.getMachineType()))
	}

	if exists, err := w.machineTypeExists(result["project"], result["zone"], result["machinetype"]); err != nil {
		errs = addErrs(errs, Errf("cannot create instance, bad machineType lookup: %q, error: %v", result["machinetype"], err))
	} else if !exists {
		errs = addErrs(errs, Errf("cannot create instance, machineType does not exist: %q", result["machinetype"]))
	}
	return
}

func (i *Instance) validateNetworks(s *Step) (errs DError) {
	for _, n := range i.NetworkInterfaces {
		if n.Subnetwork != "" {
			_, err := s.w.subnetworks.regUse(n.Subnetwork, s)
			if err != nil {
				errs = addErrs(errs, err)
			}
		}

		if n.Network != "" {
			_, err := s.w.networks.regUse(n.Network, s)
			if err != nil {
				errs = addErrs(errs, err)
				continue
			}
		}
	}
	return
}

func (i *InstanceBeta) validateNetworks(s *Step) (errs DError) {
	for _, n := range i.NetworkInterfaces {
		if n.Subnetwork != "" {
			_, err := s.w.subnetworks.regUse(n.Subnetwork, s)
			if err != nil {
				errs = addErrs(errs, err)
			}
		}

		if n.Network != "" {
			_, err := s.w.networks.regUse(n.Network, s)
			if err != nil {
				errs = addErrs(errs, err)
				continue
			}
		}
	}
	return
}

type instanceRegistry struct {
	baseResourceRegistry
}

func newInstanceRegistry(w *Workflow) *instanceRegistry {
	ir := &instanceRegistry{baseResourceRegistry: baseResourceRegistry{w: w, typeName: "instance", urlRgx: instanceURLRgx}}
	ir.baseResourceRegistry.deleteFn = ir.deleteFn
	ir.baseResourceRegistry.startFn = ir.startFn
	ir.baseResourceRegistry.stopFn = ir.stopFn
	ir.init()
	return ir
}

// SleepFn function is mocked on testing.
var SleepFn = time.Sleep

func (ir *instanceRegistry) deleteFn(res *Resource) DError {
	m, dErr := zonalInstanceLink(res)
	if dErr != nil {
		return dErr
	}
	for i := 1; i < 4; i++ {
		if _, err := ir.w.ComputeClient.GetInstance(m["project"], m["zone"], m["instance"]); err != nil {
			// Can't remove an instance that was not even yet created!
			// However as the command was already submitted, wait.
			SleepFn((time.Duration(rand.Intn(1000))*time.Millisecond + 1*time.Second) * time.Duration(i))
			continue
		}
	}
	// Proceed to instance deletion
	err := ir.w.ComputeClient.DeleteInstance(m["project"], m["zone"], m["instance"])
	if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == http.StatusNotFound {
		return typedErr(resourceDNEError, "failed to delete instance", err)
	}
	return newErr("failed to delete instance", err)
}

func (ir *instanceRegistry) startFn(res *Resource) DError {
	m := NamedSubexp(instanceURLRgx, res.link)
	err := ir.w.ComputeClient.StartInstance(m["project"], m["zone"], m["instance"])
	if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == http.StatusNotFound {
		return typedErr(resourceDNEError, "failed to start instance", err)
	}
	return newErr("failed to start instance", err)
}

func (ir *instanceRegistry) stopFn(res *Resource) DError {
	m := NamedSubexp(instanceURLRgx, res.link)
	err := ir.w.ComputeClient.StopInstance(m["project"], m["zone"], m["instance"])
	if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == http.StatusNotFound {
		return typedErr(resourceDNEError, "failed to stop instance", err)
	}
	return newErr("failed to stop instance", err)
}

func (ir *instanceRegistry) regCreate(name string, res *Resource, skipExistenceCheck bool, s *Step) DError {
	// Base creation logic.
	errs := ir.baseResourceRegistry.regCreate(name, res, s, skipExistenceCheck)

	// Find the Instance responsible for this.
	for _, i := range (*s.CreateInstances).Instances {
		if &i.Resource == res {
			i.register(name, s, ir, errs)
			return errs
		}
	}
	for _, i := range (*s.CreateInstances).InstancesBeta {
		if &i.Resource == res {
			i.register(name, s, ir, errs)
			return errs
		}
	}

	return errs
}

func (ir *instanceRegistry) regDelete(name string, s *Step) DError {
	errs := ir.baseResourceRegistry.regDelete(name, s)
	errs = addErrs(errs, ir.w.disks.regDetachAll(name, s))
	return addErrs(errs, ir.w.networks.regDisconnectAll(name, s))
}

func deleteInstance(deleteDisk bool, cc daisyCompute.Client, project, zone, name string) error {
	if !deleteDisk {
		return cc.DeleteInstance(project, zone, name)
	}
	ci, err := cc.GetInstance(project, zone, name)
	if err != nil {
		return err
	}
	for _, cd := range ci.Disks {
		if !cd.AutoDelete {
			if err := cc.SetDiskAutoDelete(project, zone, name, true, cd.DeviceName); err != nil {
				return err
			}
		}
	}
	return cc.DeleteInstance(project, zone, name)
}
