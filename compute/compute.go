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

// Package compute provides access to the Google Compute API.
package compute

import (
	"context"
	"fmt"
	logging "log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	computeAlpha "google.golang.org/api/compute/v0.alpha"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
)

// Client is a client for interacting with Google Cloud Compute.
type Client interface {
	AttachDisk(project, zone, instance string, d *compute.AttachedDisk) error
	DetachDisk(project, zone, instance, disk string) error
	CreateDisk(project, zone string, d *compute.Disk) error
	CreateDiskAlpha(project, zone string, d *computeAlpha.Disk) error
	CreateDiskBeta(project, zone string, d *computeBeta.Disk) error
	CreateForwardingRule(project, region string, fr *compute.ForwardingRule) error
	CreateFirewallRule(project string, i *compute.Firewall) error
	CreateImage(project string, i *compute.Image) error
	CreateImageAlpha(project string, i *computeAlpha.Image) error
	CreateImageBeta(project string, i *computeBeta.Image) error
	CreateInstance(project, zone string, i *compute.Instance) error
	CreateInstanceAlpha(project, zone string, i *computeAlpha.Instance) error
	CreateInstanceBeta(project, zone string, i *computeBeta.Instance) error
	CreateNetwork(project string, n *compute.Network) error
	CreateSnapshot(project, zone, disk string, s *compute.Snapshot) error
	CreateSnapshotWithGuestFlush(project, zone, disk string, s *compute.Snapshot) error
	CreateSubnetwork(project, region string, n *compute.Subnetwork) error
	CreateTargetInstance(project, zone string, ti *compute.TargetInstance) error
	DeleteDisk(project, zone, name string) error
	DeleteForwardingRule(project, region, name string) error
	DeleteFirewallRule(project, name string) error
	DeleteImage(project, name string) error
	DeleteInstance(project, zone, name string) error
	StartInstance(project, zone, name string) error
	StopInstance(project, zone, name string) error
	DeleteNetwork(project, name string) error
	DeleteSubnetwork(project, region, name string) error
	DeleteTargetInstance(project, zone, name string) error
	DeprecateImage(project, name string, deprecationstatus *compute.DeprecationStatus) error
	DeprecateImageAlpha(project, name string, deprecationstatus *computeAlpha.DeprecationStatus) error
	GetMachineType(project, zone, machineType string) (*compute.MachineType, error)
	GetProject(project string) (*compute.Project, error)
	GetSerialPortOutput(project, zone, name string, port, start int64) (*compute.SerialPortOutput, error)
	GetZone(project, zone string) (*compute.Zone, error)
	GetInstance(project, zone, name string) (*compute.Instance, error)
	GetInstanceAlpha(project, zone, name string) (*computeAlpha.Instance, error)
	GetInstanceBeta(project, zone, name string) (*computeBeta.Instance, error)
	GetDisk(project, zone, name string) (*compute.Disk, error)
	GetDiskAlpha(project, zone, name string) (*computeAlpha.Disk, error)
	GetDiskBeta(project, zone, name string) (*computeBeta.Disk, error)
	GetForwardingRule(project, region, name string) (*compute.ForwardingRule, error)
	GetFirewallRule(project, name string) (*compute.Firewall, error)
	GetGuestAttributes(project, zone, name, queryPath, variableKey string) (*compute.GuestAttributes, error)
	GetImage(project, name string) (*compute.Image, error)
	GetImageAlpha(project, name string) (*computeAlpha.Image, error)
	GetImageBeta(project, name string) (*computeBeta.Image, error)
	GetImageFromFamily(project, family string) (*compute.Image, error)
	GetImageFromFamilyBeta(project, family string) (*computeBeta.Image, error)
	GetLicense(project, name string) (*compute.License, error)
	GetNetwork(project, name string) (*compute.Network, error)
	GetRegion(project, region string) (*compute.Region, error)
	GetSubnetwork(project, region, name string) (*compute.Subnetwork, error)
	GetTargetInstance(project, zone, name string) (*compute.TargetInstance, error)
	InstanceStatus(project, zone, name string) (string, error)
	InstanceStopped(project, zone, name string) (bool, error)
	ListMachineTypes(project, zone string, opts ...ListCallOption) ([]*compute.MachineType, error)
	ListLicenses(project string, opts ...ListCallOption) ([]*compute.License, error)
	ListZones(project string, opts ...ListCallOption) ([]*compute.Zone, error)
	ListRegions(project string, opts ...ListCallOption) ([]*compute.Region, error)
	AggregatedListInstances(project string, opts ...ListCallOption) ([]*compute.Instance, error)
	ListInstances(project, zone string, opts ...ListCallOption) ([]*compute.Instance, error)
	AggregatedListDisks(project string, opts ...ListCallOption) ([]*compute.Disk, error)
	ListDisks(project, zone string, opts ...ListCallOption) ([]*compute.Disk, error)
	AggregatedListForwardingRules(project string, opts ...ListCallOption) ([]*compute.ForwardingRule, error)
	ListForwardingRules(project, zone string, opts ...ListCallOption) ([]*compute.ForwardingRule, error)
	ListFirewallRules(project string, opts ...ListCallOption) ([]*compute.Firewall, error)
	ListImages(project string, opts ...ListCallOption) ([]*compute.Image, error)
	ListImagesAlpha(project string, opts ...ListCallOption) ([]*computeAlpha.Image, error)
	GetSnapshot(project, name string) (*compute.Snapshot, error)
	ListSnapshots(project string, opts ...ListCallOption) ([]*compute.Snapshot, error)
	DeleteSnapshot(project, name string) error
	ListNetworks(project string, opts ...ListCallOption) ([]*compute.Network, error)
	AggregatedListSubnetworks(project string, opts ...ListCallOption) ([]*compute.Subnetwork, error)
	ListSubnetworks(project, region string, opts ...ListCallOption) ([]*compute.Subnetwork, error)
	ListTargetInstances(project, zone string, opts ...ListCallOption) ([]*compute.TargetInstance, error)
	ResizeDisk(project, zone, disk string, drr *compute.DisksResizeRequest) error
	SetInstanceMetadata(project, zone, name string, md *compute.Metadata) error
	SetCommonInstanceMetadata(project string, md *compute.Metadata) error
	SetDiskAutoDelete(project, zone, instance string, autoDelete bool, deviceName string) error
	ListMachineImages(project string, opts ...ListCallOption) ([]*compute.MachineImage, error)
	DeleteMachineImage(project, name string) error
	CreateMachineImage(project string, i *compute.MachineImage) error
	GetMachineImage(project, name string) (*compute.MachineImage, error)
	Suspend(project, zone, instance string) error
	Resume(project, zone, instance string) error
	SimulateMaintenanceEvent(project, zone, instance string) error
	DeleteRegionTargetHTTPProxy(project, region, name string) error
	CreateRegionTargetHTTPProxy(project, region string, p *compute.TargetHttpProxy) error
	ListRegionTargetHTTPProxies(project, region string, opts ...ListCallOption) ([]*compute.TargetHttpProxy, error)
	GetRegionTargetHTTPProxy(project, region, name string) (*compute.TargetHttpProxy, error)
	DeleteRegionURLMap(project, region, name string) error
	CreateRegionURLMap(project, region string, u *compute.UrlMap) error
	ListRegionURLMaps(project, region string, opts ...ListCallOption) ([]*compute.UrlMap, error)
	GetRegionURLMap(project, region, name string) (*compute.UrlMap, error)
	DeleteRegionBackendService(project, region, name string) error
	CreateRegionBackendService(project, region string, b *compute.BackendService) error
	ListRegionBackendServices(project, region string, opts ...ListCallOption) ([]*compute.BackendService, error)
	GetRegionBackendService(project, region, name string) (*compute.BackendService, error)
	DeleteRegionHealthCheck(project, region, name string) error
	CreateRegionHealthCheck(project, region string, h *compute.HealthCheck) error
	ListRegionHealthChecks(project, region string, opts ...ListCallOption) ([]*compute.HealthCheck, error)
	GetRegionHealthCheck(project, region, name string) (*compute.HealthCheck, error)
	DeleteRegionNetworkEndpointGroup(project, region, name string) error
	CreateRegionNetworkEndpointGroup(project, region string, n *compute.NetworkEndpointGroup) error
	ListRegionNetworkEndpointGroups(project, region string, opts ...ListCallOption) ([]*compute.NetworkEndpointGroup, error)
	GetRegionNetworkEndpointGroup(project, region, name string) (*compute.NetworkEndpointGroup, error)

	Retry(f func(opts ...googleapi.CallOption) (*compute.Operation, error), opts ...googleapi.CallOption) (op *compute.Operation, err error)
	RetryBeta(f func(opts ...googleapi.CallOption) (*computeBeta.Operation, error), opts ...googleapi.CallOption) (op *computeBeta.Operation, err error)
	BasePath() string
}

