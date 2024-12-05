/*
Copyright © 2022 - 2024 SUSE LLC

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

package e2e_test

import (
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
)

var (
	rancherHostname    string
	rancherChannel     string
	rancherHeadVersion string
	rancherVersion     string
	proxy              string
	proxyHost          string
	nightlyChart       string
	providerOperator   string
	kubeConfig         string
	k3sVersion         string
)

/**
 * Execute RunHelmBinaryWithCustomErr within a loop with timeout
 * @param s options to pass to RunHelmBinaryWithCustomErr command
 * @returns Nothing, the function will fail through Ginkgo in case of issue
 */
func RunHelmCmdWithRetry(s ...string) {
	Eventually(func() error {
		output, err := kubectl.RunHelmBinaryWithOutput(s...)
		GinkgoWriter.Write([]byte(output))
		if err != nil {
			return err
		}
		return nil
	}, tools.SetTimeout(2*time.Minute), 20*time.Second).Should(Not(HaveOccurred()))
}

func FailWithReport(message string, callerSkip ...int) {
	// Ensures the correct line numbers are reported
	Fail(message, callerSkip[0]+1)
}

func TestE2E(t *testing.T) {
	RegisterFailHandler(FailWithReport)
	RunSpecs(t, "Hosted Provider Rancher Environment Setup")
}

var _ = BeforeSuite(func() {
	// Extract environment variables
	rancherHostname = os.Getenv("RANCHER_HOSTNAME")
	Expect(rancherHostname).ToNot(BeEmpty(), "RANCHER_HOSTNAME environment variable is required")
	rancherVersion = os.Getenv("RANCHER_VERSION")
	Expect(rancherVersion).ToNot(BeEmpty(), "RANCHER_VERSION environment variable is required")
	kubeConfig = os.Getenv("KUBECONFIG")
	Expect(kubeConfig).ToNot(BeEmpty(), "KUBECONFIG environment variable is required")
	k3sVersion = os.Getenv("INSTALL_K3S_VERSION")
	Expect(k3sVersion).ToNot(BeEmpty(), "INSTALL_K3S_VERSION environment variable is required")
	proxy = os.Getenv("RANCHER_BEHIND_PROXY")
	proxyHost = os.Getenv("PROXY_HOST")
	if proxyHost == "" {
		proxyHost = "172.17.0.1:3128"
	}
	nightlyChart = os.Getenv("NIGHTLY_CHART")
	providerOperator = os.Getenv("PROVIDER")

	// Extract Rancher Manager channel/version to install
	s := strings.Split(rancherVersion, "/")
	Expect(len(s)).To(BeNumerically(">=", 2), "RANCHER_VERSION must contain at least two strings separated by '/'")
	rancherChannel = s[0]
	rancherVersion = s[1] // This can be either a string like "2.9.3[-rc4]", "devel", or "latest"
	if len(s) > 2 {
		rancherHeadVersion = s[2]
	}
})
