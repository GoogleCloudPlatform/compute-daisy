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
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"

	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

var (
	subnetworkURLRegex = regexp.MustCompile(fmt.Sprintf(`^(projects/(?P<project>%[1]s)/)?regions/(?P<region>%[2]s)/subnetworks/(?P<subnetwork>%[2]s)$`, projectRgxStr, rfc1035))

	// Valid stack types for subnetworks.
	validStackType = []string{"IPV4_ONLY", "IPV4_IPV6", "IPV6_ONLY"}
	// Valid IPv6 access types for subnetworks.
	validIpv6AccessType = []string{"INTERNAL", "EXTERNAL"}
)

func (w *Workflow) subnetworkExists(project, region, subnetwork string) (bool, DError) {
	return w.subnetworkCache.resourceExists(func(project, region string, opts ...daisyCompute.ListCallOption) (any, error) {
		return w.ComputeClient.ListSubnetworks(project, region)
	}, project, region, subnetwork)
}

// Subnetwork is used to create a GCE subnetwork.
type Subnetwork struct {
	compute.Subnetwork
	Resource
}

// MarshalJSON is a hacky workaround to compute.Subnetwork's implementation.
func (sn *Subnetwork) MarshalJSON() ([]byte, error) {
	return json.Marshal(*sn)
}

func (sn *Subnetwork) populate(ctx context.Context, s *Step) DError {
	var errs DError
	sn.Name, errs = sn.Resource.populateWithGlobal(ctx, s, sn.Name)

	sn.Description = strOr(sn.Description, defaultDescription("Subnetwork", s.w.Name, s.w.username))
	r := sn.Region
	if r == "" {
		r = getRegionFromZone(s.w.Zone)
	}
	sn.link = fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", sn.Project, r, sn.Name)
	return errs
}

func (sn *Subnetwork) validate(ctx context.Context, s *Step) DError {
	pre := fmt.Sprintf("cannot create subnetwork %q", sn.daisyName)
	errs := sn.Resource.validate(ctx, s, pre)

	if sn.Name == "" {
		errs = addErrs(errs, Errf("%s: name is mandatory", pre))
	}
	if sn.Network == "" {
		errs = addErrs(errs, Errf("%s: network is mandatory", pre))
	}
	sn.Region = strOr(sn.Region, getRegionFromZone(s.w.Zone))

	// Check the stack type.
	if sn.StackType != "" {
		if !slices.Contains(validStackType, sn.StackType) {
			errs = addErrs(errs, Errf("%s: invalid stack type: %q, must be one of %v", pre, sn.StackType, validStackType))
		}
	}
	// If unspecified, the stack type defaults to IPV4_ONLY.
	if sn.StackType == "" || strings.Contains(sn.StackType, "IPV4") {
		if _, _, err := net.ParseCIDR(sn.IpCidrRange); err != nil {
			errs = addErrs(errs, Errf("%s: bad IpCidrRange: %q, error: %v", pre, sn.IpCidrRange, err))
		}
	}
	if strings.Contains(sn.StackType, "IPV6") {
		if sn.StackType == "IPV6_ONLY" && sn.IpCidrRange != "" {
			errs = addErrs(errs, Errf("%s: IPv6-only subnetworks must not have an IPv4 CIDR range", pre))
		}
		if sn.Ipv6CidrRange != "" {
			if _, _, err := net.ParseCIDR(sn.Ipv6CidrRange); err != nil {
				errs = addErrs(errs, Errf("%s: bad Ipv6CidrRange: %q, error: %v", pre, sn.Ipv6CidrRange, err))
			}
		}
		if sn.Ipv6AccessType == "" {
			errs = addErrs(errs, Errf("%s: ipv6 access type is mandatory", pre))
		} else {
			// Check the IPv6 access type.
			if !slices.Contains(validIpv6AccessType, sn.Ipv6AccessType) {
				errs = addErrs(errs, Errf("%s: invalid IPv6 access type: %q, must be one of %v", pre, sn.Ipv6AccessType, validIpv6AccessType))
			}
			if sn.InternalIpv6Prefix != "" {
				if _, _, err := net.ParseCIDR(sn.InternalIpv6Prefix); err != nil {
					errs = addErrs(errs, Errf("%s: bad InternalIpv6Prefix: %q, error: %v", pre, sn.InternalIpv6Prefix, err))
				}
			}
			if sn.Ipv6AccessType == "EXTERNAL" && sn.ExternalIpv6Prefix != "" {
				if _, _, err := net.ParseCIDR(sn.ExternalIpv6Prefix); err != nil {
					errs = addErrs(errs, Errf("%s: bad ExternalIpv6Prefix: %q, error: %v", pre, sn.ExternalIpv6Prefix, err))
				}
			}
		}
	}

	// Register creation.
	errs = addErrs(errs, s.w.subnetworks.regCreate(sn.daisyName, &sn.Resource, s, false))
	return errs
}