// A ListCallOption is an option for a Google Compute API *ListCall.
type ListCallOption interface {
	listCallOptionApply(interface{}) interface{}
}

// OrderBy sets the optional parameter "orderBy": Sorts list results by a
// certain order. By default, results are returned in alphanumerical order
// based on the resource name.
type OrderBy string

func (o OrderBy) listCallOptionApply(i interface{}) interface{} {
	switch c := i.(type) {
	case *compute.FirewallsListCall:
		return c.OrderBy(string(o))
	case *computeAlpha.ImagesListCall:
		return c.OrderBy(string(o))
	case *compute.ImagesListCall:
		return c.OrderBy(string(o))
	case *computeAlpha.MachineImagesListCall:
		return c.OrderBy(string(o))
	case *computeBeta.MachineImagesListCall:
		return c.OrderBy(string(o))
	case *compute.MachineImagesListCall:
		return c.OrderBy(string(o))
	case *compute.MachineTypesListCall:
		return c.OrderBy(string(o))
	case *compute.ZonesListCall:
		return c.OrderBy(string(o))
	case *compute.InstancesListCall:
		return c.OrderBy(string(o))
	case *compute.DisksListCall:
		return c.OrderBy(string(o))
	case *compute.NetworksListCall:
		return c.OrderBy(string(o))
	case *compute.SubnetworksListCall:
		return c.OrderBy(string(o))
	case *compute.InstancesAggregatedListCall:
		return c.OrderBy(string(o))
	case *compute.DisksAggregatedListCall:
		return c.OrderBy(string(o))
	case *compute.SubnetworksAggregatedListCall:
		return c.OrderBy(string(o))
	}
	return i
}

// Filter sets the optional parameter "filter": Sets a filter {expression} for
// filtering listed resources. Your {expression} must be in the format:
// field_name comparison_string literal_string.
type Filter string

func (o Filter) listCallOptionApply(i interface{}) interface{} {
	switch c := i.(type) {
	case *compute.FirewallsListCall:
		return c.Filter(string(o))
	case *computeAlpha.ImagesListCall:
		return c.Filter(string(o))
	case *compute.ImagesListCall:
		return c.Filter(string(o))
	case *computeAlpha.MachineImagesListCall:
		return c.Filter(string(o))
	case *computeBeta.MachineImagesListCall:
		return c.Filter(string(o))
	case *compute.MachineImagesListCall:
		return c.Filter(string(o))
	case *compute.MachineTypesListCall:
		return c.Filter(string(o))
	case *compute.ZonesListCall:
		return c.Filter(string(o))
	case *compute.InstancesListCall:
		return c.Filter(string(o))
	case *compute.DisksListCall:
		return c.Filter(string(o))
	case *compute.NetworksListCall:
		return c.Filter(string(o))
	case *compute.SubnetworksListCall:
		return c.Filter(string(o))
	case *compute.InstancesAggregatedListCall:
		return c.Filter(string(o))
	case *compute.DisksAggregatedListCall:
		return c.Filter(string(o))
	case *compute.SubnetworksAggregatedListCall:
		return c.Filter(string(o))
	}
	return i
}

type clientImpl interface {
	Client
	zoneOperationsWait(project, zone, name string) error
	regionOperationsWait(project, region, name string) error
	globalOperationsWait(project, name string) error
}

type client struct {
	i        clientImpl
	hc       *http.Client
	raw      *compute.Service
	rawBeta  *computeBeta.Service
	rawAlpha *computeAlpha.Service
}

// shouldRetryWithWait returns true if the HTTP response / error indicates
// that the request should be attempted again.
func shouldRetryWithWait(tripper http.RoundTripper, err error, multiplier int) bool {
	if err == nil {
		return false
	}
	tkValid := true
	trans, ok := tripper.(*oauth2.Transport)
	if ok {
		if tk, err := trans.Source.Token(); err == nil {
			tkValid = tk.Valid()
		}
	}

	apiErr, ok := err.(*googleapi.Error)
	var retry bool
	switch {
	case !ok && (strings.Contains(err.Error(), "connection reset by peer") || strings.Contains(err.Error(), "unexpected EOF")):
		retry = true
	case !ok && (strings.Contains(err.Error(), "server sent GOAWAY") || strings.Contains(err.Error(), "ENHANCE_YOUR_CALM")):
		// The wait operation can return GOAWAY/ENHANCE_YOUR_CALM messages, so doubling the wait multiplier as it based on the retry count.
		multiplier = multiplier * 2
		retry = true
	case !ok && tkValid:
		// Not a googleapi.Error and the token is still valid.
		return false
	case apiErr.Code >= 500 && apiErr.Code <= 599:
		retry = true
	case apiErr.Code >= 429:
		// Too many API requests.
		retry = true
	case apiErr.Code == 403 && strings.Contains(err.Error(), "rateLimitExceeded"):
		// Quota errors are reported as 403.
		// Generally we don't want to retry on quota errors, but if it's quota on rate (GetSerialPortOutput) - we should.
		retry = true
	case !tkValid:
		// This was probably a failure to get new token from metadata server.
		retry = true
	}
	if !retry {
		return false
	}

	sleep := (time.Duration(rand.Intn(1000))*time.Millisecond + 1*time.Second) * time.Duration(multiplier)
	time.Sleep(sleep)
	return true
}

// NewClient creates a new Google Cloud Compute client.
func NewClient(ctx context.Context, opts ...option.ClientOption) (Client, error) {
	// Set these scopes to be align with compute.NewService
	o := []option.ClientOption{
		option.WithScopes(
			compute.CloudPlatformScope,
			compute.ComputeScope,
			compute.ComputeReadonlyScope,
			compute.DevstorageFullControlScope,
			compute.DevstorageReadOnlyScope,
			compute.DevstorageReadWriteScope,
		),
	}
	opts = append(o, opts...)
	hc, ep, err := transport.NewHTTPClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating HTTP API client: %v", err)
	}
	rawService, err := compute.New(hc)
	if err != nil {
		return nil, fmt.Errorf("compute client: %v", err)
	}
	if ep != "" {
		rawService.BasePath = ep
	}
	rawBetaService, err := computeBeta.New(hc)
	if err != nil {
		return nil, fmt.Errorf("beta compute client: %v", err)
	}
	if ep != "" {
		rawBetaService.BasePath = ep
	}
	rawAlphaService, err := computeAlpha.New(hc)
	if err != nil {
		return nil, fmt.Errorf("alpha compute client: %v", err)
	}
	if ep != "" {
		rawAlphaService.BasePath = ep
	}

	c := &client{hc: hc, raw: rawService, rawBeta: rawBetaService, rawAlpha: rawAlphaService}
	c.i = c

	return c, nil
}

