package k8s_chart_support_upgrade_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
	. "github.com/rancher-sandbox/qase-ginkgo"
	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	nodestat "github.com/rancher/shepherd/extensions/nodes"
	"github.com/rancher/shepherd/extensions/workloads/pods"
	"github.com/rancher/shepherd/pkg/config"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var (
	ctx                     helpers.RancherContext
	clusterName, k8sVersion string
	testCaseID              int64
	zone                    = helpers.GetGKEZone()
	project                 = helpers.GetGKEProjectID()
	k                       = kubectl.New()
)

func TestK8sChartSupportUpgrade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "K8sChartSupportUpgrade Suite")
}

var _ = BeforeEach(func() {
	// For upgrade tests, the rancher version should not be an unreleased version (for e.g. 2.9-head)
	Expect(helpers.RancherFullVersion).To(SatisfyAll(Not(BeEmpty()), Not(ContainSubstring("devel"))))

	Expect(helpers.RancherUpgradeFullVersion).ToNot(BeEmpty())
	Expect(helpers.K8sUpgradedMinorVersion).ToNot(BeEmpty())
	Expect(helpers.Kubeconfig).ToNot(BeEmpty())

	By("Adding the necessary chart repos", func() {
		helpers.AddRancherCharts()
	})

	By(fmt.Sprintf("Installing Rancher Manager %s", helpers.RancherFullVersion), func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(helpers.RancherFullVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "", "")
		helpers.CheckRancherDeployments(k)
	})

	helpers.CommonSynchronizedBeforeSuite()
	ctx = helpers.CommonBeforeSuite()

	By("creating and using a more permanent token", func() {
		token, err := ctx.RancherAdminClient.Management.Token.Create(&management.Token{})
		Expect(err).NotTo(HaveOccurred())
		rancherConfig := new(rancher.Config)
		config.LoadConfig(rancher.ConfigurationFileKey, rancherConfig)
		rancherConfig.AdminToken = token.Token
		config.UpdateConfig(rancher.ConfigurationFileKey, rancherConfig)

		rancherAdminClient, err := rancher.NewClient(rancherConfig.AdminToken, ctx.Session)
		Expect(err).To(BeNil())
		ctx.RancherAdminClient = rancherAdminClient
	})

	clusterName = namegen.AppendRandomString(helpers.ClusterNamePrefix)

	var err error
	// For k8s chart support upgrade we want to begin with the default k8s version; we will upgrade rancher and then upgrade k8s to the default available there.
	k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCredID, zone, "", false)
	Expect(err).To(BeNil())
	GinkgoLogr.Info(fmt.Sprintf("Using GKE version %s for cluster %s", k8sVersion, clusterName))
})

var _ = AfterEach(func() {
	// The test must restore the env to its original state, so we install rancher back to its original version and uninstall the operator charts
	By(fmt.Sprintf("Installing Rancher back to its original version %s", helpers.RancherFullVersion), func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(helpers.RancherFullVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "", "")
		helpers.CheckRancherDeployments(k)
	})

	By("Uninstalling the existing operator charts", func() {
		helpers.UninstallOperatorCharts()
	})
})

var _ = ReportBeforeEach(func(report SpecReport) {
	// Reset case ID
	testCaseID = -1
})

var _ = ReportAfterEach(func(report SpecReport) {
	// Add result in Qase if asked
	Qase(testCaseID, report)
})

