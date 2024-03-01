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
	"net/http"
	"testing"

	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
)

func TestSuspendPopulate(t *testing.T) {
	ctx := context.Background()
	w := testWorkflow()
	w.Project = "foo"
	w.Zone = "bar"
	s, _ := w.NewStep("sp")
	s.Suspend = &Suspend{}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating suspend step: %v", err)
	}
	if s.Suspend.Project != "foo" {
		t.Errorf("want suspend project foo, got %s", s.Suspend.Project)
	}
	if s.Suspend.Zone != "bar" {
		t.Errorf("want suspend zone bar, got %s", s.Suspend.Zone)
	}
	s, _ = w.NewStep("sp-nooverwrite")
	s.Suspend = &Suspend{
		Project: "no-overwrite",
		Zone:    "no-overwrite",
	}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating suspend step: %v", err)
	}
	if s.Suspend.Project != "no-overwrite" {
		t.Errorf("want suspend project no-overwrite, got %s", s.Suspend.Project)
	}
	if s.Suspend.Zone != "no-overwrite" {
		t.Errorf("want suspend zone no-overwrite, got %s", s.Suspend.Zone)
	}
}

func TestSuspendValidate(t *testing.T) {
	ctx := context.Background()
	w := testWorkflow()
	w.Project = "foo"
	w.Zone = "bar"
	s, _ := w.NewStep("sp")
	s.Suspend = &Suspend{
		Instance: "baz",
	}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating suspend step: %v", err)
	}
	if err := w.validate(ctx); err != nil {
		t.Errorf("got error validating suspend step: %v", err)
	}
}

func TestSuspendValidateError(t *testing.T) {
	testcases := []struct {
		name string
		s    *Suspend
	}{
		{
			name: "no project",
			s: &Suspend{
				Zone:     "no-project",
				Instance: "no-project",
			},
		},
		{
			name: "no zone",
			s: &Suspend{
				Project:  "no-zone",
				Instance: "no-zone",
			},
		},
		{
			name: "no instance",
			s: &Suspend{
				Zone:    "no-instance",
				Project: "no-instance",
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			w := testWorkflow()
			s, _ := w.NewStep("sp")
			s.Suspend = tc.s
			if err := w.validate(ctx); err == nil {
				t.Errorf("validated bad step: %v", tc.s)
			}
		})
	}
}

func TestSuspendRun(t *testing.T) {
	svr, c, err := daisyCompute.NewTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.String() == fmt.Sprintf("/projects/%s/zones/%s/instances/%s/suspend?alt=json&prettyPrint=false", testProject, testZone, testInstance) {
			fmt.Fprint(w, `{}`)
		} else if r.Method == "POST" && r.URL.String() == fmt.Sprintf("/projects/%s/zones/%s/operations//wait?alt=json&prettyPrint=false", testProject, testZone) {
			fmt.Fprint(w, `{"Status": "DONE"}`)
		} else {
			w.WriteHeader(500)
			fmt.Fprintln(w, "URL and Method not recognized:", r.Method, r.URL)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer svr.Close()

	ctx := context.Background()
	w := testWorkflow()
	w.ComputeClient = c
	w.Project = testProject
	w.Zone = testZone
	s, _ := w.NewStep("sp")
	s.Suspend = &Suspend{
		Instance: testInstance,
	}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating suspend step: %v", err)
	}
	if err := w.run(ctx); err != nil {
		t.Errorf("got error running suspend workflow: %v", err)
	}
}
