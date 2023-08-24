/*
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
*/

package main

import (
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	// Add Pprof endpoints.
	"net/http"
	_ "net/http/pprof"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	autoscalerv1alpha1 "github.com/allthatjazzleo/pvc-autoscaler-operator/api/v1alpha1"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/controller"
	"github.com/allthatjazzleo/pvc-autoscaler-operator/internal/version"
	"github.com/go-logr/zapr"
	"github.com/pkg/profile"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(autoscalerv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	root := rootCmd()

	ctx := ctrl.SetupSignalHandler()

	if err := root.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

// root command flags
var (
	metricsAddr          string
	enableLeaderElection bool
	probeAddr            string
	profileMode          string
	logLevel             string
	logFormat            string
)

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Short:        "Run the operator",
		Use:          "manager",
		Version:      version.AppVersion(),
		RunE:         startManager,
		SilenceUsage: true,
	}

	root.Flags().StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	root.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	root.Flags().BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	root.Flags().StringVar(&profileMode, "profile", "", "Enable profiling and save profile to working dir. (Must be one of 'cpu', or 'mem'.)")
	root.Flags().StringVar(&logLevel, "log-level", "info", "Logging level one of 'error', 'info', 'debug'")
	root.Flags().StringVar(&logFormat, "log-format", "console", "Logging format one of 'console' or 'json'")

	if err := viper.BindPFlags(root.Flags()); err != nil {
		panic(err)
	}

	// Add subcommands here
	root.AddCommand(healthcheckCmd())
	root.AddCommand(&cobra.Command{
		Short: "Print the version",
		Use:   "version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("App Version:", version.AppVersion())
			fmt.Println("Docker Tag:", version.DockerTag())
		},
	})

	return root
}

func startManager(cmd *cobra.Command, args []string) error {
	go func() {
		setupLog.Info("Serving pprof endpoints at localhost:6060/debug/pprof")
		if err := http.ListenAndServe("localhost:6060", nil); err != nil {
			setupLog.Error(err, "Pprof server exited with error")
		}
	}()

	logger := zapLogger(logLevel, logFormat)
	defer func() { _ = logger.Sync() }()
	ctrl.SetLogger(zapr.NewLogger(logger))

	if profileMode != "" {
		defer profile.Start(profileOpts(profileMode)...).Stop()
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "e60c8444.allthatjazzleo",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	ctx := cmd.Context()

	if err = (&controller.PodDiskInspectorReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("pod-disk-inspector-controller"),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PodDiskInspector")
		return err
	}

	// register webhook
	srv := mgr.GetWebhookServer()
	decoder := admission.NewDecoder(mgr.GetScheme())
	srv.Register("/mutate-v1-pod-sidecar-injector", &webhook.Admission{
		Handler: controller.NewPodInterceptorWebhook(
			mgr.GetClient(),
			decoder,
			mgr.GetEventRecorderFor("pod-sidecar-injector"),
		),
	})

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		return err
	}

	setupLog.Info("starting PVC Autoscaler Operator manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}

func profileOpts(mode string) []func(*profile.Profile) {
	opts := []func(*profile.Profile){profile.ProfilePath("."), profile.NoShutdownHook}
	switch mode {
	case "cpu":
		return append(opts, profile.CPUProfile)
	case "mem":
		return append(opts, profile.MemProfile)
	default:
		panic(fmt.Errorf("unknown profile mode %q", mode))
	}
}
