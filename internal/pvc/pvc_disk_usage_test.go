package pvc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/healthcheck"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type mockDiskUsager func(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error)

func (fn mockDiskUsager) DiskUsage(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error) {
	return fn(ctx, host)
}

func TestCollectDiskUsage(t *testing.T) {
	t.Parallel()

	type mockReader = mockClient[*corev1.Pod]

	ctx := context.Background()

	const namespace = "default"

	var crd v1alpha1.PodDiskInspector
	crd.Name = "poddiskinspector-sample"
	crd.Namespace = namespace
	crd.Spec.PVCScaling = &v1alpha1.PVCScalingSpec{
		UsedSpacePercentage: 80,
		IncreaseQuantity:    "20%",
		Cooldown:            metav1.Duration{Duration: 10 * time.Minute},
		MaxSize:             resource.MustParse("500Gi"),
	}

	builder := NewMockPodBuilder(&crd)
	validPods := lo.Map(lo.Range(3), func(_ int, index int) corev1.Pod {
		pod, err := builder.WithOrdinalBuild(int32(index))
		if err != nil {
			panic(err)
		}
		pod.Status.PodIP = fmt.Sprintf("10.0.0.%d", index)
		return *pod
	})

	t.Run("happy path", func(t *testing.T) {
		var reader mockReader
		reader.ObjectList = corev1.PodList{Items: validPods}
		reader.Object = corev1.PersistentVolumeClaim{
			Status: corev1.PersistentVolumeClaimStatus{
				Capacity: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("500Gi")},
			},
		}

		diskClient := mockDiskUsager(func(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error) {
			var free uint64
			var pvc string
			switch host {
			case "http://10.0.0.0":
				pvc = "pvc-poddiskinspector-sample-0"
				free = 900
			case "http://10.0.0.1":
				pvc = "pvc-poddiskinspector-sample-1"
				free = 500
			case "http://10.0.0.2":
				pvc = "pvc-poddiskinspector-sample-2"
				free = 15 // Tests rounding up
			default:
				panic(fmt.Errorf("unknown host: %s", host))
			}
			return []healthcheck.DiskUsageResponse{
				{
					PvcName:   pvc,
					AllBytes:  1000,
					FreeBytes: free,
				},
			}, nil
		})

		coll := NewDiskUsageCollector(diskClient, &reader)
		got, err := coll.CollectDiskUsage(ctx, &crd)

		require.NoError(t, err)
		require.Len(t, got, 3)

		require.Len(t, reader.GotListOpts, 1)
		var listOpt client.ListOptions
		for _, opt := range reader.GotListOpts {
			opt.ApplyToList(&listOpt)
		}
		require.Equal(t, "", listOpt.Namespace)
		require.Zero(t, listOpt.Limit)

		require.Equal(t, namespace, reader.GetObjectKey.Namespace)
		require.Contains(t, []string{"pvc-poddiskinspector-sample-0", "pvc-poddiskinspector-sample-1", "pvc-poddiskinspector-sample-2"}, reader.GetObjectKey.Name)

		sort.Slice(got, func(i, j int) bool {
			return got[i].Name < got[j].Name
		})

		result := got[0]
		require.Equal(t, "pvc-poddiskinspector-sample-0", result.Name)
		require.Equal(t, 10, result.PercentUsed)
		require.Equal(t, resource.MustParse("500Gi"), result.Capacity)

		result = got[1]
		require.Equal(t, "pvc-poddiskinspector-sample-1", result.Name)
		require.Equal(t, 50, result.PercentUsed)
		require.Equal(t, resource.MustParse("500Gi"), result.Capacity)

		result = got[2]
		require.Equal(t, "pvc-poddiskinspector-sample-2", result.Name)
		require.Equal(t, 99, result.PercentUsed) // Tests rounding to be close to output of `df`
		require.Equal(t, resource.MustParse("500Gi"), result.Capacity)
	})

	t.Run("no pods found", func(t *testing.T) {
		var reader mockReader
		diskClient := mockDiskUsager(func(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error) {
			panic("should not be called")
		})

		coll := NewDiskUsageCollector(diskClient, &reader)
		_, err := coll.CollectDiskUsage(ctx, &crd)

		require.Error(t, err)
		require.EqualError(t, err, "no pods found")
	})

	t.Run("list error", func(t *testing.T) {
		var reader mockReader
		reader.ObjectList = corev1.PodList{Items: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}, Status: corev1.PodStatus{PodIP: "10.0.0.1"}},
		}}
		reader.ListErr = errors.New("boom")
		diskClient := mockDiskUsager(func(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error) {
			panic("should not be called")
		})

		coll := NewDiskUsageCollector(diskClient, &reader)
		_, err := coll.CollectDiskUsage(ctx, &crd)

		require.Error(t, err)
		require.EqualError(t, err, "list pods: boom")
	})

	t.Run("partial disk client errors", func(t *testing.T) {
		var reader mockReader
		reader.ObjectList = corev1.PodList{Items: validPods}

		diskClient := mockDiskUsager(func(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error) {
			if host == "http://10.0.0.1" {
				return []healthcheck.DiskUsageResponse{}, errors.New("boom")
			}
			return []healthcheck.DiskUsageResponse{
				{
					AllBytes:  100,
					FreeBytes: 100,
				},
			}, nil
		})

		coll := NewDiskUsageCollector(diskClient, &reader)
		got, err := coll.CollectDiskUsage(ctx, &crd)

		require.NoError(t, err)
		require.Len(t, got, 2)

		gotNames := lo.Map(got, func(item PVCDiskUsage, _ int) string {
			return item.Name
		})
		require.NotContains(t, gotNames, "pvc-cosmoshub-1")
	})

	t.Run("disk client error", func(t *testing.T) {
		var reader mockReader
		reader.ObjectList = corev1.PodList{Items: []corev1.Pod{
			{ObjectMeta: metav1.ObjectMeta{Name: "1"}, Status: corev1.PodStatus{PodIP: "10.0.0.1"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "2"}, Status: corev1.PodStatus{PodIP: "10.0.0.2"}},
		}}

		diskClient := mockDiskUsager(func(ctx context.Context, host string) ([]healthcheck.DiskUsageResponse, error) {
			return []healthcheck.DiskUsageResponse{
				{
					Dir: "/some/dir",
				},
				{
					Dir: "/some/dir",
				},
			}, errors.New("boom")
		})

		var crd v1alpha1.PodDiskInspector

		coll := NewDiskUsageCollector(diskClient, &reader)
		_, err := coll.CollectDiskUsage(ctx, &crd)

		require.Error(t, err)
		require.Contains(t, err.Error(), "pod 1: boom")
		require.Contains(t, err.Error(), "pod 2: boom")
	})
}
