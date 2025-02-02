/*
	(c) Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package main

import (
	"embed"
	"fmt"
	"github.com/openziti/fablab"
	"github.com/openziti/fablab/kernel/lib/actions/component"
	"github.com/openziti/fablab/kernel/lib/binding"
	"github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/aws_ssh_key"
	semaphore0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/semaphore"
	terraform_0 "github.com/openziti/fablab/kernel/lib/runlevel/0_infrastructure/terraform"
	"github.com/openziti/fablab/kernel/lib/runlevel/1_configuration/config"
	"github.com/openziti/fablab/kernel/lib/runlevel/2_kitting/devkit"
	distribution "github.com/openziti/fablab/kernel/lib/runlevel/3_distribution"
	"github.com/openziti/fablab/kernel/lib/runlevel/3_distribution/rsync"
	aws_ssh_key2 "github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/aws_ssh_key"
	"github.com/openziti/fablab/kernel/lib/runlevel/6_disposal/terraform"
	"github.com/openziti/fablab/kernel/model"
	"github.com/openziti/fablab/resources"
	"github.com/openziti/ziti/network-tests/ha/actions"
	"github.com/openziti/ziti/network-tests/test_resources"
	"github.com/openziti/zitilab"
	"github.com/openziti/zitilab/actions/edge"
	zitilib_runlevel_1_configuration "github.com/openziti/zitilab/runlevel/1_configuration"
	"github.com/sirupsen/logrus"
	"os"
	"time"
)

//go:embed configs
var configResource embed.FS

func getConfigData(filePath string) []byte {
	data, err := configResource.ReadFile(fmt.Sprintf("configs/%s", filePath))
	if err != nil {
		logrus.Errorf("Unable to read config data from %s: [%s]", filePath, err)
	}
	return data
}

var m = &model.Model{
	Id: "ha",
	Scope: model.Scope{
		Defaults: model.Variables{
			"environment": "ha-smoketest",
			"credentials": model.Variables{
				"ssh": model.Variables{
					"username": "ubuntu",
				},
				"edge": model.Variables{
					"username": "admin",
					"password": "admin",
				},
			},
		},
	},

	Resources: model.Resources{
		resources.Configs:   resources.SubFolder(configResource, "configs"),
		resources.Terraform: test_resources.TerraformResources(),
	},

	Regions: model.Regions{
		"us-east-1": {
			Region: "us-east-1",
			Site:   "us-east-1a",
			Hosts: model.Hosts{
				"ctrl1": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl1": {
							Scope:          model.Scope{Tags: model.Tags{"ctrl", "spiffe:controller"}},
							BinaryName:     "ziti controller",
							ConfigSrc:      "ctrl.yml.tmpl",
							ConfigName:     "ctrl1.yml",
							PublicIdentity: "ctrl1",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
				"ctrl2": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl2": {
							Scope:          model.Scope{Tags: model.Tags{"ctrl", "spiffe:controller"}},
							BinaryName:     "ziti controller",
							ConfigSrc:      "ctrl.yml.tmpl",
							ConfigName:     "ctrl2.yml",
							PublicIdentity: "ctrl2",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},

				"router-east": {
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-east": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator"}},
							BinaryName:     "ziti router",
							ConfigSrc:      "router.yml.tmpl",
							ConfigName:     "router-east.yml",
							PublicIdentity: "router-east",
						},
						"echo-server": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "service"}},
							BinaryName:     "echo-server",
							PublicIdentity: "echo-server",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
			},
		},
		"us-west-2": {
			Region: "us-west-2",
			Site:   "us-west-2b",
			Hosts: model.Hosts{
				"ctrl3": {
					InstanceType: "t3.micro",
					Components: model.Components{
						"ctrl3": {
							Scope:          model.Scope{Tags: model.Tags{"ctrl", "spiffe:controller"}},
							BinaryName:     "ziti controller",
							ConfigSrc:      "ctrl.yml.tmpl",
							ConfigName:     "ctrl3.yml",
							PublicIdentity: "ctrl3",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},

				"router-west": {
					Scope:        model.Scope{Tags: model.Tags{}},
					InstanceType: "t2.micro",
					Components: model.Components{
						"router-west": {
							Scope:          model.Scope{Tags: model.Tags{"edge-router", "terminator"}},
							BinaryName:     "ziti router",
							ConfigSrc:      "router.yml.tmpl",
							ConfigName:     "router-west.yml",
							PublicIdentity: "router-west",
						},
						"echo-client": {
							Scope:          model.Scope{Tags: model.Tags{"sdk-app", "client"}},
							BinaryName:     "echo-client",
							PublicIdentity: "echo-client",
						},
						"consul": {
							BinaryName: "consul",
						},
					},
				},
			},
		},
	},

	Actions: model.ActionBinders{
		"bootstrap": actions.NewBootstrapAction(),
		"start": actions.NewStartAction(actions.MetricbeatConfig{
			ConfigPath: "metricbeat",
			DataPath:   "metricbeat/data",
			LogPath:    "metricbeat/logs",
		},
			actions.ConsulConfig{
				ServerAddr: os.Getenv("CONSUL_ENDPOINT"),
				ConfigDir:  "consul",
				DataPath:   "consul/data",
				LogPath:    "consul/log.out",
			}),
		"stop":  model.Bind(component.StopInParallel("*", 15)),
		"login": model.Bind(edge.Login("#ctrl1")),
	},

	Infrastructure: model.InfrastructureStages{
		aws_ssh_key.Express(),
		terraform_0.Express(),
		semaphore0.Ready(time.Minute),
	},

	Configuration: model.ConfigurationStages{
		zitilib_runlevel_1_configuration.IfPkiNeedsRefresh(
			zitilib_runlevel_1_configuration.Fabric("simple-transfer.test", ".ctrl"),
		),
		config.Component(),
		devkit.DevKitF(zitilab.ZitiRoot, []string{"ziti", "ziti-echo"}),
	},

	Distribution: model.DistributionStages{
		distribution.DistributeSshKey("*"),
		distribution.Locations("*", "logs"),
		distribution.DistributeDataWithReplaceCallbacks(
			"*",
			string(getConfigData("metricbeat.yml")),
			"metricbeat/metricbeat.yml",
			os.FileMode(0644),
			map[string]func(*model.Host) string{
				"${host}": func(h *model.Host) string {
					return os.Getenv("ELASTIC_ENDPOINT")
				},
				"${user}": func(h *model.Host) string {
					return os.Getenv("ELASTIC_USERNAME")
				},
				"${password}": func(h *model.Host) string {
					return os.Getenv("ELASTIC_PASSWORD")
				},
				"${build_number}": func(h *model.Host) string {
					return os.Getenv("BUILD_NUMBER")
				},
				"${ziti_version}": func(h *model.Host) string {
					return h.MustStringVariable("ziti_version")
				},
			},
		),

		distribution.DistributeDataWithReplaceCallbacks(
			"*",
			string(getConfigData("consul.hcl")),
			"consul/consul.hcl",
			os.FileMode(0644),
			map[string]func(*model.Host) string{
				"${public_ip}": func(h *model.Host) string {
					return h.PublicIp
				},
				"${encryption_key}": func(h *model.Host) string {
					return os.Getenv("CONSUL_ENCRYPTION_KEY")
				},
				"${build_number}": func(h *model.Host) string {
					return os.Getenv("BUILD_NUMBER")
				},
				"${ziti_version}": func(h *model.Host) string {
					return h.MustStringVariable("ziti_version")
				},
			},
		),
		distribution.DistributeDataWithReplaceCallbacks(
			"#ctrl",
			string(getConfigData("ziti.hcl")),
			"consul/ziti.hcl",
			os.FileMode(0644),
			map[string]func(*model.Host) string{
				"${build_number}": func(h *model.Host) string {
					return os.Getenv("BUILD_NUMBER")
				},
				"${ziti_version}": func(h *model.Host) string {
					return h.MustStringVariable("ziti_version")
				},
			}),
		distribution.DistributeData(
			"*",
			[]byte(os.Getenv("CONSUL_AGENT_CERT")),
			"consul/consul-agent-ca.pem"),
		rsync.RsyncStaged(),
	},

	Disposal: model.DisposalStages{
		terraform.Dispose(),
		aws_ssh_key2.Dispose(),
	},
}

func main() {
	m.AddActivationActions("stop", "bootstrap", "start")

	model.AddBootstrapExtension(
		zitilab.BootstrapWithFallbacks(
			&zitilab.BootstrapFromEnv{},
		))
	model.AddBootstrapExtension(binding.AwsCredentialsLoader)
	model.AddBootstrapExtension(aws_ssh_key.KeyManager)

	fablab.InitModel(m)
	fablab.Run()
}