// BasePath returns the base path for this client.
func (c *client) BasePath() string {
	return c.raw.BasePath
}

type operationGetterFunc func() (*compute.Operation, error)

func (c *client) zoneOperationsWait(project, zone, name string) error {
	return c.operationsWaitHelper(project, name, func() (op *compute.Operation, err error) {
		op, err = c.Retry(c.raw.ZoneOperations.Wait(project, zone, name).Do)
		if err != nil {
			err = fmt.Errorf("failed to get zone operation %s: %v", name, err)
		}
		return op, err
	})
}

func (c *client) regionOperationsWait(project, region, name string) error {
	return c.operationsWaitHelper(project, name, func() (op *compute.Operation, err error) {
		op, err = c.Retry(c.raw.RegionOperations.Wait(project, region, name).Do)
		if err != nil {
			err = fmt.Errorf("failed to get region operation %s: %v", name, err)
		}
		return op, err
	})
}

func (c *client) globalOperationsWait(project, name string) error {
	return c.operationsWaitHelper(project, name, func() (op *compute.Operation, err error) {
		op, err = c.Retry(c.raw.GlobalOperations.Wait(project, name).Do)
		if err != nil {
			err = fmt.Errorf("failed to get global operation %s: %v", name, err)
		}
		return op, err
	})
}

// OperationErrorCodeFormat is the format of operation error code.
var OperationErrorCodeFormat = "Code: %s"

var operationErrorMessageFormat = "Message: %s"

func (c *client) operationsWaitHelper(project, name string, getOperation operationGetterFunc) error {
	for {
		op, err := getOperation()
		if err != nil {
			return err
		}

		switch op.Status {
		case "PENDING", "RUNNING":
			time.Sleep(1 * time.Second)
			continue
		case "DONE":
			if op.Error != nil {
				var operrs string
				for _, operr := range op.Error.Errors {
					operrs = operrs + fmt.Sprintf(
						fmt.Sprintf("\n%v\n%v", OperationErrorCodeFormat, operationErrorMessageFormat),
						operr.Code, operr.Message)
				}
				return fmt.Errorf("operation failed %+v: %s", op, operrs)
			}
		default:
			return fmt.Errorf("unknown operation status %q: %+v", op.Status, op)
		}
		return nil
	}
}

// Retry invokes the given function, retrying it multiple times if the HTTP
// status response indicates the request should be attempted again or the
// oauth Token is no longer valid.
func (c *client) Retry(f func(opts ...googleapi.CallOption) (*compute.Operation, error), opts ...googleapi.CallOption) (op *compute.Operation, err error) {
	for i := 1; i < 4; i++ {
		op, err = f(opts...)
		if err == nil {
			return op, nil
		}
		if !shouldRetryWithWait(c.hc.Transport, err, i) {
			return nil, err
		}
	}
	return
}

// RetryBeta invokes the given function, retrying it multiple times if the HTTP
// status response indicates the request should be attempted again or the
// oauth Token is no longer valid.
func (c *client) RetryBeta(f func(opts ...googleapi.CallOption) (*computeBeta.Operation, error), opts ...googleapi.CallOption) (op *computeBeta.Operation, err error) {
	for i := 1; i < 4; i++ {
		op, err = f(opts...)
		if err == nil {
			return op, nil
		}
		if !shouldRetryWithWait(c.hc.Transport, err, i) {
			return nil, err
		}
	}
	return
}

// RetryAlpha invokes the given function, retrying it multiple times if the HTTP
// status response indicates the request should be attempted again or the
// oauth Token is no longer valid.
func (c *client) RetryAlpha(f func(opts ...googleapi.CallOption) (*computeAlpha.Operation, error), opts ...googleapi.CallOption) (op *computeAlpha.Operation, err error) {
	for i := 1; i < 4; i++ {
		op, err = f(opts...)
		if err == nil {
			return op, nil
		}
		if !shouldRetryWithWait(c.hc.Transport, err, i) {
			return nil, err
		}
	}
	return
}

