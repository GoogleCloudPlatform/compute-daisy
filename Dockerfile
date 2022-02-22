# Copyright 2017 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM gcr.io/distroless/base as certs
FROM golang as build
WORKDIR /go/src/daisy

# Pre cache mod dependencies; this speeds up local development builds when
# changes are independent of the go module dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Build daisy. CGO_ENABLED=0 forces static linking libc dependencies
# so that the resulting binary can be used in scratch.
COPY . .
RUN CGO_ENABLED=0 go build -v -o /go/bin/daisy cli/main.go

# Historically we included daisy workflows in the Docker image. This conditional
# allows the Dockerfile to be built in this repo, resulting in an image
# that doesn't include the daisy workflows. In concourse, we manually include
# the workflows in the build context.
RUN if [ ! -d "daisy_workflows" ] ; then mkdir daisy_workflows; echo done ; fi

FROM scratch
COPY --from=build /go/bin/daisy /daisy
COPY --from=build /go/src/daisy/daisy_workflows /workflows
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENTRYPOINT ["/daisy"]
