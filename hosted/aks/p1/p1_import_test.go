package p1_test

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Import", func() {
	var (
		cluster    *management.Cluster
		k8sVersion string
	)
	BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup {
			if cluster != nil && cluster.ID != "" {
				err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			}
			err := helper.DeleteAKSClusteronAzure(clusterName)
			Expect(err).To(BeNil())

		} else {
			fmt.Println("Skipping downstream cluster deletion: ", clusterName)
		}
	})

	When("a cluster is created and imported", func() {
		BeforeEach(func() {
			err := helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully update with new cloud credentials", func() {
			testCaseID = 292
			updateCloudCredentialsCheck(cluster, ctx.RancherAdminClient)
		})

		It("should fail to update with invalid (deleted) cloud credential and update when the cloud credentials becomes valid", func() {
			testCaseID = 238
			invalidateCloudCredentialsCheck(cluster, ctx.RancherAdminClient, ctx.CloudCredID)
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 266
			updateAutoScaling(cluster, ctx.RancherAdminClient)
		})

		It("should be able to update tags", func() {
			testCaseID = 270
			updateTagsCheck(cluster, ctx.RancherAdminClient)
		})

		It("should fail to change system nodepool count to 0", func() {
			testCaseID = 290
			updateSystemNodePoolCountToZeroCheck(cluster, ctx.RancherAdminClient)
		})

		It("should be able to update cluster monitoring", func() {
			testCaseID = 271
			updateMonitoringCheck(cluster, ctx.RancherAdminClient)
		})

		It("should fail to reimport an imported cluster", func() {
			testCaseID = 235
			_, err := helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(HaveOccurred())

			Expect(err.Error()).To(ContainSubstring("cluster already exists for AKS cluster"))
		})

		It("should be possible to re-import a deleted cluster", func() {
			testCaseID = 239
			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			clusterID := cluster.ID
			Eventually(func() error {
				// Wait until the cluster no longer exists
				_, err := ctx.RancherAdminClient.Management.Cluster.ByID(clusterID)
				return err
			}, "10s", "1s").ShouldNot(BeNil())
			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})
	})

	It("should successfully Import a cluster in Region without AZ", func() {
		location = "ukwest"
		testCaseID = 276

		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

		err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
		Expect(err).To(BeNil())

		cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		noAvailabilityZoneP0Checks(cluster, ctx.RancherAdminClient)
	})

	It("should be able to register a cluster with no rbac", func() {
		testCaseID = 237
		err := helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels(), "--disable-rbac")
		Expect(err).To(BeNil())

		cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	When("a cluster with custom kubelet and os config is created and imported for upgrade", func() {
		var upgradeToVersion string
		BeforeEach(func() {
			kubeletConfigJsonData := `{"cpuManagerPolicy": "static", "cpuCfsQuota": true, "cpuCfsQuotaPeriod": "200ms", "imageGcHighThreshold": 90, "imageGcLowThreshold": 70, "topologyManagerPolicy": "best-effort", "allowedUnsafeSysctls": ["kernel.msg*","net.*"], "failSwapOn": false}`
			kubeletConfigDotJson, err := os.CreateTemp("", "custom-kubelet-*.json")
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(kubeletConfigDotJson.Name(), []byte(kubeletConfigJsonData), 0644)
			Expect(err).ToNot(HaveOccurred())

			osConfigJsonData := `{"transparentHugePageEnabled": "madvise", "transparentHugePageDefrag": "defer+madvise", "swapFileSizeMB": 1500, "sysctls": {"netCoreSomaxconn": 163849, "netIpv4TcpTwReuse": true, "netIpv4IpLocalPortRange": "32000 60000"}}`
			osConfigDotJson, err := os.CreateTemp("", "custom-os-*.json")
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(osConfigDotJson.Name(), []byte(osConfigJsonData), 0644)
			Expect(err).ToNot(HaveOccurred())

			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels(), "--kubelet-config", kubeletConfigDotJson.Name(), "--linux-os-config", osConfigDotJson.Name())
			Expect(err).To(BeNil())
			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			availableVersions, err := helper.ListAKSAvailableVersions(ctx.RancherAdminClient, cluster.ID)
			Expect(err).To(BeNil())
			upgradeToVersion = availableVersions[0]
		})

		It("should successfully upgrade the cluster", func() {
			testCaseID = 260
			var err error
			By("upgrading control plane version", func() {
				cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient, true)
				Expect(err).To(BeNil())
			})
			By("upgrading nodepool version", func() {
				cluster, err = helper.UpgradeNodeKubernetesVersion(cluster, upgradeToVersion, ctx.RancherAdminClient, true, true)
				Expect(err).To(BeNil())
			})
		})
	})

	When("a cluster is created with multiple nodepools", func() {
		BeforeEach(func() {
			var err error
			err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			// AKS already consists on one system pool, so we add a user pool
			err = helper.AddNodePoolOnAzure("userpool", clusterName, clusterName, "2", "--mode", "User")
			Expect(err).To(BeNil())

			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should not be able to remove system nodepool", func() {
			testCaseID = 267
			removeSystemNpCheck(cluster, ctx.RancherAdminClient)
		})

		It("should to able to delete a nodepool and add a new one", func() {
			testCaseID = 268
			deleteAndAddNpCheck(cluster, ctx.RancherAdminClient)
		})

		It("should successfully edit System NodePool", func() {
			testCaseID = 289
			updateSystemNodePoolCheck(cluster, ctx.RancherAdminClient)
		})

		It("should successfully edit mode of the nodepool", func() {
			testCaseID = 291
			updateNodePoolModeCheck(cluster, ctx.RancherAdminClient)
		})

	})

	When("a cluster is created and imported for upgrade", func() {
		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			err = helper.CreateAKSClusterOnAzure(location, clusterName, k8sVersion, "1", helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())

			cluster, err = helper.ImportAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, location, helpers.GetCommonMetadataLabels())
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("NP cannot be upgraded to k8s version greater than CP k8s version", func() {
			testCaseID = 269
			npUpgradeToVersionGTCPCheck(cluster, ctx.RancherAdminClient)
		})
	})

})
