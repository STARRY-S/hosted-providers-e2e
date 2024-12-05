package p1_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/eks"
	namegen "github.com/rancher/shepherd/pkg/namegenerator"

	"k8s.io/utils/pointer"

	"github.com/rancher/hosted-providers-e2e/hosted/eks/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P1Provisioning", func() {
	var (
		cluster    *management.Cluster
		k8sVersion string
	)

	var _ = BeforeEach(func() {
		var err error
		k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
		Expect(err).To(BeNil())
		GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
	})

	AfterEach(func() {
		if ctx.ClusterCleanup && (cluster != nil && cluster.ID != "") {
			err := helper.DeleteEKSHostCluster(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		} else {
			GinkgoLogr.Info(fmt.Sprintf("Skipping downstream cluster deletion: %s", clusterName))
		}
	})

	Context("Provisioning/Editing a cluster with invalid config", func() {

		It("should error out to provision a cluster with no nodegroups", func() {
			testCaseID = 141

			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				*clusterConfig.NodeGroupsConfig = nil
			}

			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "Cluster must have at least one managed nodegroup or one self-managed node")
			}, "10m", "30s").Should(BeTrue())

		})

		It("should fail to provision a cluster with duplicate nodegroup names", func() {
			testCaseID = 255

			var err error
			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				var updatedNodeGroupsList []eks.NodeGroupConfig
				*clusterConfig, err = helper.AddNodeGroupToConfig(*clusterConfig, 2)
				Expect(err).To(BeNil())

				for _, ng := range *clusterConfig.NodeGroupsConfig {
					ng.NodegroupName = pointer.String("duplicate")
					updatedNodeGroupsList = append([]eks.NodeGroupConfig{ng}, updatedNodeGroupsList...)
				}
				*clusterConfig.NodeGroupsConfig = updatedNodeGroupsList
			}
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				// checking for both the messages since different operator version shows different messages. To be removed once the message is updated.
				// New Message: NodePool names must be unique within the [c-dnzzk] cluster to avoid duplication
				return cluster.Transitioning == "error" && (strings.Contains(cluster.TransitioningMessage, "is not unique within the cluster") || strings.Contains(cluster.TransitioningMessage, "NodePool names must be unique"))
			}, "1m", "3s").Should(BeTrue())
		})

		It("Fail to create cluster with different k8s versions on control plane and on nodegroup", func() {
			testCaseID = 127

			k8sVersions, err := helper.ListEKSAllVersions(ctx.RancherAdminClient)
			Expect(err).To(BeNil())

			cpK8sVersion := k8sVersions[1]
			ngK8sVersion := k8sVersions[0]

			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				var updatedNodeGroupsList []eks.NodeGroupConfig
				for _, ng := range *clusterConfig.NodeGroupsConfig {
					ng.Version = pointer.String(ngK8sVersion)
					updatedNodeGroupsList = append([]eks.NodeGroupConfig{ng}, updatedNodeGroupsList...)
				}
				*clusterConfig.NodeGroupsConfig = updatedNodeGroupsList
			}

			GinkgoLogr.Info(fmt.Sprintf("Kubernetes version %s for control plane and %s for nodegroup on cluster %s", cpK8sVersion, ngK8sVersion, clusterName))
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, cpK8sVersion, region, updateFunc)
			Expect(err).To(BeNil())

			Eventually(func() bool {
				cluster, err := ctx.RancherAdminClient.Management.Cluster.ByID(cluster.ID)
				Expect(err).To(BeNil())
				return cluster.Transitioning == "error" && strings.Contains(cluster.TransitioningMessage, "version must match cluster")
			}, "1m", "3s").Should(BeTrue())
		})

		It("Fail to create cluster with only Security groups", func() {
			testCaseID = 120

			sg := []string{namegen.AppendRandomString("sg-"), namegen.AppendRandomString("sg-")}
			updateFunc := func(clusterConfig *eks.ClusterConfig) {
				clusterConfig.SecurityGroups = sg
			}
			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, updateFunc)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("subnets must be provided if security groups are provided")))
		})

		It("Fail to update both Public/Private access as false and invalid values of the access", func() {
			testCaseID = 147 // also covers 146

			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
			invalidEndpointCheck(cluster, ctx.RancherAdminClient)
			invalidAccessValuesCheck(cluster, ctx.RancherAdminClient)
		})
	})

	It("should successfully Provision EKS from Rancher with Enabled GPU feature", func() {
		testCaseID = 274
		var gpuNodeName = "gpuenabled"
		createFunc := func(clusterConfig *eks.ClusterConfig) {
			nodeGroups := *clusterConfig.NodeGroupsConfig
			gpuNG := nodeGroups[0]
			gpuNG.Gpu = pointer.Bool(true)
			gpuNG.NodegroupName = &gpuNodeName
			gpuNG.InstanceType = pointer.String("p2.xlarge")
			nodeGroups = append(nodeGroups, gpuNG)
			clusterConfig.NodeGroupsConfig = &nodeGroups
		}
		var err error
		cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, createFunc)
		Expect(err).To(BeNil())

		cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
		Expect(err).To(BeNil())

		helpers.ClusterIsReadyChecks(cluster, ctx.RancherAdminClient, clusterName)
		var amiID string
		amiID, err = helper.GetFromEKS(region, clusterName, "nodegroup", ".[].ImageID", "--name", gpuNodeName)
		Expect(err).To(BeNil())
		Expect(amiID).To(Equal("AL2_x86_64_GPU"))
	})

	Context("Upgrade testing", func() {
		var upgradeToVersion string

		BeforeEach(func() {
			var err error
			k8sVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, true)
			Expect(err).To(BeNil())
			upgradeToVersion, err = helper.GetK8sVersion(ctx.RancherAdminClient, false)
			Expect(err).To(BeNil())
			GinkgoLogr.Info(fmt.Sprintf("While provisioning, using kubernetes version %s for cluster %s", k8sVersion, clusterName))
		})

		When("a cluster is created", func() {

			BeforeEach(func() {
				var err error
				cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})

			It("Upgrade version of node group only", func() {
				testCaseID = 126
				upgradeNodeKubernetesVersionGTCPCheck(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})

			It("Update k8s version of cluster and add node groups", func() {
				testCaseID = 125
				upgradeCPAndAddNgCheck(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})

			// eks-operator/issues/752
			XIt("should successfully update a cluster while it is still in updating state", func() {
				testCaseID = 148
				updateClusterInUpdatingState(cluster, ctx.RancherAdminClient, upgradeToVersion)
			})
		})
	})

	When("a cluster is created", func() {

		BeforeEach(func() {
			var err error
			cluster, err = helper.CreateEKSHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, region, nil)
			Expect(err).To(BeNil())
			cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
			Expect(err).To(BeNil())
		})

		It("Update cluster logging types", func() {
			// https://github.com/rancher/eks-operator/issues/938
			testCaseID = 128
			updateLoggingCheck(cluster, ctx.RancherAdminClient)
		})

		It("Update Tags and Labels", func() {
			testCaseID = 131
			updateTagsAndLabels(cluster, ctx.RancherAdminClient)
		})
	})
})
