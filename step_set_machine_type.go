//  Copyright 2024 Google Inc. All Rights Reserved.
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
	"fmt"
)

// SetMachineType is a step that sets the machine type of a GCE instance.
type SetMachineType struct {
	Project     string `json:",omitempty"`
	Zone        string `json:",omitempty"`
	Instance    string `json:",omitempty"`
	MachineType string `json:",omitempty"`
}

func (smt *SetMachineType) populate(ctx context.Context, s *Step) DError {
	if smt.Project == "" {
		smt.Project = s.w.Project
	}
	if smt.Zone == "" {
		smt.Zone = s.w.Zone
	}

	return nil
}

func (smt *SetMachineType) validate(ctx context.Context, s *Step) DError {
	var errs DError
	if smt.Project == "" {
		errs = addErrs(errs, fmt.Errorf("must specify project"))
	}
	if smt.Zone == "" {
		errs = addErrs(errs, fmt.Errorf("must specify zone"))
	}
	if smt.Instance == "" {
		errs = addErrs(errs, fmt.Errorf("must specify instance"))
	}
	if smt.MachineType == "" {
		errs = addErrs(errs, fmt.Errorf("must specify machine type"))
	}
	return errs
}

func (smt *SetMachineType) run(ctx context.Context, s *Step) DError {
	project := smt.Project
	zone := smt.Zone
	instance := smt.Instance
	i, ok := s.w.instances.get(smt.Instance)
	if ok {
		m := NamedSubexp(instanceURLRgx, i.link)
		project = m["project"]
		zone = m["zone"]
		instance = m["instance"]
	}
	machineType := smt.MachineType
	if !machineTypeURLRegex.MatchString(smt.MachineType) {
		machineType = fmt.Sprintf("projects/%s/zones/%s/machineTypes/%s", s.w.Project, s.w.Zone, smt.MachineType)
	}

	return newErr("", s.w.ComputeClient.SetMachineType(project, zone, instance, machineType))
}
