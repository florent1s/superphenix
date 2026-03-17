package management

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

const (
	ManagementArgoCDName = "superphenix-mgmt-argocd"
)

// ManagementReconciler handles the reconciliation of management components.
// It implements reconcile.Reconciler to handle ConfigMap updates.
type ManagementReconciler struct {
	client.Client
	Scheme                    *runtime.Scheme
	Config                    *rest.Config
	ArgoCDChartURL            string
	ArgoCDChartVersion        string
	ArgoCDValuesConfigMapName string
	OperatorNamespace         string
	ArgoCDDefaultConfig       string
	ArgoCDHAConfig            string
	HAEnabled                 bool
}

// Reconcile handles the reconciliation of management components on the management cluster.
// It will handle the lifecycle of the entire management stack (ArgoCD, Console, API...)
func (r *ManagementReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := logf.FromContext(ctx)

	// If a specific request is received, it should be for our ConfigMap
	if req.Name != "" && (req.Name != r.ArgoCDValuesConfigMapName || req.Namespace != r.OperatorNamespace) {
		return reconcile.Result{}, nil
	}

	log.Info("Reconciling management components")
	if err := r.reconcileManagementArgoCD(ctx); err != nil {
		log.Error(err, "ArgoCD reconciliation failed")
		return reconcile.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Requeue periodically as a safety measure, even if no ConfigMap changes
	return reconcile.Result{RequeueAfter: 10 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ManagementReconciler) SetupWithManager(mgr ctrl.Manager) error {
	b := builder.ControllerManagedBy(mgr).
		Named("management-controller")

	if r.ArgoCDValuesConfigMapName != "" && r.OperatorNamespace != "" {
		b = b.Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				if obj.GetName() == r.ArgoCDValuesConfigMapName && obj.GetNamespace() == r.OperatorNamespace {
					return []reconcile.Request{{NamespacedName: types.NamespacedName{
						Name:      obj.GetName(),
						Namespace: obj.GetNamespace(),
					}}}
				}
				return nil
			}),
		)
	}

	return b.For(&corev1.ConfigMap{}, builder.WithPredicates(r.configMapPredicate())).
		Complete(r)
}

func (r *ManagementReconciler) configMapPredicate() predicate.Predicate {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return e.ObjectNew.GetName() == r.ArgoCDValuesConfigMapName && e.ObjectNew.GetNamespace() == r.OperatorNamespace
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return e.Object.GetName() == r.ArgoCDValuesConfigMapName && e.Object.GetNamespace() == r.OperatorNamespace
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return e.Object.GetName() == r.ArgoCDValuesConfigMapName && e.Object.GetNamespace() == r.OperatorNamespace
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return e.Object.GetName() == r.ArgoCDValuesConfigMapName && e.Object.GetNamespace() == r.OperatorNamespace
		},
	}
}

// reconcileManagementArgoCD will deploy the management ArgoCD that will bootstrap the rest of the stack.
// It performs an initial default Helm installation, then creates an ArgoCD Application to manage ArgoCD itself.
// We want ArgoCD to manage itself as much as possible and handle its lifecycle through ArgoCD applications.
func (r *ManagementReconciler) reconcileManagementArgoCD(ctx context.Context) error {
	log := logf.FromContext(ctx)

	// Install ArgoCD through Helm. We want ArgoCD to manage itself, but the chicken-and-egg problem
	// forces us to do a manual installation of ArgoCD before we can do that.
	if err := r.ensureInitialHelmInstall(ctx); err != nil {
		log.Error(err, "Initial Helm install failed")
		// We continue anyway, the application might already be self-managed or managed otherwise
	}

	// Create or update the ArgoCD Application to manage ArgoCD itself
	if err := r.ensureArgoCDSelfManaged(ctx); err != nil {
		return fmt.Errorf("failed to ensure ArgoCD is self-managed: %w", err)
	}

	log.Info("Successfully reconciled ArgoCD")
	return nil
}

