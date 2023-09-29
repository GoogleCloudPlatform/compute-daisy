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
	"time"
)

const defaultQuotaInterval = "5s"

// WaitForAvailableQuotas is a daisy workflow step to wait for a list of quotas to be available at the same time.
type WaitForAvailableQuotas struct {
	// Interval to check for signal.
	// Must be parsable by https://golang.org/pkg/time/#ParseDuration.
	Interval       string `json:",omitempty"`
	parsedInterval time.Duration
	Quotas         []*QuotaAvailable
}

// QuotaAvailable waits for some units of quota to be available in a given region. The individual items to wait for in the workflow step.
type QuotaAvailable struct {
	// Metric name to wait for.
	Metric string
	// Region to check for quota in.
	Region string
	// Units of quota which must be available.
	Units float64
}

func (aq *WaitForAvailableQuotas) populate(ctx context.Context, s *Step) DError {
	if aq.Interval == "" {
		aq.Interval = defaultQuotaInterval
	}
	var err error
	aq.parsedInterval, err = time.ParseDuration(aq.Interval)
	if err != nil {
		return typedErr(invalidInputError, fmt.Sprintf("failed to parse duration for step %v", s.name), err)
	}
	return nil
}

func (aq *WaitForAvailableQuotas) validate(ctx context.Context, s *Step) DError {
	if aq.parsedInterval == 0*time.Second {
		return Errf("No interval given for step %s", s.name)
	}
	for _, q := range aq.Quotas {
		if q.Metric == "" {
			err := fmt.Errorf("No metric given for step %s", s.name)
			return typedErr(invalidInputError, err.Error(), err)
		}
		if q.Region == "" {
			err := fmt.Errorf("No region given for step %s", s.name)
			return typedErr(invalidInputError, err.Error(), err)
		}
		if q.Units < 0 {
			err := fmt.Errorf("Units must be a positive int, got %.2f for step %s", q.Units, s.name)
			return typedErr(invalidInputError, err.Error(), err)
		}
	}
	return nil
}

func (aq *WaitForAvailableQuotas) run(ctx context.Context, s *Step) DError {
	for _, a := range aq.Quotas {
		s.w.LogStepInfo(s.name, "WaitForAvailableQuotas", "Waiting for %.2f units of %s to be available in %s", a.Units, a.Metric, a.Region)
	}
	tick := time.Tick(aq.parsedInterval)
	for {
		select {
		case <-s.w.Cancel:
			return nil
		case <-ctx.Done():
			err := fmt.Errorf("context expired before quota was available in step %s", s.name)
			return typedErr(ctx.Err().Error(), err.Error(), err)
		case <-tick:
			var successmsgs []string
			for _, a := range aq.Quotas {
				r, err := s.w.ComputeClient.GetRegion(s.w.Project, a.Region)
				if err != nil {
					return typedErr(apiError, "failed to get region "+a.Region, err)
				}
				for _, q := range r.Quotas {
					if q.Metric == a.Metric && ((q.Limit - q.Usage) >= a.Units) {
						successmsgs = append(successmsgs, fmt.Sprintf("Region %s has %.2f units of %s available", a.Region, (q.Limit-q.Usage), a.Metric))
					}
				}
			}
			if len(successmsgs) == len(aq.Quotas) {
				for _, m := range successmsgs {
					s.w.LogStepInfo(s.name, "WaitForAvailableQuotas", m)
				}
				return nil
			}
		}
	}
}
