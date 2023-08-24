package inject

import (
	"fmt"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/healthcheck"
)

const healthCheckPort = healthcheck.Port

// SidecarInjector is a sidecar injector
func Sidecar(volMap map[string]string, image string) (corev1.Container, error) {
	if len(volMap) == 0 {
		return corev1.Container{}, fmt.Errorf("no PVCs to monitor")
	}

	// Mounts required by sidecar container.
	var mounts []corev1.VolumeMount
	var pvcNames []string
	for vol, pvc := range volMap {
		mountPath := filepath.Clean(healthcheck.Mount + "/" + pvc)
		mounts = append(mounts, corev1.VolumeMount{
			Name:      vol,
			MountPath: mountPath,
			ReadOnly:  true,
		})
		pvcNames = append(pvcNames, pvc)
	}

	return corev1.Container{
		Name: "diskhealthcheck",
		// Available images: https://github.com/allthatjazzleo/pvc-autoscaler-operator/packages
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/manager", "healthcheck", "--pvcs", strings.Join(pvcNames, ",")},
		VolumeMounts:    mounts,
		Ports:           []corev1.ContainerPort{{ContainerPort: healthCheckPort, Protocol: corev1.ProtocolTCP}},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("5m"),
				corev1.ResourceMemory: resource.MustParse("16Mi"),
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/disk",
					Port:   intstr.FromInt(healthCheckPort),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 1,
			TimeoutSeconds:      10,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
	}, nil
}