// AttachDisk attaches a GCE persistent disk to an instance.
func (c *client) AttachDisk(project, zone, instance string, d *compute.AttachedDisk) error {
	op, err := c.Retry(c.raw.Instances.AttachDisk(project, zone, instance, d).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// DetachDisk detaches a GCE persistent disk to an instance.
func (c *client) DetachDisk(project, zone, instance, disk string) error {
	op, err := c.Retry(c.raw.Instances.DetachDisk(project, zone, instance, disk).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// CreateDisk creates a GCE persistent disk.
func (c *client) CreateDisk(project, zone string, d *compute.Disk) error {
	op, err := c.Retry(c.raw.Disks.Insert(project, zone, d).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdDisk *compute.Disk
	if createdDisk, err = c.i.GetDisk(project, zone, d.Name); err != nil {
		return err
	}
	*d = *createdDisk
	return nil
}

// CreateDiskAlpha creates a GCE persistent disk.
func (c *client) CreateDiskAlpha(project, zone string, d *computeAlpha.Disk) error {
	op, err := c.RetryAlpha(c.rawAlpha.Disks.Insert(project, zone, d).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdDisk *computeAlpha.Disk
	if createdDisk, err = c.i.GetDiskAlpha(project, zone, d.Name); err != nil {
		return err
	}
	*d = *createdDisk
	return nil
}

// CreateDiskBeta creates a GCE persistent disk.
func (c *client) CreateDiskBeta(project, zone string, d *computeBeta.Disk) error {
	op, err := c.RetryBeta(c.rawBeta.Disks.Insert(project, zone, d).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdDisk *computeBeta.Disk
	if createdDisk, err = c.i.GetDiskBeta(project, zone, d.Name); err != nil {
		return err
	}
	*d = *createdDisk
	return nil
}

// CreateForwardingRule creates a GCE forwarding rule.
func (c *client) CreateForwardingRule(project, region string, fr *compute.ForwardingRule) error {
	op, err := c.Retry(c.raw.ForwardingRules.Insert(project, region, fr).Do)
	if err != nil {
		return err
	}

	if err := c.i.regionOperationsWait(project, region, op.Name); err != nil {
		return err
	}

	var createdForwardingRule *compute.ForwardingRule
	if createdForwardingRule, err = c.i.GetForwardingRule(project, region, fr.Name); err != nil {
		return err
	}
	*fr = *createdForwardingRule
	return nil
}

func (c *client) CreateFirewallRule(project string, i *compute.Firewall) error {
	op, err := c.Retry(c.raw.Firewalls.Insert(project, i).Do)
	if err != nil {
		return err
	}

	if err := c.i.globalOperationsWait(project, op.Name); err != nil {
		return err
	}

	var createdFirewallRule *compute.Firewall
	if createdFirewallRule, err = c.i.GetFirewallRule(project, i.Name); err != nil {
		return err
	}
	*i = *createdFirewallRule
	return nil
}

// CreateImage creates a GCE image.
// Only one of sourceDisk or sourceFile must be specified, sourceDisk is the
// url (full or partial) to the source disk, sourceFile is the full Google
// Cloud Storage URL where the disk image is stored.
func (c *client) CreateImage(project string, i *compute.Image) error {
	op, err := c.Retry(c.raw.Images.Insert(project, i).Do)
	if err != nil {
		return err
	}

	if err := c.i.globalOperationsWait(project, op.Name); err != nil {
		return err
	}

	var createdImage *compute.Image
	if createdImage, err = c.i.GetImage(project, i.Name); err != nil {
		return err
	}
	*i = *createdImage
	return nil
}

// CreateImageBeta creates a GCE image using Beta API.
// Only one of sourceDisk or sourceFile must be specified, sourceDisk is the
// url (full or partial) to the source disk, sourceFile is the full Google
// Cloud Storage URL where the disk image is stored.
func (c *client) CreateImageBeta(project string, i *computeBeta.Image) error {
	op, err := c.RetryBeta(c.rawBeta.Images.Insert(project, i).Do)
	if err != nil {
		return err
	}

	if err := c.i.globalOperationsWait(project, op.Name); err != nil {
		return err
	}

	var createdImage *computeBeta.Image
	if createdImage, err = c.i.GetImageBeta(project, i.Name); err != nil {
		return err
	}
	*i = *createdImage
	return nil
}

// CreateImageAlpha creates a GCE image using Alpha API.
// Only one of sourceDisk or sourceFile must be specified, sourceDisk is the
// url (full or partial) to the source disk, sourceFile is the full Google
// Cloud Storage URL where the disk image is stored.
func (c *client) CreateImageAlpha(project string, i *computeAlpha.Image) error {
	op, err := c.RetryAlpha(c.rawAlpha.Images.Insert(project, i).Do)
	if err != nil {
		return err
	}

	if err := c.i.globalOperationsWait(project, op.Name); err != nil {
		return err
	}

	var createdImage *computeAlpha.Image
	if createdImage, err = c.i.GetImageAlpha(project, i.Name); err != nil {
		return err
	}
	*i = *createdImage
	return nil
}

// DeleteRegionTargetHTTPProxy deletes a GCE RegionTargetHTTPProxy.
func (c *client) DeleteRegionTargetHTTPProxy(project, region, name string) error {
	op, err := c.Retry(c.raw.RegionTargetHttpProxies.Delete(project, region, name).Do)
	if err != nil {
		return err
	}
	return c.i.regionOperationsWait(project, region, op.Name)
}

// CreateRegionTargetHTTPProxy creates a GCE RegionTargetHTTPProxy.
func (c *client) CreateRegionTargetHTTPProxy(project, region string, p *compute.TargetHttpProxy) error {
	op, err := c.Retry(c.raw.RegionTargetHttpProxies.Insert(project, region, p).Do)
	if err != nil {
		return err
	}
	if err := c.i.regionOperationsWait(project, region, op.Name); err != nil {
		return err
	}
	var createdRegionTargetHTTPProxy *compute.TargetHttpProxy
	if createdRegionTargetHTTPProxy, err = c.i.GetRegionTargetHTTPProxy(project, region, p.Name); err != nil {
		return err
	}
	*p = *createdRegionTargetHTTPProxy
	return nil
}

// GetRegionTargetHTTPProxy gets a GCE RegionTargetHTTPProxy.
func (c *client) GetRegionTargetHTTPProxy(project, region, name string) (*compute.TargetHttpProxy, error) {
	i, err := c.raw.RegionTargetHttpProxies.Get(project, region, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.RegionTargetHttpProxies.Get(project, region, name).Do()
	}
	return i, err
}

// ListRegionTargetHTTPProxies lists GCE RegionTargetHTTPProxies.
func (c *client) ListRegionTargetHTTPProxies(project, region string, opts ...ListCallOption) ([]*compute.TargetHttpProxy, error) {
	var is []*compute.TargetHttpProxy
	var pt string
	call := c.raw.RegionTargetHttpProxies.List(project, region)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.RegionTargetHttpProxiesListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// DeleteRegionBackendService deletes a GCE RegionBackendService.
func (c *client) DeleteRegionBackendService(project, region, name string) error {
	op, err := c.Retry(c.raw.RegionBackendServices.Delete(project, region, name).Do)
	if err != nil {
		return err
	}
	return c.i.regionOperationsWait(project, region, op.Name)
}

// CreateRegionBackendService creates a GCE RegionBackendService.
func (c *client) CreateRegionBackendService(project, region string, p *compute.BackendService) error {
	op, err := c.Retry(c.raw.RegionBackendServices.Insert(project, region, p).Do)
	if err != nil {
		return err
	}
	if err := c.i.regionOperationsWait(project, region, op.Name); err != nil {
		return err
	}
	var createdRegionBackendService *compute.BackendService
	if createdRegionBackendService, err = c.i.GetRegionBackendService(project, region, p.Name); err != nil {
		return err
	}
	*p = *createdRegionBackendService
	return nil
}

// GetRegionBackendService gets a GCE RegionBackendService.
func (c *client) GetRegionBackendService(project, region, name string) (*compute.BackendService, error) {
	i, err := c.raw.RegionBackendServices.Get(project, region, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.RegionBackendServices.Get(project, region, name).Do()
	}
	return i, err
}

// ListRegionBackendServices lists GCE RegionBackendServices.
func (c *client) ListRegionBackendServices(project, region string, opts ...ListCallOption) ([]*compute.BackendService, error) {
	var is []*compute.BackendService
	var pt string
	call := c.raw.RegionBackendServices.List(project, region)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.RegionBackendServicesListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// DeleteRegionURLMap deletes a GCE RegionURLMap.
func (c *client) DeleteRegionURLMap(project, region, name string) error {
	op, err := c.Retry(c.raw.RegionUrlMaps.Delete(project, region, name).Do)
	if err != nil {
		return err
	}
	return c.i.regionOperationsWait(project, region, op.Name)
}

// CreateRegionURLMap creates a GCE RegionURLMap.
func (c *client) CreateRegionURLMap(project, region string, p *compute.UrlMap) error {
	op, err := c.Retry(c.raw.RegionUrlMaps.Insert(project, region, p).Do)
	if err != nil {
		return err
	}
	if err := c.i.regionOperationsWait(project, region, op.Name); err != nil {
		return err
	}
	var createdRegionURLMap *compute.UrlMap
	if createdRegionURLMap, err = c.i.GetRegionURLMap(project, region, p.Name); err != nil {
		return err
	}
	*p = *createdRegionURLMap
	return nil
}

// GetRegionURLMap gets a GCE RegionURLMap.
func (c *client) GetRegionURLMap(project, region, name string) (*compute.UrlMap, error) {
	i, err := c.raw.RegionUrlMaps.Get(project, region, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.RegionUrlMaps.Get(project, region, name).Do()
	}
	return i, err
}

// ListRegionURLMaps lists GCE RegionURLMaps.
func (c *client) ListRegionURLMaps(project, region string, opts ...ListCallOption) ([]*compute.UrlMap, error) {
	var is []*compute.UrlMap
	var pt string
	call := c.raw.RegionUrlMaps.List(project, region)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.RegionUrlMapsListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// DeleteRegionHealthCheck deletes a GCE RegionHealthCheck.
func (c *client) DeleteRegionHealthCheck(project, region, name string) error {
	op, err := c.Retry(c.raw.RegionHealthChecks.Delete(project, region, name).Do)
	if err != nil {
		return err
	}
	return c.i.regionOperationsWait(project, region, op.Name)
}

// CreateRegionHealthCheck creates a GCE RegionHealthCheck.
func (c *client) CreateRegionHealthCheck(project, region string, p *compute.HealthCheck) error {
	op, err := c.Retry(c.raw.RegionHealthChecks.Insert(project, region, p).Do)
	if err != nil {
		return err
	}
	if err := c.i.regionOperationsWait(project, region, op.Name); err != nil {
		return err
	}
	var createdRegionHealthCheck *compute.HealthCheck
	if createdRegionHealthCheck, err = c.i.GetRegionHealthCheck(project, region, p.Name); err != nil {
		return err
	}
	*p = *createdRegionHealthCheck
	return nil
}

// GetRegionHealthCheck gets a GCE RegionHealthCheck.
func (c *client) GetRegionHealthCheck(project, region, name string) (*compute.HealthCheck, error) {
	i, err := c.raw.RegionHealthChecks.Get(project, region, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.RegionHealthChecks.Get(project, region, name).Do()
	}
	return i, err
}

// ListRegionHealthChecks lists GCE RegionHealthChecks.
func (c *client) ListRegionHealthChecks(project, region string, opts ...ListCallOption) ([]*compute.HealthCheck, error) {
	var is []*compute.HealthCheck
	var pt string
	call := c.raw.RegionHealthChecks.List(project, region)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.RegionHealthChecksListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// DeleteRegionNetworkEndpointGroup deletes a GCE RegionNetworkEndpointGroup.
func (c *client) DeleteRegionNetworkEndpointGroup(project, region, name string) error {
	op, err := c.Retry(c.raw.RegionNetworkEndpointGroups.Delete(project, region, name).Do)
	if err != nil {
		return err
	}
	return c.i.regionOperationsWait(project, region, op.Name)
}

// CreateRegionNetworkEndpointGroup creates a GCE RegionNetworkEndpointGroup.
func (c *client) CreateRegionNetworkEndpointGroup(project, region string, p *compute.NetworkEndpointGroup) error {
	op, err := c.Retry(c.raw.RegionNetworkEndpointGroups.Insert(project, region, p).Do)
	if err != nil {
		return err
	}
	if err := c.i.regionOperationsWait(project, region, op.Name); err != nil {
		return err
	}
	var createdRegionNetworkEndpointGroup *compute.NetworkEndpointGroup
	if createdRegionNetworkEndpointGroup, err = c.i.GetRegionNetworkEndpointGroup(project, region, p.Name); err != nil {
		return err
	}
	*p = *createdRegionNetworkEndpointGroup
	return nil
}

// GetRegionNetworkEndpointGroup gets a GCE RegionNetworkEndpointGroup.
func (c *client) GetRegionNetworkEndpointGroup(project, region, name string) (*compute.NetworkEndpointGroup, error) {
	i, err := c.raw.RegionNetworkEndpointGroups.Get(project, region, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.RegionNetworkEndpointGroups.Get(project, region, name).Do()
	}
	return i, err
}

// ListRegionNetworkEndpointGroups lists GCE RegionNetworkEndpointGroups.
func (c *client) ListRegionNetworkEndpointGroups(project, region string, opts ...ListCallOption) ([]*compute.NetworkEndpointGroup, error) {
	var is []*compute.NetworkEndpointGroup
	var pt string
	call := c.raw.RegionNetworkEndpointGroups.List(project, region)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.RegionNetworkEndpointGroupsListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

func (c *client) CreateInstance(project, zone string, i *compute.Instance) error {
	op, err := c.Retry(c.raw.Instances.Insert(project, zone, i).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdInstance *compute.Instance
	if createdInstance, err = c.i.GetInstance(project, zone, i.Name); err != nil {
		return err
	}
	*i = *createdInstance
	return nil
}

// CreateInstanceAlpha creates a GCE image using Alpha API.
func (c *client) CreateInstanceAlpha(project, zone string, i *computeAlpha.Instance) error {
	op, err := c.RetryAlpha(c.rawAlpha.Instances.Insert(project, zone, i).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdInstance *computeAlpha.Instance
	if createdInstance, err = c.i.GetInstanceAlpha(project, zone, i.Name); err != nil {
		return err
	}
	*i = *createdInstance
	return nil
}

// CreateInstanceBeta creates a GCE image using Beta API.
func (c *client) CreateInstanceBeta(project, zone string, i *computeBeta.Instance) error {
	op, err := c.RetryBeta(c.rawBeta.Instances.Insert(project, zone, i).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdInstance *computeBeta.Instance
	if createdInstance, err = c.i.GetInstanceBeta(project, zone, i.Name); err != nil {
		return err
	}
	*i = *createdInstance
	return nil
}

func (c *client) CreateNetwork(project string, n *compute.Network) error {
	op, err := c.Retry(c.raw.Networks.Insert(project, n).Do)
	if err != nil {
		return err
	}

	if err := c.i.globalOperationsWait(project, op.Name); err != nil {
		return err
	}

	var createdNetwork *compute.Network
	if createdNetwork, err = c.i.GetNetwork(project, n.Name); err != nil {
		return err
	}
	*n = *createdNetwork
	return nil
}

func (c *client) CreateSubnetwork(project, region string, n *compute.Subnetwork) error {
	op, err := c.Retry(c.raw.Subnetworks.Insert(project, region, n).Do)
	if err != nil {
		return err
	}

	if err := c.i.regionOperationsWait(project, region, op.Name); err != nil {
		return err
	}

	var createdSubnetwork *compute.Subnetwork
	if createdSubnetwork, err = c.i.GetSubnetwork(project, region, n.Name); err != nil {
		return err
	}
	*n = *createdSubnetwork
	return nil
}

// CreateTargetInstance creates a GCE Target Instance, which can be used as
// target on ForwardingRule
func (c *client) CreateTargetInstance(project, zone string, ti *compute.TargetInstance) error {
	op, err := c.Retry(c.raw.TargetInstances.Insert(project, zone, ti).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdTargetInstance *compute.TargetInstance
	if createdTargetInstance, err = c.i.GetTargetInstance(project, zone, ti.Name); err != nil {
		return err
	}
	*ti = *createdTargetInstance
	return nil
}

// DeleteFirewallRule deletes a GCE FirewallRule.
func (c *client) DeleteFirewallRule(project, name string) error {
	op, err := c.Retry(c.raw.Firewalls.Delete(project, name).Do)
	if err != nil {
		return err
	}

	return c.i.globalOperationsWait(project, op.Name)
}

// DeleteImage deletes a GCE image.
func (c *client) DeleteImage(project, name string) error {
	op, err := c.Retry(c.raw.Images.Delete(project, name).Do)
	if err != nil {
		return err
	}

	return c.i.globalOperationsWait(project, op.Name)
}

// DeleteDisk deletes a GCE persistent disk.
func (c *client) DeleteDisk(project, zone, name string) error {
	op, err := c.Retry(c.raw.Disks.Delete(project, zone, name).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// SetDiskAutoDelete set auto-delete of an attached disk
func (c *client) SetDiskAutoDelete(project, zone, instance string, autoDelete bool, deviceName string) error {
	op, err := c.Retry(c.raw.Instances.SetDiskAutoDelete(project, zone, instance, autoDelete, deviceName).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// DeleteForwardingRule deletes a GCE ForwardingRule.
func (c *client) DeleteForwardingRule(project, region, name string) error {
	op, err := c.Retry(c.raw.ForwardingRules.Delete(project, region, name).Do)
	if err != nil {
		return err
	}

	return c.i.regionOperationsWait(project, region, op.Name)
}

// DeleteInstance deletes a GCE instance.
func (c *client) DeleteInstance(project, zone, name string) error {
	op, err := c.Retry(c.raw.Instances.Delete(project, zone, name).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// StartInstance starts a GCE instance.
func (c *client) StartInstance(project, zone, name string) error {
	op, err := c.Retry(c.raw.Instances.Start(project, zone, name).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// StopInstance stops a GCE instance.
func (c *client) StopInstance(project, zone, name string) error {
	op, err := c.Retry(c.raw.Instances.Stop(project, zone, name).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// DeleteNetwork deletes a GCE network.
func (c *client) DeleteNetwork(project, name string) error {
	op, err := c.Retry(c.raw.Networks.Delete(project, name).Do)
	if err != nil {
		return err
	}

	return c.i.globalOperationsWait(project, op.Name)
}

// DeleteSubnetwork deletes a GCE subnetwork.
func (c *client) DeleteSubnetwork(project, region, name string) error {
	op, err := c.Retry(c.raw.Subnetworks.Delete(project, region, name).Do)
	if err != nil {
		return err
	}

	return c.i.regionOperationsWait(project, region, op.Name)
}

// DeleteTargetInstance deletes a GCE TargetInstance.
func (c *client) DeleteTargetInstance(project, zone, name string) error {
	op, err := c.Retry(c.raw.TargetInstances.Delete(project, zone, name).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// DeprecateImage sets deprecation status on a GCE image.
func (c *client) DeprecateImage(project, name string, deprecationstatus *compute.DeprecationStatus) error {
	op, err := c.Retry(c.raw.Images.Deprecate(project, name, deprecationstatus).Do)
	if err != nil {
		return err
	}

	return c.i.globalOperationsWait(project, op.Name)
}

// DeprecateImageAlpha sets deprecation status on a GCE image using the Alpha API.
func (c *client) DeprecateImageAlpha(project, name string, deprecationstatus *computeAlpha.DeprecationStatus) error {
	op, err := c.RetryAlpha(c.rawAlpha.Images.Deprecate(project, name, deprecationstatus).Do)
	if err != nil {
		return err
	}
	return c.i.globalOperationsWait(project, op.Name)
}

// GetMachineType gets a GCE MachineType.
func (c *client) GetMachineType(project, zone, machineType string) (*compute.MachineType, error) {
	mt, err := c.raw.MachineTypes.Get(project, zone, machineType).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.MachineTypes.Get(project, zone, machineType).Do()
	}
	return mt, err
}

// ListMachineTypes gets a list of GCE MachineTypes.
func (c *client) ListMachineTypes(project, zone string, opts ...ListCallOption) ([]*compute.MachineType, error) {
	var mts []*compute.MachineType
	var pt string
	call := c.raw.MachineTypes.List(project, zone)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.MachineTypesListCall)
	}
	for mtl, err := call.PageToken(pt).Do(); ; mtl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			mtl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		mts = append(mts, mtl.Items...)

		if mtl.NextPageToken == "" {
			return mts, nil
		}
		pt = mtl.NextPageToken
	}
}

// GetProject gets a GCE Project.
func (c *client) GetProject(project string) (*compute.Project, error) {
	p, err := c.raw.Projects.Get(project).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Projects.Get(project).Do()
	}
	return p, err
}

// GetSerialPortOutput gets the serial port output of a GCE instance.
func (c *client) GetSerialPortOutput(project, zone, name string, port, start int64) (*compute.SerialPortOutput, error) {
	sp, err := c.raw.Instances.GetSerialPortOutput(project, zone, name).Start(start).Port(port).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Instances.GetSerialPortOutput(project, zone, name).Start(start).Port(port).Do()
	}
	return sp, err
}

// GetZone gets a GCE Zone.
func (c *client) GetZone(project, zone string) (*compute.Zone, error) {
	z, err := c.raw.Zones.Get(project, zone).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Zones.Get(project, zone).Do()
	}
	return z, err
}

// ListZones gets a list GCE Zones.
func (c *client) ListZones(project string, opts ...ListCallOption) ([]*compute.Zone, error) {
	var zs []*compute.Zone
	var pt string
	call := c.raw.Zones.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.ZonesListCall)
	}
	for zl, err := call.PageToken(pt).Do(); ; zl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			zl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		zs = append(zs, zl.Items...)

		if zl.NextPageToken == "" {
			return zs, nil
		}
		pt = zl.NextPageToken
	}
}

// ListRegions gets a list GCE Regions.
func (c *client) ListRegions(project string, opts ...ListCallOption) ([]*compute.Region, error) {
	var rs []*compute.Region
	var pt string
	call := c.raw.Regions.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.RegionsListCall)
	}
	for rl, err := call.PageToken(pt).Do(); ; rl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			rl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		rs = append(rs, rl.Items...)

		if rl.NextPageToken == "" {
			return rs, nil
		}
		pt = rl.NextPageToken
	}
}

// GetInstance gets a GCE Instance using GA API.
func (c *client) GetInstance(project, zone, name string) (*compute.Instance, error) {
	i, err := c.raw.Instances.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Instances.Get(project, zone, name).Do()
	}
	return i, err
}

// GetInstanceAlpha gets a GCE Instance using Alpha API.
func (c *client) GetInstanceAlpha(project, zone, name string) (*computeAlpha.Instance, error) {
	i, err := c.rawAlpha.Instances.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.rawAlpha.Instances.Get(project, zone, name).Do()
	}
	return i, err
}

// GetInstanceBeta gets a GCE Instance using Beta API.
func (c *client) GetInstanceBeta(project, zone, name string) (*computeBeta.Instance, error) {
	i, err := c.rawBeta.Instances.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.rawBeta.Instances.Get(project, zone, name).Do()
	}
	return i, err
}

// AggregatedListInstances gets an aggregated list of GCE Instances.
func (c *client) AggregatedListInstances(project string, opts ...ListCallOption) ([]*compute.Instance, error) {
	var is []*compute.Instance
	var pt string
	call := c.raw.Instances.AggregatedList(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.InstancesAggregatedListCall)
	}
	for ial, err := call.PageToken(pt).Do(); ; ial, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			ial, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		for _, isl := range ial.Items {
			is = append(is, isl.Instances...)
		}
		if ial.NextPageToken == "" {
			return is, nil
		}
		pt = ial.NextPageToken
	}
}

// ListInstances gets a list of GCE Instances.
func (c *client) ListInstances(project, zone string, opts ...ListCallOption) ([]*compute.Instance, error) {
	var is []*compute.Instance
	var pt string
	call := c.raw.Instances.List(project, zone)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.InstancesListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// GetDisk gets a GCE Disk.
func (c *client) GetDisk(project, zone, name string) (*compute.Disk, error) {
	d, err := c.raw.Disks.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Disks.Get(project, zone, name).Do()
	}
	return d, err
}

// GetDiskAlpha gets a GCE Disk.
func (c *client) GetDiskAlpha(project, zone, name string) (*computeAlpha.Disk, error) {
	d, err := c.rawAlpha.Disks.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.rawAlpha.Disks.Get(project, zone, name).Do()
	}
	return d, err
}

// GetDiskBeta gets a GCE Disk.
func (c *client) GetDiskBeta(project, zone, name string) (*computeBeta.Disk, error) {
	d, err := c.rawBeta.Disks.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.rawBeta.Disks.Get(project, zone, name).Do()
	}
	return d, err
}

// AggregatedListDisks gets an aggregated list of GCE Disks.
func (c *client) AggregatedListDisks(project string, opts ...ListCallOption) ([]*compute.Disk, error) {
	var is []*compute.Disk
	var pt string
	call := c.raw.Disks.AggregatedList(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.DisksAggregatedListCall)
	}
	for ial, err := call.PageToken(pt).Do(); ; ial, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			ial, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		for _, isl := range ial.Items {
			is = append(is, isl.Disks...)
		}
		if ial.NextPageToken == "" {
			return is, nil
		}
		pt = ial.NextPageToken
	}
}

// ListDisks gets a list of GCE Disks.
func (c *client) ListDisks(project, zone string, opts ...ListCallOption) ([]*compute.Disk, error) {
	var ds []*compute.Disk
	var pt string
	call := c.raw.Disks.List(project, zone)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.DisksListCall)
	}
	for dl, err := call.PageToken(pt).Do(); ; dl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			dl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		ds = append(ds, dl.Items...)

		if dl.NextPageToken == "" {
			return ds, nil
		}
		pt = dl.NextPageToken
	}
}

// GetForwardingRule gets a GCE ForwardingRule.
func (c *client) GetForwardingRule(project, region, name string) (*compute.ForwardingRule, error) {
	n, err := c.raw.ForwardingRules.Get(project, region, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.ForwardingRules.Get(project, region, name).Do()
	}
	return n, err
}

// AggregatedListForwardingRules gets an aggregated list of GCE ForwardingRules.
func (c *client) AggregatedListForwardingRules(project string, opts ...ListCallOption) ([]*compute.ForwardingRule, error) {
	var frs []*compute.ForwardingRule
	var pt string
	call := c.raw.ForwardingRules.AggregatedList(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.ForwardingRulesAggregatedListCall)
	}
	for ail, err := call.PageToken(pt).Do(); ; ail, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			ail, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		for _, frl := range ail.Items {
			frs = append(frs, frl.ForwardingRules...)
		}
		if ail.NextPageToken == "" {
			return frs, nil
		}
		pt = ail.NextPageToken
	}
}

// ListForwardingRules gets a list of GCE ForwardingRules.
func (c *client) ListForwardingRules(project, region string, opts ...ListCallOption) ([]*compute.ForwardingRule, error) {
	var frs []*compute.ForwardingRule
	var pt string
	call := c.raw.ForwardingRules.List(project, region)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.ForwardingRulesListCall)
	}
	for frl, err := call.PageToken(pt).Do(); ; frl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			frl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		frs = append(frs, frl.Items...)

		if frl.NextPageToken == "" {
			return frs, nil
		}
		pt = frl.NextPageToken
	}
}