// commonChartSupportUpgrade runs the common checks required for testing chart support
func commonChartSupportUpgrade(ctx *helpers.RancherContext, cluster *management.Cluster, clusterName, rancherUpgradedVersion, k8sUpgradedVersion string) {
	helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

	var originalChartVersion string
	By("checking the chart version", func() {
		originalChartVersion = helpers.GetCurrentOperatorChartVersion()
		Expect(originalChartVersion).ToNot(BeEmpty())
		GinkgoLogr.Info("Original chart version: " + originalChartVersion)
	})

	By("upgrading rancher", func() {
		rancherChannel, rancherVersion, rancherHeadVersion := helpers.GetRancherVersions(rancherUpgradedVersion)
		helpers.InstallRancherManager(k, helpers.RancherHostname, rancherChannel, rancherVersion, rancherHeadVersion, "", "")
		helpers.CheckRancherDeployments(k)

		By("ensuring operator pods are also up", func() {
			Eventually(func() error {
				return k.WaitForNamespaceWithPod(helpers.CattleSystemNS, fmt.Sprintf("ke.cattle.io/operator=%s", helpers.Provider))
			}, tools.SetTimeout(4*time.Minute), 30*time.Second).Should(BeNil())
		})

		By("ensuring the rancher client is connected", func() {
			isConnected, err := ctx.RancherAdminClient.IsConnected()
			Expect(err).To(BeNil())
			Expect(isConnected).To(BeTrue())
		})
	})

	By("making sure the local cluster is ready", func() {
		const localClusterID = "local"
		By("checking all management nodes are ready", func() {
			err := nodestat.AllManagementNodeReady(ctx.RancherAdminClient, localClusterID, helpers.Timeout)
			Expect(err).To(BeNil())
		})

		By("checking all pods are ready", func() {
			podErrors := pods.StatusPods(ctx.RancherAdminClient, localClusterID)
			Expect(podErrors).To(BeEmpty())
		})
	})

	var upgradedChartVersion string

	By("checking the chart version and validating it is > the old version", func() {
		helpers.WaitUntilOperatorChartInstallation(originalChartVersion, "==", 1)
		upgradedChartVersion = helpers.GetCurrentOperatorChartVersion()
		GinkgoLogr.Info("Upgraded chart version: " + upgradedChartVersion)

	})

	By("making sure the downstream cluster is ready", func() {
		var err error
		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

		// since no changes have been made to the cluster so far, we need reinstantiate GKEConfig after fetching the cluster
		if helpers.IsImport {
			cluster.GKEConfig = cluster.GKEStatus.UpstreamSpec
		}
	})

	By(fmt.Sprintf("fetching a list of available k8s versions and ensuring v%s is present in the list and upgrading the cluster to it", k8sUpgradedVersion), func() {
		versions, err := helper.ListGKEAvailableVersions(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())
		Expect(versions).ToNot(BeEmpty())
		GinkgoLogr.Info(fmt.Sprintf("Available GKE versions: %v", versions))

		highestSupportedVersionByUI := helpers.HighestK8sMinorVersionSupportedByUI(ctx.RancherAdminClient)
		var latestVersion string
		for _, v := range versions {
			if strings.Contains(v, highestSupportedVersionByUI) {
				latestVersion = v
			}
		}
		Expect(latestVersion).To(ContainSubstring(k8sUpgradedVersion))
		Expect(helpers.VersionCompare(latestVersion, cluster.Version.GitVersion)).To(BeNumerically("==", 1))

		cluster, err = helper.UpgradeKubernetesVersion(cluster, latestVersion, ctx.RancherAdminClient, true, true, true)
		Expect(err).To(BeNil())
	})

	var downgradeVersion string
	By("fetching a value to downgrade to", func() {
		downgradeVersion = helpers.GetDowngradeOperatorChartVersion(upgradedChartVersion)
	})

	By("downgrading the chart version", func() {
		helpers.DowngradeProviderChart(downgradeVersion)
	})

	By("making a change to the cluster (scaling nodepool up) to validate functionality after chart downgrade", func() {
		configNodePools := *cluster.GKEConfig.NodePools
		initialNodeCount := *configNodePools[0].InitialNodeCount
		var err error
		cluster, err = helper.ScaleNodePool(cluster, ctx.RancherAdminClient, initialNodeCount+1, true, true)
		Expect(err).To(BeNil())
	})

	By("uninstalling the operator chart", func() {
		helpers.UninstallOperatorCharts()
	})

	By("making a change(adding a nodepool) to the cluster to re-install the operator and validating it is re-installed to the latest/upgraded version", func() {
		currentNodePoolNumber := len(*cluster.GKEConfig.NodePools)
		var err error
		cluster, err = helper.AddNodePool(cluster, ctx.RancherAdminClient, 1, "", false, false)
		Expect(err).To(BeNil())

		Expect(len(*cluster.GKEConfig.NodePools)).To(BeNumerically("==", currentNodePoolNumber+1))

		By("ensuring that the chart is re-installed to the latest/upgraded version", func() {
			helpers.WaitUntilOperatorChartInstallation(upgradedChartVersion, "", 0)
		})

		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())

		Eventually(func() int {
			GinkgoLogr.Info("Waiting for the total nodepool count to increase in GKEStatus.UpstreamSpec ...")
			cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			return len(*cluster.GKEStatus.UpstreamSpec.NodePools)
		}, tools.SetTimeout(12*time.Minute), 10*time.Second).Should(BeNumerically("==", currentNodePoolNumber+1))

	})

}
