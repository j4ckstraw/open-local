/*
Copyright © 2021 Alibaba Group Holding Ltd.

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

package csi

import (
	"github.com/alibaba/open-local/pkg/csi"
	"github.com/spf13/pflag"
)

type csiOption struct {
	Master                  string
	Kubeconfig              string
	Endpoint                string
	NodeID                  string
	Driver                  string
	SysPath                 string
	GrpcConnectionTimeout   int
	LVMDPort                string
	CgroupDriver            string
	DriverMode              string
	UseNodeHostname         bool
	ExtenderSchedulerNames  []string
	FrameworkSchedulerNames []string
}

func (option *csiOption) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&option.Kubeconfig, "kubeconfig", option.Kubeconfig, "Path to the kubeconfig file to use.")
	fs.StringVar(&option.Master, "master", option.Master, "URL/IP for master.")
	fs.StringVar(&option.Endpoint, "endpoint", csi.DefaultEndpoint, "the endpointof CSI")
	fs.StringVar(&option.NodeID, "nodeID", "", "the id of node")
	fs.StringVar(&option.Driver, "driver", csi.DefaultDriverName, "the name of CSI driver")
	fs.StringVar(&option.SysPath, "path.sysfs", "/host_sys", "Path of sysfs mountpoint")
	fs.IntVar(&option.GrpcConnectionTimeout, "grpc-connection-timeout", csi.DefaultConnectTimeout, "grpc connection timeout(second)")
	fs.StringVar(&option.LVMDPort, "lvmdPort", "1736", "Port of lvm daemon")
	fs.StringVar(&option.CgroupDriver, "cgroupDriver", "systemd", "the name of cgroup driver")
	fs.StringVar(&option.DriverMode, "driver-mode", "all", "driver mode")
	fs.BoolVar(&option.UseNodeHostname, "use-node-hostname", false, "use node hostname dns for grpc connection")
	fs.StringSliceVar(&option.ExtenderSchedulerNames, "extender-scheduler-names", []string{"default-scheduler"}, "extender scheduler names")
	fs.StringSliceVar(&option.FrameworkSchedulerNames, "framework-scheduler-names", []string{}, "framework scheduler names")
}