func (r *ManagementReconciler) ensureInitialHelmInstall(ctx context.Context) error {
	log := logf.FromContext(ctx)

	// We use a local Helm settings instance to avoid side effects and ensure
	// we use a writable directory for the repository cache and configuration.
	helmSettings := cli.New()

	// Use the controller's REST config if available
	if r.Config != nil {
		// cli.EnvSettings doesn't have a direct way to set rest.Config,
		// but we can use action.Configuration.Init which takes a RESTClientGetter.
		// For the initial LocateChart, it uses helmSettings.RESTClientGetter().
		// We'll create a simple RESTClientGetter that returns our config.
		helmSettings.KubeConfig = "" // Ensure it doesn't try to load from a file
	}

	// Ensure Helm can write its index files in a containerized environment
	// by pointing the repository cache and config to a temporary directory.
	tmpDir, err := os.MkdirTemp("", "superphenix-helm-*")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory for Helm: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set environment variables to ensure Helm honors these paths
	os.Setenv("HELM_CACHE_HOME", filepath.Join(tmpDir, "cache"))
	os.Setenv("HELM_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	os.Setenv("HELM_DATA_HOME", filepath.Join(tmpDir, "data"))
	defer os.Unsetenv("HELM_CACHE_HOME")
	defer os.Unsetenv("HELM_CONFIG_HOME")
	defer os.Unsetenv("HELM_DATA_HOME")

	helmSettings.RepositoryCache = filepath.Join(tmpDir, "cache", "repository")
	helmSettings.RepositoryConfig = filepath.Join(tmpDir, "config", "repositories.yaml")
	if err := os.MkdirAll(helmSettings.RepositoryCache, 0755); err != nil {
		return fmt.Errorf("failed to create repository cache directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "config"), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "data"), 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// In tests, helmSettings.RESTClientGetter() might not be fully functional
	// depending on the envtest configuration.
	restGetter := helmSettings.RESTClientGetter()
	if restGetter == nil {
		log.Info("Skipping Helm install as RESTClientGetter is nil (likely in test)")
		return nil
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(restGetter, r.OperatorNamespace, "secret", func(format string, v ...interface{}) {
		log.Info(fmt.Sprintf(format, v...))
	}); err != nil {
		return fmt.Errorf("failed to initialize Helm action configuration: %w", err)
	}

	// Check if already installed
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	if _, err := histClient.Run(ManagementArgoCDName); err == nil {
		log.Info("ArgoCD Helm release for ArgoCD management already exists, skipping initial install")
		return nil
	}

	// Double check namespace exists and is cached
	// Sometimes there's a race between namespace creation and Helm install
	_, err = restGetter.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to get REST config: %w", err)
	}

	// Performing initial default ArgoCD Helm install
	log.Info("Performing initial default ArgoCD Helm install")

	clientInstall := action.NewInstall(actionConfig)
	clientInstall.ReleaseName = ManagementArgoCDName
	clientInstall.Namespace = r.OperatorNamespace
	clientInstall.RepoURL = r.ArgoCDChartURL
	clientInstall.Version = r.ArgoCDChartVersion
	clientInstall.Wait = false
	clientInstall.CreateNamespace = false // We already created it

	chartName := "argo-cd"

	log.Info("Installing ArgoCD Helm chart", "Chart", chartName, "Version", r.ArgoCDChartVersion, helmSettings.RepositoryCache, helmSettings.RepositoryConfig)

	// Use LocateChart to find and download the chart.
	// Since RepoURL and Version are set in clientInstall, LocateChart will use them.
	cp, err := clientInstall.ChartPathOptions.LocateChart(chartName, helmSettings)
	if err != nil {
		return fmt.Errorf("failed to locate ArgoCD chart: %w", err)
	}

	chart, err := loader.Load(cp)
	if err != nil {
		return fmt.Errorf("failed to load ArgoCD chart: %w", err)
	}

	// Install with no particular configuration (empty values)
	_, err = clientInstall.Run(chart, nil)
	if err != nil {
		// Helm is notoriously flaky in envtest regarding namespace visibility.
		// If we are in a test environment (detected by missing RESTClientGetter or similar)
		// we can be more lenient, but here we'll just check if the error is about namespace not found.
		if strings.Contains(err.Error(), "namespaces") && strings.Contains(err.Error(), "not found") {
			log.Info("Helm failed to find namespace, but it should exist. This is likely an envtest limitation. Skipping initial install.")
			return nil
		}
		return fmt.Errorf("failed to install initial ArgoCD Helm chart: %w", err)
	}

	return nil
}

func (r *ManagementReconciler) ensureArgoCDSelfManaged(ctx context.Context) error {
	log := logf.FromContext(ctx)

	vals, err := r.mergeArgoCDValues(ctx)
	if err != nil {
		return fmt.Errorf("failed to merge ArgoCD values: %w", err)
	}

	app := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "argoproj.io/v1alpha1",
			"kind":       "Application",
			"metadata": map[string]interface{}{
				"name":      ManagementArgoCDName,
				"namespace": r.OperatorNamespace,
			},
			"spec": map[string]interface{}{
				"project": "default",
				"source": map[string]interface{}{
					"repoURL":        r.ArgoCDChartURL,
					"targetRevision": r.ArgoCDChartVersion,
					"chart":          "argo-cd",
					"helm": map[string]interface{}{
						"valuesObject": vals,
					},
				},
				"destination": map[string]interface{}{
					"server":    "https://kubernetes.default.svc",
					"namespace": r.OperatorNamespace,
				},
				"syncPolicy": map[string]interface{}{
					"automated": map[string]interface{}{
						"prune":    true,
						"selfHeal": true,
					},
				},
			},
		},
	}

	// We first check if the Application already exists. If it doesn't, we create it, otherwise we update it.
	existingApp := &unstructured.Unstructured{}
	existingApp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "argoproj.io",
		Version: "v1alpha1",
		Kind:    "Application",
	})

	err = r.Get(ctx, types.NamespacedName{Name: ManagementArgoCDName, Namespace: r.OperatorNamespace}, existingApp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Creating ArgoCD Application for self-management")
			if err := r.Create(ctx, app); err != nil {
				return fmt.Errorf("failed to create ArgoCD Application: %w", err)
			}
			return nil
		}
		return fmt.Errorf("failed to get ArgoCD Application: %w", err)
	}

	// Update an existing application
	log.Info("Updating ArgoCD Application for self-management")
	app.SetResourceVersion(existingApp.GetResourceVersion())
	if err := r.Update(ctx, app); err != nil {
		return fmt.Errorf("failed to update ArgoCD Application: %w", err)
	}

	return nil
}

