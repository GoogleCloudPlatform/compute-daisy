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
	"fmt"
)

// Resume is a Daisy resume workflow step.
type Resume struct {
	Project  string
	Zone     string
	Instance string
}

// populate preprocesses fields: Instance, Project, Zone
// - sets defaults
// - extends short partial URLs to include "projects/<project>"
func (r *Resume) populate(ctx context.Context, s *Step) DError {
	if r.Project == "" {
		r.Project = s.w.Project
	}
	if r.Zone == "" {
		r.Zone = s.w.Zone
	}
	return nil
}

func (r *Resume) validate(ctx context.Context, s *Step) DError {
	var errs DError
	if r.Project == "" {
		errs = addErrs(errs, fmt.Errorf("must specify project"))
	}
	if r.Zone == "" {
		errs = addErrs(errs, fmt.Errorf("must specify zone"))
	}
	if r.Instance == "" {
		errs = addErrs(errs, fmt.Errorf("must specify instance"))
	}
	return errs
}

func (r *Resume) run(ctx context.Context, s *Step) DError {
	prj := r.Project
	zone := r.Zone
	inst := r.Instance
	i, ok := s.w.instances.get(inst)
	if ok {
		m := NamedSubexp(instanceURLRgx, i.link)
		prj = m["project"]
		zone = m["zone"]
		inst = m["instance"]
	}
	return addErrs(nil, s.w.ComputeClient.Resume(prj, zone, inst))
}
