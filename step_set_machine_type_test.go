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
	"testing"
)

func TestSetMachineTypePopulate(t *testing.T) {
	tests := []struct {
		name string
		s    *SetMachineType
	}{
		{
			name: "no-project",
			s: &SetMachineType{
				Zone:        "no-project",
				Instance:    "no-project",
				MachineType: "no-project",
			},
		},
		{
			name: "no-zone",
			s: &SetMachineType{
				Project:     "no-zone",
				Instance:    "no-zone",
				MachineType: "no-zone",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			w := testWorkflow()
			w.Project = test.name
			w.Zone = test.name
			s, _ := w.NewStep("smt")
			s.SetMachineType = test.s

			if err := w.populate(ctx); err != nil {
				t.Fatalf("populate(ctx) failed: %v", err)
			}
			if s.SetMachineType.Project != test.name {
				t.Errorf("SetMachineType.Project = %q, want %q", s.SetMachineType.Project, test.name)
			}
			if s.SetMachineType.Zone != test.name {
				t.Errorf("SetMachineType.Zone = %q, want %q", s.SetMachineType.Zone, test.name)
			}
		})
	}
}

func TestSetMachineTypeValidate(t *testing.T) {
	tests := []struct {
		name      string
		s         *SetMachineType
		expectErr bool
	}{
		{
			name: "no-project",
			s: &SetMachineType{
				Zone:        "no-project",
				Instance:    "no-project",
				MachineType: "no-project",
			},
			expectErr: true,
		},
		{
			name: "no-zone",
			s: &SetMachineType{
				Project:     "no-zone",
				Instance:    "no-zone",
				MachineType: "no-zone",
			},
			expectErr: true,
		},
		{
			name: "no-instance",
			s: &SetMachineType{
				Project:     "no-instance",
				Zone:        "no-instance",
				MachineType: "no-instance",
			},
			expectErr: true,
		},
		{
			name: "no-machine-type",
			s: &SetMachineType{
				Project:  "no-machine-type",
				Zone:     "no-machine-type",
				Instance: "no-machine-type",
			},
			expectErr: true,
		},
		{
			name: "success",
			s: &SetMachineType{
				Project:     "success",
				Zone:        "success",
				Instance:    "success",
				MachineType: "success",
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			w := testWorkflow()
			w.Project = test.name
			w.Zone = test.name
			s, _ := w.NewStep("smt")
			s.SetMachineType = test.s

			if err := w.validate(ctx); (err == nil) == test.expectErr {
				t.Errorf("validate(ctx) = %v, want %t", err, test.expectErr)
			}
		})
	}
}

func TestSetMachineTypeRun(t *testing.T) {
	ctx := context.Background()
	w := testWorkflow()
	w.Project = testProject
	w.Zone = testZone
	s, _ := w.NewStep("smt")
	s.SetMachineType = &SetMachineType{
		Instance:    testInstance,
		MachineType: "n1-standard-1",
	}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating set machine type step: %v", err)
	}
	if err := w.run(ctx); err != nil {
		t.Errorf("got error running set machine type step: %v", err)
	}
}
