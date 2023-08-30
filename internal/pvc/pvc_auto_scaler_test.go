package pvc

import (
	"context"
	"testing"
	"time"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestProcessPVCResize(t *testing.T) {
	t.Parallel()

	var nopReporter NopReporter

	ctx := context.Background()

	type mockReader = mockClient[*v1alpha1.PodDiskInspector]

	t.Run("happy path", func(t *testing.T) {
		var reader mockReader
		var (
			capacity  = resource.MustParse("100Gi")
			cooldown  = metav1.Duration{Duration: 10 * time.Minute}
			stubNow   = time.Now()
			zeroQuant resource.Quantity
		)
		const (
			usedSpacePercentage = 80
			name                = "auto-scale-test"
			namespace           = "default"
		)

		for _, tt := range []map[string]struct {
			Increase    string
			Max         resource.Quantity
			Want        resource.Quantity
			Capability  resource.Quantity
			PercentUsed int
		}{
			{
				"pvc-0": {
					"20Gi",
					resource.MustParse("500Gi"),
					resource.MustParse("120Gi"),
					capacity,
					80,
				},
				"pvc-1": {
					"20Gi",
					resource.MustParse("500Gi"),
					resource.MustParse("100Gi"),
					capacity,
					79,
				},
			},
			{
				"pvc-0": {
					"10%",
					resource.MustParse("500Gi"),
					resource.MustParse("110Gi"),
					capacity,
					80,
				},
				"pvc-1": {
					"10%",
					resource.MustParse("500Gi"),
					resource.MustParse("110Gi"),
					capacity,
					81,
				},
			},
			// Weird user input cases
			{
				"pvc-0": {
					"1",
					zeroQuant,
					*resource.NewQuantity(capacity.Value()+1, resource.BinarySI),
					capacity,
					80,
				},
				"pvc-1": {
					"1",
					zeroQuant,
					resource.MustParse("100Gi"),
					capacity,
					79,
				},
			},
		} {
			var crd v1alpha1.PodDiskInspector
			crd.Name = name
			crd.Namespace = namespace
			crd.Spec.PVCScaling = &v1alpha1.PVCScalingSpec{
				UsedSpacePercentage: usedSpacePercentage,
				Cooldown:            cooldown,
			}
			crd.Status.PVCScalingStatus = make(map[string]v1alpha1.ScalingStatus)

			usage := make([]PVCDiskUsage, 0, len(tt))

			for pvc, st := range tt {
				key := client.ObjectKey{Namespace: namespace, Name: pvc}
				crd.Status.PVCScalingStatus[key.String()] = v1alpha1.ScalingStatus{
					RequestedSize: st.Capability,
				}
				scalingSpec := crd.Spec.PVCScaling.DeepCopy()
				scalingSpec.IncreaseQuantity = st.Increase
				usage = append(usage, PVCDiskUsage{
					Name:           pvc,
					Namespace:      namespace,
					Capacity:       st.Capability,
					PercentUsed:    st.PercentUsed,
					PVCScalingSpec: scalingSpec,
					pvc: &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      pvc,
							Namespace: namespace,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceStorage: st.Capability},
							},
						},
					},
				})
			}
			reader.Object = crd
			scaler := NewPVCAutoScaler(&reader)
			scaler.now = func() time.Time {
				return stubNow
			}

			err := scaler.ProcessPVCResize(ctx, &crd, lo.Shuffle(usage), nopReporter)

			require.NoError(t, err, tt)

			crd = *reader.StatusClient.LastUpdateObject

			for pvc := range tt {
				key := client.ObjectKey{Namespace: namespace, Name: pvc}

				require.True(t, tt[pvc].Want.Equal(crd.Status.PVCScalingStatus[key.String()].RequestedSize))
			}
		}
	})

	t.Run("does not exceed max", func(t *testing.T) {
		var reader mockReader
		var (
			capacity = resource.MustParse("100Gi")
			cooldown = metav1.Duration{Duration: 10 * time.Minute}
			stubNow  = time.Now()
		)
		const (
			usedSpacePercentage = 80
			name                = "auto-scale-test"
			namespace           = "default"
			maxSize             = "120Gi"
		)

		var crd v1alpha1.PodDiskInspector
		crd.Name = name
		crd.Namespace = namespace
		crd.Spec.PVCScaling = &v1alpha1.PVCScalingSpec{
			UsedSpacePercentage: usedSpacePercentage,
			Cooldown:            cooldown,
			MaxSize:             resource.MustParse(maxSize),
		}
		crd.Status.PVCScalingStatus = make(map[string]v1alpha1.ScalingStatus)

		pvcName := "pvc-0"
		key := client.ObjectKey{Namespace: namespace, Name: pvcName}
		crd.Status.PVCScalingStatus[key.String()] = v1alpha1.ScalingStatus{
			RequestedSize: capacity,
		}
		scalingSpec := crd.Spec.PVCScaling.DeepCopy()
		scalingSpec.IncreaseQuantity = "30%"
		usage := []PVCDiskUsage{
			{
				Name:           pvcName,
				Namespace:      namespace,
				Capacity:       capacity,
				PercentUsed:    90,
				PVCScalingSpec: scalingSpec,
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pvcName,
						Namespace: namespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: capacity},
						},
					},
				},
			},
		}
		reader.Object = crd
		scaler := NewPVCAutoScaler(&reader)
		scaler.now = func() time.Time {
			return stubNow
		}

		err := scaler.ProcessPVCResize(ctx, &crd, usage, nopReporter)

		require.NoError(t, err, "ProcessPVCResize should not return an error")

		crd = *reader.StatusClient.LastUpdateObject

		require.True(t, resource.MustParse(maxSize).Equal(crd.Status.PVCScalingStatus[key.String()].RequestedSize), "Exceeded maximum size")
	})

	t.Run("skip scaling pvc if RequestedAt is within cooldown", func(t *testing.T) {
		var reader mockReader
		var (
			capacity = resource.MustParse("100Gi")
			cooldown = metav1.Duration{Duration: 10 * time.Minute}
			stubNow  = time.Now()
		)
		const (
			usedSpacePercentage = 80
			name                = "auto-scale-test"
			namespace           = "default"
		)

		var crd v1alpha1.PodDiskInspector
		crd.Name = name
		crd.Namespace = namespace
		crd.Spec.PVCScaling = &v1alpha1.PVCScalingSpec{
			UsedSpacePercentage: usedSpacePercentage,
			Cooldown:            cooldown,
		}
		crd.Status.PVCScalingStatus = make(map[string]v1alpha1.ScalingStatus)

		pvcName := "pvc-0"
		key := client.ObjectKey{Namespace: namespace, Name: pvcName}
		crd.Status.PVCScalingStatus[key.String()] = v1alpha1.ScalingStatus{
			RequestedSize: capacity,
			RequestedAt:   metav1.NewTime(stubNow.Add(-5 * time.Minute)), // Set the RequestedAt to 5 minutes ago
		}
		scalingSpec := crd.Spec.PVCScaling.DeepCopy()
		scalingSpec.IncreaseQuantity = "30%"
		usage := []PVCDiskUsage{
			{
				Name:           pvcName,
				Namespace:      namespace,
				Capacity:       capacity,
				PercentUsed:    90,
				PVCScalingSpec: scalingSpec,
				pvc: &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      pvcName,
						Namespace: namespace,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: capacity},
						},
					},
				},
			},
		}
		reader.Object = crd
		scaler := NewPVCAutoScaler(&reader)
		scaler.now = func() time.Time {
			return stubNow
		}

		err := scaler.ProcessPVCResize(ctx, &crd, usage, nopReporter)

		require.NoError(t, err, "ProcessPVCResize should not return an error")

		require.Nil(t, reader.LastPatchObject)
	})
}
