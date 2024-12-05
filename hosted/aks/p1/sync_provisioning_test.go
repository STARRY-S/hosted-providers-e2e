package p1_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("SyncProvisioning", func() {
	var cluster *management.Cluster
	var k8sVersion string
	BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
	})
	AfterEach(func() {
		if ctx.ClusterCleanup && (cluster != nil && cluster.ID != "") {
			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	When("a cluster is created", func() {
		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully Add NP from Azure and then from Rancher", func() {
			testCaseID = 224
			syncAddNodePoolFromAzureAndRancher(cluster, ctx.RancherAdminClient)
		})
	})

	When("a cluster is created for upgrade", func() {
		var availableUpgradeVersions []string

		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).NotTo(HaveOccurred())

			availableUpgradeVersions, err = helper.ListAKSAvailableVersions(ctx.RancherAdminClient, cluster.ID)
			Expect(err).To(BeNil())
		})

		It("should successfully Change k8s version from Azure should change the CP k8s version and list of available version for NPs", func() {
			testCaseID = 225
			upgradeCPK8sFromAzureAndNPFromRancherCheck(cluster, ctx.RancherAdminClient, k8sVersion, availableUpgradeVersions[0])
		})

		It("should sync changes from Azure console back to Rancher", func() {
			testCaseID = 302
			azureSyncCheck(cluster, ctx.RancherAdminClient, availableUpgradeVersions[0])
		})
	})
})
