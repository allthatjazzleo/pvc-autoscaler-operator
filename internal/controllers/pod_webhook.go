package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/inject"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/kube"
)

var _ webhook.AdmissionHandler = (*podInterceptor)(nil)

// NewPodInterceptorWebhook creates a new pod mutating webhook to be registered
func NewPodInterceptorWebhook(c client.Client, decoder *admission.Decoder, recorder record.EventRecorder) webhook.AdmissionHandler {
	return &podInterceptor{
		client:   c,
		decoder:  decoder,
		recorder: recorder,
	}
}

// You need to ensure the path here match the path in the marker.
// +kubebuilder:webhook:path=/mutate-v1-pod-sidecar-injector,mutating=true,failurePolicy=ignore,groups="core",resources=pods,sideEffects=None,verbs=create;update,versions=v1,name=mpod.sidecar-injector.kb.io,admissionReviewVersions=v1

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,resourceNames=pvc-autoscaler-operator-webhook-server-cert,verbs=get;list;watch;update;patch;create
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,resourceNames=pvc-autoscaler-operator-mutating-webhook-configuration,verbs=get;list;watch;update

// podInterceptor label pods if Sidecar is specified in pod
type podInterceptor struct {
	client   client.Client
	decoder  *admission.Decoder
	recorder record.EventRecorder
}

// Handle adds a label to a generated pod if pod or namespace provide annotaion
func (d *podInterceptor) Handle(ctx context.Context, req admission.Request) admission.Response {
	// Get the CRD
	crd := new(v1alpha1.PodDiskInspector)
	reporter := kube.NewEventReporter(log.FromContext(ctx), d.recorder, nil)

	// got request for a pod
	reporter.Info("Got request for a pod")

	pod := &corev1.Pod{}
	err := d.decoder.Decode(req, pod)

	if err != nil {
		reporter.Error(err, "failed to decode pod")
		return admission.Errored(http.StatusBadRequest, err)
	}

	enabled := strings.ToLower(strings.TrimSpace(pod.Annotations[kube.OperatorEnabled]))
	name := strings.TrimSpace(pod.Annotations[kube.OperatorName])
	namespace := strings.TrimSpace(pod.Annotations[kube.OperatorNamespace])
	image := strings.TrimSpace(pod.Annotations[kube.OperatorImage])

	if enabled == "true" && name != "" && namespace != "" {
		key := client.ObjectKey{Name: name, Namespace: namespace}
		if err := d.client.Get(ctx, key, crd); err != nil {
			msg := "no CRD found for the operator, don't do anything"
			return admission.Allowed(msg)
		}
		reporter = reporter.UpdateResource(crd)

		if image == "" {
			image = crd.Spec.SidecarImage
		}

		// Add healtcheck sidecar if pod doesn't have one named "diskhealthcheck"
		for _, container := range pod.Spec.Containers {
			if container.Name == "diskhealthcheck" {
				return admission.Allowed("no action needed")
			}
		}

		// Inject healthcheck sidecar
		sidecar, err := inject.Sidecar(pod, image)
		if err != nil {
			reporter.RecordError("InjectHealthcheckSidecar", err)
			return admission.Allowed("no pvc to monitor, no action")
		}
		pod.Spec.Containers = append(pod.Spec.Containers, sidecar)

		marshaledPod, err := json.Marshal(pod)
		if err != nil {
			reporter.RecordError("InjectHealthcheckSidecar", err)
			return admission.Errored(http.StatusInternalServerError, err)
		}
		reporter.RecordInfo("InjectHealthcheckSidecar", fmt.Sprintf("Successfully injected healthcheck sidecar for %v", pod.Name))
		return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
	}
	return admission.Allowed("no action needed")
}

// podInterceptor implements admission.DecoderInjector.
// A decoder will be automatically injected.

// InjectDecoder injects the decoder.
func (d *podInterceptor) InjectDecoder(decoder *admission.Decoder) error {
	d.decoder = decoder
	return nil
}
