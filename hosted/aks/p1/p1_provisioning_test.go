package p1_test

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"

	"github.com/epinio/epinio/acceptance/helpers/proc"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters"
	"github.com/rancher/shepherd/extensions/clusters/aks"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"
	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/aks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Provisioning", func() {
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

	It("should successfully Create a cluster in Region without AZ", func() {
		location = "ukwest"
		testCaseID = 275

		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

		updateFunc := func(aksConfig *aks.ClusterConfig) {
			aksConfig.ResourceLocation = location
			nodepools := *aksConfig.NodePools
			for i := range nodepools {
				nodepools[i].AvailabilityZones = nil
			}
		}
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		noAvailabilityZoneP0Checks(cluster, ctx.RancherAdminClient)
	})

	It("should successfully create cluster with multiple nodepools in multiple AZs", func() {
		testCaseID = 193
		updateFunc := func(aksConfig *aks.ClusterConfig) {
			nodepools := *aksConfig.NodePools
			npTemplate := nodepools[0]
			var updatedNodePools []aks.NodePool
			for i := 1; i <= 3; i++ {
				az := []string{strconv.Itoa(i)}
				for _, mode := range []string{"User", "System"} {
					updatedNodePools = append(updatedNodePools, aks.NodePool{
						AvailabilityZones:   &az,
						EnableAutoScaling:   npTemplate.EnableAutoScaling,
						MaxPods:             npTemplate.MaxPods,
						MaxCount:            npTemplate.MaxCount,
						MinCount:            npTemplate.MinCount,
						Mode:                mode,
						Name:                pointer.String(fmt.Sprintf("%s%d", strings.ToLower(mode), i)),
						NodeCount:           npTemplate.NodeCount,
						OrchestratorVersion: pointer.String(k8sVersion),
						OsDiskSizeGB:        npTemplate.OsDiskSizeGB,
						OsDiskType:          npTemplate.OsDiskType,
						OsType:              npTemplate.OsType,
						VMSize:              npTemplate.VMSize,
					})
				}
			}
			aksConfig.NodePools = &updatedNodePools
		}
		var err error
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		for _, np := range cluster.AKSConfig.NodePools {
			npName := *np.Name
			az := npName[len(npName)-1]
			Expect(*np.AvailabilityZones).To(Equal([]string{string(az)}))
		}
	})

	It("should be able to create a cluster with empty tag", func() {
		testCaseID = 205
		updateFunc := func(aksConfig *aks.ClusterConfig) {
			aksConfig.Tags["empty-tag"] = ""
		}
		var err error
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
		Expect(err).To(BeNil())
		Expect(cluster.AKSConfig.Tags).To(HaveKeyWithValue("empty-tag", ""))

		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		Eventually(func() bool {
			cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			// We wait until sync is complete Ref: https://github.com/rancher/aks-operator/issues/640
			return len(cluster.AKSConfig.Tags) == len(cluster.AKSStatus.UpstreamSpec.Tags)
		}, "2m", "5s").Should(BeTrue())
		Expect(cluster.AKSStatus.UpstreamSpec.Tags).To(HaveKeyWithValue("empty-tag", ""))
	})

	It("should be able to create cluster with container monitoring enabled", func() {
		// Refer: https://github.com/rancher/shepherd/issues/274
		testCaseID = 199
		updateFunc := func(aksConfig *aks.ClusterConfig) {
			aksConfig.Monitoring = pointer.Bool(true)
		}
		var err error
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
		Expect(err).To(BeNil())
		Expect(*cluster.AKSConfig.Monitoring).To(BeTrue())

		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		Expect(*cluster.AKSStatus.UpstreamSpec.Monitoring).To(BeTrue())
	})

	// TODO: Discuss why only one nodepool is taken into account
	XIt("updating a cluster while it is still provisioning", func() {
		// Blocked by: https://github.com/rancher/aks-operator/issues/667
		testCaseID = 222
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
		Expect(err).To(BeNil())

		Eventually(func() string {
			cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).NotTo(HaveOccurred())
			return cluster.State
		}, "1m", "1s").Should(ContainSubstring("provisioning"))

		// Wait until the cluster appears on cloud before updating it
		Eventually(func() bool {
			var existsOnCloud bool
			existsOnCloud, err = helper.ClusterExistsOnAzure(clusterName, clusterName)
			if err != nil && strings.Contains(err.Error(), "NotFound") {
				return false
			}
			return existsOnCloud
		}, "1m", "2s").Should(BeTrue())

		Expect(*cluster.AKSConfig.KubernetesVersion).To(Equal(k8sVersion))

		initialNPCount := len(cluster.AKSConfig.NodePools)
		cluster, err = helper.AddNodePool(cluster, 3, ctx.RancherAdminClient, false, false)
		Expect(err).To(BeNil())
		Expect(cluster.AKSConfig.NodePools).To(HaveLen(initialNPCount + 3))

		var upgradeK8sVersion string
		upgradeK8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, false)
		Expect(err).NotTo(HaveOccurred())
		GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

		cluster, err = helper.UpgradeClusterKubernetesVersion(cluster, upgradeK8sVersion, ctx.RancherAdminClient, false)
		Expect(err).To(BeNil())
		Expect(*cluster.AKSConfig.KubernetesVersion).To(Equal(upgradeK8sVersion))

		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		err = clusters.WaitClusterToBeUpgraded(ctx.RancherAdminClient, cluster.ID)
		Expect(err).To(BeNil())

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(cluster.AKSStatus.UpstreamSpec.NodePools).To(HaveLen(initialNPCount + 3))
		Expect(cluster.AKSStatus.UpstreamSpec.KubernetesVersion).To(Equal(upgradeK8sVersion))
	})

	It("create cluster with network policy: calico and plugin: kubenet", func() {
		testCaseID = 210
		updateFunc := func(aksConfig *aks.ClusterConfig) {
			aksConfig.NetworkPolicy = pointer.String("calico")
			aksConfig.NetworkPlugin = pointer.String("kubenet")
		}
		var err error
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		Expect(*cluster.AKSConfig.NetworkPolicy).To(Equal("calico"))
		Expect(*cluster.AKSConfig.NetworkPlugin).To(Equal("kubenet"))
		Expect(*cluster.AKSStatus.UpstreamSpec.NetworkPolicy).To(Equal("calico"))
		Expect(*cluster.AKSStatus.UpstreamSpec.NetworkPlugin).To(Equal("kubenet"))

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	XIt("should successfully create cluster with underscore in the name", func() {
		// Blocked by https://github.com/rancher/dashboard/issues/9416
		testCaseID = 261
		if ctx.ClusterCleanup {
			clusterName = namegen.AppendRandomString(fmt.Sprintf("%s_hp_ci", helpers.Provider))
		} else {
			testuser, _ := user.Current()
			clusterName = namegen.AppendRandomString(fmt.Sprintf("%s_%s_hp_ci", helpers.Provider, testuser.Username))
		}
		var err error
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	It("should successfully create cluster with custom nodepool parameters", func() {
		testCaseID = 209
		updateFunc := func(aksConfig *aks.ClusterConfig) {
			nodepools := *aksConfig.NodePools
			for i := range nodepools {
				az := []string{"3"}
				nodepools[i].AvailabilityZones = &az
				nodepools[i].OsDiskSizeGB = pointer.Int64(64)
				nodepools[i].NodeCount = pointer.Int64(3)
				nodepools[i].OsDiskType = "Ephemeral"
				nodepools[i].EnableAutoScaling = pointer.Bool(true)
				nodepools[i].MinCount = pointer.Int64(2)
				nodepools[i].MaxCount = pointer.Int64(6)
				nodepools[i].VMSize = "Standard_DS3_v2"
				nodepools[i].MaxPods = pointer.Int64(20)
				nodepools[i].MaxSurge = "2"
				nodepools[i].NodeLabels = map[string]string{"custom": "true"}
			}
		}
		var err error
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
		Expect(err).To(BeNil())
		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())
		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
	})

	When("a cluster with invalid config is created", func() {
		It("should fail to create 2 clusters with same name in 2 different resource groups", func() {
			testCaseID = 217
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			resourceGroup2 := namegen.AppendRandomString(helpers.ClusterNamePrefix)
			updateFunc := func(aksConfig *aks.ClusterConfig) {
				aksConfig.ResourceGroup = resourceGroup2
			}
			_, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
			Expect(err).ToNot(BeNil())
			Expect(err.Error()).To(ContainSubstring("cluster already exists"))
		})

		It("should fail to create a cluster with 0 nodecount", func() {
			testCaseID = 186
			updateFunc := func(aksConfig *aks.ClusterConfig) {
				nodepools := *aksConfig.NodePools
				for i := range nodepools {
					nodepools[i].NodeCount = pointer.Int64(0)
				}
				aksConfig.NodePools = &nodepools
			}
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).NotTo(HaveOccurred())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "agentPoolProfile.count was 0. It must be greater or equal to minCount:1 and less than or equal to maxCount:1000")
			}, "1m", "2s").Should(BeTrue())
		})

		It("should fail to create a cluster with 0 nodepool", func() {
			testCaseID = 187
			updateFunc := func(aksConfig *aks.ClusterConfig) {
				aksConfig.NodePools = &[]aks.NodePool{}
			}
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).NotTo(HaveOccurred())
				return cluster.Transitioning == "error" && cluster.TransitioningMessage == "at least one NodePool with mode System is required"
			}, "1m", "2s").Should(BeTrue())
		})

		It("should fail to create cluster with Nodepool Max pods per node 9", func() {
			testCaseID = 203
			updateFunc := func(aksConfig *aks.ClusterConfig) {
				nodepools := *aksConfig.NodePools
				for i := range nodepools {
					nodepools[i].MaxPods = pointer.Int64(9)
				}
				aksConfig.NodePools = &nodepools
			}
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).NotTo(HaveOccurred())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "InsufficientMaxPods")
			}, "1m", "2s").Should(BeTrue())

		})
	})

	When("a cluster is created", func() {
		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully update with new cloud credentials", func() {
			testCaseID = 221
			updateCloudCredentialsCheck(cluster, ctx.RancherAdminClient)
		})

		It("should fail to update with invalid (deleted) cloud credential and update when the cloud credentials becomes valid", func() {
			testCaseID = 299
			invalidateCloudCredentialsCheck(cluster, ctx.RancherAdminClient, ctx.CloudCredID)
		})

		It("should not be able to edit availability zone of a nodepool", func() {
			// Refer: https://github.com/rancher/aks-operator/issues/669
			testCaseID = 195
			originalNPMap := make(map[string][]string)
			newAZ := []string{"3"}
			updateFunc := func(cluster *management.Cluster) {
				nodepools := cluster.AKSConfig.NodePools
				for i := range nodepools {
					originalNPMap[*nodepools[i].Name] = *nodepools[i].AvailabilityZones
					nodepools[i].AvailabilityZones = &newAZ
				}
			}
			var err error
			cluster, err = helper.UpdateCluster(cluster, ctx.RancherAdminClient, updateFunc)
			Expect(err).To(BeNil())
			for _, np := range cluster.AKSConfig.NodePools {
				Expect(*np.AvailabilityZones).To(Equal(newAZ))
			}

			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).NotTo(HaveOccurred())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "Changing availability zones for node pool") && strings.Contains(cluster.TransitioningMessage, "is not permitted")
			}, "3m", "3s").Should(BeTrue())
		})

		It("should not delete the resource group when cluster is deleted", func() {
			testCaseID = 207
			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			// marking as nil so that AfterEach does not raise an error
			cluster = nil

			// wait until the cluster is deleted from cloud console
			Eventually(func() (exists bool) {
				exists, err = helper.ClusterExistsOnAzure(clusterName, clusterName)
				Expect(err).To(BeNil())
				return exists
			}, "5m", "5s").Should(BeFalse())

			// check that the resource group still exists
			var out string
			out, err = proc.RunW("az", "group", "show", "--subscription", os.Getenv("AKS_SUBSCRIPTION_ID"), "--name", clusterName)
			Expect(err).To(BeNil())
			Expect(out).To(ContainSubstring(fmt.Sprintf("\"name\": \"%s\"", clusterName)))
		})

		It("should be able to update autoscaling", func() {
			testCaseID = 176
			updateAutoScaling(cluster, ctx.RancherAdminClient)
		})

		It("should be able to update tags", func() {
			testCaseID = 177
			updateTagsCheck(cluster, ctx.RancherAdminClient)
		})

		It("should have cluster monitoring disabled by default", func() {
			testCaseID = 198
			Expect(cluster.AKSConfig.Monitoring).To(BeNil())
			Expect(cluster.AKSStatus.UpstreamSpec.Monitoring).To(BeNil())
		})

		It("should fail to change system nodepool count to 0", func() {
			testCaseID = 202
			updateSystemNodePoolCountToZeroCheck(cluster, ctx.RancherAdminClient)
		})

		It("should be able to update cluster monitoring", func() {
			testCaseID = 200
			updateMonitoringCheck(cluster, ctx.RancherAdminClient)
		})

		It("recreating a cluster while it is being deleted should recreate the cluster", func() {
			testCaseID = 219

			err := helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			// Wait until the cluster begins deletion process before recreating
			Eventually(func() bool {
				exists, err := helper.ClusterExistsOnAzure(clusterName, cluster.AKSConfig.ResourceGroup)
				Expect(err).To(BeNil())
				return exists
			}, "1m", "5s").Should(BeFalse())

			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())

			// wait until the error is visible on the provisioned cluster
			Eventually(func() bool {
				cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.State == "provisioning" && cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "an AKSClusterConfig exists with the same name")
			}, "30s", "2s").Should(BeTrue())

			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

	})

	// Refer: https://github.com/rancher/hosted-providers-e2e/issues/192
	It("should successfully create 2 clusters in the same RG", func() {
		testCaseID = 217
		rgName := namegen.AppendRandomString(helpers.ClusterNamePrefix + "-custom-rg")
		updateFunc := func(aksConfig *aks.ClusterConfig) {
			aksConfig.ResourceGroup = rgName
		}
		var wg sync.WaitGroup
		for i := 1; i <= 2; i++ {
			wg.Add(1)
			go func() {
				defer GinkgoRecover()
				defer wg.Done()
				clusterName := namegen.AppendRandomString(helpers.ClusterNamePrefix)
				cluster1, err := helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
				if err != nil {
					Fail(err.Error())
				}
				cluster1, err = helpers.WaitUntilClusterIsReady(cluster1, ctx.RancherAdminClient)
				if err != nil {
					Fail(err.Error())
				}
				err = helper.DeleteAKSHostCluster(cluster1, ctx.RancherAdminClient)
				if err != nil {
					Fail(err.Error())
				}
			}()
		}
		wg.Wait()
		// This is to delete the resource group
		err := helper.DeleteAKSClusteronAzure(rgName)
		Expect(err).To(BeNil())
	})

	When("a cluster is created for upgrade", func() {
		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, ctx.CloudCredID, location, true)
			Expect(err).NotTo(HaveOccurred())
			GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).NotTo(HaveOccurred())
		})

		It("NP cannot be upgraded to k8s version greater than CP k8s version", func() {
			testCaseID = 183
			npUpgradeToVersionGTCPCheck(cluster, ctx.RancherAdminClient)
		})
	})

	It("deleting a cluster while it is in creation state should delete it from rancher and cloud console", func() {
		testCaseID = 218
		var err error
		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, nil)
		Expect(err).To(BeNil())

		// Wait for the cluster to appear on cloud console before deleting it
		Eventually(func() bool {
			exists, err := helper.ClusterExistsOnAzure(clusterName, cluster.AKSConfig.ResourceGroup)
			// ignore the error that occurs when resource group or cluster could not be found
			if err != nil {
				if strings.Contains(err.Error(), fmt.Sprintf("Resource group '%s' could not be found", cluster.AKSConfig.ResourceGroup)) || strings.Contains(err.Error(), "not found") {
					err = nil
				}
			}
			Expect(err).To(BeNil())
			return exists
		}, "1m", "5s").Should(BeTrue())

		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(BeNil())
		err = helper.DeleteAKSHostCluster(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		// Wait until the cluster finishes provisioning and then begins deletion process
		Eventually(func() bool {
			exists, err := helper.ClusterExistsOnAzure(clusterName, cluster.AKSConfig.ResourceGroup)
			if err != nil {
				if strings.Contains(err.Error(), fmt.Sprintf("Resource group '%s' could not be found", cluster.AKSConfig.ResourceGroup)) || strings.Contains(err.Error(), "not found") {
					err = nil
				}
			}
			Expect(err).To(BeNil())
			return exists
		}, "10m", "10s").Should(BeFalse())

		// Keep the cluster variable as is so that there is no error in AfterEach; failed delete operation will return an empty cluster
		cluster, err = ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("should not be able to select NP K8s version; CP K8s version should take precedence", func() {
		testCaseID = 182

		k8sVersions, err := helper.ListSingleVariantAKSAllVersions(ctx.RancherAdminClient, ctx.CloudCredID, location)
		Expect(err).To(BeNil())
		Expect(len(k8sVersions)).To(BeNumerically(">=", 2))
		// CP > NP
		cpK8sVersion := k8sVersions[1]
		npK8sVersion := k8sVersions[0]

		GinkgoLogr.Info(fmt.Sprintf("Using NP K8s version: %s and CP K8s version: %s", npK8sVersion, cpK8sVersion))

		updateFunc := func(clusterConfig *aks.ClusterConfig) {
			nodePools := *clusterConfig.NodePools
			for i := range nodePools {
				nodePools[i].OrchestratorVersion = &npK8sVersion
			}
			*clusterConfig.NodePools = nodePools
		}

		cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, cpK8sVersion, location, updateFunc)
		Expect(err).To(BeNil())

		Expect(*cluster.AKSConfig.KubernetesVersion).To(Equal(cpK8sVersion))
		for _, np := range cluster.AKSConfig.NodePools {
			Expect(*np.OrchestratorVersion).To(Equal(cpK8sVersion))
		}

		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		Eventually(func() bool {
			GinkgoLogr.Info("Waiting for the k8s upgrade to appear in AKSStatus.UpstreamSpec...")
			clusterState, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
			Expect(err).To(BeNil())
			if *clusterState.AKSStatus.UpstreamSpec.KubernetesVersion != cpK8sVersion {
				return false
			}

			for _, np := range clusterState.AKSStatus.UpstreamSpec.NodePools {
				if *np.OrchestratorVersion != cpK8sVersion {
					return false
				}
			}
			return true
		}, "5m", "5s").Should(BeTrue(), "Failed while waiting for k8s upgrade.")
	})

	When("a cluster is created for with user and system mode nodepool", func() {
		BeforeEach(func() {
			updateFunc := func(clusterConfig *aks.ClusterConfig) {
				nodePools := *clusterConfig.NodePools
				npTemplate := nodePools[0]
				var updatedNodePools []aks.NodePool
				for _, mode := range []string{"User", "System"} {
					np := aks.NodePool{
						AvailabilityZones:   npTemplate.AvailabilityZones,
						EnableAutoScaling:   npTemplate.EnableAutoScaling,
						MaxPods:             npTemplate.MaxPods,
						MaxCount:            npTemplate.MaxCount,
						MinCount:            npTemplate.MinCount,
						Mode:                mode,
						Name:                pointer.String(fmt.Sprintf("%spool", strings.ToLower(mode))),
						NodeCount:           npTemplate.NodeCount,
						OrchestratorVersion: &k8sVersion,
						OsDiskSizeGB:        npTemplate.OsDiskSizeGB,
						OsDiskType:          npTemplate.OsDiskType,
						OsType:              npTemplate.OsType,
						VMSize:              npTemplate.VMSize,
					}
					updatedNodePools = append(updatedNodePools, np)

				}
				*clusterConfig.NodePools = updatedNodePools
			}
			var err error
			cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, updateFunc)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("should successfully create the cluster", func() {
			testCaseID = 189
			helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)

			Expect(len(cluster.AKSConfig.NodePools)).To(Equal(2))
			Expect(len(cluster.AKSStatus.UpstreamSpec.NodePools)).To(Equal(2))
		})

		XIt("should to able to delete a nodepool and add a new one with different availability zone", func() {
			// Blocked by: https://github.com/rancher/aks-operator/issues/667#issuecomment-2370798904
			testCaseID = 190
			// also covers testCaseID = 194
			deleteAndAddNpCheck(cluster, ctx.RancherAdminClient)
		})

		It("should not be able to remove system nodepool", func() {
			testCaseID = 191
			removeSystemNpCheck(cluster, ctx.RancherAdminClient)
		})

		It("should successfully edit System NodePool", func() {
			testCaseID = 204
			updateSystemNodePoolCheck(cluster, ctx.RancherAdminClient)
		})

		It("should successfully edit mode of the nodepool", func() {
			testCaseID = 230
			updateNodePoolModeCheck(cluster, ctx.RancherAdminClient)
		})
	})
	Context("Network Policy and plugin", func() {
		var (
			calicoPolicy  = "calico"
			kubenetPlugin = "kubenet"
			azure         = "azure"
			none          = "null"
			vnet          = os.Getenv("AKS_VNET")
			vnetRG        = os.Getenv("AKS_VNET_RG")
			subnet        = "default"
		)

		for _, data := range []struct {
			networkPlugin, networkPolicy, vnet string
			testCaseID                         int64
		}{
			{
				networkPlugin: kubenetPlugin,
				networkPolicy: calicoPolicy,
				testCaseID:    210,
			},
			{
				networkPlugin: azure,
				networkPolicy: calicoPolicy,
				vnet:          vnet,
				testCaseID:    211,
			},
			{
				networkPlugin: azure,
				networkPolicy: none,
				testCaseID:    212,
				vnet:          vnet,
			},
			{
				networkPlugin: azure,
				networkPolicy: azure,
				vnet:          vnet,
				testCaseID:    213,
			},
		} {
			data := data
			It(fmt.Sprintf("Create cluster with NetworkPolicy %s & Network plugin %s", data.networkPolicy, data.networkPlugin), func() {
				testCaseID = data.testCaseID
				createFunc := func(clusterConfig *aks.ClusterConfig) {
					clusterConfig.NetworkPlugin = &data.networkPlugin
					if data.networkPolicy != none {
						clusterConfig.NetworkPolicy = &data.networkPolicy
					}
					if data.vnet != "" {
						clusterConfig.VirtualNetwork = &data.vnet
						clusterConfig.Subnet = &subnet
						clusterConfig.VirtualNetworkResourceGroup = pointer.String(vnetRG)
					}
				}
				var err error
				cluster, err = helper.CreateAKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, location, createFunc)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
				helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
			})
		}
	})

})