// GetFirewallRule gets a GCE FirewallRule.
func (c *client) GetFirewallRule(project, name string) (*compute.Firewall, error) {
	i, err := c.raw.Firewalls.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Firewalls.Get(project, name).Do()
	}
	return i, err
}

// ListFirewallRules gets a list of GCE FirewallRules.
func (c *client) ListFirewallRules(project string, opts ...ListCallOption) ([]*compute.Firewall, error) {
	var is []*compute.Firewall
	var pt string
	call := c.raw.Firewalls.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.FirewallsListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// GetImage gets a GCE Image.
func (c *client) GetImage(project, name string) (*compute.Image, error) {
	i, err := c.raw.Images.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Images.Get(project, name).Do()
	}
	return i, err
}

// GetImageAlpha gets a GCE Image using Alpha API
func (c *client) GetImageAlpha(project, name string) (*computeAlpha.Image, error) {
	i, err := c.rawAlpha.Images.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.rawAlpha.Images.Get(project, name).Do()
	}
	return i, err
}

// GetImageBeta gets a GCE Image using Beta API
func (c *client) GetImageBeta(project, name string) (*computeBeta.Image, error) {
	i, err := c.rawBeta.Images.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.rawBeta.Images.Get(project, name).Do()
	}
	return i, err
}

