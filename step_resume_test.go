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

func TestResumePopulate(t *testing.T) {
	ctx := context.Background()
	w := testWorkflow()
	w.Project = "foo"
	w.Zone = "bar"
	s, _ := w.NewStep("sp")
	s.Resume = &Resume{}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating resume step: %v", err)
	}
	if s.Resume.Project != "foo" {
		t.Errorf("want resume project foo, got %s", s.Resume.Project)
	}
	if s.Resume.Zone != "bar" {
		t.Errorf("want resume zone bar, got %s", s.Resume.Zone)
	}
	s, _ = w.NewStep("sp-nooverwrite")
	s.Resume = &Resume{
		Project: "no-overwrite",
		Zone:    "no-overwrite",
	}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating resume step: %v", err)
	}
	if s.Resume.Project != "no-overwrite" {
		t.Errorf("want resume project no-overwrite, got %s", s.Resume.Project)
	}
	if s.Resume.Zone != "no-overwrite" {
		t.Errorf("want resume zone no-overwrite, got %s", s.Resume.Zone)
	}
}

func TestResumeValidate(t *testing.T) {
	ctx := context.Background()
	w := testWorkflow()
	w.Project = "foo"
	w.Zone = "bar"
	s, _ := w.NewStep("sp")
	s.Resume = &Resume{
		Instance: "baz",
	}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating resume step: %v", err)
	}
	if err := w.validate(ctx); err != nil {
		t.Errorf("got error validating resume step: %v", err)
	}
}

func TestResumeValidateError(t *testing.T) {
	testcases := []struct {
		name string
		s    *Resume
	}{
		{
			name: "no project",
			s: &Resume{
				Zone:     "no-project",
				Instance: "no-project",
			},
		},
		{
			name: "no zone",
			s: &Resume{
				Project:  "no-zone",
				Instance: "no-zone",
			},
		},
		{
			name: "no instance",
			s: &Resume{
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
			s.Resume = tc.s
			if err := w.validate(ctx); err == nil {
				t.Errorf("validated bad step: %v", tc.s)
			}
		})
	}
}

func TestResumeRun(t *testing.T) {
	svr, c, err := daisyCompute.NewTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && r.URL.String() == fmt.Sprintf("/projects/%s/zones/%s/instances/%s/resume?alt=json&prettyPrint=false", testProject, testZone, testInstance) {
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
	s.Resume = &Resume{
		Instance: testInstance,
	}
	if err := w.populate(ctx); err != nil {
		t.Errorf("got error populating resume step: %v", err)
	}
	if err := w.run(ctx); err != nil {
		t.Errorf("got error running resume workflow: %v", err)
	}
}
