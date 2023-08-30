package pvc

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/inject"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockStatusClient[T client.Object] struct {
	mu               sync.Mutex
	LastUpdateObject T
	UpdateCount      *int
	UpdateErr        *error
}

func (ms *mockStatusClient[T]) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	panic("implement me")
}

func (ms *mockStatusClient[T]) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ctx == nil {
		panic("nil context")
	}
	*ms.UpdateCount++
	ms.LastUpdateObject = obj.(T)
	return *ms.UpdateErr
}

func (ms *mockStatusClient[T]) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	panic("implement me")
}

type mockClient[T client.Object] struct {
	mu sync.Mutex

	Object       any
	GetObjectKey client.ObjectKey
	GetObjectErr error

	ObjectList  any
	GotListOpts []client.ListOption
	ListErr     error

	CreateCount      int
	LastCreateObject T
	CreatedObjects   []T

	DeleteCount int

	PatchCount      int
	LastPatchObject client.Object
	LastPatch       client.Patch

	LastUpdateObject T
	UpdateCount      int
	UpdateErr        error

	StatusClient mockStatusClient[T]
}

func (m *mockClient[T]) Get(ctx context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx == nil {
		panic("nil context")
	}
	m.GetObjectKey = key
	if m.Object == nil {
		return m.GetObjectErr
	}

	switch ref := obj.(type) {
	case *corev1.PersistentVolumeClaim:
		*ref = m.Object.(corev1.PersistentVolumeClaim)
	case *v1alpha1.PodDiskInspector:
		*ref = m.Object.(v1alpha1.PodDiskInspector)
	default:
		panic(fmt.Errorf("unknown Object type: %T", m.ObjectList))
	}
	return m.GetObjectErr
}

func (m *mockClient[T]) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx == nil {
		panic("nil context")
	}
	m.GotListOpts = opts

	if m.ObjectList == nil {
		return nil
	}

	switch ref := list.(type) {
	case *corev1.PodList:
		*ref = m.ObjectList.(corev1.PodList)
	case *corev1.PersistentVolumeClaimList:
		*ref = m.ObjectList.(corev1.PersistentVolumeClaimList)
	default:
		panic(fmt.Errorf("unknown ObjectList type: %T", m.ObjectList))
	}

	return m.ListErr
}

func (m *mockClient[T]) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx == nil {
		panic("nil context")
	}
	m.LastCreateObject = obj.(T)
	m.CreatedObjects = append(m.CreatedObjects, obj.(T))
	m.CreateCount++
	return nil
}

func (m *mockClient[T]) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx == nil {
		panic("nil context")
	}
	m.DeleteCount++
	return nil
}

func (m *mockClient[T]) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx == nil {
		panic("nil context")
	}
	m.UpdateCount++
	m.LastUpdateObject = obj.(T)
	return m.UpdateErr
}

func (m *mockClient[T]) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx == nil {
		panic("nil context")
	}
	m.PatchCount++
	m.LastPatchObject = obj
	m.LastPatch = patch
	return nil
}

func (m *mockClient[T]) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	panic("implement me")
}

func (m *mockClient[T]) Scheme() *runtime.Scheme {
	m.mu.Lock()
	defer m.mu.Unlock()

	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		panic(err)
	}
	return scheme
}

func (m *mockClient[T]) Status() client.StatusWriter {
	m.StatusClient.LastUpdateObject = m.LastUpdateObject
	m.StatusClient.UpdateCount = &m.UpdateCount
	m.StatusClient.UpdateErr = &m.UpdateErr
	return &m.StatusClient
}

func (m *mockClient[T]) RESTMapper() meta.RESTMapper {
	panic("implement me")
}

// ptr returns the pointer for any type.
// In k8s, many specs require a pointer to a scalar.
func ptr[T any](v T) *T {
	return &v
}

func appName(crd *v1alpha1.PodDiskInspector) string {
	return kube.ToName(crd.Name)
}

func instanceName(crd *v1alpha1.PodDiskInspector, ordinal int32) string {
	return kube.ToName(fmt.Sprintf("%s-%d", appName(crd), ordinal))
}

func pvcName(crd *v1alpha1.PodDiskInspector, ordinal int32) string {
	name := fmt.Sprintf("pvc-%s-%d", appName(crd), ordinal)
	return kube.ToName(name)
}

// mockPodBuilder builds corev1.Pods
type mockPodBuilder struct {
	crd *v1alpha1.PodDiskInspector
	pod *corev1.Pod
}

// NewMockPodBuilder returns a valid PodBuilder.
//
// Panics if any argument is nil.
func NewMockPodBuilder(crd *v1alpha1.PodDiskInspector) mockPodBuilder {
	if crd == nil {
		panic(errors.New("nil PodDiskInspector"))
	}

	pod := corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   crd.Namespace,
			Annotations: make(map[string]string),
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:           ptr(int64(1025)),
				RunAsGroup:          ptr(int64(1025)),
				RunAsNonRoot:        ptr(true),
				FSGroup:             ptr(int64(1025)),
				FSGroupChangePolicy: ptr(corev1.FSGroupChangeOnRootMismatch),
				SeccompProfile:      &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
			},
			Containers: []corev1.Container{
				// Main start container.
				{
					Name:  "nginx",
					Image: "nginx:1.14.2",
					Ports: []corev1.ContainerPort{
						{ContainerPort: 80, Protocol: corev1.ProtocolTCP},
					},
				},
			},
		},
	}
	pod.Annotations[kube.OperatorEnabled] = "true"

	return mockPodBuilder{
		crd: crd,
		pod: &pod,
	}
}

// WithOrdinal updates adds name and other metadata to the pod using "ordinal" which is the pod's
// ordered sequence. Pods have deterministic, consistent names similar to a StatefulSet instead of generated names.
func (b mockPodBuilder) WithOrdinalBuild(ordinal int32) (*corev1.Pod, error) {
	pod := b.pod.DeepCopy()
	name := instanceName(b.crd, ordinal)

	vol := "vol-sample"
	pod.Name = name

	pod.Spec.Volumes = []corev1.Volume{
		{
			Name: vol,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName(b.crd, ordinal)},
			},
		},
	}

	// Mounts required by all containers.
	mounts := []corev1.VolumeMount{
		{Name: vol, MountPath: "/tmp/pvc"},
	}

	// At this point, guaranteed to have at least 2 containers.
	pod.Spec.Containers[0].VolumeMounts = mounts

	// Inject healthcheck sidecar
	sidecar, err := inject.Sidecar(pod, b.crd.Spec.SidecarImage)
	if err != nil {
		return nil, err
	}

	pod.Spec.Containers = append(pod.Spec.Containers, sidecar)
	return pod, nil
}

// NopReporter is a no-op kube.Reporter.
type NopReporter struct{}

func (n NopReporter) Info(msg string, keysAndValues ...interface{})             {}
func (n NopReporter) Debug(msg string, keysAndValues ...interface{})            {}
func (n NopReporter) Error(err error, msg string, keysAndValues ...interface{}) {}
func (n NopReporter) RecordInfo(reason, msg string)                             {}
func (n NopReporter) RecordError(reason string, err error)                      {}
