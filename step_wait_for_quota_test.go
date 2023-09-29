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
	"time"

	daisyCompute "github.com/GoogleCloudPlatform/compute-daisy/compute"
)

func TestWaitForAvailableQuotas(t *testing.T) {
	w := testWorkflow()

	svr, c, err := daisyCompute.NewTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.String() == fmt.Sprintf("/projects/%s/regions/%s?alt=json&prettyPrint=false", testProject, testRegion) {
			fmt.Fprint(w, `{"Quotas":[{"Metric":"A", "Usage":5.0, "Limit": 10.0},{"Metric":"B", "Usage": 10.0, "Limit": 10.0},{"Metric":"C", "Usage": 4.0, "Limit": 10.0}]}`)
		} else {
			w.WriteHeader(500)
			fmt.Fprintln(w, "URL and Method not recognized:", r.Method, r.URL)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer svr.Close()

	w.ComputeClient = c
	w.Project = testProject
	s := &Step{name: "foo", w: w}
	tc := []struct {
		name  string
		input WaitForAvailableQuotas
	}{
		{
			name: "single quota",
			input: WaitForAvailableQuotas{
				Quotas: []*QuotaAvailable{
					&QuotaAvailable{Metric: "A", Region: testRegion, Units: 1.0},
				},
			},
		},
		{
			name: "multiple quotas",
			input: WaitForAvailableQuotas{
				Quotas: []*QuotaAvailable{
					&QuotaAvailable{Metric: "A", Region: testRegion, Units: 4.5},
					&QuotaAvailable{Metric: "C", Region: testRegion, Units: 6.0},
				},
			},
		},
	}
	for _, test := range tc {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(6*time.Second))
			defer cancel()
			err := test.input.populate(ctx, s)
			if err != nil {
				t.Fatalf("failed to populate test %s: %q", test.name, err)
			}
			err = test.input.validate(ctx, s)
			if err != nil {
				t.Fatalf("failed to validate test %s: %q", test.name, err)
			}
			err = test.input.run(ctx, s)
			if err != nil {
				t.Errorf("failed to run test %s: %q", test.name, err)
			}
		})
	}
}

func TestWaitForAvailableQuotasError(t *testing.T) {
	w := testWorkflow()

	svr, c, err := daisyCompute.NewTestClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.String() == fmt.Sprintf("/projects/%s/regions/%s?alt=json&prettyPrint=false", testProject, testRegion) {
			fmt.Fprint(w, `{"Quotas":[{"Metric":"A", "Usage":5.0, "Limit": 10.0},{"Metric":"B", "Usage": 10.0, "Limit": 10.0},{"Metric":"C", "Usage": 4.0, "Limit": 10.0}]}`)
		} else {
			w.WriteHeader(500)
			fmt.Fprintln(w, "URL and Method not recognized:", r.Method, r.URL)
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer svr.Close()

	w.ComputeClient = c
	w.Project = testProject
	s := &Step{name: "foo", w: w}
	tc := []struct {
		name   string
		input  WaitForAvailableQuotas
		output string
	}{
		{
			name: "unavailable quota",
			input: WaitForAvailableQuotas{
				Interval: "0.1s",
				Quotas: []*QuotaAvailable{
					&QuotaAvailable{Metric: "A", Region: testRegion, Units: 5.0},
					&QuotaAvailable{Metric: "B", Region: testRegion, Units: 1.0},
				},
			},
			output: context.DeadlineExceeded.Error(),
		},
	}
	for _, test := range tc {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1*time.Second))
			defer cancel()
			err := test.input.populate(ctx, s)
			if err != nil {
				t.Fatalf("failed to populate test %s: %q", test.name, err)
			}
			err = test.input.validate(ctx, s)
			if err != nil {
				t.Fatalf("failed to validate test %s: %q", test.name, err)
			}
			err = test.input.run(ctx, s)
			if !err.CausedByErrType(test.output) {
				t.Errorf("unexpected error type from test %s: want %v, got %v", test.name, test.output, err)
			}
		})
	}
}

func TestValidateWaitForAvailableQuotasError(t *testing.T) {
	w := testWorkflow()
	s := &Step{name: "foo", w: w}
	tc := []struct {
		name   string
		input  WaitForAvailableQuotas
		output string
	}{
		{
			name: "no metric",
			input: WaitForAvailableQuotas{
				Interval: "0.1s",
				Quotas: []*QuotaAvailable{
					&QuotaAvailable{Region: testRegion, Units: 5.0},
				},
			},
			output: invalidInputError,
		},
		{
			name: "no region",
			input: WaitForAvailableQuotas{
				Interval: "0.1s",
				Quotas: []*QuotaAvailable{
					&QuotaAvailable{Metric: "A", Units: 5.0},
				},
			},
			output: invalidInputError,
		},
		{
			name: "negative units",
			input: WaitForAvailableQuotas{
				Interval: "0.1s",
				Quotas: []*QuotaAvailable{
					&QuotaAvailable{Metric: "A", Region: testRegion, Units: -5.0},
				},
			},
			output: invalidInputError,
		},
	}
	for _, test := range tc {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1*time.Second))
			defer cancel()
			err := test.input.populate(ctx, s)
			if err != nil {
				t.Fatalf("failed to populate test %s: %q", test.name, err)
			}
			err = test.input.validate(ctx, s)
			if !err.CausedByErrType(test.output) {
				t.Errorf("unexpected error type from test %s: want %v, got %v", test.name, test.output, err)
			}
		})
	}
}

func TestPopulateWaitForAvailableQuotasError(t *testing.T) {
	w := testWorkflow()
	s := &Step{name: "foo", w: w}
	tc := []struct {
		name   string
		input  WaitForAvailableQuotas
		output string
	}{
		{
			name: "invalid interval",
			input: WaitForAvailableQuotas{
				Interval: "asdf",
			},
			output: invalidInputError,
		},
	}
	for _, test := range tc {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(1*time.Second))
			defer cancel()
			err := test.input.populate(ctx, s)
			if !err.CausedByErrType(test.output) {
				t.Errorf("unexpected error type from test %s: want %v, got %v", test.name, test.output, err)
			}
		})
	}
}