type subnetworkConnection struct {
	connector, disconnector *Step
}

type subnetworkRegistry struct {
	baseResourceRegistry
	connections          map[string]map[string]*subnetworkConnection
	testDisconnectHelper func(nName, iName string, s *Step) DError
}

func newSubnetworkRegistry(w *Workflow) *subnetworkRegistry {
	nr := &subnetworkRegistry{baseResourceRegistry: baseResourceRegistry{w: w, typeName: "subnetwork", urlRgx: subnetworkURLRegex}}
	nr.baseResourceRegistry.deleteFn = nr.deleteFn
	nr.connections = map[string]map[string]*subnetworkConnection{}
	nr.init()
	return nr
}

func (nr *subnetworkRegistry) deleteFn(res *Resource) DError {
	m := NamedSubexp(subnetworkURLRegex, res.link)
	err := nr.w.ComputeClient.DeleteSubnetwork(m["project"], m["region"], m["subnetwork"])
	if gErr, ok := err.(*googleapi.Error); ok && gErr.Code == http.StatusNotFound {
		return typedErr(resourceDNEError, "failed to delete subnetwork", err)
	}
	return newErr("failed to delete subnetwork", err)
}

func (nr *subnetworkRegistry) disconnectHelper(nName, iName string, s *Step) DError {
	if nr.testDisconnectHelper != nil {
		return nr.testDisconnectHelper(nName, iName, s)
	}
	pre := fmt.Sprintf("step %q cannot disconnect instance %q from subnetwork %q", s.name, iName, nName)
	var conn *subnetworkConnection

	if im, _ := nr.connections[nName]; im == nil {
		return Errf("%s: not connected", pre)
	} else if conn, _ = im[iName]; conn == nil {
		return Errf("%s: not attached", pre)
	} else if conn.disconnector != nil {
		return Errf("%s: already disconnected or concurrently disconnected by step %q", pre, conn.disconnector.name)
	} else if !s.nestedDepends(conn.connector) {
		return Errf("%s: step %q does not depend on connecting step %q", pre, s.name, conn.connector.name)
	}
	conn.disconnector = s
	return nil
}

// regConnect marks a subnetwork and instance as connected by a Step s.
func (nr *subnetworkRegistry) regConnect(nName, iName string, s *Step) DError {
	nr.mx.Lock()
	defer nr.mx.Unlock()

	pre := fmt.Sprintf("step %q cannot connect instance %q to subnetwork %q", s.name, iName, nName)
	if im, _ := nr.connections[nName]; im == nil {
		nr.connections[nName] = map[string]*subnetworkConnection{iName: {connector: s}}
	} else if nc, _ := im[iName]; nc != nil && !s.nestedDepends(nc.disconnector) {
		return Errf("%s: concurrently connected by step %q", pre, nc.connector.name)
	} else {
		nr.connections[nName][iName] = &subnetworkConnection{connector: s}
	}
	return nil
}

func (nr *subnetworkRegistry) regDisconnect(nName, iName string, s *Step) DError {
	nr.mx.Lock()
	defer nr.mx.Unlock()

	return nr.disconnectHelper(nName, iName, s)
}

// regDisconnect all is called by Instance.regDelete and registers Step s as the disconnector for all subnetworks that iName is currently connected to.
func (nr *subnetworkRegistry) regDisconnectAll(iName string, s *Step) DError {
	nr.mx.Lock()
	defer nr.mx.Unlock()

	var errs DError
	// For every subnetwork, if connected, disconnect.
	for nName, im := range nr.connections {
		if conn, _ := im[iName]; conn != nil && conn.disconnector == nil {
			errs = addErrs(nr.disconnectHelper(nName, iName, s))
		}
	}

	return errs
}
