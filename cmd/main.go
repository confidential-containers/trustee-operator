/*
Copyright Confidential Containers Contributors.

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
	"context"
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	configv1 "github.com/openshift/api/config/v1"
	consolev1 "github.com/openshift/api/console/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	openshifttls "github.com/openshift/controller-runtime-common/pkg/tls"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	confidentialcontainersorgv1alpha1 "github.com/confidential-containers/trustee-operator/api/v1alpha1"
	controller "github.com/confidential-containers/trustee-operator/internal/controller"
	utils "github.com/confidential-containers/trustee-operator/internal/controller/utils"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(confidentialcontainersorgv1alpha1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(consolev1.Install(scheme))
	utilruntime.Must(operatorv1.Install(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var secureMetrics bool
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Cancellable context: SecurityProfileWatcher cancels it when the TLS profile changes,
	// triggering a graceful shutdown so the operator restarts with the new profile.
	ctx, cancel := context.WithCancel(ctrl.SetupSignalHandler())
	defer cancel()

	cfg := ctrl.GetConfigOrDie()

	// Temporary client used to detect cluster type and fetch the initial TLS profile before the manager starts.
	tempClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create temporary client")
		os.Exit(1)
	}

	// Detect which OpenShift API capabilities the cluster serves. These are
	// independent: the TLS-profile handling below needs config.openshift.io,
	// while Route/NetworkPolicy handling needs route.openshift.io.
	caps, err := utils.DetectPlatformCapabilities(ctx, cfg)
	if err != nil {
		setupLog.Error(err, "unable to detect cluster type")
		os.Exit(1)
	}
	isOpenShift := caps.HasRouteAPI || caps.HasConfigAPI
	if isOpenShift {
		setupLog.Info("OpenShift cluster detected", "routeAPI", caps.HasRouteAPI, "configAPI", caps.HasConfigAPI)
	} else {
		setupLog.Info("Vanilla Kubernetes cluster detected")
	}

	// Without config.openshift.io the APIServer CR is absent, so skip the fetch
	// and leave tlsOptsList empty to use controller-runtime's TLS defaults. Gate
	// on HasConfigAPI (not general OpenShift-ness) so a cluster that serves only
	// route.openshift.io does not terminate startup here.
	var (
		tlsProfileSpec configv1.TLSProfileSpec
		tlsOptsList    []func(*tls.Config)
	)
	if caps.HasConfigAPI {
		tlsProfileSpec, err = openshifttls.FetchAPIServerTLSProfile(ctx, tempClient)
		if err != nil {
			setupLog.Error(err, "unable to fetch TLS profile from APIServer CR")
			os.Exit(1)
		}
		tlsOpt, unsupportedCiphers := openshifttls.NewTLSConfigFromProfile(tlsProfileSpec)
		if len(unsupportedCiphers) > 0 {
			setupLog.Info("TLS profile contains ciphers not supported by Go crypto/tls, ignoring them", "ciphers", unsupportedCiphers)
		}
		tlsOptsList = []func(*tls.Config){tlsOpt}
		setupLog.Info("Loaded TLS profile from cluster", "minTLSVersion", tlsProfileSpec.MinTLSVersion, "ciphersCount", len(tlsProfileSpec.Ciphers))
	} else {
		setupLog.Info("Non-OpenShift cluster detected, using controller-runtime TLS defaults")
	}

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOptsList,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// Configure webhook server with TLS profile (if webhooks are enabled in the future)
	webhookServerOptions := webhook.Options{
		Port:    9443,
		TLSOpts: tlsOptsList,
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhook.NewServer(webhookServerOptions),
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "178dc119.confidentialcontainers.org",
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
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&appsv1.Deployment{}: {
					Namespaces: map[string]cache.Config{
						controller.KbsOperatorNamespace: {},
					},
				},
				&corev1.Service{}: {
					Namespaces: map[string]cache.Config{
						controller.KbsOperatorNamespace: {},
					},
				},
				&networkingv1.NetworkPolicy{}: {
					Namespaces: map[string]cache.Config{
						controller.KbsOperatorNamespace: {},
					},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = controller.KbsOperatorNamespace
	}

	// Install SecurityProfileWatcher to detect TLS profile changes. This watches
	// the config.openshift.io APIServer CR, so gate on HasConfigAPI to match the
	// TLS-profile fetch above.
	if caps.HasConfigAPI {
		watcher := &openshifttls.SecurityProfileWatcher{
			Client:                mgr.GetClient(),
			InitialTLSProfileSpec: tlsProfileSpec,
			OnProfileChange: func(_ context.Context, oldProfile, newProfile configv1.TLSProfileSpec) {
				setupLog.Info("TLS security profile changed, triggering graceful restart",
					"oldMinTLSVersion", oldProfile.MinTLSVersion,
					"newMinTLSVersion", newProfile.MinTLSVersion,
					"oldCipherCount", len(oldProfile.Ciphers),
					"newCipherCount", len(newProfile.Ciphers))
				cancel() // Triggers graceful shutdown which will cause the pod to restart
			},
		}
		if err := watcher.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to set up TLS profile watcher")
			os.Exit(1)
		}
		setupLog.Info("SecurityProfileWatcher installed - will restart on TLS profile changes")
	}

	err = labelNamespace(ctx, mgr, namespace)
	if err != nil {
		setupLog.Error(err, "unable to add labels to namespace")
		os.Exit(1)
	}

	if err = (&controller.KbsConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		// NetworkPolicy handling (router ingress peer, openshift-dns vs
		// kube-system) depends specifically on the Route API, so gate it on
		// HasRouteAPI rather than general OpenShift-ness.
		IsOpenShift: caps.HasRouteAPI,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "KbsConfig")
		os.Exit(1)
	}

	if err = (&controller.TrusteeConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		// Route management requires the Route API; gate it on HasRouteAPI so
		// reconciliation does not fail on clusters without route.openshift.io.
		IsOpenShift: caps.HasRouteAPI,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "TrusteeConfig")
		os.Exit(1)
	}

	if isOpenShift {
		var ocpVersion string
		if caps.HasConfigAPI {
			ver, err := utils.GetOpenshiftVersion(ctx, tempClient)
			if err != nil {
				setupLog.Info("Could not detect OCP version for console plugin, OCP_VERSION will be empty", "err", err)
			} else {
				ocpVersion = ver
				setupLog.Info("Detected OCP version for console plugin", "version", ocpVersion)
			}
		}
		if utils.IsConsolePluginSupported(ocpVersion) {
			if err = (&controller.ConsolePluginReconciler{
				Client:            mgr.GetClient(),
				Scheme:            mgr.GetScheme(),
				OperatorCSVPrefix: "trustee-operator",
			}).SetupWithManager(mgr); err != nil {
				setupLog.Error(err, "unable to create controller", "controller", "ConsolePlugin")
				os.Exit(1)
			}
		} else {
			setupLog.Info("OCP version < 4.22 — console plugin controller will not be registered",
				"detectedVersion", ocpVersion)
		}
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	startErr := mgr.Start(ctx)

	// The manager has stopped: if the operator is being uninstalled, remove the
	// console plugin resources OLM cannot delete itself (cluster-scoped
	// ConsolePlugin CRs and the Console operator configuration). OLM deletes
	// the operator Deployment when the operator is removed, so this is the
	// only point where cleanup is guaranteed to run while the process is alive.
	// The Deployment name is needed to tell an uninstall from a restart under
	// OLMv1, which creates no CSV to look at.
	if isOpenShift {
		if err := controller.CleanupConsolePluginsOnUninstall(cfg, scheme, namespace,
			"trustee-operator", "trustee-operator-controller-manager", setupLog); err != nil {
			setupLog.Error(err, "failed to clean up console plugin resources on uninstall")
		}
	}

	if startErr != nil {
		setupLog.Error(startErr, "problem running manager")
		os.Exit(1)
	}
}

func labelNamespace(ctx context.Context, mgr manager.Manager, nsName string) error {

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: nsName,
		},
	}
	err := mgr.GetAPIReader().Get(ctx, client.ObjectKeyFromObject(ns), ns)
	if err != nil {
		setupLog.Error(err, "Unable to retrieve namespace details. Can't add label to the namespace")
		return err
	}

	setupLog.Info("Labelling Namespace")
	setupLog.Info("Labels: ", "Labels", ns.Labels)
	// Add namespace label to allow privilege pods via Pod Security Admission controller
	ns.Labels["pod-security.kubernetes.io/enforce"] = "privileged"
	ns.Labels["pod-security.kubernetes.io/audit"] = "privileged"
	ns.Labels["pod-security.kubernetes.io/warn"] = "privileged"

	return mgr.GetClient().Update(ctx, ns)
}