// GetImageFromFamily gets a GCE Image from an image family.
func (c *client) GetImageFromFamily(project, family string) (*compute.Image, error) {
	i, err := c.raw.Images.GetFromFamily(project, family).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Images.GetFromFamily(project, family).Do()
	}
	return i, err
}

// GetImageFromFamilyBeta gets a GCE Image from an image family using Beta API.
func (c *client) GetImageFromFamilyBeta(project, family string) (*computeBeta.Image, error) {
	i, err := c.rawBeta.Images.GetFromFamily(project, family).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.rawBeta.Images.GetFromFamily(project, family).Do()
	}
	return i, err
}

// ListImages gets a list of GCE Images.
func (c *client) ListImages(project string, opts ...ListCallOption) ([]*compute.Image, error) {
	var is []*compute.Image
	var pt string
	call := c.raw.Images.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.ImagesListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// ListImagesAlpha gets a list of GCE Images using Alpha API.
func (c *client) ListImagesAlpha(project string, opts ...ListCallOption) ([]*computeAlpha.Image, error) {
	var is []*computeAlpha.Image
	var pt string
	call := c.rawAlpha.Images.List(project)

	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*computeAlpha.ImagesListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// CreateSnapshot creates a GCE snapshot.
// SourceDisk is the url (full or partial) to the source disk.
func (c *client) CreateSnapshot(project, zone, disk string, s *compute.Snapshot) error {
	op, err := c.Retry(c.raw.Disks.CreateSnapshot(project, zone, disk, s).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdSnapshot *compute.Snapshot
	if createdSnapshot, err = c.i.GetSnapshot(project, s.Name); err != nil {
		return err
	}
	*s = *createdSnapshot
	return nil
}

// CreateSnapshotWithGuestFlush creates a GCE snapshot informing the OS to prepare for the snapshot process.
func (c *client) CreateSnapshotWithGuestFlush(project, zone, disk string, s *compute.Snapshot) error {
	op, err := c.Retry(c.raw.Disks.CreateSnapshot(project, zone, disk, s).GuestFlush(true).Do)
	if err != nil {
		return err
	}

	if err := c.i.zoneOperationsWait(project, zone, op.Name); err != nil {
		return err
	}

	var createdSnapshot *compute.Snapshot
	if createdSnapshot, err = c.i.GetSnapshot(project, s.Name); err != nil {
		return err
	}
	*s = *createdSnapshot
	return nil
}

// GetSnapshot gets a GCE Snapshot.
func (c *client) GetSnapshot(project, name string) (*compute.Snapshot, error) {
	n, err := c.raw.Snapshots.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Snapshots.Get(project, name).Do()
	}
	return n, err
}

// DeleteSnapshot deletes a GCE Snapshot.
func (c *client) DeleteSnapshot(project, name string) error {
	op, err := c.Retry(c.raw.Snapshots.Delete(project, name).Do)
	if err != nil {
		return err
	}

	return c.i.globalOperationsWait(project, op.Name)
}

// ListSnapshots gets a list of GCE Snapshots.
func (c *client) ListSnapshots(project string, opts ...ListCallOption) ([]*compute.Snapshot, error) {
	var ss []*compute.Snapshot
	var pt string
	call := c.raw.Snapshots.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.SnapshotsListCall)
	}
	for sl, err := call.PageToken(pt).Do(); ; sl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			sl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		ss = append(ss, sl.Items...)

		if sl.NextPageToken == "" {
			return ss, nil
		}
		pt = sl.NextPageToken
	}
}

