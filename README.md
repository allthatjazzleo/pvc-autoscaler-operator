# pvc-autoscaler-operator
The PVC Autoscaler Operator is an open-source solution designed to introduce autoscaling capabilities to Persistent Volume Claims (PVCs) within Kubernetes ecosystems. It accomplishes this by deploying a disk healthcheck sidecar injection mechanism, eliminating the dependence on Prometheus metrics. This progressive approach enhances the scalability and efficiency of containerized storage management.

Not extensively tested, use at your own risk.

## Motivation

Current solutions of PVC autoscaling are to use Prometheus metrics to monitor the disk usage of the PVC. However, this approach is not scalable and efficient. The Prometheus metrics are collected by the Prometheus server, which is a single point of failure. The Prometheus server is also a bottleneck for the scalability of the PVC autoscaling. The Prometheus server is not designed to hand large amount of metric data. This project aims to provide a scalable and efficient solution to PVC autoscaling by introducing a disk healthcheck sidecar injection mechanism.

## Getting Started
See the [quick start guide](./docs/quick_start.md).

## Contributing

See the [contributing guide](./docs/contributing.md).

## Acknowledgement

The initial idea for this project was inspired by how [strangelove-ventures/cosmos-operator](<https://github.com/strangelove-ventures/cosmos-operator>) manages and scales PVC resources for cosmoshub nodes. Some of the code was also borrowed from strangelove-ventures/cosmos-operator such as [internal/healthcheck](./internal/healthcheck/) and [internal/controllers/pvcscaling_controller.go](./internal/controllers/pvcscaling_controller.go).

## License

Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
