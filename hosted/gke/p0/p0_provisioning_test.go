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

package p0_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/shepherd/clients/rancher"
	management "github.com/rancher/shepherd/clients/rancher/generated/management/v3"
	"github.com/rancher/shepherd/extensions/clusters/gke"

	"github.com/rancher/hosted-providers-e2e/hosted/gke/helper"
	"github.com/rancher/hosted-providers-e2e/hosted/helpers"
)

var _ = Describe("P0Provisioning", func() {
	for _, testData := range []struct {
		qaseID    int64
		isUpgrade bool
		testBody  func(cluster *management.Cluster, client *rancher.Client, clusterName string)
		testTitle string
	}{
		{
			qaseID:    8,
			isUpgrade: false,
			testBody:  p0NodesChecks,
			testTitle: "should successfully provision the zonal cluster & add, delete, scale nodepool",
		},
		{
			qaseID:    11,
			isUpgrade: true,
			testBody:  p0upgradeK8sVersionChecks,
			testTitle: "should be able to upgrade k8s version of the zonal provisioned cluster",
		},
		{
			qaseID:    300,
			isUpgrade: false,
			testBody:  p0NodesChecks,
			testTitle: "should successfully provision the regional cluster & add, delete, scale nodepool",
		},
		{
			qaseID:    301,
			isUpgrade: true,
			testBody:  p0upgradeK8sVersionChecks,
			testTitle: "should be able to upgrade k8s version of the regional provisioned cluster",
		},
	} {
		testData := testData
		When("a cluster is created", func() {
			BeforeEach(func() {
				if testData.isUpgrade && helpers.SkipUpgradeTests {
					Skip(helpers.SkipUpgradeTestsLog)
				}

				if strings.Contains(testData.testTitle, "regional") {
					zone = ""
					updateFunc = func(clusterConfig *gke.ClusterConfig) {
						clusterConfig.Locations = append(clusterConfig.Locations, helpers.GetGKEZone())
					}
				} else {
					region = ""
				}

				k8sVersion, err := helper.GetK8sVersion(ctx.RancherAdminClient, project, ctx.CloudCredID, zone, region, testData.isUpgrade)
				Expect(err).NotTo(HaveOccurred())
				GinkgoLogr.Info(fmt.Sprintf("Using K8s version %s for cluster %s", k8sVersion, clusterName))

				cluster, err = helper.CreateGKEHostedCluster(ctx.RancherAdminClient, clusterName, ctx.CloudCredID, k8sVersion, zone, region, project, updateFunc)
				Expect(err).To(BeNil())
				cluster, err = helpers.WaitUntilClusterIsReady(cluster, ctx.RancherAdminClient)
				Expect(err).To(BeNil())
			})
			AfterEach(func() {
				if ctx.ClusterCleanup {
					if cluster != nil && cluster.ID != "" {
						GinkgoLogr.Info(fmt.Sprintf("Cleaning up resource cluster: %s %s", cluster.Name, cluster.ID))
						err := helper.DeleteGKEHostCluster(cluster, ctx.RancherAdminClient)
						Expect(err).To(BeNil())
					}
				} else {
					fmt.Println("Skipping downstream cluster deletion: ", clusterName)
				}
			})

			It(testData.testTitle, func() {
				testCaseID = testData.qaseID
				testData.testBody(cluster, ctx.RancherAdminClient, clusterName)
			})

		})

	}

})