// GetNetwork gets a GCE Network.
func (c *client) GetNetwork(project, name string) (*compute.Network, error) {
	n, err := c.raw.Networks.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Networks.Get(project, name).Do()
	}
	return n, err
}

// GetRegion gets a GCE Region
func (c *client) GetRegion(project, name string) (*compute.Region, error) {
	n, err := c.raw.Regions.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Regions.Get(project, name).Do()
	}
	return n, err
}

// Suspend an instance
func (c *client) Suspend(project, zone, name string) error {
	var op *compute.Operation
	var err error
	op, err = c.raw.Instances.Suspend(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		op, err = c.raw.Instances.Suspend(project, zone, name).Do()
	}
	if err != nil {
		return err
	}
	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// Resume an instance
func (c *client) Resume(project, zone, name string) error {
	var op *compute.Operation
	var err error
	op, err = c.raw.Instances.Resume(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		op, err = c.raw.Instances.Resume(project, zone, name).Do()
	}
	if err != nil {
		return err
	}
	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// SimulateMaintenanceEvent simulates a maintenance event on an instance.
func (c *client) SimulateMaintenanceEvent(project, zone, name string) error {
	var op *compute.Operation
	var err error
	op, err = c.raw.Instances.SimulateMaintenanceEvent(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		op, err = c.raw.Instances.SimulateMaintenanceEvent(project, zone, name).Do()
	}
	if err != nil {
		return err
	}
	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// ListNetworks gets a list of GCE Networks.
func (c *client) ListNetworks(project string, opts ...ListCallOption) ([]*compute.Network, error) {
	var ns []*compute.Network
	var pt string
	call := c.raw.Networks.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.NetworksListCall)
	}
	for nl, err := call.PageToken(pt).Do(); ; nl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			nl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		ns = append(ns, nl.Items...)

		if nl.NextPageToken == "" {
			return ns, nil
		}
		pt = nl.NextPageToken
	}
}

// GetSubnetwork gets a GCE subnetwork.
func (c *client) GetSubnetwork(project, region, name string) (*compute.Subnetwork, error) {
	n, err := c.raw.Subnetworks.Get(project, region, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Subnetworks.Get(project, region, name).Do()
	}
	return n, err
}