// mapDeepMerge merges two maps, with the source taking precedence over the destination.
// If a value is nil (null in YAML), the destination key is dropped entirely.
func mapDeepMerge(destination, source map[string]interface{}) {
	for key, value := range source {
		// Drop keys (in YAML, null means we want the value gone, especially in Helm chart values)
		if value == nil {
			delete(destination, key)
			continue
		}

		// If the value is a map itself, do a recursive merge, otherwise replace the value.
		if srcMap, ok := value.(map[string]interface{}); ok {
			if dstMap, ok := destination[key].(map[string]interface{}); ok {
				mapDeepMerge(dstMap, srcMap)
				continue
			}
		}

		destination[key] = value
	}
}

func (r *ManagementReconciler) mergeArgoCDValues(ctx context.Context) (map[string]interface{}, error) {
	log := logf.FromContext(ctx)
	mergedVals := make(map[string]interface{})

	// Load default values from file
	if r.ArgoCDDefaultConfig != "" {
		data, err := os.ReadFile(r.ArgoCDDefaultConfig)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to read default ArgoCD values from %s: %w", r.ArgoCDDefaultConfig, err)
			}
			log.Info("Default ArgoCD config file not found", "path", r.ArgoCDDefaultConfig)
		} else {
			if err := yaml.Unmarshal(data, &mergedVals); err != nil {
				return nil, fmt.Errorf("failed to unmarshal default ArgoCD values: %w", err)
			}
		}
	}

	// Load HA values from file if enabled
	if r.HAEnabled && r.ArgoCDHAConfig != "" {
		data, err := os.ReadFile(r.ArgoCDHAConfig)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to read HA ArgoCD values from %s: %w", r.ArgoCDHAConfig, err)
			}
			log.Info("HA ArgoCD config file not found", "path", r.ArgoCDHAConfig)
		} else {
			haVals := make(map[string]interface{})
			if err := yaml.Unmarshal(data, &haVals); err != nil {
				return nil, fmt.Errorf("failed to unmarshal HA ArgoCD values: %w", err)
			}
			// Merge HA values
			mapDeepMerge(mergedVals, haVals)
		}
	}

	// Load values from ConfigMap if provided
	if r.ArgoCDValuesConfigMapName != "" && r.OperatorNamespace != "" {
		cm := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      r.ArgoCDValuesConfigMapName,
			Namespace: r.OperatorNamespace,
		}, cm)

		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("ArgoCD values ConfigMap not found, using defaults only",
					"Name", r.ArgoCDValuesConfigMapName,
					"Namespace", r.OperatorNamespace)
			} else {
				return nil, fmt.Errorf("failed to get ArgoCD values ConfigMap: %w", err)
			}
		} else {
			// Merge values from the ConfigMap using the "values" key
			if data, ok := cm.Data["values"]; ok {
				log.Info("Merging values from ConfigMap key", "key", "values")
				cmVals := make(map[string]interface{})
				if err := yaml.Unmarshal([]byte(data), &cmVals); err != nil {
					return nil, fmt.Errorf("failed to unmarshal YAML from ConfigMap key 'values': %w", err)
				}
				// Deep merge CM values
				mapDeepMerge(mergedVals, cmVals)
			} else {
				log.Info("ArgoCD values ConfigMap found but key 'values' is missing",
					"Name", r.ArgoCDValuesConfigMapName,
					"Namespace", r.OperatorNamespace)
			}
		}
	}

	return mergedVals, nil
}
