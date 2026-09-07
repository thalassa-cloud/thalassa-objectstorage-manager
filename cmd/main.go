/*
Copyright 2026 Thalassa Cloud.

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
	"crypto/tls"
	"flag"
	"os"
	"strings"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/thalassa-cloud/client-go/iam"
	"github.com/thalassa-cloud/client-go/objectstorage"

	objectstoragev1 "github.com/thalassa-cloud/thalassa-objectstorage-manager/api/v1"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/controller"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/featuregates"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassa/bucket"
	"github.com/thalassa-cloud/thalassa-objectstorage-manager/internal/thalassaclient"
	// +kubebuilder:scaffold:imports
)

const (
	thalassaClientHint = "unable to create Thalassa client; set --organisation " +
		"(or THALASSA_ORGANISATION) and one of: " +
		"--thalassa-service-account-id (OIDC token exchange; uses in-cluster SA token path by default), " +
		"--thalassa-token-file or --thalassa-token, or --thalassa-client-id with " +
		"--thalassa-client-secret-file or --thalassa-client-secret (or matching THALASSA_* env vars)"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(objectstoragev1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var leaderElectionID string
	var leaderElectionNamespace string
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionID, "leader-election-id", "thalassa-objectstorage-manager.controllers.thalassa.cloud",
		"The name of the resource that leader election will use for holding the leader lock.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "",
		"Namespace in which to create the leader election lease. "+
			"Defaults to the namespace of the controller pod when running in-cluster.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")

	var (
		thalassaToken, thalassaTokenFile, thalassaClientID         string
		thalassaClientSecret, thalassaClientSecretFile             string
		thalassaURL, thalassaRegion, organisation, thalassaProject string
	)
	var thalassaServiceAccountID, thalassaSubjectTokenFile, thalassaSubjectToken string
	var thalassaOIDCTokenURL, thalassaAccessTokenLifetime string
	var thalassaInsecure bool
	flag.StringVar(&thalassaToken, "thalassa-token", "",
		"Thalassa personal access token (prefer --thalassa-token-file for production)")
	flag.StringVar(&thalassaTokenFile, "thalassa-token-file", "",
		"Path to file containing Thalassa personal access token (e.g. mounted Kubernetes secret)")
	flag.StringVar(&thalassaClientID, "thalassa-client-id", "", "Thalassa Cloud client ID (OAuth2 client credentials)")
	flag.StringVar(&thalassaClientSecret, "thalassa-client-secret", "",
		"Thalassa client secret (prefer --thalassa-client-secret-file for production)")
	flag.StringVar(&thalassaClientSecretFile, "thalassa-client-secret-file", "",
		"Path to file containing OAuth2 client secret")
	flag.BoolVar(&thalassaInsecure, "thalassa-insecure", false, "Use insecure connection to Thalassa Cloud API")
	flag.StringVar(&thalassaURL, "thalassa-url", "https://api.thalassa.cloud/", "Thalassa Cloud API URL")
	flag.StringVar(&thalassaRegion, "thalassa-region", "",
		"Thalassa Cloud region slug or identity; also used as --default-region when that flag/env is unset")
	flag.StringVar(&thalassaProject, "thalassa-project", "", "Optional Thalassa project scope")
	flag.StringVar(&organisation, "organisation", "", "Thalassa Cloud organisation ID or Slug")
	flag.StringVar(&thalassaServiceAccountID, "thalassa-service-account-id", "",
		"Thalassa service account ID for OIDC token exchange (federated workload identity); "+
			"uses Kubernetes SA token file by default")
	flag.StringVar(&thalassaSubjectTokenFile, "thalassa-subject-token-file", "",
		"Path to subject JWT for token exchange (default: in-cluster service account token path when unset)")
	flag.StringVar(&thalassaSubjectToken, "thalassa-subject-token", "",
		"Inline subject JWT for token exchange (alternative to subject token file)")
	flag.StringVar(&thalassaOIDCTokenURL, "thalassa-oidc-token-url", "",
		"OIDC token endpoint (default: {thalassa-url}/oidc/token)")
	flag.StringVar(&thalassaAccessTokenLifetime, "thalassa-access-token-lifetime", "",
		"Optional exchanged access token lifetime (e.g. 39600s)")

	var defaultRegion string
	flag.StringVar(&defaultRegion, "default-region", "",
		"Default Thalassa region (identity or slug) when Bucket spec.region is empty "+
			"(env: THALASSA_DEFAULT_REGION, REGION, or THALASSA_REGION / --thalassa-region)")
	var allowAllNamespacesSecretRef bool
	flag.BoolVar(&allowAllNamespacesSecretRef, "allow-all-namespaces-secret-ref", false,
		"SECURITY-SENSITIVE: Allow writeSecretToRef to target Secrets in any namespace. "+
			"Default false (same-namespace only). Enabling this requires cluster-wide Secret RBAC "+
			"and lets anyone who can create BucketAccess write Secrets outside their namespace.")
	var featureGatesRaw string
	flag.StringVar(&featureGatesRaw, "feature-gates", "",
		"Comma-separated feature gates (e.g. BucketAdoption=true). Default: all gates off.")
	var adoptionRequiredLabelsRaw string
	flag.StringVar(&adoptionRequiredLabelsRaw, "bucket-adoption-required-labels", "",
		"Comma-separated key=value labels a Thalassa bucket must have to be adopted when BucketAdoption is enabled. "+
			"Default: objectstorage.controllers.thalassa.cloud/adoptable=true")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Flags win; otherwise accept process env (e.g. VS Code/Cursor envFile .env).
	// Note: launch.json ${env:VAR} does not resolve values from envFile — use env fallbacks.
	thalassaToken = firstNonEmpty(thalassaToken, os.Getenv("THALASSA_TOKEN"))
	thalassaTokenFile = firstNonEmpty(thalassaTokenFile, os.Getenv("THALASSA_TOKEN_FILE"))
	thalassaClientID = firstNonEmpty(thalassaClientID, os.Getenv("THALASSA_CLIENT_ID"))
	thalassaClientSecret = firstNonEmpty(thalassaClientSecret, os.Getenv("THALASSA_CLIENT_SECRET"))
	thalassaClientSecretFile = firstNonEmpty(thalassaClientSecretFile, os.Getenv("THALASSA_CLIENT_SECRET_FILE"))
	thalassaURL = firstNonEmpty(thalassaURL, os.Getenv("THALASSA_URL"))
	thalassaRegion = firstNonEmpty(thalassaRegion, os.Getenv("THALASSA_REGION"))
	thalassaProject = firstNonEmpty(thalassaProject, os.Getenv("THALASSA_PROJECT"))
	organisation = firstNonEmpty(organisation, os.Getenv("THALASSA_ORGANISATION"), os.Getenv("ORGANISATION"))
	thalassaServiceAccountID = firstNonEmpty(thalassaServiceAccountID, os.Getenv("THALASSA_SERVICE_ACCOUNT_ID"))
	thalassaSubjectTokenFile = firstNonEmpty(thalassaSubjectTokenFile, os.Getenv("THALASSA_SUBJECT_TOKEN_FILE"))
	thalassaSubjectToken = firstNonEmpty(thalassaSubjectToken, os.Getenv("THALASSA_SUBJECT_TOKEN"))
	thalassaOIDCTokenURL = firstNonEmpty(thalassaOIDCTokenURL, os.Getenv("THALASSA_OIDC_TOKEN_URL"))
	thalassaAccessTokenLifetime = firstNonEmpty(thalassaAccessTokenLifetime, os.Getenv("THALASSA_ACCESS_TOKEN_LIFETIME"))
	// Prefer explicit default-region; accept THALASSA_REGION / --thalassa-region as fallback.
	defaultRegion = firstNonEmpty(defaultRegion, os.Getenv("THALASSA_DEFAULT_REGION"), os.Getenv("REGION"), thalassaRegion)
	if !thalassaInsecure {
		if v := strings.TrimSpace(os.Getenv("THALASSA_INSECURE")); v == "1" || strings.EqualFold(v, "true") {
			thalassaInsecure = true
		}
	}

	if thalassaToken != "" {
		viper.Set("thalassa-token", thalassaToken)
	}
	if thalassaTokenFile != "" {
		viper.Set("thalassa-token-file", thalassaTokenFile)
	}
	if thalassaClientID != "" {
		viper.Set("thalassa-client-id", thalassaClientID)
	}
	if thalassaClientSecret != "" {
		viper.Set("thalassa-client-secret", thalassaClientSecret)
	}
	if thalassaClientSecretFile != "" {
		viper.Set("thalassa-client-secret-file", thalassaClientSecretFile)
	}
	viper.Set("thalassa-insecure", thalassaInsecure)
	viper.Set("thalassa-url", thalassaURL)
	if thalassaRegion != "" {
		viper.Set("thalassa-region", thalassaRegion)
	}
	if thalassaProject != "" {
		viper.Set("thalassa-project", thalassaProject)
	}
	if organisation != "" {
		viper.Set("organisation", organisation)
	}
	if thalassaServiceAccountID != "" {
		viper.Set("thalassa-service-account-id", thalassaServiceAccountID)
	}
	if thalassaSubjectTokenFile != "" {
		viper.Set("thalassa-subject-token-file", thalassaSubjectTokenFile)
	}
	if thalassaSubjectToken != "" {
		viper.Set("thalassa-subject-token", thalassaSubjectToken)
	}
	if thalassaOIDCTokenURL != "" {
		viper.Set("thalassa-oidc-token-url", thalassaOIDCTokenURL)
	}
	if thalassaAccessTokenLifetime != "" {
		viper.Set("thalassa-access-token-lifetime", thalassaAccessTokenLifetime)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	gates, err := featuregates.Parse(featureGatesRaw)
	if err != nil {
		setupLog.Error(err, "invalid --feature-gates")
		os.Exit(1)
	}
	adoptionLabels, err := bucket.ParseLabelPairs(adoptionRequiredLabelsRaw)
	if err != nil {
		setupLog.Error(err, "invalid --bucket-adoption-required-labels")
		os.Exit(1)
	}
	if featuregates.Enabled(gates, featuregates.BucketAdoption) {
		setupLog.Info("feature gate enabled", "gate", featuregates.BucketAdoption, "requiredLabels", adoptionLabels)
	}

	if allowAllNamespacesSecretRef {
		setupLog.Info("WARNING: --allow-all-namespaces-secret-ref is enabled; " +
			"users who can create BucketAccess resources can cause the controller to write Secrets in other namespaces. " +
			"Keep this disabled in production unless you also grant cluster-wide Secret RBAC intentionally.")
	}

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServerOptions := webhook.Options{TLSOpts: tlsOpts}
	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)
		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}
	webhookServer := webhook.NewServer(webhookServerOptions)

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}
	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)
		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsServerOptions,
		WebhookServer:           webhookServer,
		HealthProbeBindAddress:  probeAddr,
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        leaderElectionID,
		LeaderElectionNamespace: leaderElectionNamespace,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	thalassaClient, err := thalassaclient.NewClientFromEnv()
	if err != nil {
		setupLog.Error(err, thalassaClientHint)
		os.Exit(1)
	}
	osClient, err := objectstorage.New(thalassaClient)
	if err != nil {
		setupLog.Error(err, "unable to create Thalassa object storage client")
		os.Exit(1)
	}
	iamClient, err := iam.New(thalassaClient)
	if err != nil {
		setupLog.Error(err, "unable to create Thalassa IAM client")
		os.Exit(1)
	}

	orgID := strings.TrimSpace(organisation)
	if orgID == "" {
		orgID = strings.TrimSpace(thalassaClient.GetOrganisationIdentity())
	}

	if err := (&controller.BucketReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		ObjectStorage:          osClient,
		DefaultRegion:          defaultRegion,
		BucketAdoptionEnabled:  featuregates.Enabled(gates, featuregates.BucketAdoption),
		AdoptionRequiredLabels: adoptionLabels,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Bucket")
		os.Exit(1)
	}
	if err := (&controller.BucketAccessReconciler{
		Client:                      mgr.GetClient(),
		Scheme:                      mgr.GetScheme(),
		IAM:                         iamClient,
		ObjectStorage:               osClient,
		OrganisationID:              orgID,
		AllowAllNamespacesSecretRef: allowAllNamespacesSecretRef,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BucketAccess")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