// AggregatedListSubnetworks gets an aggregated list of GCE Subnetworks.
func (c *client) AggregatedListSubnetworks(project string, opts ...ListCallOption) ([]*compute.Subnetwork, error) {
	var ss []*compute.Subnetwork
	var pt string
	call := c.raw.Subnetworks.AggregatedList(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.SubnetworksAggregatedListCall)
	}
	for sal, err := call.PageToken(pt).Do(); ; sal, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			sal, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		for _, sl := range sal.Items {
			ss = append(ss, sl.Subnetworks...)
		}
		if sal.NextPageToken == "" {
			return ss, nil
		}
		pt = sal.NextPageToken
	}
}

// ListSubnetworks gets a list of GCE subnetworks.
func (c *client) ListSubnetworks(project, region string, opts ...ListCallOption) ([]*compute.Subnetwork, error) {
	var ns []*compute.Subnetwork
	var pt string
	call := c.raw.Subnetworks.List(project, region)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.SubnetworksListCall)
	}
	for nl, err := call.PageToken(pt).Do(); ; nl, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			nl, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		ns = append(ns, nl.Items...)

		if nl.NextPageToken == "" {
			return ns, nil
		}
		pt = nl.NextPageToken
	}
}

// GetTargetInstance gets a GCE TargetInstance.
func (c *client) GetTargetInstance(project, zone, name string) (*compute.TargetInstance, error) {
	n, err := c.raw.TargetInstances.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.TargetInstances.Get(project, zone, name).Do()
	}
	return n, err
}

// ListTargetInstances gets a list of GCE TargetInstances.
func (c *client) ListTargetInstances(project, zone string, opts ...ListCallOption) ([]*compute.TargetInstance, error) {
	var tis []*compute.TargetInstance
	var pt string
	call := c.raw.TargetInstances.List(project, zone)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.TargetInstancesListCall)
	}
	for til, err := call.PageToken(pt).Do(); ; til, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			til, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		tis = append(tis, til.Items...)

		if til.NextPageToken == "" {
			return tis, nil
		}
		pt = til.NextPageToken
	}
}

// GetLicense gets a GCE License.
func (c *client) GetLicense(project, name string) (*compute.License, error) {
	l, err := c.raw.Licenses.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.Licenses.Get(project, name).Do()
	}
	return l, err
}

// ListLicenses gets a list GCE Licenses.
func (c *client) ListLicenses(project string, opts ...ListCallOption) ([]*compute.License, error) {
	var ls []*compute.License
	var pt string
	call := c.raw.Licenses.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.LicensesListCall)
	}
	for ll, err := call.PageToken(pt).Do(); ; ll, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			ll, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		ls = append(ls, ll.Items...)

		if ll.NextPageToken == "" {
			return ls, nil
		}
		pt = ll.NextPageToken
	}
}

// InstanceStatus returns an instances Status.
func (c *client) InstanceStatus(project, zone, name string) (string, error) {
	is, err := c.raw.Instances.Get(project, zone, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		is, err = c.raw.Instances.Get(project, zone, name).Do()
	}

	if err != nil {
		return "", err
	}
	return is.Status, nil
}

// InstanceStopped checks if a GCE instance is in a 'TERMINATED' or 'STOPPED' state.
func (c *client) InstanceStopped(project, zone, name string) (bool, error) {
	status, err := c.i.InstanceStatus(project, zone, name)
	if err != nil {
		return false, err
	}
	switch status {
	case "PROVISIONING", "REPAIRING", "RUNNING", "STAGING", "STOPPING":
		return false, nil
	case "TERMINATED", "STOPPED":
		return true, nil
	default:
		return false, fmt.Errorf("unexpected instance status %q", status)
	}
}

// ResizeDisk resizes a GCE persistent disk. You can only increase the size of the disk.
func (c *client) ResizeDisk(project, zone, disk string, drr *compute.DisksResizeRequest) error {
	op, err := c.Retry(c.raw.Disks.Resize(project, zone, disk, drr).Do)
	if err != nil {
		return err
	}

	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// SetInstanceMetadata sets an instances metadata.
func (c *client) SetInstanceMetadata(project, zone, name string, md *compute.Metadata) error {
	op, err := c.Retry(c.raw.Instances.SetMetadata(project, zone, name, md).Do)
	if err != nil {
		return err
	}
	return c.i.zoneOperationsWait(project, zone, op.Name)
}

// SetCommonInstanceMetadata sets an instances metadata.
func (c *client) SetCommonInstanceMetadata(project string, md *compute.Metadata) error {
	op, err := c.Retry(c.raw.Projects.SetCommonInstanceMetadata(project, md).Do)
	if err != nil {
		return err
	}

	return c.i.globalOperationsWait(project, op.Name)
}

// GetGuestAttributes gets a Guest Attributes.
func (c *client) GetGuestAttributes(project, zone, name, queryPath, variableKey string) (*compute.GuestAttributes, error) {
	call := c.raw.Instances.GetGuestAttributes(project, zone, name)
	logging.Printf("call %v", call)
	logging.Printf("queryPath %v", queryPath)
	if queryPath != "" {
		call = call.QueryPath(queryPath)
	}
	logging.Printf("call 2 %v", call)
	logging.Printf("variableKey %v", variableKey)
	if variableKey != "" {
		call = call.VariableKey(variableKey)
	}
	logging.Printf("call 3 %v", call)
	a, err := call.Do()
	logging.Printf("a %v", a)
	logging.Printf("err %v", err)
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return call.Do()
	}
	logging.Printf("a 2 %v", a)
	logging.Printf("err 2 %v", err)
	return a, err
}

// ListMachineImages gets a list of GCE Machine Images.
func (c *client) ListMachineImages(project string, opts ...ListCallOption) ([]*compute.MachineImage, error) {
	var is []*compute.MachineImage
	var pt string
	call := c.raw.MachineImages.List(project)
	for _, opt := range opts {
		call = opt.listCallOptionApply(call).(*compute.MachineImagesListCall)
	}
	for il, err := call.PageToken(pt).Do(); ; il, err = call.PageToken(pt).Do() {
		if shouldRetryWithWait(c.hc.Transport, err, 2) {
			il, err = call.PageToken(pt).Do()
		}
		if err != nil {
			return nil, err
		}
		is = append(is, il.Items...)

		if il.NextPageToken == "" {
			return is, nil
		}
		pt = il.NextPageToken
	}
}

// DeleteMachineImage deletes a GCE machine image.
func (c *client) DeleteMachineImage(project, name string) error {
	op, err := c.Retry(c.raw.MachineImages.Delete(project, name).Do)
	if err != nil {
		return err
	}

	return c.i.globalOperationsWait(project, op.Name)
}

// CreateMachineImage creates a GCE machine image.
// sourceInstance must be specified, which is the url (full or partial) to the
// source instance
func (c *client) CreateMachineImage(project string, mi *compute.MachineImage) error {
	op, err := c.Retry(c.raw.MachineImages.Insert(project, mi).Do)
	if err != nil {
		return err
	}

	if err := c.i.globalOperationsWait(project, op.Name); err != nil {
		return err
	}

	var createdMachineImage *compute.MachineImage
	if createdMachineImage, err = c.i.GetMachineImage(project, mi.Name); err != nil {
		return err
	}
	*mi = *createdMachineImage
	return nil
}

// GetMachineImage gets a GCE Machine Image.
func (c *client) GetMachineImage(project, name string) (*compute.MachineImage, error) {
	i, err := c.raw.MachineImages.Get(project, name).Do()
	if shouldRetryWithWait(c.hc.Transport, err, 2) {
		return c.raw.MachineImages.Get(project, name).Do()
	}
	return i, err
}
