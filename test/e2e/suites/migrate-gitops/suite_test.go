//go:build e2e
// +build e2e

/*
Copyright © 2023 - 2024 SUSE LLC

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

package migrate_gitops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/turtles/test/e2e"
	turtlesframework "github.com/rancher/turtles/test/framework"
	"github.com/rancher/turtles/test/testenv"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Test suite flags.
var (
	flagVals *e2e.FlagValues
)

// Test suite global vars.
var (
	// e2eConfig to be used for this test, read from configPath.
	e2eConfig *clusterctl.E2EConfig

	// clusterctlConfigPath to be used for this test, created by generating a clusterctl local repository
	// with the providers specified in the configPath.
	clusterctlConfigPath string

	// hostName is the host name for the Rancher Manager server.
	hostName string

	ctx = context.Background()

	setupClusterResult *testenv.SetupTestClusterResult
	giteaResult        *testenv.DeployGiteaResult
)

func init() {
	flagVals = &e2e.FlagValues{}
	e2e.InitFlags(flagVals)
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)

	ctrl.SetLogger(klog.Background())

	RunSpecs(t, "rancher-turtles-e2e-migrate-gitops")
}

var _ = BeforeSuite(func() {
	Expect(flagVals.ConfigPath).To(BeAnExistingFile(), "Invalid test suite argument. e2e.config should be an existing file.")
	Expect(os.MkdirAll(flagVals.ArtifactFolder, 0o755)).To(Succeed(), "Invalid test suite argument. Can't create e2e.artifacts-folder %q", flagVals.ArtifactFolder)
	Expect(flagVals.HelmBinaryPath).To(BeAnExistingFile(), "Invalid test suite argument. helm-binary-path should be an existing file.")
	Expect(flagVals.ChartPath).To(BeAnExistingFile(), "Invalid test suite argument. chart-path should be an existing file.")

	By(fmt.Sprintf("Loading the e2e test configuration from %q", flagVals.ConfigPath))
	e2eConfig = e2e.LoadE2EConfig(flagVals.ConfigPath)

	By(fmt.Sprintf("Creating a clusterctl config into %q", flagVals.ArtifactFolder))
	clusterctlConfigPath = e2e.CreateClusterctlLocalRepository(ctx, e2eConfig, filepath.Join(flagVals.ArtifactFolder, "repository"))

	hostName = e2eConfig.GetVariable(e2e.RancherHostnameVar)
	ingressType := testenv.NgrokIngress
	var customClusterProvider testenv.CustomClusterProvider

	if flagVals.UseEKS {
		customClusterProvider = testenv.EKSBootsrapCluster
		Expect(customClusterProvider).NotTo(BeNil(), "EKS custom cluster provider is required")
		ingressType = testenv.EKSNginxIngress
	}

	if flagVals.IsolatedMode {
		ingressType = testenv.CustomIngress
	}

	setupClusterResult = testenv.SetupTestCluster(ctx, testenv.SetupTestClusterInput{
		UseExistingCluster:    flagVals.UseExistingCluster,
		E2EConfig:             e2eConfig,
		ClusterctlConfigPath:  clusterctlConfigPath,
		Scheme:                e2e.InitScheme(),
		ArtifactFolder:        flagVals.ArtifactFolder,
		KubernetesVersion:     e2eConfig.GetVariable(e2e.KubernetesManagementVersionVar),
		IsolatedMode:          flagVals.IsolatedMode,
		HelmBinaryPath:        flagVals.HelmBinaryPath,
		CustomClusterProvider: customClusterProvider,
	})

	testenv.RancherDeployIngress(ctx, testenv.RancherDeployIngressInput{
		BootstrapClusterProxy:    setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:           flagVals.HelmBinaryPath,
		HelmExtraValuesPath:      filepath.Join(flagVals.HelmExtraValuesDir, "deploy-rancher-ingress.yaml"),
		IngressType:              ingressType,
		CustomIngress:            e2e.NginxIngress,
		CustomIngressNamespace:   e2e.NginxIngressNamespace,
		CustomIngressDeployment:  e2e.NginxIngressDeployment,
		IngressWaitInterval:      e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-rancher"),
		NgrokApiKey:              e2eConfig.GetVariable(e2e.NgrokApiKeyVar),
		NgrokAuthToken:           e2eConfig.GetVariable(e2e.NgrokAuthTokenVar),
		NgrokPath:                e2eConfig.GetVariable(e2e.NgrokPathVar),
		NgrokRepoName:            e2eConfig.GetVariable(e2e.NgrokRepoNameVar),
		NgrokRepoURL:             e2eConfig.GetVariable(e2e.NgrokUrlVar),
		DefaultIngressClassPatch: e2e.IngressClassPatch,
	})

	if flagVals.IsolatedMode {
		hostName = setupClusterResult.IsolatedHostName
	}

	rancherInput := testenv.DeployRancherInput{
		BootstrapClusterProxy:  setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:         flagVals.HelmBinaryPath,
		HelmExtraValuesPath:    filepath.Join(flagVals.HelmExtraValuesDir, "deploy-rancher.yaml"),
		InstallCertManager:     true,
		CertManagerChartPath:   e2eConfig.GetVariable(e2e.CertManagerPathVar),
		CertManagerUrl:         e2eConfig.GetVariable(e2e.CertManagerUrlVar),
		CertManagerRepoName:    e2eConfig.GetVariable(e2e.CertManagerRepoNameVar),
		RancherChartRepoName:   e2eConfig.GetVariable(e2e.RancherRepoNameVar),
		RancherChartURL:        e2eConfig.GetVariable(e2e.RancherUrlVar),
		RancherChartPath:       e2eConfig.GetVariable(e2e.RancherPathVar),
		RancherVersion:         e2eConfig.GetVariable(e2e.RancherVersionVar),
		RancherHost:            hostName,
		RancherNamespace:       e2e.RancherNamespace,
		RancherPassword:        e2eConfig.GetVariable(e2e.RancherPasswordVar),
		RancherPatches:         [][]byte{e2e.RancherSettingPatch},
		RancherWaitInterval:    e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-rancher"),
		ControllerWaitInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers"),
		Variables:              e2eConfig.Variables,
	}
	if !flagVals.IsolatedMode && !flagVals.UseEKS {
		// i.e. we are using ngrok locally
		rancherInput.RancherIngressConfig = e2e.IngressConfig
		rancherInput.RancherServicePatch = e2e.RancherServicePatch
	}
	testenv.DeployRancher(ctx, rancherInput)

	rtInput := testenv.DeployRancherTurtlesInput{
		BootstrapClusterProxy:        setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:               flagVals.HelmBinaryPath,
		ChartPath:                    "https://rancher.github.io/turtles",
		CAPIProvidersYAML:            e2e.CapiProviders,
		Namespace:                    turtlesframework.DefaultRancherTurtlesNamespace,
		Version:                      "v0.6.0",
		WaitDeploymentsReadyInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers"),
		AdditionalValues:             map[string]string{},
	}
	testenv.DeployRancherTurtles(ctx, rtInput)

	testenv.DeployChartMuseum(ctx, testenv.DeployChartMuseumInput{
		HelmBinaryPath:        flagVals.HelmBinaryPath,
		ChartsPath:            flagVals.ChartPath,
		BootstrapClusterProxy: setupClusterResult.BootstrapClusterProxy,
		WaitInterval:          e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers"),
	})

	upgradeInput := testenv.UpgradeRancherTurtlesInput{
		BootstrapClusterProxy:        setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:               flagVals.HelmBinaryPath,
		Namespace:                    turtlesframework.DefaultRancherTurtlesNamespace,
		Image:                        fmt.Sprintf("ghcr.io/rancher/turtles-e2e-%s", runtime.GOARCH),
		Tag:                          "v0.0.1",
		WaitDeploymentsReadyInterval: e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers"),
		AdditionalValues:             rtInput.AdditionalValues,
	}

	// NOTE: this was the default previously in the chart locally and ok as
	// we where loading the image into kind manually.
	rtInput.AdditionalValues["rancherTurtles.imagePullPolicy"] = "Never"
	rtInput.AdditionalValues["rancherTurtles.features.addon-provider-fleet.enabled"] = "true"
	rtInput.AdditionalValues["rancherTurtles.features.managementv3-cluster.enabled"] = "false" // disable the default management.cattle.io/v3 controller

	upgradeInput.PostUpgradeSteps = append(upgradeInput.PostUpgradeSteps, func() {
		By("Waiting for CAAPF deployment to be available")
		framework.WaitForDeploymentsAvailable(ctx, framework.WaitForDeploymentsAvailableInput{
			Getter: setupClusterResult.BootstrapClusterProxy.GetClient(),
			Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name:      "caapf-controller-manager",
				Namespace: "rancher-turtles-system",
			}},
		}, e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-controllers")...)
	})

	testenv.UpgradeRancherTurtles(ctx, upgradeInput)

	giteaValues := map[string]string{
		"gitea.admin.username": e2eConfig.GetVariable(e2e.GiteaUserNameVar),
		"gitea.admin.password": e2eConfig.GetVariable(e2e.GiteaUserPasswordVar),
	}

	giteaServiceType := corev1.ServiceTypeNodePort
	if flagVals.UseEKS {
		giteaServiceType = corev1.ServiceTypeLoadBalancer
	}

	if flagVals.GiteaCustomIngress {
		giteaServiceType = corev1.ServiceTypeClusterIP
	}

	giteaResult = testenv.DeployGitea(ctx, testenv.DeployGiteaInput{
		BootstrapClusterProxy: setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:        flagVals.HelmBinaryPath,
		ChartRepoName:         e2eConfig.GetVariable(e2e.GiteaRepoNameVar),
		ChartRepoURL:          e2eConfig.GetVariable(e2e.GiteaRepoURLVar),
		ChartName:             e2eConfig.GetVariable(e2e.GiteaChartNameVar),
		ChartVersion:          e2eConfig.GetVariable(e2e.GiteaChartVersionVar),
		ValuesFilePath:        "../../data/gitea/values.yaml",
		Values:                giteaValues,
		RolloutWaitInterval:   e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-gitea"),
		ServiceWaitInterval:   e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-gitea-service"),
		AuthSecretName:        e2e.AuthSecretName,
		Username:              e2eConfig.GetVariable(e2e.GiteaUserNameVar),
		Password:              e2eConfig.GetVariable(e2e.GiteaUserPasswordVar),
		ServiceType:           giteaServiceType,
		CustomIngressConfig:   e2e.GiteaIngress,
		Variables:             e2eConfig.Variables,
	})
})

var _ = AfterSuite(func() {
	testenv.UninstallGitea(ctx, testenv.UninstallGiteaInput{
		BootstrapClusterProxy: setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:        flagVals.HelmBinaryPath,
		DeleteWaitInterval:    e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-gitea-uninstall"),
	})

	testenv.UninstallRancherTurtles(ctx, testenv.UninstallRancherTurtlesInput{
		BootstrapClusterProxy: setupClusterResult.BootstrapClusterProxy,
		HelmBinaryPath:        flagVals.HelmBinaryPath,
		Namespace:             turtlesframework.DefaultRancherTurtlesNamespace,
		DeleteWaitInterval:    e2eConfig.GetIntervals(setupClusterResult.BootstrapClusterProxy.GetName(), "wait-turtles-uninstall"),
	})

	testenv.CleanupTestCluster(ctx, testenv.CleanupTestClusterInput{
		SetupTestClusterResult: *setupClusterResult,
		SkipCleanup:            flagVals.SkipCleanup,
		ArtifactFolder:         flagVals.ArtifactFolder,
	})
})

func shortTestOnly() bool {
	return GinkgoLabelFilter() == e2e.ShortTestLabel
}

func localTestOnly() bool {
	return GinkgoLabelFilter() == e2e.LocalTestLabel
}
