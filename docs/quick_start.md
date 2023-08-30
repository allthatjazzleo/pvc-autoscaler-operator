# Quick Start

This quick start guide creates a pvc autoscaler operator


### Prerequisites

1. Managed Kubernetes cluster (EKS, GKE, etc...)
2. Operator uses admission webhooks to create disk healthcheck sidecar. This requires an installed version of the [cert-manager](https://cert-manager.io/docs/).
3. CSI driver that supports [`VolumeExpansion`](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#csi-volume-expansion)
4. A storage class with the `allowVolumeExpansion` field set to `true`


### Install the CRDs and deploy operator in your cluster

View [docker images here](https://github.com/allthatjazzleo/pvc-autoscaler-operator/pkgs/container/pvc-autoscaler-operator).

```sh
# Deploy the latest release. Warning: May be a release candidate.
make deploy IMG="ghcr.io/allthatjazzleo/pvc-autoscaler-operator:$(git describe --tags --abbrev=0)"

# Deploy a specific version
make deploy IMG="ghcr.io/allthatjazzleo/pvc-autoscaler-operator:<version you choose>"
```

#### TODO

Helm chart coming soon.


### Create a PodDiskInspector

Using the information from the previous steps, create a yaml file using the below template.

Then `kubectl apply -f` the yaml file.

One can replace the disk healthcheck sidecar image with their own image that implements the same functionality as the default image in the [healthcheckCmd](https://github.com/allthatjazzleo/pvc-autoscaler-operator/blob/main/cmd/healtcheck_cmd.go#L16)

```yaml
apiVersion: autoscaler.allthatjazzleo/v1alpha1
kind: PodDiskInspector
metadata:
  name: poddiskinspector-sample
  namespace: default
spec:
  # disk healthcheck image
  sidecarImage: "allthatjazzleo/pvc-autoscaler-operator:<latest version of operator>" # TODO
  pvcScaling:
    usedSpacePercentage: 80 # percentage of used space to trigger scaling
    increaseQuantity: 20% # percentage of increase in size, Either a percentage (e.g. 20%) or a resource storage quantity (e.g. 100Gi).
    cooldown: 6h # time to wait before scaling again because provider like AWS EBS has a 6 hour cooldown for api call
    maxSize: 16Ti # max size of pvc to scale
```

- Add the required annoations to the pod template spec in your pod, deployment, statefulset, or other crd that allow you to add annotations to the pod template.

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: demo
  annotations:
    pvc-autoscaler-operator.kubernetes.io/enabled: "true" # required, allow operator to add sidecar
    pvc-autoscaler-operator.kubernetes.io/operator-name: "poddiskinspector-sample" # required, allow operator to add sidecar
    pvc-autoscaler-operator.kubernetes.io/operator-namespace: "default" # required, allow operator to add sidecar
    pvc-autoscaler-operator.kubernetes.io/sidecar-image: "allthatjazzleo/pvc-autoscaler-operator:v0.0.1" # optional, allow operator to use a different image from the above crd spec
spec:
  containers:
  - name: nginx
    image: nginx:1.14.2
    ports:
    - containerPort: 80
    volumeMounts:
      - mountPath: "/data"
        name: demo-vol
  volumes:
    - name: demo-vol
      persistentVolumeClaim:
        claimName: demo
```

- [Optional] Add the following optional annotations to the PersistentVolumeClaim template metadata such that you can override and have different scaling configurations for pvc from crd spec.

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo
  annotations:
    pvc-autoscaler-operator.kubernetes.io/used-space-percentage: "80" # optional, override percentage of used space to trigger scaling
    pvc-autoscaler-operator.kubernetes.io/increase-quantity: "20%" # optional, override percentage of increase in size, Either a percentage (e.g. 20%) or a resource storage quantity (e.g. 100Gi).
    pvc-autoscaler-operator.kubernetes.io/cooldown: "6h" # optional, override time to wait before scaling again
    pvc-autoscaler-operator.kubernetes.io/max-size: "16Ti" # optional, override max size of pvc to scale
spec:
  storageClassName: "standard-rwo"
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 100Gi
```
